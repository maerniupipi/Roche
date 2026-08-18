package unifiedqa

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/tracing/langfuse"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// Executor is the single knowledge-QA execution boundary used by SessionService.
type Executor interface {
	Execute(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error
}

// unifiedQAHistoryMaxRounds is the maximum number of complete user/assistant
// exchanges supplied to the unified route model, as required by Func013.
const unifiedQAHistoryMaxRounds = 7

type UnifiedQAService struct {
	runRepository            interfaces.UnifiedQARunRepository
	messages                 interfaces.MessageRepository
	scopeResolver            *AuthorizedKBResolver
	catalog                  *AgentCatalog
	nodeRunner               *NodeRunner
	router                   *MasterAgentRouter
	rerankModels             *RerankModelResolver
	retrieval                *RetrievalAdapter
	faqFastPath              FAQFastPathReviewer
	domainAgents             *DomainAgentExecutor
	aggregator               *ObservationAggregator
	answers                  *AnswerGenerator
	terminology              *TerminologyCatalog
	configuredSummaryModelID string
	now                      func() time.Time
	newID                    func() string
	streamChunkInterval      time.Duration
}

func NewUnifiedQAService(
	runRepository interfaces.UnifiedQARunRepository,
	messages interfaces.MessageRepository,
	scopeResolver *AuthorizedKBResolver,
	catalog *AgentCatalog,
	nodeRunner *NodeRunner,
	router *MasterAgentRouter,
	rerankModels *RerankModelResolver,
	retrieval *RetrievalAdapter,
	faqFastPath FAQFastPathReviewer,
	domainAgents *DomainAgentExecutor,
	aggregator *ObservationAggregator,
	answers *AnswerGenerator,
	terminology *TerminologyCatalog,
	cfg *config.Config,
) *UnifiedQAService {
	configuredSummaryModelID := ""
	if cfg != nil && cfg.UnifiedQA != nil {
		configuredSummaryModelID = strings.TrimSpace(cfg.UnifiedQA.SummaryModelID)
	}
	return &UnifiedQAService{
		runRepository:            runRepository,
		messages:                 messages,
		scopeResolver:            scopeResolver,
		catalog:                  catalog,
		nodeRunner:               nodeRunner,
		router:                   router,
		rerankModels:             rerankModels,
		retrieval:                retrieval,
		faqFastPath:              faqFastPath,
		domainAgents:             domainAgents,
		aggregator:               aggregator,
		answers:                  answers,
		terminology:              terminology,
		configuredSummaryModelID: configuredSummaryModelID,
		now:                      time.Now,
		newID:                    uuid.NewString,
		streamChunkInterval:      unifiedQATextChunkInterval,
	}
}

func (s *UnifiedQAService) Execute(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error {
	if err := s.validateRequest(req, eventBus); err != nil {
		return err
	}
	startedAt := s.now()
	summaryModelID := s.resolveSummaryModelID(ctx, req)
	runID := s.newID()
	requestID, _ := types.RequestIDFromContext(ctx)
	userID := types.SessionOwnerIDFromContext(ctx)
	if userID == "" {
		userID, _ = types.UserIDFromContext(ctx)
	}
	if userID == "" {
		userID = req.Session.UserID
	}
	langfuseTraceID := ""
	if trace, ok := langfuse.TraceFromContext(ctx); ok {
		langfuseTraceID = trace.ID
		trace.UpdateMetadata(map[string]interface{}{
			"unified_qa.run_id":               runID,
			"unified_qa.user_message_id":      req.UserMessageID,
			"unified_qa.assistant_message_id": req.AssistantMessageID,
		})
	}
	retrievalPolicy := retrievalPolicyForRequest(req)
	configSnapshot := s.catalog.ConfigSnapshot()
	unknownTerms := s.terminology.UnknownTerms(req.Query)
	if version := s.terminology.Version(); version != "" {
		configSnapshot["terminology_version"] = version
	}
	configSnapshot["retrieval_policy"] = types.JSONMap{
		"faq_priority_enabled":        retrievalPolicy.FAQPriorityEnabled,
		"faq_direct_answer_threshold": retrievalPolicy.FAQDirectAnswerThreshold,
		"faq_score_boost":             retrievalPolicy.FAQScoreBoost,
	}
	run := &types.QAExecutionRun{
		ID: runID, SessionID: req.Session.ID, RequestID: requestID,
		UserMessageID: req.UserMessageID, AssistantMessageID: req.AssistantMessageID,
		UserID: userID, EntryAgentID: s.catalog.MasterAgentID(), Status: types.QARunStatusRunning,
		OriginalQuery: req.Query, ConfigSnapshot: configSnapshot, Metrics: types.JSONMap{}, StartedAt: startedAt,
		LangfuseTraceID: langfuseTraceID,
	}
	if s.runRepository != nil {
		if err := s.runRepository.CreateRun(ctx, run); err != nil {
			logger.Warnf(ctx, "unified QA run start persistence failed: %v", err)
		}
	}
	progress := newProgressReporter(eventBus, runID, req.Session.ID, requestID, s.streamChunkInterval)
	defer progress.Close(context.WithoutCancel(ctx))

	scope, err := s.scopeResolver.Resolve(ctx)
	if err != nil {
		if errors.Is(err, ErrNoAccessibleKnowledgeBase) {
			message := "No accessible knowledge base is available for this request."
			_ = emitFinalAnswerChunks(
				ctx, eventBus, uuid.NewString()+"-answer", req.Session.ID, requestID,
				message, true, true, s.streamChunkInterval,
			)
			s.finishRun(ctx, runID, startedAt, types.QARunStatusInsufficient, "", nil, types.JSONMap{
				"authorized_kb_count":   0,
				"response_policy_codes": []string{globalResponsePolicyCode("no_accessible_kb")},
			}, "NO_ACCESSIBLE_KB")
			return nil
		}
		s.finishRun(ctx, runID, startedAt, types.QARunStatusFailed, "", nil, nil, "SCOPE_RESOLUTION_FAILED")
		return err
	}
	run.ConfigSnapshot["authorized_kb_ids"] = scope.KnowledgeBaseIDs

	history := s.loadHistoryBestEffort(ctx, req.Session.ID, unifiedQAHistoryMaxRounds)
	runContext, err := NewRunContext(RunContextInput{
		RunID: runID, SessionID: req.Session.ID, RequestID: requestID, UserID: userID,
		OriginalQuery: req.Query, History: history, AuthorizedScope: scope, ConfigSnapshot: run.ConfigSnapshot,
	})
	if err != nil {
		s.finishRun(ctx, runID, startedAt, types.QARunStatusFailed, "", nil, nil, "RUN_CONTEXT_FAILED")
		return err
	}

	var route RouteDecision
	_, routeErr := s.nodeRunner.Run(ctx, NodeSpec{
		RunID: runID, NodeName: "master_route", NodeType: "model", MasterAgentID: s.catalog.MasterAgentID(),
		InputSummary: types.JSONMap{"query": req.Query, "history_turns": len(history)}, ConfigVersion: s.catalog.Version(),
	}, func(nodeCtx context.Context) (NodeResult, error) {
		route = s.router.Route(nodeCtx, RouteRequest{OriginalQuery: req.Query, History: runContext.History, ModelID: summaryModelID})
		status := types.QANodeStatusCompleted
		if route.Degraded {
			status = types.QANodeStatusDegraded
		}
		return NodeResult{Status: status, ModelCallID: route.ModelCallID, ErrorCode: route.ErrorCode,
			OutputSummary: types.JSONMap{"standalone_query": route.Plan.StandaloneQuery, "agent_ids": routeAgentIDsFromTasks(route.Plan.Tasks)}}, nil
	})
	if routeErr != nil {
		s.finishRun(ctx, runID, startedAt, types.QARunStatusFailed, "", nil, nil, "ROUTE_FAILED")
		return routeErr
	}
	emitUnifiedQAMilestone(ctx, eventBus, req.Session.ID, requestID, event.EventQuestionUnderstood, event.AgentThoughtData{
		Content: "已完成问题理解", Done: true, RunID: runID,
		StepID: runID + "-question-understood", Stage: "question_understood",
		Status: "completed", ResultCount: len(route.Plan.Tasks), ModelCalls: 1,
	})
	if route.Plan.Outcome != RouteOutcomeRouted || len(route.Plan.Tasks) == 0 {
		kind := route.Plan.Outcome
		if kind == RouteOutcomeOutOfCoverage && len(unknownTerms) > 0 {
			kind = "term_unrecognized"
		}
		message := renderCatalogFallback(s.catalog, kind, detectAnswerLanguage(ctx, req.Query), unknownTerms)
		responsePolicyCodes := []string{globalResponsePolicyCode(kind)}
		if strings.TrimSpace(message) == "" {
			message = renderNoKnowledgeFallback(detectAnswerLanguage(ctx, req.Query))
			responsePolicyCodes = []string{globalResponsePolicyCode("no_knowledge")}
		}
		_ = emitFinalAnswerChunks(
			ctx, eventBus, uuid.NewString()+"-answer", req.Session.ID, requestID,
			message, true, true, s.streamChunkInterval,
		)
		metrics := types.JSONMap{"authorized_kb_count": len(scope.KnowledgeBaseIDs), "model_calls": 1, "tool_calls": 0,
			"coverage": CoverageInsufficient, "route_outcome": route.Plan.Outcome, "unknown_terms": unknownTerms,
			"response_policy_codes": responsePolicyCodes}
		s.finishRun(ctx, runID, startedAt, types.QARunStatusInsufficient, route.Plan.StandaloneQuery, route.Plan.Tasks, metrics, route.ErrorCode)
		return nil
	}
	progress.Begin(ctx, progressStep{
		Lane: "workflow", Stage: "master_route",
		Content: routeSelectionProgressMessage(s.catalog, route.Plan.Tasks),
	})

	var rerankerModel rerank.Reranker
	rerankModelID := ""
	if s.rerankModels != nil {
		resolved, id, resolveErr := s.rerankModels.Resolve(ctx, "")
		if resolveErr != nil {
			logger.Warnf(ctx, "unified QA reranker unavailable, continuing with retrieval scores: %v", resolveErr)
		} else {
			rerankerModel, rerankModelID = resolved, id
		}
	}
	fastPath := faqFastPathAttempt{}
	fastPathScope := AuthorizedScope{}
	fastPathTopicID := ""
	if len(route.Plan.Tasks) == 1 && len(route.Plan.Tasks[0].TopicIDs) <= 1 {
		fastPathScope, _ = scopeForAgent(scope, s.catalog, route.Plan.Tasks[0].AgentID)
		if len(route.Plan.Tasks[0].TopicIDs) == 1 {
			fastPathTopicID = route.Plan.Tasks[0].TopicIDs[0]
			fastPathScope, _ = scopeForTopic(fastPathScope, s.catalog, route.Plan.Tasks[0].AgentID, fastPathTopicID)
		}
	}
	if len(fastPathScope.KnowledgeBaseIDs) > 0 && retrievalPolicy.FAQPriorityEnabled && rerankerModel != nil && s.retrieval != nil && s.faqFastPath != nil {
		fastPath = s.tryFAQFastPath(ctx, runID, requestID, req, route.Plan.StandaloneQuery, fastPathScope, rerankerModel, retrievalPolicy, summaryModelID, nil)
		if fastPath.Eligible {
			emitUnifiedQAMilestone(ctx, eventBus, req.Session.ID, requestID, event.EventKnowledgeSearch, event.AgentThoughtData{
				Content: "检索知识库", Done: true, RunID: runID,
				StepID: runID + "-knowledge-search", Stage: "knowledge_search",
				Status: "completed", ResultCount: 1, ToolCalls: fastPath.ToolCalls, ModelCalls: fastPath.ModelCalls,
			})
			progress.Begin(ctx, progressStep{Lane: "workflow", Stage: "faq_answer", Content: "FAQ 高置信匹配已通过轻量校验，正在返回标准答案……\n"})
			faqTail := []string(nil)
			faqResponsePolicyCodes := []string(nil)
			if fastPathTopicID != "" {
				policy := buildTopicAnswerPolicy(s.catalog, AggregatedObservation{MatchedTopics: []string{fastPathTopicID}}, req.Query, detectAnswerLanguage(ctx, req.Query), unknownTerms)
				faqTail = policy.TailSections()
				faqResponsePolicyCodes = policy.TailResponsePolicyCodes()
			}
			if err := emitFAQFastPathAnswer(
				ctx, eventBus, req.Session.ID, requestID, fastPath.Candidate, fastPath.Answer,
				faqTail, progress.Close, s.streamChunkInterval,
			); err != nil {
				metrics := faqFastPathMetrics(fastPathScope, rerankModelID, fastPath, 1)
				s.finishRun(ctx, runID, startedAt, types.QARunStatusFailed, route.Plan.StandaloneQuery, route.Plan.Tasks, metrics, "FAQ_FAST_PATH_EMIT_FAILED")
				return err
			}
			progress.Close(ctx)
			metrics := faqFastPathMetrics(fastPathScope, rerankModelID, fastPath, 1)
			if fastPathTopicID != "" {
				metrics["matched_topics"] = []string{fastPathTopicID}
				metrics["fallback_topics"] = []string{}
			}
			metrics["response_policy_codes"] = faqResponsePolicyCodes
			s.finishRun(ctx, runID, startedAt, types.QARunStatusCompleted, route.Plan.StandaloneQuery, route.Plan.Tasks, metrics, "")
			return nil
		}
	}

	executionTasks := make([]AgentTask, 0, len(route.Plan.Tasks))
	executionScopes := make([]AuthorizedScope, 0, len(route.Plan.Tasks))
	domainErrors := make([]error, 0, len(route.Plan.Tasks))
	for _, task := range route.Plan.Tasks {
		agentScope, scopeErr := scopeForAgent(runContext.AuthorizedScope, s.catalog, task.AgentID)
		if len(task.TopicIDs) == 0 {
			executionTasks = append(executionTasks, task)
			executionScopes = append(executionScopes, agentScope)
			domainErrors = append(domainErrors, scopeErr)
			continue
		}
		for _, topicID := range task.TopicIDs {
			topicTask := task
			topicTask.TopicIDs = []string{topicID}
			topicScope := AuthorizedScope{}
			topicErr := scopeErr
			if topicErr == nil {
				topicScope, topicErr = scopeForTopic(agentScope, s.catalog, task.AgentID, topicID)
			}
			executionTasks = append(executionTasks, topicTask)
			executionScopes = append(executionScopes, topicScope)
			domainErrors = append(domainErrors, topicErr)
		}
	}
	domainResults := make([]DomainExecutionResult, len(executionTasks))
	var wg sync.WaitGroup
	for i, task := range executionTasks {
		wg.Add(1)
		go func(index int, agentTask AgentTask, agentScope AuthorizedScope) {
			defer wg.Done()
			if domainErrors[index] != nil {
				progress.Begin(ctx, progressStep{
					Lane: "domain", Stage: "domain_scope_unavailable", AgentID: agentTask.AgentID,
					Content: domainCompletionProgressMessage(s.catalog, agentTask.AgentID, DomainExecutionResult{}, domainErrors[index]),
				})
				return
			}
			agentExecutionID := s.newID()
			_, nodeErr := s.nodeRunner.Run(ctx, NodeSpec{
				RunID: runID, NodeName: "domain_agent", NodeType: "agent", MasterAgentID: s.catalog.MasterAgentID(),
				WorkerAgentID: agentTask.AgentID, AgentExecutionID: agentExecutionID,
				InputSummary: types.JSONMap{"goal": agentTask.Goal, "query_count": len(agentTask.SearchQueries), "topic_id": singleTopicID(agentTask)}, ConfigVersion: s.catalog.Version(),
			}, func(nodeCtx context.Context) (NodeResult, error) {
				domainResults[index], domainErrors[index] = s.domainAgents.Execute(
					nodeCtx,
					route.Plan.StandaloneQuery,
					agentTask,
					agentScope,
					summaryModelID,
					rerankerModel,
					retrievalPolicy,
					func(stage DomainProgressStage) {
						progress.Begin(nodeCtx, progressStep{
							Lane: "domain", Stage: "domain_" + string(stage), AgentID: agentTask.AgentID,
							Content: domainProgressMessage(s.catalog, agentTask.AgentID, stage),
						})
					},
				)
				result := domainResults[index]
				return NodeResult{OutputSummary: types.JSONMap{"status": result.Observation.Status, "candidate_count": len(result.Candidates), "tool_calls": result.ToolCalls, "review_calls": result.ReviewCalls}}, domainErrors[index]
			})
			if nodeErr != nil {
				domainErrors[index] = nodeErr
			}
			progress.Begin(ctx, progressStep{
				Lane: "domain", Stage: "domain_completed", AgentID: agentTask.AgentID,
				Content: domainCompletionProgressMessage(s.catalog, agentTask.AgentID, domainResults[index], domainErrors[index]),
			})
		}(i, task, executionScopes[i])
	}
	wg.Wait()
	if ctx.Err() != nil {
		s.finishRun(ctx, runID, startedAt, types.QARunStatusCancelled, route.Plan.StandaloneQuery, route.Plan.Tasks, nil, ErrorCodeContextCancelled)
		return ctx.Err()
	}

	observations := make([]AgentObservation, 0, len(domainResults))
	allCandidates := make([]EvidenceCandidate, 0)
	domainFailureDetails := make([]types.JSONMap, 0)
	modelCalls := 1 + fastPath.ModelCalls
	toolCalls := fastPath.ToolCalls
	for i, result := range domainResults {
		modelCalls += result.ReviewCalls
		toolCalls += result.ToolCalls
		if domainErrors[i] != nil {
			topicID := singleTopicID(executionTasks[i])
			detail := buildDomainFailureDetail(executionTasks[i].AgentID, topicID, result, domainErrors[i])
			domainFailureDetails = append(domainFailureDetails, detail)
			logger.Warnf(ctx,
				"unified QA domain execution failed run_id=%s agent_id=%s topic_id=%s stage=%v code=%v candidate_count=%d review_calls=%d model_call_ids=%v: %v",
				runID, executionTasks[i].AgentID, topicID, detail["stage"], detail["code"], len(result.Candidates), result.ReviewCalls,
				result.ModelCallIDs, domainErrors[i],
			)
			observations = append(observations, AgentObservation{AgentID: executionTasks[i].AgentID, TopicID: singleTopicID(executionTasks[i]), Status: "failed", MissingRequirements: []string{"domain evidence review failed"}})
			continue
		}
		observations = append(observations, result.Observation)
		allCandidates = mergeCandidateLists(allCandidates, result.Candidates)
	}
	emitUnifiedQAMilestone(ctx, eventBus, req.Session.ID, requestID, event.EventKnowledgeSearch, event.AgentThoughtData{
		Content: "检索知识库", Done: true, RunID: runID,
		StepID: runID + "-knowledge-search", Stage: "knowledge_search",
		Status: "completed", ResultCount: len(allCandidates), ToolCalls: toolCalls, ModelCalls: modelCalls - 1,
	})
	progress.Begin(ctx, progressStep{Lane: "workflow", Stage: "evidence_aggregation", Content: "正在汇总已验证的领域证据……\n"})
	aggregated := s.aggregator.Aggregate(observations)
	progress.Begin(ctx, progressStep{
		Lane: "workflow", Stage: "evidence_aggregated",
		Content: aggregationProgressMessage(len(allCandidates), len(aggregated.Facts)),
	})
	answerPolicy := buildTopicAnswerPolicy(s.catalog, aggregated, req.Query, detectAnswerLanguage(ctx, req.Query), unknownTerms)
	var answer FinalAnswerResult
	progress.Begin(ctx, progressStep{
		Lane: "workflow", Stage: "answer_generation",
		Content: answerGenerationProgressMessage(len(aggregated.Facts)),
	})
	progress.Complete(ctx, "workflow", progressCompletion{
		ResultCount: len(aggregated.Facts), ToolCalls: toolCalls, ModelCalls: modelCalls,
	})
	_, err = s.nodeRunner.Run(ctx, NodeSpec{
		RunID: runID, NodeName: "generate_answer", NodeType: "model", MasterAgentID: s.catalog.MasterAgentID(),
		InputSummary:  types.JSONMap{"coverage": aggregated.Coverage, "fact_count": len(aggregated.Facts), "candidate_count": len(allCandidates)},
		ConfigVersion: s.catalog.Version(),
	}, func(nodeCtx context.Context) (NodeResult, error) {
		var answerErr error
		answer, answerErr = s.answers.Generate(nodeCtx, FinalAnswerRequest{
			Question: req.Query, StandaloneQuery: route.Plan.StandaloneQuery, Aggregated: aggregated,
			Candidates: allCandidates, ModelID: summaryModelID, SessionID: req.Session.ID, RequestID: requestID, Policy: answerPolicy,
			BeforeAnswer: progress.Close, streamInterval: s.streamChunkInterval,
		}, eventBus)
		return NodeResult{ModelCallID: answer.ModelCallID, OutputSummary: types.JSONMap{
			"reference_count": len(answer.References), "answer_length": len(answer.Answer),
			"citation_validation_failed": answer.CitationValidationFailed,
		}}, answerErr
	})
	progress.Close(ctx)
	if answer.ModelCallID != "" {
		modelCalls++
	}
	metrics := types.JSONMap{
		"authorized_kb_count": len(scope.KnowledgeBaseIDs), "model_calls": modelCalls, "tool_calls": toolCalls,
		"rerank_model_id": rerankModelID, "coverage": aggregated.Coverage, "reference_count": len(answer.References),
		"citation_validation_failed": answer.CitationValidationFailed,
		"citation_validation_error":  answer.CitationValidationError,
		"domain_failure_details":     domainFailureDetails,
		"matched_topics":             aggregated.MatchedTopics, "fallback_topics": aggregated.FallbackTopics,
		"failed_topics": aggregated.FailedTopics, "unknown_terms": unknownTerms,
		"response_policy_codes": answer.ResponsePolicyCodes,
	}
	if err != nil {
		s.finishRun(ctx, runID, startedAt, types.QARunStatusFailed, route.Plan.StandaloneQuery, route.Plan.Tasks, metrics, "ANSWER_FAILED")
		return err
	}
	s.finishRun(ctx, runID, startedAt, coverageRunStatus(aggregated.Coverage), route.Plan.StandaloneQuery, route.Plan.Tasks, metrics, "")
	return nil
}

func buildDomainFailureDetail(agentID, topicID string, result DomainExecutionResult, err error) types.JSONMap {
	stage := "scope_resolution"
	code := "DOMAIN_SCOPE_RESOLUTION_FAILED"
	if result.ReviewCalls > 0 {
		stage = "evidence_review"
		code = "DOMAIN_EVIDENCE_REVIEW_FAILED"
		if err != nil && strings.Contains(err.Error(), "validate evidence review output") {
			code = "DOMAIN_EVIDENCE_REVIEW_OUTPUT_INVALID"
		}
	} else if result.ToolCalls > 0 || len(result.Candidates) > 0 {
		stage = "retrieval"
		code = "DOMAIN_RETRIEVAL_FAILED"
	}
	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	return types.JSONMap{
		"agent_id":        agentID,
		"topic_id":        topicID,
		"stage":           stage,
		"code":            code,
		"error":           errorMessage,
		"candidate_count": len(result.Candidates),
		"tool_calls":      result.ToolCalls,
		"review_calls":    result.ReviewCalls,
		"model_call_ids":  append([]string(nil), result.ModelCallIDs...),
	}
}

func (s *UnifiedQAService) resolveSummaryModelID(ctx context.Context, req *types.QARequest) string {
	if req != nil {
		if modelID := strings.TrimSpace(req.SummaryModelID); modelID != "" {
			return modelID
		}
	}
	configuredModelID := strings.TrimSpace(s.configuredSummaryModelID)
	if configuredModelID == "" {
		return ""
	}
	// A configured model is a server-side preference, not a client
	// requirement. If a deployment carries a stale model ID, leave the
	// preferred value empty so each unified-QA model adapter selects the
	// database default instead of turning an otherwise valid query into a
	// no-evidence fallback.
	if s.answers == nil || s.answers.models == nil {
		return configuredModelID
	}
	model, err := s.answers.models.GetModelByID(ctx, configuredModelID)
	if err != nil || model == nil || model.Type != types.ModelTypeKnowledgeQA || model.DeletedAt.Valid ||
		(model.Status != "" && model.Status != types.ModelStatusActive) {
		logger.Warnf(ctx, "configured unified QA summary model %q is unavailable; falling back to the default active KnowledgeQA model", configuredModelID)
		return ""
	}
	return configuredModelID
}

func emitUnifiedQAMilestone(
	ctx context.Context,
	eventBus *event.EventBus,
	sessionID string,
	requestID string,
	eventType event.EventType,
	data event.AgentThoughtData,
) {
	if eventBus == nil {
		return
	}
	if err := eventBus.Emit(ctx, event.Event{
		ID: data.StepID, Type: eventType, SessionID: sessionID, RequestID: requestID, Data: data,
	}); err != nil {
		logger.Warnf(ctx, "unified QA milestone %s emit failed: %v", eventType, err)
	}
}

type faqFastPathAttempt struct {
	Eligible   bool
	Candidate  EvidenceCandidate
	Answer     string
	ToolCalls  int
	ModelCalls int
}

func (s *UnifiedQAService) tryFAQFastPath(
	ctx context.Context,
	runID string,
	requestID string,
	req *types.QARequest,
	standaloneQuery string,
	scope AuthorizedScope,
	rerankerModel rerank.Reranker,
	policy RetrievalPolicy,
	summaryModelID string,
	onValidate func(),
) faqFastPathAttempt {
	attempt := faqFastPathAttempt{}
	var retrieved RetrievalResult
	_, err := s.nodeRunner.Run(ctx, NodeSpec{
		RunID: runID, NodeName: "faq_fast_path_search", NodeType: "tool", MasterAgentID: s.catalog.MasterAgentID(),
		InputSummary: types.JSONMap{"query": standaloneQuery}, ConfigVersion: s.catalog.Version(),
	}, func(nodeCtx context.Context) (NodeResult, error) {
		var retrieveErr error
		retrieved, retrieveErr = s.retrieval.Retrieve(nodeCtx, AgentTask{SearchQueries: []string{standaloneQuery}}, scope, rerankerModel, policy)
		return NodeResult{OutputSummary: types.JSONMap{
			"candidate_count": len(retrieved.Candidates), "tool_calls": retrieved.ToolCalls, "rerank_degraded": retrieved.RerankDegraded,
		}}, retrieveErr
	})
	attempt.ToolCalls = retrieved.ToolCalls
	if err != nil {
		logger.Warnf(ctx, "unified QA FAQ fast-path search failed, continuing with domain review: %v", err)
		return attempt
	}
	candidate, ok := selectFAQFastPathCandidate(retrieved.Candidates)
	if !ok {
		return attempt
	}
	if onValidate != nil {
		onValidate()
	}
	alternatives := make([]EvidenceCandidate, 0, min(5, len(retrieved.Candidates)))
	for _, current := range retrieved.Candidates {
		if current.OpaqueID == candidate.OpaqueID {
			continue
		}
		alternatives = append(alternatives, cloneEvidenceCandidate(current))
		if len(alternatives) == 5 {
			break
		}
	}
	var review FAQFastPathReviewResult
	_, err = s.nodeRunner.Run(ctx, NodeSpec{
		RunID: runID, NodeName: "faq_fast_path_validate", NodeType: "model", MasterAgentID: s.catalog.MasterAgentID(),
		InputSummary: types.JSONMap{"faq_evidence_id": candidate.OpaqueID, "raw_rerank_score": candidate.RerankScore}, ConfigVersion: s.catalog.Version(),
	}, func(nodeCtx context.Context) (NodeResult, error) {
		var reviewErr error
		review, reviewErr = s.faqFastPath.Review(nodeCtx, FAQFastPathReviewRequest{
			Question: req.Query, StandaloneQuery: standaloneQuery, Candidate: candidate,
			Alternatives: alternatives, ModelID: summaryModelID,
		})
		return NodeResult{ModelCallID: review.ModelCallID, OutputSummary: types.JSONMap{
			"eligible": review.Eligible, "risks": review.Risks,
		}}, reviewErr
	})
	if review.ModelCallID != "" {
		attempt.ModelCalls = 1
	}
	if err != nil {
		logger.Warnf(ctx, "unified QA FAQ fast-path validation failed, continuing with domain review: %v", err)
		return attempt
	}
	if !review.Eligible {
		return attempt
	}
	answer := renderFAQFastPathAnswer(candidate.FAQ, requestID)
	if answer == "" {
		return attempt
	}
	attempt.Eligible = true
	attempt.Candidate = candidate
	attempt.Answer = answer
	return attempt
}

func emitFAQFastPathAnswer(
	ctx context.Context,
	eventBus *event.EventBus,
	sessionID string,
	requestID string,
	candidate EvidenceCandidate,
	answer string,
	tail []string,
	beforeAnswer func(context.Context),
	streamInterval time.Duration,
) error {
	references := types.References{&types.SearchResult{
		ID: candidate.ChunkID, KnowledgeBaseID: candidate.KnowledgeBaseID, KnowledgeID: candidate.KnowledgeID,
		ChunkIndex: candidate.ChunkIndex, StartAt: candidate.StartAt, EndAt: candidate.EndAt,
		KnowledgeTitle: candidate.Title, KnowledgeFilename: candidate.KnowledgeFilename,
		KnowledgeSource: candidate.KnowledgeSource, KnowledgeChannel: candidate.KnowledgeChannel,
		KnowledgeDescription: candidate.Description, Content: candidate.Content, ImageInfo: candidate.ImageInfo,
		Score: candidate.Score, ChunkType: candidate.ChunkType,
	}}
	if beforeAnswer != nil {
		beforeAnswer(ctx)
	}
	if err := eventBus.Emit(ctx, event.Event{
		Type: event.EventAgentReferences, SessionID: sessionID, RequestID: requestID,
		Data: event.AgentReferencesData{References: references},
	}); err != nil {
		return err
	}
	if tag := citationTag(candidate, "1"); tag != "" && !strings.Contains(answer, "<kb ") {
		answer = strings.TrimSpace(answer) + "\n\n" + tag
	}
	if len(tail) > 0 {
		answer = strings.TrimSpace(answer) + "\n\n" + strings.Join(tail, "\n\n")
	}
	return emitFinalAnswerChunks(
		ctx, eventBus, uuid.NewString()+"-faq-answer", sessionID, requestID,
		answer, true, false, streamInterval,
	)
}

func faqFastPathMetrics(scope AuthorizedScope, rerankModelID string, attempt faqFastPathAttempt, routeModelCalls int) types.JSONMap {
	return types.JSONMap{
		"authorized_kb_count": len(scope.KnowledgeBaseIDs), "model_calls": routeModelCalls + attempt.ModelCalls,
		"tool_calls": attempt.ToolCalls, "rerank_model_id": rerankModelID, "coverage": CoverageComplete,
		"reference_count": 1, "faq_fast_path": true, "faq_raw_rerank_score": attempt.Candidate.RerankScore,
	}
}

func retrievalPolicyForRequest(req *types.QARequest) RetrievalPolicy {
	policy := DefaultRetrievalPolicy()
	if req == nil || req.CustomAgent == nil {
		return policy
	}
	agentConfig := req.CustomAgent.Config
	policy.FAQPriorityEnabled = agentConfig.FAQPriorityEnabled
	if agentConfig.FAQDirectAnswerThreshold > 0 {
		policy.FAQDirectAnswerThreshold = agentConfig.FAQDirectAnswerThreshold
	}
	if agentConfig.FAQScoreBoost > 0 {
		policy.FAQScoreBoost = agentConfig.FAQScoreBoost
	}
	return policy.normalized()
}

func (s *UnifiedQAService) validateRequest(req *types.QARequest, eventBus *event.EventBus) error {
	if s == nil || s.scopeResolver == nil || s.catalog == nil || s.nodeRunner == nil || s.router == nil || s.domainAgents == nil || s.aggregator == nil || s.answers == nil {
		return fmt.Errorf("unified QA service is not configured")
	}
	if req == nil || req.Session == nil || req.Session.ID == "" || req.Query == "" {
		return fmt.Errorf("session and query are required")
	}
	if eventBus == nil {
		return fmt.Errorf("event bus is required")
	}
	return nil
}

func (s *UnifiedQAService) loadHistoryBestEffort(ctx context.Context, sessionID string, maxRounds int) []ConversationTurn {
	if s.messages == nil || maxRounds <= 0 {
		return nil
	}
	messages, err := s.messages.GetRecentMessagesBySession(ctx, sessionID, maxRounds*4)
	if err != nil {
		logger.Warnf(ctx, "unified QA history unavailable, continuing without history: %v", err)
		return nil
	}
	type messagePair struct {
		user      *types.Message
		assistant *types.Message
	}
	pairs := make(map[string]*messagePair)
	for _, message := range messages {
		if message == nil || message.RequestID == "" {
			continue
		}
		pair := pairs[message.RequestID]
		if pair == nil {
			pair = &messagePair{}
			pairs[message.RequestID] = pair
		}
		if message.Role == "user" {
			pair.user = message
		} else if message.Role == "assistant" && message.IsCompleted {
			pair.assistant = message
		}
	}
	complete := make([]*messagePair, 0, len(pairs))
	for _, pair := range pairs {
		if pair.user != nil && pair.assistant != nil {
			complete = append(complete, pair)
		}
	}
	sort.Slice(complete, func(i, j int) bool { return complete[i].user.CreatedAt.Before(complete[j].user.CreatedAt) })
	if len(complete) > maxRounds {
		complete = complete[len(complete)-maxRounds:]
	}
	turns := make([]ConversationTurn, 0, len(complete)*2)
	for _, pair := range complete {
		turns = append(turns,
			ConversationTurn{Role: "user", Content: pair.user.Content},
			ConversationTurn{Role: "assistant", Content: pair.assistant.Content},
		)
	}
	return turns
}

func (s *UnifiedQAService) finishRun(ctx context.Context, runID string, started time.Time, status types.QARunStatus, rewritten string, tasks []AgentTask, metrics types.JSONMap, errorCode string) {
	completed := s.now()
	agentIDs := routeAgentIDsFromTasks(tasks)
	routeType := types.QARouteTypeSingleAgent
	if len(agentIDs) > 1 {
		routeType = types.QARouteTypeMultiAgent
	}
	if trace, ok := langfuse.TraceFromContext(ctx); ok {
		trace.UpdateMetadata(map[string]interface{}{
			"unified_qa.run_id":             runID,
			"unified_qa.status":             status,
			"unified_qa.route_type":         routeType,
			"unified_qa.selected_agent_ids": []string(agentIDs),
			"unified_qa.error_code":         errorCode,
			"unified_qa.metrics":            metrics,
		})
	}
	if s.runRepository == nil {
		return
	}
	if err := s.runRepository.FinishRun(context.WithoutCancel(ctx), runID, types.QARunFinishUpdate{
		Status: status, RewrittenQuery: rewritten, RouteType: routeType, SelectedAgentIDs: agentIDs,
		Metrics: metrics, ErrorCode: errorCode, CompletedAt: completed, DurationMS: completed.Sub(started).Milliseconds(),
	}); err != nil {
		logger.Warnf(ctx, "unified QA run finish persistence failed: %v", err)
	}
}

func routeAgentIDsFromTasks(tasks []AgentTask) types.JSONStringArray {
	ids := make(types.JSONStringArray, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.AgentID)
	}
	return ids
}

func mergeCandidateLists(target, incoming []EvidenceCandidate) []EvidenceCandidate {
	seen := make(map[string]struct{}, len(target)+len(incoming))
	for _, candidate := range target {
		seen[candidate.OpaqueID] = struct{}{}
	}
	for _, candidate := range incoming {
		if _, ok := seen[candidate.OpaqueID]; ok {
			continue
		}
		seen[candidate.OpaqueID] = struct{}{}
		target = append(target, candidate)
	}
	return target
}

func singleTopicID(task AgentTask) string {
	if len(task.TopicIDs) == 1 {
		return task.TopicIDs[0]
	}
	return ""
}

func coverageRunStatus(coverage string) types.QARunStatus {
	switch coverage {
	case CoverageComplete:
		return types.QARunStatusCompleted
	case CoveragePartial:
		return types.QARunStatusPartial
	default:
		return types.QARunStatusInsufficient
	}
}
