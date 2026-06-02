package navsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/store"
)

const sampleNAVAll = `Scheme Code;ISIN Div Payout/ ISIN Growth;ISIN Div Reinvestment;Scheme Name;Net Asset Value;Date

Some AMC Mutual Fund
122639;INF879O01027;-;Parag Parikh Flexi Cap Fund - Regular Plan - Growth;82.4567;06-Jun-2026
120716;INF209K01VD7;-;Some Other Fund - Growth;45.10;06-Jun-2026
`

func TestParseNAVAll(t *testing.T) {
	m, err := ParseNAVAll(strings.NewReader(sampleNAVAll))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("parsed %d navs, want 2: %v", len(m), m)
	}
	if m["122639"] != 82.4567 {
		t.Errorf("nav[122639] = %v, want 82.4567", m["122639"])
	}
}

type fakeFetcher struct{ navs map[string]float64 }

func (f fakeFetcher) Fetch(_ context.Context) (map[string]float64, error) { return f.navs, nil }

func TestSyncerUpdatesHoldingByScheme(t *testing.T) {
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, fn := range []func() error{db.Migrate, db.EnsureSettings, db.SeedDefaultCategories} {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.CreateHolding(domain.Holding{
		ID: "h1", Name: "Flexi Cap", Type: domain.MutualFund, SchemeCode: "122639",
		UnitsMicro: domain.UnitsToMicro(100), AvgCost: domain.ToPaise(50), CreatedAt: "x", UpdatedAt: "x",
	})
	// A holding without a matching scheme code should be left alone.
	_ = db.CreateHolding(domain.Holding{
		ID: "h2", Name: "Gold", Type: domain.GoldAsset, UnitsMicro: domain.UnitsToMicro(5), AvgCost: domain.ToPaise(6000), CreatedAt: "x", UpdatedAt: "x",
	})

	s := NewSyncer(fakeFetcher{map[string]float64{"122639": 82.4567, "999999": 10}}, db,
		func() time.Time { return time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC) })
	res, err := s.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated %d, want 1", res.Updated)
	}
	h1, _ := db.GetHolding("h1")
	if h1.LastPrice != domain.ToPaise(82.4567) {
		t.Errorf("h1 last price = %s, want ₹82.46", h1.LastPrice.FormatINR())
	}
	h2, _ := db.GetHolding("h2")
	if h2.LastPrice != 0 {
		t.Errorf("h2 last price = %s, want unchanged (0)", h2.LastPrice.FormatINR())
	}
}
