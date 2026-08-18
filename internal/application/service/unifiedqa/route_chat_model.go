package unifiedqa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/types"
)

type RouteChatModelProvider interface {
	GetModelByID(ctx context.Context, id string) (*types.Model, error)
	ListModels(ctx context.Context) ([]*types.Model, error)
	GetChatModel(ctx context.Context, id string) (chat.Chat, error)
}

type ChatRouteModel struct {
	models               RouteChatModelProvider
	configuredRouteModel *config.UnifiedQARouteModelConfig
}

const (
	routeModelUsage          = "unified_qa_route"
	routeOutputSchemaGRPO    = "grpo"
	routeModelUsageConfigKey = "usage"
	routeOutputSchemaKey     = "output_schema"
)

func NewChatRouteModel(models RouteChatModelProvider, configuredRouteModel ...*config.UnifiedQARouteModelConfig) *ChatRouteModel {
	var routeModel *config.UnifiedQARouteModelConfig
	if len(configuredRouteModel) > 0 {
		routeModel = configuredRouteModel[0]
	}
	return &ChatRouteModel{models: models, configuredRouteModel: routeModel}
}

func (m *ChatRouteModel) GenerateRoute(ctx context.Context, request RouteModelRequest) (RouteModelResponse, error) {
	callID := uuid.NewString()
	modelID, outputSchema, err := m.resolveRouteModel(ctx, request.ModelID)
	if err != nil {
		return RouteModelResponse{ModelCallID: callID}, err
	}
	var format json.RawMessage
	if outputSchema == routeOutputSchemaGRPO {
		format, err = buildGRPORouteJSONSchema(request.Agents, request.MaxSelectedAgents)
	} else {
		format, err = buildRouteJSONSchema(request.Agents, request.MaxSelectedAgents)
	}
	if err != nil {
		return RouteModelResponse{ModelCallID: callID}, err
	}
	systemPrompt := request.SystemPrompt
	if outputSchema == routeOutputSchemaGRPO && strings.TrimSpace(request.GRPOSystemPrompt) != "" {
		systemPrompt = request.GRPOSystemPrompt
	}
	chatModel, err := m.getRouteChatModel(ctx, modelID)
	if err != nil {
		return RouteModelResponse{ModelCallID: callID}, fmt.Errorf("get route chat model: %w", err)
	}
	userInput, err := json.Marshal(struct {
		OriginalQuery     string               `json:"original_query"`
		History           []ConversationTurn   `json:"history,omitempty"`
		Agents            []DomainAgentProfile `json:"agents"`
		MaxSelectedAgents int                  `json:"max_selected_agents"`
	}{
		OriginalQuery:     request.OriginalQuery,
		History:           request.History,
		Agents:            request.Agents,
		MaxSelectedAgents: request.MaxSelectedAgents,
	})
	if err != nil {
		return RouteModelResponse{ModelCallID: callID}, fmt.Errorf("marshal route input: %w", err)
	}
	thinking := false
	response, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(userInput)},
	}, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: 1200,
		Thinking:            &thinking,
		Format:              format,
	})
	if err != nil {
		return RouteModelResponse{ModelCallID: callID}, fmt.Errorf("generate route: %w", err)
	}
	if response == nil {
		return RouteModelResponse{ModelCallID: callID}, fmt.Errorf("generate route: empty model response")
	}
	if outputSchema == routeOutputSchemaGRPO {
		normalized, normalizeErr := normalizeGRPORouteResponse(response.Content, request)
		if normalizeErr != nil {
			return RouteModelResponse{ModelCallID: callID}, fmt.Errorf("%w: normalize grpo route: %v", ErrRouteInvalidModelOutput, normalizeErr)
		}
		response.Content = normalized
	}
	return RouteModelResponse{Content: response.Content, ModelCallID: callID}, nil
}

func (m *ChatRouteModel) getRouteChatModel(ctx context.Context, modelID string) (chat.Chat, error) {
	if m.configuredRouteModel != nil && modelID == strings.TrimSpace(m.configuredRouteModel.ID) {
		return chat.NewChat(&chat.ChatConfig{
			Source:    types.ModelSourceRemote,
			ModelID:   modelID,
			ModelName: strings.TrimSpace(m.configuredRouteModel.ModelName),
			BaseURL:   strings.TrimSpace(m.configuredRouteModel.BaseURL),
			APIKey:    strings.TrimSpace(m.configuredRouteModel.APIKey),
			Provider:  strings.TrimSpace(m.configuredRouteModel.Provider),
		})
	}
	return m.models.GetChatModel(ctx, modelID)
}

// resolveRouteModel keeps routing-model selection independent from the model
// used for evidence review and final answer generation. An active KnowledgeQA
// model with extra_config.usage=unified_qa_route wins. If the database does not
// designate one, the configured route model is used before the existing
// request/default resolution behaviour.
func (m *ChatRouteModel) resolveRouteModel(ctx context.Context, preferred string) (string, string, error) {
	if m == nil || m.models == nil {
		return "", "", fmt.Errorf("route model provider is required")
	}
	models, err := m.models.ListModels(ctx)
	if err != nil {
		return "", "", fmt.Errorf("list route models: %w", err)
	}
	candidates := make([]*types.Model, 0)
	for _, model := range models {
		if model == nil || model.Type != types.ModelTypeKnowledgeQA || model.DeletedAt.Valid ||
			(model.Status != "" && model.Status != types.ModelStatusActive) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(model.Parameters.ExtraConfig[routeModelUsageConfigKey]), routeModelUsage) {
			candidates = append(candidates, model)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].IsDefault != candidates[j].IsDefault {
			return candidates[i].IsDefault
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > 0 {
		return candidates[0].ID,
			strings.ToLower(strings.TrimSpace(candidates[0].Parameters.ExtraConfig[routeOutputSchemaKey])), nil
	}
	if routeModel := m.configuredRouteModel; routeModel != nil {
		if strings.TrimSpace(routeModel.ID) == "" || strings.TrimSpace(routeModel.ModelName) == "" ||
			strings.TrimSpace(routeModel.BaseURL) == "" {
			return "", "", fmt.Errorf("configured route model requires id, model_name, and base_url")
		}
		outputSchema := strings.ToLower(strings.TrimSpace(routeModel.OutputSchema))
		if outputSchema != "" && outputSchema != routeOutputSchemaGRPO {
			return "", "", fmt.Errorf("configured route model output_schema %q is unsupported", outputSchema)
		}
		return strings.TrimSpace(routeModel.ID), outputSchema, nil
	}
	modelID, err := m.resolveModelID(ctx, preferred)
	return modelID, "", err
}

func (m *ChatRouteModel) resolveModelID(ctx context.Context, preferred string) (string, error) {
	if m == nil || m.models == nil {
		return "", fmt.Errorf("route model provider is required")
	}
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		model, err := m.models.GetModelByID(ctx, preferred)
		if err != nil || model == nil || model.Type != types.ModelTypeKnowledgeQA || model.DeletedAt.Valid {
			return "", fmt.Errorf("preferred route model %q is unavailable", preferred)
		}
		return preferred, nil
	}
	models, err := m.models.ListModels(ctx)
	if err != nil {
		return "", fmt.Errorf("list route models: %w", err)
	}
	candidates := make([]*types.Model, 0, len(models))
	for _, model := range models {
		if model != nil && model.Type == types.ModelTypeKnowledgeQA && !model.DeletedAt.Valid &&
			(model.Status == "" || model.Status == types.ModelStatusActive) {
			candidates = append(candidates, model)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].IsDefault != candidates[j].IsDefault {
			return candidates[i].IsDefault
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) == 0 {
		return "", fmt.Errorf("no active KnowledgeQA model is available")
	}
	return candidates[0].ID, nil
}

func buildRouteJSONSchema(agents []DomainAgentProfile, maxSelectedAgents int) (json.RawMessage, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("route schema requires at least one agent")
	}
	if maxSelectedAgents < 1 || maxSelectedAgents > len(agents) {
		return nil, fmt.Errorf("route schema max selected agents is invalid")
	}
	ids := make([]string, 0, len(agents))
	topicIDs := make([]string, 0)
	for _, agent := range agents {
		if strings.TrimSpace(agent.ID) == "" {
			return nil, fmt.Errorf("route schema contains an empty agent ID")
		}
		ids = append(ids, agent.ID)
		for _, topic := range agent.Topics {
			topicIDs = append(topicIDs, topic.ID)
		}
	}
	encodedIDs, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("marshal route agent IDs: %w", err)
	}
	taskRequired := `["agent_id","goal","search_queries"]`
	topicProperty := ""
	if len(topicIDs) > 0 {
		encodedTopicIDs, marshalErr := json.Marshal(topicIDs)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal route topic IDs: %w", marshalErr)
		}
		taskRequired = `["agent_id","topic_ids","goal","search_queries"]`
		topicProperty = fmt.Sprintf(`"topic_ids":{"type":"array","minItems":1,"maxItems":%d,"items":{"type":"string","enum":%s}},`, len(topicIDs), encodedTopicIDs)
	}
	return json.RawMessage(fmt.Sprintf(`{
  "type":"object",
  "additionalProperties":false,
  "required":["standalone_query","intent","outcome","entities","tasks"],
  "properties":{
    "standalone_query":{"type":"string"},
    "intent":{"type":"string"},
    "outcome":{"type":"string","enum":["routed","out_of_service","out_of_coverage"]},
    "entities":{"type":"object","additionalProperties":{"type":"string"}},
    "tasks":{"type":"array","maxItems":%d,"items":{
      "type":"object","additionalProperties":false,
      "required":%s,
      "properties":{
        "agent_id":{"type":"string","enum":%s},
        %s
        "goal":{"type":"string"},
        "search_queries":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"string"}},
        "exact_terms":{"type":"array","maxItems":10,"items":{"type":"string"}},
        "document_types":{"type":"array","maxItems":5,"items":{"type":"string"}},
        "tool_intent":{"type":"string"}
      }
    }}
  }
}`, maxSelectedAgents, taskRequired, encodedIDs, topicProperty)), nil
}

func buildGRPORouteJSONSchema(agents []DomainAgentProfile, maxSelectedAgents int) (json.RawMessage, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("route schema requires at least one agent")
	}
	if maxSelectedAgents < 1 || maxSelectedAgents > len(agents) {
		return nil, fmt.Errorf("route schema max selected agents is invalid")
	}
	domains := make([]string, 0, len(agents))
	for _, agent := range agents {
		if strings.TrimSpace(agent.ID) == "" {
			return nil, fmt.Errorf("route schema contains an empty agent ID")
		}
		domains = append(domains, strings.ToUpper(agent.ID[:1])+agent.ID[1:])
	}
	encodedDomains, err := json.Marshal(domains)
	if err != nil {
		return nil, fmt.Errorf("marshal grpo route domains: %w", err)
	}
	return json.RawMessage(fmt.Sprintf(`{
  "type":"object",
  "additionalProperties":false,
  "required":["label"],
  "properties":{
    "label":{"type":"array","maxItems":%d,"uniqueItems":true,"items":{"type":"string","enum":%s}}
  }
}`, maxSelectedAgents, encodedDomains)), nil
}

type grpoRouteResponse struct {
	// CanSplit and Tasks are retained only so responses produced by the legacy
	// three-field protocol can be decoded during rollout. RawMessage accepts
	// their historically unstable shapes; routing never reads their contents.
	CanSplit json.RawMessage `json:"can_split"`
	Labels   []string        `json:"label"`
	Tasks    json.RawMessage `json:"tasks"`
}

func normalizeGRPORouteResponse(content string, request RouteModelRequest) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	var response grpoRouteResponse
	if err := decoder.Decode(&response); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("multiple JSON values are not allowed")
		}
		return "", fmt.Errorf("decode trailing response: %w", err)
	}
	if response.Labels == nil {
		return "", fmt.Errorf("label is required")
	}

	agentIDs := make(map[string]string, len(request.Agents))
	for _, agent := range request.Agents {
		agentIDs[strings.ToLower(strings.TrimSpace(agent.ID))] = agent.ID
	}
	tasks := make([]AgentTask, 0, request.MaxSelectedAgents)
	seen := make(map[string]struct{}, request.MaxSelectedAgents)
	appendTask := func(domain string) {
		agentID, ok := agentIDs[strings.ToLower(strings.TrimSpace(domain))]
		if !ok || len(tasks) >= request.MaxSelectedAgents {
			return
		}
		if _, duplicate := seen[agentID]; duplicate {
			return
		}
		problem := strings.TrimSpace(request.OriginalQuery)
		seen[agentID] = struct{}{}
		profile, _ := findAgentProfile(request.Agents, agentID)
		tasks = append(tasks, AgentTask{
			AgentID:       agentID,
			TopicIDs:      fallbackTopicIDs(profile, request.OriginalQuery),
			Goal:          problem,
			SearchQueries: []string{problem},
		})
	}
	// Labels are the only routing decision. Legacy can_split and task objects
	// are accepted by the decoder above but never affect scope or task content.
	for _, label := range response.Labels {
		appendTask(label)
	}

	intentParts := make([]string, 0, len(tasks))
	for _, task := range tasks {
		intentParts = append(intentParts, task.AgentID)
	}
	intent := "general"
	if len(intentParts) > 0 {
		intent = strings.Join(intentParts, "_and_")
	}
	plan := MasterRoutePlan{
		StandaloneQuery: strings.TrimSpace(request.OriginalQuery),
		Intent:          intent,
		Outcome:         RouteOutcomeRouted,
		Entities:        map[string]string{},
		Tasks:           tasks,
	}
	if len(tasks) == 0 {
		plan.Outcome = classifyUnroutedQuery(request.OriginalQuery)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("encode normalized route: %w", err)
	}
	return string(encoded), nil
}

func findAgentProfile(agents []DomainAgentProfile, agentID string) (DomainAgentProfile, bool) {
	for _, agent := range agents {
		if agent.ID == agentID {
			return agent, true
		}
	}
	return DomainAgentProfile{}, false
}
