package unifiedqa

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"roche.local/knowledge-agent-platform/internal/event"
)

func TestProgressMessagesUseConfiguredAgentName(t *testing.T) {
	catalog := mustTestThreeAgentCatalog(t)

	routeMessage := routeSelectionProgressMessage(catalog, []AgentTask{{AgentID: "hr"}})
	domainMessage := domainProgressMessage(catalog, "hr", DomainProgressRetrieving)
	domainDone := domainCompletionProgressMessage(catalog, "hr", DomainExecutionResult{
		Observation: AgentObservation{Status: EvidenceStatusSufficient},
		Candidates:  []EvidenceCandidate{{}, {}},
	}, nil)
	aggregated := aggregationProgressMessage(5, 2)
	answer := answerGenerationProgressMessage(2)
	for _, message := range []string{routeMessage, domainMessage, domainDone, aggregated, answer} {
		if strings.TrimSpace(message) == "" {
			t.Fatalf("progress message is empty: %q", message)
		}
	}
	if !strings.Contains(routeMessage, "人力资源") || !strings.Contains(domainMessage, "人力资源") ||
		!strings.Contains(domainDone, "2条候选证据") || !strings.Contains(aggregated, "2条可引用事实") ||
		!strings.Contains(answer, "2条已验证事实") {
		t.Fatalf("progress messages = %q, %q, %q, %q, %q", routeMessage, domainMessage, domainDone, aggregated, answer)
	}
}

func TestProgressReporterEmitsStructuredSteps(t *testing.T) {
	bus := event.NewEventBus()
	var events []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		events = append(events, evt.Data.(event.AgentThoughtData))
		return nil
	})
	reporter := newProgressReporter(bus, "run-1", "session-1", "request-1")
	reporter.Begin(context.Background(), progressStep{Lane: "workflow", Stage: "scope_resolution", Content: "scope"})
	reporter.Begin(context.Background(), progressStep{Lane: "workflow", Stage: "master_route", Content: "route"})
	reporter.Complete(context.Background(), "workflow", progressCompletion{ResultCount: 2, ModelCalls: 1})
	reporter.Close(context.Background())

	if len(events) != 5 {
		t.Fatalf("event count = %d, events=%+v", len(events), events)
	}
	if events[0].RunID != "run-1" || events[0].StepID == "" || events[0].Stage != "scope_resolution" || events[0].Status != "running" {
		t.Fatalf("start event = %+v", events[0])
	}
	var content strings.Builder
	for index, item := range events[:len(events)-1] {
		content.WriteString(item.Content)
		if item.Done || item.Status != "running" || item.StepID != events[0].StepID {
			t.Fatalf("progress event %d = %+v", index, item)
		}
		if utf8.RuneCountInString(item.Content) > unifiedQATextChunkRunes {
			t.Fatalf("progress chunk %q exceeds %d runes", item.Content, unifiedQATextChunkRunes)
		}
	}
	if content.String() != "scoperoute" || events[1].Stage != "scope_resolution" || events[2].Stage != "master_route" {
		t.Fatalf("progress content=%q events=%+v", content.String(), events)
	}
	last := events[len(events)-1]
	if !last.Done || last.StepID != events[0].StepID || last.Stage != "thinking" ||
		last.Status != "completed" || last.ResultCount != 2 || last.ModelCalls != 1 {
		t.Fatalf("thinking completion = %+v", last)
	}
}
