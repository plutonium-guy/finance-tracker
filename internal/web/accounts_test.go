package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestAccountsPageAndLinking(t *testing.T) {
	h, db := newTestServer(t)

	// Create an account with an opening balance.
	rec := postForm(t, h, "/accounts", url.Values{
		"name": {"HDFC Savings"}, "type": {"Bank"}, "opening": {"100000"},
	})
	if rec.Code != 200 {
		t.Fatalf("create account: status %d", rec.Code)
	}
	accts, _ := db.ListAccounts()
	if len(accts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accts))
	}
	acctID := accts[0].ID

	// Page renders and shows the net-worth strip.
	if rec := get(t, h, "/accounts"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Net worth") {
		t.Fatalf("/accounts: status %d, has net worth=%v", rec.Code, strings.Contains(rec.Body.String(), "Net worth"))
	}

	// An income transaction linked to the account raises its balance.
	rec = postForm(t, h, "/transactions", url.Values{
		"date": {"2026-06-10"}, "amount": {"5000"}, "description": {"Refund"},
		"type": {"Income"}, "payment_method": {"Bank Transfer"}, "category": {"Miscellaneous"},
		"account_id": {acctID},
	})
	if rec.Code != 200 {
		t.Fatalf("create tx: status %d", rec.Code)
	}

	// The accounts fragment shows the updated balance + net worth (opening
	// 100000 + income 5000 = 105000).
	body := get(t, h, "/accounts/list").Body.String()
	if !strings.Contains(body, "₹1,05,000.00") {
		t.Errorf("/accounts/list missing ₹1,05,000.00 balance; body:\n%s", body)
	}
}
