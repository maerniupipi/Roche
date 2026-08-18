DROP INDEX IF EXISTS idx_knowledge_bases_vector_store;
DROP INDEX IF EXISTS idx_vector_stores_name_global;

ALTER TABLE vector_stores
    ADD COLUMN knowledge_domain_id INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS idx_vector_stores_name_domain
    ON vector_stores(name, knowledge_domain_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_vector_stores_knowledge_domain_id
    ON vector_stores(knowledge_domain_id);
