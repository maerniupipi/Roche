package unifiedqa

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMasterAgentRouterAcceptsFinanceRoute(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{ModelCallID: "route-call-1", Content: `{
  "standalone_query": "What is the reimbursement limit?",
  "intent": "finance_policy",
  "entities": {"topic": "reimbursement"},
  "tasks": [{
    "agent_id": "finance",
    "goal": "Find the applicable reimbursement limit",
    "search_queries": ["reimbursement limit"],
    "exact_terms": ["limit"],
    "document_types": ["policy"],
    "tool_intent": "knowledge_search"
  }]
}`}}
	router := NewMasterAgentRouter(model, mustTestCatalog(t), "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "limit?"})
	if decision.Degraded {
		t.Fatalf("decision degraded: %+v", decision)
	}
	if decision.ModelCallID != "route-call-1" || decision.Plan.StandaloneQuery != "What is the reimbursement limit?" {
		t.Fatalf("decision = %+v", decision)
	}
	if got, want := routeAgentIDs(decision.Plan), []string{FinanceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("route agents = %v, want %v", got, want)
	}
	if model.calls != 1 {
		t.Fatalf("model calls = %d, want 1", model.calls)
	}
	if model.request.MaxSelectedAgents != 2 {
		t.Fatalf("MaxSelectedAgents = %d, want 2", model.request.MaxSelectedAgents)
	}
}

func TestMasterAgentRouterAcceptsOrderedTwoAgentRoute(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query": "Can an HCP meal be reimbursed?",
  "intent": "finance_and_compliance",
  "entities": {},
  "tasks": [
    {"agent_id":"compliance","goal":"Find HCP meal restrictions","search_queries":["HCP meal policy"]},
    {"agent_id":"finance","goal":"Find meal reimbursement rules","search_queries":["meal reimbursement"]}
  ]
}`}}
	router := NewMasterAgentRouter(model, mustTestCatalog(t), "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "Can this be paid?"})
	if decision.Degraded {
		t.Fatalf("decision degraded: %+v", decision)
	}
	if got, want := routeAgentIDs(decision.Plan), []string{FinanceAgentID, ComplianceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("route agents = %v, want stable order %v", got, want)
	}
}

func TestMasterAgentRouterForwardsDedicatedGRPOPrompt(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"Q","intent":"none","outcome":"out_of_service","entities":{},"tasks":[]
}`}}
	router := NewMasterAgentRouter(model, mustTestCatalog(t), "default prompt", "grpo prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "Q"})
	if decision.Degraded || model.request.SystemPrompt != "default prompt" || model.request.GRPOSystemPrompt != "grpo prompt" {
		t.Fatalf("decision=%+v request=%+v", decision, model.request)
	}
}

func TestMasterAgentRouterKeepsDoAAndTravelExpenseInsideOneFinanceTask(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"DoA 和 T&E 分别有什么要求？","intent":"finance_policy","outcome":"routed","entities":{},
  "tasks":[{"agent_id":"finance","topic_ids":["travel_expense","doa"],"goal":"分别核实两个主题","search_queries":["DoA 和 T&E 要求"]}]
}`}}
	decision := NewMasterAgentRouter(model, mustTestTopicCatalog(t), "route prompt").Route(context.Background(), RouteRequest{OriginalQuery: "DoA 和 T&E"})
	if decision.Degraded || len(decision.Plan.Tasks) != 1 || !reflect.DeepEqual(decision.Plan.Tasks[0].TopicIDs, []string{"doa", "travel_expense"}) {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestMasterAgentRouterAcceptsConfiguredThirdAgent(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"员工可以休几天年假吗？",
  "intent":"hr_policy",
  "entities":{},
  "tasks":[{"agent_id":"hr","goal":"查询年假政策","search_queries":["员工年假政策"]}]
}`}}
	router := NewMasterAgentRouter(model, mustTestThreeAgentCatalog(t), "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "年假几天？"})
	if decision.Degraded {
		t.Fatalf("decision degraded: %+v", decision)
	}
	if got, want := routeAgentIDs(decision.Plan), []string{"hr"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("route agents = %v, want %v", got, want)
	}
	if model.request.MaxSelectedAgents != 3 {
		t.Fatalf("MaxSelectedAgents = %d, want 3", model.request.MaxSelectedAgents)
	}
}

func TestMasterAgentRouterMarksUnmatchedModelFailureAsRouteFailed(t *testing.T) {
	cfg := testThreeAgentCatalogConfig()
	cfg.FallbackAgentIDs = []string{"hr", "finance"}
	catalog, err := NewAgentCatalog(cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	router := NewMasterAgentRouter(&fakeRouteModel{err: errors.New("timeout")}, catalog, "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "question"})
	if len(decision.Plan.Tasks) != 0 || decision.Plan.Outcome != RouteOutcomeFailed || !decision.Degraded {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestMasterAgentRouterFallbackUsesOnlyKeywordMatchedDomains(t *testing.T) {
	cfg := testAgentCatalogConfig()
	cfg.Agents[0].RouteKeywords = []string{"doa", "审批权限"}
	cfg.Agents[1].RouteKeywords = []string{"compliance", "hcp"}
	catalog, err := NewAgentCatalog(cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	router := NewMasterAgentRouter(&fakeRouteModel{err: errors.New("timeout")}, catalog, "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "HCP compliance policy"})
	if got, want := routeAgentIDs(decision.Plan), []string{ComplianceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback agents = %v, want %v", got, want)
	}
}

func TestMasterAgentRouterClassifiesFailedRoutesConservatively(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		outcome  string
		degraded bool
	}{
		{name: "explicitly external", query: "What is the weather today?", outcome: RouteOutcomeOutOfService},
		{name: "internal policy without matching domain", query: "What is the company leave policy?", outcome: RouteOutcomeOutOfCoverage, degraded: true},
		{name: "ambiguous", query: "Please help me understand this", outcome: RouteOutcomeFailed, degraded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := NewMasterAgentRouter(&fakeRouteModel{err: errors.New("timeout")}, mustTestCatalog(t), "route prompt").Route(
				context.Background(), RouteRequest{OriginalQuery: tt.query})
			if decision.Degraded != tt.degraded || decision.Plan.Outcome != tt.outcome || len(decision.Plan.Tasks) != 0 {
				t.Fatalf("decision = %+v, want outcome %q", decision, tt.outcome)
			}
		})
	}
}

func TestMasterAgentRouterDistinguishesInvalidGRPOOutputFromModelFailure(t *testing.T) {
	router := NewMasterAgentRouter(&fakeRouteModel{err: ErrRouteInvalidModelOutput}, mustTestCatalog(t), "route prompt")
	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "question"})
	if decision.ErrorCode != ErrorCodeRouteInvalidOutput {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestMasterAgentRouterRejectsInvalidOutputWithoutExpandingScope(t *testing.T) {
	tests := []struct {
		name     string
		response string
		err      error
		wantCode string
	}{
		{name: "model failure", err: errors.New("timeout"), wantCode: ErrorCodeRouteModelFailed},
		{name: "malformed JSON", response: `not-json`, wantCode: ErrorCodeRouteInvalidOutput},
		{name: "unknown field", response: `{"standalone_query":"Q","intent":"x","tasks":[],"knowledge_base_ids":["kb-1"]}`, wantCode: ErrorCodeRouteInvalidOutput},
		{name: "unknown agent", response: `{"standalone_query":"Q","intent":"x","entities":{},"tasks":[{"agent_id":"generic","goal":"g","search_queries":["q"]}]}`, wantCode: ErrorCodeRouteInvalidOutput},
		{name: "duplicate agent", response: `{"standalone_query":"Q","intent":"x","entities":{},"tasks":[{"agent_id":"finance","goal":"g","search_queries":["q"]},{"agent_id":"finance","goal":"g2","search_queries":["q2"]}]}`, wantCode: ErrorCodeRouteInvalidOutput},
		{name: "unapproved tool", response: `{"standalone_query":"Q","intent":"x","entities":{},"tasks":[{"agent_id":"finance","goal":"g","search_queries":["q"],"tool_intent":"web_search"}]}`, wantCode: ErrorCodeRouteInvalidOutput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &fakeRouteModel{response: RouteModelResponse{Content: tt.response}, err: tt.err}
			router := NewMasterAgentRouter(model, mustTestCatalog(t), "route prompt")
			decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "Original question"})
			if !decision.Degraded || decision.ErrorCode != tt.wantCode {
				t.Fatalf("decision = %+v", decision)
			}
			if decision.Plan.StandaloneQuery != "Original question" {
				t.Fatalf("standalone query = %q", decision.Plan.StandaloneQuery)
			}
			if len(decision.Plan.Tasks) != 0 || decision.Plan.Outcome != RouteOutcomeFailed {
				t.Fatalf("fallback plan = %+v", decision.Plan)
			}
			if model.calls != 1 {
				t.Fatalf("model calls = %d, want 1", model.calls)
			}
		})
	}
}

func TestMasterAgentRouterAcceptsUnroutedOutcomes(t *testing.T) {
	for _, outcome := range []string{RouteOutcomeOutOfService, RouteOutcomeOutOfCoverage} {
		t.Run(outcome, func(t *testing.T) {
			model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"Q","intent":"none","outcome":"` + outcome + `","entities":{},"tasks":[]
}`}}
			decision := NewMasterAgentRouter(model, mustTestCatalog(t), "route prompt").Route(context.Background(), RouteRequest{OriginalQuery: "Q"})
			if decision.Degraded || decision.Plan.Outcome != outcome || len(decision.Plan.Tasks) != 0 {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}

func TestMasterAgentRouterOverridesWrongDomainForExplicitExternalQuery(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"明天上海天气怎么样？",
  "intent":"finance",
  "outcome":"routed",
  "entities":{},
  "tasks":[{"agent_id":"finance","goal":"weather","search_queries":["weather"]}]
}`}}
	decision := NewMasterAgentRouter(model, mustTestCatalog(t), "route prompt").Route(
		context.Background(), RouteRequest{OriginalQuery: "明天上海天气怎么样？"},
	)
	if decision.Degraded || decision.Plan.Outcome != RouteOutcomeOutOfService || len(decision.Plan.Tasks) != 0 {
		t.Fatalf("decision = %+v", decision)
	}
	if model.calls != 0 {
		t.Fatalf("model calls = %d, want 0", model.calls)
	}
}

func TestMasterAgentRouterRecoversKeywordMatchedDomainFromEmptyModelRoute(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"向医疗卫生专业人士提供礼品时有哪些合规限制？",
  "intent":"none",
  "outcome":"out_of_coverage",
  "entities":{},
  "tasks":[]
}`}}
	catalog := mustTestTopicCatalog(t)
	complianceIndex := catalog.byID[ComplianceAgentID]
	catalog.agents[complianceIndex].RouteKeywords = []string{"合规", "医疗卫生专业人士", "hcp"}
	router := NewMasterAgentRouter(model, catalog, "route prompt")

	decision := router.Route(context.Background(), RouteRequest{
		OriginalQuery: "向医疗卫生专业人士提供礼品时有哪些合规限制？",
	})
	if !decision.Degraded || decision.ErrorCode != "" || decision.ModelCallID != "" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.Plan.Outcome != RouteOutcomeRouted {
		t.Fatalf("outcome = %q", decision.Plan.Outcome)
	}
	if got, want := routeAgentIDs(decision.Plan), []string{ComplianceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("route agents = %v, want %v", got, want)
	}
	if len(decision.Plan.Tasks) != 1 || !reflect.DeepEqual(decision.Plan.Tasks[0].TopicIDs, []string{"compliance"}) {
		t.Fatalf("tasks = %+v", decision.Plan.Tasks)
	}
}

func TestMasterAgentRouterRemovesModelDomainWithoutKeywordSupport(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"HCP gift restrictions",
  "intent":"finance_and_compliance",
  "outcome":"routed",
  "entities":{},
  "tasks":[
    {"agent_id":"finance","topic_ids":["travel_expense"],"goal":"finance","search_queries":["gift expense"]},
    {"agent_id":"compliance","topic_ids":["compliance"],"goal":"compliance","search_queries":["HCP gift"]}
  ]
}`}}
	catalog := mustTestTopicCatalog(t)
	financeIndex := catalog.byID[FinanceAgentID]
	complianceIndex := catalog.byID[ComplianceAgentID]
	catalog.agents[financeIndex].RouteKeywords = []string{"reimbursement", "expense"}
	catalog.agents[complianceIndex].RouteKeywords = []string{"hcp", "compliance"}
	router := NewMasterAgentRouter(model, catalog, "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "HCP gift restrictions"})
	if !decision.Degraded || decision.Plan.Outcome != RouteOutcomeRouted {
		t.Fatalf("decision = %+v", decision)
	}
	if got, want := routeAgentIDs(decision.Plan), []string{ComplianceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("route agents = %v, want %v", got, want)
	}
	if len(decision.Plan.Tasks) != 1 || decision.Plan.Tasks[0].Goal != "compliance" {
		t.Fatalf("model task was not preserved: %+v", decision.Plan.Tasks)
	}
}

func TestMasterAgentRouterNarrowsFinanceTopicsFromExplicitTopicKeywords(t *testing.T) {
	model := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"procurement approval amount",
  "intent":"finance",
  "outcome":"routed",
  "entities":{},
  "tasks":[{
    "agent_id":"finance",
    "topic_ids":["doa","travel_expense"],
    "goal":"finance",
    "search_queries":["procurement approval amount"]
  }]
}`}}
	catalog := mustTestTopicCatalog(t)
	financeIndex := catalog.byID[FinanceAgentID]
	catalog.agents[financeIndex].RouteKeywords = []string{"procurement", "reimbursement"}
	for index := range catalog.agents[financeIndex].Topics {
		topic := &catalog.agents[financeIndex].Topics[index]
		switch topic.ID {
		case "doa":
			topic.RouteKeywords = []string{"procurement", "approval", "amount"}
		case "travel_expense":
			topic.RouteKeywords = []string{"reimbursement", "travel"}
		}
	}
	router := NewMasterAgentRouter(model, catalog, "route prompt")

	decision := router.Route(context.Background(), RouteRequest{OriginalQuery: "procurement approval amount"})
	if !decision.Degraded || len(decision.Plan.Tasks) != 1 {
		t.Fatalf("decision = %+v", decision)
	}
	if got, want := decision.Plan.Tasks[0].TopicIDs, []string{"doa"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("topic IDs = %v, want %v", got, want)
	}
}

func mustTestCatalog(t *testing.T) *AgentCatalog {
	t.Helper()
	catalog, err := NewAgentCatalog(testAgentCatalogConfig(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	return catalog
}

func mustTestThreeAgentCatalog(t *testing.T) *AgentCatalog {
	t.Helper()
	catalog, err := NewAgentCatalog(testThreeAgentCatalogConfig(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	return catalog
}

func routeAgentIDs(plan MasterRoutePlan) []string {
	ids := make([]string, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		ids = append(ids, task.AgentID)
	}
	return ids
}

type fakeRouteModel struct {
	calls    int
	request  RouteModelRequest
	response RouteModelResponse
	err      error
}

func (f *fakeRouteModel) GenerateRoute(_ context.Context, request RouteModelRequest) (RouteModelResponse, error) {
	f.calls++
	f.request = request
	return f.response, f.err
}
