package service

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

func auditActor(ctx context.Context) string {
	userID, _ := types.UserIDFromContext(ctx)
	return userID
}

// auditActorName resolves the actor's display name (user.Username) from
// ctx. Returns "" when no hydrated *types.User is present (e.g. login
// failure paths, internal-service identity) — callers may overwrite it.
func auditActorName(ctx context.Context) string {
	return types.ActorNameFromContext(ctx)
}
