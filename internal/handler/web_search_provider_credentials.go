package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/handler/dto"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// WebSearchProviderCredentialsHandler handles credentials for web search
// providers via the dedicated /credentials subresource. Currently the only
// recognized field is "api_key" — every provider that needs credentials uses
// just one key (Bing / Google / Tavily / Baidu), and DuckDuckGo /
// SearXNG don't need credentials at all.
type WebSearchProviderCredentialsHandler struct {
	repo interfaces.WebSearchProviderRepository
	svc  interfaces.WebSearchProviderService
}

func NewWebSearchProviderCredentialsHandler(
	repo interfaces.WebSearchProviderRepository,
	svc interfaces.WebSearchProviderService,
) *WebSearchProviderCredentialsHandler {
	return &WebSearchProviderCredentialsHandler{repo: repo, svc: svc}
}

func (h *WebSearchProviderCredentialsHandler) knowledgeDomainID(c *gin.Context) uint64 {
	return c.GetUint64(types.KnowledgeDomainIDContextKey.String())
}

type webSearchCredentialsPutRequest struct {
	APIKey *string `json:"api_key,omitempty"`
}

// Put godoc
// @Summary      Update web-search provider credentials
// @Description  Writes the API key or returns configured-state metadata when api_key is omitted.
// @Tags         Web Search Credentials
// @Accept       json
// @Produce      json
// @Param        id path string true "Provider ID"
// @Param        request body webSearchCredentialsPutRequest true "Credential field"
// @Success      200 {object} map[string]interface{}
// @Failure      404 {object} errors.AppError
// @Security     Bearer
// @Router       /web-search-providers/{id}/credentials [put]
func (h *WebSearchProviderCredentialsHandler) Put(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID := h.knowledgeDomainID(c)
	if knowledgeDomainID == 0 {
		c.Error(errors.NewBadRequestError("KnowledgeDomain ID cannot be empty"))
		return
	}
	id := c.Param("id")
	var req webSearchCredentialsPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if req.APIKey == nil {
		provider, err := h.repo.GetByID(ctx, knowledgeDomainID, id)
		if err != nil || provider == nil {
			c.Error(errors.NewNotFoundError("web search provider not found"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.CredentialsResponse{
			Fields: map[string]dto.CredentialFieldMetadata{
				"api_key": {Configured: provider.Parameters.APIKey != ""},
			},
		}})
		return
	}
	updated, err := h.svc.UpdateProviderCredentials(ctx, knowledgeDomainID, id, req.APIKey)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"provider_id": secutils.SanitizeForLog(id),
		})
		c.Error(errors.NewInternalServerError("failed to update credentials: " + err.Error()))
		return
	}
	resp := dto.CredentialsResponse{
		Fields: map[string]dto.CredentialFieldMetadata{
			"api_key": {Configured: updated.Parameters.APIKey != ""},
		},
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// DeleteField godoc
// @Summary      Delete a web-search provider credential
// @Tags         Web Search Credentials
// @Param        id path string true "Provider ID"
// @Param        field path string true "Credential field" Enums(api_key)
// @Success      204
// @Failure      400 {object} errors.AppError
// @Security     Bearer
// @Router       /web-search-providers/{id}/credentials/{field} [delete]
func (h *WebSearchProviderCredentialsHandler) DeleteField(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID := h.knowledgeDomainID(c)
	if knowledgeDomainID == 0 {
		c.Error(errors.NewBadRequestError("KnowledgeDomain ID cannot be empty"))
		return
	}
	id := c.Param("id")
	field := c.Param("field")
	if field != "api_key" {
		c.Error(errors.NewBadRequestError("unknown credential field: " + secutils.SanitizeForLog(field)))
		return
	}
	if err := h.svc.ClearProviderCredential(ctx, knowledgeDomainID, id, field); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"provider_id": secutils.SanitizeForLog(id),
			"field":       field,
		})
		c.Error(errors.NewInternalServerError("failed to clear credential: " + err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}
