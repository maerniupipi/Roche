package unifiedqa

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

type RerankModelProvider interface {
	GetModelByID(ctx context.Context, id string) (*types.Model, error)
	ListModels(ctx context.Context) ([]*types.Model, error)
	GetRerankModel(ctx context.Context, id string) (rerank.Reranker, error)
}

type RerankModelResolver struct{ models RerankModelProvider }

func NewRerankModelResolver(models RerankModelProvider) *RerankModelResolver {
	return &RerankModelResolver{models: models}
}

func (r *RerankModelResolver) Resolve(ctx context.Context, preferred string) (rerank.Reranker, string, error) {
	if r == nil || r.models == nil {
		return nil, "", fmt.Errorf("rerank model provider is required")
	}
	modelID := strings.TrimSpace(preferred)
	if modelID != "" {
		model, err := r.models.GetModelByID(ctx, modelID)
		if err != nil || model == nil || model.Type != types.ModelTypeRerank || model.DeletedAt.Valid {
			return nil, "", fmt.Errorf("preferred rerank model %q is unavailable", modelID)
		}
	} else {
		models, err := r.models.ListModels(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("list rerank models: %w", err)
		}
		candidates := make([]*types.Model, 0, len(models))
		for _, model := range models {
			if model != nil && model.Type == types.ModelTypeRerank && !model.DeletedAt.Valid &&
				(model.Status == "" || model.Status == types.ModelStatusActive) {
				candidates = append(candidates, model)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].IsDefault != candidates[j].IsDefault {
				return candidates[i].IsDefault
			}
			return candidates[i].ID < candidates[j].ID
		})
		if len(candidates) == 0 {
			return nil, "", fmt.Errorf("no active Rerank model is available")
		}
		modelID = candidates[0].ID
	}
	model, err := r.models.GetRerankModel(ctx, modelID)
	if err != nil {
		return nil, "", fmt.Errorf("get rerank model %q: %w", modelID, err)
	}
	return model, modelID, nil
}
