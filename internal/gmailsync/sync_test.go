package gmailsync

import (
	"context"
	"testing"
	"time"

	"finance-tracker/internal/store"
)

type fakeFetcher struct{ msgs []Message }

func (f fakeFetcher) Fetch(_ context.Context, _ []string, _ time.Time) ([]Message, error) {
	return f.msgs, nil
}

func newDB(t *testing.T) *store.SQLite {
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
	return db
}

func sampleMessages() []Message {
	d := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	return []Message{
		{ID: "<a@hdfc>", From: "alerts@hdfcbank.net", Subject: "Alert: HDFC Card", Date: d,
			Text: "Rs.1,234.50 was spent on your HDFC Bank Credit Card xx1234 at AMAZON on 01-06-2026."},
		{ID: "<b@icici>", From: "alert@icicibank.com", Subject: "ICICI Card txn", Date: d,
			Text: "INR 2,500.00 spent on ICICI Bank Card at FLIPKART on 01-Jun-26."},
		{ID: "<c@otp>", From: "alerts@hdfcbank.net", Subject: "OTP", Date: d, Text: "Your OTP is 123456 for Rs.500"},
		{ID: "<d@pay>", From: "alert@icicibank.com", Subject: "Payment received", Date: d, Text: "Payment of Rs.5000 received. Thank you."},
		{ID: "", From: "x", Subject: "no id", Date: d, Text: "Rs.10 spent at SHOP"}, // missing Message-ID
	}
}

func TestSyncerImportsAndDedupes(t *testing.T) {
	db := newDB(t)
	clock := func() time.Time { return time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC) }
	s := NewSyncer(fakeFetcher{sampleMessages()}, db, Config{Senders: DefaultCardSenders}, clock)

	res, err := s.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 5 || res.Imported != 2 || res.Skipped != 3 {
		t.Fatalf("run1 = %+v, want scanned 5 imported 2 skipped 3", res)
	}
	rows, _ := db.AllTransactions()
	if len(rows) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(rows))
	}
	var amazon bool
	for _, tx := range rows {
		if string(tx.Type) != "Expense" || string(tx.PaymentMethod) != "Credit Card" || tx.Category != "Miscellaneous" {
			t.Errorf("imported tx has wrong defaults: %+v", tx)
		}
		if tx.Date != "2026-06-01" {
			t.Errorf("tx date = %s, want email date 2026-06-01", tx.Date)
		}
		if tx.Description == "AMAZON" && tx.Amount.FormatINR() == "₹1,234.50" {
			amazon = true
		}
	}
	if !amazon {
		t.Error("AMAZON ₹1,234.50 transaction not found")
	}

	// Second run: everything already processed → nothing new.
	res2, _ := s.Run(context.Background())
	if res2.Imported != 0 {
		t.Errorf("run2 imported %d, want 0 (idempotent)", res2.Imported)
	}
	rows2, _ := db.AllTransactions()
	if len(rows2) != 2 {
		t.Errorf("idempotent run added rows: now %d", len(rows2))
	}
}

// TestSyncerImportsRealICICIEmail drives the exact ICICI credit-card alert the
// daily cron will see through the whole pipeline and asserts the resulting DB row.
func TestSyncerImportsRealICICIEmail(t *testing.T) {
	db := newDB(t)
	d := time.Date(2026, 6, 1, 9, 0, 24, 0, time.UTC)
	msg := Message{
		ID:      "<icici-4003-20260601@icicibank.com>",
		From:    "credit_cards@icicibank.com",
		Subject: "Transaction alert for your ICICI Bank Credit Card",
		Date:    d,
		Text: "Dear Customer,\n\nYour ICICI Bank Credit Card XX4003 has been used for a transaction of INR 80.00 on Jun 01, 2026 at 09:00:24. Info: UPI-651816620443-Mr Aasi.\n\n" +
			"The Available Credit Limit on your card is INR 10,29,889.00 and Total Credit Limit is INR 10,30,000.00. The above limits are a total of the limits of all the Credit Cards issued to the primary card holder, including any supplementary cards.",
	}
	clock := func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }
	s := NewSyncer(fakeFetcher{[]Message{msg}}, db, Config{Senders: DefaultCardSenders}, clock)

	res, err := s.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Fatalf("imported %d, want 1 (res=%+v)", res.Imported, res)
	}
	rows, _ := db.AllTransactions()
	if len(rows) != 1 {
		t.Fatalf("got %d transactions, want 1", len(rows))
	}
	tx := rows[0]
	if tx.Amount.FormatINR() != "₹80.00" {
		t.Errorf("amount = %s, want ₹80.00 (must not pick up the limit amounts)", tx.Amount.FormatINR())
	}
	if tx.Description != "Mr Aasi" {
		t.Errorf("description = %q, want %q", tx.Description, "Mr Aasi")
	}
	if string(tx.Type) != "Expense" || string(tx.PaymentMethod) != "Credit Card" {
		t.Errorf("wrong type/method: %s / %s", tx.Type, tx.PaymentMethod)
	}
	if tx.Date != "2026-06-01" {
		t.Errorf("date = %s, want 2026-06-01", tx.Date)
	}
}
