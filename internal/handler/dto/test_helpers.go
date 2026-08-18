package dto

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

func adminContext() context.Context {
	return context.WithValue(context.Background(), types.SystemAdminContextKey, true)
}

func viewerContext() context.Context {
	return context.Background()
}

func ownerContext() context.Context {
	return context.WithValue(context.Background(), types.SystemAdminContextKey, true)
}
