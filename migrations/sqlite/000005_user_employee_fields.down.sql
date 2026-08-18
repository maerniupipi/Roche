-- Revert employee profile fields.
-- SQLite does not support DROP COLUMN in older versions; recreate the table
-- without the new columns if a true rollback is required.

DROP INDEX IF EXISTS idx_users_employee_id;
DROP INDEX IF EXISTS idx_users_account;
DROP INDEX IF EXISTS idx_users_department_code;
DROP INDEX IF EXISTS idx_users_english_name;
DROP INDEX IF EXISTS idx_users_chinese_name;
