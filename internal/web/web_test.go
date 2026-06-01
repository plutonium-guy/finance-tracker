package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"finance-tracker/internal/service"
	"finance-tracker/internal/store"
)

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func postMultipartCSV(t *testing.T, h http.Handler, path, csv string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "import.csv")
	fw.Write([]byte(csv))
	mw.Close()
	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newTestServer(t *testing.T) (http.Handler, *store.SQLite) {
	t.Helper()
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, fn := range []func() error{db.Migrate, db.EnsureSettings, db.SeedDefaultCategories} {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	clock := func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) }
	svc := service.New(db, clock)
	if err := svc.DemoSeed(); err != nil {
		t.Fatal(err)
	}
	rdr, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(svc, rdr)
	return h.Routes(http.FileServer(http.FS(StaticFS()))), db
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDashboardRenders(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"This Month Debits", "₹111.00", "₹2,32,942.00", `id="chart-data"`, "chartIncomeExpense"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestPagesReturn200(t *testing.T) {
	h, _ := newTestServer(t)
	for _, p := range []string{"/transactions", "/recurring", "/month", "/settings",
		"/partials/kpis", "/partials/paycycle", "/partials/charts", "/partials/recent",
		"/transactions/list", "/recurring/list", "/recurring/summary", "/month/detail?m=2026-05"} {
		if rec := get(t, h, p); rec.Code != 200 {
			t.Errorf("GET %s = %d, want 200", p, rec.Code)
		}
	}
}

func TestCreateTransactionTriggersTxChanged(t *testing.T) {
	h, db := newTestServer(t)
	form := url.Values{
		"date":           {"2026-06-05"},
		"description":    {"Coffee"},
		"amount":         {"250.50"},
		"type":           {"Expense"},
		"payment_method": {"UPI"},
		"category":       {"Food & Dining"},
	}
	req := httptest.NewRequest("POST", "/transactions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("HX-Trigger") != "txChanged" {
		t.Errorf("HX-Trigger = %q, want txChanged", rec.Header().Get("HX-Trigger"))
	}
	if !strings.Contains(rec.Body.String(), "Coffee") || !strings.Contains(rec.Body.String(), "₹250.50") {
		t.Errorf("response missing new row: %s", rec.Body.String())
	}
	rows, _ := db.AllTransactions()
	if len(rows) != 12 { // 11 seed + 1
		t.Errorf("transaction count = %d, want 12", len(rows))
	}
}

func TestCreateTransactionValidationRetargetsModal(t *testing.T) {
	h, _ := newTestServer(t)
	form := url.Values{ // amount missing/invalid
		"date": {"2026-06-05"}, "description": {"Bad"}, "amount": {"-5"},
		"type": {"Expense"}, "payment_method": {"UPI"}, "category": {"Food & Dining"},
	}
	req := httptest.NewRequest("POST", "/transactions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("HX-Retarget") != "#modal" {
		t.Errorf("expected HX-Retarget #modal on validation error, got %q", rec.Header().Get("HX-Retarget"))
	}
	if !strings.Contains(rec.Body.String(), "positive") {
		t.Errorf("expected validation message, got %s", rec.Body.String())
	}
}

func TestTransactionsListFilter(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/transactions/list?category=Travel")
	body := rec.Body.String()
	if !strings.Contains(body, "ixigo") {
		t.Error("Travel filter should include ixigo")
	}
	if strings.Contains(body, "GROCETER") {
		t.Error("Travel filter should not include GROCETER")
	}
}

func TestBackupReturnsJSON(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/backup")
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `"transactions"`) {
		t.Error("backup missing transactions key")
	}
}

func TestExportRespectsFilters(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/transactions/export?category=Travel")
	body := rec.Body.String()
	if !strings.Contains(body, "ixigo") {
		t.Error("filtered export should include Travel rows (ixigo)")
	}
	if strings.Contains(body, "GROCETER") {
		t.Error("filtered export should exclude non-Travel rows (GROCETER)")
	}
}

func TestCreateTransactionUsesOOBRow(t *testing.T) {
	h, _ := newTestServer(t)
	form := url.Values{
		"date": {"2026-06-05"}, "description": {"Coffee"}, "amount": {"250.50"},
		"type": {"Expense"}, "payment_method": {"UPI"}, "category": {"Food & Dining"},
	}
	req := httptest.NewRequest("POST", "/transactions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// New row must be an out-of-band insert so it works from any page.
	if !strings.Contains(rec.Body.String(), `hx-swap-oob="afterbegin:#tx-rows"`) {
		t.Errorf("create response should use OOB swap, got: %s", rec.Body.String())
	}
}

func TestMonthDetailMay(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/month/detail?m=2026-05")
	body := rec.Body.String()
	if !strings.Contains(body, "₹2,33,053.00") {
		t.Error("May detail should show credits ₹2,33,053.00")
	}
	if !strings.Contains(body, "month-chart-data") {
		t.Error("May detail should include doughnut data island")
	}
}

func TestTransactionsSortAscending(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/transactions/list?sort=amount&order=asc")
	body := rec.Body.String()
	// Smallest expense (₹31.00) should appear before the largest (₹36,292.00).
	i31 := strings.Index(body, "₹31.00")
	iBig := strings.Index(body, "₹36,292.00")
	if i31 < 0 || iBig < 0 || i31 > iBig {
		t.Errorf("ascending sort wrong: idx(31)=%d idx(36292)=%d", i31, iBig)
	}
}

func TestBulkDelete(t *testing.T) {
	h, db := newTestServer(t)
	all, _ := db.AllTransactions()
	form := url.Values{"action": {"delete"}}
	form.Add("ids", all[0].ID)
	form.Add("ids", all[1].ID)
	req := httptest.NewRequest("POST", "/transactions/bulk", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	left, _ := db.AllTransactions()
	if len(left) != len(all)-2 {
		t.Errorf("after bulk delete: %d rows, want %d", len(left), len(all)-2)
	}
}

func TestCategoryRenameViaPrompt(t *testing.T) {
	h, db := newTestServer(t)
	req := httptest.NewRequest("POST", "/categories/Travel/rename", nil)
	req.Header.Set("HX-Prompt", "Trips")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Trips") {
		t.Error("rename should produce a category named Trips")
	}
	// Cascade: the ixigo transactions should now be category Trips.
	txs, _, _ := db.Transactions(store.TxFilter{Category: "Trips"})
	if len(txs) == 0 {
		t.Error("rename did not cascade to transactions")
	}
}

func TestModalWiringIsVanillaNotStaleAlpine(t *testing.T) {
	h, _ := newTestServer(t)
	// Layout must use the vanilla modal overlay, not the old Alpine-v2 API.
	page := get(t, h, "/").Body.String()
	if !strings.Contains(page, `id="modal-overlay"`) {
		t.Error("layout missing vanilla #modal-overlay")
	}
	for _, stale := range []string{"__x", "modalOpen"} {
		if strings.Contains(page, stale) {
			t.Errorf("layout still references stale Alpine-v2 token %q", stale)
		}
	}
	// Modal fragments must close via data-modal-close, not @click=modalOpen.
	modal := get(t, h, "/transactions/new").Body.String()
	if !strings.Contains(modal, "data-modal-close") {
		t.Error("tx_modal missing data-modal-close control")
	}
	if strings.Contains(modal, "modalOpen") {
		t.Error("tx_modal still references modalOpen")
	}
}

func TestRecurringToggle(t *testing.T) {
	h, db := newTestServer(t)
	items, _ := db.ListRecurring()
	var id string
	for _, it := range items {
		if it.Name == "Netflix" {
			id = it.ID
		}
	}
	req := httptest.NewRequest("POST", "/recurring/"+id+"/toggle", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("HX-Trigger") != "recChanged" {
		t.Errorf("toggle HX-Trigger = %q, want recChanged", rec.Header().Get("HX-Trigger"))
	}
	if !strings.Contains(rec.Body.String(), "Paused") {
		t.Error("toggled row should show Paused")
	}
}

func TestPlanningPageAndGoal(t *testing.T) {
	h, db := newTestServer(t)
	if get(t, h, "/planning").Code != 200 {
		t.Fatal("/planning not 200")
	}
	rec := postForm(t, h, "/goals", url.Values{
		"name": {"Laptop"}, "target": {"80000"}, "saved": {"20000"}, "target_date": {"2026-12-01"},
	})
	body := rec.Body.String()
	if !strings.Contains(body, "Laptop") || !strings.Contains(body, "width: 25%") {
		t.Errorf("goal card wrong: %s", body)
	}
	if goals, _ := db.ListGoals(); len(goals) != 1 {
		t.Errorf("expected 1 goal, got %d", len(goals))
	}
}

func TestBudgetOverspendFlag(t *testing.T) {
	h, _ := newTestServer(t) // clock = June 15 2026; June Misc spend = ₹80 (Mr Aasi)
	rec := postForm(t, h, "/budgets", url.Values{"category": {"Miscellaneous"}, "limit": {"50"}})
	body := rec.Body.String()
	if !strings.Contains(body, "Miscellaneous") {
		t.Fatalf("budget not listed: %s", body)
	}
	if !strings.Contains(body, "⚠") { // ₹80 spent > ₹50 limit
		t.Error("expected overspend flag for Miscellaneous")
	}
}

func TestRecurringFireIdempotent(t *testing.T) {
	h, db := newTestServer(t)
	before, _ := db.AllTransactions()

	r1 := postForm(t, h, "/recurring/fire", url.Values{})
	if !strings.Contains(r1.Body.String(), "Posted 7 due") {
		t.Errorf("first fire = %q, want 'Posted 7 due'", r1.Body.String())
	}
	mid, _ := db.AllTransactions()
	if len(mid) != len(before)+7 {
		t.Errorf("expected +7 transactions, got +%d", len(mid)-len(before))
	}

	r2 := postForm(t, h, "/recurring/fire", url.Values{})
	if !strings.Contains(r2.Body.String(), "Posted 0 due") {
		t.Errorf("second fire should post 0 (idempotent), got %q", r2.Body.String())
	}
	after, _ := db.AllTransactions()
	if len(after) != len(mid) {
		t.Errorf("idempotent fire added transactions: +%d", len(after)-len(mid))
	}
}

func TestTransactionTagsRoundTrip(t *testing.T) {
	h, _ := newTestServer(t)
	postForm(t, h, "/transactions", url.Values{
		"date": {"2026-06-05"}, "description": {"Lunch"}, "amount": {"300"},
		"type": {"Expense"}, "payment_method": {"UPI"}, "category": {"Food & Dining"},
		"tags": {"work, reimbursable"},
	})
	// Row should render the tag chips.
	list := get(t, h, "/transactions/list").Body.String()
	if !strings.Contains(list, "#work") || !strings.Contains(list, "#reimbursable") {
		t.Errorf("tag chips missing from list")
	}
	// Filter by tag returns only the tagged row.
	filtered := get(t, h, "/transactions/list?tag=work").Body.String()
	if !strings.Contains(filtered, "Lunch") {
		t.Error("tag filter should include the tagged transaction")
	}
	if strings.Contains(filtered, "GROCETER") {
		t.Error("tag filter should exclude untagged transactions")
	}
}

func TestBankCSVImportDebitCredit(t *testing.T) {
	h, db := newTestServer(t)
	before, _ := db.AllTransactions()
	csv := "Date,Narration,Debit,Credit\n" +
		"05-06-2026,UPI Coffee,250.00,\n" +
		"06-06-2026,Salary Credit,,90000.00\n"
	rec := postMultipartCSV(t, h, "/transactions/import", csv)
	if !strings.Contains(rec.Body.String(), "Imported 2") {
		t.Errorf("import result = %q, want 'Imported 2'", rec.Body.String())
	}
	after, _ := db.AllTransactions()
	if len(after) != len(before)+2 {
		t.Fatalf("expected +2 transactions, got +%d", len(after)-len(before))
	}
	var sawExpense, sawIncome bool
	for _, tx := range after {
		if tx.Description == "UPI Coffee" && tx.Type == "Expense" && tx.Amount.FormatINR() == "₹250.00" {
			sawExpense = true
		}
		if tx.Description == "Salary Credit" && tx.Type == "Income" {
			sawIncome = true
		}
	}
	if !sawExpense || !sawIncome {
		t.Error("debit→expense / credit→income mapping failed")
	}
}

func TestYearSankeyPage(t *testing.T) {
	h, _ := newTestServer(t)
	body := get(t, h, "/year").Body.String()
	if !strings.Contains(body, "Where 2026 income went") {
		t.Error("year page missing sankey")
	}
	if !strings.Contains(body, "₹2,33,053.00") {
		t.Error("year page missing income total")
	}
}

func TestHealthz(t *testing.T) {
	h, _ := newTestServer(t)
	rec := get(t, h, "/healthz")
	if rec.Code != 200 {
		t.Fatalf("/healthz = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %q", rec.Body.String())
	}
}

type fakePusher struct {
	chat int64
	text string
	n    int
}

func (f *fakePusher) SendMessage(_ context.Context, chatID int64, text string) error {
	f.chat, f.text, f.n = chatID, text, f.n+1
	return nil
}

func newPushHandler(t *testing.T, defaultChat int64, token string) (*Handler, *fakePusher) {
	t.Helper()
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, fn := range []func() error{db.Migrate, db.EnsureSettings, db.SeedDefaultCategories} {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	rdr, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(service.New(db, nil), rdr)
	fp := &fakePusher{}
	h.EnablePush(fp, defaultChat)
	h.SetAPIToken(token)
	return h, fp
}

func doPush(h *Handler, body, auth, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/push", strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.Routes(http.NotFoundHandler()).ServeHTTP(rec, req)
	return rec
}

func TestPushDisabledByDefault(t *testing.T) {
	h, _ := newTestServer(t) // push not enabled
	req := httptest.NewRequest("POST", "/api/push", strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Errorf("disabled push = %d, want 503", rec.Code)
	}
}

func TestPushRequiresToken(t *testing.T) {
	h, fp := newPushHandler(t, 555, "secret")
	// no auth
	if rec := doPush(h, `{"text":"hi"}`, "", "application/json"); rec.Code != 401 {
		t.Errorf("no token = %d, want 401", rec.Code)
	}
	// wrong token
	if rec := doPush(h, `{"text":"hi"}`, "Bearer nope", "application/json"); rec.Code != 401 {
		t.Errorf("wrong token = %d, want 401", rec.Code)
	}
	if fp.n != 0 {
		t.Error("unauthorized requests must not send")
	}
}

func TestPushSendsToDefaultChat(t *testing.T) {
	h, fp := newPushHandler(t, 555, "secret")
	rec := doPush(h, `{"text":"deploy done ✅"}`, "Bearer secret", "application/json")
	if rec.Code != 200 {
		t.Fatalf("push = %d, body %s", rec.Code, rec.Body.String())
	}
	if fp.n != 1 || fp.chat != 555 || fp.text != "deploy done ✅" {
		t.Errorf("pusher got chat=%d text=%q n=%d", fp.chat, fp.text, fp.n)
	}
	if !strings.Contains(rec.Body.String(), `"sent":true`) {
		t.Errorf("response = %s", rec.Body.String())
	}
}

func TestPushExplicitChatAndForm(t *testing.T) {
	h, fp := newPushHandler(t, 0, "secret") // no default chat
	// missing chat -> 400
	if rec := doPush(h, `{"text":"hi"}`, "Bearer secret", "application/json"); rec.Code != 400 {
		t.Errorf("no chat = %d, want 400", rec.Code)
	}
	// form body with explicit chat
	rec := doPush(h, "text=hello&chat_id=999", "Bearer secret", "application/x-www-form-urlencoded")
	if rec.Code != 200 || fp.chat != 999 || fp.text != "hello" {
		t.Errorf("form push wrong: code=%d chat=%d text=%q", rec.Code, fp.chat, fp.text)
	}
}

func TestPushEmptyText(t *testing.T) {
	h, _ := newPushHandler(t, 555, "secret")
	if rec := doPush(h, `{"text":"   "}`, "Bearer secret", "application/json"); rec.Code != 400 {
		t.Errorf("empty text = %d, want 400", rec.Code)
	}
}

type fakeRunner struct {
	res GmailResult
	err error
	n   int
}

func (f *fakeRunner) Run(_ context.Context) (GmailResult, error) {
	f.n++
	return f.res, f.err
}

func newGmailHandler(t *testing.T, token string) (*Handler, *fakeRunner) {
	t.Helper()
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, fn := range []func() error{db.Migrate, db.EnsureSettings, db.SeedDefaultCategories} {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	rdr, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(service.New(db, nil), rdr)
	fr := &fakeRunner{res: GmailResult{Scanned: 3, Imported: 2, Skipped: 1}}
	h.EnableGmail(fr)
	if token != "" {
		h.SetAPIToken(token)
	}
	return h, fr
}

func TestGmailSyncUI(t *testing.T) {
	h, fr := newGmailHandler(t, "")
	req := httptest.NewRequest("POST", "/gmail/sync", nil)
	rec := httptest.NewRecorder()
	h.Routes(http.NotFoundHandler()).ServeHTTP(rec, req)
	if rec.Code != 200 || fr.n != 1 {
		t.Fatalf("ui sync code=%d runs=%d", rec.Code, fr.n)
	}
	if !strings.Contains(rec.Body.String(), "imported 2") {
		t.Errorf("flash = %q", rec.Body.String())
	}
	if rec.Header().Get("HX-Trigger") != "txChanged" {
		t.Errorf("ui sync should trigger txChanged")
	}
}

func TestGmailSyncUINotConfigured(t *testing.T) {
	db, _ := store.OpenSQLite(":memory:")
	t.Cleanup(func() { db.Close() })
	for _, fn := range []func() error{db.Migrate, db.EnsureSettings, db.SeedDefaultCategories} {
		fn()
	}
	rdr, _ := NewRenderer()
	h := NewHandler(service.New(db, nil), rdr) // no EnableGmail
	req := httptest.NewRequest("POST", "/gmail/sync", nil)
	rec := httptest.NewRecorder()
	h.Routes(http.NotFoundHandler()).ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Errorf("expected not-configured flash, got %q", rec.Body.String())
	}
}

func TestGmailSyncAPI(t *testing.T) {
	h, fr := newGmailHandler(t, "secret")
	routes := h.Routes(http.NotFoundHandler())

	// no auth -> 401
	req := httptest.NewRequest("POST", "/api/gmail/sync", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("no-auth = %d, want 401", rec.Code)
	}

	// with token -> 200 JSON
	req = httptest.NewRequest("POST", "/api/gmail/sync", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"imported":2`) {
		t.Errorf("api sync code=%d body=%s", rec.Code, rec.Body.String())
	}
	if fr.n != 1 {
		t.Errorf("runner called %d times, want 1", fr.n)
	}
}
