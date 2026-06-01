-- Credit cards, the card a transaction belongs to, and statement payments.
-- (Linked via a side table rather than ALTER TABLE so the migration is
-- idempotent — every migration file is re-applied on each boot.)
CREATE TABLE IF NOT EXISTS cards (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL,
  last4           TEXT NOT NULL DEFAULT '',
  limit_paise     INTEGER NOT NULL DEFAULT 0 CHECK(limit_paise >= 0),
  statement_day   INTEGER NOT NULL DEFAULT 1 CHECK(statement_day BETWEEN 1 AND 28),
  due_offset_days INTEGER NOT NULL DEFAULT 18 CHECK(due_offset_days BETWEEN 0 AND 60),
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS transaction_cards (
  transaction_id TEXT PRIMARY KEY REFERENCES transactions(id) ON DELETE CASCADE,
  card_id        TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_transaction_cards_card ON transaction_cards(card_id);

CREATE TABLE IF NOT EXISTS statement_payments (
  card_id        TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  period_end     TEXT NOT NULL, -- statement closing date "YYYY-MM-DD"
  amount_paise   INTEGER NOT NULL,
  transaction_id TEXT,
  paid_at        TEXT NOT NULL,
  PRIMARY KEY (card_id, period_end)
);
