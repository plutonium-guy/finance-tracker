package service

import (
	"strings"

	"finance-tracker/internal/domain"
)

// Reserved tags that drive reimbursement tracking.
const (
	TagReimbursable = "reimbursable"
	TagSettled      = "settled"
)

// ReimbItem is a reimbursable expense and whether it's been settled.
type ReimbItem struct {
	Tx      domain.Transaction
	Settled bool
}

// Reimbursements summarizes expenses tagged "reimbursable".
type Reimbursements struct {
	Outstanding  domain.Money // unsettled reimbursable expenses
	SettledTotal domain.Money
	Items        []ReimbItem
}

// ComputeReimbursements scans transactions tagged "reimbursable" (an item also
// tagged "settled" counts as paid back). tags maps txID → its tag list.
func ComputeReimbursements(txs []domain.Transaction, tags map[string][]string) Reimbursements {
	var r Reimbursements
	for _, t := range txs {
		if t.Type != domain.Expense || !hasTag(tags[t.ID], TagReimbursable) {
			continue
		}
		settled := hasTag(tags[t.ID], TagSettled)
		r.Items = append(r.Items, ReimbItem{Tx: t, Settled: settled})
		if settled {
			r.SettledTotal += t.Amount
		} else {
			r.Outstanding += t.Amount
		}
	}
	return r
}

func hasTag(tags []string, name string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, name) {
			return true
		}
	}
	return false
}
