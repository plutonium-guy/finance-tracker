package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"finance-tracker/internal/service"
)

// daysUntil returns whole days from now's date to t (negative if past).
func daysUntil(now, t time.Time) int {
	d0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return int(t.Sub(d0).Hours() / 24)
}

// buildAlerts assembles the list of noteworthy items to nudge the user about:
// over-budget categories, credit-card statements due soon or overdue, and
// recurring bills due this month that haven't been posted yet.
func (h *Handler) buildAlerts() []string {
	var out []string

	// Over-budget categories (this month).
	for _, b := range h.budgetStatuses() {
		if b.Over {
			out = append(out, fmt.Sprintf("⚠️ Budget: %s at %s of %s (%s)",
				b.Category, b.Spent.FormatINR(), b.Limit.FormatINR(), pctStr(b.Pct)))
		}
	}

	// Credit-card statements: overdue, or due within 3 days, and unpaid.
	for _, c := range h.cardSummaries() {
		if !c.HasStatement || c.Paid {
			continue
		}
		switch {
		case c.Overdue:
			out = append(out, fmt.Sprintf("💳 %s: statement %s OVERDUE (was due %s)",
				c.Card.Name, c.LastStatementAmount.FormatINR(), c.DueDate.Format("02 Jan")))
		case daysUntil(h.svc.Now(), c.DueDate) <= 3:
			out = append(out, fmt.Sprintf("💳 %s: %s due %s",
				c.Card.Name, c.LastStatementAmount.FormatINR(), c.DueDate.Format("02 Jan")))
		}
	}

	// Recurring bills due this month, not yet posted.
	month := h.svc.CurrentMonth()
	items, _ := h.svc.Store.ListRecurring()
	due := service.DueRecurring(items, month, func(id string) bool {
		posted, _ := h.svc.Store.WasPosted(id, month)
		return posted
	})
	for _, r := range due {
		out = append(out, fmt.Sprintf("🔁 %s: %s %s due", r.Name, string(r.Type), r.Amount.FormatINR()))
	}

	return out
}

// RunAlerts builds the alert list and, when a Telegram chat is configured, sends
// it as one message. Returns the alerts and whether a message was sent.
func (h *Handler) RunAlerts(ctx context.Context) ([]string, bool, error) {
	alerts := h.buildAlerts()
	if len(alerts) == 0 || h.pusher == nil || h.pushChat == 0 {
		return alerts, false, nil
	}
	msg := "🔔 Finance Tracker\n\n" + strings.Join(alerts, "\n")
	if err := h.pusher.SendMessage(ctx, h.pushChat, msg); err != nil {
		return alerts, false, err
	}
	return alerts, true, nil
}

// AlertsRun triggers an alert check (token-protected, for cron):
//
//	curl -X POST http://host:8080/api/alerts/run -H 'Authorization: Bearer $API_PUSH_TOKEN'
func (h *Handler) AlertsRun(w http.ResponseWriter, r *http.Request) {
	if h.apiToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "api not configured (set API_PUSH_TOKEN)"})
		return
	}
	if !h.apiAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	alerts, sent, err := h.RunAlerts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "alerts": alerts})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts, "count": len(alerts), "sent": sent})
}

func pctStr(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }
