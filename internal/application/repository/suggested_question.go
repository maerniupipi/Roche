package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type suggestedQuestionRepository struct{ db *gorm.DB }

func NewSuggestedQuestionRepository(db *gorm.DB) interfaces.SuggestedQuestionRepository {
	return &suggestedQuestionRepository{db: db}
}

func (r *suggestedQuestionRepository) List(ctx context.Context) ([]types.HomepageSuggestedQuestion, error) {
	var questions []types.HomepageSuggestedQuestion
	err := r.db.WithContext(ctx).Order("sort_order ASC").Find(&questions).Error
	return questions, err
}

func (r *suggestedQuestionRepository) Get(ctx context.Context, id string) (*types.HomepageSuggestedQuestion, error) {
	var question types.HomepageSuggestedQuestion
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&question).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &question, err
}

// ReplaceAll makes the PUT operation atomic: readers see either the old
// three-question configuration or the new one, never a partially written set.
func (r *suggestedQuestionRepository) ReplaceAll(ctx context.Context, questions []types.HomepageSuggestedQuestion) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&types.HomepageSuggestedQuestion{}).Error; err != nil {
			return err
		}
		return tx.Create(&questions).Error
	})
}
