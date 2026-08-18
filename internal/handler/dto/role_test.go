package dto

import (
	"context"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestCanViewIntegrationSecretsAdminRole(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.SystemAdminContextKey, true)
	if !CanViewIntegrationSecrets(ctx) {
		t.Fatal("system administrator should view integration secrets")
	}
}

func TestCanViewIntegrationSecretsViewerDenied(t *testing.T) {
	if CanViewIntegrationSecrets(context.Background()) {
		t.Fatal("regular user should not view integration secrets")
	}
}

func TestCanViewIntegrationSecretsSystemAdmin(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.SystemAdminContextKey, true)
	if !CanViewIntegrationSecrets(ctx) {
		t.Fatal("system administrator should view integration secrets")
	}
}
