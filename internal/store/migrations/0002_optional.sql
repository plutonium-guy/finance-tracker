-- Optional features: goals, budgets, tags, recurring auto-fire bookkeeping.

CREATE TABLE IF NOT EXISTS goals (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  target_paise INTEGER NOT NULL CHECK (target_paise > 0),
  saved_paise  INTEGER NOT NULL DEFAULT 0 CHECK (saved_paise >= 0),
  target_date  TEXT,            -- "YYYY-MM-DD", optional
  notes        TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS budgets (
  category    TEXT PRIMARY KEY REFERENCES categories(name) ON UPDATE CASCADE ON DELETE CASCADE,
  limit_paise INTEGER NOT NULL CHECK (limit_paise > 0)
);

CREATE TABLE IF NOT EXISTS tags (
  name TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS transaction_tags (
  transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  tag            TEXT NOT NULL REFERENCES tags(name) ON UPDATE CASCADE ON DELETE CASCADE,
  PRIMARY KEY (transaction_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_txtag_tag ON transaction_tags(tag);

-- One row per recurring item per month it was auto-posted (idempotency guard).
CREATE TABLE IF NOT EXISTS posted_recurring (
  recurring_id   TEXT NOT NULL,
  month          TEXT NOT NULL,   -- "YYYY-MM"
  transaction_id TEXT,
  posted_at      TEXT NOT NULL,
  PRIMARY KEY (recurring_id, month)
);
