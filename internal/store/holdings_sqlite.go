package store

import (
	"database/sql"

	"finance-tracker/internal/domain"
)

const holdingColumns = `id, name, type, units_micro, avg_cost_paise, last_price_paise, scheme_code, last_price_at, created_at, updated_at`

func scanHolding(sc interface{ Scan(...any) error }) (domain.Holding, error) {
	var h domain.Holding
	var avg, last int64
	if err := sc.Scan(&h.ID, &h.Name, &h.Type, &h.UnitsMicro, &avg, &last, &h.SchemeCode, &h.LastPriceAt, &h.CreatedAt, &h.UpdatedAt); err != nil {
		return h, err
	}
	h.AvgCost, h.LastPrice = domain.Money(avg), domain.Money(last)
	return h, nil
}

func (s *SQLite) ListHoldings() ([]domain.Holding, error) {
	rows, err := s.db.Query(`SELECT ` + holdingColumns + ` FROM holdings ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Holding
	for rows.Next() {
		h, err := scanHolding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *SQLite) GetHolding(id string) (domain.Holding, error) {
	row := s.db.QueryRow(`SELECT `+holdingColumns+` FROM holdings WHERE id=?`, id)
	h, err := scanHolding(row)
	if err == sql.ErrNoRows {
		return h, ErrNotFound
	}
	return h, err
}

func (s *SQLite) CreateHolding(h domain.Holding) error {
	_, err := s.db.Exec(`INSERT INTO holdings (`+holdingColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.Name, h.Type, h.UnitsMicro, int64(h.AvgCost), int64(h.LastPrice), h.SchemeCode, h.LastPriceAt, h.CreatedAt, h.UpdatedAt)
	return err
}

func (s *SQLite) UpdateHolding(h domain.Holding) error {
	res, err := s.db.Exec(`UPDATE holdings SET name=?, type=?, units_micro=?, avg_cost_paise=?, last_price_paise=?, scheme_code=?, last_price_at=?, updated_at=? WHERE id=?`,
		h.Name, h.Type, h.UnitsMicro, int64(h.AvgCost), int64(h.LastPrice), h.SchemeCode, h.LastPriceAt, h.UpdatedAt, h.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteHolding(id string) error {
	res, err := s.db.Exec(`DELETE FROM holdings WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetHoldingPriceBySchemeCode updates last price (paise/unit) for every holding
// with the given scheme code. Returns the number of rows updated.
func (s *SQLite) SetHoldingPriceBySchemeCode(schemeCode string, pricePaise domain.Money, at string) (int, error) {
	if schemeCode == "" {
		return 0, nil
	}
	res, err := s.db.Exec(`UPDATE holdings SET last_price_paise=?, last_price_at=?, updated_at=? WHERE scheme_code=?`,
		int64(pricePaise), at, at, schemeCode)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
