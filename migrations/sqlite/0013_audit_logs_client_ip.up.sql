-- Migration: 0011_audit_logs_client_ip
-- Description: Add a dedicated client_ip column to audit_logs (SQLite mirror).
-- Mirrors versioned/0016_audit_logs_client_ip.up.sql for local SQLite dev.
ALTER TABLE audit_logs ADD COLUMN client_ip VARCHAR(64) NOT NULL DEFAULT '';
