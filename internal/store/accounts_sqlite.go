package store

import (
	"database/sql"

	"finance-tracker/internal/domain"
)

const accountColumns = `id, name, type, opening_paise, created_at, updated_at`

func scanAccount(sc interface{ Scan(...any) error }) (domain.Account, error) {
	var a domain.Account
	var opening int64
	if err := sc.Scan(&a.ID, &a.Name, &a.Type, &opening, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return a, err
	}
	a.OpeningBalance = domain.Money(opening)
	return a, nil
}

func (s *SQLite) ListAccounts() ([]domain.Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountColumns + ` FROM accounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLite) GetAccount(id string) (domain.Account, error) {
	row := s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE id=?`, id)
	a, err := scanAccount(row)
	if err == sql.ErrNoRows {
		return a, ErrNotFound
	}
	return a, err
}

func (s *SQLite) CreateAccount(a domain.Account) error {
	_, err := s.db.Exec(`INSERT INTO accounts (`+accountColumns+`) VALUES (?,?,?,?,?,?)`,
		a.ID, a.Name, a.Type, int64(a.OpeningBalance), a.CreatedAt, a.UpdatedAt)
	return err
}

func (s *SQLite) UpdateAccount(a domain.Account) error {
	res, err := s.db.Exec(`UPDATE accounts SET name=?, type=?, opening_paise=?, updated_at=? WHERE id=?`,
		a.Name, a.Type, int64(a.OpeningBalance), a.UpdatedAt, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteAccount(id string) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetTransactionAccount links a transaction to an account; "" clears the link.
func (s *SQLite) SetTransactionAccount(txID, accountID string) error {
	if accountID == "" {
		_, err := s.db.Exec(`DELETE FROM transaction_accounts WHERE transaction_id=?`, txID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO transaction_accounts (transaction_id, account_id) VALUES (?, ?)
		ON CONFLICT(transaction_id) DO UPDATE SET account_id=excluded.account_id`, txID, accountID)
	return err
}

func (s *SQLite) AccountOfTransaction(txID string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT account_id FROM transaction_accounts WHERE transaction_id=?`, txID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *SQLite) TransactionAccountMap() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT transaction_id, account_id FROM transaction_accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var tx, acct string
		if err := rows.Scan(&tx, &acct); err != nil {
			return nil, err
		}
		out[tx] = acct
	}
	return out, rows.Err()
}
