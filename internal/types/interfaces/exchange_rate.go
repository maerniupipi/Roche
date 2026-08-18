package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

type ExchangeRateRepository interface {
	GetRMBCHF(ctx context.Context) (*types.ExchangeRate, error)
	UpsertRMBCHF(ctx context.Context, rate *types.ExchangeRate) error
}

type ExchangeRateService interface {
	GetExchangeRate(ctx context.Context) (*types.ExchangeRate, error)
	ConfigureExchangeRate(ctx context.Context, config types.ExchangeRateConfig, updatedBy string) (*types.ExchangeRate, error)
}
