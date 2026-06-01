package store

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"finance-tracker/internal/domain"
)

// SQLite is a Store backed by a SQLite database (pure-Go modernc driver).
type SQLite struct {
	db *sql.DB
}

// OpenSQLite opens (or creates) the database at path with foreign keys enabled.
// Use ":memory:" for an ephemeral test database.
func OpenSQLite(path string) (*SQLite, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection avoids "database is locked" on the default file DB and
	// keeps an in-memory DB alive for the whole process.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

// Migrate applies all embedded migrations in lexical order.
func (s *SQLite) Migrate() error {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := migrationFS.ReadFile("migrations/" + n)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(b)); err != nil {
			return fmt.Errorf("migration %s: %w", n, err)
		}
	}
	return nil
}

// EnsureSettings inserts the singleton settings row with defaults if absent.
func (s *SQLite) EnsureSettings() error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO settings (id) VALUES (1)`)
	return err
}

// SeedDefaultCategories inserts the canonical default categories (idempotent).
func (s *SQLite) SeedDefaultCategories() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range domain.DefaultCategories {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO categories (name, is_default) VALUES (?, 1)`, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- categories ----

func (s *SQLite) ListCategories() ([]domain.Category, error) {
	rows, err := s.db.Query(`SELECT name, is_default FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Category
	for rows.Next() {
		var c domain.Category
		var def int
		if err := rows.Scan(&c.Name, &def); err != nil {
			return nil, err
		}
		c.IsDefault = def != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) AddCategory(name string) error {
	_, err := s.db.Exec(`INSERT INTO categories (name, is_default) VALUES (?, 0)`, name)
	return err
}

func (s *SQLite) RenameCategory(oldName, newName string) error {
	// ON UPDATE CASCADE propagates to transactions/recurring_items.
	res, err := s.db.Exec(`UPDATE categories SET name=? WHERE name=?`, newName, oldName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) CategoryInUse(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT (SELECT COUNT(*) FROM transactions WHERE category=?) +
		        (SELECT COUNT(*) FROM recurring_items WHERE category=?)`,
		name, name).Scan(&n)
	return n > 0, err
}

func (s *SQLite) DeleteCategory(name string, reassignTo string) error {
	inUse, err := s.CategoryInUse(name)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if inUse {
		if reassignTo == "" {
			return ErrCategoryInUse
		}
		if _, err := tx.Exec(`UPDATE transactions SET category=? WHERE category=?`, reassignTo, name); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE recurring_items SET category=? WHERE category=?`, reassignTo, name); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM categories WHERE name=?`, name); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- transactions ----

const txColumns = `id, date, description, amount_paise, type, payment_method, category, notes, created_at, updated_at`

func scanTx(sc interface{ Scan(...any) error }) (domain.Transaction, error) {
	var t domain.Transaction
	var amount int64
	var notes sql.NullString
	if err := sc.Scan(&t.ID, &t.Date, &t.Description, &amount, &t.Type, &t.PaymentMethod, &t.Category, &notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return t, err
	}
	t.Amount = domain.Money(amount)
	if notes.Valid {
		t.Notes = &notes.String
	}
	return t, nil
}

var allowedSort = map[string]string{
	"date": "date", "amount": "amount_paise", "description": "description",
	"category": "category", "type": "type",
}

func (s *SQLite) Transactions(f TxFilter) ([]domain.Transaction, int, error) {
	var where []string
	var args []any
	if f.Month != "" {
		where = append(where, "substr(date,1,7) = ?")
		args = append(args, f.Month)
	}
	if f.Category != "" {
		where = append(where, "category = ?")
		args = append(args, f.Category)
	}
	if f.Type != "" {
		where = append(where, "type = ?")
		args = append(args, f.Type)
	}
	if f.PaymentMethod != "" {
		where = append(where, "payment_method = ?")
		args = append(args, f.PaymentMethod)
	}
	if f.Query != "" {
		where = append(where, "description LIKE ?")
		args = append(args, "%"+f.Query+"%")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM transactions`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	col, ok := allowedSort[f.Sort]
	if !ok {
		col = "date"
	}
	order := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		order = "ASC"
	}
	// Stable tiebreaker on created_at so equal sort keys keep deterministic order.
	q := `SELECT ` + txColumns + ` FROM transactions` + clause +
		fmt.Sprintf(" ORDER BY %s %s, created_at DESC", col, order)

	if f.PageSize > 0 {
		page := f.Page
		if page < 1 {
			page = 1
		}
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.PageSize, (page-1)*f.PageSize)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (s *SQLite) AllTransactions() ([]domain.Transaction, error) {
	rows, err := s.db.Query(`SELECT ` + txColumns + ` FROM transactions ORDER BY date DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLite) GetTransaction(id string) (domain.Transaction, error) {
	row := s.db.QueryRow(`SELECT `+txColumns+` FROM transactions WHERE id=?`, id)
	t, err := scanTx(row)
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	return t, err
}

func (s *SQLite) CreateTransaction(t domain.Transaction) error {
	_, err := s.db.Exec(`INSERT INTO transactions (`+txColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Date, t.Description, int64(t.Amount), t.Type, t.PaymentMethod, t.Category, t.Notes, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *SQLite) UpdateTransaction(t domain.Transaction) error {
	res, err := s.db.Exec(`UPDATE transactions SET date=?, description=?, amount_paise=?, type=?, payment_method=?, category=?, notes=?, updated_at=? WHERE id=?`,
		t.Date, t.Description, int64(t.Amount), t.Type, t.PaymentMethod, t.Category, t.Notes, t.UpdatedAt, t.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteTransaction(id string) error {
	res, err := s.db.Exec(`DELETE FROM transactions WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- recurring ----

const recColumns = `id, name, category, amount_paise, frequency, type, start_date, end_date, payment_method, active, notes, created_at, updated_at`

func scanRec(sc interface{ Scan(...any) error }) (domain.RecurringItem, error) {
	var r domain.RecurringItem
	var amount int64
	var active int
	var endDate, notes sql.NullString
	if err := sc.Scan(&r.ID, &r.Name, &r.Category, &amount, &r.Frequency, &r.Type, &r.StartDate, &endDate, &r.PaymentMethod, &active, &notes, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return r, err
	}
	r.Amount = domain.Money(amount)
	r.Active = active != 0
	if endDate.Valid {
		r.EndDate = &endDate.String
	}
	if notes.Valid {
		r.Notes = &notes.String
	}
	return r, nil
}

func (s *SQLite) ListRecurring() ([]domain.RecurringItem, error) {
	rows, err := s.db.Query(`SELECT ` + recColumns + ` FROM recurring_items ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RecurringItem
	for rows.Next() {
		r, err := scanRec(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) GetRecurring(id string) (domain.RecurringItem, error) {
	row := s.db.QueryRow(`SELECT `+recColumns+` FROM recurring_items WHERE id=?`, id)
	r, err := scanRec(row)
	if err == sql.ErrNoRows {
		return r, ErrNotFound
	}
	return r, err
}

func (s *SQLite) CreateRecurring(r domain.RecurringItem) error {
	_, err := s.db.Exec(`INSERT INTO recurring_items (`+recColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.Category, int64(r.Amount), r.Frequency, r.Type, r.StartDate, r.EndDate, r.PaymentMethod, boolToInt(r.Active), r.Notes, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *SQLite) UpdateRecurring(r domain.RecurringItem) error {
	res, err := s.db.Exec(`UPDATE recurring_items SET name=?, category=?, amount_paise=?, frequency=?, type=?, start_date=?, end_date=?, payment_method=?, active=?, notes=?, updated_at=? WHERE id=?`,
		r.Name, r.Category, int64(r.Amount), r.Frequency, r.Type, r.StartDate, r.EndDate, r.PaymentMethod, boolToInt(r.Active), r.Notes, r.UpdatedAt, r.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteRecurring(id string) error {
	res, err := s.db.Exec(`DELETE FROM recurring_items WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) ToggleRecurring(id string) (domain.RecurringItem, error) {
	if _, err := s.db.Exec(`UPDATE recurring_items SET active = 1 - active WHERE id=?`, id); err != nil {
		return domain.RecurringItem{}, err
	}
	return s.GetRecurring(id)
}

// ---- settings ----

func (s *SQLite) GetSettings() (domain.AppSettings, error) {
	var a domain.AppSettings
	var payCycle, dark int
	err := s.db.QueryRow(`SELECT currency, locale, month_format, date_format, fiscal_year_start, pay_cycle_enabled, dark_mode FROM settings WHERE id=1`).
		Scan(&a.Currency, &a.Locale, &a.MonthFormat, &a.DateFormat, &a.FiscalYearStart, &payCycle, &dark)
	if err == sql.ErrNoRows {
		return domain.DefaultSettings(), nil
	}
	a.PayCycleEnabled = payCycle != 0
	a.DarkMode = dark != 0
	return a, err
}

func (s *SQLite) SaveSettings(a domain.AppSettings) error {
	_, err := s.db.Exec(`UPDATE settings SET currency=?, locale=?, month_format=?, date_format=?, fiscal_year_start=?, pay_cycle_enabled=?, dark_mode=? WHERE id=1`,
		a.Currency, a.Locale, a.MonthFormat, a.DateFormat, a.FiscalYearStart, boolToInt(a.PayCycleEnabled), boolToInt(a.DarkMode))
	return err
}

// ---- backup ----

func (s *SQLite) Export() (Backup, error) {
	var b Backup
	var err error
	if b.Transactions, err = s.AllTransactions(); err != nil {
		return b, err
	}
	if b.Recurring, err = s.ListRecurring(); err != nil {
		return b, err
	}
	if b.Categories, err = s.ListCategories(); err != nil {
		return b, err
	}
	if b.Settings, err = s.GetSettings(); err != nil {
		return b, err
	}
	if b.Goals, err = s.ListGoals(); err != nil {
		return b, err
	}
	if b.Budgets, err = s.ListBudgets(); err != nil {
		return b, err
	}
	if b.Tags, err = s.ListTags(); err != nil {
		return b, err
	}
	if b.Cards, err = s.ListCards(); err != nil {
		return b, err
	}
	if b.StatementPayments, err = s.allStatementPayments(); err != nil {
		return b, err
	}
	if b.TransactionCards, err = s.TransactionCardMap(); err != nil {
		return b, err
	}
	return b, nil
}

func (s *SQLite) Import(b Backup) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`DELETE FROM statement_payments`, `DELETE FROM transaction_cards`, `DELETE FROM cards`,
		`DELETE FROM transaction_tags`, `DELETE FROM tags`, `DELETE FROM budgets`, `DELETE FROM goals`,
		`DELETE FROM transactions`, `DELETE FROM recurring_items`, `DELETE FROM categories`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	for _, c := range b.Categories {
		if _, err := tx.Exec(`INSERT INTO categories (name, is_default) VALUES (?, ?)`, c.Name, boolToInt(c.IsDefault)); err != nil {
			return err
		}
	}
	for _, t := range b.Transactions {
		if _, err := tx.Exec(`INSERT INTO transactions (`+txColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			t.ID, t.Date, t.Description, int64(t.Amount), t.Type, t.PaymentMethod, t.Category, t.Notes, t.CreatedAt, t.UpdatedAt); err != nil {
			return err
		}
	}
	for _, r := range b.Recurring {
		if _, err := tx.Exec(`INSERT INTO recurring_items (`+recColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.ID, r.Name, r.Category, int64(r.Amount), r.Frequency, r.Type, r.StartDate, r.EndDate, r.PaymentMethod, boolToInt(r.Active), r.Notes, r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	for _, g := range b.Goals {
		if _, err := tx.Exec(`INSERT INTO goals (id, name, target_paise, saved_paise, target_date, notes, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			g.ID, g.Name, int64(g.Target), int64(g.Saved), g.TargetDate, g.Notes, g.CreatedAt, g.UpdatedAt); err != nil {
			return err
		}
	}
	for _, bg := range b.Budgets {
		if _, err := tx.Exec(`INSERT INTO budgets (category, limit_paise) VALUES (?, ?)`, bg.Category, int64(bg.Limit)); err != nil {
			return err
		}
	}
	for _, t := range b.Tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (name) VALUES (?)`, t.Name); err != nil {
			return err
		}
	}
	for _, c := range b.Cards {
		if _, err := tx.Exec(`INSERT INTO cards (`+cardColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
			c.ID, c.Name, c.Last4, int64(c.Limit), c.StatementDay, c.DueOffsetDays, c.CreatedAt, c.UpdatedAt); err != nil {
			return err
		}
	}
	for txID, cardID := range b.TransactionCards {
		if _, err := tx.Exec(`INSERT INTO transaction_cards (transaction_id, card_id) VALUES (?, ?)`, txID, cardID); err != nil {
			return err
		}
	}
	for _, p := range b.StatementPayments {
		if _, err := tx.Exec(`INSERT INTO statement_payments (card_id, period_end, amount_paise, transaction_id, paid_at) VALUES (?,?,?,?,?)`,
			p.CardID, p.PeriodEnd, int64(p.Amount), p.TxID, p.PaidAt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE settings SET currency=?, locale=?, month_format=?, date_format=?, fiscal_year_start=?, pay_cycle_enabled=?, dark_mode=? WHERE id=1`,
		b.Settings.Currency, b.Settings.Locale, b.Settings.MonthFormat, b.Settings.DateFormat, b.Settings.FiscalYearStart, boolToInt(b.Settings.PayCycleEnabled), boolToInt(b.Settings.DarkMode)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`DELETE FROM statement_payments`, `DELETE FROM transaction_cards`, `DELETE FROM cards`,
		`DELETE FROM transaction_tags`, `DELETE FROM tags`, `DELETE FROM budgets`, `DELETE FROM goals`,
		`DELETE FROM posted_recurring`, `DELETE FROM gmail_processed`,
		`DELETE FROM transactions`, `DELETE FROM recurring_items`, `DELETE FROM categories`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	for _, name := range domain.DefaultCategories {
		if _, err := tx.Exec(`INSERT INTO categories (name, is_default) VALUES (?, 1)`, name); err != nil {
			return err
		}
	}
	d := domain.DefaultSettings()
	if _, err := tx.Exec(`UPDATE settings SET currency=?, locale=?, month_format=?, date_format=?, fiscal_year_start=?, pay_cycle_enabled=?, dark_mode=? WHERE id=1`,
		d.Currency, d.Locale, d.MonthFormat, d.DateFormat, d.FiscalYearStart, boolToInt(d.PayCycleEnabled), boolToInt(d.DarkMode)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) IsEmpty() (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM transactions) + (SELECT COUNT(*) FROM recurring_items)`).Scan(&n)
	return n == 0, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
