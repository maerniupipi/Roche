-- Migration: 0017_audit_logs_trim_columns
-- Description: Trim the audit_logs table to the intended audit surface.
--   Removes request_path / request_method (moved into details JSONB as
--   操作详情) and knowledge_domain_id (already dropped by 0012, kept here
--   idempotently for databases created between 0012 and now where GORM
--   AutoMigrate re-added it). Adds actor_name (操作人 name) column.
ALTER TABLE public.audit_logs
    DROP COLUMN IF EXISTS request_path,
    DROP COLUMN IF EXISTS request_method,
    DROP COLUMN IF EXISTS knowledge_domain_id,
    ADD COLUMN IF NOT EXISTS actor_name character varying(100) NOT NULL DEFAULT '';
