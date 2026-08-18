CREATE TABLE IF NOT EXISTS dashboard_feedback (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    message_id VARCHAR(36) NOT NULL,
    category VARCHAR(64) NOT NULL,
    comment TEXT DEFAULT '' NOT NULL,
    satisfaction INTEGER DEFAULT 0 NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_dashboard_feedback_kd_created
    ON dashboard_feedback (knowledge_domain_id, created_at);
