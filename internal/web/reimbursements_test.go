package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestReimbursementsFlow(t *testing.T) {
	h, db := newTestServer(t)

	// A reimbursable expense.
	if rec := postForm(t, h, "/transactions", url.Values{
		"date": {"2026-06-10"}, "amount": {"1200"}, "description": {"Client lunch"},
		"type": {"Expense"}, "payment_method": {"Credit Card"}, "category": {"Miscellaneous"},
		"tags": {"reimbursable"},
	}); rec.Code != 200 {
		t.Fatalf("create tx: status %d", rec.Code)
	}
	var id string
	rows, _ := db.AllTransactions()
	for _, tx := range rows {
		if tx.Description == "Client lunch" {
			id = tx.ID
		}
	}
	if id == "" {
		t.Fatal("transaction not found")
	}

	// Shows as outstanding ₹1,200.
	body := get(t, h, "/reimbursements/list").Body.String()
	if !strings.Contains(body, "₹1,200.00") || !strings.Contains(body, "Outstanding") {
		t.Fatalf("reimb list missing outstanding ₹1,200: %s", body)
	}

	// Settle it.
	rec := postForm(t, h, "/reimbursements/"+id+"/settle", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Settled") {
		t.Fatalf("settle: status %d, settled=%v", rec.Code, strings.Contains(rec.Body.String(), "Settled"))
	}
	tags, _ := db.TagsFor(id)
	found := false
	for _, tg := range tags {
		if tg == "settled" {
			found = true
		}
	}
	if !found {
		t.Errorf("settled tag not applied; tags=%v", tags)
	}
}
