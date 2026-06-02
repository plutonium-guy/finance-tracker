package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

type accountsPageVM struct {
	base
	Balances []service.AccountBalance
	NetWorth service.NetWorth
	Types    []domain.AccountType
}

type accountsListVM struct {
	Balances []service.AccountBalance
	NetWorth service.NetWorth
	Types    []domain.AccountType
}

type accountModalVM struct {
	Account *domain.Account
	Types   []domain.AccountType
	Error   string
}

// accountData gathers balances, net worth, and the card/investment inputs.
func (h *Handler) accountData() ([]service.AccountBalance, service.NetWorth) {
	accounts, _ := h.svc.Store.ListAccounts()
	txs, _ := h.svc.Store.AllTransactions()
	txAcct, _ := h.svc.Store.TransactionAccountMap()
	balances := service.AccountBalances(accounts, txs, txAcct)
	// Investments are added in a later increment; 0 for now.
	nw := service.ComputeNetWorth(balances, h.investmentValue(), h.cardSummaries())
	return balances, nw
}

// investmentValue is the total portfolio market value (0 until portfolio ships).
func (h *Handler) investmentValue() domain.Money { return 0 }

func (h *Handler) AccountsPage(w http.ResponseWriter, r *http.Request) {
	balances, nw := h.accountData()
	h.rdr.Page(w, "accounts", accountsPageVM{
		base: h.base("Accounts", "accounts"), Balances: balances, NetWorth: nw, Types: domain.ValidAccountTypes,
	})
}

func (h *Handler) AccountsList(w http.ResponseWriter, r *http.Request) {
	balances, nw := h.accountData()
	h.rdr.Fragment(w, "accounts_list", accountsListVM{Balances: balances, NetWorth: nw, Types: domain.ValidAccountTypes})
}

func (h *Handler) AccountNew(w http.ResponseWriter, r *http.Request) {
	h.rdr.Fragment(w, "account_modal", accountModalVM{Types: domain.ValidAccountTypes})
}

func (h *Handler) AccountEdit(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Store.GetAccount(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.rdr.Fragment(w, "account_modal", accountModalVM{Account: &a, Types: domain.ValidAccountTypes})
}

func (h *Handler) parseAccountForm(r *http.Request, existing domain.Account) (domain.Account, error) {
	if err := r.ParseForm(); err != nil {
		return existing, err
	}
	a := existing
	a.Name = strings.TrimSpace(r.FormValue("name"))
	if a.Name == "" {
		return a, errValidation("Name is required")
	}
	a.Type = domain.AccountType(r.FormValue("type"))
	if !domain.IsValidAccountType(a.Type) {
		return a, errValidation("Invalid account type")
	}
	opening, _ := strconv.ParseFloat(r.FormValue("opening"), 64)
	a.OpeningBalance = domain.ToPaise(opening) // may be negative
	return a, nil
}

func (h *Handler) AccountCreate(w http.ResponseWriter, r *http.Request) {
	a, err := h.parseAccountForm(r, domain.Account{})
	if err != nil {
		h.modalError(w, "account_modal", accountModalVM{Types: domain.ValidAccountTypes, Error: err.Error()})
		return
	}
	now := h.svc.Now().UTC().Format(time.RFC3339)
	a.ID = uuid.NewString()
	a.CreatedAt, a.UpdatedAt = now, now
	if err := h.svc.Store.CreateAccount(a); err != nil {
		h.modalError(w, "account_modal", accountModalVM{Types: domain.ValidAccountTypes, Error: friendly(err)})
		return
	}
	h.AccountsList(w, r)
}

func (h *Handler) AccountUpdate(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetAccount(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	a, err := h.parseAccountForm(r, existing)
	if err != nil {
		h.modalError(w, "account_modal", accountModalVM{Account: &existing, Types: domain.ValidAccountTypes, Error: err.Error()})
		return
	}
	a.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
	if err := h.svc.Store.UpdateAccount(a); err != nil {
		h.modalError(w, "account_modal", accountModalVM{Account: &existing, Types: domain.ValidAccountTypes, Error: friendly(err)})
		return
	}
	h.AccountsList(w, r)
}

func (h *Handler) AccountDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteAccount(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	txTrigger(w)
	h.AccountsList(w, r)
}
