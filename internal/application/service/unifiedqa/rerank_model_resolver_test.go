package unifiedqa

import (
	"context"
	"testing"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestRerankModelResolverSelectsDefaultActiveModel(t *testing.T) {
	provider := &fakeRerankModelProvider{models: []*types.Model{
		{ID: "rerank-b", Type: types.ModelTypeRerank, Status: types.ModelStatusActive},
		{ID: "rerank-a", Type: types.ModelTypeRerank, Status: types.ModelStatusActive, IsDefault: true},
		{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true},
	}, reranker: &fakeEvidenceReranker{}}
	resolver := NewRerankModelResolver(provider)

	model, id, err := resolver.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if model == nil || id != "rerank-a" || provider.requestedID != "rerank-a" {
		t.Fatalf("Resolve() = (%v, %q), requested=%q", model, id, provider.requestedID)
	}
}

type fakeRerankModelProvider struct {
	models      []*types.Model
	reranker    rerank.Reranker
	requestedID string
}

func (f *fakeRerankModelProvider) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	for _, model := range f.models {
		if model.ID == id {
			return model, nil
		}
	}
	return nil, nil
}

func (f *fakeRerankModelProvider) ListModels(context.Context) ([]*types.Model, error) {
	return f.models, nil
}

func (f *fakeRerankModelProvider) GetRerankModel(_ context.Context, id string) (rerank.Reranker, error) {
	f.requestedID = id
	return f.reranker, nil
}
