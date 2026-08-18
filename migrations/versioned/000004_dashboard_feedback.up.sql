CREATE TABLE IF NOT EXISTS public.dashboard_feedback (
    id bigint NOT NULL,
    knowledge_domain_id integer NOT NULL,
    session_id character varying(36) NOT NULL,
    message_id character varying(36) NOT NULL,
    category character varying(64) NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    satisfaction integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.dashboard_feedback_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.dashboard_feedback_id_seq OWNED BY public.dashboard_feedback.id;

ALTER TABLE ONLY public.dashboard_feedback ALTER COLUMN id SET DEFAULT nextval('public.dashboard_feedback_id_seq'::regclass);

ALTER TABLE ONLY public.dashboard_feedback
    ADD CONSTRAINT dashboard_feedback_pkey PRIMARY KEY (id);

CREATE INDEX IF NOT EXISTS idx_dashboard_feedback_kd_created
    ON public.dashboard_feedback USING btree (knowledge_domain_id, created_at);
