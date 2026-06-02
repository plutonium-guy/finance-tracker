package service

import (
	"testing"

	"finance-tracker/internal/domain"
)

func exp(date, cat string, rupees float64) domain.Transaction {
	return domain.Transaction{ID: date + cat, Date: date, Category: cat, Amount: domain.ToPaise(rupees), Type: domain.Expense}
}

func TestMonthInsights(t *testing.T) {
	txs := []domain.Transaction{
		exp("2026-05-10", "Food", 2000),
		exp("2026-05-12", "Travel", 5000),
		exp("2026-06-05", "Food", 3500),  // +1500 vs May
		exp("2026-06-08", "Travel", 1000), // -4000 vs May
		exp("2026-06-09", "Shopping", 800), // new category
	}
	ins := MonthInsightsFor(txs, "2026-06")
	if !ins.HasPrev {
		t.Fatal("expected HasPrev true")
	}
	// June total 5300, May total 7000 → delta -1700
	if ins.ExpenseThis != domain.ToPaise(5300) || ins.ExpensePrev != domain.ToPaise(7000) {
		t.Errorf("this/prev = %s/%s, want ₹5300/₹7000", ins.ExpenseThis.FormatINR(), ins.ExpensePrev.FormatINR())
	}
	if ins.ExpenseDelta != domain.ToPaise(-1700) {
		t.Errorf("delta = %s, want -₹1700", ins.ExpenseDelta.FormatINR())
	}
	// Biggest mover is Travel (-4000).
	if len(ins.TopMovers) == 0 || ins.TopMovers[0].Category != "Travel" {
		t.Errorf("top mover = %+v, want Travel", ins.TopMovers)
	}
}

func TestComputeReimbursements(t *testing.T) {
	txs := []domain.Transaction{
		{ID: "a", Type: domain.Expense, Amount: domain.ToPaise(1200)},
		{ID: "b", Type: domain.Expense, Amount: domain.ToPaise(800)},
		{ID: "c", Type: domain.Expense, Amount: domain.ToPaise(500)}, // not reimbursable
		{ID: "d", Type: domain.Income, Amount: domain.ToPaise(900)},  // income, ignored
	}
	tags := map[string][]string{
		"a": {"reimbursable"},
		"b": {"reimbursable", "settled"},
		"d": {"reimbursable"},
	}
	r := ComputeReimbursements(txs, tags)
	if r.Outstanding != domain.ToPaise(1200) {
		t.Errorf("outstanding = %s, want ₹1200", r.Outstanding.FormatINR())
	}
	if r.SettledTotal != domain.ToPaise(800) {
		t.Errorf("settled = %s, want ₹800", r.SettledTotal.FormatINR())
	}
	if len(r.Items) != 2 {
		t.Errorf("items = %d, want 2", len(r.Items))
	}
}
