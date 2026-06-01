package domain

import "testing"

func TestToPaiseRoundHalfUp(t *testing.T) {
	cases := []struct {
		in   float64
		want Money
	}{
		{0, 0},
		{1, 100},
		{1.005, 101}, // half-up
		{2895.40, 289540},
		{233053, 23305300},
		{0.014, 1},  // rounds down
		{0.015, 2},  // half-up
		{-1.005, -101},
		{-2895.40, -289540},
	}
	for _, c := range cases {
		if got := ToPaise(c.in); got != c.want {
			t.Errorf("ToPaise(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFromPaise(t *testing.T) {
	if got := Money(289540).FromPaise(); got != 2895.40 {
		t.Errorf("FromPaise = %v, want 2895.40", got)
	}
}

func TestFormatINR(t *testing.T) {
	cases := []struct {
		in   Money
		want string
	}{
		{0, "₹0.00"},
		{100, "₹1.00"},
		{12345678, "₹1,23,456.78"},
		{23305300, "₹2,33,053.00"},
		{289540, "₹2,895.40"},
		{100000000, "₹10,00,000.00"},
		{1000000000, "₹1,00,00,000.00"}, // 1 crore
		{-123400, "(₹1,234.00)"},
		{-100, "(₹1.00)"},
	}
	for _, c := range cases {
		if got := c.in.FormatINR(); got != c.want {
			t.Errorf("FormatINR(%d) = %q, want %q", int64(c.in), got, c.want)
		}
	}
}

func TestSignedAmount(t *testing.T) {
	inc := Transaction{Amount: 1000, Type: Income}
	exp := Transaction{Amount: 1000, Type: Expense}
	xfer := Transaction{Amount: 1000, Type: Transfer}
	if inc.SignedAmount() != 1000 {
		t.Errorf("income signed = %d, want 1000", inc.SignedAmount())
	}
	if exp.SignedAmount() != -1000 {
		t.Errorf("expense signed = %d, want -1000", exp.SignedAmount())
	}
	if xfer.SignedAmount() != 0 {
		t.Errorf("transfer signed = %d, want 0", xfer.SignedAmount())
	}
}

func TestTransactionMonth(t *testing.T) {
	if m := (Transaction{Date: "2026-05-29"}).Month(); m != "2026-05" {
		t.Errorf("Month = %q, want 2026-05", m)
	}
}
