package store

import (
	"database/sql"

	"finance-tracker/internal/domain"
)

const cardColumns = `id, name, last4, limit_paise, statement_day, due_offset_days, created_at, updated_at`

func scanCard(sc interface{ Scan(...any) error }) (domain.Card, error) {
	var c domain.Card
	var limit int64
	if err := sc.Scan(&c.ID, &c.Name, &c.Last4, &limit, &c.StatementDay, &c.DueOffsetDays, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, err
	}
	c.Limit = domain.Money(limit)
	return c, nil
}

func (s *SQLite) ListCards() ([]domain.Card, error) {
	rows, err := s.db.Query(`SELECT ` + cardColumns + ` FROM cards ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) GetCard(id string) (domain.Card, error) {
	row := s.db.QueryRow(`SELECT `+cardColumns+` FROM cards WHERE id=?`, id)
	c, err := scanCard(row)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	return c, err
}

func (s *SQLite) CreateCard(c domain.Card) error {
	_, err := s.db.Exec(`INSERT INTO cards (`+cardColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Last4, int64(c.Limit), c.StatementDay, c.DueOffsetDays, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *SQLite) UpdateCard(c domain.Card) error {
	res, err := s.db.Exec(`UPDATE cards SET name=?, last4=?, limit_paise=?, statement_day=?, due_offset_days=?, updated_at=? WHERE id=?`,
		c.Name, c.Last4, int64(c.Limit), c.StatementDay, c.DueOffsetDays, c.UpdatedAt, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteCard(id string) error {
	res, err := s.db.Exec(`DELETE FROM cards WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CardIDByLast4 returns the id of the (first) card whose last4 matches. Empty
// last4 never matches.
func (s *SQLite) CardIDByLast4(last4 string) (string, bool, error) {
	if last4 == "" {
		return "", false, nil
	}
	var id string
	err := s.db.QueryRow(`SELECT id FROM cards WHERE last4=? ORDER BY created_at LIMIT 1`, last4).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// SetTransactionCard links a transaction to a card. An empty cardID clears it.
func (s *SQLite) SetTransactionCard(txID, cardID string) error {
	if cardID == "" {
		_, err := s.db.Exec(`DELETE FROM transaction_cards WHERE transaction_id=?`, txID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO transaction_cards (transaction_id, card_id) VALUES (?, ?)
		ON CONFLICT(transaction_id) DO UPDATE SET card_id=excluded.card_id`, txID, cardID)
	return err
}

func (s *SQLite) CardOfTransaction(txID string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT card_id FROM transaction_cards WHERE transaction_id=?`, txID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *SQLite) TransactionCardMap() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT transaction_id, card_id FROM transaction_cards`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var tx, card string
		if err := rows.Scan(&tx, &card); err != nil {
			return nil, err
		}
		out[tx] = card
	}
	return out, rows.Err()
}

// RecordStatementPayment inserts a payment for (card, period_end). It returns
// false (and inserts nothing) if that statement was already paid.
func (s *SQLite) RecordStatementPayment(p domain.StatementPayment) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO statement_payments (card_id, period_end, amount_paise, transaction_id, paid_at) VALUES (?,?,?,?,?)`,
		p.CardID, p.PeriodEnd, int64(p.Amount), p.TxID, p.PaidAt)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *SQLite) ListStatementPayments(cardID string) ([]domain.StatementPayment, error) {
	rows, err := s.db.Query(`SELECT card_id, period_end, amount_paise, transaction_id, paid_at FROM statement_payments WHERE card_id=? ORDER BY period_end`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StatementPayment
	for rows.Next() {
		var p domain.StatementPayment
		var amount int64
		var txID sql.NullString
		if err := rows.Scan(&p.CardID, &p.PeriodEnd, &amount, &txID, &p.PaidAt); err != nil {
			return nil, err
		}
		p.Amount = domain.Money(amount)
		if txID.Valid {
			p.TxID = txID.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// allStatementPayments returns every payment (used by Export).
func (s *SQLite) allStatementPayments() ([]domain.StatementPayment, error) {
	rows, err := s.db.Query(`SELECT card_id, period_end, amount_paise, transaction_id, paid_at FROM statement_payments ORDER BY card_id, period_end`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StatementPayment
	for rows.Next() {
		var p domain.StatementPayment
		var amount int64
		var txID sql.NullString
		if err := rows.Scan(&p.CardID, &p.PeriodEnd, &amount, &txID, &p.PaidAt); err != nil {
			return nil, err
		}
		p.Amount = domain.Money(amount)
		if txID.Valid {
			p.TxID = txID.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
