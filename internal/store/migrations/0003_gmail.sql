-- Tracks Gmail messages already imported, so re-runs never double-insert.
CREATE TABLE IF NOT EXISTS gmail_processed (
  message_id     TEXT PRIMARY KEY,
  transaction_id TEXT,
  processed_at   TEXT NOT NULL
);
