package repository

import (
	"context"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type unifiedQARunRepository struct {
	db *gorm.DB
}

func NewUnifiedQARunRepository(db *gorm.DB) interfaces.UnifiedQARunRepository {
	return &unifiedQARunRepository{db: db}
}

func (r *unifiedQARunRepository) CreateRun(ctx context.Context, run *types.QAExecutionRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *unifiedQARunRepository) GetRun(ctx context.Context, runID string) (*types.QAExecutionRun, error) {
	var run types.QAExecutionRun
	if err := r.db.WithContext(ctx).Where("id = ?", runID).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *unifiedQARunRepository) FinishRun(
	ctx context.Context,
	runID string,
	update types.QARunFinishUpdate,
) error {
	return r.db.WithContext(ctx).
		Model(&types.QAExecutionRun{}).
		Where("id = ?", runID).
		Updates(map[string]any{
			"status":             update.Status,
			"rewritten_query":    update.RewrittenQuery,
			"route_type":         update.RouteType,
			"selected_agent_ids": update.SelectedAgentIDs,
			"metrics":            update.Metrics,
			"error_code":         update.ErrorCode,
			"completed_at":       update.CompletedAt,
			"duration_ms":        update.DurationMS,
		}).Error
}
