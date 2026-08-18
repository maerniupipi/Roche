package unifiedqa

import (
	"context"
	"fmt"
	"strings"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
)

type EvidenceRecoveryExecutor struct {
	retrieval *RetrievalAdapter
	maxCalls  int
}

func NewEvidenceRecoveryExecutor(retrieval *RetrievalAdapter, maxCalls int) *EvidenceRecoveryExecutor {
	if maxCalls <= 0 {
		maxCalls = 5
	}
	return &EvidenceRecoveryExecutor{retrieval: retrieval, maxCalls: maxCalls}
}

func (e *EvidenceRecoveryExecutor) Recover(
	ctx context.Context,
	request RecoveryRequest,
	scope AuthorizedScope,
	existing []EvidenceCandidate,
	usedCalls int,
	rerankerModel rerank.Reranker,
	policy RetrievalPolicy,
) (RetrievalResult, error) {
	if e == nil || e.retrieval == nil {
		return RetrievalResult{}, fmt.Errorf("evidence recovery executor is not configured")
	}
	if usedCalls >= e.maxCalls {
		return RetrievalResult{}, fmt.Errorf("research call budget exhausted")
	}
	if request.Tool != "knowledge_search" && request.Tool != "grep_chunks" {
		return RetrievalResult{}, fmt.Errorf("recovery tool %q is not executable by bounded retrieval", request.Tool)
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return RetrievalResult{}, fmt.Errorf("recovery query is required")
	}
	queries := []string{query}
	if request.Tool == "knowledge_search" && len(request.Queries) > 0 {
		queries = make([]string, 0, min(3, len(request.Queries)))
		for _, candidate := range request.Queries {
			candidate = strings.TrimSpace(candidate)
			if candidate != "" {
				queries = append(queries, candidate)
			}
			if len(queries) == 3 {
				break
			}
		}
		if len(queries) == 0 {
			queries = []string{query}
		}
	}
	if request.Tool == "grep_chunks" && len(request.Terms) > 0 {
		query = strings.Join(request.Terms, "|")
		queries = []string{query}
	}
	remainingCalls := e.maxCalls - usedCalls
	if remainingCalls <= 0 {
		return RetrievalResult{}, fmt.Errorf("research call budget exhausted")
	}
	if len(queries) > remainingCalls {
		queries = queries[:remainingCalls]
	}
	return e.retrieval.RetrieveWithExisting(ctx, AgentTask{
		SearchQueries: queries,
		ToolIntent:    request.Tool,
	}, scope, rerankerModel, existing, policy)
}
