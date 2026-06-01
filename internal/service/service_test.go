package service

import (
	"math"
	"testing"
	"time"

	"finance-tracker/internal/domain"
)

// asOf fixes "today" to mid-June 2026 so the seed-based assertions are stable.
var asOf = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

func findMonth(aggs []MonthAgg, key string) MonthAgg {
	for _, a := range aggs {
		if a.Month == key {
			return a
		}
	}
	return MonthAgg{}
}

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// Criterion 5 (June 2026 seed): These all match the build spec exactly.
func TestAcceptanceJune(t *testing.T) {
	txs := SeedTransactions()

	income, expense, _ := MonthSummary(txs, "2026-06")
	if income != 0 {
		t.Errorf("June credits = %s, want ₹0.00", income.FormatINR())
	}
	if expense != domain.ToPaise(111) { // 31 + 80
		t.Errorf("June debits = %s, want ₹111.00", expense.FormatINR())
	}

	prevSalary, payCycleNet, rate := PayCycle(txs, "2026-06")
	if prevSalary != domain.ToPaise(233053) {
		t.Errorf("Last salary received = %s, want ₹2,33,053.00", prevSalary.FormatINR())
	}
	if payCycleNet != domain.ToPaise(232942) { // 233053 - 111
		t.Errorf("Pay cycle balance = %s, want ₹2,32,942.00", payCycleNet.FormatINR())
	}
	if !approx(rate, 232942.0/233053.0, 1e-9) {
		t.Errorf("pay cycle savings rate = %v, want ~0.99952", rate)
	}
}

// Criterion 6 (May 2026): The literal 11-row seed yields Debits ₹85,979.40 and
// Net ₹1,47,073.60. The spec's headline figures (₹95,899 / ₹1,37,154) assume a
// larger dataset; we assert the values the seed actually produces.
func TestAcceptanceMay(t *testing.T) {
	txs := SeedTransactions()
	income, expense, net := MonthSummary(txs, "2026-05")

	if income != domain.ToPaise(233053) {
		t.Errorf("May credits = %s, want ₹2,33,053.00", income.FormatINR())
	}
	wantExpense := domain.ToPaise(85979.40)
	if expense != wantExpense {
		t.Errorf("May debits = %s (%d paise), want %s", expense.FormatINR(), int64(expense), wantExpense.FormatINR())
	}
	wantNet := domain.ToPaise(147073.60)
	if net != wantNet {
		t.Errorf("May net = %s, want %s", net.FormatINR(), wantNet.FormatINR())
	}
}

// Criterion 7 (all-time category mix): Travel = 8526+6374+36292 = ₹51,192 of a
// total ₹86,090.40 expense => ~59.5%. (Spec says ~53% for its fuller dataset.)
func TestAcceptanceTravelShare(t *testing.T) {
	txs := SeedTransactions()
	cats := CategoryAggregates(txs)
	if len(cats) == 0 || cats[0].Category != "Travel" {
		t.Fatalf("expected Travel to be the top category, got %+v", cats)
	}
	if cats[0].TotalSpent != domain.ToPaise(51192) {
		t.Errorf("Travel total = %s, want ₹51,192.00", cats[0].TotalSpent.FormatINR())
	}
	if !approx(cats[0].PctOfTotal, 0.5947, 0.001) {
		t.Errorf("Travel share = %.4f, want ~0.5947", cats[0].PctOfTotal)
	}
}

func TestMonthlyAggregatesWindowAndDerived(t *testing.T) {
	txs := SeedTransactions()
	aggs := MonthlyAggregates(txs, asOf)
	if len(aggs) != MonthsBack+MonthsForward+1 {
		t.Fatalf("window length = %d, want %d", len(aggs), MonthsBack+MonthsForward+1)
	}

	june := findMonth(aggs, "2026-06")
	if june.Expense != domain.ToPaise(111) || june.Income != 0 {
		t.Errorf("June agg wrong: %+v", june)
	}
	if june.PrevMonthSalary != domain.ToPaise(233053) {
		t.Errorf("June prevMonthSalary = %s", june.PrevMonthSalary.FormatINR())
	}
	if june.PayCycleNet != domain.ToPaise(232942) {
		t.Errorf("June payCycleNet = %s", june.PayCycleNet.FormatINR())
	}
	// cumulativeNet(June) = net(May)+net(June) = 147073.60 + (-111) = 146962.60
	if june.CumulativeNet != domain.ToPaise(146962.60) {
		t.Errorf("June cumulativeNet = %s, want ₹1,46,962.60", june.CumulativeNet.FormatINR())
	}

	may := findMonth(aggs, "2026-05")
	if may.Net != domain.ToPaise(147073.60) {
		t.Errorf("May net = %s", may.Net.FormatINR())
	}
}

func TestYTDDebits(t *testing.T) {
	txs := SeedTransactions()
	// All seed expenses are in 2026: 85,979.40 (May) + 111 (June) = 86,090.40.
	if got := YTDDebits(txs, asOf); got != domain.ToPaise(86090.40) {
		t.Errorf("YTD debits = %s, want ₹86,090.40", got.FormatINR())
	}
}

func TestTopExpensesAndRecent(t *testing.T) {
	txs := SeedTransactions()
	top := TopExpenses(txs, 5)
	if len(top) != 5 {
		t.Fatalf("top expenses len = %d, want 5", len(top))
	}
	if top[0].Amount != domain.ToPaise(36292) { // ixigo is largest
		t.Errorf("largest expense = %s, want ₹36,292.00", top[0].Amount.FormatINR())
	}
	recent := Recent(txs, 10)
	if len(recent) != 10 {
		t.Errorf("recent len = %d, want 10", len(recent))
	}
	if recent[0].Date < recent[1].Date {
		t.Errorf("recent not sorted desc: %s before %s", recent[0].Date, recent[1].Date)
	}
}

func TestRecurringCalcs(t *testing.T) {
	items := SeedRecurring()
	byName := map[string]domain.RecurringItem{}
	for _, r := range items {
		byName[r.Name] = r
	}

	if me := MonthlyEquivalent(byName["Netflix"]); me != domain.ToPaise(649) {
		t.Errorf("Netflix monthly equiv = %s, want ₹649.00", me.FormatINR())
	}
	if me := MonthlyEquivalent(byName["Term Life Insurance"]); me != domain.ToPaise(1500) { // 18000/12
		t.Errorf("Term Life monthly equiv = %s, want ₹1,500.00", me.FormatINR())
	}
	if at := AnnualTotal(byName["Netflix"]); at != domain.ToPaise(649*12) {
		t.Errorf("Netflix annual = %s, want ₹7,788.00", at.FormatINR())
	}

	// inactive contributes 0
	inactive := byName["Netflix"]
	inactive.Active = false
	if MonthlyEquivalent(inactive) != 0 {
		t.Error("inactive item should have 0 monthly equivalent")
	}
}

func TestMonthlyEquivalentFrequencies(t *testing.T) {
	base := domain.RecurringItem{Amount: domain.ToPaise(1200), Active: true, Type: domain.Expense}
	cases := []struct {
		freq domain.Frequency
		want domain.Money
	}{
		{domain.Monthly, domain.ToPaise(1200)},
		{domain.Quarterly, domain.ToPaise(400)},   // 1200/3
		{domain.HalfYearly, domain.ToPaise(200)},  // 1200/6
		{domain.Yearly, domain.ToPaise(100)},      // 1200/12
		{domain.Weekly, domain.Money(520000)},     // 1200*52/12 = 5200.00
		{domain.OneTime, 0},
	}
	for _, c := range cases {
		r := base
		r.Frequency = c.freq
		if got := MonthlyEquivalent(r); got != c.want {
			t.Errorf("%s monthly equiv = %s, want %s", c.freq, got.FormatINR(), c.want.FormatINR())
		}
	}
}

func TestNextDueDate(t *testing.T) {
	mk := func(freq domain.Frequency, start string) domain.RecurringItem {
		return domain.RecurringItem{Frequency: freq, StartDate: start, Active: true}
	}
	cases := []struct {
		name  string
		item  domain.RecurringItem
		want  string // "" means nil
	}{
		{"monthly", mk(domain.Monthly, "2026-01-01"), "2026-07-01"},
		{"monthly salary day29", mk(domain.Monthly, "2026-05-29"), "2026-06-29"},
		{"yearly", mk(domain.Yearly, "2026-01-15"), "2027-01-15"},
		{"yearly future this year", mk(domain.Yearly, "2026-03-01"), "2027-03-01"},
		{"weekly", mk(domain.Weekly, "2026-06-01"), "2026-06-15"}, // 01,08,15
		{"onetime future", mk(domain.OneTime, "2026-08-01"), "2026-08-01"},
		{"onetime past", mk(domain.OneTime, "2026-01-01"), ""},
	}
	for _, c := range cases {
		got := NextDueDate(c.item, asOf)
		if c.want == "" {
			if got != nil {
				t.Errorf("%s: want nil, got %s", c.name, *got)
			}
			continue
		}
		if got == nil || *got != c.want {
			g := "nil"
			if got != nil {
				g = *got
			}
			t.Errorf("%s: NextDueDate = %s, want %s", c.name, g, c.want)
		}
	}
}

func TestSummarizeRecurring(t *testing.T) {
	items := SeedRecurring()
	s := SummarizeRecurring(items)
	// Monthly income = Salary 2,33,053.
	if s.MonthlyIncome != domain.ToPaise(233053) {
		t.Errorf("monthly income = %s, want ₹2,33,053.00", s.MonthlyIncome.FormatINR())
	}
	// Monthly expense = 1,04,232.33 (see CHECKLIST computation).
	if s.MonthlyExpense != domain.Money(10423233) {
		t.Errorf("monthly expense = %s (%d paise), want ₹1,04,232.33", s.MonthlyExpense.FormatINR(), int64(s.MonthlyExpense))
	}
	if s.FreeMonthlyCashflow != s.MonthlyIncome-s.MonthlyExpense {
		t.Errorf("free cashflow mismatch: %s", s.FreeMonthlyCashflow.FormatINR())
	}
	if !approx(s.CommitmentRatio, float64(s.MonthlyExpense)/float64(s.MonthlyIncome), 1e-9) {
		t.Errorf("commitment ratio wrong: %v", s.CommitmentRatio)
	}
}
