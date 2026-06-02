package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/store"
)

// apiV1Routes mounts the versioned JSON REST API. Every endpoint is gated by
// the bearer token (API_PUSH_TOKEN).
func (h *Handler) apiV1Routes(r chi.Router) {
	r.Use(h.requireAPIToken)

	r.Get("/transactions", h.apiListTransactions)
	r.Post("/transactions", h.apiCreateTransaction)
	r.Get("/transactions/{id}", h.apiGetTransaction)
	r.Put("/transactions/{id}", h.apiUpdateTransaction)
	r.Delete("/transactions/{id}", h.apiDeleteTransaction)

	r.Get("/accounts", h.apiListAccounts)
	r.Post("/accounts", h.apiCreateAccount)
	r.Get("/accounts/{id}", h.apiGetAccount)
	r.Put("/accounts/{id}", h.apiUpdateAccount)
	r.Delete("/accounts/{id}", h.apiDeleteAccount)

	r.Get("/cards", h.apiListCards)
	r.Post("/cards", h.apiCreateCard)
	r.Get("/cards/{id}", h.apiGetCard)
	r.Put("/cards/{id}", h.apiUpdateCard)
	r.Delete("/cards/{id}", h.apiDeleteCard)

	r.Get("/holdings", h.apiListHoldings)
	r.Post("/holdings", h.apiCreateHolding)
	r.Get("/holdings/{id}", h.apiGetHolding)
	r.Put("/holdings/{id}", h.apiUpdateHolding)
	r.Delete("/holdings/{id}", h.apiDeleteHolding)

	r.Get("/recurring", h.apiListRecurring)
	r.Post("/recurring", h.apiCreateRecurring)
	r.Get("/recurring/{id}", h.apiGetRecurring)
	r.Put("/recurring/{id}", h.apiUpdateRecurring)
	r.Delete("/recurring/{id}", h.apiDeleteRecurring)

	r.Get("/goals", h.apiListGoals)
	r.Post("/goals", h.apiCreateGoal)
	r.Get("/goals/{id}", h.apiGetGoal)
	r.Put("/goals/{id}", h.apiUpdateGoal)
	r.Delete("/goals/{id}", h.apiDeleteGoal)

	r.Get("/budgets", h.apiListBudgets)
	r.Put("/budgets", h.apiSetBudget)
	r.Delete("/budgets/{category}", h.apiDeleteBudget)

	r.Get("/categories", h.apiListCategories)
	r.Post("/categories", h.apiCreateCategory)

	r.Get("/net-worth", h.apiNetWorth)

	// Actions (also available at the legacy /api/* paths).
	r.Post("/gmail/sync", h.GmailSyncAPI)
	r.Post("/nav/sync", h.NavSyncAPI)
	r.Post("/alerts/run", h.AlertsRun)
}

// requireAPIToken gates a route group on the bearer token.
func (h *Handler) requireAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.apiToken == "" {
			writeJSON(w, http.StatusServiceUnavailable, errBody("api not configured (set API_PUSH_TOKEN)"))
			return
		}
		if !h.apiAuthorized(r) {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func errBody(msg string) map[string]any { return map[string]any{"error": msg} }

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v)
}

// notFoundOr writes 404 for ErrNotFound, else 500.
func writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	writeJSON(w, http.StatusBadRequest, errBody(friendly(err)))
}

func (h *Handler) nowISO() string { return h.svc.Now().UTC().Format(time.RFC3339) }

// ---- transactions ----

type apiTxn struct {
	domain.Transaction
	Tags      []string `json:"tags,omitempty"`
	CardID    string   `json:"card_id,omitempty"`
	AccountID string   `json:"account_id,omitempty"`
}

func (h *Handler) apiListTransactions(w http.ResponseWriter, r *http.Request) {
	txs, total, err := h.svc.Store.Transactions(h.filterFrom(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, txs)
}

func (h *Handler) apiGetTransaction(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Store.GetTransaction(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	tags, _ := h.svc.Store.TagsFor(t.ID)
	card, _ := h.svc.Store.CardOfTransaction(t.ID)
	acct, _ := h.svc.Store.AccountOfTransaction(t.ID)
	writeJSON(w, http.StatusOK, apiTxn{Transaction: t, Tags: tags, CardID: card, AccountID: acct})
}

func (h *Handler) validateTxn(t domain.Transaction) error {
	if t.Amount <= 0 {
		return errValidation("amount must be > 0 (paise)")
	}
	if t.Description == "" || len(t.Description) > 200 {
		return errValidation("description is required (≤200 chars)")
	}
	if !domain.IsValidType(t.Type) {
		return errValidation("invalid type (Income|Expense|Transfer)")
	}
	if !domain.IsValidPaymentMethod(t.PaymentMethod) {
		return errValidation("invalid payment_method")
	}
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return errValidation("invalid date (want YYYY-MM-DD)")
	}
	return nil
}

func (h *Handler) apiCreateTransaction(w http.ResponseWriter, r *http.Request) {
	var in apiTxn
	if err := readJSON(w, r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	t := in.Transaction
	if t.Type == "" {
		t.Type = domain.Expense
	}
	if t.PaymentMethod == "" {
		t.PaymentMethod = domain.UPI
	}
	if t.Category == "" {
		t.Category = h.defaultCategory()
	}
	if t.Date == "" {
		t.Date = h.svc.Now().Format("2006-01-02")
	}
	if err := h.validateTxn(t); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	t.ID = uuid.NewString()
	t.CreatedAt, t.UpdatedAt = h.nowISO(), h.nowISO()
	if err := h.svc.Store.CreateTransaction(t); err != nil {
		writeStoreErr(w, err)
		return
	}
	h.svc.Store.SetTransactionTags(t.ID, in.Tags)
	if in.CardID != "" {
		h.svc.Store.SetTransactionCard(t.ID, in.CardID)
	}
	if in.AccountID != "" {
		h.svc.Store.SetTransactionAccount(t.ID, in.AccountID)
	}
	writeJSON(w, http.StatusCreated, apiTxn{Transaction: t, Tags: in.Tags, CardID: in.CardID, AccountID: in.AccountID})
}

func (h *Handler) apiUpdateTransaction(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetTransaction(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	in := apiTxn{Transaction: existing}
	if err := readJSON(w, r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	t := in.Transaction
	t.ID = existing.ID
	if err := h.validateTxn(t); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	t.UpdatedAt = h.nowISO()
	if err := h.svc.Store.UpdateTransaction(t); err != nil {
		writeStoreErr(w, err)
		return
	}
	if in.Tags != nil {
		h.svc.Store.SetTransactionTags(t.ID, in.Tags)
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) apiDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteTransaction(chi.URLParam(r, "id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- accounts ----

func (h *Handler) apiListAccounts(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) apiGetAccount(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Store.GetAccount(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) apiCreateAccount(w http.ResponseWriter, r *http.Request) {
	var a domain.Account
	if err := readJSON(w, r, &a); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if a.Name == "" || !domain.IsValidAccountType(a.Type) {
		writeJSON(w, http.StatusBadRequest, errBody("name and a valid type are required"))
		return
	}
	a.ID = uuid.NewString()
	a.CreatedAt, a.UpdatedAt = h.nowISO(), h.nowISO()
	if err := h.svc.Store.CreateAccount(a); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handler) apiUpdateAccount(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetAccount(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	a := existing
	if err := readJSON(w, r, &a); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	a.ID = existing.ID
	if a.Name == "" || !domain.IsValidAccountType(a.Type) {
		writeJSON(w, http.StatusBadRequest, errBody("name and a valid type are required"))
		return
	}
	a.UpdatedAt = h.nowISO()
	if err := h.svc.Store.UpdateAccount(a); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) apiDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteAccount(chi.URLParam(r, "id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- cards ----

func (h *Handler) apiListCards(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Store.ListCards()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) apiGetCard(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Store.GetCard(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func validateCard(c domain.Card) error {
	if c.Name == "" {
		return errValidation("name is required")
	}
	if c.StatementDay < 1 || c.StatementDay > 28 {
		return errValidation("statement_day must be 1..28")
	}
	if c.DueOffsetDays < 0 || c.DueOffsetDays > 60 {
		return errValidation("due_offset_days must be 0..60")
	}
	return nil
}

func (h *Handler) apiCreateCard(w http.ResponseWriter, r *http.Request) {
	var c domain.Card
	if err := readJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if err := validateCard(c); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	c.ID = uuid.NewString()
	c.CreatedAt, c.UpdatedAt = h.nowISO(), h.nowISO()
	if err := h.svc.Store.CreateCard(c); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) apiUpdateCard(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetCard(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	c := existing
	if err := readJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	c.ID = existing.ID
	if err := validateCard(c); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	c.UpdatedAt = h.nowISO()
	if err := h.svc.Store.UpdateCard(c); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) apiDeleteCard(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteCard(chi.URLParam(r, "id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- holdings ----

func (h *Handler) apiListHoldings(w http.ResponseWriter, r *http.Request) {
	hd, err := h.svc.Store.ListHoldings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, hd)
}

func (h *Handler) apiGetHolding(w http.ResponseWriter, r *http.Request) {
	hd, err := h.svc.Store.GetHolding(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hd)
}

func validateHolding(hd domain.Holding) error {
	if hd.Name == "" {
		return errValidation("name is required")
	}
	if !domain.IsValidAssetType(hd.Type) {
		return errValidation("invalid asset type")
	}
	if hd.UnitsMicro <= 0 {
		return errValidation("units_micro must be > 0")
	}
	return nil
}

func (h *Handler) apiCreateHolding(w http.ResponseWriter, r *http.Request) {
	var hd domain.Holding
	if err := readJSON(w, r, &hd); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if err := validateHolding(hd); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	hd.ID = uuid.NewString()
	hd.CreatedAt, hd.UpdatedAt = h.nowISO(), h.nowISO()
	if err := h.svc.Store.CreateHolding(hd); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, hd)
}

func (h *Handler) apiUpdateHolding(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetHolding(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	hd := existing
	if err := readJSON(w, r, &hd); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	hd.ID = existing.ID
	if err := validateHolding(hd); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	hd.UpdatedAt = h.nowISO()
	if err := h.svc.Store.UpdateHolding(hd); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hd)
}

func (h *Handler) apiDeleteHolding(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteHolding(chi.URLParam(r, "id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- recurring ----

func (h *Handler) apiListRecurring(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Store.ListRecurring()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) apiGetRecurring(w http.ResponseWriter, r *http.Request) {
	it, err := h.svc.Store.GetRecurring(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func validateRecurring(r domain.RecurringItem) error {
	if r.Name == "" {
		return errValidation("name is required")
	}
	if r.Amount <= 0 {
		return errValidation("amount must be > 0 (paise)")
	}
	if !domain.IsValidType(r.Type) {
		return errValidation("invalid type")
	}
	if !domain.IsValidFrequency(r.Frequency) {
		return errValidation("invalid frequency")
	}
	if _, err := time.Parse("2006-01-02", r.StartDate); err != nil {
		return errValidation("invalid start_date")
	}
	return nil
}

func (h *Handler) apiCreateRecurring(w http.ResponseWriter, r *http.Request) {
	var it domain.RecurringItem
	if err := readJSON(w, r, &it); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if err := validateRecurring(it); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	it.ID = uuid.NewString()
	it.CreatedAt, it.UpdatedAt = h.nowISO(), h.nowISO()
	if err := h.svc.Store.CreateRecurring(it); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (h *Handler) apiUpdateRecurring(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetRecurring(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	it := existing
	if err := readJSON(w, r, &it); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	it.ID = existing.ID
	if err := validateRecurring(it); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	it.UpdatedAt = h.nowISO()
	if err := h.svc.Store.UpdateRecurring(it); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (h *Handler) apiDeleteRecurring(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteRecurring(chi.URLParam(r, "id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- goals ----

func (h *Handler) apiListGoals(w http.ResponseWriter, r *http.Request) {
	g, err := h.svc.Store.ListGoals()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) apiGetGoal(w http.ResponseWriter, r *http.Request) {
	g, err := h.svc.Store.GetGoal(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) apiCreateGoal(w http.ResponseWriter, r *http.Request) {
	var g domain.Goal
	if err := readJSON(w, r, &g); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if g.Name == "" || g.Target <= 0 {
		writeJSON(w, http.StatusBadRequest, errBody("name and a positive target (paise) are required"))
		return
	}
	g.ID = uuid.NewString()
	g.CreatedAt, g.UpdatedAt = h.nowISO(), h.nowISO()
	if err := h.svc.Store.CreateGoal(g); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) apiUpdateGoal(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetGoal(chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	g := existing
	if err := readJSON(w, r, &g); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	g.ID = existing.ID
	if g.Name == "" || g.Target <= 0 {
		writeJSON(w, http.StatusBadRequest, errBody("name and a positive target (paise) are required"))
		return
	}
	g.UpdatedAt = h.nowISO()
	if err := h.svc.Store.UpdateGoal(g); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) apiDeleteGoal(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteGoal(chi.URLParam(r, "id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- budgets ----

func (h *Handler) apiListBudgets(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.Store.ListBudgets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) apiSetBudget(w http.ResponseWriter, r *http.Request) {
	var b domain.Budget
	if err := readJSON(w, r, &b); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if b.Category == "" || b.Limit < 0 {
		writeJSON(w, http.StatusBadRequest, errBody("category and a non-negative limit (paise) are required"))
		return
	}
	if err := h.svc.Store.SetBudget(b); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) apiDeleteBudget(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteBudget(chi.URLParam(r, "category")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- categories ----

func (h *Handler) apiListCategories(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Store.ListCategories()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) apiCreateCategory(w http.ResponseWriter, r *http.Request) {
	var c domain.Category
	if err := readJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}
	if !validCategoryName(c.Name) {
		writeJSON(w, http.StatusBadRequest, errBody("invalid category name (no '/' or '\\', max 40 chars)"))
		return
	}
	if err := h.svc.Store.AddCategory(c.Name); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ---- net worth ----

func (h *Handler) apiNetWorth(w http.ResponseWriter, r *http.Request) {
	_, nw := h.accountData()
	writeJSON(w, http.StatusOK, nw)
}
