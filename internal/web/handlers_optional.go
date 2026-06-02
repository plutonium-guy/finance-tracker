package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

// ---- Planning page (Goals + Budgets) ----

type planningVM struct {
	base
	Month      string
	Goals      []service.GoalProgress
	Budgets    []service.BudgetStatus
	Categories []domain.Category
}

type goalsVM struct {
	Goals []service.GoalProgress
}

type budgetsVM struct {
	Budgets    []service.BudgetStatus
	Categories []domain.Category
}

type goalModalVM struct {
	Goal  *domain.Goal
	Today string
	Error string
}

func (h *Handler) goalProgress() []service.GoalProgress {
	goals, _ := h.svc.Store.ListGoals()
	out := make([]service.GoalProgress, 0, len(goals))
	for _, g := range goals {
		out = append(out, service.GoalProgressFor(g, h.svc.Now()))
	}
	return out
}

func (h *Handler) budgetStatuses() []service.BudgetStatus {
	budgets, _ := h.svc.Store.ListBudgets()
	txs, _ := h.svc.Store.AllTransactions()
	return service.BudgetStatuses(budgets, txs, h.svc.CurrentMonth())
}

func (h *Handler) PlanningPage(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.svc.Store.ListCategories()
	h.rdr.Page(w, "planning", planningVM{
		base:       h.base("Planning", "planning"),
		Month:      h.svc.CurrentMonth(),
		Goals:      h.goalProgress(),
		Budgets:    h.budgetStatuses(),
		Categories: cats,
	})
}

func (h *Handler) GoalsList(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "goals_list", goalsVM{Goals: h.goalProgress()})
}

func (h *Handler) GoalNew(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "goal_modal", goalModalVM{Today: h.svc.Now().Format("2006-01-02")})
}

func (h *Handler) GoalEdit(w http.ResponseWriter, r *http.Request) {
	g, err := h.svc.Store.GetGoal(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.rdr.Fragment(w, "goal_modal", goalModalVM{Goal: &g, Today: h.svc.Now().Format("2006-01-02")})
}

func (h *Handler) parseGoalForm(r *http.Request, existing domain.Goal) (domain.Goal, error) {
	if err := r.ParseForm(); err != nil {
		return existing, err
	}
	g := existing
	g.Name = strings.TrimSpace(r.FormValue("name"))
	if g.Name == "" {
		return g, errValidation("Name is required")
	}
	target, err := strconv.ParseFloat(r.FormValue("target"), 64)
	if err != nil || target <= 0 {
		return g, errValidation("Target must be a positive amount")
	}
	g.Target = domain.ToPaise(target)
	saved, _ := strconv.ParseFloat(r.FormValue("saved"), 64)
	if saved < 0 {
		saved = 0
	}
	g.Saved = domain.ToPaise(saved)
	if d := strings.TrimSpace(r.FormValue("target_date")); d != "" {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return g, errValidation("Target date must be YYYY-MM-DD")
		}
		g.TargetDate = &d
	} else {
		g.TargetDate = nil
	}
	if n := strings.TrimSpace(r.FormValue("notes")); n != "" {
		g.Notes = &n
	} else {
		g.Notes = nil
	}
	return g, nil
}

func (h *Handler) GoalCreate(w http.ResponseWriter, r *http.Request) {
	g, err := h.parseGoalForm(r, domain.Goal{})
	if err != nil {
		h.modalError(w, "goal_modal", goalModalVM{Today: h.svc.Now().Format("2006-01-02"), Error: err.Error()})
		return
	}
	now := h.svc.Now().UTC().Format(time.RFC3339)
	g.ID = uuid.NewString()
	g.CreatedAt, g.UpdatedAt = now, now
	if err := h.svc.Store.CreateGoal(g); err != nil {
		h.modalError(w, "goal_modal", goalModalVM{Today: h.svc.Now().Format("2006-01-02"), Error: err.Error()})
		return
	}
	h.GoalsList(w, r)
}

func (h *Handler) GoalUpdate(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetGoal(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	g, err := h.parseGoalForm(r, existing)
	if err != nil {
		h.modalError(w, "goal_modal", goalModalVM{Goal: &existing, Today: h.svc.Now().Format("2006-01-02"), Error: err.Error()})
		return
	}
	g.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
	if err := h.svc.Store.UpdateGoal(g); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.GoalsList(w, r)
}

func (h *Handler) GoalDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteGoal(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.GoalsList(w, r)
}

// GoalAddSaved adds an amount to a goal's saved total (quick "contribute" button).
func (h *Handler) GoalAddSaved(w http.ResponseWriter, r *http.Request) {
	g, err := h.svc.Store.GetGoal(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	add, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	if add != 0 {
		g.Saved += domain.ToPaise(add)
		if g.Saved < 0 {
			g.Saved = 0
		}
		g.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
		h.svc.Store.UpdateGoal(g)
	}
	h.GoalsList(w, r)
}

// ---- Budgets ----

func (h *Handler) BudgetsList(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.svc.Store.ListCategories()
	h.rdr.Fragment(w, "budget_table", budgetsVM{Budgets: h.budgetStatuses(), Categories: cats})
}

func (h *Handler) BudgetSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	category := r.FormValue("category")
	limit, err := strconv.ParseFloat(r.FormValue("limit"), 64)
	if category == "" || err != nil || limit <= 0 {
		w.Header().Set("HX-Retarget", "#flash")
		h.flash(w, "Pick a category and a positive limit")
		return
	}
	if err := h.svc.Store.SetBudget(domain.Budget{Category: category, Limit: domain.ToPaise(limit)}); err != nil {
		http.Error(w, friendly(err), 400)
		return
	}
	h.BudgetsList(w, r)
}

func (h *Handler) BudgetDelete(w http.ResponseWriter, r *http.Request) {
	h.svc.Store.DeleteBudget(chi.URLParam(r, "category"))
	h.BudgetsList(w, r)
}

// ---- Recurring auto-fire ----

func (h *Handler) RecurringFire(w http.ResponseWriter, r *http.Request) {
	month := h.svc.CurrentMonth()
	posted, _ := h.svc.PostDue(month)
	txTrigger(w)
	h.flash(w, fmt.Sprintf("Posted %d due recurring item(s) for %s", posted, service.MonthKey(h.svc.Now())))
}

// ---- Year-end sankey ----

type yearVM struct {
	base
	Year   string
	Years  []string
	Sankey service.SankeyData
}

func (h *Handler) yearOptions(selected string) []string {
	txs, _ := h.svc.Store.AllTransactions()
	set := map[string]bool{selected: true, h.svc.Now().Format("2006"): true}
	for _, t := range txs {
		if len(t.Date) >= 4 {
			set[t.Date[:4]] = true
		}
	}
	var years []string
	for y := range set {
		if y != "" {
			years = append(years, y)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(years)))
	return years
}

func (h *Handler) YearPage(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("y")
	if year == "" {
		year = h.svc.Now().Format("2006")
	}
	txs, _ := h.svc.Store.AllTransactions()
	h.rdr.Page(w, "year", yearVM{
		base:   h.base("Year", "year"),
		Year:   year,
		Years:  h.yearOptions(year),
		Sankey: service.YearSankey(txs, year),
	})
}

func (h *Handler) YearDetail(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("y")
	if year == "" {
		year = h.svc.Now().Format("2006")
	}
	txs, _ := h.svc.Store.AllTransactions()
	h.rdr.Fragment(w, "sankey", service.YearSankey(txs, year))
}
