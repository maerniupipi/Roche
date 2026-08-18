package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestModelResponse_OmitsSecrets(t *testing.T) {
	m := &types.Model{
		ID:          "m-1",
		Name:        "gpt-x",
		DisplayName: "Support QA",
		Parameters: types.ModelParameters{
			APIKey:    "sk-real-api-key-do-not-leak",
			AppSecret: "app-real-secret-do-not-leak",
			BaseURL:   "https://api.example.com",
			Provider:  "openai",
		},
	}
	body, err := json.Marshal(NewModelResponse(adminContext(), m))
	assert.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "sk-real-api-key-do-not-leak")
	assert.NotContains(t, s, "app-real-secret-do-not-leak")
	// Parameters sub-object must contain no secret keys.
	var raw map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(body, &raw))
	params := string(raw["parameters"])
	assert.NotContains(t, params, `"api_key"`)
	assert.NotContains(t, params, `"app_secret"`)
	// Credential metadata map exposes booleans only.
	assert.Contains(t, s, `"credentials"`)
	assert.Contains(t, s, `"api_key":{"configured":true}`)
	assert.Contains(t, s, `"app_secret":{"configured":true}`)
	// Non-secret fields pass through.
	assert.Contains(t, s, "api.example.com")
	assert.Contains(t, s, `"display_name":"Support QA"`)
}

func TestModelResponse_BuiltinStripsKnowledgeDomainConfig(t *testing.T) {
	m := &types.Model{
		ID:        "builtin-1",
		IsBuiltin: true,
		Parameters: types.ModelParameters{
			BaseURL:        "https://knowledgeDomain-private.example.com",
			APIKey:         "should-not-leak",
			SupportsVision: true,
			ExtraConfig:    map[string]string{"region": "cn-hangzhou"},
		},
	}
	resp := NewModelResponse(adminContext(), m)
	assert.Empty(t, resp.Parameters.BaseURL,
		"builtin must not leak per-knowledgeDomain base URL")
	assert.Nil(t, resp.Parameters.ExtraConfig,
		"builtin must not leak per-knowledgeDomain extra_config")
	assert.True(t, resp.Parameters.SupportsVision,
		"capability metadata must survive (not per-knowledgeDomain)")

	body, _ := json.Marshal(resp)
	assert.False(t, strings.Contains(string(body), "should-not-leak"))
	assert.False(t, strings.Contains(string(body), "knowledgeDomain-private.example.com"))
}

func TestModelResponse_ViewerStripsIntegrationDetail(t *testing.T) {
	m := &types.Model{
		ID: "m-2",
		Parameters: types.ModelParameters{
			BaseURL:       "https://knowledgeDomain-private.example.com",
			CustomHeaders: map[string]string{"Authorization": "Bearer secret"},
			ExtraConfig:   map[string]string{"region": "cn-hangzhou"},
		},
	}
	resp := NewModelResponse(viewerContext(), m)
	assert.Empty(t, resp.Parameters.BaseURL)
	assert.Nil(t, resp.Parameters.CustomHeaders)
	assert.Nil(t, resp.Parameters.ExtraConfig)
}

func TestModelResponse_NilSafe(t *testing.T) {
	assert.Nil(t, NewModelResponse(adminContext(), nil))
	assert.Equal(t, []*ModelResponse{}, NewModelResponses(adminContext(), nil))
}
