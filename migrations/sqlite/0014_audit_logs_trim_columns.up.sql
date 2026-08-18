-- Migration: 0014_audit_logs_trim_columns
-- Description: Trim the audit_logs table to the intended audit surface.
--   Removes request_path / request_method (moved into details JSONB) and
--   knowledge_domain_id. SQLite refuses DROP COLUMN while a column is
--   referenced by an index, so drop the indexes that reference
--   knowledge_domain_id first. Adds actor_name (操作人 name) column.
DROP INDEX IF EXISTS idx_audit_logs_domain;
DROP INDEX IF EXISTS idx_audit_logs_knowledge_domain_id_desc;
DROP INDEX IF EXISTS idx_audit_logs_knowledge_domain_action;
ALTER TABLE audit_logs DROP COLUMN request_path;
ALTER TABLE audit_logs DROP COLUMN request_method;
ALTER TABLE audit_logs DROP COLUMN knowledge_domain_id;
ALTER TABLE audit_logs ADD COLUMN actor_name VARCHAR(100) NOT NULL DEFAULT '';
