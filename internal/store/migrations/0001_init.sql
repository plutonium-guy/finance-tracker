CREATE TABLE IF NOT EXISTS categories (
  name        TEXT PRIMARY KEY,
  is_default  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS transactions (
  id             TEXT PRIMARY KEY,
  date           TEXT NOT NULL,            -- "YYYY-MM-DD"
  description    TEXT NOT NULL,
  amount_paise   INTEGER NOT NULL CHECK (amount_paise > 0),
  type           TEXT NOT NULL CHECK (type IN ('Income','Expense','Transfer')),
  payment_method TEXT NOT NULL,
  category       TEXT NOT NULL REFERENCES categories(name) ON UPDATE CASCADE,
  notes          TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tx_date     ON transactions(date);
CREATE INDEX IF NOT EXISTS idx_tx_type     ON transactions(type);
CREATE INDEX IF NOT EXISTS idx_tx_category ON transactions(category);

CREATE TABLE IF NOT EXISTS recurring_items (
  id             TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  category       TEXT NOT NULL REFERENCES categories(name) ON UPDATE CASCADE,
  amount_paise   INTEGER NOT NULL CHECK (amount_paise > 0),
  frequency      TEXT NOT NULL,
  type           TEXT NOT NULL CHECK (type IN ('Income','Expense')),
  start_date     TEXT NOT NULL,
  end_date       TEXT,
  payment_method TEXT NOT NULL,
  active         INTEGER NOT NULL DEFAULT 1,
  notes          TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE TABLE IF NOT EXISTS settings (
  id                INTEGER PRIMARY KEY CHECK (id = 1),
  currency          TEXT NOT NULL DEFAULT '₹',
  locale            TEXT NOT NULL DEFAULT 'en-IN',
  month_format      TEXT NOT NULL DEFAULT 'mmm yyyy',
  date_format       TEXT NOT NULL DEFAULT 'dd-mmm-yyyy',
  fiscal_year_start TEXT NOT NULL DEFAULT 'April',
  pay_cycle_enabled INTEGER NOT NULL DEFAULT 1,
  dark_mode         INTEGER NOT NULL DEFAULT 0
);
