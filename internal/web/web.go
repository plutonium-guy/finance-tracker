package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

// Pusher sends an outbound message to a Telegram chat (satisfied by the bot client).
type Pusher interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// GmailSyncer runs a Gmail credit-card import on demand.
type GmailSyncer interface {
	Run(ctx context.Context) (GmailResult, error)
}

// GmailResult mirrors gmailsync.Result so the web layer avoids importing it directly.
type GmailResult struct {
	Scanned  int `json:"scanned"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// Handler wires the service and renderer to HTTP.
type Handler struct {
	svc *service.Service
	rdr *Renderer

	apiToken string // bearer token gating the protected /api/* endpoints

	// Outbound push API (POST /api/push).
	pusher   Pusher
	pushChat int64 // default target chat when the request omits one

	// Gmail import (POST /gmail/sync, POST /api/gmail/sync).
	gmail GmailSyncer
}

// NewHandler builds a Handler.
func NewHandler(svc *service.Service, rdr *Renderer) *Handler {
	return &Handler{svc: svc, rdr: rdr}
}

// SetAPIToken sets the bearer token required by the protected /api/* endpoints.
func (h *Handler) SetAPIToken(token string) { h.apiToken = token }

// EnablePush turns on POST /api/push, sending via p, defaulting to defaultChat.
func (h *Handler) EnablePush(p Pusher, defaultChat int64) {
	h.pusher = p
	h.pushChat = defaultChat
}

// EnableGmail wires the Gmail importer for the sync endpoints.
func (h *Handler) EnableGmail(s GmailSyncer) { h.gmail = s }

// base is the common layout context embedded by every page view model.
type base struct {
	Title    string
	Active   string
	Settings domain.AppSettings
}

func (h *Handler) base(title, active string) base {
	s, _ := h.svc.Store.GetSettings()
	return base{Title: title, Active: active, Settings: s}
}

// Routes builds the chi router with middleware and all endpoints.
func (h *Handler) Routes(static http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(skipLogging("/healthz", middleware.Logger))
	r.Use(middleware.Compress(5))

	r.Handle("/static/*", http.StripPrefix("/static/", static))

	r.Get("/healthz", h.Health)
	r.Post("/api/push", h.Push)
	r.Post("/api/transactions", h.TransactionCreateAPI)
	r.Post("/api/gmail/sync", h.GmailSyncAPI)
	r.Post("/api/alerts/run", h.AlertsRun)
	r.Post("/gmail/sync", h.GmailSyncUI)
	r.Get("/", h.Dashboard)
	r.Get("/partials/kpis", h.PartialKPIs)
	r.Get("/partials/paycycle", h.PartialPayCycle)
	r.Get("/partials/charts", h.PartialCharts)
	r.Get("/partials/recent", h.PartialRecent)
	r.Get("/partials/forecast", h.PartialForecast)

	r.Route("/transactions", func(r chi.Router) {
		r.Get("/", h.TransactionsPage)
		r.Get("/list", h.TransactionsList)
		r.Get("/new", h.TransactionNew)
		r.Get("/export", h.TransactionsExport)
		r.Post("/", h.TransactionCreate)
		r.Post("/import", h.TransactionsImport)
		r.Post("/bulk", h.TransactionsBulk)
		r.Get("/{id}/edit", h.TransactionEdit)
		r.Put("/{id}", h.TransactionUpdate)
		r.Delete("/{id}", h.TransactionDelete)
		r.Post("/{id}/duplicate", h.TransactionDuplicate)
	})

	r.Route("/recurring", func(r chi.Router) {
		r.Get("/", h.RecurringPage)
		r.Get("/list", h.RecurringList)
		r.Get("/new", h.RecurringNew)
		r.Get("/summary", h.RecurringSummaryFragment)
		r.Post("/", h.RecurringCreate)
		r.Post("/fire", h.RecurringFire)
		r.Get("/{id}/edit", h.RecurringEdit)
		r.Put("/{id}", h.RecurringUpdate)
		r.Delete("/{id}", h.RecurringDelete)
		r.Post("/{id}/toggle", h.RecurringToggle)
	})

	r.Get("/month", h.MonthPage)
	r.Get("/month/detail", h.MonthDetail)

	// Planning: goals + budgets.
	r.Get("/planning", h.PlanningPage)
	r.Route("/goals", func(r chi.Router) {
		r.Get("/list", h.GoalsList)
		r.Get("/new", h.GoalNew)
		r.Post("/", h.GoalCreate)
		r.Get("/{id}/edit", h.GoalEdit)
		r.Put("/{id}", h.GoalUpdate)
		r.Delete("/{id}", h.GoalDelete)
		r.Post("/{id}/add", h.GoalAddSaved)
	})
	r.Get("/budgets", h.BudgetsList)
	r.Post("/budgets", h.BudgetSet)
	r.Post("/budgets/{category}/delete", h.BudgetDelete)

	// Credit cards + billing cycles.
	r.Route("/cards", func(r chi.Router) {
		r.Get("/", h.CardsPage)
		r.Get("/list", h.CardsList)
		r.Get("/new", h.CardNew)
		r.Post("/", h.CardCreate)
		r.Get("/{id}/edit", h.CardEdit)
		r.Put("/{id}", h.CardUpdate)
		r.Delete("/{id}", h.CardDelete)
		r.Post("/{id}/pay", h.CardPay)
	})

	// Year-end sankey.
	r.Get("/year", h.YearPage)
	r.Get("/year/detail", h.YearDetail)

	r.Get("/settings", h.SettingsPage)
	r.Post("/settings", h.SettingsSave)
	r.Get("/categories", h.CategoriesList)
	r.Post("/categories", h.CategoryAdd)
	r.Post("/categories/{name}/rename", h.CategoryRename)
	r.Post("/categories/{name}/delete", h.CategoryDelete)

	r.Get("/backup", h.Backup)
	r.Post("/restore", h.Restore)
	r.Post("/reset", h.Reset)

	return r
}

// skipLogging applies the logger middleware to every request except those whose
// path matches, so noisy health probes stay out of the access log.
func skipLogging(path string, logger func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		logged := logger(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == path {
				next.ServeHTTP(w, r)
				return
			}
			logged.ServeHTTP(w, r)
		})
	}
}

type pushRequest struct {
	Text   string `json:"text"`
	ChatID int64  `json:"chat_id"`
}

// Push sends a message out to Telegram via the bot. It accepts JSON
// {"text":"...","chat_id":123} or form fields text/chat_id. chat_id is optional
// when a default chat is configured. Requires a bearer token.
//
//	curl -X POST http://host:8080/api/push \
//	  -H 'Authorization: Bearer $API_PUSH_TOKEN' \
//	  -H 'Content-Type: application/json' \
//	  -d '{"text":"deploy finished ✅"}'
func (h *Handler) Push(w http.ResponseWriter, r *http.Request) {
	if h.pusher == nil || h.apiToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "telegram push not configured"})
		return
	}
	if !h.apiAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}

	var req pushRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
			return
		}
	} else {
		r.ParseForm()
		req.Text = r.FormValue("text")
		req.ChatID, _ = strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
	}

	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text is required"})
		return
	}
	chat := req.ChatID
	if chat == 0 {
		chat = h.pushChat
	}
	if chat == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chat_id required (no default chat configured)"})
		return
	}
	if err := h.pusher.SendMessage(r.Context(), chat, req.Text); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "chat_id": chat})
}

// apiAuthorized checks the bearer token (Authorization: Bearer ... or X-Api-Token).
func (h *Handler) apiAuthorized(r *http.Request) bool {
	if h.apiToken == "" {
		return false
	}
	if t := r.Header.Get("X-Api-Token"); t != "" {
		return subtleEqual(t, h.apiToken)
	}
	const p = "Bearer "
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, p) {
		return subtleEqual(strings.TrimPrefix(auth, p), h.apiToken)
	}
	return false
}

// GmailSyncAPI runs a Gmail import and returns JSON. Token-protected, for cron/external use:
//
//	curl -X POST http://host:8080/api/gmail/sync -H 'Authorization: Bearer $API_PUSH_TOKEN'
func (h *Handler) GmailSyncAPI(w http.ResponseWriter, r *http.Request) {
	if h.gmail == nil || h.apiToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "gmail sync not configured"})
		return
	}
	if !h.apiAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	res, err := h.gmail.Run(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GmailSyncUI runs a Gmail import from the app's own UI (same-origin, no token)
// and returns a flash fragment.
func (h *Handler) GmailSyncUI(w http.ResponseWriter, r *http.Request) {
	if h.gmail == nil {
		h.flash(w, "Gmail sync is not configured")
		return
	}
	res, err := h.gmail.Run(r.Context())
	if err != nil {
		h.flash(w, "Gmail sync failed: "+err.Error())
		return
	}
	txTrigger(w)
	h.flash(w, fmtGmailFlash(res))
}

func fmtGmailFlash(res GmailResult) string {
	return "Gmail sync: imported " + strconv.Itoa(res.Imported) +
		" of " + strconv.Itoa(res.Scanned) + " scanned (" + strconv.Itoa(res.Skipped) + " skipped)"
}

func subtleEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Health is a liveness/readiness probe: 200 when the database is reachable,
// 503 otherwise. Used by the container HEALTHCHECK.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := h.svc.Store.GetSettings(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"unhealthy"}`))
		return
	}
	w.Write([]byte(`{"status":"ok"}`))
}

// ---- shared helpers ----

func (h *Handler) currentMonthOr(r *http.Request) string {
	if m := r.URL.Query().Get("m"); m != "" {
		return m
	}
	return h.svc.CurrentMonth()
}

func txTrigger(w http.ResponseWriter) { w.Header().Set("HX-Trigger", "txChanged") }
func recTrigger(w http.ResponseWriter) {
	w.Header().Set("HX-Trigger", "recChanged")
}

func methods() []domain.PaymentMethod { return domain.ValidPaymentMethods }
func types() []domain.TransactionType { return domain.ValidTransactionTypes }
func frequencies() []domain.Frequency { return domain.ValidFrequencies }

// parseTxForm reads and validates a transaction from form values. existing is
// the prior row for updates (carries ID/CreatedAt), or zero for new.
func (h *Handler) parseTxForm(r *http.Request, existing domain.Transaction) (domain.Transaction, error) {
	if err := r.ParseForm(); err != nil {
		return existing, err
	}
	t := existing
	t.Date = strings.TrimSpace(r.FormValue("date"))
	t.Description = strings.TrimSpace(r.FormValue("description"))
	t.Type = domain.TransactionType(r.FormValue("type"))
	t.PaymentMethod = domain.PaymentMethod(r.FormValue("payment_method"))
	t.Category = r.FormValue("category")
	notes := strings.TrimSpace(r.FormValue("notes"))
	if notes != "" {
		t.Notes = &notes
	} else {
		t.Notes = nil
	}

	amt, err := strconv.ParseFloat(r.FormValue("amount"), 64)
	if err != nil || amt <= 0 {
		return t, errValidation("Amount must be a positive number")
	}
	t.Amount = domain.ToPaise(amt)
	if t.Description == "" || len(t.Description) > 200 {
		return t, errValidation("Description is required (≤200 chars)")
	}
	if !domain.IsValidType(t.Type) {
		return t, errValidation("Invalid type")
	}
	if !domain.IsValidPaymentMethod(t.PaymentMethod) {
		return t, errValidation("Invalid payment method")
	}
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return t, errValidation("Invalid date")
	}
	return t, nil
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func errValidation(m string) error      { return validationError{m} }
