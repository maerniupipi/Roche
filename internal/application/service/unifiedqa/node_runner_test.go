package unifiedqa

import (
	"context"
	"errors"
	"testing"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestNodeRunnerReportsCompletedNodeToObserver(t *testing.T) {
	observer := &fakeNodeObserver{id: "observation-1"}
	runner := NewNodeRunner(observer)
	start := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	times := []time.Time{start, start.Add(125 * time.Millisecond)}
	runner.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	result, err := runner.Run(context.Background(), NodeSpec{
		RunID:         "run-1",
		NodeName:      "route",
		NodeType:      "model",
		MasterAgentID: "unified-master",
		InputSummary:  types.JSONMap{"query": "Q"},
		ConfigVersion: "catalog-v1",
	}, func(context.Context) (NodeResult, error) {
		return NodeResult{OutputSummary: types.JSONMap{"agents": []string{"finance"}}, ModelCallID: "call-1"}, nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ModelCallID != "call-1" {
		t.Fatalf("ModelCallID = %q", result.ModelCallID)
	}
	if observer.finishedMetadata["status"] != types.QANodeStatusCompleted || observer.finishedMetadata["duration_ms"] != int64(125) {
		t.Fatalf("metadata = %+v", observer.finishedMetadata)
	}
	if observer.finishedMetadata["model_call_id"] != "call-1" || observer.finishedOutput["agents"] == nil {
		t.Fatalf("metadata=%+v output=%+v", observer.finishedMetadata, observer.finishedOutput)
	}
}

func TestNodeRunnerReportsBusinessErrorToObserver(t *testing.T) {
	observer := &fakeNodeObserver{id: "observation-2"}
	runner := NewNodeRunner(observer)
	businessErr := errors.New("router failed")

	_, err := runner.Run(context.Background(), NodeSpec{RunID: "run-1", NodeName: "route"}, func(context.Context) (NodeResult, error) {
		return NodeResult{}, businessErr
	})
	if !errors.Is(err, businessErr) {
		t.Fatalf("Run() error = %v, want business error", err)
	}
	if observer.finishedMetadata["status"] != types.QANodeStatusFailed || observer.finishedMetadata["error_code"] != ErrorCodeNodeExecutionFailed {
		t.Fatalf("metadata = %+v", observer.finishedMetadata)
	}
	if !errors.Is(observer.finishedErr, businessErr) {
		t.Fatalf("observer error = %v, want business error", observer.finishedErr)
	}
}

func TestNodeRunnerIgnoresObservationFailure(t *testing.T) {
	observer := &fakeNodeObserver{startErr: errors.New("langfuse unavailable")}
	runner := NewNodeRunner(observer)

	result, err := runner.Run(context.Background(), NodeSpec{RunID: "run-1", NodeName: "route"}, func(context.Context) (NodeResult, error) {
		return NodeResult{OutputSummary: types.JSONMap{"ok": true}}, nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.OutputSummary["ok"] != true {
		t.Fatalf("result = %+v", result)
	}
}

func TestNodeRunnerPreservesDegradedStatusInObserver(t *testing.T) {
	observer := &fakeNodeObserver{}
	runner := NewNodeRunner(observer)

	_, err := runner.Run(context.Background(), NodeSpec{RunID: "run-1", NodeName: "history"}, func(context.Context) (NodeResult, error) {
		return NodeResult{Status: types.QANodeStatusDegraded, ErrorCode: "HISTORY_UNAVAILABLE"}, nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if observer.finishedMetadata["status"] != types.QANodeStatusDegraded || observer.finishedMetadata["error_code"] != "HISTORY_UNAVAILABLE" {
		t.Fatalf("metadata = %+v", observer.finishedMetadata)
	}
}

type fakeNodeObserver struct {
	id               string
	startErr         error
	finishedOutput   types.JSONMap
	finishedMetadata map[string]any
	finishedErr      error
}

func (f *fakeNodeObserver) Start(ctx context.Context, _ NodeSpec) (context.Context, NodeObservation, error) {
	if f.startErr != nil {
		return ctx, nil, f.startErr
	}
	return ctx, &fakeNodeObservation{parent: f}, nil
}

type fakeNodeObservation struct{ parent *fakeNodeObserver }

func (f *fakeNodeObservation) ID() string { return f.parent.id }
func (f *fakeNodeObservation) Finish(output types.JSONMap, metadata map[string]any, err error) error {
	f.parent.finishedOutput = output
	f.parent.finishedMetadata = metadata
	f.parent.finishedErr = err
	return nil
}
