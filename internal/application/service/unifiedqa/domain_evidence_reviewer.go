package unifiedqa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
)

const (
	EvidenceStatusSufficient   = "sufficient"
	EvidenceStatusInsufficient = "insufficient"

	evidenceReviewCandidateLimit      = 12
	evidenceReviewRetryCandidateLimit = 6
)

const truncatedReviewRetryInstruction = `上一次证据复核 JSON 因输出截断而无法解析。本次只返回最关键且不重复的事实，facts 最多 6 条；quote 使用支持事实的最短完整原文。不要解释、不要重复候选原文，只返回符合 schema 的完整 JSON。`

const finalRecoveryReviewInstruction = `This is the final evidence review after the single allowed bounded recovery. Do not request another recovery. Return the best supported facts from the current candidates, report any remaining gaps in missing_requirements, and set recovery_request to null or omit it.`

type EvidenceReviewRequest struct {
	Question   string
	Task       AgentTask
	Profile    DomainAgentProfile
	Candidates []EvidenceCandidate
	Attempt    int
	ModelID    string
}

type ReviewModelRequest struct {
	SystemPrompt string
	Question     string
	Task         AgentTask
	Profile      DomainAgentProfile
	Candidates   []EvidenceCandidate
	Attempt      int
	ModelID      string
}

type ReviewModelResponse struct {
	Content     string
	ModelCallID string
}

type ReviewModel interface {
	GenerateReview(ctx context.Context, request ReviewModelRequest) (ReviewModelResponse, error)
}

type ReviewDecision struct {
	Observation  AgentObservation
	ModelCallID  string
	ModelCallIDs []string
	ModelCalls   int
}

type PromptContentResolver func(id string) string

type DomainEvidenceReviewer struct {
	model         ReviewModel
	resolvePrompt PromptContentResolver
}

func NewDomainEvidenceReviewer(model ReviewModel, resolvePrompt PromptContentResolver) *DomainEvidenceReviewer {
	return &DomainEvidenceReviewer{model: model, resolvePrompt: resolvePrompt}
}

func (r *DomainEvidenceReviewer) Review(ctx context.Context, request EvidenceReviewRequest) (ReviewDecision, error) {
	if r == nil || r.model == nil || r.resolvePrompt == nil {
		return ReviewDecision{}, fmt.Errorf("domain evidence reviewer is not configured")
	}
	agentPrompt := strings.TrimSpace(r.resolvePrompt(request.Profile.SystemPromptVersion))
	contractPrompt := strings.TrimSpace(r.resolvePrompt("domain-evidence-review-v1"))
	if agentPrompt == "" || contractPrompt == "" {
		return ReviewDecision{}, fmt.Errorf("evidence review prompt cannot be resolved for agent %q", request.Profile.ID)
	}
	systemPrompt := agentPrompt + "\n\n" + contractPrompt
	if request.Attempt > 0 {
		systemPrompt += "\n\n" + finalRecoveryReviewInstruction
	}
	primaryRequest := request
	primaryRequest.Candidates = selectEvidenceReviewCandidates(request.Candidates, evidenceReviewCandidateLimit)
	decision, err := r.reviewOnce(ctx, primaryRequest, systemPrompt)
	if err == nil {
		setEvidenceReviewMetrics(&decision.Observation, len(request.Candidates), len(primaryRequest.Candidates), false)
		return decision, nil
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		return decision, err
	}

	retryRequest := request
	retryRequest.Candidates = selectEvidenceReviewCandidates(primaryRequest.Candidates, evidenceReviewRetryCandidateLimit)
	retryDecision, retryErr := r.reviewOnce(ctx, retryRequest, systemPrompt+"\n\n"+truncatedReviewRetryInstruction)
	retryDecision = combineReviewDecisions(decision, retryDecision)
	if retryErr != nil {
		return retryDecision, fmt.Errorf("retry truncated evidence review: %w (initial validation error: %v)", retryErr, err)
	}
	setEvidenceReviewMetrics(&retryDecision.Observation, len(request.Candidates), len(retryRequest.Candidates), true)
	return retryDecision, nil
}

func (r *DomainEvidenceReviewer) reviewOnce(ctx context.Context, request EvidenceReviewRequest, systemPrompt string) (ReviewDecision, error) {
	response, err := r.model.GenerateReview(ctx, ReviewModelRequest{
		SystemPrompt: systemPrompt,
		Question:     request.Question,
		Task:         request.Task,
		Profile:      cloneProfile(request.Profile),
		Candidates:   slices.Clone(request.Candidates),
		Attempt:      request.Attempt,
		ModelID:      request.ModelID,
	})
	decision := ReviewDecision{ModelCallID: response.ModelCallID, ModelCalls: 1}
	if response.ModelCallID != "" {
		decision.ModelCallIDs = []string{response.ModelCallID}
	}
	if err != nil {
		return decision, err
	}
	observation, err := decodeAndValidateObservation(response.Content, request)
	if err != nil {
		return decision, fmt.Errorf("validate evidence review output: %w", err)
	}
	decision.Observation = observation
	return decision, nil
}

func selectEvidenceReviewCandidates(candidates []EvidenceCandidate, limit int) []EvidenceCandidate {
	if limit <= 0 || len(candidates) <= limit {
		return slices.Clone(candidates)
	}
	return slices.Clone(candidates[:limit])
}

func combineReviewDecisions(previous, current ReviewDecision) ReviewDecision {
	current.ModelCalls += previous.ModelCalls
	current.ModelCallIDs = append(slices.Clone(previous.ModelCallIDs), current.ModelCallIDs...)
	if current.ModelCallID == "" {
		current.ModelCallID = previous.ModelCallID
	}
	return current
}

func setEvidenceReviewMetrics(observation *AgentObservation, originalCandidateCount, reviewedCandidateCount int, retried bool) {
	if observation == nil {
		return
	}
	if observation.Metrics == nil {
		observation.Metrics = make(map[string]any)
	}
	observation.Metrics["original_candidate_count"] = originalCandidateCount
	observation.Metrics["reviewed_candidate_count"] = reviewedCandidateCount
	observation.Metrics["truncation_retried"] = retried
}

func decodeAndValidateObservation(content string, request EvidenceReviewRequest) (AgentObservation, error) {
	if len(content) == 0 || len(content) > maxRouteResponseBytes {
		return AgentObservation{}, fmt.Errorf("evidence review output size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var observation AgentObservation
	if err := decoder.Decode(&observation); err != nil {
		return AgentObservation{}, fmt.Errorf("decode evidence review: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return AgentObservation{}, fmt.Errorf("evidence review contains trailing JSON")
	}
	if observation.AgentID != request.Profile.ID {
		return AgentObservation{}, fmt.Errorf("evidence review agent_id %q does not match %q", observation.AgentID, request.Profile.ID)
	}
	if len(request.Task.TopicIDs) == 1 {
		observation.TopicID = request.Task.TopicIDs[0]
	}
	if observation.Status != EvidenceStatusSufficient && observation.Status != EvidenceStatusInsufficient {
		return AgentObservation{}, fmt.Errorf("invalid evidence status %q", observation.Status)
	}
	allowedEvidence := make(map[string]EvidenceCandidate, len(request.Candidates))
	for _, candidate := range request.Candidates {
		allowedEvidence[candidate.OpaqueID] = candidate
	}
	originalFactCount := len(observation.Facts)
	validFacts := make([]ObservedFact, 0, originalFactCount)
	rejectedFactDetails := make([]string, 0)
	for i := range observation.Facts {
		fact := observation.Facts[i]
		if err := validateAndNormalizeObservedFact(&fact, allowedEvidence, request.Candidates); err != nil {
			rejectedFactDetails = append(rejectedFactDetails, fmt.Sprintf("fact %d: %v", i, err))
			continue
		}
		validFacts = append(validFacts, fact)
	}
	observation.Facts = validFacts
	if len(rejectedFactDetails) > 0 {
		if observation.Metrics == nil {
			observation.Metrics = make(map[string]any)
		}
		observation.Metrics["rejected_fact_count"] = len(rejectedFactDetails)
		observation.Metrics["rejected_fact_details"] = rejectedFactDetails
		if len(observation.Facts) > 0 {
			observation.Status = EvidenceStatusInsufficient
			appendUniqueStrings(&observation.MissingRequirements, "one or more requested facts lack a valid quote or citation")
		}
	}
	if originalFactCount > 0 && len(observation.Facts) == 0 {
		return AgentObservation{}, fmt.Errorf("all evidence review facts are invalid: %s", strings.Join(rejectedFactDetails, "; "))
	}
	if observation.Status == EvidenceStatusSufficient && len(observation.Facts) == 0 {
		return AgentObservation{}, fmt.Errorf("sufficient evidence review must contain facts")
	}
	if err := validateReviewStrings(observation.MissingRequirements, "missing_requirements"); err != nil {
		return AgentObservation{}, err
	}
	if err := validateReviewStrings(observation.Conflicts, "conflicts"); err != nil {
		return AgentObservation{}, err
	}
	if observation.RecoveryRequest != nil {
		if request.Attempt != 0 {
			observation.RecoveryRequest = nil
			if observation.Metrics == nil {
				observation.Metrics = make(map[string]any)
			}
			observation.Metrics["second_recovery_request_suppressed"] = true
			if observation.Status == EvidenceStatusInsufficient {
				appendUniqueStrings(&observation.MissingRequirements, "bounded evidence recovery budget exhausted")
			}
			return observation, nil
		}
		if observation.Status != EvidenceStatusInsufficient {
			return AgentObservation{}, fmt.Errorf("recovery is allowed only after the first insufficient review")
		}
		if !slices.Contains(request.Profile.AllowedResearchTools, observation.RecoveryRequest.Tool) {
			return AgentObservation{}, fmt.Errorf("recovery tool %q is not allowed", observation.RecoveryRequest.Tool)
		}
		if strings.TrimSpace(observation.RecoveryRequest.Query) == "" {
			return AgentObservation{}, fmt.Errorf("recovery query is required")
		}
		if len(observation.RecoveryRequest.Queries) > 3 {
			return AgentObservation{}, fmt.Errorf("recovery queries exceed limit")
		}
		if err := validateReviewStrings(observation.RecoveryRequest.Queries, "recovery queries"); err != nil {
			return AgentObservation{}, err
		}
		if len(observation.RecoveryRequest.Terms) > 10 {
			return AgentObservation{}, fmt.Errorf("recovery exact terms exceed limit")
		}
	}
	return observation, nil
}

func validateAndNormalizeObservedFact(
	fact *ObservedFact,
	allowedEvidence map[string]EvidenceCandidate,
	candidates []EvidenceCandidate,
) error {
	if fact == nil || strings.TrimSpace(fact.Statement) == "" || len(fact.Citations) == 0 {
		return fmt.Errorf("must have a statement and citations")
	}
	for _, citation := range fact.Citations {
		if _, ok := allowedEvidence[citation.OpaqueID]; !ok {
			return fmt.Errorf("cites unknown evidence %q", citation.OpaqueID)
		}
	}
	fact.Quote = strings.TrimSpace(fact.Quote)
	if fact.Quote == "" {
		return fmt.Errorf("must preserve a policy quote")
	}
	fact.Scenario = strings.TrimSpace(fact.Scenario)
	fact.DocumentLevel = normalizeDocumentLevel(fact.DocumentLevel)
	fact.Currency = normalizeFactCurrency(fact.Currency)
	if !factQuoteExistsInCitedEvidence(*fact, candidates) {
		return fmt.Errorf("quote is not present in its cited evidence: %s", describeFactQuoteMismatch(*fact, allowedEvidence))
	}
	fact.IsAmbiguous = fact.IsAmbiguous || containsAmbiguousPolicyLanguage(fact.Quote)
	if fact.DocumentLevel == "unspecified" {
		fact.DocumentLevel = inferFactDocumentLevel(*fact, allowedEvidence)
	}
	if fact.Currency == "unspecified" {
		fact.Currency = inferFactCurrency(fact.Statement + " " + fact.Quote)
	}
	return nil
}

func containsAmbiguousPolicyLanguage(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{"有限数量", "偶尔", "适当", "必要时", "原则上", "limited quantities", "occasionally", "as appropriate", "where necessary"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func inferFactDocumentLevel(fact ObservedFact, candidates map[string]EvidenceCandidate) string {
	best := "unspecified"
	for _, citation := range fact.Citations {
		candidate, ok := candidates[citation.OpaqueID]
		if !ok {
			continue
		}
		var metadata strings.Builder
		for key, value := range candidate.Metadata {
			metadata.WriteString(" ")
			metadata.WriteString(key)
			metadata.WriteString(" ")
			metadata.WriteString(value)
		}
		value := strings.ToLower(candidate.Title + " " + candidate.KnowledgeFilename + " " + metadata.String())
		level := "unspecified"
		switch {
		case strings.Contains(value, "sop"), strings.Contains(value, "实施细则"), strings.Contains(value, "操作规程"):
			level = "internal_sop"
		case strings.Contains(value, "policy"), strings.Contains(value, "政策"), strings.Contains(value, "制度"):
			level = "formal_policy"
		case strings.Contains(value, "guideline"), strings.Contains(value, "行业规范"), strings.Contains(value, "行业准则"):
			level = "industry_guideline"
		}
		if documentLevelRank(level) < documentLevelRank(best) {
			best = level
		}
	}
	return best
}

func inferFactCurrency(value string) string {
	upper := strings.ToUpper(value)
	hasRMB := strings.Contains(upper, "RMB") || strings.Contains(upper, "CNY") || strings.Contains(value, "人民币")
	hasCHF := strings.Contains(upper, "CHF") || strings.Contains(value, "瑞士法郎")
	switch {
	case hasRMB && hasCHF:
		return "mixed"
	case hasRMB:
		return "RMB"
	case hasCHF:
		return "CHF"
	default:
		return "unspecified"
	}
}

func factQuoteExistsInCitedEvidence(fact ObservedFact, candidates []EvidenceCandidate) bool {
	want := normalizeEvidenceText(fact.Quote)
	if want == "" {
		return true
	}
	cited := make(map[string]struct{}, len(fact.Citations))
	for _, citation := range fact.Citations {
		cited[citation.OpaqueID] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := cited[candidate.OpaqueID]; ok && strings.Contains(normalizeEvidenceText(candidate.Content), want) {
			return true
		}
	}
	return false
}

func describeFactQuoteMismatch(fact ObservedFact, candidates map[string]EvidenceCandidate) string {
	const maxDiagnosticRunes = 1200

	parts := []string{fmt.Sprintf("quote=%q", truncateEvidenceDiagnostic(fact.Quote, maxDiagnosticRunes))}
	for _, citation := range fact.Citations {
		candidate, ok := candidates[citation.OpaqueID]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf(
			"citation=%q title=%q content=%q",
			citation.OpaqueID,
			truncateEvidenceDiagnostic(candidate.Title, 200),
			truncateEvidenceDiagnostic(candidate.Content, maxDiagnosticRunes),
		))
	}
	return strings.Join(parts, "; ")
}

func truncateEvidenceDiagnostic(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func normalizeEvidenceText(value string) string {
	var normalized strings.Builder
	var previous rune
	var hasPrevious bool
	var pendingWhitespace bool

	for _, current := range []rune(strings.ToLower(value)) {
		if unicode.IsSpace(current) {
			pendingWhitespace = hasPrevious
			continue
		}
		if pendingWhitespace && !(unicode.Is(unicode.Han, previous) && unicode.Is(unicode.Han, current)) {
			normalized.WriteByte(' ')
		}
		normalized.WriteRune(current)
		previous = current
		hasPrevious = true
		pendingWhitespace = false
	}
	return strings.TrimSpace(normalized.String())
}

func normalizeDocumentLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "internal_sop", "formal_policy", "industry_guideline", "other":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unspecified"
	}
}

func normalizeFactCurrency(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "RMB", "CHF":
		return strings.ToUpper(strings.TrimSpace(value))
	case "MIXED":
		return "mixed"
	default:
		return "unspecified"
	}
}

func validateReviewStrings(values []string, field string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", field)
		}
	}
	return nil
}
