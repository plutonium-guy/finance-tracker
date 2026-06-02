package pdfimport

import (
	"testing"

	"finance-tracker/internal/domain"
)

func TestParseLines(t *testing.T) {
	lines := []string{
		"Date Narration Amount Balance",                    // header — no money-with-decimals except none; skipped
		"05/06/2026 UPI-SWIGGY-Order 450.00 12,300.00 Dr",  // expense
		"06/06/2026 SALARY CREDIT 85,000.00 97,300.00 Cr",  // income (Cr)
		"07-Jun-2026 AMAZON PURCHASE 1,299.50",             // expense, no balance
		"Opening Balance 10,000.00",                        // no date -> skipped
		"random footer text",                               // skipped
	}
	got := ParseLines(lines)
	if len(got) != 3 {
		t.Fatalf("parsed %d candidates, want 3: %+v", len(got), got)
	}

	if got[0].Date != "2026-06-05" || got[0].Amount != 450.00 || got[0].Type != domain.Expense {
		t.Errorf("line0 = %+v, want 2026-06-05 / 450 / Expense", got[0])
	}
	if !contains(got[0].Description, "SWIGGY") {
		t.Errorf("line0 desc = %q, want it to mention SWIGGY", got[0].Description)
	}
	if got[1].Type != domain.Income || got[1].Amount != 85000.00 {
		t.Errorf("line1 = %+v, want Income / 85000", got[1])
	}
	if got[2].Date != "2026-06-07" || got[2].Amount != 1299.50 {
		t.Errorf("line2 = %+v, want 2026-06-07 / 1299.50", got[2])
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
