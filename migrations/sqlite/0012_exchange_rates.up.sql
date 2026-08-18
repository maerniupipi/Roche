CREATE TABLE exchange_rates (
    currency_pair TEXT PRIMARY KEY CHECK (currency_pair = 'RMB_CHF'),
    rmb_amount NUMERIC NOT NULL CHECK (rmb_amount > 0),
    chf_amount NUMERIC NOT NULL CHECK (chf_amount > 0),
    updated_by TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
