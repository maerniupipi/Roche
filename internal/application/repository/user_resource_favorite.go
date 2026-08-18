package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type userResourceFavoriteRepository struct {
	db *gorm.DB
}

func NewUserResourceFavoriteRepository(db *gorm.DB) interfaces.UserResourceFavoriteRepository {
	return &userResourceFavoriteRepository{db: db}
}

func (r *userResourceFavoriteRepository) List(
	ctx context.Context, userID string, resourceType string,
) ([]*types.UserResourceFavorite, error) {
	var list []*types.UserResourceFavorite
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND resource_type = ?", userID, resourceType).
		Order("created_at DESC").
		Find(&list).Error
	return list, err
}

func (r *userResourceFavoriteRepository) Add(
	ctx context.Context, userID string, resourceType, resourceID string,
) (bool, error) {
	rec := &types.UserResourceFavorite{
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}
	res := r.db.WithContext(ctx).Where(rec).FirstOrCreate(rec)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *userResourceFavoriteRepository) Remove(
	ctx context.Context, userID string, resourceType, resourceID string,
) (bool, error) {
	res := r.db.WithContext(ctx).
		Where("user_id = ? AND resource_type = ? AND resource_id = ?", userID, resourceType, resourceID).
		Delete(&types.UserResourceFavorite{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *userResourceFavoriteRepository) IsFavorite(
	ctx context.Context, userID string, resourceType, resourceID string,
) (bool, error) {
	var rec types.UserResourceFavorite
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND resource_type = ? AND resource_id = ?", userID, resourceType, resourceID).
		First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
