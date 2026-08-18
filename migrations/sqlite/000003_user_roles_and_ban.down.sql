DROP INDEX IF EXISTS idx_users_role_knowledge_officer;
DROP INDEX IF EXISTS idx_users_role_ops_admin;
DROP INDEX IF EXISTS idx_users_status;

-- SQLite does not support DROP COLUMN directly in older versions;
-- for test environments, a full re-create would be necessary.
-- This down migration is intentionally minimal; in production, PostgreSQL is used.
