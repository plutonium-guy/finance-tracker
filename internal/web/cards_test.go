package web

import (
	"net/url"
	"strings"
	"testing"

	"finance-tracker/internal/domain"
)

func TestCardsPageAndMarkPaid(t *testing.T) {
	h, db := newTestServer(t)

	// Create a card (statement day 5; clock is 2026-06-15 so the May-05..Jun-05
	// statement has just closed).
	rec := postForm(t, h, "/cards", url.Values{
		"name": {"Test Card"}, "last4": {"1234"}, "limit": {"100000"},
		"statement_day": {"5"}, "due_offset_days": {"18"},
	})
	if rec.Code != 200 {
		t.Fatalf("create card: status %d", rec.Code)
	}
	cards, _ := db.ListCards()
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards))
	}
	cardID := cards[0].ID

	if rec := get(t, h, "/cards"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Test Card") {
		t.Fatalf("/cards page: status %d, contains card name=%v", rec.Code, strings.Contains(rec.Body.String(), "Test Card"))
	}

	// A credit-card purchase in the closed statement cycle, linked to the card.
	rec = postForm(t, h, "/transactions", url.Values{
		"date": {"2026-05-20"}, "amount": {"6200"}, "description": {"Shopping"},
		"type": {"Expense"}, "payment_method": {"Credit Card"}, "category": {"Miscellaneous"},
		"card_id": {cardID},
	})
	if rec.Code != 200 {
		t.Fatalf("create tx: status %d", rec.Code)
	}

	transfers := func() int {
		txs, _ := db.AllTransactions()
		n := 0
		for _, tx := range txs {
			if tx.Type == domain.Transfer {
				n++
			}
		}
		return n
	}
	if transfers() != 0 {
		t.Fatalf("expected 0 transfers before payment, got %d", transfers())
	}

	// Mark the statement paid: creates exactly one Transfer + one payment row.
	if rec := postForm(t, h, "/cards/"+cardID+"/pay", url.Values{}); rec.Code != 200 {
		t.Fatalf("pay: status %d", rec.Code)
	}
	if transfers() != 1 {
		t.Fatalf("expected 1 transfer after payment, got %d", transfers())
	}
	pays, _ := db.ListStatementPayments(cardID)
	if len(pays) != 1 || pays[0].Amount != domain.ToPaise(6200) {
		t.Fatalf("payments = %+v, want one of ₹6200", pays)
	}

	// Paying again is idempotent — no second transfer, no second payment row.
	postForm(t, h, "/cards/"+cardID+"/pay", url.Values{})
	if transfers() != 1 {
		t.Errorf("expected payment to be idempotent, got %d transfers", transfers())
	}
	if pays, _ := db.ListStatementPayments(cardID); len(pays) != 1 {
		t.Errorf("expected 1 payment row after double-pay, got %d", len(pays))
	}
}
