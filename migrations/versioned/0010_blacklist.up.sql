-- 黑名单表：用户拉黑后加入此表，解除拉黑后移出
-- 与 users.status 字段联动，作为独立拦截层
CREATE TABLE IF NOT EXISTS blacklist (
    id          VARCHAR(36)  PRIMARY KEY,
    user_id     VARCHAR(36)  NOT NULL UNIQUE,
    banned_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    banned_by   VARCHAR(36),
    reason      TEXT         DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_blacklist_user_id ON blacklist(user_id);
