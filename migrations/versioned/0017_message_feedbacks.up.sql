-- This migration was originally numbered 0012 on the unified-QA feature
-- branch. Main already owns versions 0011-0015, so it follows unified QA at
-- version 0017. IF NOT EXISTS supports databases that already have the table.
CREATE TABLE IF NOT EXISTS public.message_feedbacks (
    id VARCHAR(36) PRIMARY KEY,
    message_id VARCHAR(36) NOT NULL
        REFERENCES public.messages(id) ON DELETE CASCADE,
    session_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(512) NOT NULL,
    rating VARCHAR(16) NOT NULL,
    reason VARCHAR(32) NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_message_feedbacks_message UNIQUE (message_id),
    CONSTRAINT ck_message_feedbacks_rating CHECK (rating IN ('like', 'dislike')),
    CONSTRAINT ck_message_feedbacks_reason CHECK (
        reason IN ('', 'factual_error', 'logic_confusion', 'outdated',
                   'format_error', 'too_long', 'repetitive', 'other')
    )
);

CREATE INDEX IF NOT EXISTS idx_message_feedbacks_session
    ON public.message_feedbacks(session_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_feedbacks_user
    ON public.message_feedbacks(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_feedbacks_rating
    ON public.message_feedbacks(rating, updated_at DESC);

COMMENT ON TABLE public.message_feedbacks IS
    'Current user like/dislike feedback for assistant messages';
COMMENT ON COLUMN public.message_feedbacks.reason IS
    'Stable dislike reason key; empty for likes';
