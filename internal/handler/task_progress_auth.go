package handler

import (
	"context"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/utils"
)

// requireTaskProgressKnowledgeDomain ensures async task progress endpoints only
// return data for tasks created under the caller's knowledgeDomain. Cross-knowledgeDomain
// probes are hidden as not-found to avoid confirming task existence.
func requireTaskProgressKnowledgeDomain(ctx context.Context, taskID string) error {
	taskKnowledgeDomainID, err := utils.TaskKnowledgeDomainID(taskID)
	if err != nil {
		return apperrors.NewBadRequestError("invalid task ID")
	}
	callerKnowledgeDomainID, ok := types.KnowledgeDomainIDFromContext(ctx)
	if !ok || callerKnowledgeDomainID == 0 {
		return apperrors.NewUnauthorizedError("Unauthorized")
	}
	if taskKnowledgeDomainID != callerKnowledgeDomainID {
		return apperrors.NewNotFoundError("task not found")
	}
	return nil
}
