package unifiedqa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"roche.local/knowledge-agent-platform/internal/types"
)

const faqFastPathPromptID = "unified-faq-fast-path-v1"

var allowedFAQFastPathRisks = map[string]struct{}{
	"time_sensitive":           {},
	"amount_or_currency":       {},
	"regulatory_or_compliance": {},
	"conflicting_evidence":     {},
	"scope_or_condition":       {},
	"ambiguous_match":          {},
	"incomplete_answer":        {},
}

type FAQFastPathReviewRequest struct {
	Question        string
	StandaloneQuery string
	Candidate       EvidenceCandidate
	Alternatives    []EvidenceCandidate
	ModelID         string
}

type FAQFastPathReviewResult struct {
	Eligible    bool     `json:"eligible"`
	Risks       []string `json:"risks"`
	Reason      string   `json:"reason"`
	ModelCallID string   `json:"-"`
}

type FAQFastPathReviewer interface {
	Review(ctx context.Context, request FAQFastPathReviewRequest) (FAQFastPathReviewResult, error)
}

type FAQFastPathModelRequest struct {
	SystemPrompt    string
	Question        string
	StandaloneQuery string
	Candidate       EvidenceCandidate
	Alternatives    []EvidenceCandidate
	ModelID         string
}

type FAQFastPathModelResponse struct {
	Content     string
	ModelCallID string
}

type FAQFastPathModel interface {
	GenerateFAQFastPathReview(ctx context.Context, request FAQFastPathModelRequest) (FAQFastPathModelResponse, error)
}

type FAQFastPathValidator struct {
	model         FAQFastPathModel
	resolvePrompt PromptContentResolver
}

func NewFAQFastPathValidator(model FAQFastPathModel, resolvePrompt PromptContentResolver) *FAQFastPathValidator {
	return &FAQFastPathValidator{model: model, resolvePrompt: resolvePrompt}
}

func (v *FAQFastPathValidator) Review(ctx context.Context, request FAQFastPathReviewRequest) (FAQFastPathReviewResult, error) {
	if v == nil || v.model == nil || v.resolvePrompt == nil {
		return FAQFastPathReviewResult{}, fmt.Errorf("FAQ fast-path validator is not configured")
	}
	if !isCompleteDirectFAQ(request.Candidate) {
		return FAQFastPathReviewResult{}, fmt.Errorf("FAQ fast-path candidate is incomplete or not a direct match")
	}
	prompt := strings.TrimSpace(v.resolvePrompt(faqFastPathPromptID))
	if prompt == "" {
		return FAQFastPathReviewResult{}, fmt.Errorf("FAQ fast-path prompt is not configured")
	}
	response, err := v.model.GenerateFAQFastPathReview(ctx, FAQFastPathModelRequest{
		SystemPrompt: prompt, Question: request.Question, StandaloneQuery: request.StandaloneQuery,
		Candidate: cloneEvidenceCandidate(request.Candidate), Alternatives: slices.Clone(request.Alternatives), ModelID: request.ModelID,
	})
	if err != nil {
		return FAQFastPathReviewResult{ModelCallID: response.ModelCallID}, err
	}
	result, err := decodeFAQFastPathReview(response.Content)
	result.ModelCallID = response.ModelCallID
	return result, err
}

func decodeFAQFastPathReview(content string) (FAQFastPathReviewResult, error) {
	if len(content) == 0 || len(content) > maxRouteResponseBytes {
		return FAQFastPathReviewResult{}, fmt.Errorf("FAQ fast-path review output size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var result FAQFastPathReviewResult
	if err := decoder.Decode(&result); err != nil {
		return FAQFastPathReviewResult{}, fmt.Errorf("decode FAQ fast-path review: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return FAQFastPathReviewResult{}, fmt.Errorf("FAQ fast-path review contains trailing JSON")
	}
	if strings.TrimSpace(result.Reason) == "" {
		return FAQFastPathReviewResult{}, fmt.Errorf("FAQ fast-path review reason is required")
	}
	seen := make(map[string]struct{}, len(result.Risks))
	for _, risk := range result.Risks {
		if _, ok := allowedFAQFastPathRisks[risk]; !ok {
			return FAQFastPathReviewResult{}, fmt.Errorf("invalid FAQ fast-path risk %q", risk)
		}
		if _, duplicate := seen[risk]; duplicate {
			return FAQFastPathReviewResult{}, fmt.Errorf("duplicate FAQ fast-path risk %q", risk)
		}
		seen[risk] = struct{}{}
	}
	if result.Eligible && len(result.Risks) > 0 {
		return FAQFastPathReviewResult{}, fmt.Errorf("eligible FAQ fast-path review cannot contain risks")
	}
	if !result.Eligible && len(result.Risks) == 0 {
		return FAQFastPathReviewResult{}, fmt.Errorf("ineligible FAQ fast-path review must identify at least one risk")
	}
	return result, nil
}

func isCompleteDirectFAQ(candidate EvidenceCandidate) bool {
	if !candidate.FAQDirectMatch || candidate.FAQ == nil || strings.TrimSpace(candidate.FAQ.StandardQuestion) == "" {
		return false
	}
	for _, answer := range candidate.FAQ.Answers {
		if strings.TrimSpace(answer) != "" {
			return true
		}
	}
	return false
}

func selectFAQFastPathCandidate(candidates []EvidenceCandidate) (EvidenceCandidate, bool) {
	var selected EvidenceCandidate
	found := false
	for _, candidate := range candidates {
		if !isCompleteDirectFAQ(candidate) {
			continue
		}
		if !found {
			selected, found = cloneEvidenceCandidate(candidate), true
			continue
		}
		// Multiple high-confidence FAQ entries with different answers are a
		// deterministic conflict and must fall back to full domain review.
		if normalizedFAQAnswers(selected.FAQ.Answers) != normalizedFAQAnswers(candidate.FAQ.Answers) {
			return EvidenceCandidate{}, false
		}
	}
	return selected, found
}

func normalizedFAQAnswers(answers []string) string {
	normalized := make([]string, 0, len(answers))
	for _, answer := range answers {
		if answer = strings.TrimSpace(answer); answer != "" {
			normalized = append(normalized, strings.Join(strings.Fields(strings.ToLower(answer)), " "))
		}
	}
	slices.Sort(normalized)
	return strings.Join(normalized, "\x00")
}

func renderFAQFastPathAnswer(faq *FAQEvidence, selector string) string {
	if faq == nil {
		return ""
	}
	answers := make([]string, 0, len(faq.Answers))
	for _, answer := range faq.Answers {
		if answer = strings.TrimSpace(answer); answer != "" {
			answers = append(answers, answer)
		}
	}
	if len(answers) == 0 {
		return ""
	}
	if faq.AnswerStrategy == types.AnswerStrategyRandom && len(answers) > 1 {
		hash := sha256.Sum256([]byte(selector))
		return answers[int(hash[0])%len(answers)]
	}
	return strings.Join(answers, "\n\n")
}
