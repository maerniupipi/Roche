-- Migration: 0012_drop_audit_knowledge_domain_id
-- Description: Drop knowledge_domain_id column and related indexes from audit_logs.
-- The column was used for knowledge-domain-scoped audit queries, but has been
-- removed in favour of a simpler flat audit log.

DROP INDEX IF EXISTS public.idx_audit_logs_knowledge_domain_action;
DROP INDEX IF EXISTS public.idx_audit_logs_knowledge_domain_id_desc;
ALTER TABLE public.audit_logs DROP COLUMN IF EXISTS knowledge_domain_id;
