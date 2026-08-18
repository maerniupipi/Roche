-- Migration: 0011_audit_logs_client_ip (down)
-- Rollback: drop the dedicated client_ip column from audit_logs.
ALTER TABLE audit_logs DROP COLUMN client_ip;
