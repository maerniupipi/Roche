DROP TABLE IF EXISTS knowledge_resource_grants;

CREATE TABLE knowledge_base_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    subject_type VARCHAR(16) NOT NULL,
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read',
    granted_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_base_id, subject_type, subject_id)
);

CREATE TABLE knowledge_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    subject_type VARCHAR(16) NOT NULL,
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read',
    granted_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_id, subject_type, subject_id)
);
