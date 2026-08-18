-- Add user status, ban tracking, and role flags (ops_admin, knowledge_officer).
-- Uses tinyint integer states (0=否, 1=是) instead of boolean for expandability.

ALTER TABLE public.users
    ADD COLUMN status smallint DEFAULT 0 NOT NULL,
    ADD COLUMN banned_reason text,
    ADD COLUMN banned_at timestamptz,
    ADD COLUMN banned_by character varying(36),
    ADD COLUMN role_ops_admin smallint DEFAULT 0 NOT NULL,
    ADD COLUMN role_knowledge_officer smallint DEFAULT 0 NOT NULL;

COMMENT ON COLUMN public.users.status IS '用户状态: 0=正常, 1=已拉黑';
COMMENT ON COLUMN public.users.banned_reason IS '拉黑原因';
COMMENT ON COLUMN public.users.banned_at IS '拉黑时间';
COMMENT ON COLUMN public.users.banned_by IS '操作人 ID';
COMMENT ON COLUMN public.users.role_ops_admin IS '运维管理员角色: 0=否, 1=是';
COMMENT ON COLUMN public.users.role_knowledge_officer IS '知识官角色: 0=否, 1=是';

CREATE INDEX idx_users_status ON public.users(status);
CREATE INDEX idx_users_role_ops_admin ON public.users(role_ops_admin);
CREATE INDEX idx_users_role_knowledge_officer ON public.users(role_knowledge_officer);
