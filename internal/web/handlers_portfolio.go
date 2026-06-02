package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

// NavSyncer runs a mutual-fund NAV refresh on demand.
type NavSyncer interface {
	Run(ctx context.Context) (NavResult, error)
}

// NavResult mirrors navsync.Result so the web layer avoids importing it directly.
type NavResult struct {
	Fetched int `json:"fetched"`
	Updated int `json:"updated"`
}

type portfolioPageVM struct {
	base
	Summary service.PortfolioSummary
	Types   []domain.AssetType
}

type portfolioListVM struct {
	Summary service.PortfolioSummary
	Types   []domain.AssetType
}

type holdingModalVM struct {
	Holding *domain.Holding
	Units   string // pre-formatted unit count for edit
	Types   []domain.AssetType
	Error   string
}

func (h *Handler) portfolioSummary() service.PortfolioSummary {
	holdings, _ := h.svc.Store.ListHoldings()
	return service.SummarizePortfolio(holdings)
}

func (h *Handler) PortfolioPage(w http.ResponseWriter, r *http.Request) {
	h.rdr.Page(w, "portfolio", portfolioPageVM{
		base: h.base("Portfolio", "portfolio"), Summary: h.portfolioSummary(), Types: domain.ValidAssetTypes,
	})
}

func (h *Handler) PortfolioList(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "portfolio_list", portfolioListVM{Summary: h.portfolioSummary(), Types: domain.ValidAssetTypes})
}

func (h *Handler) HoldingNew(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "holding_modal", holdingModalVM{Types: domain.ValidAssetTypes})
}

func (h *Handler) HoldingEdit(w http.ResponseWriter, r *http.Request) {
	hd, err := h.svc.Store.GetHolding(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.rdr.Fragment(w, "holding_modal", holdingModalVM{Holding: &hd, Units: domain.FormatUnits(hd.UnitsMicro), Types: domain.ValidAssetTypes})
}

func (h *Handler) parseHoldingForm(r *http.Request, existing domain.Holding) (domain.Holding, error) {
	if err := r.ParseForm(); err != nil {
		return existing, err
	}
	hd := existing
	hd.Name = strings.TrimSpace(r.FormValue("name"))
	if hd.Name == "" {
		return hd, errValidation("Name is required")
	}
	hd.Type = domain.AssetType(r.FormValue("type"))
	if !domain.IsValidAssetType(hd.Type) {
		return hd, errValidation("Invalid asset type")
	}
	units, err := strconv.ParseFloat(r.FormValue("units"), 64)
	if err != nil || units <= 0 {
		return hd, errValidation("Units must be a positive number")
	}
	hd.UnitsMicro = domain.UnitsToMicro(units)
	avg, err := strconv.ParseFloat(r.FormValue("avg_cost"), 64)
	if err != nil || avg < 0 {
		return hd, errValidation("Average cost must be a number")
	}
	hd.AvgCost = domain.ToPaise(avg)
	if p := strings.TrimSpace(r.FormValue("last_price")); p != "" {
		lp, err := strconv.ParseFloat(p, 64)
		if err != nil || lp < 0 {
			return hd, errValidation("Last price must be a number")
		}
		hd.LastPrice = domain.ToPaise(lp)
		hd.LastPriceAt = h.svc.Now().UTC().Format(time.RFC3339)
	}
	hd.SchemeCode = strings.TrimSpace(r.FormValue("scheme_code"))
	return hd, nil
}

func (h *Handler) HoldingCreate(w http.ResponseWriter, r *http.Request) {
	hd, err := h.parseHoldingForm(r, domain.Holding{})
	if err != nil {
		h.modalError(w, "holding_modal", holdingModalVM{Types: domain.ValidAssetTypes, Error: err.Error()})
		return
	}
	now := h.svc.Now().UTC().Format(time.RFC3339)
	hd.ID = uuid.NewString()
	hd.CreatedAt, hd.UpdatedAt = now, now
	if err := h.svc.Store.CreateHolding(hd); err != nil {
		h.modalError(w, "holding_modal", holdingModalVM{Types: domain.ValidAssetTypes, Error: friendly(err)})
		return
	}
	h.PortfolioList(w, r)
}

func (h *Handler) HoldingUpdate(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetHolding(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	hd, err := h.parseHoldingForm(r, existing)
	if err != nil {
		h.modalError(w, "holding_modal", holdingModalVM{Holding: &existing, Units: domain.FormatUnits(existing.UnitsMicro), Types: domain.ValidAssetTypes, Error: err.Error()})
		return
	}
	hd.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
	if err := h.svc.Store.UpdateHolding(hd); err != nil {
		h.modalError(w, "holding_modal", holdingModalVM{Holding: &existing, Units: domain.FormatUnits(existing.UnitsMicro), Types: domain.ValidAssetTypes, Error: friendly(err)})
		return
	}
	h.PortfolioList(w, r)
}

func (h *Handler) HoldingDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteHolding(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.PortfolioList(w, r)
}

// EnableNav wires the NAV syncer for the refresh endpoints.
func (h *Handler) EnableNav(s NavSyncer) { h.nav = s }

// NavSyncUI refreshes NAVs from the portfolio page (same-origin, no token).
func (h *Handler) NavSyncUI(w http.ResponseWriter, r *http.Request) {
	if h.nav == nil {
		h.flash(w, "NAV sync is not available")
		return
	}
	if _, err := h.nav.Run(r.Context()); err != nil {
		w.Header().Set("HX-Retarget", "#flash")
		h.flash(w, "NAV sync failed: "+err.Error())
		return
	}
	h.PortfolioList(w, r)
}

// NavSyncAPI refreshes NAVs (token-protected, for cron).
func (h *Handler) NavSyncAPI(w http.ResponseWriter, r *http.Request) {
	if h.nav == nil || h.apiToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "nav sync not configured"})
		return
	}
	if !h.apiAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	res, err := h.nav.Run(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
