-- Migration: 0017_audit_logs_trim_columns (down)
-- Restore the dropped columns and remove actor_name.
ALTER TABLE public.audit_logs
    DROP COLUMN IF EXISTS actor_name,
    ADD COLUMN IF NOT EXISTS request_path character varying(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_method character varying(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS knowledge_domain_id bigint NOT NULL DEFAULT 0;
