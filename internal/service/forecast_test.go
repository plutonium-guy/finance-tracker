package service

import (
	"testing"
	"time"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/store"
)

func newSvc(t *testing.T) *Service {
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
	return New(db, func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) })
}

func recItem(id, name string, rupees float64, day int, typ domain.TransactionType) domain.RecurringItem {
	return domain.RecurringItem{
		ID: id, Name: name, Category: "Miscellaneous", Amount: domain.ToPaise(rupees),
		Frequency: domain.Monthly, Type: typ, StartDate: time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
		PaymentMethod: domain.UPI, Active: true,
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}
}

func TestPostDueIdempotent(t *testing.T) {
	s := newSvc(t)
	if err := s.Store.CreateRecurring(recItem("r1", "Rent", 20000, 5, domain.Expense)); err != nil {
		t.Fatal(err)
	}
	n, err := s.PostDue("2026-06")
	if err != nil || n != 1 {
		t.Fatalf("first PostDue = %d,%v want 1,nil", n, err)
	}
	rows, _ := s.Store.AllTransactions()
	if len(rows) != 1 || rows[0].Description != "Rent" || rows[0].Amount != domain.ToPaise(20000) {
		t.Fatalf("posted tx wrong: %+v", rows)
	}
	// Second run posts nothing (idempotent via posted_recurring).
	if n, _ := s.PostDue("2026-06"); n != 0 {
		t.Errorf("second PostDue = %d, want 0", n)
	}
	if rows, _ := s.Store.AllTransactions(); len(rows) != 1 {
		t.Errorf("idempotent run added rows: %d", len(rows))
	}
}

func TestForecastFor(t *testing.T) {
	s := newSvc(t)
	// An actual booked expense this month.
	_ = s.Store.CreateTransaction(domain.Transaction{
		ID: "t1", Date: "2026-06-10", Description: "Groceries", Amount: domain.ToPaise(3000),
		Type: domain.Expense, PaymentMethod: domain.UPI, Category: "Miscellaneous",
		CreatedAt: "x", UpdatedAt: "x",
	})
	// Recurring due 2026-06-20 (after today=15) → counts as upcoming.
	_ = s.Store.CreateRecurring(recItem("r-rent", "Rent", 20000, 20, domain.Expense))
	// Recurring due 2026-06-05 (before today) → already passed, excluded.
	_ = s.Store.CreateRecurring(recItem("r-old", "Wifi", 999, 5, domain.Expense))
	// Upcoming income due 2026-06-28.
	_ = s.Store.CreateRecurring(recItem("r-sal", "Salary", 80000, 28, domain.Income))

	txs, _ := s.Store.AllTransactions()
	rec, _ := s.Store.ListRecurring()
	f := s.ForecastFor(txs, rec, "2026-06")

	if f.NetSoFar != domain.ToPaise(-3000) {
		t.Errorf("NetSoFar = %s, want -₹3000", f.NetSoFar.FormatINR())
	}
	if f.UpcomingExpense != domain.ToPaise(20000) {
		t.Errorf("UpcomingExpense = %s, want ₹20000 (only the day-20 item)", f.UpcomingExpense.FormatINR())
	}
	if f.UpcomingIncome != domain.ToPaise(80000) {
		t.Errorf("UpcomingIncome = %s, want ₹80000", f.UpcomingIncome.FormatINR())
	}
	// -3000 - 20000 + 80000 = 57000
	if f.ProjectedNet != domain.ToPaise(57000) {
		t.Errorf("ProjectedNet = %s, want ₹57000", f.ProjectedNet.FormatINR())
	}
	if f.DaysLeft != 15 { // June has 30 days, today 15
		t.Errorf("DaysLeft = %d, want 15", f.DaysLeft)
	}
}
