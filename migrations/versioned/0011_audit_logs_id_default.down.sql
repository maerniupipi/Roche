-- Migration: 0011_audit_logs_id_default (rollback)
-- Remove the DEFAULT nextval() added in the up migration.
ALTER TABLE public.audit_logs ALTER COLUMN id DROP DEFAULT;
