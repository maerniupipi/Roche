CREATE TABLE suggested_questions (
    id TEXT PRIMARY KEY,
    question TEXT NOT NULL,
    answer_mode TEXT NOT NULL CHECK (answer_mode IN ('generated', 'custom')),
    custom_answer TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL UNIQUE CHECK (sort_order BETWEEN 1 AND 3),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (answer_mode <> 'custom' OR LENGTH(TRIM(custom_answer)) > 0)
);
