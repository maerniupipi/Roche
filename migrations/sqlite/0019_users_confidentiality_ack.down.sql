-- Revert the confidentiality acknowledgement column.
DROP INDEX IF EXISTS idx_users_confidentiality_ack;
ALTER TABLE users DROP COLUMN confidentiality_acknowledged_at;
