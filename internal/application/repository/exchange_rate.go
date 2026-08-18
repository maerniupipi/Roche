package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type exchangeRateRepository struct{ db *gorm.DB }

func NewExchangeRateRepository(db *gorm.DB) interfaces.ExchangeRateRepository {
	return &exchangeRateRepository{db: db}
}

func (r *exchangeRateRepository) GetRMBCHF(ctx context.Context) (*types.ExchangeRate, error) {
	var rate types.ExchangeRate
	err := r.db.WithContext(ctx).
		Where("currency_pair = ?", types.RMBCHFExchangeRatePair).
		First(&rate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rate, err
}

func (r *exchangeRateRepository) UpsertRMBCHF(ctx context.Context, rate *types.ExchangeRate) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "currency_pair"}},
			DoUpdates: clause.AssignmentColumns([]string{"rmb_amount", "chf_amount", "updated_by", "updated_at"}),
		}).
		Create(rate).Error
}
