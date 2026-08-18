package unifiedqa

import (
	"context"
	"errors"
	"time"

	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/tracing/langfuse"
	"roche.local/knowledge-agent-platform/internal/types"
)

type NodeSpec struct {
	RunID            string
	NodeName         string
	NodeType         string
	MasterAgentID    string
	WorkerAgentID    string
	AgentExecutionID string
	Attempt          int
	InputSummary     types.JSONMap
	ConfigVersion    string
}

type NodeResult struct {
	Status        types.QANodeStatus
	OutputSummary types.JSONMap
	ModelCallID   string
	ErrorCode     string
}

type NodeObservation interface {
	ID() string
	Finish(output types.JSONMap, metadata map[string]any, err error) error
}

type NodeObserver interface {
	Start(ctx context.Context, spec NodeSpec) (context.Context, NodeObservation, error)
}

type NodeRunner struct {
	observer NodeObserver
	now      func() time.Time
}

func NewNodeRunner(observer NodeObserver) *NodeRunner {
	return &NodeRunner{observer: observer, now: time.Now}
}

// Run executes business work even when node persistence or tracing is
// unavailable. Only the work function's error is returned to the caller.
func (r *NodeRunner) Run(ctx context.Context, spec NodeSpec, work func(context.Context) (NodeResult, error)) (NodeResult, error) {
	if r == nil {
		return NodeResult{}, errors.New("node runner is nil")
	}
	if work == nil {
		return NodeResult{}, errors.New("node work is nil")
	}
	startedAt := r.now()
	spec.InputSummary = cloneJSONMapBestEffort(spec.InputSummary)

	workCtx := ctx
	var observation NodeObservation
	if r.observer != nil {
		observedCtx, startedObservation, err := r.observer.Start(ctx, spec)
		if err != nil {
			logger.Warnf(ctx, "unified QA node %s observation start failed: %v", spec.NodeName, err)
		} else {
			if observedCtx != nil {
				workCtx = observedCtx
			}
			observation = startedObservation
		}
	}

	result, businessErr := work(workCtx)
	completedAt := r.now()
	status := result.Status
	if businessErr != nil {
		status = nodeFailureStatus(businessErr)
	} else if status != types.QANodeStatusDegraded {
		status = types.QANodeStatusCompleted
	}
	errorCode := result.ErrorCode
	if businessErr != nil && errorCode == "" {
		errorCode = nodeErrorCode(businessErr)
	}
	outputSummary := cloneJSONMapBestEffort(result.OutputSummary)
	if observation != nil {
		metadata := map[string]any{
			"status":        status,
			"model_call_id": result.ModelCallID,
			"error_code":    errorCode,
			"duration_ms":   completedAt.Sub(startedAt).Milliseconds(),
		}
		if err := observation.Finish(outputSummary, metadata, businessErr); err != nil {
			logger.Warnf(ctx, "unified QA node %s observation finish failed: %v", spec.NodeName, err)
		}
	}
	return result, businessErr
}

func nodeFailureStatus(err error) types.QANodeStatus {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return types.QANodeStatusCancelled
	}
	return types.QANodeStatusFailed
}

func nodeErrorCode(err error) string {
	var coded ErrorCoder
	if errors.As(err, &coded) && coded.ErrorCode() != "" {
		return coded.ErrorCode()
	}
	if errors.Is(err, context.Canceled) {
		return ErrorCodeContextCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorCodeContextDeadline
	}
	return ErrorCodeNodeExecutionFailed
}

func cloneJSONMapBestEffort(value types.JSONMap) types.JSONMap {
	cloned, err := cloneJSONMap(value)
	if err != nil {
		return types.JSONMap{"summary_error": "not_json_serializable"}
	}
	if cloned == nil {
		return types.JSONMap{}
	}
	return cloned
}

type langfuseNodeObserver struct{ manager *langfuse.Manager }

func NewLangfuseNodeObserver(manager *langfuse.Manager) NodeObserver {
	return &langfuseNodeObserver{manager: manager}
}

func (o *langfuseNodeObserver) Start(ctx context.Context, spec NodeSpec) (context.Context, NodeObservation, error) {
	spanCtx, span := o.manager.StartSpan(ctx, langfuse.SpanOptions{
		Name:  "unified_qa." + spec.NodeName,
		Input: spec.InputSummary,
		Metadata: map[string]any{
			"run_id":             spec.RunID,
			"node_type":          spec.NodeType,
			"master_agent_id":    spec.MasterAgentID,
			"worker_agent_id":    spec.WorkerAgentID,
			"agent_execution_id": spec.AgentExecutionID,
			"attempt":            spec.Attempt,
			"config_version":     spec.ConfigVersion,
		},
	})
	return spanCtx, &langfuseNodeObservation{span: span}, nil
}

type langfuseNodeObservation struct{ span *langfuse.Span }

func (o *langfuseNodeObservation) ID() string {
	if o == nil || o.span == nil {
		return ""
	}
	return o.span.ID
}

func (o *langfuseNodeObservation) Finish(output types.JSONMap, metadata map[string]any, err error) error {
	if o != nil && o.span != nil {
		o.span.Finish(output, metadata, err)
	}
	return nil
}
