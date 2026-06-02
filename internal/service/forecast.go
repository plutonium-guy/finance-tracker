package service

import (
	"time"

	"github.com/google/uuid"

	"finance-tracker/internal/domain"
)

// PostDue posts every active recurring item due in month that hasn't already
// been posted, creating one transaction each. Idempotent via posted_recurring.
// Returns the number of items posted. Used by the "Post due" button and, when
// RECURRING_AUTOFIRE is set, on startup.
func (s *Service) PostDue(month string) (int, error) {
	items, err := s.Store.ListRecurring()
	if err != nil {
		return 0, err
	}
	due := DueRecurring(items, month, func(id string) bool {
		posted, _ := s.Store.WasPosted(id, month)
		return posted
	})
	now := s.now().UTC().Format(time.RFC3339)
	posted := 0
	for _, r := range due {
		t := domain.Transaction{
			ID:            uuid.NewString(),
			Date:          PostingDate(r, month),
			Description:   r.Name,
			Amount:        r.Amount,
			Type:          r.Type,
			PaymentMethod: r.PaymentMethod,
			Category:      r.Category,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		note := "auto-posted from recurring"
		t.Notes = &note
		if err := s.Store.CreateTransaction(t); err != nil {
			continue
		}
		s.Store.MarkPosted(r.ID, month, t.ID, now)
		posted++
	}
	return posted, nil
}

// Forecast projects where this month's net cashflow will land, by adding the
// recurring obligations still due (not yet posted, posting date today or later)
// to the actual net booked so far.
type Forecast struct {
	Month           string       `json:"month"`
	NetSoFar        domain.Money `json:"net_so_far"`
	UpcomingIncome  domain.Money `json:"upcoming_income"`
	UpcomingExpense domain.Money `json:"upcoming_expense"`
	ProjectedNet    domain.Money `json:"projected_net"`
	DaysLeft        int          `json:"days_left"`
}

// ForecastFor builds the cashflow projection for month from actual transactions
// plus the recurring items still expected to fire this month.
func (s *Service) ForecastFor(txs []domain.Transaction, recurring []domain.RecurringItem, month string) Forecast {
	_, _, net := MonthSummary(txs, month)
	asOf := s.now()
	today := asOf.Format(isoDate)
	f := Forecast{Month: month, NetSoFar: net}

	// Only project for the current month; other months have no "upcoming".
	if month == MonthKey(asOf) {
		for _, r := range recurring {
			if !DueInMonth(r, month) {
				continue
			}
			if posted, _ := s.Store.WasPosted(r.ID, month); posted {
				continue
			}
			// Skip items whose posting date already passed — likely logged by hand.
			if PostingDate(r, month) < today {
				continue
			}
			switch r.Type {
			case domain.Income:
				f.UpcomingIncome += r.Amount
			case domain.Expense:
				f.UpcomingExpense += r.Amount
			}
		}
		last := time.Date(asOf.Year(), asOf.Month()+1, 0, 0, 0, 0, 0, asOf.Location()).Day()
		f.DaysLeft = last - asOf.Day()
	}
	f.ProjectedNet = f.NetSoFar + f.UpcomingIncome - f.UpcomingExpense
	return f
}
