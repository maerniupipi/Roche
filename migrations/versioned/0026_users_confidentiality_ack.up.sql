-- Per-user confidentiality acknowledgement timestamp.
-- NULL  = the user has not yet confirmed the confidentiality statement;
--         the Web client should keep showing the acknowledgement dialog.
-- non-NULL = the user has confirmed; the dialog must not reappear on
--            subsequent logins, even after logout/relogin or browser reset.
-- Once set the timestamp is never cleared by the application.
ALTER TABLE users ADD COLUMN confidentiality_acknowledged_at TIMESTAMP WITH TIME ZONE;
CREATE INDEX IF NOT EXISTS idx_users_confidentiality_ack ON users(confidentiality_acknowledged_at);
