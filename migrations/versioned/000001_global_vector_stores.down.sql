DROP INDEX IF EXISTS public.idx_knowledge_bases_vector_store;
DROP INDEX IF EXISTS public.idx_vector_stores_name_global;

ALTER TABLE public.vector_stores
    ADD COLUMN IF NOT EXISTS knowledge_domain_id bigint NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS idx_vector_stores_name_domain
    ON public.vector_stores (name, knowledge_domain_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_vector_stores_knowledge_domain_id
    ON public.vector_stores (knowledge_domain_id);
