-- Reconcile the enterprise identity, organization and access-control schema.
-- Every statement is additive so an existing server database keeps its data.

CREATE TABLE IF NOT EXISTS public.org_units (
    id VARCHAR(36) PRIMARY KEY,
    parent_id VARCHAR(36),
    code VARCHAR(128) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    source VARCHAR(32) NOT NULL DEFAULT 'manual',
    external_id VARCHAR(255),
    sort_order INTEGER NOT NULL DEFAULT 0,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_org_units_source
        CHECK (source IN ('manual', 'workday', 'bootstrap')),
    CONSTRAINT chk_org_units_status
        CHECK (status IN ('active', 'inactive')),
    CONSTRAINT org_units_parent_id_fkey
        FOREIGN KEY (parent_id) REFERENCES public.org_units(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_units_code_unique
    ON public.org_units(code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_org_units_parent
    ON public.org_units(parent_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_units_source_external_unique
    ON public.org_units(source, external_id)
    WHERE external_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_org_units_status
    ON public.org_units(status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS public.user_org_memberships (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL
        REFERENCES public.users(id) ON DELETE CASCADE,
    org_unit_id VARCHAR(36) NOT NULL
        REFERENCES public.org_units(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    source VARCHAR(32) NOT NULL DEFAULT 'manual',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_user_org_membership_source
        CHECK (source IN ('manual', 'workday', 'bootstrap')),
    CONSTRAINT chk_user_org_membership_status
        CHECK (status IN ('active', 'inactive'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_org_memberships_user_org
    ON public.user_org_memberships(user_id, org_unit_id);
CREATE INDEX IF NOT EXISTS idx_user_org_memberships_org
    ON public.user_org_memberships(org_unit_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_org_memberships_primary
    ON public.user_org_memberships(user_id)
    WHERE is_primary = TRUE AND status = 'active';
CREATE INDEX IF NOT EXISTS idx_user_org_memberships_user
    ON public.user_org_memberships(user_id, status);

CREATE TABLE IF NOT EXISTS public.sso_identities (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL
        REFERENCES public.users(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL DEFAULT 'saml',
    issuer VARCHAR(255) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sso_identities_provider_subject
    ON public.sso_identities(provider, issuer, subject);
CREATE INDEX IF NOT EXISTS idx_sso_identities_user
    ON public.sso_identities(user_id);

CREATE TABLE IF NOT EXISTS public.external_org_units (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    external_org_id VARCHAR(255) NOT NULL,
    parent_external_org_id VARCHAR(255),
    org_unit_id VARCHAR(36)
        REFERENCES public.org_units(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    org_type VARCHAR(64),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    checksum VARCHAR(64) NOT NULL,
    effective_from TIMESTAMPTZ,
    effective_to TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_external_org_units_status
        CHECK (status IN ('active', 'inactive'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_external_org_units_provider_id
    ON public.external_org_units(provider, external_org_id);
CREATE INDEX IF NOT EXISTS idx_external_org_units_canonical
    ON public.external_org_units(org_unit_id);
CREATE INDEX IF NOT EXISTS idx_external_org_units_parent
    ON public.external_org_units(provider, parent_external_org_id);

CREATE TABLE IF NOT EXISTS public.external_workers (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    external_worker_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(36)
        REFERENCES public.users(id) ON DELETE SET NULL,
    primary_org_external_id VARCHAR(255),
    manager_external_worker_id VARCHAR(255),
    corporate_email VARCHAR(255),
    worker_status VARCHAR(20) NOT NULL DEFAULT 'active',
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    checksum VARCHAR(64) NOT NULL,
    effective_from TIMESTAMPTZ,
    effective_to TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_external_workers_status
        CHECK (worker_status IN ('active', 'inactive', 'leave'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_external_workers_provider_id
    ON public.external_workers(provider, external_worker_id);
CREATE INDEX IF NOT EXISTS idx_external_workers_email
    ON public.external_workers(lower(corporate_email));
CREATE INDEX IF NOT EXISTS idx_external_workers_org
    ON public.external_workers(provider, primary_org_external_id);
CREATE INDEX IF NOT EXISTS idx_external_workers_user
    ON public.external_workers(user_id);

CREATE TABLE IF NOT EXISTS public.integration_sync_runs (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    connection_key VARCHAR(128) NOT NULL,
    mode VARCHAR(20) NOT NULL DEFAULT 'incremental',
    cursor_before JSONB NOT NULL DEFAULT '{}'::jsonb,
    cursor_after JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    counters JSONB NOT NULL DEFAULT '{}'::jsonb,
    trace_id VARCHAR(128),
    error_code VARCHAR(64),
    error_summary TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_integration_sync_runs_mode
        CHECK (mode IN ('full', 'incremental')),
    CONSTRAINT chk_integration_sync_runs_status
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_integration_sync_runs_provider_created
    ON public.integration_sync_runs(provider, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_integration_sync_runs_status
    ON public.integration_sync_runs(status);

CREATE TABLE IF NOT EXISTS public.integration_events (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    external_event_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload_hash VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'received',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    trace_id VARCHAR(128),
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMPTZ,
    error_summary TEXT,
    CONSTRAINT chk_integration_events_status
        CHECK (status IN ('received', 'processing', 'processed', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_events_provider_id
    ON public.integration_events(provider, external_event_id);
CREATE INDEX IF NOT EXISTS idx_integration_events_status
    ON public.integration_events(provider, status, received_at);

CREATE TABLE IF NOT EXISTS public.knowledge_base_officers (
    id BIGSERIAL PRIMARY KEY,
    knowledge_base_id VARCHAR(36) NOT NULL
        REFERENCES public.knowledge_bases(id) ON DELETE CASCADE,
    user_id VARCHAR(36) NOT NULL
        REFERENCES public.users(id) ON DELETE CASCADE,
    granted_by VARCHAR(36)
        REFERENCES public.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_officer
    ON public.knowledge_base_officers(knowledge_base_id, user_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_base_officers_user
    ON public.knowledge_base_officers(user_id);

COMMENT ON TABLE public.knowledge_base_officers IS
    'Knowledge officers explicitly assigned to individual knowledge bases';
