-- Clean SQLite baseline for repository tests.
-- Server deployments use PostgreSQL; both schemas expose the same business tables.

CREATE TABLE IF NOT EXISTS knowledge_domains (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    name_en VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT,
    status VARCHAR(50) DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_knowledge_domains_status ON knowledge_domains(status);

CREATE TABLE IF NOT EXISTS knowledge_domain_storage (
    knowledge_domain_id INTEGER PRIMARY KEY REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    storage_quota BIGINT NOT NULL DEFAULT 10737418240,
    storage_used BIGINT NOT NULL DEFAULT 0 CHECK (storage_used >= 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS platform_runtime_configs (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    retriever_engines TEXT NOT NULL DEFAULT '{"engines":[]}',
    context_config TEXT,
    web_search_config TEXT DEFAULT NULL,
    parser_engine_config TEXT DEFAULT NULL,
    storage_engine_config TEXT DEFAULT NULL,
    retrieval_config TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO platform_runtime_configs (id) VALUES (1);

CREATE TABLE IF NOT EXISTS models (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    type VARCHAR(50) NOT NULL,
    source VARCHAR(50) NOT NULL,
    description TEXT,
    parameters TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    is_builtin BOOLEAN NOT NULL DEFAULT 0,
    managed_by VARCHAR(32) NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_models_type ON models(type);
CREATE INDEX IF NOT EXISTS idx_models_source ON models(source);
CREATE INDEX IF NOT EXISTS idx_models_is_builtin ON models(is_builtin);
CREATE INDEX IF NOT EXISTS idx_models_managed_by ON models(managed_by);

CREATE TABLE IF NOT EXISTS knowledge_bases (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    knowledge_domain_id INTEGER NOT NULL,
    type VARCHAR(32) NOT NULL DEFAULT 'document',
    chunking_config TEXT NOT NULL DEFAULT '{"chunk_size": 512, "chunk_overlap": 50, "split_markers": ["\n\n", "\n", "。"], "keep_separator": true}',
    image_processing_config TEXT NOT NULL DEFAULT '{"enable_multimodal": false, "model_id": ""}',
    embedding_model_id VARCHAR(64) NOT NULL,
    summary_model_id VARCHAR(64) NOT NULL,
    cos_config TEXT NOT NULL DEFAULT '{}',
    storage_provider_config TEXT DEFAULT NULL,
    vlm_config TEXT NOT NULL DEFAULT '{}',
    extract_config TEXT NULL DEFAULT NULL,
    faq_config TEXT,
    question_generation_config TEXT NULL,
    is_temporary BOOLEAN NOT NULL DEFAULT 0,
    asr_config TEXT,
    vector_store_id VARCHAR(36),
    indexing_strategy TEXT,
    creator_id VARCHAR(36),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_knowledge_domain_id ON knowledge_bases(knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_knowledge_domain_vector_store
    ON knowledge_bases(knowledge_domain_id, vector_store_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_knowledge_domain_creator
    ON knowledge_bases(knowledge_domain_id, creator_id);

CREATE TABLE IF NOT EXISTS knowledges (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    source VARCHAR(2048) NOT NULL,
    parse_status VARCHAR(50) NOT NULL DEFAULT 'unprocessed',
    enable_status VARCHAR(50) NOT NULL DEFAULT 'enabled',
    embedding_model_id VARCHAR(64),
    file_name VARCHAR(255),
    file_type VARCHAR(50),
    file_size BIGINT,
    file_path TEXT,
    file_hash VARCHAR(64),
    storage_size BIGINT NOT NULL DEFAULT 0,
    metadata TEXT,
    tag_id VARCHAR(36),
    summary_status VARCHAR(32) DEFAULT 'none',
    last_faq_import_result TEXT DEFAULT NULL,
    channel VARCHAR(50) NOT NULL DEFAULT 'web',
    pending_subtasks_count INTEGER NOT NULL DEFAULT 0,
    folder_id VARCHAR(36),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME,
    error_message TEXT,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_knowledges_knowledge_domain_id ON knowledges(knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_knowledges_base_id ON knowledges(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledges_parse_status ON knowledges(parse_status);
CREATE INDEX IF NOT EXISTS idx_knowledges_enable_status ON knowledges(enable_status);
CREATE INDEX IF NOT EXISTS idx_knowledges_tag ON knowledges(tag_id);
CREATE INDEX IF NOT EXISTS idx_knowledges_summary_status ON knowledges(summary_status);

CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(36) PRIMARY KEY,
    title VARCHAR(255),
    description TEXT,
    last_request_state TEXT,
    user_id VARCHAR(512),
    is_pinned BOOLEAN NOT NULL DEFAULT 0,
    pinned_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_pin
    ON sessions (user_id, is_pinned, pinned_at, updated_at)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS messages (
    id VARCHAR(36) PRIMARY KEY,
    request_id VARCHAR(36) NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    role VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    rendered_content TEXT NOT NULL DEFAULT '',
    knowledge_references TEXT NOT NULL DEFAULT '[]',
    agent_steps TEXT DEFAULT NULL,
    mentioned_items TEXT DEFAULT '[]',
    images TEXT DEFAULT '[]',
    is_completed BOOLEAN NOT NULL DEFAULT 0,
    is_fallback BOOLEAN NOT NULL DEFAULT 0,
    channel VARCHAR(50) NOT NULL DEFAULT '',
    agent_duration_ms INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);

CREATE TABLE IF NOT EXISTS chunks (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    content TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT 1,
    start_at INTEGER NOT NULL,
    end_at INTEGER NOT NULL,
    pre_chunk_id VARCHAR(36),
    next_chunk_id VARCHAR(36),
    chunk_type VARCHAR(20) NOT NULL DEFAULT 'text',
    parent_chunk_id VARCHAR(36),
    image_info TEXT,
    video_info TEXT,
    relation_chunks TEXT,
    indirect_relation_chunks TEXT,
    metadata TEXT,
    tag_id VARCHAR(36),
    status INTEGER NOT NULL DEFAULT 0,
    content_hash VARCHAR(64),
    flags INTEGER NOT NULL DEFAULT 1,
    seq_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_chunks_knowledge_domain_kg ON chunks(knowledge_domain_id, knowledge_id);
CREATE INDEX IF NOT EXISTS idx_chunks_parent_id ON chunks(parent_chunk_id);
CREATE INDEX IF NOT EXISTS idx_chunks_chunk_type ON chunks(chunk_type);
CREATE INDEX IF NOT EXISTS idx_chunks_tag ON chunks(tag_id);
CREATE INDEX IF NOT EXISTS idx_chunks_content_hash ON chunks(content_hash);
CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_seq_id ON chunks(seq_id);
CREATE INDEX IF NOT EXISTS idx_chunks_kb_knowledge_domain ON chunks(knowledge_base_id, knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_chunks_knowledge_enabled ON chunks(knowledge_id, is_enabled, deleted_at);

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    avatar VARCHAR(500),
    is_active BOOLEAN NOT NULL DEFAULT 1,
    is_system_admin BOOLEAN NOT NULL DEFAULT 0,
    -- Per-user JSON preferences (memory toggle, future UI knobs).
    -- SQLite has no JSONB; store as TEXT and let GORM (de)serialise via
    -- the driver.Valuer / sql.Scanner methods on types.UserPreferences.
    preferences TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_is_system_admin ON users(is_system_admin);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

CREATE TABLE IF NOT EXISTS auth_tokens (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    token TEXT NOT NULL,
    token_type VARCHAR(50) NOT NULL,
    expires_at DATETIME NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_auth_tokens_user_id ON auth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_token ON auth_tokens(token);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_token_type ON auth_tokens(token_type);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_expires_at ON auth_tokens(expires_at);

-- Audit events can be platform-wide (knowledge_domain_id=0) or scoped to
-- one knowledge-management domain.
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL,
    actor_user_id VARCHAR(36) NOT NULL DEFAULT '',
    actor_role VARCHAR(32) NOT NULL DEFAULT '',
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(32) NOT NULL DEFAULT '',
    target_id VARCHAR(64) NOT NULL DEFAULT '',
    target_user_id VARCHAR(36) NOT NULL DEFAULT '',
    request_path VARCHAR(512) NOT NULL DEFAULT '',
    request_method VARCHAR(16) NOT NULL DEFAULT '',
    outcome VARCHAR(16) NOT NULL DEFAULT 'success',
    details TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_knowledge_domain_id_desc
    ON audit_logs(knowledge_domain_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor
    ON audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_knowledge_domain_action
    ON audit_logs(knowledge_domain_id, action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
    ON audit_logs(created_at);

-- user_resource_favorites — sqlite mirror of migration 000047. Same
-- composite PK (user_id, knowledge_domain_id, resource_type, resource_id) so the
-- GORM model and FirstOrCreate idempotency carry over.
CREATE TABLE IF NOT EXISTS user_resource_favorites (
    user_id VARCHAR(36) NOT NULL,
    resource_type VARCHAR(16) NOT NULL,
    resource_id VARCHAR(64) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, resource_type, resource_id)
);
CREATE INDEX IF NOT EXISTS idx_user_resource_favorites_user_type_created_at
    ON user_resource_favorites(user_id, resource_type, created_at DESC);

-- Per-user knowledge-base pin state. KnowledgeBase.is_pinned is an API
-- projection populated from this table, not a knowledge_bases column.
CREATE TABLE IF NOT EXISTS user_kb_pins (
    knowledge_domain_id INTEGER NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    kb_id VARCHAR(36) NOT NULL,
    pinned_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (knowledge_domain_id, user_id, kb_id)
);
CREATE INDEX IF NOT EXISTS idx_user_kb_pins_user_domain_pinned_at
    ON user_kb_pins(knowledge_domain_id, user_id, pinned_at DESC);

CREATE TABLE IF NOT EXISTS knowledge_tags (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(128) NOT NULL,
    color VARCHAR(32),
    sort_order INTEGER NOT NULL DEFAULT 0,
    seq_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_kb_name ON knowledge_tags(knowledge_domain_id, knowledge_base_id, name);
CREATE INDEX IF NOT EXISTS idx_knowledge_tags_kb ON knowledge_tags(knowledge_domain_id, knowledge_base_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_seq_id ON knowledge_tags(seq_id);

CREATE TABLE IF NOT EXISTS mcp_services (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT 1,
    transport_type VARCHAR(50) NOT NULL,
    url VARCHAR(512),
    headers TEXT,
    auth_config TEXT,
    advanced_config TEXT,
    stdio_config TEXT,
    env_vars TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_mcp_services_knowledge_domain_id ON mcp_services(knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_mcp_services_enabled ON mcp_services(enabled);
CREATE INDEX IF NOT EXISTS idx_mcp_services_is_builtin ON mcp_services(is_builtin);
CREATE INDEX IF NOT EXISTS idx_mcp_services_deleted_at ON mcp_services(deleted_at);

CREATE TABLE IF NOT EXISTS mcp_tool_approvals (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    service_id VARCHAR(36) NOT NULL,
    tool_name VARCHAR(512) NOT NULL,
    require_approval BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES mcp_services(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_tool_approvals_domain_service_tool ON mcp_tool_approvals(knowledge_domain_id, service_id, tool_name);
CREATE INDEX IF NOT EXISTS idx_mcp_tool_approvals_service_id ON mcp_tool_approvals(service_id);

CREATE TABLE IF NOT EXISTS mcp_oauth_clients (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    service_id VARCHAR(36) NOT NULL,
    client_id VARCHAR(512) NOT NULL,
    client_secret TEXT,
    redirect_uri VARCHAR(1024),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES mcp_services(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_oauth_clients_domain_service ON mcp_oauth_clients(knowledge_domain_id, service_id);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_clients_service_id ON mcp_oauth_clients(service_id);

CREATE TABLE IF NOT EXISTS mcp_oauth_tokens (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    principal_type VARCHAR(32) NOT NULL DEFAULT 'user',
    principal_id VARCHAR(128) NOT NULL,
    service_id VARCHAR(36) NOT NULL,
    access_token TEXT,
    refresh_token TEXT,
    token_type VARCHAR(32),
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES mcp_services(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_domain_principal_service
    ON mcp_oauth_tokens(knowledge_domain_id, principal_type, principal_id, service_id);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_service_id ON mcp_oauth_tokens(service_id);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_user_id ON mcp_oauth_tokens(user_id);

CREATE TABLE IF NOT EXISTS custom_agents (
    id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    avatar VARCHAR(64),
    is_builtin BOOLEAN NOT NULL DEFAULT 0,
    created_by VARCHAR(36),
    config TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_custom_agents_creator
    ON custom_agents(created_by, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_custom_agents_is_builtin ON custom_agents(is_builtin);
CREATE INDEX IF NOT EXISTS idx_custom_agents_deleted_at ON custom_agents(deleted_at);

CREATE TABLE IF NOT EXISTS sso_identities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL DEFAULT 'oidc',
    issuer VARCHAR(255) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sso_identities_provider_subject
    ON sso_identities(provider, issuer, subject);
CREATE INDEX IF NOT EXISTS idx_sso_identities_user ON sso_identities(user_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_bases_id_knowledge_domain
    ON knowledge_bases(id, knowledge_domain_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledges_id_kb_domain
    ON knowledges(id, knowledge_base_id, knowledge_domain_id);

CREATE TABLE IF NOT EXISTS knowledge_domain_admins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status = 'active'),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_domain_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_domain_admins_user
    ON knowledge_domain_admins(user_id, knowledge_domain_id);

CREATE TABLE IF NOT EXISTS org_units (
    id VARCHAR(36) PRIMARY KEY,
    parent_id VARCHAR(36) REFERENCES org_units(id) ON DELETE RESTRICT,
    code VARCHAR(128) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    source VARCHAR(32) NOT NULL DEFAULT 'manual'
        CHECK (source IN ('manual', 'workday', 'bootstrap')),
    external_id VARCHAR(255),
    sort_order INTEGER NOT NULL DEFAULT 0,
    attributes TEXT NOT NULL DEFAULT '{}',
    created_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_units_code_unique
    ON org_units(code)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_units_source_external_unique
    ON org_units(source, external_id)
    WHERE external_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_org_units_parent
    ON org_units(parent_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_org_units_status
    ON org_units(status)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_org_memberships (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_unit_id VARCHAR(36) NOT NULL REFERENCES org_units(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    source VARCHAR(32) NOT NULL DEFAULT 'manual'
        CHECK (source IN ('manual', 'workday', 'bootstrap')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, org_unit_id)
);

CREATE INDEX IF NOT EXISTS idx_user_org_memberships_org
    ON user_org_memberships(org_unit_id, status);
CREATE INDEX IF NOT EXISTS idx_user_org_memberships_user
    ON user_org_memberships(user_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_org_memberships_primary
    ON user_org_memberships(user_id)
    WHERE is_primary = 1 AND status = 'active';

CREATE TABLE IF NOT EXISTS knowledge_base_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL,
    subject_type VARCHAR(16) NOT NULL
        CHECK (subject_type IN ('user', 'org_unit')),
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read'
        CHECK (permission = 'read'),
    granted_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_base_id, subject_type, subject_id),
    FOREIGN KEY (knowledge_base_id, knowledge_domain_id)
        REFERENCES knowledge_bases(id, knowledge_domain_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_knowledge_base_grants_subject
    ON knowledge_base_grants(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_base_grants_domain
    ON knowledge_base_grants(knowledge_domain_id, knowledge_base_id);

CREATE TABLE IF NOT EXISTS knowledge_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    subject_type VARCHAR(16) NOT NULL
        CHECK (subject_type IN ('user', 'org_unit')),
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read'
        CHECK (permission = 'read'),
    granted_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_id, subject_type, subject_id),
    FOREIGN KEY (knowledge_base_id, knowledge_domain_id)
        REFERENCES knowledge_bases(id, knowledge_domain_id) ON DELETE CASCADE,
    FOREIGN KEY (knowledge_id, knowledge_base_id, knowledge_domain_id)
        REFERENCES knowledges(id, knowledge_base_id, knowledge_domain_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_knowledge_grants_subject
    ON knowledge_grants(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_grants_resource
    ON knowledge_grants(knowledge_domain_id, knowledge_base_id, knowledge_id);

CREATE TABLE IF NOT EXISTS data_sources (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    config TEXT,
    sync_schedule VARCHAR(100),
    sync_mode VARCHAR(20) DEFAULT 'incremental',
    status VARCHAR(32) DEFAULT 'active',
    conflict_strategy VARCHAR(32) DEFAULT 'overwrite',
    sync_deletions INTEGER DEFAULT 1,
    last_sync_at DATETIME NULL,
    last_sync_cursor TEXT,
    last_sync_result TEXT,
    error_message TEXT,
    sync_log_retention_days INTEGER DEFAULT 30,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

CREATE INDEX IF NOT EXISTS idx_data_sources_knowledge_domain_id ON data_sources (knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_data_sources_knowledge_base_id ON data_sources (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_data_sources_type ON data_sources (type);
CREATE INDEX IF NOT EXISTS idx_data_sources_status ON data_sources (status);
CREATE INDEX IF NOT EXISTS idx_data_sources_deleted_at ON data_sources (deleted_at);

CREATE TABLE IF NOT EXISTS sync_logs (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    data_source_id VARCHAR(36) NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    knowledge_domain_id INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME NULL,
    items_total INTEGER DEFAULT 0,
    items_created INTEGER DEFAULT 0,
    items_updated INTEGER DEFAULT 0,
    items_deleted INTEGER DEFAULT 0,
    items_skipped INTEGER DEFAULT 0,
    items_failed INTEGER DEFAULT 0,
    error_message TEXT,
    result TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sync_logs_data_source_id ON sync_logs (data_source_id);
CREATE INDEX IF NOT EXISTS idx_sync_logs_knowledge_domain_id ON sync_logs (knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_sync_logs_status ON sync_logs (status);
CREATE INDEX IF NOT EXISTS idx_sync_logs_started_at ON sync_logs (started_at);

CREATE TABLE IF NOT EXISTS web_search_providers (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    description TEXT,
    parameters TEXT,
    is_default INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

CREATE INDEX IF NOT EXISTS idx_web_search_providers_knowledge_domain_id ON web_search_providers (knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_web_search_providers_provider ON web_search_providers (provider);
CREATE INDEX IF NOT EXISTS idx_web_search_providers_deleted_at ON web_search_providers (deleted_at);

CREATE TABLE IF NOT EXISTS vector_stores (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    engine_type VARCHAR(50) NOT NULL,
    connection_config TEXT NOT NULL DEFAULT '{}',
    index_config TEXT NOT NULL DEFAULT '{}',
    knowledge_domain_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_vector_stores_name_domain
    ON vector_stores(name, knowledge_domain_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vector_stores_knowledge_domain_id ON vector_stores(knowledge_domain_id);
CREATE INDEX IF NOT EXISTS idx_vector_stores_engine_type ON vector_stores(engine_type);
CREATE INDEX IF NOT EXISTS idx_vector_stores_deleted_at ON vector_stores(deleted_at);

CREATE TABLE IF NOT EXISTS knowledge_folders (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL REFERENCES knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    parent_id VARCHAR(36) REFERENCES knowledge_folders(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    relative_path VARCHAR(2048) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_base_id, relative_path)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_domain_base
    ON knowledge_folders(knowledge_domain_id, knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_folders_parent
    ON knowledge_folders(parent_id);

CREATE TABLE IF NOT EXISTS knowledge_tag_relations (
    knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    tag_id VARCHAR(36) NOT NULL REFERENCES knowledge_tags(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (knowledge_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_ktr_knowledge ON knowledge_tag_relations(knowledge_id);
CREATE INDEX IF NOT EXISTS idx_ktr_tag ON knowledge_tag_relations(tag_id);

CREATE TABLE IF NOT EXISTS knowledge_processing_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_id VARCHAR(36) NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    span_id VARCHAR(64) NOT NULL,
    parent_span_id VARCHAR(64),
    name VARCHAR(64) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL,
    input TEXT,
    output TEXT,
    metadata TEXT,
    error_code VARCHAR(64),
    error_message TEXT,
    error_detail TEXT,
    started_at DATETIME,
    finished_at DATETIME,
    duration_ms BIGINT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_id, attempt, span_id)
);

CREATE INDEX IF NOT EXISTS idx_kpspan_knowledge_attempt
    ON knowledge_processing_spans(knowledge_id, attempt);
CREATE INDEX IF NOT EXISTS idx_kpspan_parent
    ON knowledge_processing_spans(parent_span_id)
    WHERE parent_span_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kpspan_status_started
    ON knowledge_processing_spans(status, started_at);

CREATE TABLE IF NOT EXISTS system_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key VARCHAR(128) NOT NULL UNIQUE,
    value TEXT NOT NULL,
    value_type VARCHAR(16) NOT NULL,
    category VARCHAR(32) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_secret BOOLEAN NOT NULL DEFAULT 0,
    requires_restart BOOLEAN NOT NULL DEFAULT 0,
    last_modified_by VARCHAR(36) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_system_settings_category ON system_settings(category);

CREATE TABLE IF NOT EXISTS task_dead_letters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL,
    task_type VARCHAR(64) NOT NULL,
    scope VARCHAR(32) NOT NULL,
    scope_id VARCHAR(64) NOT NULL,
    related_id VARCHAR(64) NOT NULL DEFAULT '',
    payload TEXT NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    fail_count INTEGER NOT NULL,
    failed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_dead_letters_domain
    ON task_dead_letters(knowledge_domain_id, failed_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_dead_letters_scope
    ON task_dead_letters(scope, scope_id, failed_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_dead_letters_task_type
    ON task_dead_letters(task_type, failed_at DESC);

CREATE TABLE IF NOT EXISTS external_org_units (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    external_org_id VARCHAR(255) NOT NULL,
    parent_external_org_id VARCHAR(255),
    org_unit_id VARCHAR(36) REFERENCES org_units(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    org_type VARCHAR(64),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    attributes TEXT NOT NULL DEFAULT '{}',
    checksum VARCHAR(64) NOT NULL,
    effective_from DATETIME,
    effective_to DATETIME,
    last_seen_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider, external_org_id)
);

CREATE INDEX IF NOT EXISTS idx_external_org_units_parent
    ON external_org_units(provider, parent_external_org_id);
CREATE INDEX IF NOT EXISTS idx_external_org_units_canonical
    ON external_org_units(org_unit_id);

CREATE TABLE IF NOT EXISTS external_workers (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    external_worker_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    primary_org_external_id VARCHAR(255),
    manager_external_worker_id VARCHAR(255),
    corporate_email VARCHAR(255),
    worker_status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (worker_status IN ('active', 'inactive', 'leave')),
    attributes TEXT NOT NULL DEFAULT '{}',
    checksum VARCHAR(64) NOT NULL,
    effective_from DATETIME,
    effective_to DATETIME,
    last_seen_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider, external_worker_id)
);

CREATE INDEX IF NOT EXISTS idx_external_workers_email
    ON external_workers(corporate_email COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_external_workers_user
    ON external_workers(user_id);
CREATE INDEX IF NOT EXISTS idx_external_workers_org
    ON external_workers(provider, primary_org_external_id);

CREATE TABLE IF NOT EXISTS integration_sync_runs (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    connection_key VARCHAR(128) NOT NULL,
    mode VARCHAR(20) NOT NULL DEFAULT 'incremental'
        CHECK (mode IN ('full', 'incremental')),
    cursor_before TEXT NOT NULL DEFAULT '{}',
    cursor_after TEXT NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    counters TEXT NOT NULL DEFAULT '{}',
    trace_id VARCHAR(128),
    error_code VARCHAR(64),
    error_summary TEXT,
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_integration_sync_runs_provider_created
    ON integration_sync_runs(provider, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_integration_sync_runs_status
    ON integration_sync_runs(status);

CREATE TABLE IF NOT EXISTS integration_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider VARCHAR(32) NOT NULL,
    external_event_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload_hash VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'received'
        CHECK (status IN ('received', 'processing', 'processed', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    trace_id VARCHAR(128),
    received_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME,
    error_summary TEXT,
    UNIQUE (provider, external_event_id)
);

CREATE INDEX IF NOT EXISTS idx_integration_events_status
    ON integration_events(provider, status, received_at);
