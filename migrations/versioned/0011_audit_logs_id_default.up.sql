-- Migration: 0011_audit_logs_id_default
-- Description: Add DEFAULT nextval() for audit_logs.id column.
-- The 000000_init migration created the sequence audit_logs_id_seq and
-- set OWNED BY, but omitted ALTER COLUMN ... SET DEFAULT, so GORM
-- INSERT (which omits the auto-increment column) fails with a NOT NULL
-- constraint violation. This adds the missing DEFAULT expression.
ALTER TABLE public.audit_logs ALTER COLUMN id SET DEFAULT nextval('public.audit_logs_id_seq'::regclass);
