package service

import (
	"testing"

	"finance-tracker/internal/domain"
)

func TestGoalProgress(t *testing.T) {
	td := "2026-12-15"
	g := domain.Goal{Target: domain.ToPaise(120000), Saved: domain.ToPaise(30000), TargetDate: &td}
	p := GoalProgressFor(g, asOf) // asOf = 2026-06-15
	if !approx(p.Pct, 0.25, 1e-9) {
		t.Errorf("pct = %v, want 0.25", p.Pct)
	}
	if p.Remaining != domain.ToPaise(90000) {
		t.Errorf("remaining = %s, want ₹90,000", p.Remaining.FormatINR())
	}
	// 6 months to Dec → 90000/6 = 15000/month
	if p.MonthlyNeeded != domain.ToPaise(15000) {
		t.Errorf("monthlyNeeded = %s, want ₹15,000", p.MonthlyNeeded.FormatINR())
	}
	if p.Complete {
		t.Error("should not be complete")
	}

	done := domain.Goal{Target: domain.ToPaise(100), Saved: domain.ToPaise(150)}
	if dp := GoalProgressFor(done, asOf); !dp.Complete || dp.Remaining != 0 {
		t.Errorf("completed goal wrong: %+v", dp)
	}
}

func TestBudgetStatuses(t *testing.T) {
	txs := []domain.Transaction{
		{Date: "2026-06-01", Type: domain.Expense, Category: "Groceries", Amount: domain.ToPaise(4000)},
		{Date: "2026-06-09", Type: domain.Expense, Category: "Groceries", Amount: domain.ToPaise(7000)},
		{Date: "2026-05-01", Type: domain.Expense, Category: "Groceries", Amount: domain.ToPaise(9000)}, // other month
		{Date: "2026-06-02", Type: domain.Expense, Category: "Travel", Amount: domain.ToPaise(2000)},
	}
	budgets := []domain.Budget{
		{Category: "Groceries", Limit: domain.ToPaise(10000)},
		{Category: "Travel", Limit: domain.ToPaise(5000)},
	}
	st := BudgetStatuses(budgets, txs, "2026-06")
	if len(st) != 2 {
		t.Fatalf("got %d statuses", len(st))
	}
	// Groceries: 4000+7000=11000 over 10000
	if st[0].Category != "Groceries" || st[0].Spent != domain.ToPaise(11000) || !st[0].Over {
		t.Errorf("groceries status wrong: %+v", st[0])
	}
	// Travel: 2000 under 5000
	if st[1].Category != "Travel" || st[1].Over {
		t.Errorf("travel status wrong: %+v", st[1])
	}
}

func TestDueInMonth(t *testing.T) {
	mk := func(freq domain.Frequency, start string) domain.RecurringItem {
		return domain.RecurringItem{Active: true, Frequency: freq, StartDate: start}
	}
	cases := []struct {
		name  string
		item  domain.RecurringItem
		month string
		want  bool
	}{
		{"monthly due", mk(domain.Monthly, "2026-01-01"), "2026-06", true},
		{"monthly before start", mk(domain.Monthly, "2026-07-01"), "2026-06", false},
		{"quarterly on step", mk(domain.Quarterly, "2026-01-01"), "2026-07", true},  // +6 months, 6%3==0
		{"quarterly off step", mk(domain.Quarterly, "2026-01-01"), "2026-06", false}, // +5, 5%3!=0
		{"yearly anniversary", mk(domain.Yearly, "2026-03-01"), "2027-03", true},
		{"yearly off", mk(domain.Yearly, "2026-03-01"), "2026-09", false},
		{"onetime its month", mk(domain.OneTime, "2026-06-10"), "2026-06", true},
		{"onetime other month", mk(domain.OneTime, "2026-06-10"), "2026-07", false},
		{"weekly skipped", mk(domain.Weekly, "2026-01-01"), "2026-06", false},
	}
	for _, c := range cases {
		if got := DueInMonth(c.item, c.month); got != c.want {
			t.Errorf("%s: DueInMonth = %v, want %v", c.name, got, c.want)
		}
	}

	inactive := mk(domain.Monthly, "2026-01-01")
	inactive.Active = false
	if DueInMonth(inactive, "2026-06") {
		t.Error("inactive item should not be due")
	}
	end := "2026-04-30"
	ended := mk(domain.Monthly, "2026-01-01")
	ended.EndDate = &end
	if DueInMonth(ended, "2026-06") {
		t.Error("item past end date should not be due")
	}
}

func TestPostingDate(t *testing.T) {
	r := domain.RecurringItem{StartDate: "2026-01-31"}
	// Feb clamps day 31 -> 28
	if d := PostingDate(r, "2026-02"); d != "2026-02-28" {
		t.Errorf("clamped posting date = %s, want 2026-02-28", d)
	}
	if d := PostingDate(domain.RecurringItem{StartDate: "2026-01-15"}, "2026-06"); d != "2026-06-15" {
		t.Errorf("posting date = %s, want 2026-06-15", d)
	}
}

func TestYearSankey(t *testing.T) {
	txs := SeedTransactions() // all 2026
	d := YearSankey(txs, "2026")
	if d.Income != domain.ToPaise(233053) {
		t.Errorf("income = %s, want ₹2,33,053", d.Income.FormatINR())
	}
	if d.Expense != domain.ToPaise(86090.40) {
		t.Errorf("expense = %s, want ₹86,090.40", d.Expense.FormatINR())
	}
	if len(d.Flows) == 0 || d.Flows[0].Category != "Travel" {
		t.Errorf("top flow should be Travel, got %+v", d.Flows)
	}
	if d.Saved != d.Income-d.Expense {
		t.Errorf("saved mismatch")
	}
}

func TestDueRecurringExcludesPosted(t *testing.T) {
	items := []domain.RecurringItem{
		{ID: "a", Active: true, Frequency: domain.Monthly, StartDate: "2026-01-01"},
		{ID: "b", Active: true, Frequency: domain.Monthly, StartDate: "2026-01-01"},
	}
	posted := map[string]bool{"a": true}
	due := DueRecurring(items, "2026-06", func(id string) bool { return posted[id] })
	if len(due) != 1 || due[0].ID != "b" {
		t.Errorf("due should exclude posted 'a', got %+v", due)
	}
}
