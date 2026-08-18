-- Migration: 0012_drop_audit_knowledge_domain_id (DOWN)
-- Description: Restore knowledge_domain_id column and indexes on audit_logs.

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS knowledge_domain_id bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_audit_logs_knowledge_domain_action ON public.audit_logs USING btree (knowledge_domain_id, action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_knowledge_domain_id_desc ON public.audit_logs USING btree (knowledge_domain_id, id DESC);
