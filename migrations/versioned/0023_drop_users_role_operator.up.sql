-- Drop role_operator column from users table.
-- The operator role is now expressed via is_system_admin
-- (operator endpoints map role_operator 1 -> is_system_admin=true, 0 -> false).

DROP INDEX IF EXISTS idx_users_role_operator;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS role_operator;
