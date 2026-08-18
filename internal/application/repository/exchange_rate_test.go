package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestExchangeRateRepositoryUpsertsSingleRMBCHFRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.ExchangeRate{}); err != nil {
		t.Fatalf("migrate exchange rate: %v", err)
	}
	repo := NewExchangeRateRepository(db)
	ctx := context.Background()
	missing, err := repo.GetRMBCHF(ctx)
	if err != nil || missing != nil {
		t.Fatalf("initial GetRMBCHF() rate=%+v err=%v", missing, err)
	}
	if err := repo.UpsertRMBCHF(ctx, &types.ExchangeRate{
		CurrencyPair: types.RMBCHFExchangeRatePair, RMBAmount: 6, CHFAmount: 1,
	}); err != nil {
		t.Fatalf("first UpsertRMBCHF(): %v", err)
	}
	if err := repo.UpsertRMBCHF(ctx, &types.ExchangeRate{
		CurrencyPair: types.RMBCHFExchangeRatePair, RMBAmount: 6.25, CHFAmount: 1,
	}); err != nil {
		t.Fatalf("second UpsertRMBCHF(): %v", err)
	}
	rate, err := repo.GetRMBCHF(ctx)
	if err != nil || rate == nil || rate.RMBAmount != 6.25 || rate.CHFAmount != 1 {
		t.Fatalf("GetRMBCHF() rate=%+v err=%v", rate, err)
	}
}
