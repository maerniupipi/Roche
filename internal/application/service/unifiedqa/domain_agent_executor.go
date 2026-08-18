package unifiedqa

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
)

type DomainRetriever interface {
	Retrieve(context.Context, AgentTask, AuthorizedScope, rerank.Reranker, RetrievalPolicy) (RetrievalResult, error)
}

type DomainReviewer interface {
	Review(context.Context, EvidenceReviewRequest) (ReviewDecision, error)
}

type DomainRecoverer interface {
	Recover(context.Context, RecoveryRequest, AuthorizedScope, []EvidenceCandidate, int, rerank.Reranker, RetrievalPolicy) (RetrievalResult, error)
}

type DomainExecutionResult struct {
	Observation  AgentObservation
	Candidates   []EvidenceCandidate
	ToolCalls    int
	ReviewCalls  int
	ModelCallIDs []string
	Degraded     bool
}

type DomainProgressStage string

const (
	DomainProgressRetrieving        DomainProgressStage = "retrieving"
	DomainProgressReviewing         DomainProgressStage = "reviewing"
	DomainProgressRecovering        DomainProgressStage = "recovering"
	DomainProgressReviewingRecovery DomainProgressStage = "reviewing_recovery"
)

type DomainProgressFunc func(DomainProgressStage)

type DomainAgentExecutor struct {
	catalog   *AgentCatalog
	retrieval DomainRetriever
	reviewer  DomainReviewer
	recovery  DomainRecoverer
}

func NewDomainAgentExecutor(
	catalog *AgentCatalog,
	retrieval DomainRetriever,
	reviewer DomainReviewer,
	recovery DomainRecoverer,
) *DomainAgentExecutor {
	return &DomainAgentExecutor{catalog: catalog, retrieval: retrieval, reviewer: reviewer, recovery: recovery}
}

func (e *DomainAgentExecutor) Execute(
	ctx context.Context,
	question string,
	task AgentTask,
	scope AuthorizedScope,
	modelID string,
	rerankerModel rerank.Reranker,
	retrievalPolicy RetrievalPolicy,
	onProgress DomainProgressFunc,
) (DomainExecutionResult, error) {
	if e == nil || e.catalog == nil || e.retrieval == nil || e.reviewer == nil {
		return DomainExecutionResult{}, fmt.Errorf("domain agent executor is not configured")
	}
	profile, ok := e.catalog.Get(task.AgentID)
	if !ok {
		return DomainExecutionResult{}, fmt.Errorf("agent %q is not in fixed catalog", task.AgentID)
	}
	reportDomainProgress(onProgress, DomainProgressRetrieving)
	retrieved, err := e.retrieval.Retrieve(ctx, task, scope, rerankerModel, retrievalPolicy)
	if err != nil {
		return DomainExecutionResult{ToolCalls: retrieved.ToolCalls}, err
	}
	result := DomainExecutionResult{
		Candidates: retrieved.Candidates,
		ToolCalls:  retrieved.ToolCalls,
		Degraded:   retrieved.RerankDegraded,
	}
	reportDomainProgress(onProgress, DomainProgressReviewing)
	firstReview, err := e.reviewer.Review(ctx, EvidenceReviewRequest{
		Question: question, Task: task, Profile: profile, Candidates: result.Candidates, Attempt: 0, ModelID: modelID,
	})
	recordReviewDecision(&result, firstReview)
	if err != nil {
		return result, fmt.Errorf("initial evidence review: %w", err)
	}
	firstReview.Observation = ensureCoverageRecovery(firstReview.Observation, question, task, profile)
	result.Observation = firstReview.Observation
	if firstReview.Observation.RecoveryRequest == nil {
		return result, nil
	}
	if e.recovery == nil {
		result.Degraded = true
		result.Observation.RecoveryRequest = nil
		appendUniqueStrings(&result.Observation.MissingRequirements, "bounded evidence recovery is unavailable")
		return result, nil
	}
	reportDomainProgress(onProgress, DomainProgressRecovering)
	recovered, err := e.recovery.Recover(ctx, *firstReview.Observation.RecoveryRequest, scope, result.Candidates, result.ToolCalls, rerankerModel, retrievalPolicy)
	if err != nil {
		result.Degraded = true
		result.Observation.RecoveryRequest = nil
		appendUniqueStrings(&result.Observation.MissingRequirements, "bounded evidence recovery failed")
		return result, nil
	}
	result.Candidates = recovered.Candidates
	result.ToolCalls += recovered.ToolCalls
	result.Degraded = result.Degraded || recovered.RerankDegraded
	reportDomainProgress(onProgress, DomainProgressReviewingRecovery)
	secondReview, err := e.reviewer.Review(ctx, EvidenceReviewRequest{
		Question: question, Task: task, Profile: profile, Candidates: result.Candidates, Attempt: 1, ModelID: modelID,
	})
	recordReviewDecision(&result, secondReview)
	if err != nil {
		return result, fmt.Errorf("recovery evidence review: %w", err)
	}
	result.Observation = secondReview.Observation
	return result, nil
}

// ensureCoverageRecovery closes a reliability gap in the model contract: an
// evidence reviewer can correctly report missing requirements but forget to
// request the single bounded recovery. The backend turns that explicit gap
// into one focused search. It never invents facts and never expands the
// authorized scope; the recovered candidates still go through a second full
// evidence review before they can reach the answer.
func ensureCoverageRecovery(
	observation AgentObservation,
	question string,
	task AgentTask,
	profile DomainAgentProfile,
) AgentObservation {
	if len(observation.MissingRequirements) == 0 {
		return observation
	}
	if observation.RecoveryRequest != nil {
		if observation.RecoveryRequest.Tool == "knowledge_search" {
			observation.RecoveryRequest.Queries = buildCoverageRecoveryQueries(
				question,
				observation.MissingRequirements,
				observation.RecoveryRequest.Query,
			)
		}
		return observation
	}
	tool := ""
	for _, candidate := range []string{"knowledge_search", "grep_chunks"} {
		if slices.Contains(profile.AllowedResearchTools, candidate) {
			tool = candidate
			break
		}
	}
	if tool == "" {
		return observation
	}

	baseQuestion := strings.TrimSpace(question)
	if baseQuestion == "" {
		baseQuestion = strings.TrimSpace(task.Goal)
	}
	queries := buildCoverageRecoveryQueries(baseQuestion, observation.MissingRequirements, "")
	if len(queries) == 0 {
		return observation
	}
	observation.Status = EvidenceStatusInsufficient
	observation.RecoveryRequest = &RecoveryRequest{Tool: tool, Query: queries[0]}
	if tool == "knowledge_search" {
		observation.RecoveryRequest.Queries = queries
	}
	if observation.Metrics == nil {
		observation.Metrics = make(map[string]any)
	}
	observation.Metrics["coverage_recovery_injected"] = true
	observation.Metrics["coverage_missing_requirement_count"] = len(observation.MissingRequirements)
	return observation
}

// buildCoverageRecoveryQueries turns distinct user-facing evidence gaps into
// separate semantic searches. This remains one bounded recovery phase, but it
// avoids diluting several unrelated requirements into one oversized query.
func buildCoverageRecoveryQueries(question string, missing []string, modelQuery string) []string {
	queries := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	appendQuery := func(value string) {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" {
			return
		}
		if runes := []rune(value); len(runes) > 500 {
			value = string(runes[:500])
		}
		key := normalizeEvidenceText(value)
		if _, duplicate := seen[key]; duplicate {
			return
		}
		seen[key] = struct{}{}
		queries = append(queries, value)
	}
	appendQuery(modelQuery)
	question = strings.TrimSpace(question)
	for _, requirement := range missing {
		requirement = strings.TrimSpace(requirement)
		if requirement == "" || missingRequirementIsInternal(requirement) {
			continue
		}
		focused := requirement
		if question != "" {
			focused = question + "；重点查找：" + requirement
		}
		appendQuery(focused)
		if len(queries) == 3 {
			break
		}
	}
	if len(queries) == 0 {
		appendQuery(question)
	}
	return queries
}

func missingRequirementIsInternal(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{
		"valid quote", "citation", "bounded evidence", "evidence review", "recovery budget",
		"有效原文", "引用校验", "证据复核", "补查预算", "内部工具", "模型输出",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func recordReviewDecision(result *DomainExecutionResult, decision ReviewDecision) {
	if result == nil {
		return
	}
	calls := decision.ModelCalls
	if calls <= 0 {
		calls = 1
	}
	result.ReviewCalls += calls
	if len(decision.ModelCallIDs) > 0 {
		result.ModelCallIDs = append(result.ModelCallIDs, decision.ModelCallIDs...)
		return
	}
	if decision.ModelCallID != "" {
		result.ModelCallIDs = append(result.ModelCallIDs, decision.ModelCallID)
	}
}

func reportDomainProgress(onProgress DomainProgressFunc, stage DomainProgressStage) {
	if onProgress != nil {
		onProgress(stage)
	}
}
