package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestKnowledgeDomainResponse_ViewerOmitsSecrets(t *testing.T) {
	knowledgeDomain := sampleSecretKnowledgeDomain()
	body, err := json.Marshal(NewKnowledgeDomainResponse(viewerContext(), knowledgeDomain))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "legacy-search-secret-999")
	assert.NotContains(t, s, "wk-app-secret-def")
	assert.NotContains(t, s, "parser-secret-123")
	assert.NotContains(t, s, "minio-secret-789")
	assert.NotContains(t, s, "web_search_config")
	assert.NotContains(t, s, "parser_engine_config")
	assert.NotContains(t, s, "storage_engine_config")
	assert.NotContains(t, s, "credentials")
}

func TestKnowledgeDomainResponse_AdminOmitsPlatformRuntimeConfig(t *testing.T) {
	knowledgeDomain := sampleSecretKnowledgeDomain()
	body, err := json.Marshal(NewKnowledgeDomainResponse(ownerContext(), knowledgeDomain))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "legacy-search-secret-999")
	assert.NotContains(t, s, "parser-secret-123")
	assert.NotContains(t, s, "web_search_config")
	assert.NotContains(t, s, "parser_engine_config")
	assert.NotContains(t, s, "storage_engine_config")
}

func TestKnowledgeDomainResponse_AdminUsesDedicatedRuntimeConfigEndpoint(t *testing.T) {
	knowledgeDomain := sampleSecretKnowledgeDomain()
	body, err := json.Marshal(NewKnowledgeDomainResponse(adminContext(), knowledgeDomain))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "web_search_config")
	assert.NotContains(t, s, "parser_engine_config")
	assert.NotContains(t, s, "storage_engine_config")
}

func TestKnowledgeDomainResponsesNeverExposeRuntimeConfiguration(t *testing.T) {
	knowledgeDomain := sampleSecretKnowledgeDomain()
	body, err := json.Marshal(NewKnowledgeDomainResponses(viewerContext(), []*types.KnowledgeDomain{knowledgeDomain}))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "parser-secret-123")
}

func sampleSecretKnowledgeDomain() *types.KnowledgeDomain {
	return &types.KnowledgeDomain{
		ID:   42,
		Name: "knowledgeDomain",
		WebSearchConfig: &types.WebSearchConfig{
			APIKey:   "legacy-search-secret-999",
			ProxyURL: "http://proxy.internal:8080",
		},
		ParserEngineConfig: &types.ParserEngineConfig{
			MinerUAPIKey:          "parser-secret-123",
			PaddleOCRVLCloudToken: "paddle-secret-456",
		},
		StorageEngineConfig: &types.StorageEngineConfig{
			DefaultProvider: "minio",
			MinIO: &types.MinIOEngineConfig{
				AccessKeyID:     "minio-access-id",
				SecretAccessKey: "minio-secret-789",
			},
		},
	}
}
