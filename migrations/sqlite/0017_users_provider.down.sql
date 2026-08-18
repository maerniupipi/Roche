-- Revert the provider column.
DROP INDEX IF EXISTS idx_users_provider;
ALTER TABLE users DROP COLUMN provider;
