-- 黑名单表（SQLite 版本）
CREATE TABLE IF NOT EXISTS blacklist (
    id          TEXT     PRIMARY KEY,
    user_id     TEXT     NOT NULL UNIQUE,
    banned_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    banned_by   TEXT,
    reason      TEXT     DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_blacklist_user_id ON blacklist(user_id);
