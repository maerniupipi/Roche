package unifiedqa

import (
	"context"
	"errors"
	"strings"
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestChatRouteModelSelectsDefaultKnowledgeQAModelAndRequestsJSON(t *testing.T) {
	chatModel := &fakeRouteChat{response: &types.ChatResponse{Content: `{"tasks":[]}`}}
	provider := &fakeRouteChatProvider{
		models: []*types.Model{
			{ID: "model-b", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
			{ID: "model-a", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true},
			{ID: "embedding", Type: types.ModelTypeEmbedding, IsDefault: true},
		},
		chat: chatModel,
	}
	model := NewChatRouteModel(provider)

	catalog := mustTestCatalog(t)
	response, err := model.GenerateRoute(context.Background(), RouteModelRequest{
		SystemPrompt:      "route",
		OriginalQuery:     "question",
		Agents:            catalog.Agents(),
		MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err != nil {
		t.Fatalf("GenerateRoute() error = %v", err)
	}
	if provider.requestedModelID != "model-a" {
		t.Fatalf("requested model = %q", provider.requestedModelID)
	}
	if response.Content != `{"tasks":[]}` || response.ModelCallID == "" {
		t.Fatalf("response = %+v", response)
	}
	if chatModel.options == nil || len(chatModel.options.Format) == 0 || chatModel.options.Temperature != 0 {
		t.Fatalf("chat options = %+v", chatModel.options)
	}
	schema := string(chatModel.options.Format)
	if !strings.Contains(schema, `"enum":["finance","compliance"]`) || !strings.Contains(schema, `"maxItems":2`) {
		t.Fatalf("route schema = %s", schema)
	}
	if !strings.Contains(chatModel.messages[1].Content, `"max_selected_agents":2`) {
		t.Fatalf("route user input = %s", chatModel.messages[1].Content)
	}
	if len(chatModel.messages) != 2 || chatModel.messages[0].Role != "system" || chatModel.messages[1].Role != "user" {
		t.Fatalf("messages = %+v", chatModel.messages)
	}
}

func TestBuildRouteJSONSchemaUsesConfiguredAgentIDs(t *testing.T) {
	catalog, err := NewAgentCatalog(testThreeAgentCatalogConfig(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	schema, err := buildRouteJSONSchema(catalog.Agents(), catalog.MaxSelectedAgents())
	if err != nil {
		t.Fatalf("buildRouteJSONSchema() error = %v", err)
	}
	if got := string(schema); !strings.Contains(got, `"enum":["finance","compliance","hr"]`) || !strings.Contains(got, `"maxItems":3`) {
		t.Fatalf("route schema = %s", got)
	}
}

func TestBuildRouteJSONSchemaIncludesConfiguredTopicsAndOutcomes(t *testing.T) {
	catalog := mustTestTopicCatalog(t)
	schema, err := buildRouteJSONSchema(catalog.Agents(), catalog.MaxSelectedAgents())
	if err != nil {
		t.Fatalf("buildRouteJSONSchema() error = %v", err)
	}
	got := string(schema)
	for _, expected := range []string{`"outcome"`, `"topic_ids"`, `"doa"`, `"travel_expense"`, `"out_of_service"`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("schema does not contain %q: %s", expected, got)
		}
	}
}

func TestChatRouteModelSelectsDedicatedGRPOModelAndNormalizesOutput(t *testing.T) {
	chatModel := &fakeRouteChat{response: &types.ChatResponse{Content: `{
  "label":["Finance","Compliance"]
}`}}
	provider := &fakeRouteChatProvider{
		models: []*types.Model{
			{ID: "answer-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
			{ID: "route-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive,
				Parameters: types.ModelParameters{ExtraConfig: map[string]string{
					routeModelUsageConfigKey: routeModelUsage,
					routeOutputSchemaKey:     routeOutputSchemaGRPO,
				}}},
		},
		chat: chatModel,
	}
	model := NewChatRouteModel(provider)
	catalog := mustTestCatalog(t)

	response, err := model.GenerateRoute(context.Background(), RouteModelRequest{
		SystemPrompt:      "route",
		GRPOSystemPrompt:  "grpo route",
		OriginalQuery:     "Can this expense be paid?",
		Agents:            catalog.Agents(),
		MaxSelectedAgents: catalog.MaxSelectedAgents(),
		ModelID:           "answer-model",
	})
	if err != nil {
		t.Fatalf("GenerateRoute() error = %v", err)
	}
	if provider.requestedModelID != "route-model" {
		t.Fatalf("requested model = %q", provider.requestedModelID)
	}
	plan, err := decodeAndValidateRoute(response.Content, catalog)
	if err != nil {
		t.Fatalf("normalized route is invalid: %v; content=%s", err, response.Content)
	}
	if plan.StandaloneQuery != "Can this expense be paid?" || plan.Intent != "finance_and_compliance" {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Tasks) != 2 || plan.Tasks[0].AgentID != "finance" || plan.Tasks[1].AgentID != "compliance" {
		t.Fatalf("tasks = %+v", plan.Tasks)
	}
	if chatModel.options == nil {
		t.Fatalf("GRPO format was not requested: %+v", chatModel.options)
	}
	format := string(chatModel.options.Format)
	if !strings.Contains(format, `"required":["label"]`) || !strings.Contains(format, `"label"`) ||
		strings.Contains(format, `"can_split"`) || strings.Contains(format, `"tasks"`) {
		t.Fatalf("GRPO format must contain only label: %s", format)
	}
	if len(chatModel.messages) == 0 || chatModel.messages[0].Content != "grpo route" {
		t.Fatalf("GRPO system prompt was not selected: %+v", chatModel.messages)
	}
}

func TestChatRouteModelMarksMalformedGRPOResponseAsInvalidOutput(t *testing.T) {
	chatModel := &fakeRouteChat{response: &types.ChatResponse{Content: `not-json`}}
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{
			ID: "route-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive,
			Parameters: types.ModelParameters{ExtraConfig: map[string]string{
				routeModelUsageConfigKey: routeModelUsage,
				routeOutputSchemaKey:     routeOutputSchemaGRPO,
			}},
		}},
		chat: chatModel,
	}
	_, err := NewChatRouteModel(provider).GenerateRoute(context.Background(), RouteModelRequest{
		SystemPrompt: "route", OriginalQuery: "question", Agents: mustTestCatalog(t).Agents(), MaxSelectedAgents: 2,
	})
	if !errors.Is(err, ErrRouteInvalidModelOutput) {
		t.Fatalf("GenerateRoute() error = %v", err)
	}
}

func TestChatRouteModelUsesConfiguredModelWhenDatabaseHasNoDedicatedRouteModel(t *testing.T) {
	t.Setenv("SSRF_WHITELIST_EXTRA", "10.3.97.217")
	provider := &fakeRouteChatProvider{
		models: []*types.Model{
			{ID: "answer-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true},
		},
	}
	model := NewChatRouteModel(provider, &config.UnifiedQARouteModelConfig{
		ID: "router-grpo", ModelName: "router-grpo", BaseURL: "http://10.3.97.217:11434/v1",
		APIKey: "ollama", Provider: "generic", OutputSchema: "grpo",
	})

	modelID, schema, err := model.resolveRouteModel(context.Background(), "answer-model")
	if err != nil {
		t.Fatalf("resolveRouteModel() error = %v", err)
	}
	if modelID != "router-grpo" || schema != "grpo" {
		t.Fatalf("route model = %q, schema = %q", modelID, schema)
	}
	chatModel, err := model.getRouteChatModel(context.Background(), modelID)
	if err != nil {
		t.Fatalf("getRouteChatModel() error = %v", err)
	}
	if chatModel.GetModelID() != "router-grpo" || chatModel.GetModelName() != "router-grpo" {
		t.Fatalf("configured chat model ID/name = %q/%q", chatModel.GetModelID(), chatModel.GetModelName())
	}
}

func TestChatRouteModelDatabaseDedicatedModelOverridesConfiguredModel(t *testing.T) {
	provider := &fakeRouteChatProvider{models: []*types.Model{
		{ID: "configured-route-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
		{ID: "database-route-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive,
			Parameters: types.ModelParameters{ExtraConfig: map[string]string{routeModelUsageConfigKey: routeModelUsage}}},
	}}
	model := NewChatRouteModel(provider, &config.UnifiedQARouteModelConfig{
		ID: "configured-route-model", ModelName: "router-grpo", BaseURL: "http://router.example/v1",
	})

	modelID, _, err := model.resolveRouteModel(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveRouteModel() error = %v", err)
	}
	if modelID != "database-route-model" {
		t.Fatalf("model ID = %q", modelID)
	}
}

func TestChatRouteModelRejectsInvalidConfiguredModel(t *testing.T) {
	provider := &fakeRouteChatProvider{}
	model := NewChatRouteModel(provider, &config.UnifiedQARouteModelConfig{ID: "router-grpo"})

	_, _, err := model.resolveRouteModel(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "requires id, model_name, and base_url") {
		t.Fatalf("resolveRouteModel() error = %v", err)
	}
}

func TestNormalizeGRPORouteResponseUsesLabelOnlyProtocol(t *testing.T) {
	catalog := mustTestCatalog(t)
	content, err := normalizeGRPORouteResponse(`{"label":["Compliance"]}`, RouteModelRequest{
		OriginalQuery:     "gift policy",
		Agents:            catalog.Agents(),
		MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err != nil {
		t.Fatalf("normalizeGRPORouteResponse() error = %v", err)
	}
	plan, err := decodeAndValidateRoute(content, catalog)
	if err != nil {
		t.Fatalf("normalized route is invalid: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].AgentID != "compliance" || plan.Tasks[0].SearchQueries[0] != "gift policy" {
		t.Fatalf("tasks = %+v", plan.Tasks)
	}
}

func TestNormalizeGRPORouteResponseIgnoresUnstableLegacyFields(t *testing.T) {
	catalog := mustTestCatalog(t)
	content, err := normalizeGRPORouteResponse(`{
		"can_split":"not-a-boolean",
		"label":["Finance"],
		"tasks":["placeholder",{"unexpected":true}]
	}`, RouteModelRequest{
		OriginalQuery:     "Can I reimburse this business meal?",
		Agents:            catalog.Agents(),
		MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err != nil {
		t.Fatalf("normalizeGRPORouteResponse() error = %v", err)
	}
	plan, err := decodeAndValidateRoute(content, catalog)
	if err != nil {
		t.Fatalf("normalized route is invalid: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].AgentID != "finance" {
		t.Fatalf("tasks = %+v", plan.Tasks)
	}
	if plan.Tasks[0].Goal != "Can I reimburse this business meal?" || plan.Tasks[0].SearchQueries[0] != "Can I reimburse this business meal?" {
		t.Fatalf("legacy task content must be ignored: %+v", plan.Tasks[0])
	}
}

func TestNormalizeGRPORouteResponseIgnoresLegacyTaskProblem(t *testing.T) {
	catalog := mustTestCatalog(t)
	content, err := normalizeGRPORouteResponse(`{
		"can_split":false,
		"label":["Finance"],
		"tasks":[
			{"domain":"Finance","id":"finance-task","problem":"Check reimbursement policy."},
			{"domain":"Compliance","id":"compliance-task","problem":"Check compliance policy."}
		]
	}`, RouteModelRequest{
		OriginalQuery:     "Can I reimburse this business meal?",
		Agents:            catalog.Agents(),
		MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err != nil {
		t.Fatalf("normalizeGRPORouteResponse() error = %v", err)
	}
	plan, err := decodeAndValidateRoute(content, catalog)
	if err != nil {
		t.Fatalf("normalized route is invalid: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].AgentID != "finance" || plan.Tasks[0].Goal != "Can I reimburse this business meal?" {
		t.Fatalf("tasks = %+v", plan.Tasks)
	}
}

func TestNormalizeGRPORouteResponseEmptyLabelsDoNotUseTasks(t *testing.T) {
	catalog := mustTestCatalog(t)
	content, err := normalizeGRPORouteResponse(`{
		"can_split":false,
		"label":[],
		"tasks":[{"domain":"Finance","id":"finance-task","problem":"Check reimbursement policy."}]
	}`, RouteModelRequest{
		OriginalQuery:     "Can I reimburse this business meal?",
		Agents:            catalog.Agents(),
		MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err != nil {
		t.Fatalf("normalizeGRPORouteResponse() error = %v", err)
	}
	plan, err := decodeAndValidateRoute(content, catalog)
	if err != nil || plan.Outcome != RouteOutcomeOutOfCoverage || len(plan.Tasks) != 0 {
		t.Fatalf("plan=%+v error=%v", plan, err)
	}
}

func TestNormalizeGRPORouteResponseClassifiesWeatherAsOutOfService(t *testing.T) {
	catalog := mustTestCatalog(t)
	content, err := normalizeGRPORouteResponse(`{"label":[]}`, RouteModelRequest{
		OriginalQuery: "What is the weather today?", Agents: catalog.Agents(), MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err != nil {
		t.Fatalf("normalizeGRPORouteResponse() error = %v", err)
	}
	plan, err := decodeAndValidateRoute(content, catalog)
	if err != nil || plan.Outcome != RouteOutcomeOutOfService || len(plan.Tasks) != 0 {
		t.Fatalf("plan=%+v error=%v", plan, err)
	}
}

func TestNormalizeGRPORouteResponseRequiresLabel(t *testing.T) {
	catalog := mustTestCatalog(t)
	_, err := normalizeGRPORouteResponse(`{"can_split":false,"tasks":[]}`, RouteModelRequest{
		OriginalQuery: "question", Agents: catalog.Agents(), MaxSelectedAgents: catalog.MaxSelectedAgents(),
	})
	if err == nil || !strings.Contains(err.Error(), "label is required") {
		t.Fatalf("normalizeGRPORouteResponse() error = %v", err)
	}
}

type fakeRouteChatProvider struct {
	models           []*types.Model
	chat             chat.Chat
	requestedModelID string
}

func (f *fakeRouteChatProvider) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	for _, model := range f.models {
		if model.ID == id {
			return model, nil
		}
	}
	return nil, nil
}

func (f *fakeRouteChatProvider) ListModels(context.Context) ([]*types.Model, error) {
	return f.models, nil
}

func (f *fakeRouteChatProvider) GetChatModel(_ context.Context, id string) (chat.Chat, error) {
	f.requestedModelID = id
	return f.chat, nil
}

type fakeRouteChat struct {
	messages []chat.Message
	options  *chat.ChatOptions
	response *types.ChatResponse
}

func (f *fakeRouteChat) Chat(_ context.Context, messages []chat.Message, options *chat.ChatOptions) (*types.ChatResponse, error) {
	f.messages = messages
	f.options = options
	return f.response, nil
}

func (f *fakeRouteChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	return nil, nil
}

func (f *fakeRouteChat) GetModelName() string { return "fake" }
func (f *fakeRouteChat) GetModelID() string   { return "fake" }
