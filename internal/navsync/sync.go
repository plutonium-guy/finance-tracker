package navsync

import (
	"context"
	"sync"
	"time"

	"finance-tracker/internal/domain"
)

// DB is the slice of the store the NAV syncer needs.
type DB interface {
	ListHoldings() ([]domain.Holding, error)
	SetHoldingPriceBySchemeCode(schemeCode string, pricePaise domain.Money, at string) (int, error)
}

// Syncer updates holdings' last price from AMFI NAVs.
type Syncer struct {
	fetcher Fetcher
	db      DB
	now     func() time.Time
	mu      sync.Mutex // serialize runs (scheduled vs manual trigger)
}

// Result summarizes a NAV sync run.
type Result struct {
	Fetched int `json:"fetched"` // scheme codes in the AMFI file
	Updated int `json:"updated"` // holdings whose price changed
}

func NewSyncer(fetcher Fetcher, db DB, now func() time.Time) *Syncer {
	if now == nil {
		now = time.Now
	}
	return &Syncer{fetcher: fetcher, db: db, now: now}
}

// Run fetches the latest NAVs and updates every holding that carries a matching
// scheme code. Only scheme codes actually held are touched.
func (s *Syncer) Run(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	holdings, err := s.db.ListHoldings()
	if err != nil {
		return Result{}, err
	}
	// Collect the distinct scheme codes we actually hold.
	want := map[string]bool{}
	for _, h := range holdings {
		if h.SchemeCode != "" {
			want[h.SchemeCode] = true
		}
	}
	if len(want) == 0 {
		return Result{}, nil
	}

	navs, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return Result{}, err
	}
	at := s.now().UTC().Format(time.RFC3339)
	res := Result{Fetched: len(navs)}
	for code := range want {
		nav, ok := navs[code]
		if !ok {
			continue
		}
		n, err := s.db.SetHoldingPriceBySchemeCode(code, domain.ToPaise(nav), at)
		if err != nil {
			continue
		}
		res.Updated += n
	}
	return res, nil
}
