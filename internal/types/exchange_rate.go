package types

import "time"

const RMBCHFExchangeRatePair = "RMB_CHF"

// ExchangeRate stores the single platform-wide RMB/CHF ratio used by Func033.
// RMBAmount and CHFAmount are the two sides of the configured ratio, for
// example 6 and 1 means 6 RMB = 1 CHF.
type ExchangeRate struct {
	CurrencyPair string    `json:"currency_pair" gorm:"type:varchar(16);primaryKey"`
	RMBAmount    float64   `json:"rmb_amount" gorm:"type:decimal(20,8);not null"`
	CHFAmount    float64   `json:"chf_amount" gorm:"type:decimal(20,8);not null"`
	UpdatedBy    string    `json:"updated_by,omitempty" gorm:"type:varchar(36);not null;default:''"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
	IsDefault    bool      `json:"is_default" gorm:"-"`
}

func (ExchangeRate) TableName() string { return "exchange_rates" }

type ExchangeRateConfig struct {
	RMBAmount float64 `json:"rmb_amount"`
	CHFAmount float64 `json:"chf_amount"`
}
