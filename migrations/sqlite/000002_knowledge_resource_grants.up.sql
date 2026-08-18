DROP TABLE IF EXISTS knowledge_grants;
DROP TABLE IF EXISTS knowledge_base_grants;

CREATE TABLE knowledge_resource_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL
        REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL
        REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    resource_type VARCHAR(24) NOT NULL
        CHECK (resource_type IN ('knowledge_base', 'folder', 'knowledge')),
    resource_id VARCHAR(36) NOT NULL,
    subject_type VARCHAR(16) NOT NULL
        CHECK (subject_type IN ('user', 'org_unit')),
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read'
        CHECK (permission IN ('read', 'manage')),
    effect VARCHAR(8) NOT NULL DEFAULT 'allow'
        CHECK (effect IN ('allow', 'deny')),
    inherit_to_children BOOLEAN NOT NULL DEFAULT 1,
    granted_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (
        knowledge_base_id,
        resource_type,
        resource_id,
        subject_type,
        subject_id,
        permission
    ),
    CHECK (resource_type <> 'knowledge_base' OR resource_id = knowledge_base_id)
);

CREATE INDEX idx_knowledge_resource_grants_subject
    ON knowledge_resource_grants(subject_type, subject_id);
CREATE INDEX idx_knowledge_resource_grants_resource
    ON knowledge_resource_grants(
        knowledge_domain_id,
        knowledge_base_id,
        resource_type,
        resource_id
    );
