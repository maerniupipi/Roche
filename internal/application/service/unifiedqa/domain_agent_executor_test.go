package unifiedqa

import (
	"context"
	"strings"
	"testing"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
)

func TestDomainAgentExecutorRecoversOnceAndReviewsTwice(t *testing.T) {
	retriever := &fakeDomainRetriever{result: RetrievalResult{Candidates: []EvidenceCandidate{{OpaqueID: "e_1"}}, ToolCalls: 2}}
	reviewer := &sequenceDomainReviewer{decisions: []ReviewDecision{
		{Observation: AgentObservation{AgentID: FinanceAgentID, Status: EvidenceStatusInsufficient, RecoveryRequest: &RecoveryRequest{Tool: "knowledge_search", Query: "date"}}, ModelCallID: "review-1"},
		{Observation: AgentObservation{AgentID: FinanceAgentID, Status: EvidenceStatusSufficient, Facts: []ObservedFact{{Statement: "fact", Citations: []EvidenceCitation{{OpaqueID: "e_2"}}}}}, ModelCallID: "review-2"},
	}}
	recoverer := &fakeDomainRecoverer{result: RetrievalResult{Candidates: []EvidenceCandidate{{OpaqueID: "e_1"}, {OpaqueID: "e_2"}}, ToolCalls: 1}}
	executor := NewDomainAgentExecutor(mustTestCatalog(t), retriever, reviewer, recoverer)
	var stages []DomainProgressStage

	result, err := executor.Execute(context.Background(), "Q", AgentTask{AgentID: FinanceAgentID}, AuthorizedScope{}, "model", nil, DefaultRetrievalPolicy(),
		func(stage DomainProgressStage) { stages = append(stages, stage) })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ReviewCalls != 2 || result.ToolCalls != 3 || recoverer.calls != 1 || reviewer.calls != 2 {
		t.Fatalf("result=%+v recovery calls=%d review calls=%d", result, recoverer.calls, reviewer.calls)
	}
	wantStages := []DomainProgressStage{DomainProgressRetrieving, DomainProgressReviewing, DomainProgressRecovering, DomainProgressReviewingRecovery}
	if len(stages) != len(wantStages) {
		t.Fatalf("progress stages = %v, want %v", stages, wantStages)
	}
	for i := range wantStages {
		if stages[i] != wantStages[i] {
			t.Fatalf("progress stages = %v, want %v", stages, wantStages)
		}
	}
}

func TestRecordReviewDecisionCountsTruncationRetry(t *testing.T) {
	result := DomainExecutionResult{}
	recordReviewDecision(&result, ReviewDecision{
		ModelCallID:  "review-retry",
		ModelCallIDs: []string{"review-truncated", "review-retry"},
		ModelCalls:   2,
	})
	if result.ReviewCalls != 2 || len(result.ModelCallIDs) != 2 || result.ModelCallIDs[1] != "review-retry" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDomainAgentExecutorInjectsOneRecoveryForReportedCoverageGap(t *testing.T) {
	retriever := &fakeDomainRetriever{result: RetrievalResult{Candidates: []EvidenceCandidate{{OpaqueID: "e_1"}}, ToolCalls: 1}}
	reviewer := &sequenceDomainReviewer{decisions: []ReviewDecision{
		{Observation: AgentObservation{
			AgentID: FinanceAgentID, Status: EvidenceStatusSufficient,
			Facts:               []ObservedFact{{Statement: "partial", Citations: []EvidenceCitation{{OpaqueID: "e_1"}}}},
			MissingRequirements: []string{"approval evidence and required supporting documents"},
		}},
		{Observation: AgentObservation{
			AgentID: FinanceAgentID, Status: EvidenceStatusSufficient,
			Facts: []ObservedFact{{Statement: "complete", Citations: []EvidenceCitation{{OpaqueID: "e_2"}}}},
		}},
	}}
	recoverer := &fakeDomainRecoverer{result: RetrievalResult{
		Candidates: []EvidenceCandidate{{OpaqueID: "e_1"}, {OpaqueID: "e_2"}}, ToolCalls: 1,
	}}
	executor := NewDomainAgentExecutor(mustTestCatalog(t), retriever, reviewer, recoverer)

	result, err := executor.Execute(
		context.Background(),
		"How do I reimburse this expense?",
		AgentTask{AgentID: FinanceAgentID, Goal: "Find the complete process", SearchQueries: []string{"expense reimbursement"}},
		AuthorizedScope{}, "model", nil, DefaultRetrievalPolicy(), nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if recoverer.calls != 1 || reviewer.calls != 2 || result.ReviewCalls != 2 {
		t.Fatalf("result=%+v recovery calls=%d review calls=%d", result, recoverer.calls, reviewer.calls)
	}
	if recoverer.lastRequest.Tool != "knowledge_search" ||
		!strings.Contains(recoverer.lastRequest.Query, "How do I reimburse this expense?") ||
		!strings.Contains(recoverer.lastRequest.Query, "supporting documents") {
		t.Fatalf("injected recovery request = %+v", recoverer.lastRequest)
	}
}

func TestBuildCoverageRecoveryQueriesSeparatesSpecificGaps(t *testing.T) {
	queries := buildCoverageRecoveryQueries("HCP 活动结束后需要做什么？", []string{
		"缺少抽查方法和频率",
		"bounded evidence recovery budget exhausted",
		"缺少留档材料和保存期限",
		"缺少异常处理流程",
	}, "")
	if len(queries) != 3 {
		t.Fatalf("queries = %v", queries)
	}
	if !strings.Contains(queries[0], "抽查方法和频率") ||
		!strings.Contains(queries[1], "留档材料和保存期限") ||
		!strings.Contains(queries[2], "异常处理流程") {
		t.Fatalf("queries = %v", queries)
	}
	for _, query := range queries {
		if strings.Contains(query, "bounded evidence") {
			t.Fatalf("internal gap leaked into query: %q", query)
		}
	}
}

type fakeDomainRetriever struct{ result RetrievalResult }

func (f *fakeDomainRetriever) Retrieve(context.Context, AgentTask, AuthorizedScope, rerank.Reranker, RetrievalPolicy) (RetrievalResult, error) {
	return f.result, nil
}

type sequenceDomainReviewer struct {
	decisions []ReviewDecision
	calls     int
}

func (f *sequenceDomainReviewer) Review(context.Context, EvidenceReviewRequest) (ReviewDecision, error) {
	decision := f.decisions[f.calls]
	f.calls++
	return decision, nil
}

type fakeDomainRecoverer struct {
	result      RetrievalResult
	calls       int
	lastRequest RecoveryRequest
}

func (f *fakeDomainRecoverer) Recover(_ context.Context, request RecoveryRequest, _ AuthorizedScope, _ []EvidenceCandidate, _ int, _ rerank.Reranker, _ RetrievalPolicy) (RetrievalResult, error) {
	f.calls++
	f.lastRequest = request
	return f.result, nil
}
