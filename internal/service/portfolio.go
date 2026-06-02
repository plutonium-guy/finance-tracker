package service

import "finance-tracker/internal/domain"

// HoldingValue decorates a holding with computed valuation figures.
type HoldingValue struct {
	Holding     domain.Holding
	Invested    domain.Money
	MarketValue domain.Money
	PnL         domain.Money
	ReturnPct   float64
	Priced      bool // true if a live/last price is known (else valued at cost)
}

// holdingMoney computes units(micro) × pricePerUnit(paise) / 1e6 → paise.
func holdingMoney(unitsMicro int64, perUnit domain.Money) domain.Money {
	return domain.Money(unitsMicro * int64(perUnit) / domain.UnitScale)
}

// ValueHolding computes invested, market value, P&L and return for a holding.
// When no last price is known it is valued at cost (PnL 0).
func ValueHolding(h domain.Holding) HoldingValue {
	hv := HoldingValue{Holding: h}
	hv.Invested = holdingMoney(h.UnitsMicro, h.AvgCost)
	price := h.LastPrice
	if price > 0 {
		hv.Priced = true
	} else {
		price = h.AvgCost
	}
	hv.MarketValue = holdingMoney(h.UnitsMicro, price)
	hv.PnL = hv.MarketValue - hv.Invested
	hv.ReturnPct = ratio(hv.PnL, hv.Invested)
	return hv
}

// PortfolioSummary aggregates a set of holdings.
type PortfolioSummary struct {
	Holdings    []HoldingValue
	Invested    domain.Money
	MarketValue domain.Money
	PnL         domain.Money
	ReturnPct   float64
	Allocation  []AllocationSlice // by asset type, descending by value
}

// AllocationSlice is one asset-type wedge of the portfolio.
type AllocationSlice struct {
	Type  domain.AssetType
	Value domain.Money
	Pct   float64
}

// SummarizePortfolio values every holding and rolls up totals + allocation.
func SummarizePortfolio(holdings []domain.Holding) PortfolioSummary {
	var ps PortfolioSummary
	byType := map[domain.AssetType]domain.Money{}
	var order []domain.AssetType
	for _, h := range holdings {
		hv := ValueHolding(h)
		ps.Holdings = append(ps.Holdings, hv)
		ps.Invested += hv.Invested
		ps.MarketValue += hv.MarketValue
		if _, ok := byType[h.Type]; !ok {
			order = append(order, h.Type)
		}
		byType[h.Type] += hv.MarketValue
	}
	ps.PnL = ps.MarketValue - ps.Invested
	ps.ReturnPct = ratio(ps.PnL, ps.Invested)
	for _, ty := range order {
		v := byType[ty]
		ps.Allocation = append(ps.Allocation, AllocationSlice{Type: ty, Value: v, Pct: ratio(v, ps.MarketValue)})
	}
	// sort allocation descending by value (simple insertion; few slices)
	for i := 1; i < len(ps.Allocation); i++ {
		for j := i; j > 0 && ps.Allocation[j].Value > ps.Allocation[j-1].Value; j-- {
			ps.Allocation[j], ps.Allocation[j-1] = ps.Allocation[j-1], ps.Allocation[j]
		}
	}
	return ps
}
