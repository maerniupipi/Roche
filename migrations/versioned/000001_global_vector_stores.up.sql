DROP INDEX IF EXISTS public.idx_vector_stores_name_domain;
DROP INDEX IF EXISTS public.idx_vector_stores_knowledge_domain_id;

ALTER TABLE public.vector_stores
    DROP COLUMN IF EXISTS knowledge_domain_id;

CREATE INDEX IF NOT EXISTS idx_vector_stores_name_global
    ON public.vector_stores (name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_vector_store
    ON public.knowledge_bases (vector_store_id)
    WHERE vector_store_id IS NOT NULL AND deleted_at IS NULL;
