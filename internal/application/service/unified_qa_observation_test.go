package service

import (
	"context"
	"testing"

	"gorm.io/gorm"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestUnifiedQAObservationScopesRunsToOwner(t *testing.T) {
	repo := &observationRepository{run: &types.QAExecutionRun{ID: "run", UserID: "owner"}}
	service := NewUnifiedQAObservationService(repo)
	ownerCtx := context.WithValue(context.Background(), types.UserIDContextKey, "owner")
	observation, err := service.GetRunObservation(ownerCtx, "run")
	if err != nil || observation.Run.ID != "run" {
		t.Fatalf("GetRunObservation() = (%+v, %v)", observation, err)
	}
	otherCtx := context.WithValue(context.Background(), types.UserIDContextKey, "other")
	if _, err := service.GetRunObservation(otherCtx, "run"); err == nil {
		t.Fatal("GetRunObservation(other) error = nil")
	} else if appErr, ok := err.(*apperrors.AppError); !ok || appErr.Code != apperrors.ErrForbidden {
		t.Fatalf("GetRunObservation(other) error = %v", err)
	}
	adminCtx := context.WithValue(otherCtx, types.SystemAdminContextKey, true)
	if _, err := service.GetRunObservation(adminCtx, "run"); err != nil {
		t.Fatalf("GetRunObservation(admin) error = %v", err)
	}
}

type observationRepository struct {
	run *types.QAExecutionRun
}

func (r *observationRepository) CreateRun(context.Context, *types.QAExecutionRun) error { return nil }
func (r *observationRepository) GetRun(context.Context, string) (*types.QAExecutionRun, error) {
	if r.run == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.run, nil
}
func (r *observationRepository) FinishRun(context.Context, string, types.QARunFinishUpdate) error {
	return nil
}
