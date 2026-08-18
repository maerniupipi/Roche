-- Add is_public column to knowledge_bases to support company-wide access
-- with per-user deny override via ACL grants.
ALTER TABLE public.knowledge_bases
    ADD COLUMN IF NOT EXISTS is_public boolean DEFAULT false NOT NULL;
