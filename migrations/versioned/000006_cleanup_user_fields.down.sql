-- Reverse: add back role_ops_admin, is_active, employee_type columns

ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS is_active boolean DEFAULT true NOT NULL,
    ADD COLUMN IF NOT EXISTS role_ops_admin smallint DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS employee_type character varying(64);

COMMENT ON COLUMN public.users.is_active IS 'Whether the user is active';
COMMENT ON COLUMN public.users.role_ops_admin IS '运维管理员角色: 0=否, 1=是';
COMMENT ON COLUMN public.users.employee_type IS '员工类型，例如 Manager、Staff';

CREATE INDEX IF NOT EXISTS idx_users_role_ops_admin ON public.users(role_ops_admin);
