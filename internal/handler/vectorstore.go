package handler

import (
	"context"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// VectorStoreHandler handles HTTP requests for vector store CRUD
type VectorStoreHandler struct {
	repo    interfaces.VectorStoreRepository
	service interfaces.VectorStoreService
}

// NewVectorStoreHandler creates a new handler
func NewVectorStoreHandler(
	repo interfaces.VectorStoreRepository,
	service interfaces.VectorStoreService,
) *VectorStoreHandler {
	return &VectorStoreHandler{repo: repo, service: service}
}

// --- request DTOs ---

// CreateStoreRequest defines the request body for creating a vector store
type CreateStoreRequest struct {
	Name             string                    `json:"name" binding:"required"`
	EngineType       types.RetrieverEngineType `json:"engine_type" binding:"required"`
	ConnectionConfig types.ConnectionConfig    `json:"connection_config" binding:"required"`
	IndexConfig      types.IndexConfig         `json:"index_config"`
}

// UpdateStoreRequest defines the request body for updating a vector store.
// Only name is mutable — engine_type, connection_config, index_config are immutable.
type UpdateStoreRequest struct {
	Name string `json:"name" binding:"required"`
}

// TestStoreRequest defines the body for testing raw credentials
type TestStoreRequest struct {
	EngineType       types.RetrieverEngineType `json:"engine_type" binding:"required"`
	ConnectionConfig types.ConnectionConfig    `json:"connection_config" binding:"required"`
}

// VectorStoreOption is the credential-free projection exposed to knowledge
// base editors. Connection and index settings remain system-admin only.
type VectorStoreOption struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	EngineType string `json:"engine_type"`
	Source     string `json:"source"`
	Status     string `json:"status"`
}

// --- helpers ---

// getStore loads a global vector store.
// Returns (nil, status, msg) on failure so callers can respond immediately.
func (h *VectorStoreHandler) getStore(ctx context.Context, id string) (*types.VectorStore, int, string) {
	store, err := h.repo.GetByID(ctx, 0, id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to query vector store"
	}
	if store == nil {
		return nil, http.StatusNotFound, "vector store not found"
	}
	return store, http.StatusOK, ""
}

// envStoreReadonlyError returns a 400 error for attempting to modify env stores.
func envStoreReadonlyError() gin.H {
	return gin.H{"success": false, "error": "environment-configured vector stores cannot be modified via API"}
}

// --- endpoints ---

// CreateStore godoc
// @Summary      Create vector store
// @Description  Create a new platform-wide vector store configuration
// @Tags         VectorStore
// @Accept       json
// @Produce      json
// @Param        request  body      CreateStoreRequest       true  "Vector store configuration"
// @Success      201      {object}  map[string]interface{}   "Created vector store"
// @Failure      400      {object}  errors.AppError          "Invalid request or validation error"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      409      {object}  errors.AppError          "Duplicate endpoint and index"
// @Security     Bearer
// @Router       /vector-stores [post]
func (h *VectorStoreHandler) CreateStore(c *gin.Context) {
	ctx := c.Request.Context()

	var req CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warnf(ctx, "Invalid create vector store request: %v", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	store := &types.VectorStore{
		Name:             req.Name,
		EngineType:       req.EngineType,
		ConnectionConfig: req.ConnectionConfig,
		IndexConfig:      req.IndexConfig,
	}

	if err := h.service.CreateStore(ctx, store); err != nil {
		logger.Warnf(ctx, "Failed to create vector store: %v", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    types.NewVectorStoreResponse(store, "user", false),
	})
}

// ListStores godoc
// @Summary      List vector stores
// @Description  List all platform vector stores, including environment-configured and user-created stores
// @Tags         VectorStore
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of vector stores (env + DB)"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Router       /vector-stores [get]
func (h *VectorStoreHandler) ListStores(c *gin.Context) {
	ctx := c.Request.Context()

	dbStores, err := h.repo.List(ctx, 0)
	if err != nil {
		logger.Warnf(ctx, "Failed to list vector stores: %v", err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// DB stores → VectorStoreResponse (masked)
	maskedDBStores := make([]types.VectorStoreResponse, len(dbStores))
	for i, s := range dbStores {
		maskedDBStores[i] = types.NewVectorStoreResponse(s, "user", false)
	}

	// env stores → VectorStore → VectorStoreResponse (masked)
	envStores := types.BuildEnvVectorStores(os.Getenv("RETRIEVE_DRIVER"), os.Getenv)
	maskedEnvStores := make([]types.VectorStoreResponse, len(envStores))
	for i := range envStores {
		maskedEnvStores[i] = types.NewVectorStoreResponse(&envStores[i], "env", true)
	}

	// Merge: env stores first, then DB stores
	allStores := append(maskedEnvStores, maskedDBStores...)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": allStores})
}

// ListStoreOptions godoc
// @Summary      List vector store options
// @Description  List credential-free platform vector store options for knowledge base selection
// @Tags         VectorStore
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "Safe vector store options"
// @Failure      401  {object}  map[string]interface{}  "Unauthorized"
// @Security     Bearer
// @Router       /vector-stores/options [get]
func (h *VectorStoreHandler) ListStoreOptions(c *gin.Context) {
	ctx := c.Request.Context()

	dbStores, err := h.repo.List(ctx, 0)
	if err != nil {
		logger.Warnf(ctx, "Failed to list vector store options: %v", err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	envStores := types.BuildEnvVectorStores(os.Getenv("RETRIEVE_DRIVER"), os.Getenv)
	options := make([]VectorStoreOption, 0, len(envStores)+len(dbStores))
	for i := range envStores {
		options = append(options, VectorStoreOption{
			ID:         envStores[i].ID,
			Name:       envStores[i].Name,
			EngineType: string(envStores[i].EngineType),
			Source:     types.StoreSourceEnv,
			Status:     "available",
		})
	}
	for _, store := range dbStores {
		options = append(options, VectorStoreOption{
			ID:         store.ID,
			Name:       store.Name,
			EngineType: string(store.EngineType),
			Source:     types.StoreSourceUser,
			Status:     "available",
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": options})
}

// GetStore godoc
// @Summary      Get vector store
// @Description  Retrieve a single vector store by ID. Supports both DB stores and env stores (__env_* IDs)
// @Tags         VectorStore
// @Produce      json
// @Param        id   path      string  true  "Vector store ID"
// @Success      200  {object}  map[string]interface{}   "Vector store details"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Router       /vector-stores/{id} [get]
func (h *VectorStoreHandler) GetStore(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	// Handle env store
	if types.IsEnvStoreID(id) {
		envStore := types.FindEnvVectorStore(os.Getenv("RETRIEVE_DRIVER"), os.Getenv, id)
		if envStore == nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vector store not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    types.NewVectorStoreResponse(envStore, "env", true),
		})
		return
	}

	// DB store
	store, status, msg := h.getStore(ctx, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.NewVectorStoreResponse(store, "user", false),
	})
}

// UpdateStore godoc
// @Summary      Update vector store
// @Description  Update a vector store (name only). Engine type, connection config, and index config are immutable. Env stores cannot be modified.
// @Tags         VectorStore
// @Accept       json
// @Produce      json
// @Param        id       path      string               true  "Vector store ID"
// @Param        request  body      UpdateStoreRequest   true  "Updated fields"
// @Success      200      {object}  map[string]interface{}   "Updated vector store"
// @Failure      400      {object}  map[string]interface{}   "Env store or validation error"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      404      {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Router       /vector-stores/{id} [put]
func (h *VectorStoreHandler) UpdateStore(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	// env stores are read-only
	if types.IsEnvStoreID(id) {
		c.JSON(http.StatusBadRequest, envStoreReadonlyError())
		return
	}

	// Ownership check
	if _, status, msg := h.getStore(ctx, id); status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	var req UpdateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	updated := &types.VectorStore{
		ID:   id,
		Name: req.Name,
	}

	if err := h.service.UpdateStore(ctx, updated); err != nil {
		logger.Warnf(ctx, "Failed to update vector store %s: %v", id, err)
		c.Error(err)
		return
	}

	// Re-fetch to return full state
	result, err := h.repo.GetByID(ctx, 0, id)
	if err != nil {
		logger.Warnf(ctx, "Failed to re-fetch vector store %s after update: %v", id, err)
	}
	if result != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    types.NewVectorStoreResponse(result, "user", false),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
	}
}

// DeleteStore godoc
// @Summary      Delete vector store
// @Description  Soft-delete a vector store. Env stores cannot be deleted.
// @Tags         VectorStore
// @Produce      json
// @Param        id   path      string  true  "Vector store ID"
// @Success      200  {object}  map[string]interface{}   "Deletion success"
// @Failure      400  {object}  map[string]interface{}   "Env store cannot be deleted"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Router       /vector-stores/{id} [delete]
func (h *VectorStoreHandler) DeleteStore(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	// env stores are read-only
	if types.IsEnvStoreID(id) {
		c.JSON(http.StatusBadRequest, envStoreReadonlyError())
		return
	}

	// Ownership check
	if _, status, msg := h.getStore(ctx, id); status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	if err := h.service.DeleteStore(ctx, 0, id); err != nil {
		logger.Warnf(ctx, "Failed to delete vector store %s: %v", id, err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListStoreTypes godoc
// @Summary      List vector store types
// @Description  Return supported engine types with connection and index field schemas for UI form generation
// @Tags         VectorStore
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of engine types with config schemas"
// @Router       /vector-stores/types [get]
func (h *VectorStoreHandler) ListStoreTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.GetVectorStoreTypes(),
	})
}

// TestStoreByID godoc
// @Summary      Test vector store connection by ID
// @Description  Test connectivity of an existing saved or env store. Returns detected server version. For DB stores, the version is automatically saved to connection_config.
// @Tags         VectorStore
// @Produce      json
// @Param        id   path      string  true  "Vector store ID"
// @Success      200  {object}  map[string]interface{}   "Connection test result (success, version)"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Router       /vector-stores/{id}/test [post]
func (h *VectorStoreHandler) TestStoreByID(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	// env store — test with unmasked config
	if types.IsEnvStoreID(id) {
		envStore := types.FindEnvVectorStore(os.Getenv("RETRIEVE_DRIVER"), os.Getenv, id)
		if envStore == nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vector store not found"})
			return
		}
		version, err := h.service.TestConnection(ctx, envStore.EngineType, envStore.ConnectionConfig)
		if err != nil {
			logger.Warnf(ctx, "Vector store connection test failed: %v", err)
			c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
		return
	}

	// DB store
	store, status, msg := h.getStore(ctx, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	version, err := h.service.TestConnection(ctx, store.EngineType, store.ConnectionConfig)
	if err != nil {
		logger.Warnf(ctx, "Vector store connection test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Update stored version if detected
	if version != "" && version != store.ConnectionConfig.Version {
		if updateErr := h.service.SaveDetectedVersion(ctx, store, version); updateErr != nil {
			logger.Warnf(ctx, "Failed to update detected version for store %s: %v", store.ID, updateErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
}

// TestStoreRaw godoc
// @Summary      Test vector store connection with raw credentials
// @Description  Test connectivity using provided credentials without persisting. Returns detected server version.
// @Tags         VectorStore
// @Accept       json
// @Produce      json
// @Param        request  body      TestStoreRequest         true  "Engine type and connection config"
// @Success      200      {object}  map[string]interface{}   "Connection test result (success, version)"
// @Failure      400      {object}  errors.AppError          "Invalid request"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Router       /vector-stores/test [post]
func (h *VectorStoreHandler) TestStoreRaw(c *gin.Context) {
	ctx := c.Request.Context()

	var req TestStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	// Raw user input: TestRawConnection applies the engine-type allowlist,
	// required-field, and SSRF checks before any dial (unlike TestStoreByID,
	// which probes trusted stored/env configs).
	version, err := h.service.TestRawConnection(ctx, req.EngineType, req.ConnectionConfig)
	if err != nil {
		logger.Warnf(ctx, "Vector store connection test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
}
