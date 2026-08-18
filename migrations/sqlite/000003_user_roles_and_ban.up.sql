-- Add user status, ban tracking, and role flags (ops_admin, knowledge_officer).
-- Uses integer states (0=否, 1=是) instead of boolean for expandability.

ALTER TABLE users ADD COLUMN status INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN banned_reason TEXT;
ALTER TABLE users ADD COLUMN banned_at DATETIME;
ALTER TABLE users ADD COLUMN banned_by VARCHAR(36);
ALTER TABLE users ADD COLUMN role_ops_admin INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN role_knowledge_officer INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_role_ops_admin ON users(role_ops_admin);
CREATE INDEX IF NOT EXISTS idx_users_role_knowledge_officer ON users(role_knowledge_officer);
