DROP INDEX IF EXISTS public.idx_users_role_knowledge_officer;
DROP INDEX IF EXISTS public.idx_users_role_ops_admin;
DROP INDEX IF EXISTS public.idx_users_status;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS role_knowledge_officer,
    DROP COLUMN IF EXISTS role_ops_admin,
    DROP COLUMN IF EXISTS banned_by,
    DROP COLUMN IF EXISTS banned_at,
    DROP COLUMN IF EXISTS banned_reason,
    DROP COLUMN IF EXISTS status;
