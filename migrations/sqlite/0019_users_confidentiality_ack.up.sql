-- Per-user confidentiality acknowledgement timestamp (SQLite counterpart
-- of the Postgres migration 0026). NULL = not yet acknowledged.
ALTER TABLE users ADD COLUMN confidentiality_acknowledged_at DATETIME;
CREATE INDEX IF NOT EXISTS idx_users_confidentiality_ack ON users(confidentiality_acknowledged_at);
