package unifiedqa

import (
	"context"
	"testing"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestEvidenceRecoveryMergesExistingAndReranksOnce(t *testing.T) {
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"effective date": {{ID: "new", KnowledgeBaseID: "kb", KnowledgeID: "doc-2", Content: "effective 2026", Score: 0.8}},
	}}
	rerankerModel := &fakeEvidenceReranker{results: []rerank.RankResult{{Index: 1, RelevanceScore: 0.9}, {Index: 0, RelevanceScore: 0.7}}}
	executor := NewEvidenceRecoveryExecutor(NewRetrievalAdapter(searcher, RetrievalSettings{RerankTopK: 5}), 5)

	result, err := executor.Recover(context.Background(), RecoveryRequest{Tool: "knowledge_search", Query: "effective date"},
		AuthorizedScope{KnowledgeBaseIDs: []string{"kb"}},
		[]EvidenceCandidate{{OpaqueID: "old", KnowledgeBaseID: "kb", KnowledgeID: "doc-1", ChunkID: "old", Content: "limit", RetrievalScore: 0.9, Score: 0.9}},
		3, rerankerModel, DefaultRetrievalPolicy())
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if rerankerModel.calls != 1 || result.ToolCalls != 1 || len(result.Candidates) != 2 {
		t.Fatalf("result=%+v rerank calls=%d", result, rerankerModel.calls)
	}
}

func TestEvidenceRecoveryEnforcesBudgetAndToolBoundary(t *testing.T) {
	executor := NewEvidenceRecoveryExecutor(NewRetrievalAdapter(&fakeHybridSearcher{}, RetrievalSettings{}), 5)
	for _, tc := range []struct {
		request RecoveryRequest
		used    int
	}{
		{request: RecoveryRequest{Tool: "knowledge_search", Query: "q"}, used: 5},
		{request: RecoveryRequest{Tool: "web_search", Query: "q"}, used: 0},
	} {
		if _, err := executor.Recover(context.Background(), tc.request, AuthorizedScope{KnowledgeBaseIDs: []string{"kb"}}, nil, tc.used, nil, DefaultRetrievalPolicy()); err == nil {
			t.Fatalf("Recover(%+v, used=%d) error = nil", tc.request, tc.used)
		}
	}
}

func TestEvidenceRecoveryUsesFocusedQueriesWithinRemainingBudget(t *testing.T) {
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"approval":  {{ID: "approval", KnowledgeBaseID: "kb", KnowledgeID: "doc-1", Content: "approval"}},
		"documents": {{ID: "documents", KnowledgeBaseID: "kb", KnowledgeID: "doc-2", Content: "documents"}},
		"deadline":  {{ID: "deadline", KnowledgeBaseID: "kb", KnowledgeID: "doc-3", Content: "deadline"}},
	}}
	executor := NewEvidenceRecoveryExecutor(NewRetrievalAdapter(searcher, RetrievalSettings{RerankTopK: 5}), 5)
	result, err := executor.Recover(
		context.Background(),
		RecoveryRequest{Tool: "knowledge_search", Query: "approval", Queries: []string{"approval", "documents", "deadline"}},
		AuthorizedScope{KnowledgeBaseIDs: []string{"kb"}}, nil, 3, nil, DefaultRetrievalPolicy(),
	)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if result.ToolCalls != 2 || len(result.Candidates) != 2 {
		t.Fatalf("result = %+v", result)
	}
}
