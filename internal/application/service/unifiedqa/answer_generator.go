package unifiedqa

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type FinalAnswerRequest struct {
	Question        string
	StandaloneQuery string
	Aggregated      AggregatedObservation
	Candidates      []EvidenceCandidate
	ModelID         string
	SessionID       string
	RequestID       string
	Policy          TopicAnswerPolicy
	BeforeAnswer    func(context.Context)
	streamInterval  time.Duration
}

type FinalAnswerResult struct {
	Answer                   string
	ModelCallID              string
	References               types.References
	ResponsePolicyCodes      []string
	CitationValidationFailed bool
	CitationValidationError  string
}

type AnswerGenerator struct {
	models        RouteChatModelProvider
	resolvePrompt PromptContentResolver
	exchangeRates interfaces.ExchangeRateService
}

func NewAnswerGenerator(
	models RouteChatModelProvider,
	resolvePrompt PromptContentResolver,
	exchangeRates ...interfaces.ExchangeRateService,
) *AnswerGenerator {
	generator := &AnswerGenerator{models: models, resolvePrompt: resolvePrompt}
	if len(exchangeRates) > 0 {
		generator.exchangeRates = exchangeRates[0]
	}
	return generator
}

func (g *AnswerGenerator) Generate(
	ctx context.Context,
	request FinalAnswerRequest,
	eventBus *event.EventBus,
) (FinalAnswerResult, error) {
	callID := uuid.NewString()
	if g == nil || g.models == nil || g.resolvePrompt == nil || eventBus == nil {
		return FinalAnswerResult{ModelCallID: callID}, fmt.Errorf("answer generator is not configured")
	}
	prompt := strings.TrimSpace(g.resolvePrompt("unified-answer-v1"))
	if prompt == "" {
		return FinalAnswerResult{ModelCallID: callID}, fmt.Errorf("unified answer prompt cannot be resolved")
	}
	request.Aggregated.Facts = sortFactsByDocumentPriority(request.Aggregated.Facts)
	language := detectAnswerLanguage(ctx, request.Question)
	businessPolicy := buildAnswerBusinessPolicy(
		request.Question,
		request.Aggregated.Facts,
		request.Aggregated.RequiresScenarioSelection,
	)
	// Most questions do not involve currency conversion. Only hit the database
	// after the deterministic parser confirms that an RMB/CHF conversion is
	// actually needed, so ordinary unified-QA requests pay no extra query cost.
	if businessPolicy.CurrencyConversion != nil {
		businessPolicy = buildAnswerBusinessPolicy(
			request.Question,
			request.Aggregated.Facts,
			request.Aggregated.RequiresScenarioSelection,
			g.resolveRMBCHFRate(ctx),
		)
	}
	if businessPolicy.RequiresScenarioClarification {
		return emitDeterministicAnswer(ctx, eventBus, request, renderScenarioClarification(language), false)
	}
	if len(request.Aggregated.Facts) == 0 {
		if businessPolicy.CurrencyConversion != nil {
			return emitDeterministicAnswer(ctx, eventBus, request, renderCurrencyPolicyAddendum(businessPolicy, language), false)
		}
		content := strings.TrimSpace(request.Policy.NoKnowledgeResponse())
		if content == "" {
			content = renderNoKnowledgeFallback(language)
		}
		return emitDeterministicAnswer(ctx, eventBus, request, content, true, request.Policy.NoKnowledgeResponsePolicyCodes()...)
	}
	references, evidence, safeFacts := buildAnswerEvidence(request.Aggregated, request.Candidates)
	if len(evidence) == 0 || len(safeFacts) == 0 {
		content := strings.TrimSpace(request.Policy.NoKnowledgeResponse())
		if content == "" {
			content = renderNoKnowledgeFallback(language)
		}
		return emitDeterministicAnswer(ctx, eventBus, request, content, true, request.Policy.NoKnowledgeResponsePolicyCodes()...)
	}
	modelID, err := NewChatRouteModel(g.models).resolveModelID(ctx, request.ModelID)
	if err != nil {
		return FinalAnswerResult{ModelCallID: callID}, err
	}
	chatModel, err := g.models.GetChatModel(ctx, modelID)
	if err != nil {
		return FinalAnswerResult{ModelCallID: callID}, fmt.Errorf("get final answer model: %w", err)
	}
	input, err := json.Marshal(map[string]any{
		"question":          request.Question,
		"standalone_query":  request.StandaloneQuery,
		"coverage":          request.Aggregated.Coverage,
		"facts":             safeFacts,
		"conflicts":         request.Aggregated.Conflicts,
		"missing":           request.Aggregated.MissingRequirements,
		"topic_statuses":    request.Aggregated.TopicStatuses,
		"evidence":          evidence,
		"business_policy":   businessPolicy,
		"response_language": language.promptName(),
		"citation_language": "preserve the original quote language",
	})
	if err != nil {
		return FinalAnswerResult{ModelCallID: callID}, fmt.Errorf("marshal final answer input: %w", err)
	}
	thinking := false
	stream, err := chatModel.ChatStream(ctx, []chat.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: string(input)},
	}, &chat.ChatOptions{Temperature: 0.1, MaxCompletionTokens: 2400, Thinking: &thinking})
	if err != nil {
		return FinalAnswerResult{ModelCallID: callID}, fmt.Errorf("generate final answer: %w", err)
	}
	if stream == nil {
		return FinalAnswerResult{ModelCallID: callID}, fmt.Errorf("generate final answer: nil stream")
	}
	answerID := uuid.NewString() + "-answer"
	answerStarted := false
	startAnswer := func() error {
		if answerStarted {
			return nil
		}
		answerStarted = true
		if request.BeforeAnswer != nil {
			request.BeforeAnswer(ctx)
		}
		return eventBus.Emit(ctx, event.Event{
			Type: event.EventAgentReferences, SessionID: request.SessionID, RequestID: request.RequestID,
			Data: event.AgentReferencesData{References: references},
		})
	}
	emitAnswerContent := func(content string, done bool) error {
		if err := startAnswer(); err != nil {
			return err
		}
		return emitFinalAnswerChunks(
			ctx, eventBus, answerID, request.SessionID, request.RequestID,
			content, done, false, request.streamInterval,
		)
	}
	var pending strings.Builder
	var accepted strings.Builder
	var deferredStructure strings.Builder
	acceptedFactualContent := false
	acceptedFactIDs := make(map[string]struct{}, len(safeFacts))
	validationErrors := make([]string, 0, 1)
	policyAddendum := renderCurrencyPolicyAddendum(businessPolicy, language)
	coverageLimitations := renderCoverageLimitations(request.Aggregated.MissingRequirements, language)
	appendAccepted := func(content string, factIDs []string, done bool) (bool, error) {
		if strings.TrimSpace(content) == "" {
			return false, nil
		}
		acceptedFactualContent = true
		for _, factID := range factIDs {
			acceptedFactIDs[factID] = struct{}{}
		}
		accepted.WriteString(content)
		return true, emitAnswerContent(content, done)
	}
	replaceRejectedSegment := func(segment string, factIDs []string, validationErr error, done bool) (bool, error) {
		validationErrors = append(validationErrors, validationErr.Error())
		missingIDs := make([]string, 0, len(factIDs))
		for _, factID := range factIDs {
			if _, acceptedAlready := acceptedFactIDs[factID]; !acceptedAlready {
				missingIDs = append(missingIDs, factID)
			}
		}
		replacementFacts := answerFactsByID(safeFacts, missingIDs)
		if len(replacementFacts) == 0 {
			return false, nil
		}
		replacement := renderCitationSafeAnswer(replacementFacts, evidence, language)
		replacement += answerTrailingLineBreaks(segment)
		validated := deferredStructure.String() + replacement
		deferredStructure.Reset()
		return appendAccepted(validated, missingIDs, done)
	}
	emitUnit := func(unit string, done bool) (bool, error) {
		if strings.TrimSpace(unit) == "" {
			deferredStructure.WriteString(unit)
			return false, nil
		}
		// A model occasionally emits a fact reference on its own line after a
		// list. Accepting that line would mark the fact as covered while the
		// uncited list items are discarded, leaving an orphan heading followed
		// only by a citation. Replace a reference-only unit with the verified
		// fact text instead.
		if answerUnitContainsOnlyFactReferences(unit) {
			_, factIDs, err := resolveAnswerFactReferences(unit, safeFacts, evidence)
			if err != nil {
				return replaceRejectedSegment(unit, factIDs, err, done)
			}
			return replaceRejectedSegment(unit, factIDs, fmt.Errorf("final answer contains a fact reference without factual content"), done)
		}
		if !answerSegmentHasFactualContent(unit) {
			deferredStructure.WriteString(unit)
			return false, nil
		}
		resolved, factIDs, err := resolveAnswerFactReferences(unit, safeFacts, evidence)
		if err != nil {
			return replaceRejectedSegment(unit, factIDs, err, done)
		}
		unit = resolved
		segmentFacts := answerFactsByID(safeFacts, factIDs)
		if err := validateAnswerCitationCoverage(unit, evidence); err != nil {
			return replaceRejectedSegment(unit, factIDs, err, done)
		}
		if err := validateFactQuotePreservation(unit, segmentFacts, evidence, language); err != nil {
			return replaceRejectedSegment(unit, factIDs, err, done)
		}
		if policyAddendum != "" && answerContainsCurrencyCalculation(unit) {
			return replaceRejectedSegment(unit, factIDs, fmt.Errorf("model answer must not duplicate the deterministic currency calculation"), done)
		}
		if answerContainsDisclaimer(unit) {
			return replaceRejectedSegment(unit, factIDs, fmt.Errorf("model answer must not duplicate the deterministic disclaimer"), done)
		}
		if err := validateAnswerLanguage(unit, language, segmentFacts); err != nil {
			return replaceRejectedSegment(unit, factIDs, err, done)
		}
		validated := deferredStructure.String() + unit
		deferredStructure.Reset()
		return appendAccepted(validated, factIDs, done)
	}
	emitSegment := func(segment string, done bool) (bool, error) {
		units := splitAnswerSegmentUnits(segment)
		emitted := false
		for index, unit := range units {
			unitEmitted, err := emitUnit(unit, done && index == len(units)-1)
			if err != nil {
				return emitted, err
			}
			emitted = emitted || unitEmitted
		}
		return emitted, nil
	}
	finish := func() (FinalAnswerResult, error) {
		if rest := pending.String(); rest != "" {
			var emitErr error
			_, emitErr = emitSegment(rest, false)
			if emitErr != nil {
				return FinalAnswerResult{Answer: accepted.String(), ModelCallID: callID, References: references}, emitErr
			}
			pending.Reset()
		}
		if !acceptedFactualContent {
			validationErrors = append(validationErrors, "final answer contains no validated factual content")
		}
		deferredStructure.Reset()
		finalContent := ""
		if acceptedFactualContent && len(validationErrors) > 0 {
			missingIDs := make([]string, 0, len(safeFacts))
			for _, fact := range safeFacts {
				if _, ok := acceptedFactIDs[fact.FactID]; !ok {
					missingIDs = append(missingIDs, fact.FactID)
				}
			}
			if missingFacts := answerFactsByID(safeFacts, missingIDs); len(missingFacts) > 0 {
				finalContent = renderCitationSafeAnswer(missingFacts, evidence, language)
				if accepted.Len() > 0 {
					finalContent = "\n\n" + finalContent
				}
				accepted.WriteString(finalContent)
				for _, factID := range missingIDs {
					acceptedFactIDs[factID] = struct{}{}
				}
			}
		}
		result := FinalAnswerResult{
			Answer: accepted.String(), ModelCallID: callID, References: references,
			CitationValidationFailed: len(validationErrors) > 0,
			CitationValidationError:  strings.Join(validationErrors, "; "),
		}
		if !acceptedFactualContent {
			finalContent = renderCitationSafeAnswer(safeFacts, evidence, language)
			result.Answer = finalContent
		}
		if finalContent != "" {
			if err := emitAnswerContent(finalContent, false); err != nil {
				return result, err
			}
		}
		tail := make([]string, 0, 4)
		result.ResponsePolicyCodes = request.Policy.TailResponsePolicyCodes()
		if coverageLimitations != "" {
			tail = append(tail, coverageLimitations)
		}
		if policyAddendum != "" {
			tail = append(tail, policyAddendum)
		}
		tail = append(tail, request.Policy.TailSections()...)
		if len(request.Policy.SuccessDisclaimers) == 0 && len(request.Policy.Addenda) == 0 {
			tail = append(tail, renderAnswerDisclaimer(language))
			result.ResponsePolicyCodes = append(result.ResponsePolicyCodes, globalResponsePolicyCode("answer_disclaimer"))
		}
		tailContent := "\n\n" + strings.Join(tail, "\n\n")
		result.Answer += tailContent
		if err := emitAnswerContent(tailContent, true); err != nil {
			return result, err
		}
		return result, nil
	}
	for {
		select {
		case <-ctx.Done():
			return FinalAnswerResult{Answer: accepted.String(), ModelCallID: callID, References: references}, ctx.Err()
		case response, ok := <-stream:
			if !ok {
				return finish()
			}
			if response.ResponseType == types.ResponseTypeError {
				return FinalAnswerResult{Answer: accepted.String(), ModelCallID: callID, References: references}, fmt.Errorf("final answer stream: %s", response.Content)
			}
			if response.ResponseType != types.ResponseTypeAnswer {
				continue
			}
			pending.WriteString(response.Content)
			segments, rest := takeCompleteAnswerSegments(pending.String())
			pending.Reset()
			pending.WriteString(rest)
			for _, segment := range segments {
				if _, err := emitSegment(segment, false); err != nil {
					return FinalAnswerResult{Answer: accepted.String(), ModelCallID: callID, References: references}, err
				}
			}
			if response.Done {
				return finish()
			}
		}
	}
}

func (g *AnswerGenerator) resolveRMBCHFRate(ctx context.Context) rmbCHFRate {
	if g == nil || g.exchangeRates == nil {
		return defaultRMBCHFRate
	}
	rate, err := g.exchangeRates.GetExchangeRate(ctx)
	if err != nil {
		logger.Warnf(ctx, "load RMB/CHF exchange rate failed, using Func033 default 1:6: %v", err)
		return defaultRMBCHFRate
	}
	if rate == nil {
		return defaultRMBCHFRate
	}
	configured := rmbCHFRate{
		RMBAmount: strconv.FormatFloat(rate.RMBAmount, 'f', -1, 64),
		CHFAmount: strconv.FormatFloat(rate.CHFAmount, 'f', -1, 64),
	}
	if !configured.valid() {
		logger.Warnf(ctx, "invalid RMB/CHF exchange rate configuration, using Func033 default 1:6")
		return defaultRMBCHFRate
	}
	return configured
}

type numberedEvidence struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	CitationTag string `json:"-"`
	Content     string `json:"content"`
}

type answerFact struct {
	FactID             string   `json:"fact_id"`
	Statement          string   `json:"statement"`
	Quote              string   `json:"quote,omitempty"`
	IsAmbiguous        bool     `json:"is_ambiguous,omitempty"`
	Scenario           string   `json:"scenario,omitempty"`
	DocumentLevel      string   `json:"document_level,omitempty"`
	Currency           string   `json:"currency,omitempty"`
	SourceNumbers      []int    `json:"source_numbers"`
	ContributingAgents []string `json:"contributing_agents,omitempty"`
}

func emitDeterministicAnswer(
	ctx context.Context,
	eventBus *event.EventBus,
	request FinalAnswerRequest,
	content string,
	isFallback bool,
	responsePolicyCodes ...string,
) (FinalAnswerResult, error) {
	if eventBus == nil {
		return FinalAnswerResult{}, fmt.Errorf("answer event bus is not configured")
	}
	if request.BeforeAnswer != nil {
		request.BeforeAnswer(ctx)
	}
	err := emitFinalAnswerChunks(
		ctx, eventBus, uuid.NewString()+"-answer", request.SessionID, request.RequestID,
		content, true, isFallback, request.streamInterval,
	)
	return FinalAnswerResult{Answer: content, ResponsePolicyCodes: slices.Clone(responsePolicyCodes)}, err
}

const verifiedAnswerChunkRunes = unifiedQATextChunkRunes

func splitVerifiedAnswerContent(content string, maxRunes int) []string {
	return splitTextChunks(content, maxRunes)
}

var (
	answerCitationTagPattern       = regexp.MustCompile(`<kb\s+doc="[^"]*"\s+chunk_id="([0-9]+)"\s*/>`)
	answerFactTagPattern           = regexp.MustCompile(`<fact\s+ids?\s*=\s*["']([^"']+)["']\s*/?>(?:</fact>)?`)
	answerFactIDPattern            = regexp.MustCompile(`fact-[0-9]+`)
	answerMarkdownLabelPattern     = regexp.MustCompile(`^\s*(?:[-*+]\s+|[0-9]+[.)]\s+)?\*\*[^*]+\*\*[：:]?\s*$`)
	answerParagraphBoundaryPattern = regexp.MustCompile(`\r?\n[ \t]*\r?\n`)
	answerListItemPattern          = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)`)
)

func takeCompleteAnswerSegments(buffer string) ([]string, string) {
	boundaries := answerParagraphBoundaryPattern.FindAllStringIndex(buffer, -1)
	if len(boundaries) == 0 {
		return nil, buffer
	}
	segments := make([]string, 0, len(boundaries))
	start := 0
	for _, boundary := range boundaries {
		segments = append(segments, buffer[start:boundary[1]])
		start = boundary[1]
	}
	return segments, buffer[start:]
}

// splitAnswerSegmentUnits keeps Markdown layout intact while isolating list
// items and table rows. Citation validation can then reject or safely replace
// one factual unit without discarding valid sibling facts in the same streamed
// paragraph.
func splitAnswerSegmentUnits(segment string) []string {
	if segment == "" {
		return nil
	}
	lines := strings.SplitAfter(segment, "\n")
	units := make([]string, 0, len(lines))
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		units = append(units, current.String())
		current.Reset()
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || answerMarkdownLabelPattern.MatchString(trimmed) {
			flush()
			units = append(units, line)
			continue
		}
		if answerListItemPattern.MatchString(trimmed) || strings.HasPrefix(trimmed, "|") {
			flush()
		}
		current.WriteString(line)
	}
	flush()
	return units
}

func answerUnitContainsOnlyFactReferences(unit string) bool {
	if !answerFactTagPattern.MatchString(unit) {
		return false
	}
	remaining := answerFactTagPattern.ReplaceAllString(unit, "")
	remaining = strings.TrimSpace(strings.Trim(remaining, "-_*|:："))
	return remaining == ""
}

func answerTrailingLineBreaks(value string) string {
	index := len(value)
	for index > 0 && (value[index-1] == '\n' || value[index-1] == '\r') {
		index--
	}
	return value[index:]
}

func validateAnswerCitationCoverage(answer string, evidence []numberedEvidence) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return fmt.Errorf("final answer is empty")
	}
	allowed := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		allowed[item.CitationTag] = struct{}{}
	}
	matches := answerCitationTagPattern.FindAllString(answer, -1)
	if strings.Count(answer, "<kb") != len(matches) {
		return fmt.Errorf("final answer contains a malformed citation tag")
	}
	for _, tag := range matches {
		if _, ok := allowed[tag]; !ok {
			return fmt.Errorf("final answer contains an unknown citation tag")
		}
	}
	if len(evidence) == 0 {
		return fmt.Errorf("final answer has no verified evidence")
	}
	for _, unit := range answerCitationUnits(answer) {
		if answerUnitNeedsCitation(unit) && !answerCitationTagPattern.MatchString(unit) {
			return fmt.Errorf("final answer contains an uncited factual unit: %q", truncateValidationText(unit, 120))
		}
	}
	return nil
}

func validateFactQuotePreservation(answer string, facts []answerFact, evidence []numberedEvidence, language answerLanguage) error {
	tags := make(map[int]string, len(evidence))
	for _, item := range evidence {
		tags[item.Number] = item.CitationTag
	}
	normalizedAnswer := normalizeEvidenceText(answerCitationTagPattern.ReplaceAllString(answer, ""))
	for number, tag := range tags {
		if !strings.Contains(answer, tag) {
			continue
		}
		quoteRequired := false
		quoteMatched := false
		for _, fact := range facts {
			if !fact.IsAmbiguous || strings.TrimSpace(fact.Quote) == "" || !textMatchesAnswerLanguage(fact.Quote, language) || !slices.Contains(fact.SourceNumbers, number) {
				continue
			}
			quoteRequired = true
			if strings.Contains(normalizedAnswer, normalizeEvidenceText(fact.Quote)) {
				quoteMatched = true
				break
			}
		}
		if quoteRequired && !quoteMatched {
			return fmt.Errorf("ambiguous policy fact must preserve the verified quote")
		}
	}
	return nil
}

func answerCitationUnits(answer string) []string {
	blocks := regexp.MustCompile(`\r?\n\s*\r?\n`).Split(answer, -1)
	units := make([]string, 0, len(blocks))
	listItem := regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)`)
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		containsStructuredRows := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if listItem.MatchString(trimmed) || strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "#") {
				containsStructuredRows = true
				break
			}
		}
		if !containsStructuredRows {
			if strings.TrimSpace(block) != "" {
				units = append(units, strings.TrimSpace(block))
			}
			continue
		}
		var current strings.Builder
		flush := func() {
			if value := strings.TrimSpace(current.String()); value != "" {
				units = append(units, value)
			}
			current.Reset()
		}
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				flush()
				units = append(units, trimmed)
				continue
			}
			if listItem.MatchString(trimmed) || strings.HasPrefix(trimmed, "|") {
				flush()
			}
			if trimmed != "" {
				if current.Len() > 0 {
					current.WriteByte('\n')
				}
				current.WriteString(trimmed)
			}
		}
		flush()
	}
	return units
}

func answerUnitNeedsCitation(unit string) bool {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	withoutPunctuation := strings.Trim(trimmed, "-_*|:： ")
	if withoutPunctuation == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "**") && strings.HasSuffix(trimmed, "**") && strings.Count(trimmed, "**") == 2 {
		return false
	}
	if answerMarkdownLabelPattern.MatchString(trimmed) {
		return false
	}
	if answerUnitIsStructuralLead(trimmed) {
		return false
	}
	// Markdown table separator rows contain only pipes, colons and dashes.
	if strings.HasPrefix(trimmed, "|") && strings.Trim(trimmed, "|:- ") == "" {
		return false
	}
	return true
}

func answerUnitIsStructuralLead(unit string) bool {
	if answerFactTagPattern.MatchString(unit) || answerCitationTagPattern.MatchString(unit) {
		return false
	}
	value := answerListItemPattern.ReplaceAllString(strings.TrimSpace(unit), "")
	value = strings.TrimSpace(strings.Trim(value, "*_"))
	if len([]rune(value)) > 100 || (!strings.HasSuffix(value, ":") && !strings.HasSuffix(value, "：")) {
		return false
	}
	core := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(value, ":"), "："))
	lower := strings.ToLower(core)
	for _, label := range []string{
		"总结", "结论", "适用情况", "相关要求", "具体要求", "主要要求",
		"summary", "in summary", "conclusion", "applicable requirements", "requirements",
	} {
		if lower == label {
			return true
		}
	}
	for _, suffix := range []string{"如下", "主要包括", "具体包括", "分为以下", "as follows", "the following"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func answerSegmentHasFactualContent(segment string) bool {
	for _, unit := range answerCitationUnits(segment) {
		if answerUnitNeedsCitation(unit) {
			return true
		}
	}
	return false
}

func resolveAnswerFactReferences(answer string, facts []answerFact, evidence []numberedEvidence) (string, []string, error) {
	matches := answerFactTagPattern.FindAllStringSubmatch(answer, -1)
	if strings.Count(answer, "<fact") != len(matches) {
		return "", nil, fmt.Errorf("final answer contains a malformed fact reference")
	}
	if strings.Contains(answer, "<kb") {
		return "", nil, fmt.Errorf("model answer must use fact references instead of knowledge citations")
	}
	if len(matches) == 0 {
		if answerSegmentHasFactualContent(answer) {
			return "", nil, fmt.Errorf("final answer contains a factual unit without a fact reference: %q", truncateValidationText(answer, 120))
		}
		return answer, nil, nil
	}

	factsByID := make(map[string]answerFact, len(facts))
	for _, fact := range facts {
		factsByID[fact.FactID] = fact
	}
	tagsByNumber := make(map[int]string, len(evidence))
	for _, item := range evidence {
		tagsByNumber[item.Number] = item.CitationTag
	}
	factIDs := make([]string, 0, len(matches))
	seenFacts := make(map[string]struct{}, len(matches))
	var resolveErr error
	resolved := answerFactTagPattern.ReplaceAllStringFunc(answer, func(tag string) string {
		if resolveErr != nil {
			return ""
		}
		match := answerFactTagPattern.FindStringSubmatch(tag)
		referencedIDs := answerFactIDPattern.FindAllString(match[1], -1)
		remainder := answerFactIDPattern.ReplaceAllString(match[1], "")
		if len(referencedIDs) == 0 || strings.Trim(remainder, " ,，、;；|/") != "" {
			resolveErr = fmt.Errorf("final answer contains a malformed fact reference")
			return ""
		}
		citations := make([]string, 0, len(referencedIDs))
		seenCitations := make(map[string]struct{}, len(referencedIDs))
		for _, factID := range referencedIDs {
			fact, ok := factsByID[factID]
			if !ok {
				resolveErr = fmt.Errorf("final answer contains an unknown fact reference %q", factID)
				return ""
			}
			if _, ok := seenFacts[fact.FactID]; !ok {
				seenFacts[fact.FactID] = struct{}{}
				factIDs = append(factIDs, fact.FactID)
			}
			for _, number := range fact.SourceNumbers {
				citation := tagsByNumber[number]
				if citation == "" {
					continue
				}
				if _, ok := seenCitations[citation]; ok {
					continue
				}
				seenCitations[citation] = struct{}{}
				citations = append(citations, citation)
			}
		}
		if len(citations) == 0 {
			resolveErr = fmt.Errorf("fact reference %q has no verified citation", strings.Join(referencedIDs, ","))
			return ""
		}
		return strings.Join(citations, " ")
	})
	if resolveErr != nil {
		return "", factIDs, resolveErr
	}
	return resolved, factIDs, nil
}

func answerFactsByID(facts []answerFact, factIDs []string) []answerFact {
	selected := make([]answerFact, 0, len(factIDs))
	wanted := make(map[string]struct{}, len(factIDs))
	for _, factID := range factIDs {
		wanted[factID] = struct{}{}
	}
	for _, fact := range facts {
		if _, ok := wanted[fact.FactID]; ok {
			selected = append(selected, fact)
		}
	}
	return selected
}

func renderCitationSafeAnswer(facts []answerFact, evidence []numberedEvidence, language answerLanguage) string {
	tags := make(map[int]string, len(evidence))
	for _, item := range evidence {
		tags[item.Number] = item.CitationTag
	}
	lines := make([]string, 0, len(facts)+1)
	for _, fact := range facts {
		factTags := make([]string, 0, len(fact.SourceNumbers))
		for _, number := range fact.SourceNumbers {
			if tag := tags[number]; tag != "" {
				factTags = append(factTags, tag)
			}
		}
		statement := strings.TrimSpace(fact.Statement)
		// Exact source wording is mandatory only for ambiguous policy language.
		// Keep the reviewer's statement as the readable conclusion and append the
		// exact wording for ambiguity instead of replacing the conclusion with a
		// truncated table cell or email fragment.
		if fact.IsAmbiguous && strings.TrimSpace(fact.Quote) != "" && textMatchesAnswerLanguage(fact.Quote, language) {
			quote := strings.Join(strings.Fields(fact.Quote), " ")
			if normalizeEvidenceText(statement) == normalizeEvidenceText(quote) || statement == "" {
				statement = quote
			} else if language == answerLanguageEnglish {
				statement += ` Source wording: "` + quote + `"`
			} else {
				statement += ` 原文：“` + quote + `”`
			}
		}
		if statement == "" || len(factTags) == 0 {
			continue
		}
		lines = append(lines, "- "+statement+" "+strings.Join(factTags, " "))
	}
	if len(lines) == 0 {
		return renderNoKnowledgeFallback(language)
	}
	return strings.Join(lines, "\n")
}

func renderCoverageLimitations(missing []string, language answerLanguage) string {
	items := make([]string, 0, min(len(missing), 5))
	seen := make(map[string]struct{}, len(missing))
	for _, requirement := range missing {
		requirement = strings.Join(strings.Fields(requirement), " ")
		if requirement == "" {
			continue
		}
		requirement = strings.NewReplacer("<", "‹", ">", "›").Replace(requirement)
		if runes := []rune(requirement); len(runes) > 300 {
			requirement = string(runes[:300]) + "…"
		}
		lower := strings.ToLower(requirement)
		for _, internal := range []string{
			"bounded evidence", "evidence review", "valid quote", "citation", "recovery budget", "domain evidence",
			"证据复核", "引用校验", "补查预算", "内部工具", "模型输出",
		} {
			if strings.Contains(lower, internal) {
				if language == answerLanguageEnglish {
					requirement = "Some requested details still lack sufficient verifiable evidence."
				} else {
					requirement = "部分问题细节仍缺少充分、可验证的依据。"
				}
				break
			}
		}
		key := normalizeEvidenceText(requirement)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, requirement)
		if len(items) == 5 {
			break
		}
	}
	if len(items) == 0 {
		return ""
	}
	if len(items) > 1 {
		genericZH := normalizeEvidenceText("部分问题细节仍缺少充分、可验证的依据。")
		genericEN := normalizeEvidenceText("Some requested details still lack sufficient verifiable evidence.")
		filtered := items[:0]
		for _, item := range items {
			normalized := normalizeEvidenceText(item)
			if normalized == genericZH || normalized == genericEN {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
	}
	if language == answerLanguageEnglish {
		return "Evidence coverage note: the following requested details are not fully supported by the currently verified evidence:\n- " + strings.Join(items, "\n- ")
	}
	return "证据覆盖说明：以下问题细节尚未获得完整、可验证的依据：\n- " + strings.Join(items, "\n- ")
}

func truncateValidationText(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "..."
}

func buildAnswerEvidence(aggregated AggregatedObservation, candidates []EvidenceCandidate) (types.References, []numberedEvidence, []answerFact) {
	cited := make(map[string]struct{})
	for _, fact := range aggregated.Facts {
		for _, citation := range fact.Citations {
			cited[citation.OpaqueID] = struct{}{}
		}
	}
	refs := make(types.References, 0, len(cited))
	evidence := make([]numberedEvidence, 0, len(cited))
	numbers := make(map[string]int, len(cited))
	for _, candidate := range candidates {
		if _, ok := cited[candidate.OpaqueID]; !ok {
			continue
		}
		refs = append(refs, &types.SearchResult{
			ID: candidate.ChunkID, KnowledgeBaseID: candidate.KnowledgeBaseID, KnowledgeID: candidate.KnowledgeID,
			ChunkIndex: candidate.ChunkIndex, StartAt: candidate.StartAt, EndAt: candidate.EndAt,
			KnowledgeTitle: candidate.Title, KnowledgeFilename: candidate.KnowledgeFilename,
			KnowledgeSource: candidate.KnowledgeSource, KnowledgeChannel: candidate.KnowledgeChannel,
			KnowledgeDescription: candidate.Description, Content: candidate.Content,
			ImageInfo: candidate.ImageInfo, ChunkType: candidate.ChunkType, Score: candidate.Score,
		})
		number := len(evidence) + 1
		numbers[candidate.OpaqueID] = number
		evidence = append(evidence, numberedEvidence{
			Number: number, Title: candidate.Title,
			CitationTag: citationTag(candidate, fmt.Sprintf("%d", number)),
			Content:     truncateAnswerEvidence(candidate.Content, 6000),
		})
	}
	facts := make([]answerFact, 0, len(aggregated.Facts))
	for _, fact := range aggregated.Facts {
		safe := answerFact{
			Statement: fact.Statement, Quote: fact.Quote, IsAmbiguous: fact.IsAmbiguous,
			Scenario: fact.Scenario, DocumentLevel: fact.DocumentLevel, Currency: fact.Currency,
			ContributingAgents: fact.ContributingAgents,
		}
		for _, citation := range fact.Citations {
			if number := numbers[citation.OpaqueID]; number > 0 {
				safe.SourceNumbers = append(safe.SourceNumbers, number)
			}
		}
		if len(safe.SourceNumbers) > 0 {
			safe.FactID = fmt.Sprintf("fact-%d", len(facts)+1)
			facts = append(facts, safe)
		}
	}
	return refs, evidence, facts
}

func citationTag(candidate EvidenceCandidate, token string) string {
	title := strings.TrimSpace(candidate.KnowledgeFilename)
	if title == "" {
		title = strings.TrimSpace(candidate.Title)
	}
	title = strings.ReplaceAll(title, `"`, `'`)
	return fmt.Sprintf(`<kb doc="%s" chunk_id="%s" />`, title, token)
}

func truncateAnswerEvidence(content string, maxChars int) string {
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}
	return string(runes[:maxChars]) + "…"
}
