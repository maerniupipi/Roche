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

// DataSourceCredentialsHandler handles credentials for data source
// connectors via the dedicated /credentials subresource.
//
// Unlike the other three resources (MCP / Model / WebSearch), DataSource
// credentials are a per-connector atomic map — there's no individual-field
// PUT or DELETE because half-configured connector auth doesn't work. So we
// expose a single logical field "credentials": GET returns whether anything
// is stored, PUT replaces the whole map, DELETE wipes it.
type DataSourceCredentialsHandler struct {
	service   interfaces.DataSourceService
	kbService interfaces.KnowledgeBaseService
}

func NewDataSourceCredentialsHandler(
	service interfaces.DataSourceService,
	kbService interfaces.KnowledgeBaseService,
) *DataSourceCredentialsHandler {
	return &DataSourceCredentialsHandler{service: service, kbService: kbService}
}

// ownDataSource is the same knowledgeDomain-isolation check used in datasource.go,
// duplicated here to avoid coupling the two handlers via internal helpers.
func (h *DataSourceCredentialsHandler) ownDataSource(c *gin.Context) (*types.DataSource, bool) {
	ctx := c.Request.Context()
	knowledgeDomainID := c.GetUint64(types.KnowledgeDomainIDContextKey.String())
	if knowledgeDomainID == 0 {
		c.Error(errors.NewBadRequestError("KnowledgeDomain ID cannot be empty"))
		return nil, false
	}
	id := c.Param("id")
	ds, err := h.service.GetDataSource(ctx, id)
	if err != nil || ds == nil {
		c.Error(errors.NewNotFoundError("data source not found"))
		return nil, false
	}
	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, ds.KnowledgeBaseID)
	if err != nil || kb == nil || kb.KnowledgeDomainID != knowledgeDomainID {
		c.Error(errors.NewNotFoundError("data source not found"))
		return nil, false
	}
	return ds, true
}

type dataSourceCredentialsPutRequest struct {
	Credentials map[string]interface{} `json:"credentials" binding:"required"`
}

// Put godoc
// @Summary      Replace data-source credentials
// @Description  Atomically replaces the connector-specific credential map. Secrets are never returned.
// @Tags         DataSource Credentials
// @Accept       json
// @Produce      json
// @Param        id path string true "Data source ID"
// @Param        request body dataSourceCredentialsPutRequest true "Connector credentials"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} errors.AppError
// @Security     Bearer
// @Router       /datasource/{id}/credentials [put]
func (h *DataSourceCredentialsHandler) Put(c *gin.Context) {
	ds, ok := h.ownDataSource(c)
	if !ok {
		return
	}
	var req dataSourceCredentialsPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if len(req.Credentials) == 0 {
		c.Error(errors.NewBadRequestError(
			"credentials map must be non-empty; to remove credentials use DELETE /credentials/credentials"))
		return
	}
	updated, err := h.service.UpdateDataSourceCredentials(c.Request.Context(), ds.ID, req.Credentials)
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, map[string]interface{}{
			"data_source_id": secutils.SanitizeForLog(ds.ID),
		})
		c.Error(errors.NewBadRequestError("failed to update credentials: " + err.Error()))
		return
	}
	configured := false
	if parsed, err := updated.ParseConfig(); err == nil && parsed != nil {
		configured = parsed.HasConfiguredCredentials(updated.Type)
	}
	resp := dto.CredentialsResponse{
		Fields: map[string]dto.CredentialFieldMetadata{
			"credentials": {Configured: configured},
		},
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// DeleteField godoc
// @Summary      Clear data-source credentials
// @Tags         DataSource Credentials
// @Param        id path string true "Data source ID"
// @Param        field path string true "Logical credential field" Enums(credentials)
// @Success      204
// @Failure      400 {object} errors.AppError
// @Security     Bearer
// @Router       /datasource/{id}/credentials/{field} [delete]
func (h *DataSourceCredentialsHandler) DeleteField(c *gin.Context) {
	ds, ok := h.ownDataSource(c)
	if !ok {
		return
	}
	field := c.Param("field")
	if field != "credentials" {
		c.Error(errors.NewBadRequestError("unknown credential field: " + secutils.SanitizeForLog(field)))
		return
	}
	if err := h.service.ClearDataSourceCredentials(c.Request.Context(), ds.ID); err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, map[string]interface{}{
			"data_source_id": secutils.SanitizeForLog(ds.ID),
		})
		c.Error(errors.NewInternalServerError("failed to clear credentials: " + err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}
