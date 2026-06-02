package service

import "finance-tracker/internal/domain"

// AccountBalance is an account with its computed current balance.
type AccountBalance struct {
	Account domain.Account
	Balance domain.Money // opening + Σ signed amounts of linked transactions
}

// AccountBalances computes each account's balance from its opening balance plus
// the signed amounts of transactions linked to it. (Transfers are net-neutral,
// per SignedAmount, so they don't move a single-linked account's balance.)
func AccountBalances(accounts []domain.Account, txs []domain.Transaction, txAcct map[string]string) []AccountBalance {
	delta := map[string]domain.Money{}
	for _, t := range txs {
		if acct := txAcct[t.ID]; acct != "" {
			delta[acct] += t.SignedAmount()
		}
	}
	out := make([]AccountBalance, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, AccountBalance{Account: a, Balance: a.OpeningBalance + delta[a.ID]})
	}
	return out
}

// NetWorth summarizes overall position.
type NetWorth struct {
	Accounts       domain.Money `json:"accounts"`        // Σ account balances
	Investments    domain.Money `json:"investments"`     // Σ portfolio market value
	CardOutstanding domain.Money `json:"card_outstanding"` // Σ unpaid card balances
	Total          domain.Money `json:"total"`           // accounts + investments − card outstanding
}

// ComputeNetWorth combines account balances, investment value, and card debt.
func ComputeNetWorth(balances []AccountBalance, investments domain.Money, cards []CardSummary) NetWorth {
	var nw NetWorth
	for _, b := range balances {
		nw.Accounts += b.Balance
	}
	nw.Investments = investments
	for _, c := range cards {
		if c.Outstanding > 0 {
			nw.CardOutstanding += c.Outstanding
		}
	}
	nw.Total = nw.Accounts + nw.Investments - nw.CardOutstanding
	return nw
}
