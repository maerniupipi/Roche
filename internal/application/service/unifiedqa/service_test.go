package unifiedqa

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestUnifiedQAServiceSummaryModelResolution(t *testing.T) {
	provider := &fakeRouteChatProvider{models: []*types.Model{{
		ID: "configured-summary", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive,
	}}}
	service := &UnifiedQAService{
		configuredSummaryModelID: "configured-summary",
		answers:                  &AnswerGenerator{models: provider},
	}
	if got := service.resolveSummaryModelID(context.Background(), &types.QARequest{SummaryModelID: "request-summary"}); got != "request-summary" {
		t.Fatalf("request summary model = %q", got)
	}
	if got := service.resolveSummaryModelID(context.Background(), &types.QARequest{}); got != "configured-summary" {
		t.Fatalf("configured summary model = %q", got)
	}
	service.configuredSummaryModelID = "missing-summary"
	if got := service.resolveSummaryModelID(context.Background(), &types.QARequest{}); got != "" {
		t.Fatalf("missing configured summary model = %q, want database default selection", got)
	}
}

func TestBuildDomainFailureDetailRecordsReviewValidationError(t *testing.T) {
	detail := buildDomainFailureDetail("compliance", "compliance", DomainExecutionResult{
		Candidates:   []EvidenceCandidate{{OpaqueID: "e_1"}},
		ToolCalls:    1,
		ReviewCalls:  1,
		ModelCallIDs: []string{"review-1"},
	}, fmt.Errorf("initial evidence review: validate evidence review output: fact 0 quote is not present in its cited evidence"))
	if detail["stage"] != "evidence_review" || detail["code"] != "DOMAIN_EVIDENCE_REVIEW_OUTPUT_INVALID" ||
		detail["candidate_count"] != 1 || !strings.Contains(detail["error"].(string), "fact 0 quote") {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestUnifiedQAServiceRunsTwoAgentsInParallelAndStreamsAnswer(t *testing.T) {
	catalog := mustTestCatalog(t)
	routeModel := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"Can an HCP meal be reimbursed?","intent":"both","entities":{},
  "tasks":[
    {"agent_id":"finance","goal":"finance facts","search_queries":["meal reimbursement"]},
    {"agent_id":"compliance","goal":"compliance facts","search_queries":["HCP meal policy"]}
  ]
}`}}
	searcher := &concurrentHybridSearcher{}
	retrieval := NewRetrievalAdapter(searcher, RetrievalSettings{MatchCount: 5, RerankTopK: 5},
		&fakeKnowledgeDomainLookup{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1}, 2: {ID: 2}}})
	reviewModel := &concurrentReviewModel{}
	reviewer := NewDomainEvidenceReviewer(reviewModel, func(string) string { return "prompt" })
	domainExecutor := NewDomainAgentExecutor(catalog, retrieval, reviewer, NewEvidenceRecoveryExecutor(retrieval, 5))
	answerStream := make(chan types.StreamResponse, 1)
	answerStream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: `Combined answer: meal reimbursement evidence <fact id="fact-1" /> HCP meal policy evidence <fact id="fact-2" />`, Done: true}
	close(answerStream)
	answerChat := &fakeAnswerChat{stream: answerStream}
	answerProvider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   answerChat,
	}
	store := &serviceRunStore{}
	nodeObserver := &serviceNodeObserver{}
	service := &UnifiedQAService{
		runRepository:            store,
		configuredSummaryModelID: "missing-summary",
		scopeResolver: NewAuthorizedKBResolver(&fakeKnowledgeBaseLister{kbs: []*types.KnowledgeBase{
			{ID: "kb-finance", Name: "Finance KB", KnowledgeDomainID: 1},
			{ID: "kb-compliance", Name: "Compliance KB", KnowledgeDomainID: 2},
		}}, &fakeKnowledgeDomainBatchResolver{domains: map[uint64]*types.KnowledgeDomain{
			1: {ID: 1, Name: "财务部门"}, 2: {ID: 2, Name: "合规部门"},
		}}),
		catalog:      catalog,
		nodeRunner:   NewNodeRunner(nodeObserver),
		router:       NewMasterAgentRouter(routeModel, catalog, "route prompt"),
		domainAgents: domainExecutor,
		aggregator:   NewObservationAggregator(),
		answers:      NewAnswerGenerator(answerProvider, func(string) string { return "answer prompt" }),
		now:          testClock(),
		newID:        sequentialIDs(),
	}
	bus := event.NewEventBus()
	var answer string
	var captureMu sync.Mutex
	var thoughtEvents []event.Event
	var questionEvents []event.Event
	var knowledgeSearchEvents []event.Event
	var eventOrder []string
	var answerDoneSeen bool
	bus.On(event.EventQuestionUnderstood, func(_ context.Context, evt event.Event) error {
		captureMu.Lock()
		defer captureMu.Unlock()
		questionEvents = append(questionEvents, evt)
		eventOrder = append(eventOrder, "question_understood")
		return nil
	})
	bus.On(event.EventKnowledgeSearch, func(_ context.Context, evt event.Event) error {
		captureMu.Lock()
		defer captureMu.Unlock()
		knowledgeSearchEvents = append(knowledgeSearchEvents, evt)
		eventOrder = append(eventOrder, "knowledge_search")
		return nil
	})
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		captureMu.Lock()
		thoughtEvents = append(thoughtEvents, evt)
		if evt.Data.(event.AgentThoughtData).Done {
			eventOrder = append(eventOrder, "thinking_done")
		}
		captureMu.Unlock()
		return nil
	})
	bus.On(event.EventAgentReferences, func(_ context.Context, _ event.Event) error {
		captureMu.Lock()
		defer captureMu.Unlock()
		eventOrder = append(eventOrder, "references")
		return nil
	})
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		captureMu.Lock()
		data := evt.Data.(event.AgentFinalAnswerData)
		answer += data.Content
		if !data.Done {
			eventOrder = append(eventOrder, "answer")
			captureMu.Unlock()
			return nil
		}
		eventOrder = append(eventOrder, "answer_done")
		answerDoneSeen = true
		captureMu.Unlock()
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, _ event.Event) error {
		captureMu.Lock()
		defer captureMu.Unlock()
		eventOrder = append(eventOrder, "complete")
		return nil
	})

	err := service.Execute(context.Background(), &types.QARequest{
		Session: &types.Session{ID: "session"}, Query: "Can this be reimbursed?",
	}, bus)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !answerDoneSeen {
		t.Fatal("final answer did not emit done=true")
	}
	if err := bus.Emit(context.Background(), event.Event{Type: event.EventAgentComplete}); err != nil {
		t.Fatalf("emit complete: %v", err)
	}
	if !strings.Contains(answer, "Combined answer: meal reimbursement evidence") ||
		!strings.Contains(answer, `<kb doc="Policy" chunk_id="1" />`) ||
		!strings.Contains(answer, `<kb doc="Policy" chunk_id="2" />`) ||
		!strings.Contains(answer, renderAnswerDisclaimer(answerLanguageEnglish)) ||
		routeModel.calls != 1 || reviewModel.calls != 2 || searcher.calls != 2 {
		t.Fatalf("answer=%q route=%d review=%d search=%d", answer, routeModel.calls, reviewModel.calls, searcher.calls)
	}
	if store.finish.Status != types.QARunStatusCompleted {
		t.Fatalf("run finish = %+v", store.finish)
	}
	if store.finish.Metrics["model_calls"] != 4 {
		t.Fatalf("model_calls = %v, want 4", store.finish.Metrics["model_calls"])
	}
	if nodeObserver.count() != 4 {
		t.Fatalf("node count = %d, want route + 2 agents + answer", nodeObserver.count())
	}
	if len(questionEvents) != 1 || len(knowledgeSearchEvents) != 1 {
		t.Fatalf("milestones question=%d search=%d", len(questionEvents), len(knowledgeSearchEvents))
	}
	question := questionEvents[0].Data.(event.AgentThoughtData)
	search := knowledgeSearchEvents[0].Data.(event.AgentThoughtData)
	if question.Content != "已完成问题理解" || !question.Done || question.Stage != "question_understood" {
		t.Fatalf("question milestone = %+v", question)
	}
	if search.Content != "检索知识库" || !search.Done || search.Stage != "knowledge_search" {
		t.Fatalf("knowledge search milestone = %+v", search)
	}
	if len(thoughtEvents) < 2 {
		t.Fatalf("thought event count = %d, want streamed progress plus done", len(thoughtEvents))
	}
	var progressText strings.Builder
	started := make(map[string]bool)
	doneCount := 0
	for _, evt := range thoughtEvents {
		data := evt.Data.(event.AgentThoughtData)
		if data.RunID == "" || data.StepID == "" || data.Stage == "" {
			t.Fatalf("thought event is not structured: %+v", data)
		}
		if data.Content != "" {
			started[data.StepID] = true
		}
		if data.Done {
			doneCount++
			if data.Stage != "thinking" || data.Status != "completed" {
				t.Fatalf("unexpected thinking terminal event: %+v", data)
			}
		} else if data.Status == "completed" && !started[data.StepID] {
			t.Fatalf("thought step completed before start: %+v", data)
		}
		progressText.WriteString(data.Content)
	}
	if !thoughtEvents[len(thoughtEvents)-1].Data.(event.AgentThoughtData).Done {
		t.Fatal("final thought event is not done")
	}
	if doneCount != 1 {
		t.Fatalf("thinking done event count = %d, want 1", doneCount)
	}
	for _, expected := range []string{
		"已识别需要由财务子智能体和合规子智能体分别核对授权知识库",
		"财务子智能体正在检索授权知识库",
		"合规子智能体正在检索授权知识库",
		"财务子智能体正在复核候选证据",
		"合规子智能体正在复核候选证据",
		"财务子智能体已完成证据复核",
		"合规子智能体已完成证据复核",
		"正在汇总已验证的领域证据",
		"已从2条候选证据中确认2条可引用事实",
		"正在根据2条已验证事实组织最终回答并校验引用",
	} {
		if !strings.Contains(progressText.String(), expected) {
			t.Fatalf("progress %q does not contain %q", progressText.String(), expected)
		}
	}
	firstAnswer := slices.Index(eventOrder, "answer")
	thinkingDone := slices.Index(eventOrder, "thinking_done")
	references := slices.Index(eventOrder, "references")
	answerDone := slices.Index(eventOrder, "answer_done")
	complete := slices.Index(eventOrder, "complete")
	if thinkingDone < 1 || references <= thinkingDone || firstAnswer <= references || answerDone <= firstAnswer || complete <= answerDone || eventOrder[len(eventOrder)-1] != "complete" {
		t.Fatalf("event order = %v, want thinking_done, references, answer chunks, answer_done, complete", eventOrder)
	}
	if eventOrder[0] != "question_understood" || eventOrder[1] != "knowledge_search" {
		t.Fatalf("event order = %v, want question then knowledge search", eventOrder)
	}
}

func TestUnifiedQAServiceAppendsTravelExpenseFallbackAfterDoAMatch(t *testing.T) {
	catalog := mustTestTopicCatalog(t)
	routeModel := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"DoA 和 T&E 分别有什么要求？","intent":"finance_policy","outcome":"routed","entities":{},
  "tasks":[{"agent_id":"finance","topic_ids":["doa","travel_expense"],"goal":"分别核实 DoA 和 T&E","search_queries":["DoA 和 T&E 要求"]}]
}`}}
	searcher := &topicHybridSearcher{}
	retrieval := NewRetrievalAdapter(searcher, RetrievalSettings{MatchCount: 5, RerankTopK: 5},
		&fakeKnowledgeDomainLookup{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1}}})
	reviewer := NewDomainEvidenceReviewer(&topicReviewModel{}, func(string) string { return "prompt" })
	answerStream := make(chan types.StreamResponse, 1)
	answerStream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: `DoA 事实 <fact id="fact-1" />`, Done: true}
	close(answerStream)
	answerProvider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   &fakeAnswerChat{stream: answerStream},
	}
	store := &serviceRunStore{}
	service := &UnifiedQAService{
		runRepository: store,
		scopeResolver: NewAuthorizedKBResolver(&fakeKnowledgeBaseLister{kbs: []*types.KnowledgeBase{
			{ID: "kb-doa", Name: "RDSL_DOA", KnowledgeDomainID: 1},
			{ID: "kb-te", Name: "China T&E", KnowledgeDomainID: 1},
		}}, &fakeKnowledgeDomainBatchResolver{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1, Name: "财务"}}}),
		catalog: catalog, nodeRunner: NewNodeRunner(&serviceNodeObserver{}),
		router:       NewMasterAgentRouter(routeModel, catalog, "route prompt"),
		domainAgents: NewDomainAgentExecutor(catalog, retrieval, reviewer, NewEvidenceRecoveryExecutor(retrieval, 5)),
		aggregator:   NewObservationAggregator(),
		answers:      NewAnswerGenerator(answerProvider, func(string) string { return "answer prompt" }),
		now:          testClock(), newID: sequentialIDs(),
	}
	bus := event.NewEventBus()
	var answer string
	var anyFallback bool
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		answer += data.Content
		anyFallback = anyFallback || data.IsFallback
		return nil
	})

	if err := service.Execute(context.Background(), &types.QARequest{
		Session: &types.Session{ID: "session"}, Query: "DoA 和 T&E 分别有什么要求？", SummaryModelID: "chat",
	}, bus); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if anyFallback || !strings.Contains(answer, "DoA 事实") || !strings.Contains(answer, "未能在差旅报销政策中找到") ||
		!strings.Contains(answer, "以上信息基于现行授权手册（DoA）") || strings.Contains(answer, "免责声明：本回答") {
		t.Fatalf("answer=%q anyFallback=%v", answer, anyFallback)
	}
	if searcher.calls != 2 || store.finish.Metrics["coverage"] != CoveragePartial {
		t.Fatalf("search calls=%d metrics=%+v", searcher.calls, store.finish.Metrics)
	}
	if got, ok := store.finish.Metrics["fallback_topics"].([]string); !ok || !reflect.DeepEqual(got, []string{"travel_expense"}) {
		t.Fatalf("fallback_topics=%#v", store.finish.Metrics["fallback_topics"])
	}
	if got, want := store.finish.Metrics["response_policy_codes"], []string{
		"topic.travel_expense.no_match",
		"topic.doa.answer_disclaimer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("response_policy_codes=%#v, want %v", got, want)
	}
}

func TestUnifiedQAServiceMarksCompletelyUnrecognizedTermAsFallback(t *testing.T) {
	catalog := mustTestTopicCatalog(t)
	terms, err := NewTerminologyCatalog(&config.UnifiedQATermsConfig{Version: "test-v1", AcceptedTerms: []string{"DoA", "T&E"}})
	if err != nil {
		t.Fatalf("NewTerminologyCatalog() error = %v", err)
	}
	store := &serviceRunStore{}
	retrieval := NewRetrievalAdapter(&topicHybridSearcher{}, RetrievalSettings{},
		&fakeKnowledgeDomainLookup{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1}}})
	service := &UnifiedQAService{
		runRepository: store,
		scopeResolver: NewAuthorizedKBResolver(&fakeKnowledgeBaseLister{kbs: []*types.KnowledgeBase{
			{ID: "kb-doa", Name: "DoA", KnowledgeDomainID: 1},
		}}, &fakeKnowledgeDomainBatchResolver{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1, Name: "财务"}}}),
		catalog: catalog, nodeRunner: NewNodeRunner(&serviceNodeObserver{}),
		router: NewMasterAgentRouter(&fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"ABC 政策是什么？","intent":"none","outcome":"out_of_coverage","entities":{},"tasks":[]
}`}}, catalog, "route prompt"),
		domainAgents: NewDomainAgentExecutor(catalog, retrieval, NewDomainEvidenceReviewer(&topicReviewModel{}, func(string) string { return "prompt" }), nil),
		aggregator:   NewObservationAggregator(), answers: NewAnswerGenerator(&fakeRouteChatProvider{}, func(string) string { return "prompt" }),
		terminology: terms, now: testClock(), newID: sequentialIDs(),
	}
	bus := event.NewEventBus()
	var final event.AgentFinalAnswerData
	var finalContent strings.Builder
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		final = evt.Data.(event.AgentFinalAnswerData)
		finalContent.WriteString(final.Content)
		return nil
	})
	if err := service.Execute(context.Background(), &types.QARequest{Session: &types.Session{ID: "session"}, Query: "ABC 政策是什么？"}, bus); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !final.IsFallback || !final.Done || !strings.Contains(finalContent.String(), "未识别术语：ABC") {
		t.Fatalf("final = %+v content=%q", final, finalContent.String())
	}
	if got, want := store.finish.Metrics["response_policy_codes"], []string{"global.term_unrecognized"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("response_policy_codes=%#v, want %v", got, want)
	}
}

func TestUnifiedQAServiceReturnsSafeHighConfidenceFAQAfterOneLightweightReview(t *testing.T) {
	catalog := mustTestCatalog(t)
	routeModel := &fakeRouteModel{response: RouteModelResponse{Content: `{
  "standalone_query":"如何重置密码？","intent":"finance","entities":{},
  "tasks":[{"agent_id":"finance","goal":"查找账户帮助","search_queries":["如何重置密码？"]}]
}`}}
	searcher := &fakeHybridSearcher{results: map[string][]*types.SearchResult{
		"如何重置密码？": {faqSearchResult(t, "使用登录页的重置密码功能。")},
	}}
	reranker := &fakeEvidenceReranker{results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.95}}}
	retrieval := NewRetrievalAdapter(searcher, RetrievalSettings{MatchCount: 5, RerankTopK: 5, RerankThreshold: 0.3},
		&fakeKnowledgeDomainLookup{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1}}})
	fastReviewer := &fakeFAQFastPathReviewer{result: FAQFastPathReviewResult{
		Eligible: true, Reason: "safe", ModelCallID: "faq-review",
	}}
	store := &serviceRunStore{}
	nodeObserver := &serviceNodeObserver{}
	service := &UnifiedQAService{
		runRepository: store,
		scopeResolver: NewAuthorizedKBResolver(&fakeKnowledgeBaseLister{kbs: []*types.KnowledgeBase{{ID: "kb", Name: "KB", KnowledgeDomainID: 1}}}, &fakeKnowledgeDomainBatchResolver{domains: map[uint64]*types.KnowledgeDomain{1: {ID: 1, Name: "财务部门"}}}),
		catalog:       catalog,
		nodeRunner:    NewNodeRunner(nodeObserver),
		router:        NewMasterAgentRouter(routeModel, catalog, "route prompt"),
		rerankModels: NewRerankModelResolver(&fakeRerankModelProvider{
			models:   []*types.Model{{ID: "rerank", Type: types.ModelTypeRerank, Status: types.ModelStatusActive, IsDefault: true}},
			reranker: reranker,
		}),
		retrieval:    retrieval,
		faqFastPath:  fastReviewer,
		domainAgents: &DomainAgentExecutor{},
		aggregator:   NewObservationAggregator(),
		answers:      &AnswerGenerator{},
		now:          testClock(),
		newID:        sequentialIDs(),
	}
	bus := event.NewEventBus()
	var answer string
	var references int
	var milestones []event.EventType
	bus.On(event.EventQuestionUnderstood, func(_ context.Context, evt event.Event) error {
		milestones = append(milestones, evt.Type)
		return nil
	})
	bus.On(event.EventKnowledgeSearch, func(_ context.Context, evt event.Event) error {
		milestones = append(milestones, evt.Type)
		return nil
	})
	bus.On(event.EventAgentReferences, func(_ context.Context, evt event.Event) error {
		references = len(evt.Data.(event.AgentReferencesData).References.(types.References))
		return nil
	})
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		answer += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})

	err := service.Execute(context.Background(), &types.QARequest{
		Session: &types.Session{ID: "session"}, Query: "如何重置密码？", SummaryModelID: "",
	}, bus)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(answer, "使用登录页的重置密码功能。") || !strings.Contains(answer, `<kb doc="账户帮助" chunk_id="1" />`) || references != 1 {
		t.Fatalf("answer=%q references=%d", answer, references)
	}
	if fastReviewer.calls != 1 || len(searcher.calls) != 1 || reranker.calls != 1 {
		t.Fatalf("fast review=%d search=%d rerank=%d", fastReviewer.calls, len(searcher.calls), reranker.calls)
	}
	if store.finish.Status != types.QARunStatusCompleted || store.finish.Metrics["faq_fast_path"] != true {
		t.Fatalf("run finish = %+v", store.finish)
	}
	if store.finish.Metrics["model_calls"] != 2 || store.finish.Metrics["tool_calls"] != 1 {
		t.Fatalf("metrics = %+v", store.finish.Metrics)
	}
	if nodeObserver.count() != 3 {
		t.Fatalf("node count = %d, want route + FAQ search + FAQ validation", nodeObserver.count())
	}
	if len(milestones) != 2 || milestones[0] != event.EventQuestionUnderstood || milestones[1] != event.EventKnowledgeSearch {
		t.Fatalf("milestones = %v", milestones)
	}
}

type concurrentHybridSearcher struct {
	mu    sync.Mutex
	calls int
}

type topicHybridSearcher struct {
	mu    sync.Mutex
	calls int
}

func (s *topicHybridSearcher) HybridSearch(_ context.Context, knowledgeBaseID string, _ types.SearchParams) ([]*types.SearchResult, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if knowledgeBaseID != "kb-doa" {
		return nil, nil
	}
	return []*types.SearchResult{{
		ID: "doa-chunk", KnowledgeBaseID: knowledgeBaseID, KnowledgeID: "doa-doc",
		KnowledgeTitle: "DoA Policy", Content: "DoA evidence", Score: 0.9,
	}}, nil
}

type topicReviewModel struct{}

func (*topicReviewModel) GenerateReview(_ context.Context, request ReviewModelRequest) (ReviewModelResponse, error) {
	observation := AgentObservation{AgentID: request.Profile.ID, Status: EvidenceStatusInsufficient}
	if len(request.Candidates) > 0 {
		observation.Status = EvidenceStatusSufficient
		observation.Facts = []ObservedFact{{
			Statement: "DoA 事实", Quote: request.Candidates[0].Content,
			Citations: []EvidenceCitation{{OpaqueID: request.Candidates[0].OpaqueID}},
		}}
	}
	data, _ := json.Marshal(observation)
	return ReviewModelResponse{Content: string(data), ModelCallID: singleTopicID(request.Task) + "-review"}, nil
}

func (s *concurrentHybridSearcher) HybridSearch(_ context.Context, knowledgeBaseID string, params types.SearchParams) ([]*types.SearchResult, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	return []*types.SearchResult{{
		ID: "chunk-" + params.QueryText, KnowledgeBaseID: knowledgeBaseID, KnowledgeID: "doc-" + params.QueryText,
		KnowledgeTitle: "Policy", Content: params.QueryText + " evidence", Score: 0.8 + float64(call)/100,
	}}, nil
}

type concurrentReviewModel struct {
	mu    sync.Mutex
	calls int
}

func (m *concurrentReviewModel) GenerateReview(_ context.Context, request ReviewModelRequest) (ReviewModelResponse, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	observation := AgentObservation{
		AgentID: request.Profile.ID, Status: EvidenceStatusSufficient,
		Facts: []ObservedFact{{
			Statement: request.Profile.ID + " fact", Quote: request.Candidates[0].Content,
			Citations: []EvidenceCitation{{OpaqueID: request.Candidates[0].OpaqueID}},
		}},
	}
	data, _ := json.Marshal(observation)
	return ReviewModelResponse{Content: string(data), ModelCallID: request.Profile.ID + "-review"}, nil
}

type serviceRunStore struct {
	mu     sync.Mutex
	finish types.QARunFinishUpdate
}

func (s *serviceRunStore) CreateRun(context.Context, *types.QAExecutionRun) error { return nil }
func (s *serviceRunStore) GetRun(context.Context, string) (*types.QAExecutionRun, error) {
	return nil, nil
}
func (s *serviceRunStore) FinishRun(_ context.Context, _ string, update types.QARunFinishUpdate) error {
	s.mu.Lock()
	s.finish = update
	s.mu.Unlock()
	return nil
}

type serviceNodeObserver struct {
	mu    sync.Mutex
	specs []NodeSpec
}

func (o *serviceNodeObserver) Start(ctx context.Context, spec NodeSpec) (context.Context, NodeObservation, error) {
	o.mu.Lock()
	o.specs = append(o.specs, spec)
	o.mu.Unlock()
	return ctx, serviceNodeObservation{}, nil
}

func (o *serviceNodeObserver) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.specs)
}

type serviceNodeObservation struct{}

func (serviceNodeObservation) ID() string { return "" }
func (serviceNodeObservation) Finish(types.JSONMap, map[string]any, error) error {
	return nil
}

func testClock() func() time.Time {
	var mu sync.Mutex
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Millisecond)
		return now
	}
}

func sequentialIDs() func() string {
	var mu sync.Mutex
	index := 0
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		index++
		return fmt.Sprintf("id-%d", index)
	}
}

var _ chat.Chat = (*fakeAnswerChat)(nil)
