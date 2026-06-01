package store

import (
	"testing"

	"finance-tracker/internal/domain"
)

func newTestStore(t *testing.T) *SQLite {
	t.Helper()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSettings(); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDefaultCategories(); err != nil {
		t.Fatal(err)
	}
	return s
}

func sampleTx(id, date, desc string, amt domain.Money, typ domain.TransactionType, cat string) domain.Transaction {
	return domain.Transaction{
		ID: id, Date: date, Description: desc, Amount: amt, Type: typ,
		PaymentMethod: domain.CreditCard, Category: cat,
		CreatedAt: "2026-06-01T00:00:00Z", UpdatedAt: "2026-06-01T00:00:00Z",
	}
}

func TestTransactionCRUD(t *testing.T) {
	s := newTestStore(t)
	tx := sampleTx("t1", "2026-05-28", "ixigo", 852600, domain.Expense, "Travel")
	if err := s.CreateTransaction(tx); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTransaction("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != 852600 || got.Description != "ixigo" {
		t.Errorf("got %+v", got)
	}
	got.Amount = 900000
	if err := s.UpdateTransaction(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetTransaction("t1")
	if got2.Amount != 900000 {
		t.Errorf("update failed: %d", got2.Amount)
	}
	if err := s.DeleteTransaction("t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTransaction("t1"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAmountCheckConstraint(t *testing.T) {
	s := newTestStore(t)
	tx := sampleTx("bad", "2026-05-01", "neg", -100, domain.Expense, "Travel")
	if err := s.CreateTransaction(tx); err == nil {
		t.Error("expected CHECK (amount_paise > 0) to reject negative amount")
	}
}

func TestForeignKeyCategory(t *testing.T) {
	s := newTestStore(t)
	tx := sampleTx("fk", "2026-05-01", "x", 100, domain.Expense, "NoSuchCategory")
	if err := s.CreateTransaction(tx); err == nil {
		t.Error("expected FK violation for unknown category")
	}
}

func TestTransactionsFilterAndPaging(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateTransaction(sampleTx("a", "2026-05-28", "ixigo", 852600, domain.Expense, "Travel"))
	_ = s.CreateTransaction(sampleTx("b", "2026-05-22", "kundan", 95000, domain.Expense, "Miscellaneous"))
	_ = s.CreateTransaction(sampleTx("c", "2026-06-01", "groceter", 3100, domain.Expense, "Groceries"))

	rows, total, err := s.Transactions(TxFilter{Month: "2026-05"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 2 {
		t.Errorf("month filter: total=%d len=%d, want 2,2", total, len(rows))
	}

	rows, _, _ = s.Transactions(TxFilter{Category: "Travel"})
	if len(rows) != 1 || rows[0].ID != "a" {
		t.Errorf("category filter wrong: %+v", rows)
	}

	rows, _, _ = s.Transactions(TxFilter{Query: "grocet"})
	if len(rows) != 1 || rows[0].ID != "c" {
		t.Errorf("query filter wrong: %+v", rows)
	}

	rows, total, _ = s.Transactions(TxFilter{PageSize: 2, Page: 1})
	if total != 3 || len(rows) != 2 {
		t.Errorf("paging: total=%d len=%d, want 3,2", total, len(rows))
	}
}

func TestRenameCategoryCascades(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateTransaction(sampleTx("a", "2026-05-28", "ixigo", 852600, domain.Expense, "Travel"))
	if err := s.RenameCategory("Travel", "Trips"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTransaction("a")
	if got.Category != "Trips" {
		t.Errorf("cascade rename failed: category=%q", got.Category)
	}
}

func TestDeleteCategoryInUse(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateTransaction(sampleTx("a", "2026-05-28", "ixigo", 852600, domain.Expense, "Travel"))
	if err := s.DeleteCategory("Travel", ""); err != ErrCategoryInUse {
		t.Errorf("expected ErrCategoryInUse, got %v", err)
	}
	// With reassignment it should succeed and move rows.
	if err := s.DeleteCategory("Travel", "Miscellaneous"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTransaction("a")
	if got.Category != "Miscellaneous" {
		t.Errorf("reassign failed: %q", got.Category)
	}
}

func TestRecurringToggle(t *testing.T) {
	s := newTestStore(t)
	r := domain.RecurringItem{
		ID: "r1", Name: "Netflix", Category: "Entertainment", Amount: 64900,
		Frequency: domain.Monthly, Type: domain.Expense, StartDate: "2026-01-01",
		PaymentMethod: domain.CreditCard, Active: true,
		CreatedAt: "x", UpdatedAt: "x",
	}
	if err := s.CreateRecurring(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.ToggleRecurring("r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Active {
		t.Error("toggle should have set active=false")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.GetSettings()
	if a.Currency != "₹" || a.FiscalYearStart != "April" {
		t.Errorf("defaults wrong: %+v", a)
	}
	a.DarkMode = true
	a.FiscalYearStart = "January"
	if err := s.SaveSettings(a); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetSettings()
	if !b.DarkMode || b.FiscalYearStart != "January" {
		t.Errorf("save/load failed: %+v", b)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateTransaction(sampleTx("a", "2026-05-28", "ixigo", 852600, domain.Expense, "Travel"))
	exp, err := s.Export()
	if err != nil {
		t.Fatal(err)
	}
	// Wipe then import back.
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.AllTransactions(); len(rows) != 0 {
		t.Fatalf("reset left %d transactions", len(rows))
	}
	if err := s.Import(exp); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.AllTransactions()
	if len(rows) != 1 || rows[0].ID != "a" {
		t.Errorf("import round-trip failed: %+v", rows)
	}
}

func TestCardCRUDAndLinking(t *testing.T) {
	s := newTestStore(t)
	card := domain.Card{ID: "c1", Name: "ICICI", Last4: "4003", Limit: 103000000, StatementDay: 18, DueOffsetDays: 18, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := s.CreateCard(card); err != nil {
		t.Fatal(err)
	}
	if id, ok, _ := s.CardIDByLast4("4003"); !ok || id != "c1" {
		t.Errorf("CardIDByLast4 = %q,%v want c1,true", id, ok)
	}
	if _, ok, _ := s.CardIDByLast4("9999"); ok {
		t.Error("CardIDByLast4 matched a non-existent last4")
	}

	_ = s.CreateTransaction(sampleTx("t1", "2026-05-20", "Shopping", 620000, domain.Expense, "Travel"))
	if err := s.SetTransactionCard("t1", "c1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CardOfTransaction("t1"); got != "c1" {
		t.Errorf("CardOfTransaction = %q, want c1", got)
	}
	if m, _ := s.TransactionCardMap(); m["t1"] != "c1" {
		t.Errorf("TransactionCardMap[t1] = %q, want c1", m["t1"])
	}

	// Statement payment is idempotent per (card, period_end).
	pay := domain.StatementPayment{CardID: "c1", PeriodEnd: "2026-05-18", Amount: 620000, TxID: "pay-tx", PaidAt: "2026-06-01T00:00:00Z"}
	if ok, _ := s.RecordStatementPayment(pay); !ok {
		t.Error("first RecordStatementPayment should report inserted=true")
	}
	if ok, _ := s.RecordStatementPayment(pay); ok {
		t.Error("second RecordStatementPayment should report inserted=false")
	}

	// Deleting the card cascades the link and payment away.
	if err := s.DeleteCard("c1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CardOfTransaction("t1"); got != "" {
		t.Errorf("link survived card delete: %q", got)
	}
	if pays, _ := s.ListStatementPayments("c1"); len(pays) != 0 {
		t.Errorf("payments survived card delete: %d", len(pays))
	}
}

func TestExportImportIncludesCards(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateCard(domain.Card{ID: "c1", Name: "ICICI", Last4: "4003", Limit: 103000000, StatementDay: 18, DueOffsetDays: 18, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"})
	_ = s.CreateTransaction(sampleTx("t1", "2026-05-20", "Shopping", 620000, domain.Expense, "Travel"))
	_ = s.SetTransactionCard("t1", "c1")
	_, _ = s.RecordStatementPayment(domain.StatementPayment{CardID: "c1", PeriodEnd: "2026-05-18", Amount: 620000, TxID: "p", PaidAt: "2026-06-01T00:00:00Z"})

	exp, err := s.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Cards) != 1 || len(exp.StatementPayments) != 1 || exp.TransactionCards["t1"] != "c1" {
		t.Fatalf("export missing card data: cards=%d pays=%d links=%v", len(exp.Cards), len(exp.StatementPayments), exp.TransactionCards)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if cards, _ := s.ListCards(); len(cards) != 0 {
		t.Fatalf("reset left %d cards", len(cards))
	}
	if err := s.Import(exp); err != nil {
		t.Fatal(err)
	}
	if cards, _ := s.ListCards(); len(cards) != 1 {
		t.Errorf("import restored %d cards, want 1", len(cards))
	}
	if got, _ := s.CardOfTransaction("t1"); got != "c1" {
		t.Errorf("import restored link = %q, want c1", got)
	}
	if pays, _ := s.ListStatementPayments("c1"); len(pays) != 1 {
		t.Errorf("import restored %d payments, want 1", len(pays))
	}
}

func TestIsEmpty(t *testing.T) {
	s := newTestStore(t)
	empty, _ := s.IsEmpty()
	if !empty {
		t.Error("fresh store should be empty")
	}
	_ = s.CreateTransaction(sampleTx("a", "2026-05-28", "x", 100, domain.Expense, "Travel"))
	empty, _ = s.IsEmpty()
	if empty {
		t.Error("store with a transaction should not be empty")
	}
}
