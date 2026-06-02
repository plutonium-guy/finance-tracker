package service

import (
	"testing"

	"finance-tracker/internal/domain"
)

func TestSummarizePortfolio(t *testing.T) {
	holdings := []domain.Holding{
		{
			ID: "h1", Name: "Flexi Cap", Type: domain.MutualFund,
			UnitsMicro: domain.UnitsToMicro(100), AvgCost: domain.ToPaise(50), LastPrice: domain.ToPaise(60),
		},
		{
			ID: "h2", Name: "Gold", Type: domain.GoldAsset,
			UnitsMicro: domain.UnitsToMicro(10), AvgCost: domain.ToPaise(6000), // no last price → valued at cost
		},
	}
	s := SummarizePortfolio(holdings)

	// h1: invested 100×50=5000, value 100×60=6000, pnl 1000
	if s.Holdings[0].Invested != domain.ToPaise(5000) || s.Holdings[0].MarketValue != domain.ToPaise(6000) {
		t.Errorf("h1 invested/value = %s/%s, want ₹5000/₹6000", s.Holdings[0].Invested.FormatINR(), s.Holdings[0].MarketValue.FormatINR())
	}
	if !s.Holdings[0].Priced {
		t.Error("h1 should be marked priced")
	}
	// h2: valued at cost 10×6000=60000, pnl 0
	if s.Holdings[1].MarketValue != domain.ToPaise(60000) || s.Holdings[1].PnL != 0 || s.Holdings[1].Priced {
		t.Errorf("h2 value/pnl/priced = %s/%s/%v, want ₹60000/₹0/false", s.Holdings[1].MarketValue.FormatINR(), s.Holdings[1].PnL.FormatINR(), s.Holdings[1].Priced)
	}
	// totals: invested 5000+60000=65000, value 6000+60000=66000, pnl 1000
	if s.Invested != domain.ToPaise(65000) || s.MarketValue != domain.ToPaise(66000) || s.PnL != domain.ToPaise(1000) {
		t.Errorf("totals = inv %s val %s pnl %s", s.Invested.FormatINR(), s.MarketValue.FormatINR(), s.PnL.FormatINR())
	}
	// allocation sorted desc by value: Gold (60000) before Mutual Fund (6000)
	if len(s.Allocation) != 2 || s.Allocation[0].Type != domain.GoldAsset {
		t.Errorf("allocation order wrong: %+v", s.Allocation)
	}
}
