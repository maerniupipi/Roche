-- Track where each user account came from: "white_list" for accounts
-- pre-created by an administrator via POST /system/admin/users, "workday"
-- for accounts provisioned or managed by the Workday directory sync.
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider VARCHAR(32);
CREATE INDEX IF NOT EXISTS idx_users_provider ON users(provider);
