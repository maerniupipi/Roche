-- Migration: 0016_audit_logs_client_ip (down)
-- Rollback: drop the dedicated client_ip column from audit_logs.
ALTER TABLE public.audit_logs
    DROP COLUMN IF EXISTS client_ip;
