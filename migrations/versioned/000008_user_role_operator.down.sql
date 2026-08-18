-- Drop role_operator column from users table.

DROP INDEX IF EXISTS idx_users_role_operator;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS role_operator;
