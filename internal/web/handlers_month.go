package web

import (
	"net/http"
	"time"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
	"finance-tracker/internal/store"
)

type monthOpt struct {
	Key   string
	Label string
}

type monthDetailVM struct {
	Month          string
	KPIs           service.KPIs
	MonthChartJSON any // doughnut struct; html/template JSON-encodes it in the island
	Categories     []service.CategoryStat
	Txs            []domain.Transaction
	Insights       service.MonthInsights
}

type monthPageVM struct {
	base
	Months []monthOpt
	monthDetailVM
}

// addMonthKey returns the month key n months from key ("YYYY-MM").
func addMonthKey(key string, n int) string {
	t, err := time.Parse("2006-01", key)
	if err != nil {
		return key
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}

func (h *Handler) monthOptions() []monthOpt {
	current := h.svc.CurrentMonth()
	var opts []monthOpt
	for i := -service.MonthsBack; i <= service.MonthsForward; i++ {
		key := addMonthKey(current, i)
		opts = append(opts, monthOpt{Key: key, Label: formatMonth(key)})
	}
	return opts
}

func (h *Handler) buildMonthDetail(month string) monthDetailVM {
	allTx, _ := h.svc.Store.AllTransactions()
	monthTxs, _, _ := h.svc.Store.Transactions(store.TxFilter{Month: month})
	cats := service.CategoryAggregates(monthTxs)

	type doughnut struct {
		CategoryLabels []string  `json:"categoryLabels"`
		CategoryValues []float64 `json:"categoryValues"`
	}
	var d doughnut
	for _, c := range cats {
		d.CategoryLabels = append(d.CategoryLabels, c.Category)
		d.CategoryValues = append(d.CategoryValues, c.TotalSpent.Rupees())
	}
	return monthDetailVM{
		Month:          month,
		KPIs:           h.svc.KPIsFor(allTx, month),
		MonthChartJSON: d,
		Categories:     cats,
		Txs:            monthTxs,
		Insights:       service.MonthInsightsFor(allTx, month),
	}
}

func (h *Handler) MonthPage(w http.ResponseWriter, r *http.Request) {
	month := h.currentMonthOr(r)
	h.rdr.Page(w, "month", monthPageVM{
		base:          h.base("Monthly View", "month"),
		Months:        h.monthOptions(),
		monthDetailVM: h.buildMonthDetail(month),
	})
}

func (h *Handler) MonthDetail(w http.ResponseWriter, r *http.Request) {
	month := h.currentMonthOr(r)
	h.rdr.Fragment(w, "month_detail", h.buildMonthDetail(month))
}
