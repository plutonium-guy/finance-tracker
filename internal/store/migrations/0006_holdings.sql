-- Investment holdings (mutual funds, stocks, ETFs, gold, …).
CREATE TABLE IF NOT EXISTS holdings (
  id             TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  type           TEXT NOT NULL DEFAULT 'Mutual Fund',
  units_micro    INTEGER NOT NULL DEFAULT 0,
  avg_cost_paise INTEGER NOT NULL DEFAULT 0, -- paise per unit
  last_price_paise INTEGER NOT NULL DEFAULT 0, -- paise per unit (0 = unknown)
  scheme_code    TEXT NOT NULL DEFAULT '',
  last_price_at  TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_holdings_scheme ON holdings(scheme_code);
