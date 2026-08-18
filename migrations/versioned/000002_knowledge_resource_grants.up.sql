DROP TABLE IF EXISTS public.knowledge_grants CASCADE;
DROP TABLE IF EXISTS public.knowledge_base_grants CASCADE;

CREATE TABLE public.knowledge_resource_grants (
    id BIGSERIAL PRIMARY KEY,
    knowledge_domain_id BIGINT NOT NULL
        REFERENCES public.knowledge_domains(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL
        REFERENCES public.knowledge_bases(id) ON DELETE CASCADE,
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
    inherit_to_children BOOLEAN NOT NULL DEFAULT TRUE,
    granted_by VARCHAR(36)
        REFERENCES public.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_knowledge_resource_grant UNIQUE (
        knowledge_base_id,
        resource_type,
        resource_id,
        subject_type,
        subject_id,
        permission
    ),
    CONSTRAINT ck_knowledge_resource_grant_kb_id CHECK (
        resource_type <> 'knowledge_base' OR resource_id = knowledge_base_id
    )
);

CREATE INDEX idx_knowledge_resource_grants_subject
    ON public.knowledge_resource_grants(subject_type, subject_id);
CREATE INDEX idx_knowledge_resource_grants_resource
    ON public.knowledge_resource_grants(
        knowledge_domain_id,
        knowledge_base_id,
        resource_type,
        resource_id
    );

COMMENT ON TABLE public.knowledge_resource_grants IS
    'Allow/deny ACL entries for knowledge bases, logical folders, and documents';
COMMENT ON COLUMN public.knowledge_resource_grants.resource_id IS
    'References knowledge_bases.id, knowledge_folders.id, or knowledges.id according to resource_type';
COMMENT ON COLUMN public.knowledge_resource_grants.inherit_to_children IS
    'Whether a knowledge-base or folder rule applies to descendant folders and documents';
