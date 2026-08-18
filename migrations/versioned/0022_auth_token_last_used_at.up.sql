ALTER TABLE auth_tokens ADD COLUMN last_used_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_auth_tokens_last_used_at
    ON auth_tokens(last_used_at);
