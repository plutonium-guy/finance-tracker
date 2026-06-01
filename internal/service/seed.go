package service

import (
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
)

// ptr returns a pointer to v (for optional fields).
func ptr[T any](v T) *T { return &v }

// SeedTransactions returns the demo transactions from the build spec, with
// amounts converted to paise and dates in ISO form. IDs are freshly generated.
func SeedTransactions() []domain.Transaction {
	const ts = "2026-06-01T00:00:00Z"
	rows := []struct {
		date string
		desc string
		amt  float64
		typ  domain.TransactionType
		pm   domain.PaymentMethod
		cat  string
		note string
	}{
		{"2026-06-01", "GROCETER", 31.00, domain.Expense, domain.CreditCard, "Groceries", "ICICI CC"},
		{"2026-06-01", "Mr Aasi", 80.00, domain.Expense, domain.CreditCard, "Miscellaneous", "ICICI CC"},
		{"2026-05-29", "Salary — May 2026", 233053, domain.Income, domain.BankTransfer, "Salary", "Sample"},
		{"2026-05-28", "ixigo", 8526, domain.Expense, domain.CreditCard, "Travel", ""},
		{"2026-05-28", "ixigo", 6374, domain.Expense, domain.CreditCard, "Travel", ""},
		{"2026-05-22", "KUNDAN S", 950, domain.Expense, domain.CreditCard, "Miscellaneous", ""},
		{"2026-05-17", "ixigo", 36292, domain.Expense, domain.CreditCard, "Travel", ""},
		{"2026-05-10", "JAYPEE FACTORY OUTLETS", 6200, domain.Expense, domain.CreditCard, "Shopping", ""},
		{"2026-05-10", "DOLLY MOTORS", 2895.40, domain.Expense, domain.CreditCard, "Transport", ""},
		{"2026-05-08", "Urban Company", 13999, domain.Expense, domain.CreditCard, "Utilities", ""},
		{"2026-05-03", "Maruti Suzuki", 10743, domain.Expense, domain.CreditCard, "Transport", ""},
	}
	out := make([]domain.Transaction, 0, len(rows))
	for _, r := range rows {
		t := domain.Transaction{
			ID:            uuid.NewString(),
			Date:          r.date,
			Description:   r.desc,
			Amount:        domain.ToPaise(r.amt),
			Type:          r.typ,
			PaymentMethod: r.pm,
			Category:      r.cat,
			CreatedAt:     ts,
			UpdatedAt:     ts,
		}
		note := r.note
		if note == "" {
			note = "Demo Data — Delete Me"
		}
		t.Notes = ptr(note)
		out = append(out, t)
	}
	return out
}

// SeedRecurring returns the demo recurring items from the build spec.
func SeedRecurring() []domain.RecurringItem {
	const ts = "2026-06-01T00:00:00Z"
	rows := []struct {
		name  string
		cat   string
		amt   float64
		freq  domain.Frequency
		typ   domain.TransactionType
		start string
		pm    domain.PaymentMethod
	}{
		{"UTI Nifty 50 SIP", "Investment", 20000, domain.Monthly, domain.Expense, "2026-06-01", domain.BankTransfer},
		{"Nippon Next 50 SIP", "Investment", 16000, domain.Monthly, domain.Expense, "2026-06-01", domain.BankTransfer},
		{"Parag Parikh Flexi Cap SIP", "Investment", 20000, domain.Monthly, domain.Expense, "2026-06-01", domain.BankTransfer},
		{"Gold ETF GOLDBEES", "Investment", 20000, domain.Monthly, domain.Expense, "2026-06-01", domain.BankTransfer},
		{"Direct stocks (8 × ₹3k)", "Investment", 24000, domain.Monthly, domain.Expense, "2026-06-01", domain.BankTransfer},
		{"Salary", "Salary", 233053, domain.Monthly, domain.Income, "2026-05-29", domain.BankTransfer},
		{"Term Life Insurance", "Insurance", 18000, domain.Yearly, domain.Expense, "2026-01-15", domain.BankTransfer},
		{"Health Insurance", "Insurance", 25000, domain.Yearly, domain.Expense, "2026-03-01", domain.CreditCard},
		{"Netflix", "Entertainment", 649, domain.Monthly, domain.Expense, "2026-01-01", domain.CreditCard},
	}
	out := make([]domain.RecurringItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.RecurringItem{
			ID:            uuid.NewString(),
			Name:          r.name,
			Category:      r.cat,
			Amount:        domain.ToPaise(r.amt),
			Frequency:     r.freq,
			Type:          r.typ,
			StartDate:     r.start,
			PaymentMethod: r.pm,
			Active:        true,
			CreatedAt:     ts,
			UpdatedAt:     ts,
		})
	}
	return out
}
