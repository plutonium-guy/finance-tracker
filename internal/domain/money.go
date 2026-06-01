package domain

import (
	"math"
	"strconv"
	"strings"
)

// Money is an amount stored internally as int64 paise (1 rupee = 100 paise).
// All arithmetic happens in paise; floats appear only at the edges.
type Money int64

// ToPaise converts a rupee float to paise, rounding half-up (away from zero at
// the .5 boundary) to 2 decimals. It first snaps floating-point representation
// noise at the 1e-6 scale so decimal-exact inputs like 1.005 round as a human
// expects (→ 101 paise) rather than being dragged down by binary error.
func ToPaise(rupees float64) Money {
	scaled := rupees * 100
	denoised := math.Round(scaled*1e6) / 1e6
	return Money(int64(math.Round(denoised)))
}

// FromPaise converts paise back to a rupee float.
func (m Money) FromPaise() float64 {
	return float64(m) / 100
}

// Rupees is an alias for FromPaise for readability at call sites.
func (m Money) Rupees() float64 { return m.FromPaise() }

// FormatINR renders paise using Indian digit grouping with a ₹ prefix and two
// decimals, e.g. 12345678 paise -> "₹1,23,456.78". Negative values are rendered
// in accounting style with parentheses, e.g. "(₹1,234.00)".
func (m Money) FormatINR() string {
	neg := m < 0
	v := int64(m)
	if neg {
		v = -v
	}
	rupees := v / 100
	paise := v % 100

	grouped := groupIndian(rupees)
	s := "₹" + grouped + "." + pad2(paise)
	if neg {
		return "(" + s + ")"
	}
	return s
}

// IsNegative reports whether the amount is below zero (for styling hooks).
func (m Money) IsNegative() bool { return m < 0 }

// groupIndian formats a non-negative integer with Indian grouping: the last
// three digits, then groups of two (e.g. 1234567 -> "12,34,567").
func groupIndian(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	last3 := s[len(s)-3:]
	rest := s[:len(s)-3]

	var groups []string
	for len(rest) > 2 {
		groups = append([]string{rest[len(rest)-2:]}, groups...)
		rest = rest[:len(rest)-2]
	}
	if len(rest) > 0 {
		groups = append([]string{rest}, groups...)
	}
	return strings.Join(groups, ",") + "," + last3
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
