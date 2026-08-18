-- Drop role_operator column from users table.

DROP INDEX IF EXISTS idx_users_role_operator;

-- SQLite does not support DROP COLUMN directly in older versions.
-- This migration assumes SQLite 3.35.0+ which supports DROP COLUMN.
ALTER TABLE users
    DROP COLUMN role_operator;
