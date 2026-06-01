package telegrambot

import (
	"context"
	"strings"
	"testing"
	"time"

	"finance-tracker/internal/service"
	"finance-tracker/internal/store"
)

type fakeSender struct {
	msgs []string
	chat int64
}

func (f *fakeSender) SendMessage(_ context.Context, chatID int64, text string) error {
	f.chat = chatID
	f.msgs = append(f.msgs, text)
	return nil
}

func newTestBot(t *testing.T, allowedChat int64) (*Bot, *store.SQLite, *fakeSender) {
	t.Helper()
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
	svc := service.New(db, func() time.Time { return fixedNow })
	fs := &fakeSender{}
	return NewBot(svc, fs, allowedChat, ""), db, fs
}

func msg(chat int64, text string) *Message {
	return &Message{Chat: Chat{ID: chat}, Text: text, From: &User{ID: chat}}
}

func TestHandleAddCreatesTransaction(t *testing.T) {
	bot, db, fs := newTestBot(t, 0)
	bot.HandleMessage(context.Background(), msg(42, "250.50 Coffee cat:food method:cc"))

	rows, _ := db.AllTransactions()
	if len(rows) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(rows))
	}
	tx := rows[0]
	if tx.Description != "Coffee" || tx.Amount.FormatINR() != "₹250.50" {
		t.Errorf("transaction wrong: %+v", tx)
	}
	if tx.Category != "Food & Dining" { // "food" resolves to the real category
		t.Errorf("category = %q, want Food & Dining", tx.Category)
	}
	if string(tx.PaymentMethod) != "Credit Card" {
		t.Errorf("method = %q, want Credit Card", tx.PaymentMethod)
	}
	if len(fs.msgs) != 1 || !strings.Contains(fs.msgs[0], "✅") || !strings.Contains(fs.msgs[0], "₹250.50") {
		t.Errorf("reply wrong: %v", fs.msgs)
	}
}

func TestHandleDefaultsCategoryToMiscellaneous(t *testing.T) {
	bot, db, _ := newTestBot(t, 0)
	bot.HandleMessage(context.Background(), msg(1, "75 Parking"))
	rows, _ := db.AllTransactions()
	if len(rows) != 1 || rows[0].Category != "Miscellaneous" {
		t.Errorf("default category wrong: %+v", rows)
	}
}

func TestHandleUnknownCategoryReplies(t *testing.T) {
	bot, db, fs := newTestBot(t, 0)
	bot.HandleMessage(context.Background(), msg(1, "75 X cat:zzznope"))
	if rows, _ := db.AllTransactions(); len(rows) != 0 {
		t.Errorf("should not have created a transaction for unknown category")
	}
	if len(fs.msgs) == 0 || !strings.Contains(fs.msgs[0], "unknown category") {
		t.Errorf("expected unknown-category reply, got %v", fs.msgs)
	}
}

func TestHandleParseErrorReplies(t *testing.T) {
	bot, db, fs := newTestBot(t, 0)
	bot.HandleMessage(context.Background(), msg(1, "notanumber here"))
	if rows, _ := db.AllTransactions(); len(rows) != 0 {
		t.Error("parse error should not create a transaction")
	}
	if len(fs.msgs) == 0 || !strings.Contains(fs.msgs[0], "amount") {
		t.Errorf("expected amount-hint reply, got %v", fs.msgs)
	}
}

func TestHandleHelpAndBalance(t *testing.T) {
	bot, _, fs := newTestBot(t, 0)
	bot.HandleMessage(context.Background(), msg(1, "/help"))
	bot.HandleMessage(context.Background(), msg(1, "/balance"))
	if len(fs.msgs) != 2 {
		t.Fatalf("expected 2 replies, got %d", len(fs.msgs))
	}
	if !strings.Contains(fs.msgs[0], "Add a transaction") {
		t.Errorf("help text wrong: %s", fs.msgs[0])
	}
	if !strings.Contains(fs.msgs[1], "Credits") {
		t.Errorf("balance text wrong: %s", fs.msgs[1])
	}
}

func TestAllowedChatRestriction(t *testing.T) {
	bot, db, fs := newTestBot(t, 99) // only chat 99 may add
	bot.HandleMessage(context.Background(), msg(7, "250 Coffee"))
	if rows, _ := db.AllTransactions(); len(rows) != 0 {
		t.Error("message from unauthorized chat should be ignored")
	}
	if len(fs.msgs) != 0 {
		t.Errorf("should not reply to unauthorized chat, got %v", fs.msgs)
	}
	bot.HandleMessage(context.Background(), msg(99, "250 Coffee"))
	if rows, _ := db.AllTransactions(); len(rows) != 1 {
		t.Error("authorized chat should be able to add")
	}
}
