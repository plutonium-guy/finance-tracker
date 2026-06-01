// Package service holds all aggregation and business logic. The functions in
// aggregate.go are pure (operate on slices, no I/O) so they are directly unit
// testable against the seed data.
package service

import (
	"sort"
	"strings"
	"time"

	"finance-tracker/internal/domain"
)

// Window of months shown across the app: 6 months back through 17 months
// forward of the current month (24 months total, current included).
const (
	MonthsBack    = 6
	MonthsForward = 17
)

// MonthAgg is the aggregation for a single month.
type MonthAgg struct {
	Month         string       `json:"month"`  // "YYYY-MM"
	Income        domain.Money `json:"income"`
	Expense       domain.Money `json:"expense"`
	Net           domain.Money `json:"net"`
	CumulativeNet domain.Money `json:"cumulative_net"`
	SavingsRate   float64      `json:"savings_rate"`

	PrevMonthSalary     domain.Money `json:"prev_month_salary"`
	PayCycleNet         domain.Money `json:"pay_cycle_net"`
	PayCycleSavingsRate float64      `json:"pay_cycle_savings_rate"`
	RunningBuffer       domain.Money `json:"running_buffer"`
}

// MonthKey returns the "YYYY-MM" key for t.
func MonthKey(t time.Time) string { return t.Format("2006-01") }

// addMonths returns the month key n months after key ("YYYY-MM").
func addMonths(key string, n int) string {
	t, err := time.Parse("2006-01", key)
	if err != nil {
		return key
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}

// rawMonth accumulates income/expense for a single month before derived fields.
type rawMonth struct{ income, expense domain.Money }

// monthlyRaw sums income and expense per month across all transactions.
func monthlyRaw(txs []domain.Transaction) map[string]*rawMonth {
	m := map[string]*rawMonth{}
	for _, t := range txs {
		key := t.Month()
		r := m[key]
		if r == nil {
			r = &rawMonth{}
			m[key] = r
		}
		switch t.Type {
		case domain.Income:
			r.income += t.Amount
		case domain.Expense:
			r.expense += t.Amount
		}
	}
	return m
}

// MonthlyAggregates returns the per-month aggregation for the display window
// (current-6 .. current+17). Cumulative and running-buffer fields are computed
// over the full union of months present in the data plus the window, so values
// at the window edges correctly include history before the window.
func MonthlyAggregates(txs []domain.Transaction, asOf time.Time) []MonthAgg {
	raw := monthlyRaw(txs)
	current := MonthKey(asOf)

	// Union of all months (data + window), sorted ascending.
	set := map[string]struct{}{}
	for k := range raw {
		set[k] = struct{}{}
	}
	for i := -MonthsBack; i <= MonthsForward; i++ {
		set[addMonths(current, i)] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Compute derived fields cumulatively over the sorted union.
	agg := map[string]MonthAgg{}
	var cumNet, runBuffer domain.Money
	for _, k := range keys {
		var inc, exp domain.Money
		if r := raw[k]; r != nil {
			inc, exp = r.income, r.expense
		}
		net := inc - exp
		cumNet += net

		var prevSalary domain.Money
		if r := raw[addMonths(k, -1)]; r != nil {
			prevSalary = r.income
		}
		payCycleNet := prevSalary - exp
		runBuffer += payCycleNet

		agg[k] = MonthAgg{
			Month:               k,
			Income:              inc,
			Expense:             exp,
			Net:                 net,
			CumulativeNet:       cumNet,
			SavingsRate:         ratio(net, inc),
			PrevMonthSalary:     prevSalary,
			PayCycleNet:         payCycleNet,
			PayCycleSavingsRate: ratio(payCycleNet, prevSalary),
			RunningBuffer:       runBuffer,
		}
	}

	// Extract the window in order.
	out := make([]MonthAgg, 0, MonthsBack+MonthsForward+1)
	for i := -MonthsBack; i <= MonthsForward; i++ {
		out = append(out, agg[addMonths(current, i)])
	}
	return out
}

// MonthSummary returns income, expense, and net for a single month key.
func MonthSummary(txs []domain.Transaction, month string) (income, expense, net domain.Money) {
	for _, t := range txs {
		if t.Month() != month {
			continue
		}
		switch t.Type {
		case domain.Income:
			income += t.Amount
		case domain.Expense:
			expense += t.Amount
		}
	}
	return income, expense, income - expense
}

// PayCycle returns the pay-cycle figures for a month: previous month's salary
// (income), pay-cycle net (prevSalary - thisMonthExpense), and savings rate.
func PayCycle(txs []domain.Transaction, month string) (prevSalary, payCycleNet domain.Money, savingsRate float64) {
	prevSalary, _, _ = MonthSummary(txs, addMonths(month, -1))
	_, expense, _ := MonthSummary(txs, month)
	payCycleNet = prevSalary - expense
	return prevSalary, payCycleNet, ratio(payCycleNet, prevSalary)
}

// CategoryStat is per-category spending (expense only).
type CategoryStat struct {
	Category   string       `json:"category"`
	TotalSpent domain.Money `json:"total_spent"`
	Count      int          `json:"count"`
	PctOfTotal float64      `json:"pct_of_total"`
}

// CategoryAggregates returns expense totals per category, sorted by spend desc,
// with each category's share of all expense.
func CategoryAggregates(txs []domain.Transaction) []CategoryStat {
	totals := map[string]*CategoryStat{}
	var grand domain.Money
	for _, t := range txs {
		if t.Type != domain.Expense {
			continue
		}
		c := totals[t.Category]
		if c == nil {
			c = &CategoryStat{Category: t.Category}
			totals[t.Category] = c
		}
		c.TotalSpent += t.Amount
		c.Count++
		grand += t.Amount
	}
	out := make([]CategoryStat, 0, len(totals))
	for _, c := range totals {
		c.PctOfTotal = ratio(c.TotalSpent, grand)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalSpent != out[j].TotalSpent {
			return out[i].TotalSpent > out[j].TotalSpent
		}
		return out[i].Category < out[j].Category
	})
	return out
}

// YTDDebits returns total expense for the calendar year of asOf.
func YTDDebits(txs []domain.Transaction, asOf time.Time) domain.Money {
	year := asOf.Format("2006")
	var total domain.Money
	for _, t := range txs {
		if t.Type == domain.Expense && strings.HasPrefix(t.Date, year+"-") {
			total += t.Amount
		}
	}
	return total
}

// TopExpenses returns the n largest expense transactions, descending.
func TopExpenses(txs []domain.Transaction, n int) []domain.Transaction {
	var exp []domain.Transaction
	for _, t := range txs {
		if t.Type == domain.Expense {
			exp = append(exp, t)
		}
	}
	sort.Slice(exp, func(i, j int) bool { return exp[i].Amount > exp[j].Amount })
	if n > 0 && len(exp) > n {
		exp = exp[:n]
	}
	return exp
}

// Recent returns the n most recent transactions by date (desc).
func Recent(txs []domain.Transaction, n int) []domain.Transaction {
	cp := append([]domain.Transaction(nil), txs...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Date > cp[j].Date })
	if n > 0 && len(cp) > n {
		cp = cp[:n]
	}
	return cp
}

// ratio returns num/den as a float in [0,1+]; 0 when den == 0.
func ratio(num, den domain.Money) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}
