package datasource

import (
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestFeishuMetadataDoesNotAdvertiseWebhook(t *testing.T) {
	meta := ConnectorMetadataRegistry[types.ConnectorTypeFeishu]

	for _, capability := range meta.Capabilities {
		if capability == "webhook" {
			t.Fatalf("Feishu connector should not advertise webhook until webhook sync is implemented")
		}
	}
}

func TestGoogleDriveMetadataMatchesImplementation(t *testing.T) {
	meta := ConnectorMetadataRegistry[types.ConnectorTypeGoogleDrive]
	if meta.AuthType != "service_account" {
		t.Fatalf("Google Drive auth type = %q, want service_account", meta.AuthType)
	}

	capabilities := make(map[string]bool)
	for _, capability := range meta.Capabilities {
		capabilities[capability] = true
	}
	if !capabilities["incremental"] || !capabilities["deletion_sync"] {
		t.Fatalf("Google Drive capabilities = %#v", meta.Capabilities)
	}
}
