package tools

import (
	"context"
	"sync"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

type knowledgeSearchContextCall struct {
	knowledgeBaseID   string
	knowledgeDomainID uint64
	knowledgeBaseIDs  []string
}

type contextRecordingKnowledgeBaseService struct {
	*stubKnowledgeBaseService

	mu             sync.Mutex
	knowledgeBases []*types.KnowledgeBase
	modelKeys      map[string]string
	embeddingCalls []knowledgeSearchContextCall
	searchCalls    []knowledgeSearchContextCall
}

func (s *contextRecordingKnowledgeBaseService) GetKnowledgeBasesByIDsOnly(
	context.Context,
	[]string,
) ([]*types.KnowledgeBase, error) {
	return s.knowledgeBases, nil
}

func (s *contextRecordingKnowledgeBaseService) ResolveEmbeddingModelKeys(
	context.Context,
	[]*types.KnowledgeBase,
) map[string]string {
	return s.modelKeys
}

func (s *contextRecordingKnowledgeBaseService) GetQueryEmbedding(
	ctx context.Context,
	knowledgeBaseID string,
	_ string,
) ([]float32, error) {
	knowledgeDomainID, _ := types.KnowledgeDomainIDFromContext(ctx)
	s.mu.Lock()
	s.embeddingCalls = append(s.embeddingCalls, knowledgeSearchContextCall{
		knowledgeBaseID:   knowledgeBaseID,
		knowledgeDomainID: knowledgeDomainID,
	})
	s.mu.Unlock()
	return []float32{1}, nil
}

func (s *contextRecordingKnowledgeBaseService) HybridSearch(
	ctx context.Context,
	knowledgeBaseID string,
	params types.SearchParams,
) ([]*types.SearchResult, error) {
	knowledgeDomainID, _ := types.KnowledgeDomainIDFromContext(ctx)
	s.mu.Lock()
	s.searchCalls = append(s.searchCalls, knowledgeSearchContextCall{
		knowledgeBaseID:   knowledgeBaseID,
		knowledgeDomainID: knowledgeDomainID,
		knowledgeBaseIDs:  append([]string(nil), params.KnowledgeBaseIDs...),
	})
	s.mu.Unlock()
	return nil, nil
}

func TestConcurrentSearchByTargetsSeparatesKnowledgeDomainsAndSetsContext(t *testing.T) {
	t.Parallel()

	kbs := []*types.KnowledgeBase{
		{
			ID:                "finance-kb",
			KnowledgeDomainID: 11,
			IndexingStrategy:  types.DefaultIndexingStrategy(),
		},
		{
			ID:                "compliance-kb",
			KnowledgeDomainID: 22,
			IndexingStrategy:  types.DefaultIndexingStrategy(),
		},
	}
	service := &contextRecordingKnowledgeBaseService{
		stubKnowledgeBaseService: &stubKnowledgeBaseService{},
		knowledgeBases:           kbs,
		modelKeys: map[string]string{
			"finance-kb":    "shared-model",
			"compliance-kb": "shared-model",
		},
	}
	tool := &KnowledgeSearchTool{knowledgeBaseService: service}
	targets := types.SearchTargets{
		{
			Type:              types.SearchTargetTypeKnowledgeBase,
			KnowledgeBaseID:   "finance-kb",
			KnowledgeDomainID: 11,
		},
		{
			Type:              types.SearchTargetTypeKnowledgeBase,
			KnowledgeBaseID:   "compliance-kb",
			KnowledgeDomainID: 22,
		},
	}

	tool.concurrentSearchByTargets(context.Background(), []string{"approval"}, targets, 5, 0.1, 0.1, nil)

	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.embeddingCalls) != 2 {
		t.Fatalf("embedding calls = %d, want 2", len(service.embeddingCalls))
	}
	if len(service.searchCalls) != 2 {
		t.Fatalf("search calls = %d, want 2", len(service.searchCalls))
	}

	searchDomains := make(map[uint64]string, len(service.searchCalls))
	for _, call := range service.searchCalls {
		if len(call.knowledgeBaseIDs) != 1 {
			t.Fatalf("combined cross-domain search scope = %v, want one KB", call.knowledgeBaseIDs)
		}
		searchDomains[call.knowledgeDomainID] = call.knowledgeBaseIDs[0]
	}
	if searchDomains[11] != "finance-kb" || searchDomains[22] != "compliance-kb" {
		t.Fatalf("search domain contexts = %#v", searchDomains)
	}

	embeddingDomains := make(map[uint64]bool, len(service.embeddingCalls))
	for _, call := range service.embeddingCalls {
		embeddingDomains[call.knowledgeDomainID] = true
	}
	if !embeddingDomains[11] || !embeddingDomains[22] {
		t.Fatalf("embedding domain contexts = %#v", embeddingDomains)
	}
}

func TestConcurrentSearchByTargetsSkipsMissingKnowledgeDomain(t *testing.T) {
	t.Parallel()

	service := &contextRecordingKnowledgeBaseService{
		stubKnowledgeBaseService: &stubKnowledgeBaseService{},
		knowledgeBases: []*types.KnowledgeBase{{
			ID:               "missing-domain-kb",
			IndexingStrategy: types.DefaultIndexingStrategy(),
		}},
		modelKeys: map[string]string{"missing-domain-kb": "shared-model"},
	}
	tool := &KnowledgeSearchTool{knowledgeBaseService: service}
	targets := types.SearchTargets{{
		Type:            types.SearchTargetTypeKnowledgeBase,
		KnowledgeBaseID: "missing-domain-kb",
	}}

	tool.concurrentSearchByTargets(context.Background(), []string{"approval"}, targets, 5, 0.1, 0.1, nil)

	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.embeddingCalls) != 0 || len(service.searchCalls) != 0 {
		t.Fatalf("missing-domain target should be skipped, embedding=%d search=%d",
			len(service.embeddingCalls), len(service.searchCalls))
	}
}
