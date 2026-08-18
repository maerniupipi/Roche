package dto

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// CanViewIntegrationSecrets is restricted to platform administrators because
// model, storage, parser and external-integration configuration is global.
func CanViewIntegrationSecrets(ctx context.Context) bool {
	return types.IsSystemAdminFromContext(ctx)
}
