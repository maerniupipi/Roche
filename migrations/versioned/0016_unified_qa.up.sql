-- This migration was originally numbered 0011 on the unified-QA feature
-- branch. Main already owns versions 0011-0015, so it must follow them.
-- IF NOT EXISTS keeps deployments safe where the feature table was created
-- before the migration histories were reconciled.
CREATE TABLE IF NOT EXISTS public.qa_execution_runs (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    request_id VARCHAR(64) NOT NULL DEFAULT '',
    user_message_id VARCHAR(36) NOT NULL DEFAULT '',
    assistant_message_id VARCHAR(36) NOT NULL DEFAULT '',
    user_id VARCHAR(36) NOT NULL,
    entry_agent_id VARCHAR(36) NOT NULL DEFAULT '',
    route_type VARCHAR(24) NOT NULL DEFAULT '',
    selected_agent_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(24) NOT NULL,
    original_query TEXT NOT NULL,
    rewritten_query TEXT NOT NULL DEFAULT '',
    config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    langfuse_trace_id VARCHAR(64) NOT NULL DEFAULT '',
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_qa_execution_runs_session_started
    ON public.qa_execution_runs(session_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_qa_execution_runs_user_started
    ON public.qa_execution_runs(user_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_qa_execution_runs_status
    ON public.qa_execution_runs(status);
CREATE INDEX IF NOT EXISTS idx_qa_execution_runs_langfuse_trace_id
    ON public.qa_execution_runs(langfuse_trace_id)
    WHERE langfuse_trace_id <> '';
