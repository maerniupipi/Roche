-- Add name_en column to knowledge_domains to support bilingual menu
-- titles (Chinese name + English name).
ALTER TABLE public.knowledge_domains
    ADD COLUMN IF NOT EXISTS name_en character varying(255) NOT NULL DEFAULT '';
