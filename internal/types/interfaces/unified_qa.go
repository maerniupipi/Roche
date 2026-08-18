package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

type UnifiedQARunRepository interface {
	CreateRun(ctx context.Context, run *types.QAExecutionRun) error
	GetRun(ctx context.Context, runID string) (*types.QAExecutionRun, error)
	FinishRun(ctx context.Context, runID string, update types.QARunFinishUpdate) error
}

type UnifiedQAObservationService interface {
	GetRunObservation(ctx context.Context, runID string) (*types.QARunObservation, error)
}
