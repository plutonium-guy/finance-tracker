package service

import (
	"time"

	"finance-tracker/internal/domain"
)

const isoDate = "2006-01-02"

// RecurringCalc holds derived figures for a recurring item.
type RecurringCalc struct {
	MonthlyEquivalent domain.Money `json:"monthly_equivalent"`
	AnnualTotal       domain.Money `json:"annual_total"`
	NextDueDate       *string      `json:"next_due_date,omitempty"` // ISO; nil if none
}

// MonthlyEquivalent returns the per-month cost of a recurring item. Inactive
// items contribute 0. Division results are rounded half-up to the nearest paise.
func MonthlyEquivalent(r domain.RecurringItem) domain.Money {
	if !r.Active {
		return 0
	}
	a := r.Amount
	switch r.Frequency {
	case domain.Monthly:
		return a
	case domain.Quarterly:
		return divRound(a, 3)
	case domain.HalfYearly:
		return divRound(a, 6)
	case domain.Yearly:
		return divRound(a, 12)
	case domain.Weekly:
		return divRound(a*52, 12)
	default: // One-time
		return 0
	}
}

// AnnualTotal is the monthly equivalent times twelve.
func AnnualTotal(r domain.RecurringItem) domain.Money {
	return MonthlyEquivalent(r) * 12
}

// NextDueDate computes the next occurrence on or after asOf, per frequency.
// Returns nil for a One-time item whose start date is in the past.
func NextDueDate(r domain.RecurringItem, asOf time.Time) *string {
	start, err := time.Parse(isoDate, r.StartDate)
	if err != nil {
		return nil
	}
	today := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)

	switch r.Frequency {
	case domain.OneTime:
		if !start.Before(today) {
			return iso(start)
		}
		return nil

	case domain.Monthly:
		// Next occurrence of the start day-of-month on/after today.
		d := time.Date(today.Year(), today.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		for d.Before(today) {
			d = d.AddDate(0, 1, 0)
		}
		return iso(d)

	case domain.Yearly:
		// Next month/day anniversary on/after today.
		d := time.Date(today.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		for d.Before(today) {
			d = d.AddDate(1, 0, 0)
		}
		return iso(d)

	case domain.Quarterly:
		return iso(iterateDays(start, today, 90))
	case domain.HalfYearly:
		return iso(iterateDays(start, today, 180))
	case domain.Weekly:
		return iso(iterateDays(start, today, 7))
	}
	return nil
}

// Calc bundles all derived figures for a recurring item as of a given time.
func Calc(r domain.RecurringItem, asOf time.Time) RecurringCalc {
	return RecurringCalc{
		MonthlyEquivalent: MonthlyEquivalent(r),
		AnnualTotal:       AnnualTotal(r),
		NextDueDate:       NextDueDate(r, asOf),
	}
}

// RecurringSummary aggregates active recurring items into the dashboard summary.
type RecurringSummary struct {
	MonthlyExpense      domain.Money `json:"monthly_expense"`
	AnnualExpense       domain.Money `json:"annual_expense"`
	MonthlyIncome       domain.Money `json:"monthly_income"`
	FreeMonthlyCashflow domain.Money `json:"free_monthly_cashflow"`
	CommitmentRatio     float64      `json:"commitment_ratio"` // monthly expense / monthly income
}

// SummarizeRecurring computes the recurring summary across items.
func SummarizeRecurring(items []domain.RecurringItem) RecurringSummary {
	var s RecurringSummary
	for _, r := range items {
		me := MonthlyEquivalent(r)
		switch r.Type {
		case domain.Expense:
			s.MonthlyExpense += me
		case domain.Income:
			s.MonthlyIncome += me
		}
	}
	s.AnnualExpense = s.MonthlyExpense * 12
	s.FreeMonthlyCashflow = s.MonthlyIncome - s.MonthlyExpense
	s.CommitmentRatio = ratio(s.MonthlyExpense, s.MonthlyIncome)
	return s
}

// iterateDays advances start by stepDays until it is on/after today.
func iterateDays(start, today time.Time, stepDays int) time.Time {
	d := start
	for d.Before(today) {
		d = d.AddDate(0, 0, stepDays)
	}
	return d
}

// divRound divides paise by d, rounding half-up.
func divRound(a domain.Money, d int64) domain.Money {
	n := int64(a)
	if n >= 0 {
		return domain.Money((n + d/2) / d)
	}
	return domain.Money(-((-n + d/2) / d))
}

func iso(t time.Time) *string {
	s := t.Format(isoDate)
	return &s
}
