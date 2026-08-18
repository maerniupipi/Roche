package service

import (
	"context"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

type exchangeRateRepoStub struct {
	rate  *types.ExchangeRate
	saved *types.ExchangeRate
	err   error
}

func (r *exchangeRateRepoStub) GetRMBCHF(context.Context) (*types.ExchangeRate, error) {
	return r.rate, r.err
}

func (r *exchangeRateRepoStub) UpsertRMBCHF(_ context.Context, rate *types.ExchangeRate) error {
	r.saved = rate
	r.rate = rate
	return r.err
}

func TestExchangeRateServiceReturnsFunc033DefaultWhenNotConfigured(t *testing.T) {
	rate, err := NewExchangeRateService(&exchangeRateRepoStub{}).GetExchangeRate(context.Background())
	if err != nil || rate.RMBAmount != 6 || rate.CHFAmount != 1 || !rate.IsDefault {
		t.Fatalf("rate=%+v err=%v", rate, err)
	}
}

func TestExchangeRateServiceConfiguresPositiveRatio(t *testing.T) {
	repo := &exchangeRateRepoStub{}
	rate, err := NewExchangeRateService(repo).ConfigureExchangeRate(
		context.Background(), types.ExchangeRateConfig{RMBAmount: 6.25, CHFAmount: 1}, "admin-id",
	)
	if err != nil || rate.RMBAmount != 6.25 || rate.IsDefault || repo.saved.UpdatedBy != "admin-id" {
		t.Fatalf("rate=%+v saved=%+v err=%v", rate, repo.saved, err)
	}
}

func TestExchangeRateServiceRejectsNonPositiveRatio(t *testing.T) {
	_, err := NewExchangeRateService(&exchangeRateRepoStub{}).ConfigureExchangeRate(
		context.Background(), types.ExchangeRateConfig{RMBAmount: 0, CHFAmount: 1}, "admin-id",
	)
	if err == nil {
		t.Fatal("ConfigureExchangeRate() error = nil")
	}
}
