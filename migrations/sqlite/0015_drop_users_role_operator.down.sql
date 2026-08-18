-- Restore role_operator column on users table.

ALTER TABLE users
    ADD COLUMN role_operator INTEGER DEFAULT 0 NOT NULL;

CREATE INDEX IF NOT EXISTS idx_users_role_operator ON users(role_operator);
