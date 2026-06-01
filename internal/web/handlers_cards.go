package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

type cardsPageVM struct {
	base
	Cards []service.CardSummary
}

type cardsListVM struct {
	Cards []service.CardSummary
}

type cardModalVM struct {
	Card  *domain.Card
	Error string
}

// cardSummaries builds the cycle/statement view for every card.
func (h *Handler) cardSummaries() []service.CardSummary {
	cards, _ := h.svc.Store.ListCards()
	txs, _ := h.svc.Store.AllTransactions()
	txCard, _ := h.svc.Store.TransactionCardMap()
	out := make([]service.CardSummary, 0, len(cards))
	for _, c := range cards {
		pays, _ := h.svc.Store.ListStatementPayments(c.ID)
		out = append(out, service.CardSummaryFor(c, txs, txCard, pays, h.svc.Now()))
	}
	return out
}

func (h *Handler) CardsPage(w http.ResponseWriter, r *http.Request) {
	h.rdr.Page(w, "cards", cardsPageVM{base: h.base("Cards", "cards"), Cards: h.cardSummaries()})
}

func (h *Handler) CardsList(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "cards_list", cardsListVM{Cards: h.cardSummaries()})
}

func (h *Handler) CardNew(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "card_modal", cardModalVM{})
}

func (h *Handler) CardEdit(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Store.GetCard(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.rdr.Fragment(w, "card_modal", cardModalVM{Card: &c})
}

func (h *Handler) parseCardForm(r *http.Request, existing domain.Card) (domain.Card, error) {
	if err := r.ParseForm(); err != nil {
		return existing, err
	}
	c := existing
	c.Name = strings.TrimSpace(r.FormValue("name"))
	if c.Name == "" {
		return c, errValidation("Name is required")
	}
	c.Last4 = digitsOnly(r.FormValue("last4"))
	if len(c.Last4) > 4 {
		c.Last4 = c.Last4[len(c.Last4)-4:]
	}
	limit, _ := strconv.ParseFloat(r.FormValue("limit"), 64)
	if limit < 0 {
		limit = 0
	}
	c.Limit = domain.ToPaise(limit)
	day, _ := strconv.Atoi(r.FormValue("statement_day"))
	if day < 1 || day > 28 {
		return c, errValidation("Statement day must be between 1 and 28")
	}
	c.StatementDay = day
	due, _ := strconv.Atoi(r.FormValue("due_offset_days"))
	if due < 0 || due > 60 {
		return c, errValidation("Due offset must be between 0 and 60 days")
	}
	c.DueOffsetDays = due
	return c, nil
}

func (h *Handler) CardCreate(w http.ResponseWriter, r *http.Request) {
	c, err := h.parseCardForm(r, domain.Card{})
	if err != nil {
		h.modalError(w, "card_modal", cardModalVM{Error: err.Error()})
		return
	}
	now := h.svc.Now().UTC().Format(time.RFC3339)
	c.ID = uuid.NewString()
	c.CreatedAt, c.UpdatedAt = now, now
	if err := h.svc.Store.CreateCard(c); err != nil {
		h.modalError(w, "card_modal", cardModalVM{Error: friendly(err)})
		return
	}
	h.CardsList(w, r)
}

func (h *Handler) CardUpdate(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetCard(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	c, err := h.parseCardForm(r, existing)
	if err != nil {
		h.modalError(w, "card_modal", cardModalVM{Card: &existing, Error: err.Error()})
		return
	}
	c.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
	if err := h.svc.Store.UpdateCard(c); err != nil {
		h.modalError(w, "card_modal", cardModalVM{Card: &existing, Error: friendly(err)})
		return
	}
	h.CardsList(w, r)
}

func (h *Handler) CardDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteCard(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	txTrigger(w)
	h.CardsList(w, r)
}

// CardPay marks the card's most recent closed statement as paid: it records a
// Type=Transfer transaction (no effect on income/expense totals) and a payment
// row that draws down the outstanding balance. Idempotent per statement.
func (h *Handler) CardPay(w http.ResponseWriter, r *http.Request) {
	card, err := h.svc.Store.GetCard(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	txs, _ := h.svc.Store.AllTransactions()
	txCard, _ := h.svc.Store.TransactionCardMap()
	pays, _ := h.svc.Store.ListStatementPayments(card.ID)
	sum := service.CardSummaryFor(card, txs, txCard, pays, h.svc.Now())

	if !sum.HasStatement {
		w.Header().Set("HX-Retarget", "#flash")
		h.flash(w, "No closed statement to pay for "+card.Name)
		return
	}
	if sum.Paid {
		w.Header().Set("HX-Retarget", "#flash")
		h.flash(w, card.Name+" statement is already paid")
		return
	}

	now := h.svc.Now().UTC().Format(time.RFC3339)
	tx := domain.Transaction{
		ID:            uuid.NewString(),
		Date:          h.svc.Now().Format("2006-01-02"),
		Description:   card.Name + " statement payment",
		Amount:        sum.LastStatementAmount,
		Type:          domain.Transfer,
		PaymentMethod: domain.BankTransfer,
		Category:      h.defaultCategory(),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.svc.Store.CreateTransaction(tx); err != nil {
		w.Header().Set("HX-Retarget", "#flash")
		h.flash(w, "Could not record payment: "+friendly(err))
		return
	}
	ok, _ := h.svc.Store.RecordStatementPayment(domain.StatementPayment{
		CardID:    card.ID,
		PeriodEnd: sum.LastStatementDate.Format("2006-01-02"),
		Amount:    sum.LastStatementAmount,
		TxID:      tx.ID,
		PaidAt:    now,
	})
	if !ok {
		// Lost a race: the statement was already marked paid. Undo the transfer.
		h.svc.Store.DeleteTransaction(tx.ID)
	}
	txTrigger(w)
	h.CardsList(w, r)
}

// defaultCategory returns "Miscellaneous" when present, else the first category.
func (h *Handler) defaultCategory() string {
	cats, _ := h.svc.Store.ListCategories()
	for _, c := range cats {
		if c.Name == "Miscellaneous" {
			return c.Name
		}
	}
	if len(cats) > 0 {
		return cats[0].Name
	}
	return "Miscellaneous"
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
