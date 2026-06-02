package service

import (
	"testing"

	"finance-tracker/internal/domain"
)

func TestAccountBalancesAndNetWorth(t *testing.T) {
	accounts := []domain.Account{
		{ID: "bank", Name: "HDFC", Type: domain.BankAccount, OpeningBalance: domain.ToPaise(100000)},
		{ID: "cash", Name: "Cash", Type: domain.CashAccount, OpeningBalance: domain.ToPaise(5000)},
	}
	txs := []domain.Transaction{
		{ID: "t1", Type: domain.Income, Amount: domain.ToPaise(80000)},  // -> bank
		{ID: "t2", Type: domain.Expense, Amount: domain.ToPaise(12000)}, // -> bank
		{ID: "t3", Type: domain.Expense, Amount: domain.ToPaise(2000)},  // -> cash
		{ID: "t4", Type: domain.Expense, Amount: domain.ToPaise(9999)},  // unlinked -> ignored
		{ID: "t5", Type: domain.Transfer, Amount: domain.ToPaise(5000)}, // -> bank, net-neutral
	}
	txAcct := map[string]string{"t1": "bank", "t2": "bank", "t3": "cash", "t5": "bank"}

	bals := AccountBalances(accounts, txs, txAcct)
	got := map[string]domain.Money{}
	for _, b := range bals {
		got[b.Account.ID] = b.Balance
	}
	// bank: 100000 + 80000 - 12000 + 0(transfer) = 168000
	if got["bank"] != domain.ToPaise(168000) {
		t.Errorf("bank balance = %s, want ₹168000", got["bank"].FormatINR())
	}
	// cash: 5000 - 2000 = 3000
	if got["cash"] != domain.ToPaise(3000) {
		t.Errorf("cash balance = %s, want ₹3000", got["cash"].FormatINR())
	}

	cards := []CardSummary{{Outstanding: domain.ToPaise(20000)}}
	nw := ComputeNetWorth(bals, domain.ToPaise(50000), cards)
	// accounts 171000 + investments 50000 - card 20000 = 201000
	if nw.Accounts != domain.ToPaise(171000) {
		t.Errorf("net worth accounts = %s, want ₹171000", nw.Accounts.FormatINR())
	}
	if nw.Total != domain.ToPaise(201000) {
		t.Errorf("net worth total = %s, want ₹201000", nw.Total.FormatINR())
	}
}
