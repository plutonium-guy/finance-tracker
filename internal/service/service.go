package service

import (
	"time"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/store"
)

// Service wires the store with business logic and a clock, and produces the
// view models the web layer renders.
type Service struct {
	Store store.Store
	now   func() time.Time
}

// New builds a Service. If now is nil, time.Now is used.
func New(s store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{Store: s, now: now}
}

// Now returns the service's notion of the current time.
func (s *Service) Now() time.Time { return s.now() }

// CurrentMonth returns the current "YYYY-MM" key.
func (s *Service) CurrentMonth() string { return MonthKey(s.now()) }

// DemoSeed inserts the demo transactions and recurring items when the store is
// empty. Categories are expected to already be seeded.
func (s *Service) DemoSeed() error {
	empty, err := s.Store.IsEmpty()
	if err != nil || !empty {
		return err
	}
	for _, t := range SeedTransactions() {
		if err := s.Store.CreateTransaction(t); err != nil {
			return err
		}
	}
	for _, r := range SeedRecurring() {
		if err := s.Store.CreateRecurring(r); err != nil {
			return err
		}
	}
	return nil
}

// KPIs is the primary dashboard KPI strip.
type KPIs struct {
	MonthCredits domain.Money
	MonthDebits  domain.Money
	MonthNet     domain.Money
	YTDDebits    domain.Money
	SavingsRate  float64
}

// PayCycleKPIs is the pay-cycle KPI strip.
type PayCycleKPIs struct {
	LastSalary     domain.Money
	MonthBills     domain.Money
	Balance        domain.Money
	SavingsRate    float64
	NextSalaryDate string // ISO or "" if none
}

// KPIsFor builds the primary KPI strip for a month.
func (s *Service) KPIsFor(txs []domain.Transaction, month string) KPIs {
	income, expense, net := MonthSummary(txs, month)
	return KPIs{
		MonthCredits: income,
		MonthDebits:  expense,
		MonthNet:     net,
		YTDDebits:    YTDDebits(txs, s.now()),
		SavingsRate:  ratio(net, income),
	}
}

// PayCycleFor builds the pay-cycle KPI strip for a month.
func (s *Service) PayCycleFor(txs []domain.Transaction, recurring []domain.RecurringItem, month string) PayCycleKPIs {
	prevSalary, payCycleNet, rate := PayCycle(txs, month)
	k := PayCycleKPIs{
		LastSalary:  prevSalary,
		Balance:     payCycleNet,
		SavingsRate: rate,
	}
	_, expense, _ := MonthSummary(txs, month)
	k.MonthBills = expense
	// Next salary date = next due date of an active Salary recurring item.
	for _, r := range recurring {
		if r.Active && r.Category == "Salary" && r.Type == domain.Income {
			if d := NextDueDate(r, s.now()); d != nil {
				k.NextSalaryDate = *d
				break
			}
		}
	}
	return k
}

// ChartData is the JSON data island consumed by Chart.js. Amounts are in rupees.
type ChartData struct {
	Months         []string  `json:"months"`
	Income         []float64 `json:"income"`
	Expense        []float64 `json:"expense"`
	Net            []float64 `json:"net"`
	CumulativeNet  []float64 `json:"cumulativeNet"`
	PrevSalary     []float64 `json:"prevSalary"`
	PayCycleNet    []float64 `json:"payCycleNet"`
	RunningBuffer  []float64 `json:"runningBuffer"`
	CategoryLabels []string  `json:"categoryLabels"`
	CategoryValues []float64 `json:"categoryValues"`
}

// ChartDataFor builds the dashboard chart data island from all transactions.
func (s *Service) ChartDataFor(txs []domain.Transaction) ChartData {
	aggs := MonthlyAggregates(txs, s.now())
	cd := ChartData{}
	for _, a := range aggs {
		cd.Months = append(cd.Months, a.Month)
		cd.Income = append(cd.Income, a.Income.Rupees())
		cd.Expense = append(cd.Expense, a.Expense.Rupees())
		cd.Net = append(cd.Net, a.Net.Rupees())
		cd.CumulativeNet = append(cd.CumulativeNet, a.CumulativeNet.Rupees())
		cd.PrevSalary = append(cd.PrevSalary, a.PrevMonthSalary.Rupees())
		cd.PayCycleNet = append(cd.PayCycleNet, a.PayCycleNet.Rupees())
		cd.RunningBuffer = append(cd.RunningBuffer, a.RunningBuffer.Rupees())
	}
	for _, c := range CategoryAggregates(txs) {
		cd.CategoryLabels = append(cd.CategoryLabels, c.Category)
		cd.CategoryValues = append(cd.CategoryValues, c.TotalSpent.Rupees())
	}
	return cd
}

// RecurringRow is a recurring item with its computed figures, for the table.
type RecurringRow struct {
	Item domain.RecurringItem
	Calc RecurringCalc
}

// RecurringRows decorates items with computed monthly/annual/next-due figures.
func (s *Service) RecurringRows(items []domain.RecurringItem) []RecurringRow {
	out := make([]RecurringRow, 0, len(items))
	for _, r := range items {
		out = append(out, RecurringRow{Item: r, Calc: Calc(r, s.now())})
	}
	return out
}
