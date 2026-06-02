package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"finance-tracker/internal/service"
)

type reimbVM struct {
	base
	Reimb service.Reimbursements
}

func (h *Handler) reimbursements() service.Reimbursements {
	txs, _ := h.svc.Store.AllTransactions()
	ids := make([]string, len(txs))
	for i, t := range txs {
		ids[i] = t.ID
	}
	tags, _ := h.svc.Store.TagsForMany(ids)
	return service.ComputeReimbursements(txs, tags)
}

func (h *Handler) ReimbursementsPage(w http.ResponseWriter, r *http.Request) {
	h.rdr.Page(w, "reimbursements", reimbVM{base: h.base("Reimbursements", "reimbursements"), Reimb: h.reimbursements()})
}

func (h *Handler) ReimbursementsList(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "reimb_list", reimbVM{Reimb: h.reimbursements()})
}

// ReimbursementToggle flips the "settled" tag on a reimbursable transaction.
func (h *Handler) ReimbursementToggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tags, _ := h.svc.Store.TagsFor(id)
	has := false
	var next []string
	for _, t := range tags {
		if t == service.TagSettled {
			has = true
			continue // drop it (toggle off)
		}
		next = append(next, t)
	}
	if !has {
		next = append(next, service.TagSettled)
	}
	h.svc.Store.SetTransactionTags(id, next)
	txTrigger(w)
	h.ReimbursementsList(w, r)
}
