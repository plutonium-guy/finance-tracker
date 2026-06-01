package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
	"finance-tracker/internal/store"
)

// newTestServerWithToken builds a server like newTestServer but with the API
// bearer token configured, so the /api/* endpoints are active.
func newTestServerWithToken(t *testing.T, token string) (http.Handler, *store.SQLite) {
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
	svc := service.New(db, func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) })
	rdr, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(svc, rdr)
	h.SetAPIToken(token)
	return h.Routes(http.FileServer(http.FS(StaticFS()))), db
}

func postJSON(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTransactionCreateAPIWithToken(t *testing.T) {
	h, db := newTestServerWithToken(t, "secret")

	// No token configured at all is covered by the router default; here a token
	// IS set, so a wrong token → 401.
	if rec := postJSON(t, h, "/api/transactions", "nope", `{"amount":250,"description":"x"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: status %d, want 401", rec.Code)
	}
	// Bad amount → 400.
	if rec := postJSON(t, h, "/api/transactions", "secret", `{"amount":0,"description":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero amount: status %d, want 400", rec.Code)
	}
	// Valid create → 201, applies defaults (Expense/UPI/today), returns id.
	rec := postJSON(t, h, "/api/transactions", "secret", `{"amount":250.5,"description":"Coffee","tags":["work"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d body %s", rec.Code, rec.Body.String())
	}
	var resp txAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID == "" || resp.AmountPaise != 25050 || resp.Type != "Expense" || resp.PaymentMethod != "UPI" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	rows, _ := db.AllTransactions()
	if len(rows) != 1 || rows[0].Description != "Coffee" {
		t.Fatalf("transaction not stored: %+v", rows)
	}
	if tags, _ := db.TagsFor(resp.ID); len(tags) != 1 || tags[0] != "work" {
		t.Errorf("tags = %v, want [work]", tags)
	}
}

func TestCardsPageAndMarkPaid(t *testing.T) {
	h, db := newTestServer(t)

	// Create a card (statement day 5; clock is 2026-06-15 so the May-05..Jun-05
	// statement has just closed).
	rec := postForm(t, h, "/cards", url.Values{
		"name": {"Test Card"}, "last4": {"1234"}, "limit": {"100000"},
		"statement_day": {"5"}, "due_offset_days": {"18"},
	})
	if rec.Code != 200 {
		t.Fatalf("create card: status %d", rec.Code)
	}
	cards, _ := db.ListCards()
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards))
	}
	cardID := cards[0].ID

	if rec := get(t, h, "/cards"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Test Card") {
		t.Fatalf("/cards page: status %d, contains card name=%v", rec.Code, strings.Contains(rec.Body.String(), "Test Card"))
	}

	// A credit-card purchase in the closed statement cycle, linked to the card.
	rec = postForm(t, h, "/transactions", url.Values{
		"date": {"2026-05-20"}, "amount": {"6200"}, "description": {"Shopping"},
		"type": {"Expense"}, "payment_method": {"Credit Card"}, "category": {"Miscellaneous"},
		"card_id": {cardID},
	})
	if rec.Code != 200 {
		t.Fatalf("create tx: status %d", rec.Code)
	}

	transfers := func() int {
		txs, _ := db.AllTransactions()
		n := 0
		for _, tx := range txs {
			if tx.Type == domain.Transfer {
				n++
			}
		}
		return n
	}
	if transfers() != 0 {
		t.Fatalf("expected 0 transfers before payment, got %d", transfers())
	}

	// Mark the statement paid: creates exactly one Transfer + one payment row.
	if rec := postForm(t, h, "/cards/"+cardID+"/pay", url.Values{}); rec.Code != 200 {
		t.Fatalf("pay: status %d", rec.Code)
	}
	if transfers() != 1 {
		t.Fatalf("expected 1 transfer after payment, got %d", transfers())
	}
	pays, _ := db.ListStatementPayments(cardID)
	if len(pays) != 1 || pays[0].Amount != domain.ToPaise(6200) {
		t.Fatalf("payments = %+v, want one of ₹6200", pays)
	}

	// Paying again is idempotent — no second transfer, no second payment row.
	postForm(t, h, "/cards/"+cardID+"/pay", url.Values{})
	if transfers() != 1 {
		t.Errorf("expected payment to be idempotent, got %d transfers", transfers())
	}
	if pays, _ := db.ListStatementPayments(cardID); len(pays) != 1 {
		t.Errorf("expected 1 payment row after double-pay, got %d", len(pays))
	}
}
