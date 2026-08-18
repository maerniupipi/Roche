-- Drop removed columns: role_ops_admin, is_active, employee_type
-- These fields are now handled by status (active/banned/resigned) and role_knowledge_officer.

DROP INDEX IF EXISTS idx_users_role_ops_admin;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS role_ops_admin,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS employee_type;
