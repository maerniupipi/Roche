package dto

import (
	"context"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
)

// ModelResponse mirrors types.Model for response bodies, with all secret
// fields (APIKey, AppSecret) removed by construction. Credential presence
// metadata lives behind the /credentials subresource, not inlined here.
//
// BaseURL is preserved for knowledgeDomain-owned models (the frontend needs it to
// render which endpoint a custom model points at). For builtin models it is
// stripped along with every other field that could leak how a particular
// knowledgeDomain configured the upstream provider.
type ModelResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name"`
	Type        types.ModelType    `json:"type"`
	Source      types.ModelSource  `json:"source"`
	Description string             `json:"description"`
	Parameters  ModelParametersDTO `json:"parameters"`
	IsDefault   bool               `json:"is_default"`
	IsBuiltin   bool               `json:"is_builtin"`
	Status      types.ModelStatus  `json:"status"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	// Per-field "configured?" map. Omitted for builtin models (no
	// per-knowledgeDomain credentials). See MCPServiceResponse.Credentials.
	Credentials map[string]CredentialFieldMetadata `json:"credentials,omitempty"`
}

// ModelParametersDTO carries every parameter field EXCEPT the two secret
// ones (APIKey, AppSecret). CustomHeaders is retained as structural metadata.
type ModelParametersDTO struct {
	BaseURL             string                    `json:"base_url"`
	InterfaceType       string                    `json:"interface_type"`
	EmbeddingParameters types.EmbeddingParameters `json:"embedding_parameters"`
	Provider            string                    `json:"provider"`
	ExtraConfig         map[string]string         `json:"extra_config,omitempty"`
	CustomHeaders       map[string]string         `json:"custom_headers,omitempty"`
	SupportsVision      bool                      `json:"supports_vision"`
}

// NewModelResponse converts a stored Model into its response shape.
//
// Builtin models are shared across knowledgeDomains — strip BaseURL (which can leak
// the knowledgeDomain's private endpoint) and any non-shared parameters.
func NewModelResponse(ctx context.Context, m *types.Model) *ModelResponse {
	if m == nil {
		return nil
	}
	params := ModelParametersDTO{
		BaseURL:             m.Parameters.BaseURL,
		InterfaceType:       m.Parameters.InterfaceType,
		EmbeddingParameters: m.Parameters.EmbeddingParameters,
		Provider:            m.Parameters.Provider,
		ExtraConfig:         m.Parameters.ExtraConfig,
		CustomHeaders:       m.Parameters.CustomHeaders,
		SupportsVision:      m.Parameters.SupportsVision,
	}
	if !CanViewIntegrationSecrets(ctx) {
		params.ExtraConfig = nil
		params.CustomHeaders = nil
		params.BaseURL = ""
	}
	if m.IsBuiltin {
		// Builtin: strip everything that could reveal per-knowledgeDomain config.
		// EmbeddingParameters / Provider / InterfaceType /
		// SupportsVision are intentionally preserved (they describe the
		// capability surface, not the configured endpoint).
		params.BaseURL = ""
		params.ExtraConfig = nil
		params.CustomHeaders = nil
	}
	var creds map[string]CredentialFieldMetadata
	if !m.IsBuiltin {
		creds = map[string]CredentialFieldMetadata{
			"api_key":    {Configured: m.Parameters.APIKey != ""},
			"app_secret": {Configured: m.Parameters.AppSecret != ""},
		}
	}
	return &ModelResponse{
		ID:          m.ID,
		Name:        m.Name,
		DisplayName: m.DisplayName,
		Type:        m.Type,
		Source:      m.Source,
		Description: m.Description,
		Parameters:  params,
		IsDefault:   m.IsDefault,
		IsBuiltin:   m.IsBuiltin,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		Credentials: creds,
	}
}

// NewModelResponses is the slice convenience wrapper.
func NewModelResponses(ctx context.Context, models []*types.Model) []*ModelResponse {
	out := make([]*ModelResponse, 0, len(models))
	for _, m := range models {
		out = append(out, NewModelResponse(ctx, m))
	}
	return out
}
