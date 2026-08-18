package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// modelRepository implements the model repository interface
type modelRepository struct {
	db *gorm.DB
}

// NewModelRepository creates a new model repository
func NewModelRepository(db *gorm.DB) interfaces.ModelRepository {
	return &modelRepository{db: db}
}

// Create creates a new model
func (r *modelRepository) Create(ctx context.Context, m *types.Model) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByID retrieves a model by ID
func (r *modelRepository) GetByID(ctx context.Context, id string) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// List lists models with optional filtering
func (r *modelRepository) List(
	ctx context.Context, modelType types.ModelType, source types.ModelSource,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx)

	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}

	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return models, nil
}

// Update updates a model
func (r *modelRepository) Update(ctx context.Context, m *types.Model) error {
	// Use Select to explicitly update all fields, including zero values like false
	return r.db.WithContext(ctx).Model(&types.Model{}).Where("id = ?", m.ID).Select("*").Updates(m).Error
}

// Delete deletes a model
func (r *modelRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.Model{}).Error
}

// ClearDefaultByType clears the default flag for all models of a specific type
// This is a batch operation that updates all matching records in one query
func (r *modelRepository) ClearDefaultByType(
	ctx context.Context,
	modelType types.ModelType,
	excludeID string,
) error {
	query := r.db.WithContext(ctx).Model(&types.Model{}).Where("type = ? AND is_default = ?", modelType, true)

	// If excludeID is provided, exclude that model from the update
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	// Batch update: set is_default to false for all matching records
	return query.Update("is_default", false).Error
}
