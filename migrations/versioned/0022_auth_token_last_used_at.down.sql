DROP INDEX IF EXISTS idx_auth_tokens_last_used_at;
ALTER TABLE auth_tokens DROP COLUMN IF EXISTS last_used_at;
