-- Add employee profile fields to users for the user-management page.
-- These fields are maintained by system administrators; SSO/OIDC provisioning
-- can optionally populate them from directory claims.

ALTER TABLE public.users
    ADD COLUMN employee_id character varying(64),
    ADD COLUMN account character varying(100),
    ADD COLUMN english_name character varying(100),
    ADD COLUMN chinese_name character varying(100),
    ADD COLUMN employee_type character varying(64),
    ADD COLUMN department_code character varying(64),
    ADD COLUMN department_name character varying(255);

COMMENT ON COLUMN public.users.employee_id IS '员工ID（HR 系统工号）';
COMMENT ON COLUMN public.users.account IS '用户账号/AD 账号';
COMMENT ON COLUMN public.users.english_name IS '英文名';
COMMENT ON COLUMN public.users.chinese_name IS '中文名';
COMMENT ON COLUMN public.users.employee_type IS '员工类型，例如 Manager、Staff';
COMMENT ON COLUMN public.users.department_code IS '部门编码';
COMMENT ON COLUMN public.users.department_name IS '部门名称';

CREATE INDEX idx_users_employee_id ON public.users(employee_id);
CREATE INDEX idx_users_account ON public.users(account);
CREATE INDEX idx_users_department_code ON public.users(department_code);
CREATE INDEX idx_users_english_name ON public.users(english_name);
CREATE INDEX idx_users_chinese_name ON public.users(chinese_name);
