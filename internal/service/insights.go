package service

import (
	"sort"
	"time"

	"finance-tracker/internal/domain"
)

// CategoryMove is a category's spend this month vs the previous month.
type CategoryMove struct {
	Category string
	This     domain.Money
	Prev     domain.Money
	Delta    domain.Money // This − Prev (positive = spent more)
}

// MonthInsights compares a month's spending to the prior month.
type MonthInsights struct {
	HasPrev         bool
	ExpenseThis     domain.Money
	ExpensePrev     domain.Money
	ExpenseDelta    domain.Money
	ExpenseDeltaPct float64
	TopMovers       []CategoryMove // largest absolute change first, up to 5
}

// MonthInsightsFor builds month-over-month insights for "YYYY-MM".
func MonthInsightsFor(txs []domain.Transaction, month string) MonthInsights {
	prev := addMonthKeyS(month, -1)
	thisCat := expenseByCategory(txs, month)
	prevCat := expenseByCategory(txs, prev)

	var ins MonthInsights
	for _, v := range thisCat {
		ins.ExpenseThis += v
	}
	for _, v := range prevCat {
		ins.ExpensePrev += v
	}
	ins.HasPrev = ins.ExpensePrev > 0
	ins.ExpenseDelta = ins.ExpenseThis - ins.ExpensePrev
	ins.ExpenseDeltaPct = ratio(ins.ExpenseDelta, ins.ExpensePrev)

	// Union of categories, biggest absolute change first.
	seen := map[string]bool{}
	var moves []CategoryMove
	add := func(cat string) {
		if seen[cat] {
			return
		}
		seen[cat] = true
		moves = append(moves, CategoryMove{Category: cat, This: thisCat[cat], Prev: prevCat[cat], Delta: thisCat[cat] - prevCat[cat]})
	}
	for c := range thisCat {
		add(c)
	}
	for c := range prevCat {
		add(c)
	}
	sort.Slice(moves, func(i, j int) bool { return absMoney(moves[i].Delta) > absMoney(moves[j].Delta) })
	if len(moves) > 5 {
		moves = moves[:5]
	}
	ins.TopMovers = moves
	return ins
}

func expenseByCategory(txs []domain.Transaction, month string) map[string]domain.Money {
	out := map[string]domain.Money{}
	for _, t := range txs {
		if t.Type == domain.Expense && t.Month() == month {
			out[t.Category] += t.Amount
		}
	}
	return out
}

func addMonthKeyS(key string, n int) string {
	t, err := time.Parse("2006-01", key)
	if err != nil {
		return key
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}

func absMoney(m domain.Money) domain.Money {
	if m < 0 {
		return -m
	}
	return m
}
