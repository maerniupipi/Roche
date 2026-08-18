-- Allow users to be pre-created from an employee id only. The email column
-- becomes nullable and is back-filled when the user first signs in via SSO.
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
