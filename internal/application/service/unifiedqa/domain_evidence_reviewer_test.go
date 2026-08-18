package unifiedqa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDomainEvidenceReviewerAcceptsCitedFacts(t *testing.T) {
	model := &fakeReviewModel{response: ReviewModelResponse{ModelCallID: "review-1", Content: `{
  "agent_id":"finance",
  "status":"sufficient",
  "facts":[{"statement":"The limit is 100.","quote":"limit 100","is_ambiguous":false,"scenario":"","document_level":"unspecified","currency":"unspecified","citations":[{"opaque_id":"e_1","quote":"limit 100"}]}],
  "missing_requirements":[],
  "conflicts":[]
}`}}
	reviewer := NewDomainEvidenceReviewer(model, func(id string) string { return "prompt:" + id })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)

	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
		Question:   "What is the limit?",
		Task:       AgentTask{AgentID: FinanceAgentID, Goal: "find limit"},
		Profile:    profile,
		Candidates: []EvidenceCandidate{{OpaqueID: "e_1", Content: "limit 100"}},
		Attempt:    0,
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if decision.ModelCallID != "review-1" || decision.Observation.Status != EvidenceStatusSufficient || len(decision.Observation.Facts) != 1 {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDomainEvidenceReviewerAllowsOneBoundedRecoveryRequest(t *testing.T) {
	model := &fakeReviewModel{response: ReviewModelResponse{Content: `{
  "agent_id":"finance",
  "status":"insufficient",
  "facts":[],
  "missing_requirements":["effective date"],
  "conflicts":[],
  "recovery_request":{"tool":"knowledge_search","query":"expense policy effective date","terms":[]}
}`}}
	reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)

	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{Question: "Q", Profile: profile, Attempt: 0})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if decision.Observation.RecoveryRequest == nil || decision.Observation.RecoveryRequest.Query == "" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDomainEvidenceReviewerRejectsInvalidCitationsAndUnapprovedRecovery(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		content string
	}{
		{name: "unknown citation", content: `{"agent_id":"finance","status":"sufficient","facts":[{"statement":"x","citations":[{"opaque_id":"not-input"}]}],"missing_requirements":[],"conflicts":[]}`},
		{name: "unapproved tool", content: `{"agent_id":"finance","status":"insufficient","facts":[],"missing_requirements":["x"],"conflicts":[],"recovery_request":{"tool":"web_search","query":"web"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &fakeReviewModel{response: ReviewModelResponse{Content: tt.content}}
			reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
			profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
			_, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
				Question: "Q", Profile: profile, Attempt: tt.attempt,
				Candidates: []EvidenceCandidate{{OpaqueID: "e_1"}},
			})
			if err == nil {
				t.Fatal("Review() error = nil")
			}
		})
	}
}

func TestDomainEvidenceReviewerSuppressesSecondRecoveryAndPreservesFacts(t *testing.T) {
	model := &fakeReviewModel{response: ReviewModelResponse{Content: `{
  "agent_id":"finance",
  "status":"insufficient",
  "facts":[{"statement":"The meal allowance is 120.","quote":"meal allowance is 120","is_ambiguous":false,"scenario":"travel","document_level":"formal_policy","currency":"RMB","citations":[{"opaque_id":"e_1"}]}],
  "missing_requirements":["effective date"],
  "conflicts":[],
  "recovery_request":{"tool":"knowledge_search","query":"search again"}
}`}}
	reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)

	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
		Question: "Q", Profile: profile, Attempt: 1,
		Candidates: []EvidenceCandidate{{OpaqueID: "e_1", Content: "The meal allowance is 120 for business travel."}},
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if decision.Observation.RecoveryRequest != nil || len(decision.Observation.Facts) != 1 {
		t.Fatalf("observation = %+v", decision.Observation)
	}
	if decision.Observation.Metrics["second_recovery_request_suppressed"] != true {
		t.Fatalf("metrics = %+v", decision.Observation.Metrics)
	}
	if !strings.Contains(strings.Join(decision.Observation.MissingRequirements, " "), "recovery budget exhausted") {
		t.Fatalf("missing requirements = %+v", decision.Observation.MissingRequirements)
	}
	if !strings.Contains(model.request.SystemPrompt, finalRecoveryReviewInstruction) {
		t.Fatalf("system prompt = %q", model.request.SystemPrompt)
	}
}

func TestDomainEvidenceReviewerPropagatesModelFailure(t *testing.T) {
	want := errors.New("model down")
	reviewer := NewDomainEvidenceReviewer(&fakeReviewModel{err: want}, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
	_, err := reviewer.Review(context.Background(), EvidenceReviewRequest{Question: "Q", Profile: profile})
	if !errors.Is(err, want) {
		t.Fatalf("Review() error = %v, want %v", err, want)
	}
}

func TestDomainEvidenceReviewerRequiresVerifiedQuoteForAmbiguousFact(t *testing.T) {
	tests := []struct {
		name    string
		quote   string
		wantErr bool
	}{
		{name: "verified quote", quote: "limited quantities", wantErr: false},
		{name: "invented quote", quote: "three times per year", wantErr: true},
		{name: "missing quote", quote: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `{"agent_id":"finance","status":"sufficient","facts":[{"statement":"The policy permits limited quantities.","quote":"` + tt.quote + `","is_ambiguous":true,"scenario":"gifts","document_level":"internal_sop","currency":"unspecified","citations":[{"opaque_id":"e_1"}]}],"missing_requirements":[],"conflicts":[]}`
			model := &fakeReviewModel{response: ReviewModelResponse{Content: content}}
			reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
			profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
			_, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
				Question: "Q", Profile: profile,
				Candidates: []EvidenceCandidate{{OpaqueID: "e_1", Content: "The policy permits limited quantities of gifts."}},
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Review() error = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestDomainEvidenceReviewerAcceptsChineseQuoteSplitByOCRLineBreak(t *testing.T) {
	model := &fakeReviewModel{response: ReviewModelResponse{Content: `{
  "agent_id":"compliance",
  "status":"sufficient",
  "facts":[{"statement":"礼品禁令适用于HCP工作人员。","quote":"向医疗保健专业人员提供的物品，并须遵守本准则。","is_ambiguous":false,"scenario":"礼品","document_level":"unspecified","currency":"unspecified","citations":[{"opaque_id":"e_1"}]}],
  "missing_requirements":[],
  "conflicts":[]
}`}}
	reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(ComplianceAgentID)

	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
		Question: "礼品禁令是否适用于HCP工作人员？", Profile: profile,
		Candidates: []EvidenceCandidate{{OpaqueID: "e_1", Content: "向医疗保健专业人员提供的物\n品，并须遵守本准则。"}},
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if len(decision.Observation.Facts) != 1 {
		t.Fatalf("facts = %+v", decision.Observation.Facts)
	}
}

func TestDomainEvidenceReviewerRejectsOnlyInvalidFacts(t *testing.T) {
	model := &fakeReviewModel{response: ReviewModelResponse{Content: `{
  "agent_id":"finance",
  "status":"sufficient",
  "facts":[
    {"statement":"需要发票。","quote":"需要提供发票","is_ambiguous":false,"scenario":"报销","document_level":"unspecified","currency":"unspecified","citations":[{"opaque_id":"e_1"}]},
    {"statement":"需要结账单。","quote":"需要提供发票...3.需要结账单","is_ambiguous":false,"scenario":"报销","document_level":"unspecified","currency":"unspecified","citations":[{"opaque_id":"e_1"}]}
  ],
  "missing_requirements":[],
  "conflicts":[]
}`}}
	reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)

	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
		Question: "报销需要什么？", Profile: profile,
		Candidates: []EvidenceCandidate{{OpaqueID: "e_1", Content: "需要提供发票。1.餐费标准。2.酒水标准。3.需要结账单。"}},
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if len(decision.Observation.Facts) != 1 || decision.Observation.Facts[0].Statement != "需要发票。" {
		t.Fatalf("facts = %+v", decision.Observation.Facts)
	}
	if decision.Observation.Metrics["rejected_fact_count"] != 1 {
		t.Fatalf("metrics = %+v", decision.Observation.Metrics)
	}
	details, ok := decision.Observation.Metrics["rejected_fact_details"].([]string)
	if !ok || len(details) != 1 || !strings.Contains(details[0], "fact 1: quote is not present") {
		t.Fatalf("rejected fact details = %#v", decision.Observation.Metrics["rejected_fact_details"])
	}
}

func TestNormalizeEvidenceTextPreservesMeaningfulWordSpaces(t *testing.T) {
	got := normalizeEvidenceText("医疗保健专\n业人员 must\nfollow rules")
	want := "医疗保健专业人员 must follow rules"
	if got != want {
		t.Fatalf("normalizeEvidenceText() = %q, want %q", got, want)
	}
}

func TestDomainEvidenceReviewerLabelsOutputValidationFailure(t *testing.T) {
	model := &fakeReviewModel{response: ReviewModelResponse{ModelCallID: "review-invalid", Content: `{
  "agent_id":"finance",
  "status":"sufficient",
  "facts":[{"statement":"x","quote":"invented quote","is_ambiguous":false,"scenario":"","document_level":"unspecified","currency":"unspecified","citations":[{"opaque_id":"e_1"}]}],
  "missing_requirements":[],
  "conflicts":[]
}`}}
	reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
		Question: "Q", Profile: profile,
		Candidates: []EvidenceCandidate{{OpaqueID: "e_1", Content: "actual evidence"}},
	})
	if err == nil || !strings.Contains(err.Error(), "validate evidence review output: all evidence review facts are invalid: fact 0: quote is not present") {
		t.Fatalf("Review() error = %v", err)
	}
	if decision.ModelCallID != "review-invalid" {
		t.Fatalf("ModelCallID = %q", decision.ModelCallID)
	}
}

func TestDomainEvidenceReviewerRetriesTruncatedOutputWithSmallerCandidateSet(t *testing.T) {
	candidates := make([]EvidenceCandidate, 20)
	for i := range candidates {
		candidates[i] = EvidenceCandidate{OpaqueID: fmt.Sprintf("e_%02d", i), Content: fmt.Sprintf("evidence %d", i)}
	}
	model := &fakeReviewModel{responses: []ReviewModelResponse{
		{ModelCallID: "review-truncated", Content: `{"agent_id":"finance","status":"sufficient","facts":[`},
		{ModelCallID: "review-retry", Content: `{"agent_id":"finance","status":"insufficient","facts":[],"missing_requirements":["direct policy evidence"],"conflicts":[]}`},
	}}
	reviewer := NewDomainEvidenceReviewer(model, func(string) string { return "prompt" })
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)

	decision, err := reviewer.Review(context.Background(), EvidenceReviewRequest{
		Question: "Q", Profile: profile, Candidates: candidates,
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if model.calls != 2 || len(model.requests) != 2 {
		t.Fatalf("model calls = %d, requests = %d", model.calls, len(model.requests))
	}
	if got := len(model.requests[0].Candidates); got != evidenceReviewCandidateLimit {
		t.Fatalf("primary candidate count = %d, want %d", got, evidenceReviewCandidateLimit)
	}
	if got := len(model.requests[1].Candidates); got != evidenceReviewRetryCandidateLimit {
		t.Fatalf("retry candidate count = %d, want %d", got, evidenceReviewRetryCandidateLimit)
	}
	if !strings.Contains(model.requests[1].SystemPrompt, truncatedReviewRetryInstruction) {
		t.Fatalf("retry system prompt = %q", model.requests[1].SystemPrompt)
	}
	if decision.ModelCalls != 2 || len(decision.ModelCallIDs) != 2 || decision.ModelCallID != "review-retry" {
		t.Fatalf("decision model calls = %+v", decision)
	}
	if decision.Observation.Metrics["original_candidate_count"] != 20 ||
		decision.Observation.Metrics["reviewed_candidate_count"] != evidenceReviewRetryCandidateLimit ||
		decision.Observation.Metrics["truncation_retried"] != true {
		t.Fatalf("review metrics = %+v", decision.Observation.Metrics)
	}
}

type fakeReviewModel struct {
	calls     int
	request   ReviewModelRequest
	requests  []ReviewModelRequest
	responses []ReviewModelResponse
	response  ReviewModelResponse
	err       error
}

func (f *fakeReviewModel) GenerateReview(_ context.Context, request ReviewModelRequest) (ReviewModelResponse, error) {
	call := f.calls
	f.calls++
	f.request = request
	f.requests = append(f.requests, request)
	if call < len(f.responses) {
		return f.responses[call], f.err
	}
	return f.response, f.err
}
