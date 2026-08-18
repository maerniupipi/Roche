-- Restore the previous non-null email constraint.
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
