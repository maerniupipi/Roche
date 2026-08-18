package session

import (
	"context"
	"testing"
	"time"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type milestoneStreamManager struct {
	events []interfaces.StreamEvent
}

func (m *milestoneStreamManager) AppendEvent(
	_ context.Context,
	_, _ string,
	evt interfaces.StreamEvent,
) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *milestoneStreamManager) GetEvents(
	_ context.Context,
	_, _ string,
	from int,
) ([]interfaces.StreamEvent, int, error) {
	if from >= len(m.events) {
		return nil, len(m.events), nil
	}
	return m.events[from:], len(m.events), nil
}

func TestAgentStreamHandlerMapsUnifiedQAMilestonesToDedicatedResponseTypes(t *testing.T) {
	bus := event.NewEventBus()
	stream := &milestoneStreamManager{}
	handler := NewAgentStreamHandler(
		context.Background(), "session", "message", "request", time.Now(),
		&types.Message{}, stream, bus,
	)
	handler.Subscribe()

	for _, item := range []struct {
		eventType event.EventType
		content   string
		stepID    string
	}{
		{event.EventQuestionUnderstood, "已完成问题理解", "question"},
		{event.EventKnowledgeSearch, "检索知识库", "search"},
	} {
		if err := bus.Emit(context.Background(), event.Event{
			ID: item.stepID, Type: item.eventType,
			Data: event.AgentThoughtData{Content: item.content, StepID: item.stepID, Done: true, Status: "completed"},
		}); err != nil {
			t.Fatalf("emit %s: %v", item.eventType, err)
		}
	}

	if len(stream.events) != 2 {
		t.Fatalf("events = %+v", stream.events)
	}
	if stream.events[0].Type != types.ResponseTypeQuestionUnderstood {
		t.Fatalf("question type = %q", stream.events[0].Type)
	}
	if stream.events[1].Type != types.ResponseTypeKnowledgeRetrieved {
		t.Fatalf("search type = %q", stream.events[1].Type)
	}
}
