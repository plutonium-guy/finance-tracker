package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
	"finance-tracker/internal/store"
)

// newAlertsServer builds a server with token + a fake Telegram pusher wired in.
// (fakePusher is defined in web_test.go.)
func newAlertsServer(t *testing.T) (http.Handler, *Handler, *store.SQLite, *fakePusher) {
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
	h.SetAPIToken("secret")
	p := &fakePusher{}
	h.EnablePush(p, 4242)
	return h.Routes(http.FileServer(http.FS(StaticFS()))), h, db, p
}

func TestAlertsBudgetAndCard(t *testing.T) {
	router, h, db, pusher := newAlertsServer(t)

	// Over-budget category: limit ₹1000, spend ₹1500 this month.
	_ = db.SetBudget(domain.Budget{Category: "Food & Dining", Limit: domain.ToPaise(1000)})
	_ = db.CreateTransaction(domain.Transaction{
		ID: "t1", Date: "2026-06-10", Description: "Dinner", Amount: domain.ToPaise(1500),
		Type: domain.Expense, PaymentMethod: domain.UPI, Category: "Food & Dining", CreatedAt: "x", UpdatedAt: "x",
	})
	// Overdue card statement (statement day 5, due +1 → 2026-06-06, today is 15).
	_ = db.CreateCard(domain.Card{ID: "c1", Name: "ICICI", Limit: domain.ToPaise(100000), StatementDay: 5, DueOffsetDays: 1, CreatedAt: "x", UpdatedAt: "x"})
	_ = db.CreateTransaction(domain.Transaction{
		ID: "t2", Date: "2026-06-02", Description: "Shopping", Amount: domain.ToPaise(4000),
		Type: domain.Expense, PaymentMethod: domain.CreditCard, Category: "Miscellaneous", CreatedAt: "x", UpdatedAt: "x",
	})
	_ = db.SetTransactionCard("t2", "c1")

	alerts := h.buildAlerts()
	joined := strings.Join(alerts, "\n")
	if !strings.Contains(joined, "Budget: Food & Dining") {
		t.Errorf("missing budget alert in: %q", joined)
	}
	if !strings.Contains(joined, "ICICI") || !strings.Contains(strings.ToLower(joined), "overdue") {
		t.Errorf("missing overdue card alert in: %q", joined)
	}

	// Endpoint requires the token.
	if rec := postForm(t, router, "/api/alerts/run", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no-token alerts: status %d, want 401", rec.Code)
	}
	// With the token it runs and sends via the pusher.
	rec := postJSON(t, router, "/api/alerts/run", "secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("alerts run: status %d body %s", rec.Code, rec.Body.String())
	}
	if pusher.n != 1 || pusher.chat != 4242 {
		t.Fatalf("expected one push to chat 4242, got n=%d chat=%d", pusher.n, pusher.chat)
	}
	if !strings.Contains(pusher.text, "Finance Tracker") {
		t.Errorf("push body missing header: %q", pusher.text)
	}
}
