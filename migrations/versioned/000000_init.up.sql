-- Roche Knowledge Agent Platform clean PostgreSQL baseline.
-- Fresh installations only: no legacy workspace compatibility schema is retained.

--
--



SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;


--
--

COMMENT ON EXTENSION pg_trgm IS 'text similarity measurement and index searching based on trigrams';


--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: unify_prompt_placeholder(text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.unify_prompt_placeholder(input text) RETURNS text
    LANGUAGE plpgsql
    AS $$
DECLARE
    result TEXT := COALESCE(input, '');
    replacements TEXT[][] := ARRAY[
        -- Go template variables -> simple placeholders
        ['{{.Query}}', '{{query}}'],
        ['{{.Answer}}', '{{answer}}'],
        ['{{.CurrentTime}}', '{{current_time}}'],
        ['{{.CurrentWeek}}', '{{current_week}}'],
        ['{{.Yesterday}}', '{{yesterday}}'],
        ['{{.Contexts}}', '{{contexts}}'],
        -- Go template control structures -> simple placeholders or remove
        ['{{range .Contexts}}', '{{contexts}}'],
        -- Remove Go template syntax
        ['{{if .Contexts}}', ''],
        ['{{else}}', ''],
        ['{{.}}', '']
    ];
    r TEXT[];
BEGIN
    FOREACH r SLICE 1 IN ARRAY replacements LOOP
        result := REPLACE(result, r[1], r[2]);
    END LOOP;
    -- Handle {{range .Conversation}}...{{end}} block specially
    -- Replace the entire block with just {{conversation}}
    -- The pattern matches: {{range .Conversation}} followed by any content until {{end}}
    result := regexp_replace(
        result,
        '\{\{range \.Conversation\}\}[\s\S]*?\{\{end\}\}',
        '{{conversation}}',
        'g'
    );
    -- Clean up any remaining {{end}} tags
    result := REPLACE(result, '{{end}}', '');
    RETURN result;
END;
$$;


--
-- Name: update_mcp_services_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.update_mcp_services_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_logs (
    id bigint NOT NULL,
    knowledge_domain_id bigint NOT NULL,
    actor_user_id character varying(36) DEFAULT ''::character varying NOT NULL,
    actor_role character varying(32) DEFAULT ''::character varying NOT NULL,
    action character varying(64) NOT NULL,
    target_type character varying(32) DEFAULT ''::character varying NOT NULL,
    target_id character varying(64) DEFAULT ''::character varying NOT NULL,
    target_user_id character varying(36) DEFAULT ''::character varying NOT NULL,
    request_path character varying(512) DEFAULT ''::character varying NOT NULL,
    request_method character varying(16) DEFAULT ''::character varying NOT NULL,
    outcome character varying(16) DEFAULT 'success'::character varying NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: audit_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audit_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audit_logs_id_seq OWNED BY public.audit_logs.id;


--
-- Name: auth_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_tokens (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    user_id character varying(36) NOT NULL,
    token text NOT NULL,
    token_type character varying(50) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    is_revoked boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
--

COMMENT ON TABLE public.auth_tokens IS 'Authentication tokens for users';


--
--

COMMENT ON COLUMN public.auth_tokens.id IS 'Unique identifier of the token';


--
--

COMMENT ON COLUMN public.auth_tokens.user_id IS 'User ID that owns this token';


--
--

COMMENT ON COLUMN public.auth_tokens.token IS 'Token value (JWT or other format)';


--
--

COMMENT ON COLUMN public.auth_tokens.token_type IS 'Token type (access_token, refresh_token)';


--
--

COMMENT ON COLUMN public.auth_tokens.expires_at IS 'Token expiration time';


--
--

COMMENT ON COLUMN public.auth_tokens.is_revoked IS 'Whether the token is revoked';


--
-- Name: chunks_seq_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chunks_seq_id_seq
    START WITH 100000000
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chunks (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    knowledge_domain_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    knowledge_id character varying(36) NOT NULL,
    content text NOT NULL,
    chunk_index integer NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    start_at integer NOT NULL,
    end_at integer NOT NULL,
    pre_chunk_id character varying(36),
    next_chunk_id character varying(36),
    chunk_type character varying(20) DEFAULT 'text'::character varying NOT NULL,
    parent_chunk_id character varying(36),
    image_info text,
    relation_chunks jsonb,
    indirect_relation_chunks jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    metadata jsonb,
    tag_id character varying(36),
    status integer DEFAULT 0 NOT NULL,
    content_hash character varying(64),
    flags integer DEFAULT 1 NOT NULL,
    seq_id bigint DEFAULT nextval('public.chunks_seq_id_seq'::regclass) NOT NULL,
    video_info text
);


--
--

COMMENT ON COLUMN public.chunks.video_info IS 'Video information in JSON format: {"url": string, "frame_count": int, "has_vlm_analysis": bool, "has_asr": bool, "video_summary": string, "asr_text": string, "frame_descriptions": string[]}';


--
-- Name: custom_agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_agents (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    avatar character varying(64),
    is_builtin boolean DEFAULT false NOT NULL,
    created_by character varying(36),
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone
);


--
-- Name: data_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_sources (
    id character varying(36) NOT NULL,
    knowledge_domain_id bigint NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    name character varying(255) NOT NULL,
    type character varying(50) NOT NULL,
    config jsonb,
    sync_schedule character varying(100),
    sync_mode character varying(20) DEFAULT 'incremental'::character varying,
    status character varying(32) DEFAULT 'active'::character varying,
    conflict_strategy character varying(32) DEFAULT 'overwrite'::character varying,
    sync_deletions boolean DEFAULT true,
    last_sync_at timestamp without time zone,
    last_sync_cursor jsonb,
    last_sync_result jsonb,
    error_message text,
    sync_log_retention_days integer DEFAULT 30,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);


--
-- Name: external_org_units; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.external_org_units (
    id character varying(36) NOT NULL,
    provider character varying(32) NOT NULL,
    external_org_id character varying(255) NOT NULL,
    parent_external_org_id character varying(255),
    org_unit_id character varying(36),
    name character varying(255) NOT NULL,
    org_type character varying(64),
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    attributes jsonb DEFAULT '{}'::jsonb NOT NULL,
    checksum character varying(64) NOT NULL,
    effective_from timestamp with time zone,
    effective_to timestamp with time zone,
    last_seen_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_external_org_units_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.external_org_units IS 'External directory organization projection mapped to canonical org_units';


--
-- Name: external_workers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.external_workers (
    id character varying(36) NOT NULL,
    provider character varying(32) NOT NULL,
    external_worker_id character varying(255) NOT NULL,
    user_id character varying(36),
    primary_org_external_id character varying(255),
    manager_external_worker_id character varying(255),
    corporate_email character varying(255),
    worker_status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    attributes jsonb DEFAULT '{}'::jsonb NOT NULL,
    checksum character varying(64) NOT NULL,
    effective_from timestamp with time zone,
    effective_to timestamp with time zone,
    last_seen_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_external_workers_status CHECK (((worker_status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying, 'leave'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.external_workers IS 'External worker projection mapped to local users; email is only a matching aid';


--
-- Name: integration_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.integration_events (
    id bigint NOT NULL,
    provider character varying(32) NOT NULL,
    external_event_id character varying(255) NOT NULL,
    event_type character varying(128) NOT NULL,
    payload_hash character varying(64) NOT NULL,
    status character varying(20) DEFAULT 'received'::character varying NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    trace_id character varying(128),
    received_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    processed_at timestamp with time zone,
    error_summary text,
    CONSTRAINT chk_integration_events_status CHECK (((status)::text = ANY ((ARRAY['received'::character varying, 'processing'::character varying, 'processed'::character varying, 'failed'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.integration_events IS 'Idempotency and audit envelope for external events; raw sensitive payload is not stored';


--
-- Name: integration_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.integration_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: integration_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.integration_events_id_seq OWNED BY public.integration_events.id;


--
-- Name: integration_sync_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.integration_sync_runs (
    id character varying(36) NOT NULL,
    provider character varying(32) NOT NULL,
    connection_key character varying(128) NOT NULL,
    mode character varying(20) DEFAULT 'incremental'::character varying NOT NULL,
    cursor_before jsonb DEFAULT '{}'::jsonb NOT NULL,
    cursor_after jsonb DEFAULT '{}'::jsonb NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    counters jsonb DEFAULT '{}'::jsonb NOT NULL,
    trace_id character varying(128),
    error_code character varying(64),
    error_summary text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_integration_sync_runs_mode CHECK (((mode)::text = ANY ((ARRAY['full'::character varying, 'incremental'::character varying])::text[]))),
    CONSTRAINT chk_integration_sync_runs_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'running'::character varying, 'succeeded'::character varying, 'failed'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.integration_sync_runs IS 'Cursor, counters and terminal state for enterprise integration synchronization';


--
-- Name: knowledge_base_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_base_grants (
    id bigint NOT NULL,
    knowledge_domain_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    subject_type character varying(16) NOT NULL,
    subject_id character varying(36) NOT NULL,
    permission character varying(16) DEFAULT 'read'::character varying NOT NULL,
    granted_by character varying(36),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_knowledge_base_grant_permission CHECK (((permission)::text = 'read'::text)),
    CONSTRAINT chk_knowledge_base_grant_subject CHECK (((subject_type)::text = ANY ((ARRAY['user'::character varying, 'org_unit'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.knowledge_base_grants IS 'Explicit full knowledge-base read grants for a user or organization unit';


--
--

COMMENT ON COLUMN public.knowledge_base_grants.subject_id IS 'References users.id when subject_type=user, otherwise org_units.id';


--
-- Name: knowledge_base_grants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.knowledge_base_grants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: knowledge_base_grants_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.knowledge_base_grants_id_seq OWNED BY public.knowledge_base_grants.id;


--
-- Name: knowledge_bases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_bases (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    knowledge_domain_id integer NOT NULL,
    chunking_config jsonb DEFAULT '{"chunk_size": 512, "chunk_overlap": 50, "split_markers": ["\n\n", "\n", "。"], "keep_separator": true}'::jsonb NOT NULL,
    image_processing_config jsonb DEFAULT '{"model_id": "", "enable_multimodal": false}'::jsonb NOT NULL,
    embedding_model_id character varying(64) NOT NULL,
    summary_model_id character varying(64) NOT NULL,
    cos_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    vlm_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    extract_config jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    is_temporary boolean DEFAULT false NOT NULL,
    type character varying(32) DEFAULT 'document'::character varying NOT NULL,
    faq_config jsonb,
    question_generation_config jsonb,
    storage_provider_config jsonb,
    asr_config jsonb,
    vector_store_id character varying(36),
    indexing_strategy jsonb,
    creator_id character varying(36)
);


--
--

COMMENT ON COLUMN public.knowledge_bases.is_temporary IS 'Whether this knowledge base is temporary (ephemeral) and should be hidden from UI';


--
--

COMMENT ON COLUMN public.knowledge_bases.storage_provider_config IS 'Storage provider config for this knowledge base. Only the provider name is stored; credentials come from the platform storage configuration.';


--
--

COMMENT ON COLUMN public.knowledge_bases.asr_config IS 'ASR (Automatic Speech Recognition) configuration: {"enabled": bool, "model_id": string, "language": string}';


--
--

COMMENT ON COLUMN public.knowledge_bases.vector_store_id IS 'References vector_stores.id. NULL means the platform default derived from RETRIEVE_DRIVER. Immutable after creation (enforced at ORM and service layer). No FK by design.';


--
--

COMMENT ON COLUMN public.knowledge_bases.indexing_strategy IS 'Indexing pipeline strategy: {"vector_enabled": bool, "keyword_enabled": bool, "graph_enabled": bool}';


--
-- Name: knowledge_domain_admins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_domain_admins (
    id bigint NOT NULL,
    knowledge_domain_id integer NOT NULL,
    user_id character varying(36) NOT NULL,
    granted_by character varying(36),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
--

COMMENT ON TABLE public.knowledge_domain_admins IS 'Users allowed to manage knowledge bases and grants in a knowledge domain';


--
-- Name: knowledge_domain_admins_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.knowledge_domain_admins_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: knowledge_domain_admins_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.knowledge_domain_admins_id_seq OWNED BY public.knowledge_domain_admins.id;


--
-- Name: knowledge_domain_storage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_domain_storage (
    knowledge_domain_id integer NOT NULL,
    storage_quota bigint DEFAULT '10737418240'::bigint NOT NULL,
    storage_used bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT knowledge_domain_storage_storage_used_check CHECK ((storage_used >= 0))
);


--
--

COMMENT ON TABLE public.knowledge_domain_storage IS 'Per-knowledge-domain storage quota and usage accounting';


--
-- Name: knowledge_domains; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_domains (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    name_en character varying(255) NOT NULL DEFAULT ''::character varying,
    description text,
    status character varying(50) DEFAULT 'active'::character varying,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    code character varying(64) NOT NULL
);


--
--

COMMENT ON TABLE public.knowledge_domains IS 'Knowledge-management grouping only; independent from the enterprise organization tree';


--
--

COMMENT ON COLUMN public.knowledge_domains.code IS 'Stable application-facing code for the knowledge domain';


--
-- Name: knowledge_domains_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.knowledge_domains_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: knowledge_domains_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.knowledge_domains_id_seq OWNED BY public.knowledge_domains.id;


--
-- Name: knowledge_folders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_folders (
    id character varying(36) NOT NULL,
    knowledge_domain_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    parent_id character varying(36),
    name character varying(255) NOT NULL,
    relative_path character varying(2048) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
--

COMMENT ON TABLE public.knowledge_folders IS 'Folder hierarchy used to organize documents inside a knowledge base.';


--
--

COMMENT ON COLUMN public.knowledge_folders.relative_path IS 'Normalized slash-separated path relative to the knowledge-base root.';


--
-- Name: knowledge_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_grants (
    id bigint NOT NULL,
    knowledge_domain_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    knowledge_id character varying(36) NOT NULL,
    subject_type character varying(16) NOT NULL,
    subject_id character varying(36) NOT NULL,
    permission character varying(16) DEFAULT 'read'::character varying NOT NULL,
    granted_by character varying(36),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_knowledge_grant_permission CHECK (((permission)::text = 'read'::text)),
    CONSTRAINT chk_knowledge_grant_subject CHECK (((subject_type)::text = ANY ((ARRAY['user'::character varying, 'org_unit'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.knowledge_grants IS 'Explicit single-document read grants for a user or organization unit';


--
--

COMMENT ON COLUMN public.knowledge_grants.subject_id IS 'References users.id when subject_type=user, otherwise org_units.id';


--
-- Name: knowledge_grants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.knowledge_grants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: knowledge_grants_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.knowledge_grants_id_seq OWNED BY public.knowledge_grants.id;


--
-- Name: knowledge_processing_spans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_processing_spans (
    id bigint NOT NULL,
    knowledge_id character varying(64) NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    span_id character varying(64) NOT NULL,
    parent_span_id character varying(64),
    name character varying(64) NOT NULL,
    kind character varying(16) NOT NULL,
    status character varying(16) NOT NULL,
    input jsonb,
    output jsonb,
    metadata jsonb,
    error_code character varying(64),
    error_message text,
    error_detail text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    duration_ms bigint,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: knowledge_processing_spans_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.knowledge_processing_spans_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: knowledge_processing_spans_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.knowledge_processing_spans_id_seq OWNED BY public.knowledge_processing_spans.id;


--
-- Name: knowledge_tag_relations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_tag_relations (
    knowledge_id character varying(36) NOT NULL,
    tag_id character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT now()
);


--
-- Name: knowledge_tags_seq_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.knowledge_tags_seq_id_seq
    START WITH 10000000
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: knowledge_tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_tags (
    id character varying(36) NOT NULL,
    knowledge_domain_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    name character varying(128) NOT NULL,
    color character varying(32),
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    seq_id bigint DEFAULT nextval('public.knowledge_tags_seq_id_seq'::regclass) NOT NULL
);


--
-- Name: knowledges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledges (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    knowledge_domain_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    type character varying(50) NOT NULL,
    title character varying(255) NOT NULL,
    description text,
    source character varying(2048) NOT NULL,
    parse_status character varying(50) DEFAULT 'unprocessed'::character varying NOT NULL,
    enable_status character varying(50) DEFAULT 'enabled'::character varying NOT NULL,
    embedding_model_id character varying(64),
    file_name character varying(255),
    file_type character varying(50),
    file_size bigint,
    file_path text,
    file_hash character varying(64),
    storage_size bigint DEFAULT 0 NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    processed_at timestamp with time zone,
    error_message text,
    deleted_at timestamp with time zone,
    summary_status character varying(32) DEFAULT 'none'::character varying,
    last_faq_import_result json,
    channel character varying(50) DEFAULT 'web'::character varying NOT NULL,
    pending_subtasks_count integer DEFAULT 0 NOT NULL,
    folder_id character varying(36)
);


--
--

COMMENT ON COLUMN public.knowledges.channel IS 'Source channel of the knowledge: web, api, browser_extension, wechat, etc.';


--
--

COMMENT ON COLUMN public.knowledges.folder_id IS 'Folder containing this knowledge document; NULL means knowledge-base root.';


--
-- Name: mcp_oauth_clients; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mcp_oauth_clients (
    id character varying(36) NOT NULL,
    knowledge_domain_id integer NOT NULL,
    service_id character varying(36) NOT NULL,
    client_id character varying(512) NOT NULL,
    client_secret text,
    redirect_uri character varying(1024),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mcp_oauth_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mcp_oauth_tokens (
    id character varying(36) NOT NULL,
    knowledge_domain_id integer NOT NULL,
    user_id character varying(512) NOT NULL,
    service_id character varying(36) NOT NULL,
    access_token text,
    refresh_token text,
    token_type character varying(32),
    expires_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    principal_type character varying(32) NOT NULL,
    principal_id character varying(512) NOT NULL
);


--
-- Name: mcp_services; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mcp_services (
    id character varying(36) NOT NULL,
    knowledge_domain_id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    enabled boolean DEFAULT true,
    transport_type character varying(50) NOT NULL,
    url character varying(512),
    headers jsonb,
    auth_config jsonb,
    advanced_config jsonb,
    stdio_config jsonb,
    env_vars jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone,
    is_builtin boolean DEFAULT false NOT NULL
);


--
--

COMMENT ON TABLE public.mcp_services IS 'MCP service configurations';


--
-- Name: mcp_tool_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mcp_tool_approvals (
    id character varying(36) NOT NULL,
    knowledge_domain_id integer NOT NULL,
    service_id character varying(36) NOT NULL,
    tool_name character varying(512) NOT NULL,
    require_approval boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.messages (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    request_id character varying(36) NOT NULL,
    session_id character varying(36) NOT NULL,
    role character varying(50) NOT NULL,
    content text NOT NULL,
    knowledge_references jsonb DEFAULT '[]'::jsonb NOT NULL,
    agent_steps jsonb,
    is_completed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    mentioned_items jsonb DEFAULT '[]'::jsonb,
    is_fallback boolean DEFAULT false,
    agent_duration_ms bigint DEFAULT 0,
    images jsonb DEFAULT '[]'::jsonb,
    channel character varying(50) DEFAULT ''::character varying NOT NULL,
    rendered_content text DEFAULT ''::text NOT NULL,
    attachments jsonb DEFAULT '[]'::jsonb
);


--
--

COMMENT ON COLUMN public.messages.agent_steps IS 'Agent execution steps (reasoning process and tool calls)';


--
--

COMMENT ON COLUMN public.messages.mentioned_items IS 'Stores @mentioned knowledge bases and files (id, name, type) when user sends a message';


--
--

COMMENT ON COLUMN public.messages.channel IS 'Source channel of the message: web, api, im, etc.';


--
--

COMMENT ON COLUMN public.messages.rendered_content IS 'Full RAG-augmented user message sent to LLM, preserving retrieval context across turns';


--
-- Name: models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.models (
    id character varying(64) DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    display_name character varying(255) DEFAULT ''::character varying NOT NULL,
    type character varying(50) NOT NULL,
    source character varying(50) NOT NULL,
    description text,
    parameters jsonb NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    status character varying(50) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    is_builtin boolean DEFAULT false NOT NULL,
    managed_by character varying(32) DEFAULT ''::character varying NOT NULL
);


--
-- Name: org_units; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.org_units (
    id character varying(36) NOT NULL,
    parent_id character varying(36),
    code character varying(128) NOT NULL,
    name character varying(255) NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    source character varying(32) DEFAULT 'manual'::character varying NOT NULL,
    external_id character varying(255),
    sort_order integer DEFAULT 0 NOT NULL,
    attributes jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by character varying(36),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT chk_org_units_source CHECK (((source)::text = ANY ((ARRAY['manual'::character varying, 'workday'::character varying, 'bootstrap'::character varying])::text[]))),
    CONSTRAINT chk_org_units_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.org_units IS 'Enterprise organization tree projection, independent from knowledge-management domains';


--
-- Name: platform_runtime_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_runtime_configs (
    id smallint DEFAULT 1 NOT NULL,
    retriever_engines jsonb DEFAULT '{"engines": []}'::jsonb NOT NULL,
    context_config jsonb,
    web_search_config jsonb,
    parser_engine_config jsonb,
    storage_engine_config jsonb,
    retrieval_config jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT platform_runtime_configs_id_check CHECK ((id = 1))
);


--
--

COMMENT ON TABLE public.platform_runtime_configs IS 'Singleton platform-wide parser, storage, web-search and retrieval configuration';


--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    title character varying(255),
    description text,
    last_request_state jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    user_id character varying(512),
    is_pinned boolean DEFAULT false NOT NULL,
    pinned_at timestamp with time zone
);


--
--

COMMENT ON COLUMN public.sessions.last_request_state IS 'Last chat input UI state; not an authorization input';


--
-- Name: sso_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sso_identities (
    id bigint NOT NULL,
    user_id character varying(36) NOT NULL,
    provider character varying(64) DEFAULT 'oidc'::character varying NOT NULL,
    issuer character varying(255) NOT NULL,
    subject character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_login_at timestamp with time zone
);


--
--

COMMENT ON TABLE public.sso_identities IS 'Stable enterprise SSO subject to local user mapping';


--
-- Name: sso_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sso_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sso_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sso_identities_id_seq OWNED BY public.sso_identities.id;


--
-- Name: sync_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sync_logs (
    id character varying(36) NOT NULL,
    data_source_id character varying(36) NOT NULL,
    knowledge_domain_id bigint NOT NULL,
    status character varying(32) NOT NULL,
    started_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    finished_at timestamp without time zone,
    items_total integer DEFAULT 0,
    items_created integer DEFAULT 0,
    items_updated integer DEFAULT 0,
    items_deleted integer DEFAULT 0,
    items_skipped integer DEFAULT 0,
    items_failed integer DEFAULT 0,
    error_message text,
    result jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_settings (
    id bigint NOT NULL,
    key character varying(128) NOT NULL,
    value jsonb NOT NULL,
    value_type character varying(16) NOT NULL,
    category character varying(32) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_secret boolean DEFAULT false NOT NULL,
    requires_restart boolean DEFAULT false NOT NULL,
    last_modified_by character varying(36) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: system_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.system_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.system_settings_id_seq OWNED BY public.system_settings.id;


--
-- Name: task_dead_letters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_dead_letters (
    id bigint NOT NULL,
    knowledge_domain_id bigint NOT NULL,
    task_type character varying(64) NOT NULL,
    scope character varying(32) NOT NULL,
    scope_id character varying(64) NOT NULL,
    related_id character varying(64) DEFAULT ''::character varying NOT NULL,
    payload jsonb NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    fail_count integer NOT NULL,
    failed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
--

COMMENT ON TABLE public.task_dead_letters IS 'Permanent archive of asynchronous tasks that exhausted retries.';


--
--

COMMENT ON COLUMN public.task_dead_letters.related_id IS 'Optional secondary identifier. Wiki ingest puts knowledge_id here so retract/ingest dead letters cluster around the source document.';


--
--

COMMENT ON COLUMN public.task_dead_letters.payload IS 'Raw task payload (asynq.Task.Payload) at the time of failure. Allows manual requeue via SQL + asynq.Client.Enqueue.';


--
--

COMMENT ON COLUMN public.task_dead_letters.last_error IS 'String form of the error that caused the final retry to fail. Long stack traces are kept verbatim.';


--
-- Name: task_dead_letters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.task_dead_letters_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: task_dead_letters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.task_dead_letters_id_seq OWNED BY public.task_dead_letters.id;


--
-- Name: user_kb_pins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_kb_pins (
    knowledge_domain_id bigint NOT NULL,
    user_id character varying(36) NOT NULL,
    kb_id character varying(36) NOT NULL,
    pinned_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: user_org_memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_org_memberships (
    id bigint NOT NULL,
    user_id character varying(36) NOT NULL,
    org_unit_id character varying(36) NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    source character varying(32) DEFAULT 'manual'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_user_org_membership_source CHECK (((source)::text = ANY ((ARRAY['manual'::character varying, 'workday'::character varying, 'bootstrap'::character varying])::text[]))),
    CONSTRAINT chk_user_org_membership_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying])::text[])))
);


--
--

COMMENT ON TABLE public.user_org_memberships IS 'User membership in enterprise organization units; membership alone grants no knowledge access';


--
-- Name: user_org_memberships_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_org_memberships_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_org_memberships_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_org_memberships_id_seq OWNED BY public.user_org_memberships.id;


--
-- Name: user_resource_favorites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_resource_favorites (
    user_id character varying(36) NOT NULL,
    resource_type character varying(16) NOT NULL,
    resource_id character varying(64) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    username character varying(100) NOT NULL,
    email character varying(255) NOT NULL,
    password_hash character varying(255) NOT NULL,
    avatar character varying(500),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_system_admin boolean DEFAULT false NOT NULL
);


--
--

COMMENT ON TABLE public.users IS 'User accounts in the system';


--
--

COMMENT ON COLUMN public.users.id IS 'Unique identifier of the user';


--
--

COMMENT ON COLUMN public.users.username IS 'Username of the user';


--
--

COMMENT ON COLUMN public.users.email IS 'Email address of the user';


--
--

COMMENT ON COLUMN public.users.password_hash IS 'Hashed password of the user';


--
--

COMMENT ON COLUMN public.users.avatar IS 'Avatar URL of the user';


--
--

COMMENT ON COLUMN public.users.is_active IS 'Whether the user is active';


--
--

COMMENT ON COLUMN public.users.preferences IS 'Per-user JSON preferences (memory toggle, future UI knobs)';


--
--

COMMENT ON COLUMN public.users.is_system_admin IS 'Whether the user is a platform-wide system administrator';


--
-- Name: vector_stores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vector_stores (
    id character varying(36) NOT NULL,
    name character varying(255) NOT NULL,
    engine_type character varying(50) NOT NULL,
    connection_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    index_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    knowledge_domain_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);


--
-- Name: web_search_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.web_search_providers (
    id character varying(36) NOT NULL,
    knowledge_domain_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    provider character varying(50) NOT NULL,
    description text,
    parameters jsonb,
    is_default boolean DEFAULT false,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);


--
-- Name: audit_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_logs ALTER COLUMN id SET DEFAULT nextval('public.audit_logs_id_seq'::regclass);


--
-- Name: integration_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_events ALTER COLUMN id SET DEFAULT nextval('public.integration_events_id_seq'::regclass);


--
-- Name: knowledge_base_grants id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_base_grants ALTER COLUMN id SET DEFAULT nextval('public.knowledge_base_grants_id_seq'::regclass);


--
-- Name: knowledge_domain_admins id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_admins ALTER COLUMN id SET DEFAULT nextval('public.knowledge_domain_admins_id_seq'::regclass);


--
-- Name: knowledge_domains id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domains ALTER COLUMN id SET DEFAULT nextval('public.knowledge_domains_id_seq'::regclass);


--
-- Name: knowledge_grants id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants ALTER COLUMN id SET DEFAULT nextval('public.knowledge_grants_id_seq'::regclass);


--
-- Name: knowledge_processing_spans id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_processing_spans ALTER COLUMN id SET DEFAULT nextval('public.knowledge_processing_spans_id_seq'::regclass);


--
-- Name: sso_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sso_identities ALTER COLUMN id SET DEFAULT nextval('public.sso_identities_id_seq'::regclass);


--
-- Name: system_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings ALTER COLUMN id SET DEFAULT nextval('public.system_settings_id_seq'::regclass);


--
-- Name: task_dead_letters id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_dead_letters ALTER COLUMN id SET DEFAULT nextval('public.task_dead_letters_id_seq'::regclass);


--
-- Name: user_org_memberships id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_memberships ALTER COLUMN id SET DEFAULT nextval('public.user_org_memberships_id_seq'::regclass);


--
-- Name: audit_logs audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);


--
-- Name: auth_tokens auth_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT auth_tokens_pkey PRIMARY KEY (id);


--
-- Name: chunks chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chunks
    ADD CONSTRAINT chunks_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);


--
-- Name: external_org_units external_org_units_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_org_units
    ADD CONSTRAINT external_org_units_pkey PRIMARY KEY (id);


--
-- Name: external_workers external_workers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_workers
    ADD CONSTRAINT external_workers_pkey PRIMARY KEY (id);


--
-- Name: integration_events integration_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_events
    ADD CONSTRAINT integration_events_pkey PRIMARY KEY (id);


--
-- Name: integration_sync_runs integration_sync_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_sync_runs
    ADD CONSTRAINT integration_sync_runs_pkey PRIMARY KEY (id);


--
-- Name: knowledge_base_grants knowledge_base_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_base_grants
    ADD CONSTRAINT knowledge_base_grants_pkey PRIMARY KEY (id);


--
-- Name: knowledge_bases knowledge_bases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_bases
    ADD CONSTRAINT knowledge_bases_pkey PRIMARY KEY (id);


--
-- Name: knowledge_domain_admins knowledge_domain_admins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_admins
    ADD CONSTRAINT knowledge_domain_admins_pkey PRIMARY KEY (id);


--
-- Name: knowledge_domain_storage knowledge_domain_storage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_storage
    ADD CONSTRAINT knowledge_domain_storage_pkey PRIMARY KEY (knowledge_domain_id);


--
-- Name: knowledge_domains knowledge_domains_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domains
    ADD CONSTRAINT knowledge_domains_pkey PRIMARY KEY (id);


--
-- Name: knowledge_folders knowledge_folders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_folders
    ADD CONSTRAINT knowledge_folders_pkey PRIMARY KEY (id);


--
-- Name: knowledge_grants knowledge_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants
    ADD CONSTRAINT knowledge_grants_pkey PRIMARY KEY (id);


--
-- Name: knowledge_processing_spans knowledge_processing_spans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_processing_spans
    ADD CONSTRAINT knowledge_processing_spans_pkey PRIMARY KEY (id);


--
-- Name: knowledge_tag_relations knowledge_tag_relations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_tag_relations
    ADD CONSTRAINT knowledge_tag_relations_pkey PRIMARY KEY (knowledge_id, tag_id);


--
-- Name: knowledge_tags knowledge_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_tags
    ADD CONSTRAINT knowledge_tags_pkey PRIMARY KEY (id);


--
-- Name: knowledges knowledges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledges
    ADD CONSTRAINT knowledges_pkey PRIMARY KEY (id);


--
-- Name: mcp_oauth_clients mcp_oauth_clients_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_oauth_clients
    ADD CONSTRAINT mcp_oauth_clients_pkey PRIMARY KEY (id);


--
-- Name: mcp_oauth_tokens mcp_oauth_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_oauth_tokens
    ADD CONSTRAINT mcp_oauth_tokens_pkey PRIMARY KEY (id);


--
-- Name: mcp_services mcp_services_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_services
    ADD CONSTRAINT mcp_services_pkey PRIMARY KEY (id);


--
-- Name: mcp_tool_approvals mcp_tool_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_tool_approvals
    ADD CONSTRAINT mcp_tool_approvals_pkey PRIMARY KEY (id);


--
-- Name: messages messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_pkey PRIMARY KEY (id);


--
-- Name: models models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.models
    ADD CONSTRAINT models_pkey PRIMARY KEY (id);


--
-- Name: org_units org_units_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_units
    ADD CONSTRAINT org_units_pkey PRIMARY KEY (id);


--
-- Name: platform_runtime_configs platform_runtime_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_runtime_configs
    ADD CONSTRAINT platform_runtime_configs_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);


--
-- Name: sso_identities sso_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sso_identities
    ADD CONSTRAINT sso_identities_pkey PRIMARY KEY (id);


--
-- Name: sync_logs sync_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_logs
    ADD CONSTRAINT sync_logs_pkey PRIMARY KEY (id);


--
-- Name: system_settings system_settings_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_key_key UNIQUE (key);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);


--
-- Name: task_dead_letters task_dead_letters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_dead_letters
    ADD CONSTRAINT task_dead_letters_pkey PRIMARY KEY (id);


--
-- Name: external_org_units uq_external_org_units_provider_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_org_units
    ADD CONSTRAINT uq_external_org_units_provider_id UNIQUE (provider, external_org_id);


--
-- Name: external_workers uq_external_workers_provider_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_workers
    ADD CONSTRAINT uq_external_workers_provider_id UNIQUE (provider, external_worker_id);


--
-- Name: integration_events uq_integration_events_provider_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_events
    ADD CONSTRAINT uq_integration_events_provider_id UNIQUE (provider, external_event_id);


--
-- Name: knowledge_base_grants uq_knowledge_base_grant; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_base_grants
    ADD CONSTRAINT uq_knowledge_base_grant UNIQUE (knowledge_base_id, subject_type, subject_id);


--
-- Name: knowledge_domain_admins uq_knowledge_domain_admin; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_admins
    ADD CONSTRAINT uq_knowledge_domain_admin UNIQUE (knowledge_domain_id, user_id);


--
-- Name: knowledge_folders uq_knowledge_folders_base_path; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_folders
    ADD CONSTRAINT uq_knowledge_folders_base_path UNIQUE (knowledge_base_id, relative_path);


--
-- Name: knowledge_grants uq_knowledge_grant; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants
    ADD CONSTRAINT uq_knowledge_grant UNIQUE (knowledge_id, subject_type, subject_id);


--
-- Name: knowledge_processing_spans uq_kpspan_attempt_span; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_processing_spans
    ADD CONSTRAINT uq_kpspan_attempt_span UNIQUE (knowledge_id, attempt, span_id);


--
-- Name: user_org_memberships uq_user_org_membership; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_memberships
    ADD CONSTRAINT uq_user_org_membership UNIQUE (user_id, org_unit_id);


--
-- Name: user_kb_pins user_kb_pins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_kb_pins
    ADD CONSTRAINT user_kb_pins_pkey PRIMARY KEY (knowledge_domain_id, user_id, kb_id);


--
-- Name: user_org_memberships user_org_memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_memberships
    ADD CONSTRAINT user_org_memberships_pkey PRIMARY KEY (id);


--
-- Name: user_resource_favorites user_resource_favorites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_resource_favorites
    ADD CONSTRAINT user_resource_favorites_pkey PRIMARY KEY (user_id, resource_type, resource_id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: vector_stores vector_stores_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vector_stores
    ADD CONSTRAINT vector_stores_pkey PRIMARY KEY (id);


--
-- Name: web_search_providers web_search_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.web_search_providers
    ADD CONSTRAINT web_search_providers_pkey PRIMARY KEY (id);


--
-- Name: idx_audit_logs_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_actor ON public.audit_logs USING btree (actor_user_id);


--
-- Name: idx_audit_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_created_at ON public.audit_logs USING btree (created_at);


--
-- Name: idx_audit_logs_knowledge_domain_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_knowledge_domain_action ON public.audit_logs USING btree (knowledge_domain_id, action);


--
-- Name: idx_audit_logs_knowledge_domain_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_knowledge_domain_id_desc ON public.audit_logs USING btree (knowledge_domain_id, id DESC);


--
-- Name: idx_auth_tokens_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_expires_at ON public.auth_tokens USING btree (expires_at);


--
-- Name: idx_auth_tokens_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_token ON public.auth_tokens USING btree (token);


--
-- Name: idx_auth_tokens_token_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_token_type ON public.auth_tokens USING btree (token_type);


--
-- Name: idx_auth_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_user_id ON public.auth_tokens USING btree (user_id);


--
-- Name: idx_chunks_chunk_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_chunk_type ON public.chunks USING btree (chunk_type);


--
-- Name: idx_chunks_content_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_content_hash ON public.chunks USING btree (content_hash);


--
-- Name: idx_chunks_kb_knowledge_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_kb_knowledge_domain ON public.chunks USING btree (knowledge_base_id, knowledge_domain_id);


--
-- Name: idx_chunks_knowledge_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_knowledge_enabled ON public.chunks USING btree (knowledge_id, is_enabled, deleted_at);


--
-- Name: idx_chunks_parent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_parent_id ON public.chunks USING btree (parent_chunk_id);


--
-- Name: idx_chunks_seq_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_chunks_seq_id ON public.chunks USING btree (seq_id);


--
-- Name: idx_chunks_tag; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_tag ON public.chunks USING btree (tag_id);


--
-- Name: idx_chunks_knowledge_domain_kg; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_knowledge_domain_kg ON public.chunks USING btree (knowledge_domain_id, knowledge_id);


--
-- Name: idx_custom_agents_creator; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_custom_agents_creator ON public.custom_agents USING btree (created_by, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_custom_agents_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_custom_agents_deleted_at ON public.custom_agents USING btree (deleted_at);


--
-- Name: idx_custom_agents_is_builtin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_custom_agents_is_builtin ON public.custom_agents USING btree (is_builtin);


--
-- Name: idx_data_sources_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_sources_deleted_at ON public.data_sources USING btree (deleted_at);


--
-- Name: idx_data_sources_knowledge_base_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_sources_knowledge_base_id ON public.data_sources USING btree (knowledge_base_id);


--
-- Name: idx_data_sources_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_sources_status ON public.data_sources USING btree (status);


--
-- Name: idx_data_sources_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_sources_knowledge_domain_id ON public.data_sources USING btree (knowledge_domain_id);


--
-- Name: idx_data_sources_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_sources_type ON public.data_sources USING btree (type);


--
-- Name: idx_external_org_units_canonical; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_org_units_canonical ON public.external_org_units USING btree (org_unit_id);


--
-- Name: idx_external_org_units_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_org_units_parent ON public.external_org_units USING btree (provider, parent_external_org_id);


--
-- Name: idx_external_workers_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_workers_email ON public.external_workers USING btree (lower((corporate_email)::text));


--
-- Name: idx_external_workers_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_workers_org ON public.external_workers USING btree (provider, primary_org_external_id);


--
-- Name: idx_external_workers_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_workers_user ON public.external_workers USING btree (user_id);


--
-- Name: idx_integration_events_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integration_events_status ON public.integration_events USING btree (provider, status, received_at);


--
-- Name: idx_integration_sync_runs_provider_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integration_sync_runs_provider_created ON public.integration_sync_runs USING btree (provider, created_at DESC);


--
-- Name: idx_integration_sync_runs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integration_sync_runs_status ON public.integration_sync_runs USING btree (status);


--
-- Name: idx_knowledge_base_grants_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_base_grants_domain ON public.knowledge_base_grants USING btree (knowledge_domain_id, knowledge_base_id);


--
-- Name: idx_knowledge_base_grants_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_base_grants_subject ON public.knowledge_base_grants USING btree (subject_type, subject_id);


--
-- Name: idx_knowledge_bases_id_knowledge_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_knowledge_bases_id_knowledge_domain ON public.knowledge_bases USING btree (id, knowledge_domain_id);


--
-- Name: idx_knowledge_bases_knowledge_domain_creator; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_bases_knowledge_domain_creator ON public.knowledge_bases USING btree (knowledge_domain_id, creator_id);


--
-- Name: idx_knowledge_bases_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_bases_knowledge_domain_id ON public.knowledge_bases USING btree (knowledge_domain_id);


--
-- Name: idx_knowledge_bases_knowledge_domain_vector_store; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_bases_knowledge_domain_vector_store ON public.knowledge_bases USING btree (knowledge_domain_id, vector_store_id);


--
-- Name: idx_knowledge_domain_admins_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_domain_admins_user ON public.knowledge_domain_admins USING btree (user_id, knowledge_domain_id);


--
-- Name: idx_knowledge_domains_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_knowledge_domains_code ON public.knowledge_domains USING btree (code);


--
-- Name: idx_knowledge_domains_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_domains_status ON public.knowledge_domains USING btree (status);


--
-- Name: idx_knowledge_folders_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_folders_parent ON public.knowledge_folders USING btree (parent_id);


--
-- Name: idx_knowledge_folders_knowledge_domain_base; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_folders_knowledge_domain_base ON public.knowledge_folders USING btree (knowledge_domain_id, knowledge_base_id);


--
-- Name: idx_knowledge_grants_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_grants_resource ON public.knowledge_grants USING btree (knowledge_domain_id, knowledge_base_id, knowledge_id);


--
-- Name: idx_knowledge_grants_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_grants_subject ON public.knowledge_grants USING btree (subject_type, subject_id);


--
-- Name: idx_knowledge_tags_kb; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_tags_kb ON public.knowledge_tags USING btree (knowledge_domain_id, knowledge_base_id);


--
-- Name: idx_knowledge_tags_kb_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_knowledge_tags_kb_name ON public.knowledge_tags USING btree (knowledge_domain_id, knowledge_base_id, name);


--
-- Name: idx_knowledge_tags_seq_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_knowledge_tags_seq_id ON public.knowledge_tags USING btree (seq_id);


--
-- Name: idx_knowledges_base_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledges_base_id ON public.knowledges USING btree (knowledge_base_id);


--
-- Name: idx_knowledges_enable_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledges_enable_status ON public.knowledges USING btree (enable_status);


--
-- Name: idx_knowledges_folder_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledges_folder_id ON public.knowledges USING btree (folder_id);


--
-- Name: idx_knowledges_id_kb_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_knowledges_id_kb_domain ON public.knowledges USING btree (id, knowledge_base_id, knowledge_domain_id);


--
-- Name: idx_knowledges_parse_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledges_parse_status ON public.knowledges USING btree (parse_status);


--
-- Name: idx_knowledges_summary_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledges_summary_status ON public.knowledges USING btree (summary_status);


--
-- Name: idx_knowledges_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledges_knowledge_domain_id ON public.knowledges USING btree (knowledge_domain_id);


--
-- Name: idx_kpspan_knowledge_attempt; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_kpspan_knowledge_attempt ON public.knowledge_processing_spans USING btree (knowledge_id, attempt);


--
-- Name: idx_kpspan_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_kpspan_parent ON public.knowledge_processing_spans USING btree (parent_span_id) WHERE (parent_span_id IS NOT NULL);


--
-- Name: idx_kpspan_status_started; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_kpspan_status_started ON public.knowledge_processing_spans USING btree (status, started_at);


--
-- Name: idx_ktr_knowledge; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ktr_knowledge ON public.knowledge_tag_relations USING btree (knowledge_id);


--
-- Name: idx_ktr_tag; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ktr_tag ON public.knowledge_tag_relations USING btree (tag_id);


--
-- Name: idx_mcp_oauth_clients_service_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_oauth_clients_service_id ON public.mcp_oauth_clients USING btree (service_id);


--
-- Name: idx_mcp_oauth_clients_domain_service; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mcp_oauth_clients_domain_service ON public.mcp_oauth_clients USING btree (knowledge_domain_id, service_id);


--
-- Name: idx_mcp_oauth_tokens_principal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_oauth_tokens_principal ON public.mcp_oauth_tokens USING btree (principal_type, principal_id);


--
-- Name: idx_mcp_oauth_tokens_service_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_oauth_tokens_service_id ON public.mcp_oauth_tokens USING btree (service_id);


--
-- Name: idx_mcp_oauth_tokens_domain_principal_service; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mcp_oauth_tokens_domain_principal_service ON public.mcp_oauth_tokens USING btree (knowledge_domain_id, principal_type, principal_id, service_id);


--
-- Name: idx_mcp_oauth_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_oauth_tokens_user_id ON public.mcp_oauth_tokens USING btree (user_id);


--
-- Name: idx_mcp_services_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_services_deleted_at ON public.mcp_services USING btree (deleted_at);


--
-- Name: idx_mcp_services_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_services_enabled ON public.mcp_services USING btree (enabled);


--
-- Name: idx_mcp_services_is_builtin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_services_is_builtin ON public.mcp_services USING btree (is_builtin);


--
-- Name: idx_mcp_services_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_services_knowledge_domain_id ON public.mcp_services USING btree (knowledge_domain_id);


--
-- Name: idx_mcp_tool_approvals_service_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mcp_tool_approvals_service_id ON public.mcp_tool_approvals USING btree (service_id);


--
-- Name: idx_mcp_tool_approvals_domain_service_tool; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mcp_tool_approvals_domain_service_tool ON public.mcp_tool_approvals USING btree (knowledge_domain_id, service_id, tool_name);


--
-- Name: idx_messages_agent_steps; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_messages_agent_steps ON public.messages USING gin (agent_steps);


--
-- Name: idx_messages_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_messages_session_id ON public.messages USING btree (session_id);


--
-- Name: idx_models_is_builtin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_models_is_builtin ON public.models USING btree (is_builtin);


--
-- Name: idx_models_managed_by_yaml; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_models_managed_by_yaml ON public.models USING btree (managed_by) WHERE ((managed_by)::text <> ''::text);


--
-- Name: idx_models_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_models_source ON public.models USING btree (source);


--
-- Name: idx_models_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_models_type ON public.models USING btree (type);


--
-- Name: idx_org_units_code_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_org_units_code_unique ON public.org_units USING btree (code) WHERE (deleted_at IS NULL);


--
-- Name: idx_org_units_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_org_units_parent ON public.org_units USING btree (parent_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_org_units_source_external_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_org_units_source_external_unique ON public.org_units USING btree (source, external_id) WHERE ((external_id IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: idx_org_units_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_org_units_status ON public.org_units USING btree (status) WHERE (deleted_at IS NULL);


--
-- Name: idx_sessions_agent_config; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sessions_agent_config ON public.sessions USING gin (last_request_state);


--
-- Name: idx_sessions_user_pin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sessions_user_pin ON public.sessions USING btree (user_id, is_pinned DESC, pinned_at DESC, updated_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_sso_identities_provider_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_sso_identities_provider_subject ON public.sso_identities USING btree (provider, issuer, subject);


--
-- Name: idx_sso_identities_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sso_identities_user ON public.sso_identities USING btree (user_id);


--
-- Name: idx_sync_logs_data_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sync_logs_data_source_id ON public.sync_logs USING btree (data_source_id);


--
-- Name: idx_sync_logs_started_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sync_logs_started_at ON public.sync_logs USING btree (started_at);


--
-- Name: idx_sync_logs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sync_logs_status ON public.sync_logs USING btree (status);


--
-- Name: idx_sync_logs_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sync_logs_knowledge_domain_id ON public.sync_logs USING btree (knowledge_domain_id);


--
-- Name: idx_system_settings_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_system_settings_category ON public.system_settings USING btree (category);


--
-- Name: idx_task_dead_letters_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_dead_letters_scope ON public.task_dead_letters USING btree (scope, scope_id, failed_at DESC);


--
-- Name: idx_task_dead_letters_task_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_dead_letters_task_type ON public.task_dead_letters USING btree (task_type, failed_at DESC);


--
-- Name: idx_task_dead_letters_knowledge_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_dead_letters_knowledge_domain ON public.task_dead_letters USING btree (knowledge_domain_id, failed_at DESC);


--
-- Name: idx_user_kb_pins_user_domain_pinned_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_kb_pins_user_domain_pinned_at ON public.user_kb_pins USING btree (knowledge_domain_id, user_id, pinned_at DESC);


--
-- Name: idx_user_org_memberships_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_org_memberships_org ON public.user_org_memberships USING btree (org_unit_id, status);


--
-- Name: idx_user_org_memberships_primary; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_user_org_memberships_primary ON public.user_org_memberships USING btree (user_id) WHERE ((is_primary = true) AND ((status)::text = 'active'::text));


--
-- Name: idx_user_org_memberships_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_org_memberships_user ON public.user_org_memberships USING btree (user_id, status);


--
-- Name: idx_user_resource_favorites_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_resource_favorites_user_type_created_at ON public.user_resource_favorites USING btree (user_id, resource_type, created_at DESC);


--
-- Name: idx_users_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_deleted_at ON public.users USING btree (deleted_at);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_is_system_admin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_is_system_admin ON public.users USING btree (is_system_admin);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_username ON public.users USING btree (username);


--
-- Name: idx_vector_stores_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vector_stores_deleted_at ON public.vector_stores USING btree (deleted_at);


--
-- Name: idx_vector_stores_engine_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vector_stores_engine_type ON public.vector_stores USING btree (engine_type);


--
-- Name: idx_vector_stores_name_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_vector_stores_name_domain ON public.vector_stores USING btree (name, knowledge_domain_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_vector_stores_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vector_stores_knowledge_domain_id ON public.vector_stores USING btree (knowledge_domain_id);


--
-- Name: idx_web_search_providers_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_web_search_providers_deleted_at ON public.web_search_providers USING btree (deleted_at);


--
-- Name: idx_web_search_providers_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_web_search_providers_provider ON public.web_search_providers USING btree (provider);


--
-- Name: idx_web_search_providers_knowledge_domain_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_web_search_providers_knowledge_domain_id ON public.web_search_providers USING btree (knowledge_domain_id);


--
-- Name: mcp_services trigger_mcp_services_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trigger_mcp_services_updated_at BEFORE UPDATE ON public.mcp_services FOR EACH ROW EXECUTE FUNCTION public.update_mcp_services_updated_at();


--
-- Name: external_org_units external_org_units_org_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_org_units
    ADD CONSTRAINT external_org_units_org_unit_id_fkey FOREIGN KEY (org_unit_id) REFERENCES public.org_units(id) ON DELETE SET NULL;


--
-- Name: external_workers external_workers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_workers
    ADD CONSTRAINT external_workers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: auth_tokens fk_auth_tokens_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT fk_auth_tokens_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: knowledge_base_grants fk_knowledge_base_grant_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_base_grants
    ADD CONSTRAINT fk_knowledge_base_grant_kb FOREIGN KEY (knowledge_base_id, knowledge_domain_id) REFERENCES public.knowledge_bases(id, knowledge_domain_id) ON DELETE CASCADE;


--
-- Name: knowledge_grants fk_knowledge_grant_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants
    ADD CONSTRAINT fk_knowledge_grant_kb FOREIGN KEY (knowledge_base_id, knowledge_domain_id) REFERENCES public.knowledge_bases(id, knowledge_domain_id) ON DELETE CASCADE;


--
-- Name: knowledge_grants fk_knowledge_grant_knowledge; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants
    ADD CONSTRAINT fk_knowledge_grant_knowledge FOREIGN KEY (knowledge_id, knowledge_base_id, knowledge_domain_id) REFERENCES public.knowledges(id, knowledge_base_id, knowledge_domain_id) ON DELETE CASCADE;


--
-- Name: knowledge_base_grants knowledge_base_grants_granted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_base_grants
    ADD CONSTRAINT knowledge_base_grants_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: knowledge_base_grants knowledge_base_grants_knowledge_domain_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_base_grants
    ADD CONSTRAINT knowledge_base_grants_knowledge_domain_id_fkey FOREIGN KEY (knowledge_domain_id) REFERENCES public.knowledge_domains(id) ON DELETE CASCADE;


--
-- Name: knowledge_domain_admins knowledge_domain_admins_granted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_admins
    ADD CONSTRAINT knowledge_domain_admins_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: knowledge_domain_admins knowledge_domain_admins_knowledge_domain_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_admins
    ADD CONSTRAINT knowledge_domain_admins_knowledge_domain_id_fkey FOREIGN KEY (knowledge_domain_id) REFERENCES public.knowledge_domains(id) ON DELETE CASCADE;


--
-- Name: knowledge_domain_admins knowledge_domain_admins_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_admins
    ADD CONSTRAINT knowledge_domain_admins_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: knowledge_domain_storage knowledge_domain_storage_knowledge_domain_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_domain_storage
    ADD CONSTRAINT knowledge_domain_storage_knowledge_domain_id_fkey FOREIGN KEY (knowledge_domain_id) REFERENCES public.knowledge_domains(id) ON DELETE CASCADE;


--
-- Name: knowledge_folders knowledge_folders_knowledge_base_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_folders
    ADD CONSTRAINT knowledge_folders_knowledge_base_id_fkey FOREIGN KEY (knowledge_base_id) REFERENCES public.knowledge_bases(id) ON DELETE CASCADE;


--
-- Name: knowledge_folders knowledge_folders_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_folders
    ADD CONSTRAINT knowledge_folders_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.knowledge_folders(id) ON DELETE CASCADE;


--
-- Name: knowledge_grants knowledge_grants_granted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants
    ADD CONSTRAINT knowledge_grants_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: knowledge_grants knowledge_grants_knowledge_domain_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_grants
    ADD CONSTRAINT knowledge_grants_knowledge_domain_id_fkey FOREIGN KEY (knowledge_domain_id) REFERENCES public.knowledge_domains(id) ON DELETE CASCADE;


--
-- Name: knowledges knowledges_folder_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledges
    ADD CONSTRAINT knowledges_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES public.knowledge_folders(id) ON DELETE SET NULL;


--
-- Name: mcp_oauth_clients mcp_oauth_clients_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_oauth_clients
    ADD CONSTRAINT mcp_oauth_clients_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.mcp_services(id) ON DELETE CASCADE;


--
-- Name: mcp_oauth_tokens mcp_oauth_tokens_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_oauth_tokens
    ADD CONSTRAINT mcp_oauth_tokens_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.mcp_services(id) ON DELETE CASCADE;


--
-- Name: mcp_tool_approvals mcp_tool_approvals_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mcp_tool_approvals
    ADD CONSTRAINT mcp_tool_approvals_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.mcp_services(id) ON DELETE CASCADE;


--
-- Name: org_units org_units_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_units
    ADD CONSTRAINT org_units_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: org_units org_units_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_units
    ADD CONSTRAINT org_units_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.org_units(id) ON DELETE RESTRICT;


--
-- Name: sso_identities sso_identities_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sso_identities
    ADD CONSTRAINT sso_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: sync_logs sync_logs_data_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_logs
    ADD CONSTRAINT sync_logs_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES public.data_sources(id) ON DELETE CASCADE;


--
-- Name: user_org_memberships user_org_memberships_org_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_memberships
    ADD CONSTRAINT user_org_memberships_org_unit_id_fkey FOREIGN KEY (org_unit_id) REFERENCES public.org_units(id) ON DELETE CASCADE;


--
-- Name: user_org_memberships user_org_memberships_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_memberships
    ADD CONSTRAINT user_org_memberships_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
--



ALTER TABLE ONLY public.knowledge_domain_admins ADD COLUMN IF NOT EXISTS status character varying(16) DEFAULT 'active'::character varying NOT NULL;
ALTER TABLE ONLY public.knowledge_domain_admins ADD CONSTRAINT knowledge_domain_admins_status_check CHECK (((status)::text = 'active'::text));

INSERT INTO public.platform_runtime_configs (id, retriever_engines, created_at, updated_at) VALUES (1, '{"engines":[]}'::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) ON CONFLICT (id) DO NOTHING;
