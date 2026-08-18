CREATE TABLE public.exchange_rates (
    currency_pair VARCHAR(16) PRIMARY KEY,
    rmb_amount NUMERIC(20, 8) NOT NULL,
    chf_amount NUMERIC(20, 8) NOT NULL,
    updated_by VARCHAR(36) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_exchange_rates_pair CHECK (currency_pair = 'RMB_CHF'),
    CONSTRAINT ck_exchange_rates_rmb_positive CHECK (rmb_amount > 0),
    CONSTRAINT ck_exchange_rates_chf_positive CHECK (chf_amount > 0)
);

COMMENT ON TABLE public.exchange_rates IS
    'Global RMB/CHF conversion ratio used by unified QA Func033';
