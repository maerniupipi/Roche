package unifiedqa

import (
	"context"
	"encoding/json"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestFAQFastPathValidatorAcceptsSafeCompleteFAQ(t *testing.T) {
	model := &fakeFAQFastPathModel{response: FAQFastPathModelResponse{
		Content:     `{"eligible":true,"risks":[],"reason":"问题明确且不涉及受控风险"}`,
		ModelCallID: "faq-review",
	}}
	validator := NewFAQFastPathValidator(model, func(string) string { return "prompt" })
	candidate := completeDirectFAQCandidate("使用登录页的重置密码功能。")

	result, err := validator.Review(context.Background(), FAQFastPathReviewRequest{
		Question: "如何重置密码？", StandaloneQuery: "如何重置密码？", Candidate: candidate,
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !result.Eligible || result.ModelCallID != "faq-review" || model.calls != 1 {
		t.Fatalf("result=%+v calls=%d", result, model.calls)
	}
}

func TestFAQFastPathValidatorRejectsUnsafeOutputContract(t *testing.T) {
	model := &fakeFAQFastPathModel{response: FAQFastPathModelResponse{
		Content: `{"eligible":true,"risks":["amount_or_currency"],"reason":"包含金额"}`,
	}}
	validator := NewFAQFastPathValidator(model, func(string) string { return "prompt" })
	_, err := validator.Review(context.Background(), FAQFastPathReviewRequest{
		Question: "报销限额是多少？", Candidate: completeDirectFAQCandidate("限额为 800 元。"),
	})
	if err == nil {
		t.Fatal("Review() error = nil, want inconsistent eligibility error")
	}
}

func TestFAQFastPathValidatorRoutesRiskyFAQToFullReview(t *testing.T) {
	model := &fakeFAQFastPathModel{response: FAQFastPathModelResponse{
		Content:     `{"eligible":false,"risks":["amount_or_currency"],"reason":"答案包含金额，应走财务复核"}`,
		ModelCallID: "faq-review",
	}}
	validator := NewFAQFastPathValidator(model, func(string) string { return "prompt" })
	result, err := validator.Review(context.Background(), FAQFastPathReviewRequest{
		Question: "报销限额是多少？", Candidate: completeDirectFAQCandidate("限额为 800 元。"),
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Eligible || len(result.Risks) != 1 || result.Risks[0] != "amount_or_currency" {
		t.Fatalf("result = %+v", result)
	}
}

func TestSelectFAQFastPathCandidateRejectsConflictingDirectFAQs(t *testing.T) {
	first := completeDirectFAQCandidate("答案 A")
	first.OpaqueID = "e_1"
	second := completeDirectFAQCandidate("答案 B")
	second.OpaqueID = "e_2"
	if _, ok := selectFAQFastPathCandidate([]EvidenceCandidate{first, second}); ok {
		t.Fatal("selectFAQFastPathCandidate() accepted conflicting direct FAQs")
	}
}

func completeDirectFAQCandidate(answer string) EvidenceCandidate {
	return EvidenceCandidate{
		OpaqueID: "e_faq", ChunkID: "faq", KnowledgeBaseID: "kb", KnowledgeID: "faq-doc",
		ChunkType: string(types.ChunkTypeFAQ), RerankScore: 0.95, FAQDirectMatch: true,
		FAQ: &FAQEvidence{StandardQuestion: "如何重置密码？", Answers: []string{answer}, AnswerStrategy: types.AnswerStrategyAll},
	}
}

type fakeFAQFastPathModel struct {
	response FAQFastPathModelResponse
	err      error
	calls    int
}

func (m *fakeFAQFastPathModel) GenerateFAQFastPathReview(context.Context, FAQFastPathModelRequest) (FAQFastPathModelResponse, error) {
	m.calls++
	return m.response, m.err
}

type fakeFAQFastPathReviewer struct {
	result FAQFastPathReviewResult
	err    error
	calls  int
}

func (r *fakeFAQFastPathReviewer) Review(context.Context, FAQFastPathReviewRequest) (FAQFastPathReviewResult, error) {
	r.calls++
	return r.result, r.err
}

func faqSearchResult(t *testing.T, answer string) *types.SearchResult {
	t.Helper()
	metadata, err := json.Marshal(types.FAQChunkMetadata{
		StandardQuestion: "如何重置密码？", Answers: []string{answer}, AnswerStrategy: types.AnswerStrategyAll,
	})
	if err != nil {
		t.Fatalf("marshal FAQ metadata: %v", err)
	}
	return &types.SearchResult{
		ID: "faq", KnowledgeBaseID: "kb", KnowledgeID: "faq-doc", KnowledgeTitle: "账户帮助",
		Content: "如何重置密码？", Score: 0.8, ChunkType: string(types.ChunkTypeFAQ), ChunkMetadata: types.JSON(metadata),
	}
}
