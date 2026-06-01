package service

import (
	"time"

	"finance-tracker/internal/domain"
)

// CardSummary is the computed billing-cycle view of a card.
type CardSummary struct {
	Card domain.Card

	CycleStart time.Time     // first day of the current (open) cycle
	CycleEnd   time.Time     // next statement (close) date
	Accrued    domain.Money  // spend in the current open cycle so far

	HasStatement        bool         // a statement has closed with spend on it
	LastStatementDate   time.Time    // most recent closing date <= asOf
	LastStatementAmount domain.Money // spend billed on that statement
	DueDate             time.Time    // LastStatementDate + DueOffsetDays
	Paid                bool         // that statement has a recorded payment
	Overdue             bool         // unpaid and past the due date

	Outstanding domain.Money // total owed now (all purchases − all payments)
	Available   domain.Money // Limit − Outstanding (may be negative if over limit)
	UsedPct     float64      // Outstanding / Limit (0 when no limit set)
}

// CardSummaryFor computes a card's cycle/statement view from the full
// transaction set, the txID→cardID map, the card's recorded payments, and a
// reference time. Only transactions linked to this card and of type Expense
// count as purchases.
func CardSummaryFor(card domain.Card, txs []domain.Transaction, txCard map[string]string, payments []domain.StatementPayment, asOf time.Time) CardSummary {
	asOf = dateOnly(asOf)
	lastStmt := statementOnOrBefore(card.StatementDay, asOf)
	prevStmt := lastStmt.AddDate(0, -1, 0)
	nextStmt := lastStmt.AddDate(0, 1, 0)

	var accrued, lastAmount, totalPurchases domain.Money
	for _, t := range txs {
		if t.Type != domain.Expense || txCard[t.ID] != card.ID {
			continue
		}
		d, ok := parseDate(t.Date)
		if !ok || d.After(asOf) {
			continue
		}
		totalPurchases += t.Amount
		switch {
		case d.After(lastStmt): // current open cycle
			accrued += t.Amount
		case d.After(prevStmt): // cycle that closed on lastStmt
			lastAmount += t.Amount
		}
	}

	var totalPaid domain.Money
	paidLast := false
	lastKey := lastStmt.Format("2006-01-02")
	for _, p := range payments {
		totalPaid += p.Amount
		if p.PeriodEnd == lastKey {
			paidLast = true
		}
	}

	due := lastStmt.AddDate(0, 0, card.DueOffsetDays)
	s := CardSummary{
		Card:                card,
		CycleStart:          lastStmt.AddDate(0, 0, 1),
		CycleEnd:            nextStmt,
		Accrued:             accrued,
		HasStatement:        lastAmount > 0,
		LastStatementDate:   lastStmt,
		LastStatementAmount: lastAmount,
		DueDate:             due,
		Paid:                paidLast,
		Outstanding:         totalPurchases - totalPaid,
		Available:           card.Limit - (totalPurchases - totalPaid),
	}
	s.Overdue = s.HasStatement && !s.Paid && asOf.After(due)
	if card.Limit > 0 {
		s.UsedPct = float64(s.Outstanding) / float64(card.Limit)
	}
	return s
}

// statementOnOrBefore returns the most recent statement closing date (day-of-month
// = day, capped at 28) that is on or before asOf.
func statementOnOrBefore(day int, asOf time.Time) time.Time {
	if day < 1 {
		day = 1
	}
	if day > 28 {
		day = 28
	}
	cur := time.Date(asOf.Year(), asOf.Month(), day, 0, 0, 0, 0, time.UTC)
	if cur.After(asOf) {
		cur = cur.AddDate(0, -1, 0)
	}
	return cur
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func parseDate(s string) (time.Time, bool) {
	if len(s) > 10 {
		s = s[:10]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
