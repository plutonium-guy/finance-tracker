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

type recurringPageVM struct {
	base
	Rows    []service.RecurringRow
	Summary service.RecurringSummary
}

type recurringModalVM struct {
	Rec         *domain.RecurringItem
	Categories  []domain.Category
	Methods     []domain.PaymentMethod
	Frequencies []domain.Frequency
	Today       string
	Error       string
}

func (h *Handler) RecurringPage(w http.ResponseWriter, r *http.Request) {
	items, _ := h.svc.Store.ListRecurring()
	h.rdr.Page(w, "recurring", recurringPageVM{
		base:    h.base("Recurring", "recurring"),
		Rows:    h.svc.RecurringRows(items),
		Summary: service.SummarizeRecurring(items),
	})
}

func (h *Handler) RecurringList(w http.ResponseWriter, r *http.Request) {
	items, _ := h.svc.Store.ListRecurring()
	h.rdr.Fragment(w, "recurring_table", recurringPageVM{Rows: h.svc.RecurringRows(items)})
}

func (h *Handler) RecurringSummaryFragment(w http.ResponseWriter, r *http.Request) {
	items, _ := h.svc.Store.ListRecurring()
	h.rdr.Fragment(w, "recurring_summary", recurringPageVM{Summary: service.SummarizeRecurring(items)})
}

func (h *Handler) recModal(rec *domain.RecurringItem, errMsg string) recurringModalVM {
	cats, _ := h.svc.Store.ListCategories()
	return recurringModalVM{
		Rec: rec, Categories: cats, Methods: methods(), Frequencies: frequencies(),
		Today: h.svc.Now().Format("2006-01-02"), Error: errMsg,
	}
}

func (h *Handler) RecurringNew(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "recurring_modal", h.recModal(nil, ""))
}

func (h *Handler) RecurringEdit(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Store.GetRecurring(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.rdr.Fragment(w, "recurring_modal", h.recModal(&item, ""))
}

func (h *Handler) parseRecForm(r *http.Request, existing domain.RecurringItem) (domain.RecurringItem, error) {
	if err := r.ParseForm(); err != nil {
		return existing, err
	}
	it := existing
	it.Name = strings.TrimSpace(r.FormValue("name"))
	it.Category = r.FormValue("category")
	it.Frequency = domain.Frequency(r.FormValue("frequency"))
	it.Type = domain.TransactionType(r.FormValue("type"))
	it.StartDate = r.FormValue("start_date")
	it.PaymentMethod = domain.PaymentMethod(r.FormValue("payment_method"))
	it.Active = true
	if existing.ID != "" {
		it.Active = existing.Active
	}
	end := strings.TrimSpace(r.FormValue("end_date"))
	if end != "" {
		it.EndDate = &end
	} else {
		it.EndDate = nil
	}
	amt, err := strconv.ParseFloat(r.FormValue("amount"), 64)
	if err != nil || amt <= 0 {
		return it, errValidation("Amount must be positive")
	}
	it.Amount = domain.ToPaise(amt)
	if it.Name == "" {
		return it, errValidation("Name is required")
	}
	if !domain.IsValidFrequency(it.Frequency) {
		return it, errValidation("Invalid frequency")
	}
	if it.Type != domain.Income && it.Type != domain.Expense {
		return it, errValidation("Type must be Income or Expense")
	}
	if _, err := time.Parse("2006-01-02", it.StartDate); err != nil {
		return it, errValidation("Invalid start date")
	}
	if it.EndDate != nil && *it.EndDate < it.StartDate {
		return it, errValidation("End date must be on or after start date")
	}
	return it, nil
}

func (h *Handler) RecurringCreate(w http.ResponseWriter, r *http.Request) {
	it, err := h.parseRecForm(r, domain.RecurringItem{})
	if err != nil {
		h.modalError(w, "recurring_modal", h.recModal(nil, err.Error()))
		return
	}
	now := h.svc.Now().UTC().Format(time.RFC3339)
	it.ID = uuid.NewString()
	it.CreatedAt, it.UpdatedAt = now, now
	if err := h.svc.Store.CreateRecurring(it); err != nil {
		h.modalError(w, "recurring_modal", h.recModal(nil, friendly(err)))
		return
	}
	recTrigger(w)
	h.rdr.Fragment(w, "recurring_row", service.RecurringRow{Item: it, Calc: service.Calc(it, h.svc.Now())})
}

func (h *Handler) RecurringUpdate(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetRecurring(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	it, err := h.parseRecForm(r, existing)
	if err != nil {
		h.modalError(w, "recurring_modal", h.recModal(&existing, err.Error()))
		return
	}
	it.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
	if err := h.svc.Store.UpdateRecurring(it); err != nil {
		h.modalError(w, "recurring_modal", h.recModal(&existing, friendly(err)))
		return
	}
	recTrigger(w)
	h.rdr.Fragment(w, "recurring_row", service.RecurringRow{Item: it, Calc: service.Calc(it, h.svc.Now())})
}

func (h *Handler) RecurringDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteRecurring(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	recTrigger(w)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) RecurringToggle(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Store.ToggleRecurring(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	recTrigger(w)
	h.rdr.Fragment(w, "recurring_row", service.RecurringRow{Item: item, Calc: service.Calc(item, h.svc.Now())})
}
