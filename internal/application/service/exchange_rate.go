package service

import (
	"context"
	"math"
	"strings"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

const (
	defaultRMBAmount = 6
	defaultCHFAmount = 1
)

type exchangeRateService struct {
	repo interfaces.ExchangeRateRepository
}

func NewExchangeRateService(repo interfaces.ExchangeRateRepository) interfaces.ExchangeRateService {
	return &exchangeRateService{repo: repo}
}

// GetExchangeRate returns the configured ratio or Func033's 6:1 fallback when
// the platform has not saved a configuration yet.
func (s *exchangeRateService) GetExchangeRate(ctx context.Context) (*types.ExchangeRate, error) {
	rate, err := s.repo.GetRMBCHF(ctx)
	if err != nil {
		return nil, err
	}
	if rate == nil {
		return &types.ExchangeRate{
			CurrencyPair: types.RMBCHFExchangeRatePair,
			RMBAmount:    defaultRMBAmount, CHFAmount: defaultCHFAmount,
			IsDefault: true,
		}, nil
	}
	return rate, nil
}

func (s *exchangeRateService) ConfigureExchangeRate(
	ctx context.Context,
	config types.ExchangeRateConfig,
	updatedBy string,
) (*types.ExchangeRate, error) {
	if !validExchangeRateAmount(config.RMBAmount) || !validExchangeRateAmount(config.CHFAmount) {
		return nil, apperrors.NewBadRequestError("rmb_amount and chf_amount must be finite positive numbers")
	}
	rate := &types.ExchangeRate{
		CurrencyPair: types.RMBCHFExchangeRatePair,
		RMBAmount:    config.RMBAmount, CHFAmount: config.CHFAmount,
		UpdatedBy: strings.TrimSpace(updatedBy),
	}
	if err := s.repo.UpsertRMBCHF(ctx, rate); err != nil {
		return nil, err
	}
	return s.GetExchangeRate(ctx)
}

func validExchangeRateAmount(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
