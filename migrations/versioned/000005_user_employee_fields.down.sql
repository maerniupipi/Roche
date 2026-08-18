-- Revert employee profile fields.

DROP INDEX IF EXISTS public.idx_users_employee_id;
DROP INDEX IF EXISTS public.idx_users_account;
DROP INDEX IF EXISTS public.idx_users_department_code;
DROP INDEX IF EXISTS public.idx_users_english_name;
DROP INDEX IF EXISTS public.idx_users_chinese_name;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS employee_id,
    DROP COLUMN IF EXISTS account,
    DROP COLUMN IF EXISTS english_name,
    DROP COLUMN IF EXISTS chinese_name,
    DROP COLUMN IF EXISTS employee_type,
    DROP COLUMN IF EXISTS department_code,
    DROP COLUMN IF EXISTS department_name;
