-- Add employee profile fields to users for the user-management page.

ALTER TABLE users ADD COLUMN employee_id VARCHAR(64);
ALTER TABLE users ADD COLUMN account VARCHAR(100);
ALTER TABLE users ADD COLUMN english_name VARCHAR(100);
ALTER TABLE users ADD COLUMN chinese_name VARCHAR(100);
ALTER TABLE users ADD COLUMN employee_type VARCHAR(64);
ALTER TABLE users ADD COLUMN department_code VARCHAR(64);
ALTER TABLE users ADD COLUMN department_name VARCHAR(255);

CREATE INDEX IF NOT EXISTS idx_users_employee_id ON users(employee_id);
CREATE INDEX IF NOT EXISTS idx_users_account ON users(account);
CREATE INDEX IF NOT EXISTS idx_users_department_code ON users(department_code);
CREATE INDEX IF NOT EXISTS idx_users_english_name ON users(english_name);
CREATE INDEX IF NOT EXISTS idx_users_chinese_name ON users(chinese_name);
