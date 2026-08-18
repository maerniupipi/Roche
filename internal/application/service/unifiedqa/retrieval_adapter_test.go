package unifiedqa

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestRetrievalAdapterSearchesFullScopeMergesAndReranksOnce(t *testing.T) {
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"expense policy": {
			{ID: "chunk-1", KnowledgeBaseID: "kb-a", KnowledgeID: "doc-1", Content: "limit 100", Score: 0.7, MatchType: types.MatchTypeEmbedding,
				KnowledgeFilename: "expense.pdf", ChunkIndex: 2, StartAt: 10, EndAt: 20, ImageInfo: `{"page":3}`},
			{ID: "chunk-2", KnowledgeBaseID: "kb-b", KnowledgeID: "doc-2", Content: "approval", Score: 0.6, MatchType: types.MatchTypeKeywords},
		},
		"reimbursement limit": {
			{ID: "chunk-1", KnowledgeBaseID: "kb-a", KnowledgeID: "doc-1", Content: "limit 100", Score: 0.9, MatchType: types.MatchTypeKeywords},
			{ID: "outside", KnowledgeBaseID: "kb-outside", KnowledgeID: "doc-x", Content: "secret", Score: 1, MatchType: types.MatchTypeEmbedding},
		},
	}}
	reranker := &fakeEvidenceReranker{results: []rerank.RankResult{
		{Index: 1, RelevanceScore: 0.95},
		{Index: 0, RelevanceScore: 0.8},
	}}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{
		MatchCount:       20,
		VectorThreshold:  0.2,
		KeywordThreshold: 0.1,
		RerankTopK:       10,
		RerankThreshold:  0.5,
	})
	scope := AuthorizedScope{KnowledgeBaseIDs: []string{"kb-a", "kb-b"}}
	task := AgentTask{AgentID: FinanceAgentID, SearchQueries: []string{"expense policy", "reimbursement limit"}}

	result, err := adapter.Retrieve(context.Background(), task, scope, reranker, DefaultRetrievalPolicy())
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(searcher.calls) != 2 {
		t.Fatalf("HybridSearch calls = %d, want 2", len(searcher.calls))
	}
	for _, call := range searcher.calls {
		if call.primaryKBID != "kb-a" || !reflect.DeepEqual(call.params.KnowledgeBaseIDs, []string{"kb-a", "kb-b"}) {
			t.Fatalf("search call scope = %+v", call)
		}
	}
	if reranker.calls != 1 {
		t.Fatalf("Rerank calls = %d, want 1", reranker.calls)
	}
	if got, want := len(result.Candidates), 2; got != want {
		t.Fatalf("candidate count = %d, want %d", got, want)
	}
	if result.Candidates[0].ChunkID != "chunk-2" || result.Candidates[0].RerankScore != 0.95 {
		t.Fatalf("first candidate = %+v", result.Candidates[0])
	}
	chunk1 := result.Candidates[1]
	if chunk1.RetrievalScore != 0.9 || !reflect.DeepEqual(chunk1.MatchedQueries, []string{"expense policy", "reimbursement limit"}) {
		t.Fatalf("merged chunk = %+v", chunk1)
	}
	if !reflect.DeepEqual(chunk1.RetrievalChannels, []string{"embedding", "keywords"}) {
		t.Fatalf("retrieval channels = %v", chunk1.RetrievalChannels)
	}
	if chunk1.KnowledgeFilename != "expense.pdf" || chunk1.ChunkIndex != 2 || chunk1.StartAt != 10 || chunk1.EndAt != 20 || chunk1.ImageInfo == "" {
		t.Fatalf("preview metadata was not preserved: %+v", chunk1)
	}
}

func TestRetrievalAdapterGroupsByKnowledgeDomainAndPreservesPartialScope(t *testing.T) {
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{"q": nil}}
	domains := &fakeKnowledgeDomainLookup{domains: map[uint64]*types.KnowledgeDomain{
		1: {ID: 1, Name: "Domain 1"},
		2: {ID: 2, Name: "Domain 2"},
	}}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{MatchCount: 5}, domains)
	scope := AuthorizedScope{
		KnowledgeBases: []AuthorizedKnowledgeBase{
			{ID: "kb-a", KnowledgeDomainID: 1, EmbeddingModelID: "embedding-a", Type: types.KnowledgeBaseTypeDocument, FullAccess: true},
			{ID: "kb-b", KnowledgeDomainID: 2, EmbeddingModelID: "embedding-a", Type: types.KnowledgeBaseTypeDocument, FullAccess: true},
			{ID: "kb-c", KnowledgeDomainID: 2, EmbeddingModelID: "embedding-a", Type: types.KnowledgeBaseTypeDocument, KnowledgeIDs: []string{"doc-c"}},
		},
		KnowledgeBaseIDs: []string{"kb-a", "kb-b", "kb-c"},
		SearchTargets: types.SearchTargets{
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-a", KnowledgeDomainID: 1},
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-b", KnowledgeDomainID: 2},
			{Type: types.SearchTargetTypeKnowledge, KnowledgeBaseID: "kb-c", KnowledgeDomainID: 2, KnowledgeIDs: []string{"doc-c"}},
		},
	}

	_, err := adapter.Retrieve(context.Background(), AgentTask{SearchQueries: []string{"q"}}, scope, nil, DefaultRetrievalPolicy())
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(searcher.calls) != 3 {
		t.Fatalf("HybridSearch calls = %d, want 3: %+v", len(searcher.calls), searcher.calls)
	}
	if searcher.calls[0].knowledgeDomainID != 1 || searcher.calls[0].knowledgeDomainInfoID != 1 ||
		!reflect.DeepEqual(searcher.calls[0].params.KnowledgeBaseIDs, []string{"kb-a"}) {
		t.Fatalf("domain 1 call = %+v", searcher.calls[0])
	}
	if searcher.calls[1].knowledgeDomainID != 2 || !reflect.DeepEqual(searcher.calls[1].params.KnowledgeBaseIDs, []string{"kb-b"}) ||
		len(searcher.calls[1].params.KnowledgeIDs) != 0 {
		t.Fatalf("domain 2 full call = %+v", searcher.calls[1])
	}
	if searcher.calls[2].knowledgeDomainID != 2 || !reflect.DeepEqual(searcher.calls[2].params.KnowledgeBaseIDs, []string{"kb-c"}) ||
		!reflect.DeepEqual(searcher.calls[2].params.KnowledgeIDs, []string{"doc-c"}) {
		t.Fatalf("domain 2 partial call = %+v", searcher.calls[2])
	}
	if got, want := domains.requestedIDs, []uint64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requested domain IDs = %v, want %v", got, want)
	}
}

func TestRetrievalAdapterEnrichesAndPrioritizesFAQEvidence(t *testing.T) {
	faqMetadata, err := json.Marshal(types.FAQChunkMetadata{
		StandardQuestion: "差旅住宿标准是多少？",
		Answers:          []string{"一线城市每晚不超过 800 元。"},
		AnswerStrategy:   types.AnswerStrategyAll,
	})
	if err != nil {
		t.Fatalf("marshal FAQ metadata: %v", err)
	}
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"住宿标准": {
			{ID: "faq", KnowledgeBaseID: "kb-faq", KnowledgeID: "faq-doc", Content: "差旅住宿标准是多少？", Score: 0.7, ChunkType: string(types.ChunkTypeFAQ), ChunkMetadata: types.JSON(faqMetadata)},
			{ID: "doc", KnowledgeBaseID: "kb-doc", KnowledgeID: "manual", Content: "普通文档内容", Score: 0.8, ChunkType: string(types.ChunkTypeText)},
		},
	}}
	reranker := &fakeEvidenceReranker{results: []rerank.RankResult{
		{Index: 1, RelevanceScore: 0.95},
		{Index: 0, RelevanceScore: 0.85},
	}}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{RerankTopK: 5, RerankThreshold: 0.1})

	result, err := adapter.Retrieve(
		context.Background(),
		AgentTask{SearchQueries: []string{"住宿标准"}},
		AuthorizedScope{KnowledgeBaseIDs: []string{"kb-doc", "kb-faq"}},
		reranker,
		RetrievalPolicy{FAQPriorityEnabled: true, FAQDirectAnswerThreshold: 0.9, FAQScoreBoost: 1.2},
	)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].ChunkID != "faq" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	faq := result.Candidates[0]
	if faq.ChunkType != string(types.ChunkTypeFAQ) || faq.FAQ == nil || !faq.FAQDirectMatch {
		t.Fatalf("FAQ candidate = %+v", faq)
	}
	if faq.Score != 1 || faq.RerankScore != 0.95 || faq.FAQ.StandardQuestion != "差旅住宿标准是多少？" {
		t.Fatalf("FAQ scoring = %+v", faq)
	}
	if !strings.Contains(faq.Content, "一线城市每晚不超过 800 元") || !strings.Contains(reranker.passages[1], "一线城市每晚不超过 800 元") {
		t.Fatalf("FAQ answer was not enriched before rerank: candidate=%q passages=%v", faq.Content, reranker.passages)
	}
}

func TestRetrievalAdapterEnrichesQuestionOnlyFAQWithoutReranker(t *testing.T) {
	faqMetadata, _ := json.Marshal(types.FAQChunkMetadata{
		StandardQuestion: "如何报销？",
		Answers:          []string{"提交发票并完成审批。"},
	})
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"报销": {{
			ID: "faq", KnowledgeBaseID: "kb", KnowledgeID: "faq-doc", Content: "Q: 如何报销？",
			Score: 0.8, ChunkType: string(types.ChunkTypeFAQ), ChunkMetadata: types.JSON(faqMetadata),
		}},
	}}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{RerankTopK: 5})

	result, err := adapter.Retrieve(context.Background(), AgentTask{SearchQueries: []string{"报销"}},
		AuthorizedScope{KnowledgeBaseIDs: []string{"kb"}}, nil, DefaultRetrievalPolicy())
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(result.Candidates) != 1 || !strings.Contains(result.Candidates[0].Content, "提交发票并完成审批") {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if result.Candidates[0].FAQDirectMatch || result.Candidates[0].Score != 0.96 {
		t.Fatalf("FAQ priority = %+v", result.Candidates[0])
	}
}

func TestRetrievalAdapterDoesNotUseBoostedScoreForFAQDirectMatch(t *testing.T) {
	faqMetadata, _ := json.Marshal(types.FAQChunkMetadata{
		StandardQuestion: "如何报销？", Answers: []string{"提交发票并完成审批。"},
	})
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"报销": {{
			ID: "faq", KnowledgeBaseID: "kb", KnowledgeID: "faq-doc", Content: "如何报销？",
			Score: 0.8, ChunkType: string(types.ChunkTypeFAQ), ChunkMetadata: types.JSON(faqMetadata),
		}},
	}}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{RerankTopK: 5, RerankThreshold: 0.3})
	result, err := adapter.Retrieve(context.Background(), AgentTask{SearchQueries: []string{"报销"}},
		AuthorizedScope{KnowledgeBaseIDs: []string{"kb"}},
		&fakeEvidenceReranker{results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.8}}},
		RetrievalPolicy{FAQPriorityEnabled: true, FAQDirectAnswerThreshold: 0.9, FAQScoreBoost: 1.2})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Score != 0.96 || result.Candidates[0].FAQDirectMatch {
		t.Fatalf("boosted FAQ must rank higher without becoming direct match: %+v", result.Candidates)
	}
}

func TestRetrievalPolicyUsesEntryAgentFAQSettings(t *testing.T) {
	defaults := retrievalPolicyForRequest(nil)
	if !defaults.FAQPriorityEnabled || defaults.FAQDirectAnswerThreshold != 0.9 || defaults.FAQScoreBoost != 1.2 {
		t.Fatalf("default policy = %+v", defaults)
	}
	policy := retrievalPolicyForRequest(&types.QARequest{CustomAgent: &types.CustomAgent{Config: types.CustomAgentConfig{
		FAQPriorityEnabled:       false,
		FAQDirectAnswerThreshold: 0.8,
		FAQScoreBoost:            1.5,
	}}})
	if policy.FAQPriorityEnabled || policy.FAQDirectAnswerThreshold != 0.8 || policy.FAQScoreBoost != 1.5 {
		t.Fatalf("custom policy = %+v", policy)
	}
}

func TestRetrievalAdapterFallsBackWhenRerankerFails(t *testing.T) {
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"q": {{ID: "chunk", KnowledgeBaseID: "kb", KnowledgeID: "doc", Content: "evidence", Score: 0.75}},
	}}
	reranker := &fakeEvidenceReranker{err: errors.New("rerank down")}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{MatchCount: 5, RerankTopK: 5})

	result, err := adapter.Retrieve(context.Background(), AgentTask{AgentID: FinanceAgentID, SearchQueries: []string{"q"}}, AuthorizedScope{KnowledgeBaseIDs: []string{"kb"}}, reranker, DefaultRetrievalPolicy())
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if !result.RerankDegraded || len(result.Candidates) != 1 || result.Candidates[0].Score != 0.75 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRetrievalAdapterRejectsEmptyAuthorizedScopeBeforeSearch(t *testing.T) {
	searcher := &fakeHybridSearcher{}
	adapter := NewRetrievalAdapter(searcher, RetrievalSettings{})
	_, err := adapter.Retrieve(context.Background(), AgentTask{SearchQueries: []string{"q"}}, AuthorizedScope{}, nil, DefaultRetrievalPolicy())
	if !errors.Is(err, ErrNoAccessibleKnowledgeBase) {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(searcher.calls) != 0 {
		t.Fatalf("HybridSearch calls = %d, want 0", len(searcher.calls))
	}
}

type hybridSearchCall struct {
	primaryKBID           string
	params                types.SearchParams
	knowledgeDomainID     uint64
	knowledgeDomainInfoID uint64
}

type fakeHybridSearcher struct {
	results map[string][]*types.SearchResult
	calls   []hybridSearchCall
	err     error
}

func (f *fakeHybridSearcher) HybridSearch(ctx context.Context, primaryKBID string, params types.SearchParams) ([]*types.SearchResult, error) {
	domainID, _ := ctx.Value(types.KnowledgeDomainIDContextKey).(uint64)
	domainInfo, _ := ctx.Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain)
	domainInfoID := uint64(0)
	if domainInfo != nil {
		domainInfoID = domainInfo.ID
	}
	f.calls = append(f.calls, hybridSearchCall{
		primaryKBID: primaryKBID, params: params,
		knowledgeDomainID: domainID, knowledgeDomainInfoID: domainInfoID,
	})
	return f.results[params.QueryText], f.err
}

type fakeKnowledgeDomainLookup struct {
	domains      map[uint64]*types.KnowledgeDomain
	requestedIDs []uint64
	err          error
}

func (f *fakeKnowledgeDomainLookup) GetKnowledgeDomainsByIDs(
	_ context.Context,
	ids []uint64,
) (map[uint64]*types.KnowledgeDomain, error) {
	f.requestedIDs = append([]uint64(nil), ids...)
	return f.domains, f.err
}

type fakeEvidenceReranker struct {
	calls    int
	query    string
	passages []string
	results  []rerank.RankResult
	err      error
}

func (f *fakeEvidenceReranker) Rerank(_ context.Context, query string, passages []string) ([]rerank.RankResult, error) {
	f.calls++
	f.query = query
	f.passages = passages
	return f.results, f.err
}

func (f *fakeEvidenceReranker) GetModelName() string { return "fake" }
func (f *fakeEvidenceReranker) GetModelID() string   { return "fake" }
