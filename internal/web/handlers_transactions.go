package web

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/store"
)

// txRow is a transaction decorated with its tags for table rendering.
type txRow struct {
	domain.Transaction
	Tags []string
}

type txPageVM struct {
	base
	Txs        []txRow
	Total      int
	Categories []domain.Category
	Methods    []domain.PaymentMethod
	Tags       []domain.Tag
}

type txTableVM struct {
	Txs   []txRow
	Total int
}

type txModalVM struct {
	Tx         *domain.Transaction
	Categories []domain.Category
	Methods    []domain.PaymentMethod
	Types      []domain.TransactionType
	Today      string
	TagList    string // comma-separated existing tags (for edit)
	Error      string
}

// decorate attaches each transaction's tags for rendering.
func (h *Handler) decorate(txs []domain.Transaction) []txRow {
	ids := make([]string, len(txs))
	for i, t := range txs {
		ids[i] = t.ID
	}
	tagMap, _ := h.svc.Store.TagsForMany(ids)
	rows := make([]txRow, len(txs))
	for i, t := range txs {
		rows[i] = txRow{Transaction: t, Tags: tagMap[t.ID]}
	}
	return rows
}

// one decorates a single transaction.
func (h *Handler) one(t domain.Transaction) txRow {
	tags, _ := h.svc.Store.TagsFor(t.ID)
	return txRow{Transaction: t, Tags: tags}
}

// parseTags splits a comma/space separated tag string into a clean list.
func parseTags(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' }) {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (h *Handler) filterFrom(r *http.Request) store.TxFilter {
	r.ParseForm() // merges URL query and POST body so it works for GET and POST
	q := r.Form
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	return store.TxFilter{
		Month:         q.Get("m"),
		Category:      q.Get("category"),
		Type:          q.Get("type"),
		PaymentMethod: q.Get("paymentMethod"),
		Query:         q.Get("q"),
		Sort:          q.Get("sort"),
		Order:         q.Get("order"),
		Page:          page,
		PageSize:      size,
	}
}

func (h *Handler) TransactionsPage(w http.ResponseWriter, r *http.Request) {
	txs, total, err := h.svc.Store.Transactions(h.filterFrom(r))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	txs, total = h.filterByTag(r, txs, total)
	cats, _ := h.svc.Store.ListCategories()
	tags, _ := h.svc.Store.ListTags()
	h.rdr.Page(w, "transactions", txPageVM{
		base:       h.base("Transactions", "transactions"),
		Txs:        h.decorate(txs),
		Total:      total,
		Categories: cats,
		Methods:    methods(),
		Tags:       tags,
	})
}

func (h *Handler) TransactionsList(w http.ResponseWriter, r *http.Request) {
	txs, total, err := h.svc.Store.Transactions(h.filterFrom(r))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	txs, total = h.filterByTag(r, txs, total)
	h.rdr.Fragment(w, "tx_table", txTableVM{Txs: h.decorate(txs), Total: total})
}

// filterByTag narrows txs to those carrying the "tag" query value, if present.
func (h *Handler) filterByTag(r *http.Request, txs []domain.Transaction, total int) ([]domain.Transaction, int) {
	tag := r.Form.Get("tag")
	if tag == "" {
		return txs, total
	}
	ids, _ := h.svc.Store.TransactionIDsWithTag(tag)
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	var out []domain.Transaction
	for _, t := range txs {
		if set[t.ID] {
			out = append(out, t)
		}
	}
	return out, len(out)
}

func (h *Handler) TransactionNew(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.svc.Store.ListCategories()
	h.rdr.Fragment(w, "tx_modal", txModalVM{
		Categories: cats, Methods: methods(), Types: types(),
		Today: h.svc.Now().Format("2006-01-02"),
	})
}

func (h *Handler) TransactionEdit(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Store.GetTransaction(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	cats, _ := h.svc.Store.ListCategories()
	tags, _ := h.svc.Store.TagsFor(t.ID)
	h.rdr.Fragment(w, "tx_modal", txModalVM{
		Tx: &t, Categories: cats, Methods: methods(), Types: types(),
		Today: h.svc.Now().Format("2006-01-02"), TagList: strings.Join(tags, ", "),
	})
}

func (h *Handler) modalError(w http.ResponseWriter, name string, vm any) {
	// Re-render the modal in place instead of swapping into the table.
	w.Header().Set("HX-Retarget", "#modal")
	w.Header().Set("HX-Reswap", "innerHTML")
	h.rdr.Fragment(w, name, vm)
}

func (h *Handler) TransactionCreate(w http.ResponseWriter, r *http.Request) {
	t, err := h.parseTxForm(r, domain.Transaction{})
	if err != nil {
		cats, _ := h.svc.Store.ListCategories()
		h.modalError(w, "tx_modal", txModalVM{Tx: nil, Categories: cats, Methods: methods(), Types: types(), Today: h.svc.Now().Format("2006-01-02"), Error: err.Error()})
		return
	}
	now := h.svc.Now().UTC().Format(time.RFC3339)
	t.ID = uuid.NewString()
	t.CreatedAt, t.UpdatedAt = now, now
	if err := h.svc.Store.CreateTransaction(t); err != nil {
		cats, _ := h.svc.Store.ListCategories()
		h.modalError(w, "tx_modal", txModalVM{Categories: cats, Methods: methods(), Types: types(), Today: h.svc.Now().Format("2006-01-02"), Error: friendly(err)})
		return
	}
	h.svc.Store.SetTransactionTags(t.ID, parseTags(r.FormValue("tags")))
	txTrigger(w)
	// OOB insert into #tx-rows (no-ops on pages without the table).
	h.rdr.Fragment(w, "tx_created", h.one(t))
}

func (h *Handler) TransactionUpdate(w http.ResponseWriter, r *http.Request) {
	existing, err := h.svc.Store.GetTransaction(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	t, err := h.parseTxForm(r, existing)
	if err != nil {
		cats, _ := h.svc.Store.ListCategories()
		h.modalError(w, "tx_modal", txModalVM{Tx: &existing, Categories: cats, Methods: methods(), Types: types(), Today: h.svc.Now().Format("2006-01-02"), Error: err.Error()})
		return
	}
	t.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
	if err := h.svc.Store.UpdateTransaction(t); err != nil {
		http.Error(w, friendly(err), 400)
		return
	}
	h.svc.Store.SetTransactionTags(t.ID, parseTags(r.FormValue("tags")))
	txTrigger(w)
	h.rdr.Fragment(w, "tx_row", h.one(t))
}

func (h *Handler) TransactionDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.DeleteTransaction(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	txTrigger(w)
	w.WriteHeader(http.StatusOK) // empty body replaces the row
}

func (h *Handler) TransactionDuplicate(w http.ResponseWriter, r *http.Request) {
	src, err := h.svc.Store.GetTransaction(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	srcTags, _ := h.svc.Store.TagsFor(src.ID)
	now := h.svc.Now().UTC().Format(time.RFC3339)
	src.ID = uuid.NewString()
	src.Description += " (copy)"
	src.CreatedAt, src.UpdatedAt = now, now
	if err := h.svc.Store.CreateTransaction(src); err != nil {
		http.Error(w, friendly(err), 400)
		return
	}
	h.svc.Store.SetTransactionTags(src.ID, srcTags)
	txTrigger(w)
	h.rdr.Fragment(w, "tx_row", h.one(src))
}

func (h *Handler) TransactionsExport(w http.ResponseWriter, r *http.Request) {
	txs, _, err := h.svc.Store.Transactions(h.filterFrom(r))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=transactions.csv")
	cw := csv.NewWriter(w)
	cw.Write([]string{"Date", "Description", "Amount", "Type", "PaymentMethod", "Category", "Notes"})
	for _, t := range txs {
		notes := ""
		if t.Notes != nil {
			notes = *t.Notes
		}
		cw.Write([]string{t.Date, t.Description, strconv.FormatFloat(t.Amount.Rupees(), 'f', 2, 64),
			string(t.Type), string(t.PaymentMethod), t.Category, notes})
	}
	cw.Flush()
}

// TransactionsImport ingests a CSV. It auto-detects columns so both the app's
// own export format (Date,Description,Amount,Type,PaymentMethod,Category,Notes)
// and common bank statements (Date, Narration/Description, Debit/Credit columns)
// work without a mapping wizard.
func (h *Handler) TransactionsImport(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		h.flash(w, "No file uploaded")
		return
	}
	defer file.Close()
	cr := csv.NewReader(file)
	cr.FieldsPerRecord = -1 // tolerate ragged rows
	rows, err := cr.ReadAll()
	if err != nil || len(rows) < 2 {
		h.flash(w, "Could not parse CSV")
		return
	}

	col := indexHeaders(rows[0])
	dateC := firstCol(col, "date", "txn date", "transaction date", "value date")
	descC := firstCol(col, "description", "narration", "particulars", "details", "remarks", "transaction details")
	amtC := firstCol(col, "amount")
	typeC := firstCol(col, "type")
	debitC := firstCol(col, "debit", "withdrawal", "withdrawal amt", "withdrawal amt.", "dr")
	creditC := firstCol(col, "credit", "deposit", "deposit amt", "deposit amt.", "cr")
	catC := firstCol(col, "category")
	methodC := firstCol(col, "payment method", "method", "mode")
	notesC := firstCol(col, "notes")

	if descC < 0 || (amtC < 0 && debitC < 0 && creditC < 0) {
		h.flash(w, "CSV needs a description column and either an Amount or Debit/Credit column")
		return
	}

	knownCats := map[string]string{} // lower -> canonical
	if cats, err := h.svc.Store.ListCategories(); err == nil {
		for _, c := range cats {
			knownCats[strings.ToLower(c.Name)] = c.Name
		}
	}

	now := h.svc.Now().UTC().Format(time.RFC3339)
	imported, skipped := 0, 0
	for _, row := range rows[1:] {
		get := func(i int) string {
			if i >= 0 && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		desc := get(descC)
		if desc == "" {
			skipped++
			continue
		}

		var amount float64
		txType := domain.Expense
		switch {
		case amtC >= 0 && parseNum(get(amtC)) != 0:
			amount = parseNum(get(amtC))
			if amount < 0 { // negative amount => expense, positive => keep type col / income
				amount = -amount
				txType = domain.Expense
			} else if typeC >= 0 {
				if tt := domain.TransactionType(get(typeC)); domain.IsValidType(tt) {
					txType = tt
				}
			}
		case debitC >= 0 && parseNum(get(debitC)) > 0:
			amount, txType = parseNum(get(debitC)), domain.Expense
		case creditC >= 0 && parseNum(get(creditC)) > 0:
			amount, txType = parseNum(get(creditC)), domain.Income
		default:
			skipped++
			continue
		}
		if amount <= 0 {
			skipped++
			continue
		}

		date := normalizeDate(get(dateC), h.svc.Now())
		category := "Miscellaneous"
		if catC >= 0 {
			if c, ok := knownCats[strings.ToLower(get(catC))]; ok {
				category = c
			}
		}
		method := domain.OtherMethod
		if methodC >= 0 {
			if m := domain.PaymentMethod(get(methodC)); domain.IsValidPaymentMethod(m) {
				method = m
			}
		}

		t := domain.Transaction{
			ID: uuid.NewString(), Date: date, Description: desc,
			Amount: domain.ToPaise(amount), Type: txType, PaymentMethod: method,
			Category: category, CreatedAt: now, UpdatedAt: now,
		}
		if notesC >= 0 {
			if n := get(notesC); n != "" {
				t.Notes = &n
			}
		}
		if h.svc.Store.CreateTransaction(t) == nil {
			imported++
		} else {
			skipped++
		}
	}
	txTrigger(w)
	h.flash(w, fmt.Sprintf("Imported %d transactions (%d skipped)", imported, skipped))
}

// indexHeaders maps lowercased trimmed header names to their column index.
func indexHeaders(header []string) map[string]int {
	m := map[string]int{}
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// firstCol returns the index of the first matching header name, or -1.
func firstCol(col map[string]int, names ...string) int {
	for _, n := range names {
		if i, ok := col[n]; ok {
			return i
		}
	}
	return -1
}

// parseNum strips ₹, commas, and spaces, returning 0 on failure.
func parseNum(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("₹", "", ",", "", " ", "").Replace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// normalizeDate parses common date layouts to ISO "2006-01-02"; falls back to
// today on failure.
func normalizeDate(s string, now time.Time) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "02-01-2006", "02/01/2006", "02-Jan-2006", "02-Jan-06", "01/02/2006", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return now.Format("2006-01-02")
}

func (h *Handler) TransactionsBulk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ids := r.Form["ids[]"]
	if len(ids) == 0 {
		ids = r.Form["ids"]
	}
	action := r.FormValue("action")
	for _, id := range ids {
		switch action {
		case "delete":
			h.svc.Store.DeleteTransaction(id)
		case "setCategory":
			if t, err := h.svc.Store.GetTransaction(id); err == nil {
				t.Category = r.FormValue("bulkCategory")
				t.UpdatedAt = h.svc.Now().UTC().Format(time.RFC3339)
				h.svc.Store.UpdateTransaction(t)
			}
		}
	}
	txTrigger(w)
	// Re-render the rows for the current filters so the table reflects the change.
	txs, total, _ := h.svc.Store.Transactions(h.filterFrom(r))
	txs, total = h.filterByTag(r, txs, total)
	h.rdr.Fragment(w, "tx_table", txTableVM{Txs: h.decorate(txs), Total: total})
}

// flash renders a small flash message fragment.
func (h *Handler) flash(w http.ResponseWriter, msg string) {
	h.rdr.Fragment(w, "flash", msg)
}

// friendly turns common store errors into readable messages.
func friendly(err error) string {
	if errors.Is(err, store.ErrCategoryInUse) {
		return "Category is in use"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "FOREIGN KEY"):
		return "Category does not exist"
	case strings.Contains(msg, "CHECK"):
		return "Value violates a constraint (amount must be > 0)"
	}
	return msg
}
