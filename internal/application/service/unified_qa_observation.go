package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type unifiedQAObservationService struct {
	repository interfaces.UnifiedQARunRepository
}

func NewUnifiedQAObservationService(
	repository interfaces.UnifiedQARunRepository,
) interfaces.UnifiedQAObservationService {
	return &unifiedQAObservationService{repository: repository}
}

func (s *unifiedQAObservationService) GetRunObservation(ctx context.Context, runID string) (*types.QARunObservation, error) {
	run, err := s.getAuthorizedRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return &types.QARunObservation{Run: run}, nil
}

func (s *unifiedQAObservationService) getAuthorizedRun(ctx context.Context, runID string) (*types.QAExecutionRun, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("unified QA observation repository is not configured")
	}
	if strings.TrimSpace(runID) == "" {
		return nil, apperrors.NewValidationError("run id is required")
	}
	run, err := s.repository.GetRun(ctx, runID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NewNotFoundError("unified QA run not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get unified QA run: %w", err)
	}
	caller := types.SessionOwnerIDFromContext(ctx)
	if caller == "" {
		caller, _ = types.UserIDFromContext(ctx)
	}
	if !types.IsSystemAdminFromContext(ctx) && (caller == "" || run.UserID != caller) {
		return nil, apperrors.NewForbiddenError("unified QA run is outside your access scope")
	}
	return run, nil
}
