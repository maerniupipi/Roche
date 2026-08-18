-- Drop banned_reason column from users table.
-- This field is no longer used; ban information is tracked via status + banned_at + banned_by.

ALTER TABLE public.users
    DROP COLUMN IF EXISTS banned_reason;
