package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// ErrCustomAgentNotFound is returned when a custom agent is not found
var ErrCustomAgentNotFound = errors.New("custom agent not found")

// customAgentRepository implements the CustomAgentRepository interface
type customAgentRepository struct {
	db *gorm.DB
}

// NewCustomAgentRepository creates a new custom agent repository
func NewCustomAgentRepository(db *gorm.DB) interfaces.CustomAgentRepository {
	return &customAgentRepository{db: db}
}

// CreateAgent creates a new custom agent
func (r *customAgentRepository) CreateAgent(ctx context.Context, agent *types.CustomAgent) error {
	return r.db.WithContext(ctx).Create(agent).Error
}

// GetAgentByID gets an agent by id.
func (r *customAgentRepository) GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error) {
	var agent types.CustomAgent
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomAgentNotFound
		}
		return nil, err
	}
	return &agent, nil
}

func (r *customAgentRepository) ListAgents(ctx context.Context) ([]*types.CustomAgent, error) {
	var agents []*types.CustomAgent
	if err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Find(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// UpdateAgent updates an agent
func (r *customAgentRepository) UpdateAgent(ctx context.Context, agent *types.CustomAgent) error {
	return r.db.WithContext(ctx).Save(agent).Error
}

// DeleteAgent deletes an agent (soft delete)
func (r *customAgentRepository) DeleteAgent(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.CustomAgent{}).Error
}

// CountByModelID counts active agents whose config references modelID.
func (r *customAgentRepository) CountByModelID(
	ctx context.Context, modelID string,
) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).
		Model(&types.CustomAgent{})
	query = scopeCustomAgentsByModelID(query, modelID)
	err := query.Count(&count).Error
	return count, err
}
