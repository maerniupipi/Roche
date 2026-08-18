DROP INDEX IF EXISTS idx_vector_stores_name_domain;
DROP INDEX IF EXISTS idx_vector_stores_knowledge_domain_id;

ALTER TABLE vector_stores DROP COLUMN knowledge_domain_id;

CREATE INDEX IF NOT EXISTS idx_vector_stores_name_global
    ON vector_stores(name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_vector_store
    ON knowledge_bases(vector_store_id)
    WHERE vector_store_id IS NOT NULL AND deleted_at IS NULL;
