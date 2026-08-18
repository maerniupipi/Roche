package service

import (
	"context"
	"errors"
	"testing"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestSessionKnowledgeQADispatchesOnlyToUnifiedQA(t *testing.T) {
	wantErr := errors.New("unified result")
	executor := &fakeUnifiedQAExecutor{err: wantErr}
	svc := &sessionService{unifiedQAService: executor}
	req := &types.QARequest{
		Session:          &types.Session{ID: "session-1"},
		Query:            "question",
		CustomAgent:      &types.CustomAgent{ID: "legacy-agent"},
		KnowledgeBaseIDs: []string{"request-kb-must-not-scope-unified-qa"},
		WebSearchEnabled: true,
	}
	bus := event.NewEventBus()

	err := svc.KnowledgeQA(context.Background(), req, bus)
	if !errors.Is(err, wantErr) {
		t.Fatalf("KnowledgeQA() error = %v, want %v", err, wantErr)
	}
	if executor.calls != 1 || executor.req != req || executor.eventBus != bus {
		t.Fatalf("executor calls=%d req=%p bus=%p", executor.calls, executor.req, executor.eventBus)
	}
}

func TestSessionKnowledgeQAFailsWhenUnifiedQAIsNotConfigured(t *testing.T) {
	svc := &sessionService{}
	if err := svc.KnowledgeQA(context.Background(), &types.QARequest{}, event.NewEventBus()); err == nil {
		t.Fatal("KnowledgeQA() error = nil, want configuration error")
	}
}

type fakeUnifiedQAExecutor struct {
	calls    int
	req      *types.QARequest
	eventBus *event.EventBus
	err      error
}

func (f *fakeUnifiedQAExecutor) Execute(_ context.Context, req *types.QARequest, eventBus *event.EventBus) error {
	f.calls++
	f.req = req
	f.eventBus = eventBus
	return f.err
}
