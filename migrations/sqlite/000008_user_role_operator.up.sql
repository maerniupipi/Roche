-- Add role_operator column to users table (replaces previously-removed role_ops_admin).
-- role_operator: 0=否, 1=是

ALTER TABLE users
    ADD COLUMN role_operator INTEGER DEFAULT 0 NOT NULL;

CREATE INDEX IF NOT EXISTS idx_users_role_operator ON users(role_operator);
