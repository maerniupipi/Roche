CREATE TABLE public.suggested_questions (
    id VARCHAR(36) PRIMARY KEY,
    question TEXT NOT NULL,
    answer_mode VARCHAR(16) NOT NULL,
    custom_answer TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_suggested_questions_answer_mode
        CHECK (answer_mode IN ('generated', 'custom')),
    CONSTRAINT ck_suggested_questions_sort_order
        CHECK (sort_order BETWEEN 1 AND 3),
    CONSTRAINT ck_suggested_questions_custom_answer
        CHECK (answer_mode <> 'custom' OR LENGTH(TRIM(custom_answer)) > 0)
);

COMMENT ON TABLE public.suggested_questions IS
    'Three global homepage suggested questions configured by system administrators';
