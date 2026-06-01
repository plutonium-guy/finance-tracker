package service

import (
	"testing"
	"time"

	"finance-tracker/internal/domain"
)

func ccTx(id, date string, rupees float64, typ domain.TransactionType) domain.Transaction {
	return domain.Transaction{ID: id, Date: date, Amount: domain.ToPaise(rupees), Type: typ, PaymentMethod: domain.CreditCard}
}

func TestCardSummaryCycle(t *testing.T) {
	card := domain.Card{
		ID: "c1", Name: "ICICI", Last4: "4003",
		Limit: domain.ToPaise(1030000), StatementDay: 18, DueOffsetDays: 18,
	}
	asOf := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	txs := []domain.Transaction{
		ccTx("a", "2026-04-18", 500, domain.Expense),  // on prev-prev statement (older)
		ccTx("b", "2026-05-10", 6200, domain.Expense), // last statement cycle (Apr18, May18]
		ccTx("c", "2026-05-20", 1000, domain.Expense), // current cycle (May18, now]
		ccTx("d", "2026-06-01", 240, domain.Expense),  // current cycle, on asOf
		ccTx("e", "2026-05-12", 9999, domain.Income),  // income on card -> ignored
		ccTx("f", "2026-05-12", 7777, domain.Expense), // belongs to a different card
	}
	txCard := map[string]string{"a": "c1", "b": "c1", "c": "c1", "d": "c1", "e": "c1", "f": "other"}

	s := CardSummaryFor(card, txs, txCard, nil, asOf)

	if got, want := s.LastStatementDate.Format("2006-01-02"), "2026-05-18"; got != want {
		t.Errorf("LastStatementDate = %s, want %s", got, want)
	}
	if s.CycleStart.Format("2006-01-02") != "2026-05-19" || s.CycleEnd.Format("2006-01-02") != "2026-06-18" {
		t.Errorf("cycle = %s..%s, want 2026-05-19..2026-06-18", s.CycleStart.Format("2006-01-02"), s.CycleEnd.Format("2006-01-02"))
	}
	if s.LastStatementAmount != domain.ToPaise(6200) {
		t.Errorf("LastStatementAmount = %s, want ₹6200", s.LastStatementAmount.FormatINR())
	}
	if s.Accrued != domain.ToPaise(1240) {
		t.Errorf("Accrued = %s, want ₹1240", s.Accrued.FormatINR())
	}
	// outstanding = 500 + 6200 + 1000 + 240 = 7940 (income and other-card excluded)
	if s.Outstanding != domain.ToPaise(7940) {
		t.Errorf("Outstanding = %s, want ₹7940", s.Outstanding.FormatINR())
	}
	if s.Available != domain.ToPaise(1030000-7940) {
		t.Errorf("Available = %s, want ₹%v", s.Available.FormatINR(), 1030000-7940)
	}
	if !s.HasStatement || s.Paid || s.Overdue {
		t.Errorf("flags: HasStatement=%v Paid=%v Overdue=%v, want true/false/false", s.HasStatement, s.Paid, s.Overdue)
	}
	if s.DueDate.Format("2006-01-02") != "2026-06-05" {
		t.Errorf("DueDate = %s, want 2026-06-05", s.DueDate.Format("2006-01-02"))
	}
}

func TestCardSummaryPaidAndOverdue(t *testing.T) {
	card := domain.Card{ID: "c1", Name: "HDFC", Limit: domain.ToPaise(100000), StatementDay: 18, DueOffsetDays: 18}
	txs := []domain.Transaction{ccTx("b", "2026-05-10", 6200, domain.Expense)}
	txCard := map[string]string{"b": "c1"}

	// Paid: a payment recorded for the May-18 statement clears the flag and balance.
	pays := []domain.StatementPayment{{CardID: "c1", PeriodEnd: "2026-05-18", Amount: domain.ToPaise(6200)}}
	s := CardSummaryFor(card, txs, txCard, pays, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if !s.Paid {
		t.Error("expected Paid=true after recording the statement payment")
	}
	if s.Outstanding != 0 {
		t.Errorf("Outstanding = %s, want ₹0 after payment", s.Outstanding.FormatINR())
	}

	// Overdue: unpaid and now past the due date (May18 + 18d = Jun 5).
	s2 := CardSummaryFor(card, txs, txCard, nil, time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC))
	if !s2.Overdue {
		t.Error("expected Overdue=true when unpaid and past due date")
	}
}
