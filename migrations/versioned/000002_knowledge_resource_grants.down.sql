DROP TABLE IF EXISTS public.knowledge_resource_grants CASCADE;

CREATE TABLE public.knowledge_base_grants (
    id BIGSERIAL PRIMARY KEY,
    knowledge_domain_id BIGINT NOT NULL REFERENCES public.knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL REFERENCES public.knowledge_bases(id) ON DELETE CASCADE,
    subject_type VARCHAR(16) NOT NULL,
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read',
    granted_by VARCHAR(36) REFERENCES public.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_base_id, subject_type, subject_id)
);

CREATE TABLE public.knowledge_grants (
    id BIGSERIAL PRIMARY KEY,
    knowledge_domain_id BIGINT NOT NULL REFERENCES public.knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL REFERENCES public.knowledge_bases(id) ON DELETE CASCADE,
    knowledge_id VARCHAR(36) NOT NULL REFERENCES public.knowledges(id) ON DELETE CASCADE,
    subject_type VARCHAR(16) NOT NULL,
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL DEFAULT 'read',
    granted_by VARCHAR(36) REFERENCES public.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_id, subject_type, subject_id)
);
