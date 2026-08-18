CREATE TABLE message_feedbacks (
    id VARCHAR(36) PRIMARY KEY,
    message_id VARCHAR(36) NOT NULL
        REFERENCES messages(id) ON DELETE CASCADE,
    session_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(512) NOT NULL,
    rating VARCHAR(16) NOT NULL
        CHECK (rating IN ('like', 'dislike')),
    reason VARCHAR(32) NOT NULL DEFAULT ''
        CHECK (reason IN ('', 'factual_error', 'logic_confusion', 'outdated',
                          'format_error', 'too_long', 'repetitive', 'other')),
    comment TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (message_id)
);

CREATE INDEX idx_message_feedbacks_session
    ON message_feedbacks(session_id, updated_at DESC);
CREATE INDEX idx_message_feedbacks_user
    ON message_feedbacks(user_id, updated_at DESC);
CREATE INDEX idx_message_feedbacks_rating
    ON message_feedbacks(rating, updated_at DESC);
