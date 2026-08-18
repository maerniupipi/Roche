-- Migration: 0014_audit_logs_trim_columns (down)
-- Restore the dropped columns and remove actor_name.
ALTER TABLE audit_logs DROP COLUMN actor_name;
ALTER TABLE audit_logs ADD COLUMN request_path VARCHAR(512) NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN request_method VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN knowledge_domain_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_audit_logs_knowledge_domain_id_desc
    ON audit_logs(knowledge_domain_id, id DESC);
CREATE INDEX idx_audit_logs_knowledge_domain_action
    ON audit_logs(knowledge_domain_id, action);
