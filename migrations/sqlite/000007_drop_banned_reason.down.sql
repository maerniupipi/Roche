-- Reverse: add back banned_reason column

CREATE TABLE users_new (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    avatar VARCHAR(500),
    is_system_admin BOOLEAN NOT NULL DEFAULT 0,
    status INTEGER NOT NULL DEFAULT 0,
    banned_reason TEXT,
    banned_at DATETIME,
    banned_by VARCHAR(36),
    role_knowledge_officer INTEGER NOT NULL DEFAULT 0,
    employee_id VARCHAR(64),
    account VARCHAR(100),
    english_name VARCHAR(100),
    chinese_name VARCHAR(100),
    department_code VARCHAR(64),
    department_name VARCHAR(255),
    preferences JSONB NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME
);

INSERT INTO users_new
SELECT
    id, username, email, password_hash, avatar,
    is_system_admin, status, '', banned_at, banned_by,
    role_knowledge_officer,
    employee_id, account, english_name, chinese_name,
    department_code, department_name, preferences,
    created_at, updated_at, deleted_at
FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_is_system_admin ON users(is_system_admin);
CREATE INDEX IF NOT EXISTS idx_users_role_knowledge_officer ON users(role_knowledge_officer);
CREATE INDEX IF NOT EXISTS idx_users_employee_id ON users(employee_id);
CREATE INDEX IF NOT EXISTS idx_users_account ON users(account);
CREATE INDEX IF NOT EXISTS idx_users_department_code ON users(department_code);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);
