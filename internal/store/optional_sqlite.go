package store

import (
	"database/sql"
	"strings"

	"finance-tracker/internal/domain"
)

// ---- goals ----

func (s *SQLite) ListGoals() ([]domain.Goal, error) {
	rows, err := s.db.Query(`SELECT id, name, target_paise, saved_paise, target_date, notes, created_at, updated_at FROM goals ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Goal
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func scanGoal(sc interface{ Scan(...any) error }) (domain.Goal, error) {
	var g domain.Goal
	var target, saved int64
	var date, notes sql.NullString
	if err := sc.Scan(&g.ID, &g.Name, &target, &saved, &date, &notes, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return g, err
	}
	g.Target, g.Saved = domain.Money(target), domain.Money(saved)
	if date.Valid {
		g.TargetDate = &date.String
	}
	if notes.Valid {
		g.Notes = &notes.String
	}
	return g, nil
}

func (s *SQLite) GetGoal(id string) (domain.Goal, error) {
	row := s.db.QueryRow(`SELECT id, name, target_paise, saved_paise, target_date, notes, created_at, updated_at FROM goals WHERE id=?`, id)
	g, err := scanGoal(row)
	if err == sql.ErrNoRows {
		return g, ErrNotFound
	}
	return g, err
}

func (s *SQLite) CreateGoal(g domain.Goal) error {
	_, err := s.db.Exec(`INSERT INTO goals (id, name, target_paise, saved_paise, target_date, notes, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		g.ID, g.Name, int64(g.Target), int64(g.Saved), g.TargetDate, g.Notes, g.CreatedAt, g.UpdatedAt)
	return err
}

func (s *SQLite) UpdateGoal(g domain.Goal) error {
	res, err := s.db.Exec(`UPDATE goals SET name=?, target_paise=?, saved_paise=?, target_date=?, notes=?, updated_at=? WHERE id=?`,
		g.Name, int64(g.Target), int64(g.Saved), g.TargetDate, g.Notes, g.UpdatedAt, g.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteGoal(id string) error {
	res, err := s.db.Exec(`DELETE FROM goals WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- budgets ----

func (s *SQLite) ListBudgets() ([]domain.Budget, error) {
	rows, err := s.db.Query(`SELECT category, limit_paise FROM budgets ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Budget
	for rows.Next() {
		var b domain.Budget
		var limit int64
		if err := rows.Scan(&b.Category, &limit); err != nil {
			return nil, err
		}
		b.Limit = domain.Money(limit)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *SQLite) SetBudget(b domain.Budget) error {
	_, err := s.db.Exec(`INSERT INTO budgets (category, limit_paise) VALUES (?, ?)
		ON CONFLICT(category) DO UPDATE SET limit_paise=excluded.limit_paise`,
		b.Category, int64(b.Limit))
	return err
}

func (s *SQLite) DeleteBudget(category string) error {
	_, err := s.db.Exec(`DELETE FROM budgets WHERE category=?`, category)
	return err
}

// ---- tags ----

func (s *SQLite) ListTags() ([]domain.Tag, error) {
	rows, err := s.db.Query(`SELECT name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Tag
	for rows.Next() {
		var t domain.Tag
		if err := rows.Scan(&t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetTransactionTags replaces the tag set for a transaction, creating any new
// tags. An empty list clears the transaction's tags.
func (s *SQLite) SetTransactionTags(txID string, tags []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM transaction_tags WHERE transaction_id=?`, txID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, raw := range tags {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (name) VALUES (?)`, name); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO transaction_tags (transaction_id, tag) VALUES (?, ?)`, txID, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) TagsFor(txID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM transaction_tags WHERE transaction_id=? ORDER BY tag`, txID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TagsForMany returns a map of transaction id -> tags for the given ids.
func (s *SQLite) TagsForMany(txIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(txIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(txIDs)), ",")
	args := make([]any, len(txIDs))
	for i, id := range txIDs {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT transaction_id, tag FROM transaction_tags WHERE transaction_id IN (`+placeholders+`) ORDER BY tag`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return nil, err
		}
		out[id] = append(out[id], tag)
	}
	return out, rows.Err()
}

func (s *SQLite) TransactionIDsWithTag(tag string) ([]string, error) {
	rows, err := s.db.Query(`SELECT transaction_id FROM transaction_tags WHERE tag=?`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---- recurring auto-fire ----

func (s *SQLite) WasPosted(recurringID, month string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM posted_recurring WHERE recurring_id=? AND month=?`, recurringID, month).Scan(&n)
	return n > 0, err
}

func (s *SQLite) MarkPosted(recurringID, month, txID, postedAt string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO posted_recurring (recurring_id, month, transaction_id, posted_at) VALUES (?,?,?,?)`,
		recurringID, month, txID, postedAt)
	return err
}

// ---- gmail import dedupe ----

func (s *SQLite) WasGmailProcessed(messageID string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM gmail_processed WHERE message_id=?`, messageID).Scan(&n)
	return n > 0, err
}

func (s *SQLite) MarkGmailProcessed(messageID, txID, processedAt string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO gmail_processed (message_id, transaction_id, processed_at) VALUES (?,?,?)`,
		messageID, txID, processedAt)
	return err
}
