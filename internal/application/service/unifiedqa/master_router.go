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
	"time"
)

const maxRouteResponseBytes = 128 * 1024

const (
	RouteOutcomeRouted        = "routed"
	RouteOutcomeOutOfService  = "out_of_service"
	RouteOutcomeOutOfCoverage = "out_of_coverage"
	RouteOutcomeFailed        = "route_failed"
)

type RouteRequest struct {
	OriginalQuery string
	History       []ConversationTurn
	ModelID       string
}

type RouteModelRequest struct {
	SystemPrompt      string
	GRPOSystemPrompt  string
	OriginalQuery     string
	History           []ConversationTurn
	Agents            []DomainAgentProfile
	MaxSelectedAgents int
	ModelID           string
}

type RouteModelResponse struct {
	Content     string
	ModelCallID string
}

type RouteModel interface {
	GenerateRoute(ctx context.Context, request RouteModelRequest) (RouteModelResponse, error)
}

type RouteDecision struct {
	Plan        MasterRoutePlan
	ModelCallID string
	Degraded    bool
	ErrorCode   string
}

type MasterAgentRouter struct {
	model            RouteModel
	catalog          *AgentCatalog
	systemPrompt     string
	grpoSystemPrompt string
	timeout          time.Duration
}

func NewMasterAgentRouter(model RouteModel, catalog *AgentCatalog, systemPrompt string, grpoSystemPrompts ...string) *MasterAgentRouter {
	grpoSystemPrompt := ""
	if len(grpoSystemPrompts) > 0 {
		grpoSystemPrompt = grpoSystemPrompts[0]
	}
	return &MasterAgentRouter{
		model:            model,
		catalog:          catalog,
		systemPrompt:     systemPrompt,
		grpoSystemPrompt: grpoSystemPrompt,
		timeout:          5 * time.Minute,
	}
}

// Route makes exactly one model call. Any model or validation failure returns
// the configured deterministic fallback and never escapes into retrieval scope.
func (r *MasterAgentRouter) Route(ctx context.Context, request RouteRequest) RouteDecision {
	if r == nil || r.model == nil || r.catalog == nil || strings.TrimSpace(r.systemPrompt) == "" {
		return fallbackRoute(request.OriginalQuery, "", ErrorCodeRouteUnavailable, catalogOrNil(r))
	}
	// Explicitly unsupported requests do not need an LLM decision. Besides
	// making this deterministic, the early exit prevents a slow or unavailable
	// route model from turning a simple out-of-service response into a
	// multi-minute request.
	if isExplicitOutOfServiceQuery(request.OriginalQuery) {
		return RouteDecision{Plan: MasterRoutePlan{
			StandaloneQuery: request.OriginalQuery,
			Intent:          "out_of_service",
			Outcome:         RouteOutcomeOutOfService,
			Entities:        map[string]string{},
		}}
	}
	modelCtx := ctx
	cancel := func() {}
	if r.timeout > 0 {
		modelCtx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	response, err := r.model.GenerateRoute(modelCtx, RouteModelRequest{
		SystemPrompt:      r.systemPrompt,
		GRPOSystemPrompt:  r.grpoSystemPrompt,
		OriginalQuery:     request.OriginalQuery,
		History:           slices.Clone(request.History),
		Agents:            r.catalog.Agents(),
		MaxSelectedAgents: r.catalog.MaxSelectedAgents(),
		ModelID:           request.ModelID,
	})
	if err != nil {
		errorCode := ErrorCodeRouteModelFailed
		if errors.Is(err, ErrRouteInvalidModelOutput) {
			errorCode = ErrorCodeRouteInvalidOutput
		}
		return fallbackRoute(request.OriginalQuery, response.ModelCallID, errorCode, r.catalog)
	}
	plan, err := decodeAndValidateRoute(response.Content, r.catalog)
	if err != nil {
		return fallbackRoute(request.OriginalQuery, response.ModelCallID, ErrorCodeRouteInvalidOutput, r.catalog)
	}
	return reconcileRouteWithKeywordMatches(plan, request.OriginalQuery, response.ModelCallID, r.catalog)
}

// reconcileRouteWithKeywordMatches treats configured route keywords as a
// deterministic domain boundary. The model still supplies the task details and
// topic selection for every correctly selected domain, while explicit keyword
// matches remove unsupported extra domains and restore omitted domains. When a
// query has no configured keyword match, the validated model decision is kept.
func reconcileRouteWithKeywordMatches(
	plan MasterRoutePlan,
	originalQuery string,
	modelCallID string,
	catalog *AgentCatalog,
) RouteDecision {
	if isExplicitOutOfServiceQuery(originalQuery) {
		if plan.Outcome == RouteOutcomeOutOfService && len(plan.Tasks) == 0 {
			return RouteDecision{Plan: plan, ModelCallID: modelCallID}
		}
		return RouteDecision{
			Plan: MasterRoutePlan{
				StandaloneQuery: originalQuery,
				Intent:          "out_of_service",
				Outcome:         RouteOutcomeOutOfService,
				Entities:        map[string]string{},
				Tasks:           nil,
				Degraded:        true,
			},
			ModelCallID: modelCallID,
			Degraded:    true,
		}
	}
	matchedAgentIDs := fallbackAgentIDsForQuery(originalQuery, catalog)
	if len(matchedAgentIDs) == 0 {
		return RouteDecision{Plan: plan, ModelCallID: modelCallID}
	}

	existingTasks := make(map[string]AgentTask, len(plan.Tasks))
	for _, task := range plan.Tasks {
		existingTasks[task.AgentID] = task
	}
	reconciledTasks := make([]AgentTask, 0, len(matchedAgentIDs))
	changed := plan.Outcome != RouteOutcomeRouted || len(plan.Tasks) != len(matchedAgentIDs)
	for _, agentID := range matchedAgentIDs {
		if task, ok := existingTasks[agentID]; ok {
			if profile, profileOK := catalog.Get(agentID); profileOK {
				matchedTopics := matchedTopicIDs(profile, originalQuery)
				if len(matchedTopics) > 0 && !slices.Equal(task.TopicIDs, matchedTopics) {
					task.TopicIDs = matchedTopics
					changed = true
				}
			}
			reconciledTasks = append(reconciledTasks, task)
			continue
		}
		profile, ok := catalog.Get(agentID)
		if !ok {
			continue
		}
		changed = true
		reconciledTasks = append(reconciledTasks, fallbackTask(profile, originalQuery))
	}
	if !changed {
		for index, task := range reconciledTasks {
			if plan.Tasks[index].AgentID != task.AgentID {
				changed = true
				break
			}
		}
	}
	if !changed {
		return RouteDecision{Plan: plan, ModelCallID: modelCallID}
	}
	return RouteDecision{
		Plan: MasterRoutePlan{
			StandaloneQuery: originalQuery,
			Intent:          plan.Intent,
			Outcome:         RouteOutcomeRouted,
			Entities:        plan.Entities,
			Tasks:           reconciledTasks,
			Degraded:        true,
		},
		ModelCallID: modelCallID,
		Degraded:    true,
	}
}

func decodeAndValidateRoute(content string, catalog *AgentCatalog) (MasterRoutePlan, error) {
	if len(content) == 0 || len(content) > maxRouteResponseBytes {
		return MasterRoutePlan{}, fmt.Errorf("route output size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var plan MasterRoutePlan
	if err := decoder.Decode(&plan); err != nil {
		return MasterRoutePlan{}, fmt.Errorf("decode route output: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return MasterRoutePlan{}, fmt.Errorf("multiple JSON values are not allowed")
		}
		return MasterRoutePlan{}, fmt.Errorf("decode trailing route output: %w", err)
	}
	if strings.TrimSpace(plan.StandaloneQuery) == "" || strings.TrimSpace(plan.Intent) == "" {
		return MasterRoutePlan{}, fmt.Errorf("standalone_query and intent are required")
	}
	if plan.Outcome == "" {
		if len(plan.Tasks) > 0 {
			plan.Outcome = RouteOutcomeRouted
		} else {
			plan.Outcome = RouteOutcomeOutOfCoverage
		}
	}
	switch plan.Outcome {
	case RouteOutcomeRouted:
		if len(plan.Tasks) < 1 || len(plan.Tasks) > catalog.MaxSelectedAgents() {
			return MasterRoutePlan{}, fmt.Errorf("routed plan must contain between one and %d tasks", catalog.MaxSelectedAgents())
		}
	case RouteOutcomeOutOfService, RouteOutcomeOutOfCoverage:
		if len(plan.Tasks) != 0 {
			return MasterRoutePlan{}, fmt.Errorf("route outcome %q must not contain tasks", plan.Outcome)
		}
	default:
		return MasterRoutePlan{}, fmt.Errorf("invalid route outcome %q", plan.Outcome)
	}
	if len(plan.Entities) > 32 {
		return MasterRoutePlan{}, fmt.Errorf("too many route entities")
	}
	for key, value := range plan.Entities {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return MasterRoutePlan{}, fmt.Errorf("route entities must have non-empty keys and values")
		}
	}

	seen := make(map[string]struct{}, len(plan.Tasks))
	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		profile, ok := catalog.Get(task.AgentID)
		if !ok {
			return MasterRoutePlan{}, fmt.Errorf("agent %q is not in the enabled catalog", task.AgentID)
		}
		if _, duplicate := seen[task.AgentID]; duplicate {
			return MasterRoutePlan{}, fmt.Errorf("agent %q appears more than once", task.AgentID)
		}
		seen[task.AgentID] = struct{}{}
		if strings.TrimSpace(task.Goal) == "" || len(task.SearchQueries) == 0 || len(task.SearchQueries) > 3 {
			return MasterRoutePlan{}, fmt.Errorf("agent %q task goal and one to three search queries are required", task.AgentID)
		}
		if err := validateRouteStrings(task.SearchQueries, 3); err != nil {
			return MasterRoutePlan{}, fmt.Errorf("agent %q search_queries: %w", task.AgentID, err)
		}
		if err := validateRouteStrings(task.ExactTerms, 10); err != nil {
			return MasterRoutePlan{}, fmt.Errorf("agent %q exact_terms: %w", task.AgentID, err)
		}
		if err := validateRouteStrings(task.DocumentTypes, 5); err != nil {
			return MasterRoutePlan{}, fmt.Errorf("agent %q document_types: %w", task.AgentID, err)
		}
		if task.ToolIntent != "" && !slices.Contains(profile.AllowedResearchTools, task.ToolIntent) {
			return MasterRoutePlan{}, fmt.Errorf("agent %q tool_intent %q is not allowed", task.AgentID, task.ToolIntent)
		}
		if len(profile.Topics) > 0 {
			if len(task.TopicIDs) == 0 {
				task.TopicIDs = matchedTopicIDs(profile, strings.Join(append([]string{plan.StandaloneQuery}, task.SearchQueries...), " "))
			}
			if len(task.TopicIDs) == 0 {
				return MasterRoutePlan{}, fmt.Errorf("agent %q must select at least one configured topic", task.AgentID)
			}
			allowedTopics := make(map[string]int, len(profile.Topics))
			for index, topic := range profile.Topics {
				allowedTopics[topic.ID] = index
			}
			seenTopics := make(map[string]struct{}, len(task.TopicIDs))
			for _, topicID := range task.TopicIDs {
				if _, ok := allowedTopics[topicID]; !ok {
					return MasterRoutePlan{}, fmt.Errorf("topic %q does not belong to agent %q", topicID, task.AgentID)
				}
				if _, duplicate := seenTopics[topicID]; duplicate {
					return MasterRoutePlan{}, fmt.Errorf("agent %q contains duplicate topic %q", task.AgentID, topicID)
				}
				seenTopics[topicID] = struct{}{}
			}
			slices.SortFunc(task.TopicIDs, func(a, b string) int { return allowedTopics[a] - allowedTopics[b] })
		} else if len(task.TopicIDs) > 0 {
			return MasterRoutePlan{}, fmt.Errorf("agent %q does not define topics", task.AgentID)
		}
	}
	slices.SortFunc(plan.Tasks, func(a, b AgentTask) int {
		return catalog.OrderOf(a.AgentID) - catalog.OrderOf(b.AgentID)
	})
	return plan, nil
}

func validateRouteStrings(values []string, max int) error {
	if len(values) > max {
		return fmt.Errorf("contains more than %d values", max)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("contains an empty value")
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("contains duplicate value %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func fallbackRoute(originalQuery, modelCallID, errorCode string, catalog *AgentCatalog) RouteDecision {
	tasks := make([]AgentTask, 0)
	if catalog != nil {
		agentIDs := fallbackAgentIDsForQuery(originalQuery, catalog)
		for _, agentID := range agentIDs {
			profile, ok := catalog.Get(agentID)
			if !ok {
				continue
			}
			tasks = append(tasks, fallbackTask(profile, originalQuery))
		}
	}
	outcome := RouteOutcomeRouted
	if len(tasks) == 0 {
		outcome = classifyFailedRouteQuery(originalQuery)
	}
	return RouteDecision{
		Plan: MasterRoutePlan{
			StandaloneQuery: originalQuery,
			Intent:          "unknown",
			Outcome:         outcome,
			Tasks:           tasks,
			Degraded:        true,
		},
		ModelCallID: modelCallID,
		Degraded:    true,
		ErrorCode:   errorCode,
	}
}

func fallbackTask(profile DomainAgentProfile, originalQuery string) AgentTask {
	return AgentTask{
		AgentID:       profile.ID,
		TopicIDs:      fallbackTopicIDs(profile, originalQuery),
		Goal:          "Research the complete user question according to the responsibilities of " + profile.Name + ".",
		SearchQueries: []string{originalQuery},
	}
}

func fallbackAgentIDsForQuery(query string, catalog *AgentCatalog) []string {
	if catalog == nil {
		return nil
	}
	normalized := strings.ToLower(query)
	matched := make([]string, 0, catalog.MaxSelectedAgents())
	for _, profile := range catalog.Agents() {
		for _, keyword := range profile.RouteKeywords {
			if strings.Contains(normalized, strings.ToLower(keyword)) {
				matched = append(matched, profile.ID)
				break
			}
		}
		if len(matched) == catalog.MaxSelectedAgents() {
			return matched
		}
	}
	return matched
}

func matchedTopicIDs(profile DomainAgentProfile, query string) []string {
	normalized := strings.ToLower(query)
	matched := make([]string, 0, len(profile.Topics))
	for _, topic := range profile.Topics {
		for _, keyword := range topic.RouteKeywords {
			if strings.Contains(normalized, strings.ToLower(strings.TrimSpace(keyword))) {
				matched = append(matched, topic.ID)
				break
			}
		}
	}
	return matched
}

func fallbackTopicIDs(profile DomainAgentProfile, query string) []string {
	matched := matchedTopicIDs(profile, query)
	if len(matched) > 0 || len(profile.Topics) == 0 {
		return matched
	}
	all := make([]string, 0, len(profile.Topics))
	for _, topic := range profile.Topics {
		all = append(all, topic.ID)
	}
	return all
}

func classifyUnroutedQuery(query string) string {
	normalized := strings.ToLower(query)
	for _, marker := range []string{
		"政策", "制度", "公司", "罗氏", "审批", "报销", "差旅", "合规", "授权", "流程", "费用", "发票",
		"policy", "company", "roche", "approval", "reimburse", "reimbursement", "expense", "compliance", "authority", "invoice",
	} {
		if strings.Contains(normalized, marker) {
			return RouteOutcomeOutOfCoverage
		}
	}
	return RouteOutcomeOutOfService
}

// classifyFailedRouteQuery is deliberately more conservative than
// classifyUnroutedQuery. A valid empty route is an explicit model decision and
// can default to out_of_service. After a technical or format failure, however,
// only an unmistakably external question should be presented that way; an
// ambiguous question remains route_failed so an outage is not disguised as a
// business classification.
func classifyFailedRouteQuery(query string) string {
	if isExplicitOutOfServiceQuery(query) {
		return RouteOutcomeOutOfService
	}
	normalized := strings.ToLower(query)
	for _, marker := range []string{
		"政策", "制度", "公司", "罗氏", "审批", "报销", "差旅", "合规", "授权", "流程", "费用", "发票",
		"policy", "company", "roche", "approval", "reimburse", "reimbursement", "expense", "compliance", "authority", "invoice",
	} {
		if strings.Contains(normalized, marker) {
			return RouteOutcomeOutOfCoverage
		}
	}
	return RouteOutcomeFailed
}

func isExplicitOutOfServiceQuery(query string) bool {
	normalized := strings.ToLower(query)
	for _, marker := range []string{
		"天气", "气温", "降雨", "下雨", "空气质量", "股票", "股价", "新闻", "彩票", "菜谱",
		"weather", "temperature", "forecast", "air quality", "stock price", "lottery", "recipe",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func catalogOrNil(router *MasterAgentRouter) *AgentCatalog {
	if router == nil {
		return nil
	}
	return router.catalog
}
