-- Migration: 0016_audit_logs_client_ip
-- Description: Add a dedicated client_ip column to audit_logs.
-- Previously the client IP only lived inside the details JSONB blob
-- (details->>'client_ip'), which made filtering/indexing by IP impossible
-- at the SQL layer. The new first-class column is populated alongside
-- details by GlobalAuditRecorder (http.request) and BusinessAuditRecorder
-- (login/login_failed), mirroring the value stored in details.
ALTER TABLE public.audit_logs
    ADD COLUMN IF NOT EXISTS client_ip character varying(64) NOT NULL DEFAULT '';
