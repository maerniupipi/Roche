-- Reverse: add back banned_reason column

ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS banned_reason text;

COMMENT ON COLUMN public.users.banned_reason IS '拉黑原因';
