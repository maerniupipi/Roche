-- Add role_operator column to users table (replaces previously-removed role_ops_admin).
-- role_operator: 0=否, 1=是

ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS role_operator smallint DEFAULT 0 NOT NULL;

COMMENT ON COLUMN public.users.role_operator IS '运维员角色: 0=否, 1=是';

CREATE INDEX IF NOT EXISTS idx_users_role_operator ON public.users(role_operator);
