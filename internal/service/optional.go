package service

import (
	"sort"
	"strconv"
	"time"

	"finance-tracker/internal/domain"
)

// ---- goals ----

// GoalProgress decorates a goal with computed progress figures.
type GoalProgress struct {
	Goal          domain.Goal
	Pct           float64      // saved/target, capped display done in template
	Remaining     domain.Money // target-saved, >= 0
	MonthlyNeeded domain.Money // remaining / months until target date (0 if no date/overdue)
	Complete      bool
}

// GoalProgressFor computes progress for a goal as of now.
func GoalProgressFor(g domain.Goal, now time.Time) GoalProgress {
	p := GoalProgress{Goal: g}
	if g.Target > 0 {
		p.Pct = float64(g.Saved) / float64(g.Target)
	}
	p.Remaining = g.Target - g.Saved
	if p.Remaining < 0 {
		p.Remaining = 0
	}
	p.Complete = g.Saved >= g.Target
	if g.TargetDate != nil && !p.Complete {
		if td, err := time.Parse("2006-01-02", *g.TargetDate); err == nil {
			months := monthDiff(MonthKey(now), MonthKey(td))
			if months > 0 {
				p.MonthlyNeeded = domain.Money(int64(p.Remaining) / int64(months))
			} else {
				p.MonthlyNeeded = p.Remaining // due now/overdue
			}
		}
	}
	return p
}

// ---- budgets ----

// BudgetStatus is a budget compared against actual spend for a month.
type BudgetStatus struct {
	Category string
	Limit    domain.Money
	Spent    domain.Money
	Pct      float64
	Over     bool
}

// BudgetStatuses computes per-budget spend for the given month.
func BudgetStatuses(budgets []domain.Budget, txs []domain.Transaction, month string) []BudgetStatus {
	spent := map[string]domain.Money{}
	for _, t := range txs {
		if t.Type == domain.Expense && t.Month() == month {
			spent[t.Category] += t.Amount
		}
	}
	out := make([]BudgetStatus, 0, len(budgets))
	for _, b := range budgets {
		s := spent[b.Category]
		st := BudgetStatus{Category: b.Category, Limit: b.Limit, Spent: s, Over: s > b.Limit}
		if b.Limit > 0 {
			st.Pct = float64(s) / float64(b.Limit)
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

// ---- recurring auto-fire ----

// monthStep returns the number of months between occurrences for a frequency,
// and whether the frequency is auto-fireable on a monthly schedule.
func monthStep(f domain.Frequency) (int, bool) {
	switch f {
	case domain.Monthly:
		return 1, true
	case domain.Quarterly:
		return 3, true
	case domain.HalfYearly:
		return 6, true
	case domain.Yearly:
		return 12, true
	case domain.OneTime:
		return 0, true // only its start month
	default: // Weekly — sub-monthly, not auto-fired monthly
		return 0, false
	}
}

// DueInMonth reports whether a recurring item should produce a transaction in
// the given month ("YYYY-MM"), based on its frequency, start, and end dates.
func DueInMonth(r domain.RecurringItem, month string) bool {
	if !r.Active {
		return false
	}
	step, ok := monthStep(r.Frequency)
	if !ok {
		return false
	}
	startMonth := monthOf(r.StartDate)
	diff := monthDiff(startMonth, month)
	if diff < 0 {
		return false
	}
	if r.EndDate != nil && month > monthOf(*r.EndDate) {
		return false
	}
	if r.Frequency == domain.OneTime {
		return diff == 0
	}
	return diff%step == 0
}

// DueRecurring returns the active items that should be posted in month and are
// not yet posted (per the wasPosted predicate).
func DueRecurring(items []domain.RecurringItem, month string, wasPosted func(id string) bool) []domain.RecurringItem {
	var out []domain.RecurringItem
	for _, r := range items {
		if DueInMonth(r, month) && !wasPosted(r.ID) {
			out = append(out, r)
		}
	}
	return out
}

// PostingDate returns the ISO date to stamp on an auto-fired transaction: the
// item's start day-of-month, clamped into the target month.
func PostingDate(r domain.RecurringItem, month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return month + "-01"
	}
	day := 1
	if st, err := time.Parse("2006-01-02", r.StartDate); err == nil {
		day = st.Day()
	}
	// Clamp to the last day of the target month.
	last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		day = last
	}
	return time.Date(t.Year(), t.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// ---- year-end sankey ----

// SankeyFlow is one income→category flow.
type SankeyFlow struct {
	Category string
	Amount   domain.Money
	Pct      float64
}

// SankeyData is the year-end income→category breakdown.
type SankeyData struct {
	Year     string
	Income   domain.Money
	Flows    []SankeyFlow // expense by category, desc
	Saved    domain.Money // income - total expense (may be negative)
	SavedPct float64      // saved / income (0 if no income or negative)
	Expense  domain.Money
}

// YearSankey builds the income→category flows for a calendar year.
func YearSankey(txs []domain.Transaction, year string) SankeyData {
	d := SankeyData{Year: year}
	byCat := map[string]domain.Money{}
	for _, t := range txs {
		if len(t.Date) < 4 || t.Date[:4] != year {
			continue
		}
		switch t.Type {
		case domain.Income:
			d.Income += t.Amount
		case domain.Expense:
			d.Expense += t.Amount
			byCat[t.Category] += t.Amount
		}
	}
	for cat, amt := range byCat {
		pct := 0.0
		if d.Income > 0 {
			pct = float64(amt) / float64(d.Income)
		}
		d.Flows = append(d.Flows, SankeyFlow{Category: cat, Amount: amt, Pct: pct})
	}
	sort.Slice(d.Flows, func(i, j int) bool { return d.Flows[i].Amount > d.Flows[j].Amount })
	d.Saved = d.Income - d.Expense
	if d.Income > 0 && d.Saved > 0 {
		d.SavedPct = float64(d.Saved) / float64(d.Income)
	}
	return d
}

// ---- helpers ----

// monthOf returns the "YYYY-MM" prefix of an ISO date.
func monthOf(date string) string {
	if len(date) >= 7 {
		return date[:7]
	}
	return date
}

// monthDiff returns the number of months from a to b (both "YYYY-MM").
func monthDiff(a, b string) int {
	ay, am := splitMonth(a)
	by, bm := splitMonth(b)
	return (by-ay)*12 + (bm - am)
}

func splitMonth(m string) (year, month int) {
	if len(m) < 7 {
		return 0, 0
	}
	year, _ = strconv.Atoi(m[:4])
	month, _ = strconv.Atoi(m[5:7])
	return year, month
}
