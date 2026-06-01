package web

import (
	"net/http"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

type dashboardVM struct {
	base
	Month     string
	KPIs      service.KPIs
	PayCycle  service.PayCycleKPIs
	ChartJSON any // ChartData struct; html/template JSON-encodes it in the island
	Recent    []domain.Transaction
	Top       []domain.Transaction
	Goals     []service.GoalProgress
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	month := h.currentMonthOr(r)
	txs, err := h.svc.Store.AllTransactions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	rec, _ := h.svc.Store.ListRecurring()
	vm := dashboardVM{
		base:      h.base("Dashboard", "dashboard"),
		Month:     month,
		KPIs:      h.svc.KPIsFor(txs, month),
		PayCycle:  h.svc.PayCycleFor(txs, rec, month),
		ChartJSON: h.svc.ChartDataFor(txs),
		Recent:    service.Recent(txs, 10),
		Top:       service.TopExpenses(txs, 5),
		Goals:     h.goalProgress(),
	}
	h.rdr.Page(w, "dashboard", vm)
}

func (h *Handler) PartialKPIs(w http.ResponseWriter, r *http.Request) {
	month := h.currentMonthOr(r)
	txs, _ := h.svc.Store.AllTransactions()
	h.rdr.Fragment(w, "kpi_strip", dashboardVM{Month: month, KPIs: h.svc.KPIsFor(txs, month)})
}

func (h *Handler) PartialPayCycle(w http.ResponseWriter, r *http.Request) {
	month := h.currentMonthOr(r)
	txs, _ := h.svc.Store.AllTransactions()
	rec, _ := h.svc.Store.ListRecurring()
	h.rdr.Fragment(w, "paycycle_strip", dashboardVM{Month: month, PayCycle: h.svc.PayCycleFor(txs, rec, month)})
}

func (h *Handler) PartialCharts(w http.ResponseWriter, r *http.Request) {
	month := h.currentMonthOr(r)
	txs, _ := h.svc.Store.AllTransactions()
	h.rdr.Fragment(w, "charts", dashboardVM{Month: month, ChartJSON: h.svc.ChartDataFor(txs)})
}

func (h *Handler) PartialRecent(w http.ResponseWriter, r *http.Request) {
	txs, _ := h.svc.Store.AllTransactions()
	h.rdr.Fragment(w, "recent_activity", dashboardVM{
		Recent: service.Recent(txs, 10),
		Top:    service.TopExpenses(txs, 5),
	})
}
