package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/datasource"
	"roche.local/knowledge-agent-platform/internal/handler/dto"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// DataSourceHandler handles HTTP requests for data source management
type DataSourceHandler struct {
	service   interfaces.DataSourceService
	kbService interfaces.KnowledgeBaseService
}

// NewDataSourceHandler creates a new data source handler
func NewDataSourceHandler(
	service interfaces.DataSourceService,
	kbService interfaces.KnowledgeBaseService,
) *DataSourceHandler {
	return &DataSourceHandler{
		service:   service,
		kbService: kbService,
	}
}

// getKnowledgeDomainID safely extracts and validates knowledgeDomain ID from context
// Returns 0 if knowledgeDomain ID is not found (caller should return 401)
func (h *DataSourceHandler) getKnowledgeDomainID(c *gin.Context) uint64 {
	knowledgeDomainID := c.GetUint64(types.KnowledgeDomainIDContextKey.String())
	return knowledgeDomainID
}

// requireKnowledgeDomainID returns the knowledgeDomainID from context, allowing
// system admins to bypass the check when no domain is set. Returns (0, false) when
// the caller should abort with 401.
func (h *DataSourceHandler) requireKnowledgeDomainID(c *gin.Context) (uint64, bool) {
	knowledgeDomainID := h.getKnowledgeDomainID(c)
	if knowledgeDomainID == 0 {
		if !types.IsSystemAdminFromContext(c.Request.Context()) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return 0, false
		}
	}
	return knowledgeDomainID, true
}

// Data source settings contain connector credentials, so only the owning knowledgeDomain can access them.
// System admins bypass domain ownership checks when no knowledgeDomainID is set on the context.
func (h *DataSourceHandler) getOwnedKnowledgeBase(
	ctx context.Context,
	knowledgeDomainID uint64,
	kbID string,
) (*types.KnowledgeBase, int, string) {
	if kbID == "" {
		// System admins operating outside a domain context may omit the kb_id.
		if knowledgeDomainID == 0 && types.IsSystemAdminFromContext(ctx) {
			return nil, http.StatusOK, ""
		}
		return nil, http.StatusBadRequest, "kb_id is required"
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil || kb == nil {
		return nil, http.StatusNotFound, "knowledge base not found"
	}

	// System admins operating without a domain context can access any KB.
	if knowledgeDomainID == 0 {
		if types.IsSystemAdminFromContext(ctx) {
			return kb, http.StatusOK, ""
		}
		return nil, http.StatusForbidden, "access denied"
	}

	if kb.KnowledgeDomainID != knowledgeDomainID {
		return nil, http.StatusForbidden, "access denied"
	}
	return kb, http.StatusOK, ""
}

func (h *DataSourceHandler) getOwnedDataSource(
	ctx context.Context,
	knowledgeDomainID uint64,
	id string,
) (*types.DataSource, int, string) {
	ds, err := h.service.GetDataSource(ctx, id)
	if err != nil {
		return nil, http.StatusNotFound, "data source not found"
	}

	// System admins operating without a domain context bypass ownership checks.
	if knowledgeDomainID == 0 && types.IsSystemAdminFromContext(ctx) {
		return ds, http.StatusOK, ""
	}

	if _, status, msg := h.getOwnedKnowledgeBase(ctx, knowledgeDomainID, ds.KnowledgeBaseID); status != http.StatusOK {
		return nil, status, msg
	}

	return ds, http.StatusOK, ""
}

// CreateDataSource godoc
// @Summary Create a new data source
// @Description Create a new data source configuration for a knowledge base
// @Tags DataSource
// @Accept json
// @Produce json
// @Param request body types.DataSource true "Data source configuration"
// @Success 201 {object} types.DataSource
// @Failure 400 {object} map[string]string
// @Router /datasource [post]
func (h *DataSourceHandler) CreateDataSource(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract knowledgeDomain ID from context (set by auth middleware)
	knowledgeDomainID := c.GetUint64(types.KnowledgeDomainIDContextKey.String())
	if knowledgeDomainID == 0 {
		if !types.IsSystemAdminFromContext(ctx) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: knowledgeDomain context missing"})
			return
		}
	}

	var req types.DataSource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// System admins operating outside a domain context: fall back to
	// the first available knowledge base so the data source can be created.
	if knowledgeDomainID == 0 && req.KnowledgeBaseID == "" {
		kbs, err := h.kbService.ListKnowledgeBases(ctx)
		if err == nil && len(kbs) > 0 {
			knowledgeDomainID = kbs[0].KnowledgeDomainID
			req.KnowledgeBaseID = kbs[0].ID
		}
	}

	if _, status, msg := h.getOwnedKnowledgeBase(ctx, knowledgeDomainID, req.KnowledgeBaseID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	// Enforce knowledgeDomain isolation
	req.KnowledgeDomainID = knowledgeDomainID

	ds, err := h.service.CreateDataSource(ctx, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dto.NewDataSourceResponse(ds))
}

// GetDataSource godoc
// @Summary Get a data source by ID
// @Description Retrieve a data source configuration by ID
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} types.DataSource
// @Failure 404 {object} map[string]string
// @Router /datasource/{id} [get]
func (h *DataSourceHandler) GetDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	ds, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, dto.NewDataSourceResponse(ds))
}

// ListDataSources godoc
// @Summary List data sources for a knowledge base
// @Description List all data sources for a specific knowledge base
// @Tags DataSource
// @Produce json
// @Param kb_id query string true "Knowledge base ID"
// @Success 200 {object} []types.DataSource
// @Failure 400 {object} map[string]string
// @Router /datasource [get]
func (h *DataSourceHandler) ListDataSources(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract knowledgeDomain ID from context
	knowledgeDomainID := c.GetUint64(types.KnowledgeDomainIDContextKey.String())
	if knowledgeDomainID == 0 {
		if !types.IsSystemAdminFromContext(ctx) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: knowledgeDomain context missing"})
			return
		}
	}

	kbID := c.Query("kb_id")
	// System admins operating outside a domain context: fall back to
	// the first available knowledge base.
	if knowledgeDomainID == 0 && kbID == "" {
		kbs, err := h.kbService.ListKnowledgeBases(ctx)
		if err == nil && len(kbs) > 0 {
			knowledgeDomainID = kbs[0].KnowledgeDomainID
			kbID = kbs[0].ID
		}
	}
	if _, status, msg := h.getOwnedKnowledgeBase(ctx, knowledgeDomainID, kbID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}
	dataSources, err := h.service.ListDataSources(ctx, kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list data sources"})
		return
	}

	if dataSources == nil {
		dataSources = make([]*types.DataSource, 0)
	}
	c.JSON(http.StatusOK, dto.NewDataSourceResponses(dataSources))
}

// UpdateDataSource godoc
// @Summary Update a data source
// @Description Update an existing data source configuration
// @Tags DataSource
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param request body types.DataSource true "Updated configuration"
// @Success 200 {object} types.DataSource
// @Failure 400 {object} map[string]string
// @Router /datasource/{id} [put]
func (h *DataSourceHandler) UpdateDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	var req types.DataSource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	existing, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	req.ID = id
	req.KnowledgeDomainID = existing.KnowledgeDomainID
	req.KnowledgeBaseID = existing.KnowledgeBaseID
	ds, err := h.service.UpdateDataSource(ctx, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.NewDataSourceResponse(ds))
}

// DeleteDataSource godoc
// @Summary Delete a data source
// @Description Delete a data source (soft delete)
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 204
// @Failure 404 {object} map[string]string
// @Router /datasource/{id} [delete]
func (h *DataSourceHandler) DeleteDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.DeleteDataSource(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete data source"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ValidateConnection godoc
// @Summary Test data source connection
// @Description Validate the connection to an external data source
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/validate [post]
func (h *DataSourceHandler) ValidateConnection(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.ValidateConnection(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "connected"})
}

// ValidateCredentials godoc
// @Summary Test connection with raw credentials (no persistence)
// @Description Validate connectivity to an external data source using type + credentials
//
//	without creating or updating any database records.
//	Used by the frontend "Test Connection" button during data source creation.
//
// @Tags DataSource
// @Accept json
// @Produce json
// @Param request body object true "type and credentials"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /datasource/validate-credentials [post]
func (h *DataSourceHandler) ValidateCredentials(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID := h.getKnowledgeDomainID(c)
	if knowledgeDomainID == 0 {
		// System admins can validate credentials without a knowledge domain context.
		// The validate-credentials endpoint is already guarded by the Admin middleware.
		if !types.IsSystemAdminFromContext(ctx) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	var req struct {
		Type        string                 `json:"type" binding:"required"`
		Credentials map[string]interface{} `json:"credentials" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: type and credentials are required"})
		return
	}

	if err := h.service.ValidateCredentials(ctx, req.Type, req.Credentials); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "connected"})
}

// @Summary List available resources in data source
// @Description List resources available for sync in the external system. Pass parent_id to lazily load the direct children of a resource (used for large hierarchical sources such as Feishu wiki).
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Param parent_id query string false "Parent resource ExternalID; empty lists the top level"
// @Success 200 {object} []types.Resource
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/resources [get]
func (h *DataSourceHandler) ListAvailableResources(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	parentID := c.Query("parent_id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	resources, err := h.service.ListAvailableResources(ctx, id, parentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if resources == nil {
		resources = make([]types.Resource, 0)
	}
	c.JSON(http.StatusOK, resources)
}

// @Summary Resolve resource ancestors
// @Description Resolve the ancestor ExternalIDs that must be expanded to reveal the given (possibly deeply nested) resources in a lazily-loaded picker. Used to restore an existing selection when editing a data source.
// @Tags DataSource
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param request body resolveAncestorsRequest true "Resource IDs to resolve"
// @Success 200 {object} map[string][]string
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/resource-ancestors [post]
func (h *DataSourceHandler) ResolveResourceAncestors(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	var req resolveAncestorsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ancestors, err := h.service.ResolveResourceAncestors(ctx, id, req.ResourceIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if ancestors == nil {
		ancestors = make([]string, 0)
	}
	c.JSON(http.StatusOK, gin.H{"ancestors": ancestors})
}

// resolveAncestorsRequest is the body for ResolveResourceAncestors.
type resolveAncestorsRequest struct {
	ResourceIDs []string `json:"resource_ids"`
}

// ManualSync godoc
// @Summary Trigger immediate sync
// @Description Trigger an immediate sync for a data source
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} types.SyncLog
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/sync [post]
func (h *DataSourceHandler) ManualSync(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	// The sync endpoint is open by design so the system can trigger it
	// daily at 00:00 without a user session. Authenticated users still go
	// through the normal domain-ownership check; unauthenticated
	// (system-triggered) calls only require the data source to exist.
	if _, ok := types.UserIDFromContext(ctx); ok {
		knowledgeDomainID := h.getKnowledgeDomainID(c)
		if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
			c.JSON(status, gin.H{"error": msg})
			return
		}
	} else if _, err := h.service.GetDataSource(ctx, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "data source not found"})
		return
	}

	syncLog, err := h.service.ManualSync(ctx, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, syncLog)
}

// PauseDataSource godoc
// @Summary Pause data source
// @Description Pause a data source's scheduled syncs
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/pause [post]
func (h *DataSourceHandler) PauseDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.PauseDataSource(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "paused"})
}

// ResumeDataSource godoc
// @Summary Resume data source
// @Description Resume a paused data source
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/resume [post]
func (h *DataSourceHandler) ResumeDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.ResumeDataSource(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "active"})
}

// GetSyncLogs godoc
// @Summary Get sync logs
// @Description Retrieve sync history for a data source
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Param limit query int false "Limit (default: 10)"
// @Param offset query int false "Offset (default: 0)"
// @Success 200 {object} []types.SyncLog
// @Failure 400 {object} map[string]string
// @Router /datasource/{id}/logs [get]
func (h *DataSourceHandler) GetSyncLogs(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	limit := 10
	offset := 0

	if l := c.Query("limit"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil || v <= 0 || v > maxListPageSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and " + strconv.Itoa(maxListPageSize)})
			return
		}
		limit = v
	}

	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	logs, err := h.service.GetSyncLogs(ctx, id, limit, offset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if logs == nil {
		logs = make([]*types.SyncLog, 0)
	}
	c.JSON(http.StatusOK, logs)
}

// GetSyncLog godoc
// @Summary Get specific sync log
// @Description Retrieve a specific sync log entry
// @Tags DataSource
// @Produce json
// @Param log_id path string true "Sync log ID"
// @Success 200 {object} types.SyncLog
// @Failure 404 {object} map[string]string
// @Router /datasource/logs/{log_id} [get]
func (h *DataSourceHandler) GetSyncLog(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, ok := h.requireKnowledgeDomainID(c)
	if !ok {
		return
	}

	logID := c.Param("log_id")

	log, err := h.service.GetSyncLog(ctx, logID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync log not found"})
		return
	}

	if _, status, msg := h.getOwnedDataSource(ctx, knowledgeDomainID, log.DataSourceID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetAvailableConnectors godoc
// @Summary Get available connectors
// @Description Get list of available data source connectors
// @Tags DataSource
// @Produce json
// @Success 200 {object} []datasource.ConnectorMetadata
// @Router /datasource/types [get]
func (h *DataSourceHandler) GetAvailableConnectors(c *gin.Context) {
	connectors := datasource.ListAvailableConnectors()
	c.JSON(http.StatusOK, connectors)
}
