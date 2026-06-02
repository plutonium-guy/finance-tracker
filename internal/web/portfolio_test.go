package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestPortfolioPageAndNetWorth(t *testing.T) {
	h, db := newTestServer(t)

	rec := postForm(t, h, "/portfolio", url.Values{
		"name": {"Flexi Cap"}, "type": {"Mutual Fund"}, "scheme_code": {"122639"},
		"units": {"100"}, "avg_cost": {"50"}, "last_price": {"60"},
	})
	if rec.Code != 200 {
		t.Fatalf("create holding: status %d", rec.Code)
	}
	if hs, _ := db.ListHoldings(); len(hs) != 1 {
		t.Fatalf("got %d holdings, want 1", len(hs))
	}

	// Portfolio page shows current value (100 × ₹60 = ₹6,000).
	body := get(t, h, "/portfolio").Body.String()
	if !strings.Contains(body, "₹6,000.00") {
		t.Errorf("/portfolio missing ₹6,000.00 value")
	}

	// Net worth picks up the investment value.
	nwBody := get(t, h, "/accounts/list").Body.String()
	if !strings.Contains(nwBody, "₹6,000.00") {
		t.Errorf("/accounts/list net worth missing investment value ₹6,000.00")
	}
}
