package telegrambot

import (
	"testing"
	"time"

	"finance-tracker/internal/domain"
)

var fixedNow = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

func TestParseMessage(t *testing.T) {
	cases := []struct {
		in      string
		amount  float64
		desc    string
		cat     string
		method  domain.PaymentMethod
		typ     domain.TransactionType
		date    string
		wantErr bool
	}{
		{in: "250 Coffee", amount: 250, desc: "Coffee", method: domain.UPI, typ: domain.Expense, date: "2026-06-15"},
		{in: "1,234.50 Big Lunch cat:Food method:cc", amount: 1234.50, desc: "Big Lunch", cat: "Food", method: domain.CreditCard, typ: domain.Expense, date: "2026-06-15"},
		{in: "₹99 Tea", amount: 99, desc: "Tea", method: domain.UPI, typ: domain.Expense, date: "2026-06-15"},
		{in: "/add 85000 Salary type:income method:bank", amount: 85000, desc: "Salary", method: domain.BankTransfer, typ: domain.Income, date: "2026-06-15"},
		{in: "500 Refund date:2026-05-01", amount: 500, desc: "Refund", method: domain.UPI, typ: domain.Expense, date: "2026-05-01"},
		{in: "", wantErr: true},
		{in: "Coffee 250", wantErr: true},   // amount must be first
		{in: "250", wantErr: true},          // missing description
		{in: "-5 Bad", wantErr: true},       // non-positive
		{in: "250 X type:bogus", wantErr: true},
		{in: "250 X method:bogus", wantErr: true},
		{in: "250 X date:2026-13-40", wantErr: true},
	}
	for _, c := range cases {
		p, err := ParseMessage(c.in, fixedNow)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseMessage(%q) expected error, got %+v", c.in, p)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMessage(%q) error: %v", c.in, err)
			continue
		}
		if p.Amount != c.amount || p.Description != c.desc || p.Category != c.cat ||
			p.Method != c.method || p.Type != c.typ || p.Date != c.date {
			t.Errorf("ParseMessage(%q) = %+v, want amt=%v desc=%q cat=%q method=%v type=%v date=%v",
				c.in, p, c.amount, c.desc, c.cat, c.method, c.typ, c.date)
		}
	}
}
