package unifiedqa

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/types"
)

type answerExchangeRateService struct {
	rate *types.ExchangeRate
	err  error
}

func (s *answerExchangeRateService) GetExchangeRate(context.Context) (*types.ExchangeRate, error) {
	return s.rate, s.err
}

func (s *answerExchangeRateService) ConfigureExchangeRate(
	context.Context, types.ExchangeRateConfig, string,
) (*types.ExchangeRate, error) {
	return nil, errors.New("not implemented")
}

func TestAnswerGeneratorStreamsReferencesAndFinalAnswer(t *testing.T) {
	stream := make(chan types.StreamResponse, 2)
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "Answer ", Done: false}
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: `<fact id="fact-1" />`, Done: true}
	close(stream)
	chatModel := &fakeAnswerChat{stream: stream}
	provider := &fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: chatModel}
	generator := NewAnswerGenerator(provider, func(string) string { return "answer prompt" })
	bus := event.NewEventBus()
	var references int
	var chunks string
	var chunkCount int
	var doneCount int
	bus.On(event.EventAgentReferences, func(_ context.Context, evt event.Event) error {
		references = len(evt.Data.(event.AgentReferencesData).References.(types.References))
		return nil
	})
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		chunks += data.Content
		if data.Content != "" {
			chunkCount++
			if size := utf8.RuneCountInString(data.Content); size > verifiedAnswerChunkRunes {
				t.Fatalf("streamed answer chunk has %d runes, want at most %d: %q", size, verifiedAnswerChunkRunes, data.Content)
			}
		}
		if data.Done {
			doneCount++
		}
		return nil
	})

	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "Q", StandaloneQuery: "Q", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{Statement: "fact", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "Policy", Content: "fact"}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := `Answer <kb doc="Policy" chunk_id="1" />` + "\n\n" + renderAnswerDisclaimer(answerLanguageEnglish)
	if result.Answer != want || chunks != result.Answer || chunkCount < 2 || doneCount != 1 || references != 1 || result.CitationValidationFailed {
		t.Fatalf("result=%+v chunks=%q chunkCount=%d doneCount=%d references=%d", result, chunks, chunkCount, doneCount, references)
	}
}

func TestAnswerGeneratorKeepsPartialTopicAnswerNonFallbackAndAppendsTopicTail(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: `DoA 已命中 <fact id="fact-1" />`, Done: true}
	close(stream)
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   &fakeAnswerChat{stream: stream},
	}
	bus := event.NewEventBus()
	var emitted string
	var anyFallback bool
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		emitted += data.Content
		anyFallback = anyFallback || data.IsFallback
		return nil
	})

	result, err := NewAnswerGenerator(provider, func(string) string { return "answer prompt" }).Generate(context.Background(), FinalAnswerRequest{
		Question: "DoA 和 T&E 的要求是什么？", StandaloneQuery: "DoA 和 T&E 的要求是什么？", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoveragePartial, Facts: []ObservedFact{{Statement: "DoA 已命中", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "DoA", Content: "DoA 已命中"}},
		Policy: TopicAnswerPolicy{
			NoMatchNotices: []string{"T&E 未命中"}, SuccessDisclaimers: []string{"DoA 专属免责声明"},
		},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if anyFallback || emitted != result.Answer || !strings.Contains(result.Answer, "T&E 未命中") || !strings.Contains(result.Answer, "DoA 专属免责声明") || strings.Contains(result.Answer, "免责声明：本回答") {
		t.Fatalf("answer=%q emitted=%q anyFallback=%v", result.Answer, emitted, anyFallback)
	}
}

func TestAnswerGeneratorMarksAllTopicMissesAsFallback(t *testing.T) {
	bus := event.NewEventBus()
	var emitted event.AgentFinalAnswerData
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted = evt.Data.(event.AgentFinalAnswerData)
		return nil
	})
	result, err := NewAnswerGenerator(&fakeRouteChatProvider{}, func(string) string { return "answer prompt" }).Generate(context.Background(), FinalAnswerRequest{
		Question: "DoA 和 T&E", SessionID: "s", Aggregated: AggregatedObservation{Coverage: CoverageInsufficient},
		Policy: TopicAnswerPolicy{NoMatchNotices: []string{"DoA 未命中", "T&E 未命中"}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Answer != "DoA 未命中\n\nT&E 未命中" || !emitted.IsFallback || !emitted.Done {
		t.Fatalf("result=%+v emitted=%+v", result, emitted)
	}
}

func TestAnswerGeneratorBlocksUncitedSegmentAndStreamsValidSegment(t *testing.T) {
	stream := make(chan types.StreamResponse, 2)
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "Uncited claim.\n\n", Done: false}
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "Verified fact. <fact id=\"fact-1\" />", Done: true}
	close(stream)
	chatModel := &fakeAnswerChat{stream: stream}
	provider := &fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: chatModel}
	generator := NewAnswerGenerator(provider, func(string) string { return "answer prompt" })
	bus := event.NewEventBus()
	var emitted string
	var done bool
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		emitted += data.Content
		done = done || data.Done
		return nil
	})

	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "Q", StandaloneQuery: "Q", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{Statement: "Verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "Policy", Content: "Verified fact."}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !result.CitationValidationFailed || strings.Contains(emitted, "Uncited claim") || emitted != result.Answer || !done || !strings.Contains(emitted, "Disclaimer:") {
		t.Fatalf("result=%+v emitted=%q done=%v", result, emitted, done)
	}
}

func TestAnswerGeneratorUsesSafeFallbackWhenAllSegmentsFailCitationValidation(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "Uncited claim.", Done: true}
	close(stream)
	chatModel := &fakeAnswerChat{stream: stream}
	provider := &fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: chatModel}
	generator := NewAnswerGenerator(provider, func(string) string { return "answer prompt" })
	bus := event.NewEventBus()
	var emitted string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})

	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "Q", StandaloneQuery: "Q", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{Statement: "Verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "Policy", Content: "Verified fact."}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := `- Verified fact. <kb doc="Policy" chunk_id="1" />` + "\n\n" + renderAnswerDisclaimer(answerLanguageEnglish)
	if !result.CitationValidationFailed || result.Answer != want || emitted != want {
		t.Fatalf("result=%+v emitted=%q", result, emitted)
	}
}

func TestAnswerGeneratorDropsOrphanHeadingAndUsesSafeFallback(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      "### Policy requirements\n\nUncited claim.",
		Done:         true,
	}
	close(stream)
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   &fakeAnswerChat{stream: stream},
	}
	bus := event.NewEventBus()
	var emitted string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})

	result, err := NewAnswerGenerator(provider, func(string) string { return "answer prompt" }).Generate(
		context.Background(),
		FinalAnswerRequest{
			Question: "Q", StandaloneQuery: "Q", SessionID: "s",
			Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{
				Statement: "Verified fact.", Quote: "Verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e"}},
			}}},
			Candidates: []EvidenceCandidate{{
				OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "Policy", Content: "Verified fact.",
			}},
		},
		bus,
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if strings.Contains(emitted, "Policy requirements") || strings.Contains(emitted, "Uncited claim") {
		t.Fatalf("invalid model content leaked: %q", emitted)
	}
	if !strings.Contains(emitted, `Verified fact. <kb doc="Policy" chunk_id="1" />`) || emitted != result.Answer {
		t.Fatalf("result=%+v emitted=%q", result, emitted)
	}
	if !result.CitationValidationFailed || !strings.Contains(result.CitationValidationError, "without a fact reference") {
		t.Fatalf("validation result=%+v", result)
	}
}

func TestAnswerGeneratorKeepsValidSiblingFactsWhenOneListItemIsRejected(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content: "### Requirements\n" +
			"- First verified fact. <fact id=\"fact-1\" />\n" +
			"- Unsupported model claim.\n" +
			"- Second verified fact. <fact id=\"fact-2\" />",
		Done: true,
	}
	close(stream)
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   &fakeAnswerChat{stream: stream},
	}
	bus := event.NewEventBus()
	var emitted string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})

	result, err := NewAnswerGenerator(provider, func(string) string { return "answer prompt" }).Generate(
		context.Background(),
		FinalAnswerRequest{
			Question: "Q", SessionID: "s",
			Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{
				{Statement: "First verified fact.", Quote: "First verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e1"}}},
				{Statement: "Second verified fact.", Quote: "Second verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e2"}}},
			}},
			Candidates: []EvidenceCandidate{
				{OpaqueID: "e1", ChunkID: "c1", KnowledgeID: "d1", Title: "Policy 1", Content: "First verified fact."},
				{OpaqueID: "e2", ChunkID: "c2", KnowledgeID: "d2", Title: "Policy 2", Content: "Second verified fact."},
			},
		},
		bus,
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !result.CitationValidationFailed || emitted != result.Answer || strings.Contains(emitted, "Unsupported model claim") {
		t.Fatalf("result=%+v emitted=%q", result, emitted)
	}
	for _, required := range []string{"### Requirements", "First verified fact", "Second verified fact", `chunk_id="1"`, `chunk_id="2"`} {
		if !strings.Contains(emitted, required) {
			t.Fatalf("answer does not contain %q: %q", required, emitted)
		}
	}
}

func TestAnswerGeneratorReplacesStandaloneFactReferenceWithVerifiedText(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content: "**Onsite monitoring archive**\n\n" +
			"- Unsupported generated item one.\n" +
			"- Unsupported generated item two.\n\n" +
			`<fact id="fact-1" />`,
		Done: true,
	}
	close(stream)
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   &fakeAnswerChat{stream: stream},
	}
	bus := event.NewEventBus()
	var emitted string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})

	result, err := NewAnswerGenerator(provider, func(string) string { return "answer prompt" }).Generate(
		context.Background(),
		FinalAnswerRequest{
			Question: "Q", SessionID: "s",
			Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{
				Statement: "Archive the monitoring report and checklist.",
				Citations: []EvidenceCitation{{OpaqueID: "e"}},
			}}},
			Candidates: []EvidenceCandidate{{
				OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "Policy",
				Content: "Archive the monitoring report and checklist.",
			}},
		},
		bus,
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if strings.Contains(result.Answer, "Unsupported generated item") || strings.Contains(result.Answer, "<fact") {
		t.Fatalf("invalid generated content leaked: %q", result.Answer)
	}
	wantFact := `- Archive the monitoring report and checklist. <kb doc="Policy" chunk_id="1" />`
	if !strings.Contains(result.Answer, wantFact) || emitted != result.Answer {
		t.Fatalf("result=%+v emitted=%q", result, emitted)
	}
	if !result.CitationValidationFailed {
		t.Fatalf("expected validation recovery, result=%+v", result)
	}
}

func TestAnswerGeneratorPreservesPureStructuralLeadWithoutCitation(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      "The applicable requirements are as follows:\n- Verified fact. <fact id=\"fact-1\" />",
		Done:         true,
	}
	close(stream)
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}},
		chat:   &fakeAnswerChat{stream: stream},
	}
	result, err := NewAnswerGenerator(provider, func(string) string { return "answer prompt" }).Generate(
		context.Background(),
		FinalAnswerRequest{
			Question: "Q", SessionID: "s",
			Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{
				Statement: "Verified fact.", Quote: "Verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e"}},
			}}},
			Candidates: []EvidenceCandidate{{OpaqueID: "e", ChunkID: "c", KnowledgeID: "d", Title: "Policy", Content: "Verified fact."}},
		},
		event.NewEventBus(),
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.CitationValidationFailed || !strings.Contains(result.Answer, "The applicable requirements are as follows:") ||
		!strings.Contains(result.Answer, "Verified fact.") {
		t.Fatalf("result=%+v", result)
	}
}

func TestAnswerGeneratorEmitsValidatedParagraphBeforeModelStreamCompletes(t *testing.T) {
	stream := make(chan types.StreamResponse)
	chatModel := &fakeAnswerChat{stream: stream}
	provider := &fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: chatModel}
	generator := NewAnswerGenerator(provider, func(string) string { return "answer prompt" })
	bus := event.NewEventBus()
	firstParagraph := make(chan string, 1)
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		if data.Content != "" && !data.Done {
			select {
			case firstParagraph <- data.Content:
			default:
			}
		}
		return nil
	})
	resultCh := make(chan error, 1)
	go func() {
		_, err := generator.Generate(context.Background(), FinalAnswerRequest{
			Question: "Q", StandaloneQuery: "Q", SessionID: "s",
			Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{Statement: "Verified fact.", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
			Candidates: []EvidenceCandidate{{OpaqueID: "e", ChunkID: "c", KnowledgeBaseID: "kb", KnowledgeID: "doc", Title: "Policy", Content: "Verified fact."}},
		}, bus)
		resultCh <- err
	}()

	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "Verified fact. <fact id=\"fact-1\" />\n\n"}
	select {
	case content := <-firstParagraph:
		if strings.TrimSpace(content) == "" || utf8.RuneCountInString(content) > verifiedAnswerChunkRunes {
			t.Fatalf("first streamed chunk = %q", content)
		}
	case <-time.After(time.Second):
		t.Fatal("validated paragraph was not emitted before the model stream completed")
	}
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Done: true}
	close(stream)
	if err := <-resultCh; err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestSplitVerifiedAnswerContentPreservesUnicode(t *testing.T) {
	content := "中文回答🙂 with citation <kb doc=\"Policy\" chunk_id=\"1\" />"
	chunks := splitVerifiedAnswerContent(content, 5)
	if len(chunks) < 2 || strings.Join(chunks, "") != content {
		t.Fatalf("chunks = %q", chunks)
	}
	for _, chunk := range chunks {
		if size := utf8.RuneCountInString(chunk); size > 5 {
			t.Fatalf("chunk has %d runes: %q", size, chunk)
		}
	}
}

func TestAnswerGeneratorClarifiesThreeScenariosWithoutCallingModel(t *testing.T) {
	provider := &fakeRouteChatProvider{}
	generator := NewAnswerGenerator(provider, func(string) string { return "answer prompt" })
	bus := event.NewEventBus()
	var answer string
	var isFallback bool
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		answer += data.Content
		isFallback = isFallback || data.IsFallback
		return nil
	})

	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "Can this request be approved?", SessionID: "s",
		Aggregated: AggregatedObservation{RequiresScenarioSelection: true, Facts: []ObservedFact{
			{Scenario: "one"}, {Scenario: "two"}, {Scenario: "three"},
		}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.ModelCallID != "" || answer != renderScenarioClarification(answerLanguageEnglish) || result.Answer != answer {
		t.Fatalf("result=%+v answer=%q", result, answer)
	}
	if isFallback {
		t.Fatal("scenario clarification must not be marked as fallback")
	}
}

func TestAnswerGeneratorCalculatesPureCurrencyConversionWithoutCallingModel(t *testing.T) {
	provider := &fakeRouteChatProvider{}
	generator := NewAnswerGenerator(provider, func(string) string { return "answer prompt" })
	bus := event.NewEventBus()
	var answer string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		answer += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})

	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "100 CHF 等于多少人民币？", SessionID: "s",
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.ModelCallID != "" || !strings.Contains(answer, "100 CHF = 600 RMB") || result.Answer != answer {
		t.Fatalf("result=%+v answer=%q", result, answer)
	}
}

func TestAnswerGeneratorUsesConfiguredCurrencyRate(t *testing.T) {
	provider := &fakeRouteChatProvider{}
	generator := NewAnswerGenerator(
		provider,
		func(string) string { return "answer prompt" },
		&answerExchangeRateService{rate: &types.ExchangeRate{RMBAmount: 6.25, CHFAmount: 1}},
	)
	bus := event.NewEventBus()
	var answer string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		answer += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})
	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "100 CHF 等于多少人民币？", SessionID: "s",
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.ModelCallID != "" || !strings.Contains(answer, "100 CHF = 625 RMB") ||
		!strings.Contains(answer, "1 CHF = 6.25 RMB") || result.Answer != answer {
		t.Fatalf("result=%+v answer=%q", result, answer)
	}
}

func TestAnswerGeneratorReturnsLocalizedNoKnowledgeFallbackWithoutCallingModel(t *testing.T) {
	tests := []struct {
		question string
		want     string
	}{
		{question: "What does this unsupported term mean?", want: "No relevant knowledge was found"},
		{question: "这个未识别术语是什么意思？", want: "未检索到可支持回答的相关知识"},
	}
	for _, tt := range tests {
		t.Run(tt.question, func(t *testing.T) {
			generator := NewAnswerGenerator(&fakeRouteChatProvider{}, func(string) string { return "answer prompt" })
			bus := event.NewEventBus()
			var answer string
			bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
				answer += evt.Data.(event.AgentFinalAnswerData).Content
				return nil
			})
			result, err := generator.Generate(context.Background(), FinalAnswerRequest{Question: tt.question, SessionID: "s"}, bus)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if result.ModelCallID != "" || result.Answer != answer || !strings.Contains(answer, tt.want) || answerContainsDisclaimer(answer) {
				t.Fatalf("result=%+v answer=%q", result, answer)
			}
		})
	}
}

func TestAnswerGeneratorAppendsDisclaimerOnceAndUsesSingleDoneEvent(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: `Answer fact. <fact id="fact-1" />`, Done: true}
	close(stream)
	generator := NewAnswerGenerator(
		&fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: &fakeAnswerChat{stream: stream}},
		func(string) string { return "answer prompt" },
	)
	bus := event.NewEventBus()
	var answer string
	var doneCount int
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data := evt.Data.(event.AgentFinalAnswerData)
		answer += data.Content
		if data.Done {
			doneCount++
		}
		return nil
	})
	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "What is the policy?", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{Statement: "Answer fact.", Quote: "Answer fact.", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", KnowledgeID: "doc", ChunkID: "c", Title: "Policy", Content: "Answer fact."}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Answer != answer || strings.Count(answer, "Disclaimer:") != 1 || doneCount != 1 {
		t.Fatalf("result=%+v answer=%q doneCount=%d", result, answer, doneCount)
	}
}

func TestDetectAnswerLanguageFollowsInputBeforeContextLocale(t *testing.T) {
	englishContext := context.WithValue(context.Background(), types.LanguageContextKey, "en-US")
	if got := detectAnswerLanguage(englishContext, "这是中文问题"); got != answerLanguageChinese {
		t.Fatalf("Chinese question language = %q", got)
	}
	chineseContext := context.WithValue(context.Background(), types.LanguageContextKey, "zh-CN")
	if got := detectAnswerLanguage(chineseContext, "This is an English question"); got != answerLanguageEnglish {
		t.Fatalf("English question language = %q", got)
	}
}

func TestAnswerGeneratorUsesQuestionLanguageWhileKeepingForeignQuoteInReferences(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: `政策允许有限数量的礼品。<fact id="fact-1" />`, Done: true}
	close(stream)
	generator := NewAnswerGenerator(
		&fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: &fakeAnswerChat{stream: stream}},
		func(string) string { return "answer prompt" },
	)
	bus := event.NewEventBus()
	var answer string
	var referenceContent string
	bus.On(event.EventAgentReferences, func(_ context.Context, evt event.Event) error {
		references := evt.Data.(event.AgentReferencesData).References.(types.References)
		referenceContent = references[0].Content
		return nil
	})
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		answer += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})
	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "政策允许多少礼品？", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{
			Statement: "政策允许有限数量的礼品。", Quote: "The policy permits limited quantities of gifts.", IsAmbiguous: true,
			Citations: []EvidenceCitation{{OpaqueID: "e"}},
		}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", KnowledgeID: "doc", ChunkID: "c", Title: "Policy", Content: "The policy permits limited quantities of gifts."}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.CitationValidationFailed || !strings.Contains(answer, "政策允许有限数量") || !strings.Contains(answer, "免责声明：") || referenceContent != "The policy permits limited quantities of gifts." {
		t.Fatalf("result=%+v answer=%q reference=%q", result, answer, referenceContent)
	}
}

func TestAnswerGeneratorAllowsOrdinaryParaphraseAndInjectsCitation(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      `Use the online portal to submit requests. <fact id="fact-1" />`,
		Done:         true,
	}
	close(stream)
	generator := NewAnswerGenerator(
		&fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: &fakeAnswerChat{stream: stream}},
		func(string) string { return "answer prompt" },
	)
	bus := event.NewEventBus()
	var emitted string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})
	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "How do I submit a request?", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{
			Statement: "Requests may be submitted through the portal.",
			Quote:     "Requests may be submitted through the portal.",
			Citations: []EvidenceCitation{{OpaqueID: "e"}},
		}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", KnowledgeID: "doc", ChunkID: "c", Title: "Policy", Content: "Requests may be submitted through the portal."}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.CitationValidationFailed || emitted != result.Answer || !strings.Contains(emitted, "Use the online portal") ||
		!strings.Contains(emitted, `<kb doc="Policy" chunk_id="1" />`) || strings.Contains(emitted, `<fact`) {
		t.Fatalf("result=%+v emitted=%q", result, emitted)
	}
}

func TestAnswerGeneratorReplacesRejectedAmbiguousParaphraseWithVerifiedFact(t *testing.T) {
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      `The policy permits exactly three gifts. <fact id="fact-1" />`,
		Done:         true,
	}
	close(stream)
	generator := NewAnswerGenerator(
		&fakeRouteChatProvider{models: []*types.Model{{ID: "chat", Type: types.ModelTypeKnowledgeQA, IsDefault: true}}, chat: &fakeAnswerChat{stream: stream}},
		func(string) string { return "answer prompt" },
	)
	bus := event.NewEventBus()
	var emitted string
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		emitted += evt.Data.(event.AgentFinalAnswerData).Content
		return nil
	})
	result, err := generator.Generate(context.Background(), FinalAnswerRequest{
		Question: "How many gifts are permitted?", SessionID: "s",
		Aggregated: AggregatedObservation{Coverage: CoverageComplete, Facts: []ObservedFact{{
			Statement:   "The policy permits limited quantities of gifts.",
			Quote:       "The policy permits limited quantities of gifts.",
			IsAmbiguous: true,
			Citations:   []EvidenceCitation{{OpaqueID: "e"}},
		}}},
		Candidates: []EvidenceCandidate{{OpaqueID: "e", KnowledgeID: "doc", ChunkID: "c", Title: "Policy", Content: "The policy permits limited quantities of gifts."}},
	}, bus)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !result.CitationValidationFailed || emitted != result.Answer || strings.Contains(emitted, "exactly three") ||
		!strings.Contains(emitted, `The policy permits limited quantities of gifts. <kb doc="Policy" chunk_id="1" />`) {
		t.Fatalf("result=%+v emitted=%q", result, emitted)
	}
}

func TestRenderCitationSafeAnswerUsesStatementForOrdinaryFact(t *testing.T) {
	answer := renderCitationSafeAnswer(
		[]answerFact{{
			Statement:     "A VP approval email is required for this exception.",
			Quote:         "Emma Qu For T&E policy perspective，a VP approval email is enough...",
			SourceNumbers: []int{1},
		}},
		[]numberedEvidence{{Number: 1, CitationTag: `<kb doc="Policy" chunk_id="1" />`}},
		answerLanguageEnglish,
	)
	if !strings.Contains(answer, "A VP approval email is required") || strings.Contains(answer, "Emma Qu") {
		t.Fatalf("answer = %q", answer)
	}
}

func TestRenderCitationSafeAnswerPreservesAmbiguousQuote(t *testing.T) {
	answer := renderCitationSafeAnswer(
		[]answerFact{{
			Statement:     "The policy permits a limited number of gifts.",
			Quote:         "limited quantities",
			IsAmbiguous:   true,
			SourceNumbers: []int{1},
		}},
		[]numberedEvidence{{Number: 1, CitationTag: `<kb doc="Policy" chunk_id="1" />`}},
		answerLanguageEnglish,
	)
	if !strings.Contains(answer, "The policy permits a limited number of gifts.") ||
		!strings.Contains(answer, `Source wording: "limited quantities"`) {
		t.Fatalf("answer = %q", answer)
	}
}

func TestValidateFactQuotePreservation(t *testing.T) {
	evidence := []numberedEvidence{{Number: 1, CitationTag: `<kb doc="Policy" chunk_id="1" />`}}
	facts := []answerFact{{Quote: "limited quantities", IsAmbiguous: true, SourceNumbers: []int{1}}}
	if err := validateFactQuotePreservation(`This means three times. <kb doc="Policy" chunk_id="1" />`, facts, evidence, answerLanguageEnglish); err == nil {
		t.Fatal("paraphrased ambiguous policy must be rejected")
	}
	if err := validateFactQuotePreservation(`The policy says limited quantities. <kb doc="Policy" chunk_id="1" />`, facts, evidence, answerLanguageEnglish); err != nil {
		t.Fatalf("preserved ambiguous quote was rejected: %v", err)
	}
}

func TestResolveAnswerFactReferencesAcceptsCombinedIDsAndInjectsBackendCitations(t *testing.T) {
	evidence := []numberedEvidence{
		{Number: 1, CitationTag: `<kb doc="Policy A" chunk_id="1" />`},
		{Number: 2, CitationTag: `<kb doc="Policy B" chunk_id="2" />`},
	}
	facts := []answerFact{
		{FactID: "fact-1", SourceNumbers: []int{1}},
		{FactID: "fact-2", SourceNumbers: []int{2}},
	}
	resolved, factIDs, err := resolveAnswerFactReferences(
		`Combined policy conclusion. <fact id='fact-1, fact-2' />`,
		facts,
		evidence,
	)
	if err != nil {
		t.Fatalf("resolveAnswerFactReferences() error = %v", err)
	}
	if len(factIDs) != 2 || factIDs[0] != "fact-1" || factIDs[1] != "fact-2" || strings.Contains(resolved, "<fact") ||
		!strings.Contains(resolved, evidence[0].CitationTag) || !strings.Contains(resolved, evidence[1].CitationTag) {
		t.Fatalf("resolved=%q factIDs=%v", resolved, factIDs)
	}
}

func TestResolveAnswerFactReferencesAcceptsCommonModelTagVariants(t *testing.T) {
	tag := `<kb doc="Policy" chunk_id="1" />`
	evidence := []numberedEvidence{{Number: 1, CitationTag: tag}}
	facts := []answerFact{
		{FactID: "fact-1", SourceNumbers: []int{1}},
		{FactID: "fact-2", SourceNumbers: []int{1}},
	}
	for _, answer := range []string{
		`Conclusion. <fact ids="fact-1、fact-2">`,
		`Conclusion. <fact id="fact-1 / fact-2"></fact>`,
	} {
		resolved, factIDs, err := resolveAnswerFactReferences(answer, facts, evidence)
		if err != nil {
			t.Fatalf("resolveAnswerFactReferences(%q) error = %v", answer, err)
		}
		if len(factIDs) != 2 || strings.Contains(resolved, "<fact") || !strings.Contains(resolved, tag) {
			t.Fatalf("answer=%q resolved=%q factIDs=%v", answer, resolved, factIDs)
		}
	}
}

func TestResolveAnswerFactReferencesKeepsSharedCitationForEachFactUnit(t *testing.T) {
	tag := `<kb doc="Policy" chunk_id="1" />`
	evidence := []numberedEvidence{{Number: 1, CitationTag: tag}}
	facts := []answerFact{
		{FactID: "fact-1", SourceNumbers: []int{1}},
		{FactID: "fact-2", SourceNumbers: []int{1}},
	}
	resolved, _, err := resolveAnswerFactReferences(
		"First conclusion. <fact id=\"fact-1\" />\nSecond conclusion. <fact id=\"fact-2\" />",
		facts,
		evidence,
	)
	if err != nil {
		t.Fatalf("resolveAnswerFactReferences() error = %v", err)
	}
	if strings.Count(resolved, tag) != 2 {
		t.Fatalf("shared citation count = %d, resolved=%q", strings.Count(resolved, tag), resolved)
	}
}

func TestBuildAnswerEvidencePreservesPreviewMetadataAndCitationTag(t *testing.T) {
	refs, evidence, _ := buildAnswerEvidence(
		AggregatedObservation{Facts: []ObservedFact{{Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		[]EvidenceCandidate{{
			OpaqueID: "e", ChunkID: "chunk", ChunkIndex: 3, KnowledgeBaseID: "kb", KnowledgeID: "doc",
			Title: "Policy", KnowledgeFilename: "policy.docx", KnowledgeSource: "upload", KnowledgeChannel: "web",
			Description: "desc", Content: "fact", ImageInfo: `{"page":4}`, StartAt: 10, EndAt: 20,
		}},
	)
	if len(refs) != 1 || len(evidence) != 1 {
		t.Fatalf("refs=%+v evidence=%+v", refs, evidence)
	}
	ref := refs[0]
	if ref.KnowledgeFilename != "policy.docx" || ref.ChunkIndex != 3 || ref.StartAt != 10 || ref.EndAt != 20 || ref.ImageInfo == "" {
		t.Fatalf("reference metadata was lost: %+v", ref)
	}
	if evidence[0].CitationTag != `<kb doc="policy.docx" chunk_id="1" />` {
		t.Fatalf("citation tag = %q", evidence[0].CitationTag)
	}
	if facts := func() []answerFact {
		_, _, facts := buildAnswerEvidence(
			AggregatedObservation{Facts: []ObservedFact{{Statement: "fact", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
			[]EvidenceCandidate{{OpaqueID: "e", Title: "Policy"}},
		)
		return facts
	}(); len(facts) != 1 || facts[0].FactID != "fact-1" {
		t.Fatalf("fact IDs = %+v", facts)
	}
}

func TestValidateAnswerCitationCoverageChecksTextBelowHeading(t *testing.T) {
	evidence := []numberedEvidence{{Number: 1, CitationTag: `<kb doc="Policy" chunk_id="1" />`}}
	if err := validateAnswerCitationCoverage("# Summary\nUncited fact.", evidence); err == nil {
		t.Fatal("text below a heading must still require a citation")
	}
	if err := validateAnswerCitationCoverage("# Summary\nVerified fact. <kb doc=\"Policy\" chunk_id=\"1\" />", evidence); err != nil {
		t.Fatalf("valid cited text below heading was rejected: %v", err)
	}
}

func TestAnswerUnitNeedsCitationTreatsBoldListLabelAsStructure(t *testing.T) {
	if answerUnitNeedsCitation("* **适用对象延伸**：") {
		t.Fatal("bold list label must not be treated as a factual unit")
	}
	if !answerUnitNeedsCitation("* **适用对象延伸**：工作人员也适用该政策。") {
		t.Fatal("bold list item with a factual statement must require a citation")
	}
	if answerUnitNeedsCitation("具体要求如下：") {
		t.Fatal("pure structural lead must not require a citation")
	}
	if !answerUnitNeedsCitation("结论：礼品完全禁止：") {
		t.Fatal("a factual statement disguised as a structural lead must still require a citation")
	}
}

func TestRenderCoverageLimitationsIsDeterministicAndHidesInternalErrors(t *testing.T) {
	answer := renderCoverageLimitations([]string{
		"缺少具体凭证清单",
		"bounded evidence recovery budget exhausted",
		"bounded evidence recovery budget exhausted",
	}, answerLanguageChinese)
	if !strings.Contains(answer, "证据覆盖说明") || !strings.Contains(answer, "缺少具体凭证清单") {
		t.Fatalf("coverage limitations = %q", answer)
	}
	if strings.Contains(answer, "bounded evidence") || strings.Contains(answer, "部分问题细节仍缺少充分、可验证的依据") {
		t.Fatalf("internal or duplicate limitation leaked: %q", answer)
	}
}

func TestBuildAnswerEvidenceUsesGlobalCitationNumbersAcrossDocumentAndFAQ(t *testing.T) {
	refs, evidence, _ := buildAnswerEvidence(
		AggregatedObservation{Facts: []ObservedFact{{Citations: []EvidenceCitation{
			{OpaqueID: "doc-1"}, {OpaqueID: "faq-1"}, {OpaqueID: "doc-2"},
		}}}},
		[]EvidenceCandidate{
			{OpaqueID: "doc-1", ChunkID: "chunk-1", KnowledgeID: "knowledge-1", Title: "Policy A"},
			{OpaqueID: "faq-1", ChunkID: "chunk-2", KnowledgeID: "knowledge-2", Title: "FAQ", ChunkType: string(types.ChunkTypeFAQ)},
			{OpaqueID: "doc-2", ChunkID: "chunk-3", KnowledgeID: "knowledge-3", Title: "Policy B"},
		},
	)
	if len(refs) != 3 || len(evidence) != 3 {
		t.Fatalf("refs=%d evidence=%d", len(refs), len(evidence))
	}
	want := []string{
		`<kb doc="Policy A" chunk_id="1" />`,
		`<kb doc="FAQ" chunk_id="2" />`,
		`<kb doc="Policy B" chunk_id="3" />`,
	}
	for i := range want {
		if evidence[i].CitationTag != want[i] {
			t.Fatalf("evidence[%d].CitationTag = %q, want %q", i, evidence[i].CitationTag, want[i])
		}
	}
}

type fakeAnswerChat struct{ stream <-chan types.StreamResponse }

func (f *fakeAnswerChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, nil
}
func (f *fakeAnswerChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	return f.stream, nil
}
func (f *fakeAnswerChat) GetModelName() string { return "fake" }
func (f *fakeAnswerChat) GetModelID() string   { return "fake" }
