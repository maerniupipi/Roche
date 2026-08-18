package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/handler/dto"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// KnowledgeDomainHandler manages knowledge-domain identity, storage accounting
// and explicit administrator assignments.
type KnowledgeDomainHandler struct {
	service          interfaces.KnowledgeDomainService
	adminService     interfaces.KnowledgeDomainAdminService
	config           *config.Config
	systemSettingSvc interfaces.SystemSettingService
}

// NewKnowledgeDomainHandler creates a knowledge-domain handler.
// Parameters:
//   - service: An implementation of the KnowledgeDomainService interface for business logic
//   - config: Application configuration
//
// # Returns a pointer to the newly created KnowledgeDomainHandler
func NewKnowledgeDomainHandler(
	service interfaces.KnowledgeDomainService,
	adminService interfaces.KnowledgeDomainAdminService,
	config *config.Config,
	systemSettingSvc interfaces.SystemSettingService,
) *KnowledgeDomainHandler {
	return &KnowledgeDomainHandler{
		service:          service,
		adminService:     adminService,
		config:           config,
		systemSettingSvc: systemSettingSvc,
	}
}

// createKnowledgeDomainRequest limits creation to domain identity fields.
type createKnowledgeDomainRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	NameEn      string `json:"name_en" binding:"max=128"`
	Description string `json:"description" binding:"max=512"`
}

// updateKnowledgeDomainRequest contains the domain identity fields that a
// system or assigned domain administrator may edit.
//
// Pointers so we can distinguish "not sent" from "explicit empty
// string"; when nil we leave the existing column untouched.
type updateKnowledgeDomainRequest struct {
	Name        *string `json:"name"        binding:"omitempty,min=1,max=128"`
	NameEn      *string `json:"name_en"     binding:"omitempty,max=128"`
	Description *string `json:"description" binding:"omitempty,max=512"`
}

// CreateKnowledgeDomain godoc
// @Summary      创建知识域
// @Description  系统管理员创建企业知识域，普通用户无自助创建入口。
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Param        request  body      handler.createKnowledgeDomainRequest  true  "知识域信息"
// @Success      201      {object}  map[string]interface{}  "创建的知识域"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-domains [post]
func (h *KnowledgeDomainHandler) CreateKnowledgeDomain(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start creating knowledgeDomain")

	var req createKnowledgeDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	knowledgeDomainData := types.KnowledgeDomain{
		Name:        strings.TrimSpace(req.Name),
		NameEn:      strings.TrimSpace(req.NameEn),
		Description: strings.TrimSpace(req.Description),
	}

	// Persist an explicit quota so later setting changes do not silently alter
	// existing knowledgeDomains.
	if knowledgeDomainData.StorageQuota <= 0 {
		gb := h.systemSettingSvc.GetInt(
			ctx,
			"knowledge_domain.default_storage_quota_gb",
			"ROCHE_KAP_KNOWLEDGE_DOMAIN_DEFAULT_STORAGE_QUOTA_GB",
			10,
		)
		if gb <= 0 {
			gb = 10
		}
		knowledgeDomainData.StorageQuota = gb * 1024 * 1024 * 1024
	}

	logger.Infof(ctx, "Creating knowledgeDomain, name: %s", secutils.SanitizeForLog(knowledgeDomainData.Name))

	createdKnowledgeDomain, err := h.service.CreateKnowledgeDomain(ctx, &knowledgeDomainData)
	if err != nil {
		// Check if this is an application-specific error
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to create knowledgeDomain: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to create knowledgeDomain").WithDetails(err.Error()))
		}
		return
	}

	logger.Infof(
		ctx,
		"KnowledgeDomain created successfully, ID: %d, name: %s",
		createdKnowledgeDomain.ID,
		secutils.SanitizeForLog(createdKnowledgeDomain.Name),
	)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    dto.NewKnowledgeDomainResponse(ctx, createdKnowledgeDomain),
	})
}

// GetKnowledgeDomain godoc
// @Summary      获取知识域详情
// @Description  根据ID获取知识域详情
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "知识域ID"
// @Success      200  {object}  map[string]interface{}  "知识域详情"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "知识域不存在"
// @Security     Bearer
// @Router       /knowledge-domains/{id} [get]
func (h *KnowledgeDomainHandler) GetKnowledgeDomain(c *gin.Context) {
	ctx := c.Request.Context()

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		logger.Errorf(ctx, "Invalid knowledgeDomain ID: %s", secutils.SanitizeForLog(c.Param("id")))
		c.Error(errors.NewBadRequestError("Invalid knowledgeDomain ID"))
		return
	}

	knowledgeDomain, err := h.service.GetKnowledgeDomainByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to retrieve knowledgeDomain: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to retrieve knowledgeDomain").WithDetails(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewKnowledgeDomainResponse(ctx, knowledgeDomain),
	})
}

// UpdateKnowledgeDomain godoc
// @Summary      更新知识域
// @Description  更新知识域信息
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Param        id       path      int           true  "知识域ID"
// @Param        request  body      types.KnowledgeDomain  true  "知识域信息"
// @Success      200      {object}  map[string]interface{}  "更新后的知识域"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-domains/{id} [put]
func (h *KnowledgeDomainHandler) UpdateKnowledgeDomain(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start updating knowledgeDomain")

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		logger.Errorf(ctx, "Invalid knowledgeDomain ID: %s", secutils.SanitizeForLog(c.Param("id")))
		c.Error(errors.NewBadRequestError("Invalid knowledgeDomain ID"))
		return
	}

	// Strict whitelist: only Name / NameEn / Description are mutable
	// through the public PUT. Storage quota, status, business, configs,
	// api_key and every other privileged column live behind dedicated
	// endpoints (PUT /system/runtime-config/:key, ...). Without this, an
	// knowledgeDomain Admin could flip status / bump storage_quota by crafting an extended
	// JSON body. Pointers distinguish "field omitted" from "explicit
	// empty string" so we can leave untouched columns alone.
	var req updateKnowledgeDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	// Load the persisted knowledgeDomain so any column the request omits keeps
	// its current value through the GORM `Updates(struct)` zero-skip
	// behaviour (we always pass back the full struct).
	existing, err := h.service.GetKnowledgeDomainByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to load knowledgeDomain").WithDetails(err.Error()))
		}
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			c.Error(errors.NewValidationError("name cannot be blank"))
			return
		}
		existing.Name = trimmed
	}
	if req.NameEn != nil {
		existing.NameEn = strings.TrimSpace(*req.NameEn)
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}

	logger.Infof(ctx, "Updating knowledgeDomain, ID: %d, Name: %s", id, secutils.SanitizeForLog(existing.Name))

	updatedKnowledgeDomain, err := h.service.UpdateKnowledgeDomain(ctx, existing)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to update knowledgeDomain: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update knowledgeDomain").WithDetails(err.Error()))
		}
		return
	}

	logger.Infof(
		ctx,
		"KnowledgeDomain updated successfully, ID: %d, Name: %s",
		updatedKnowledgeDomain.ID,
		secutils.SanitizeForLog(updatedKnowledgeDomain.Name),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewKnowledgeDomainResponse(ctx, updatedKnowledgeDomain),
	})
}

// DeleteKnowledgeDomain godoc
// @Summary      删除知识域
// @Description  删除指定的知识域
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "知识域ID"
// @Success      200  {object}  map[string]interface{}  "删除成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-domains/{id} [delete]
func (h *KnowledgeDomainHandler) DeleteKnowledgeDomain(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting knowledgeDomain")

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		logger.Errorf(ctx, "Invalid knowledgeDomain ID: %s", secutils.SanitizeForLog(c.Param("id")))
		c.Error(errors.NewBadRequestError("Invalid knowledgeDomain ID"))
		return
	}

	logger.Infof(ctx, "Deleting knowledgeDomain, ID: %d", id)

	if err := h.service.DeleteKnowledgeDomain(ctx, id); err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to delete knowledgeDomain: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to delete knowledgeDomain").WithDetails(err.Error()))
		}
		return
	}

	logger.Infof(ctx, "KnowledgeDomain deleted successfully, ID: %d", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "KnowledgeDomain deleted successfully",
	})
}

// ListKnowledgeDomains godoc
// @Summary      获取知识域列表
// @Description  获取当前用户可访问的知识域列表
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "知识域列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Router       /knowledge-domains [get]
func (h *KnowledgeDomainHandler) ListKnowledgeDomains(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(userID) == "" {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	var domains []*types.KnowledgeDomain
	var err error
	if types.IsSystemAdminFromContext(ctx) {
		domains, err = h.service.ListKnowledgeDomains(ctx)
	} else {
		if h.adminService == nil {
			c.Error(errors.NewServiceUnavailableError("Knowledge domain authorization is unavailable"))
			return
		}
		var ids []uint64
		ids, err = h.adminService.ListDomainIDs(ctx, userID)
		if err == nil && len(ids) > 0 {
			var byID map[uint64]*types.KnowledgeDomain
			byID, err = h.service.GetKnowledgeDomainsByIDs(ctx, ids)
			if err == nil {
				domains = make([]*types.KnowledgeDomain, 0, len(ids))
				for _, id := range ids {
					if domain := byID[id]; domain != nil {
						domains = append(domains, domain)
					}
				}
			}
		}
	}
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list knowledgeDomains"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": dto.NewKnowledgeDomainResponses(ctx, domains),
		},
	})
}

// ListAllKnowledgeDomains godoc
// @Summary      获取所有知识域列表
// @Description  获取系统中所有知识域（需要跨知识域访问权限）
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "所有知识域列表"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Router       /knowledge-domains/all [get]
func (h *KnowledgeDomainHandler) ListAllKnowledgeDomains(c *gin.Context) {
	ctx := c.Request.Context()

	// System-administrator gating is enforced at the route layer via middleware.RequireCrossKnowledgeDomainAccess
	// (router.go). The handler stays focused on listing.
	knowledgeDomains, err := h.service.ListAllKnowledgeDomains(ctx)
	if err != nil {
		// Check if this is an application-specific error
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to retrieve all knowledgeDomains list: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to retrieve all knowledgeDomains list").WithDetails(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": dto.NewKnowledgeDomainResponses(ctx, knowledgeDomains),
		},
	})
}

// SearchKnowledgeDomains godoc
// @Summary      搜索知识域
// @Description  分页搜索知识域（需要跨知识域访问权限）
// @Tags         知识域管理
// @Accept       json
// @Produce      json
// @Param        keyword    query     string  false  "搜索关键词"
// @Param        knowledge_domain_id  query     int     false  "知识域ID筛选"
// @Param        page       query     int     false  "页码"  default(1)
// @Param        page_size  query     int     false  "每页数量"  default(20)
// @Success      200        {object}  map[string]interface{}  "搜索结果"
// @Failure      403        {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Router       /knowledge-domains/search [get]
func (h *KnowledgeDomainHandler) SearchKnowledgeDomains(c *gin.Context) {
	ctx := c.Request.Context()

	// Cross-knowledgeDomain gating is enforced at the route layer via
	// middleware.RequireCrossKnowledgeDomainAccess (router.go); the handler only
	// parses query params and delegates to the service.

	// Parse query parameters
	keyword := c.Query("keyword")
	knowledgeDomainIDStr := c.Query("knowledge_domain_id")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	var knowledgeDomainID uint64
	if knowledgeDomainIDStr != "" {
		parsedID, err := strconv.ParseUint(knowledgeDomainIDStr, 10, 64)
		if err == nil {
			knowledgeDomainID = parsedID
		}
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Limit max page size
	}

	knowledgeDomains, total, err := h.service.SearchKnowledgeDomains(ctx, keyword, knowledgeDomainID, page, pageSize)
	if err != nil {
		// Check if this is an application-specific error
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to search knowledgeDomains: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to search knowledgeDomains").WithDetails(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     dto.NewKnowledgeDomainResponses(ctx, knowledgeDomains),
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetPlatformRuntimeConfig godoc
// @Summary      获取平台运行配置
// @Description  获取平台级运行配置（支持web-search-config、prompt-templates、parser-engine-config、storage-engine-config、retrieval-config）
// @Tags         平台配置
// @Accept       json
// @Produce      json
// @Param        key  path      string  true  "配置键名"
// @Success      200  {object}  map[string]interface{}  "配置值"
// @Failure      400  {object}  errors.AppError         "不支持的键"
// @Security     Bearer
// @Router       /system/runtime-config/{key} [get]
func (h *KnowledgeDomainHandler) GetPlatformRuntimeConfig(c *gin.Context) {
	ctx := c.Request.Context()
	key := secutils.SanitizeForLog(c.Param("key"))

	switch key {
	case "web-search-config", "parser-engine-config", "storage-engine-config":
		if !dto.CanViewIntegrationSecrets(ctx) {
			c.Error(errors.NewForbiddenError("integration configuration requires admin access"))
			return
		}
	}

	switch key {
	case "web-search-config":
		h.GetKnowledgeDomainWebSearchConfig(c)
		return
	case "prompt-templates":
		h.GetPromptTemplates(c)
		return
	case "parser-engine-config":
		h.GetKnowledgeDomainParserEngineConfig(c)
		return
	case "storage-engine-config":
		h.GetKnowledgeDomainStorageEngineConfig(c)
		return
	case "retrieval-config":
		h.GetKnowledgeDomainRetrievalConfig(c)
		return
	default:
		logger.Info(ctx, "KV key not supported", "key", key)
		c.Error(errors.NewBadRequestError("unsupported key"))
		return
	}
}

// UpdatePlatformRuntimeConfig godoc
// @Summary      更新平台运行配置
// @Description  更新平台级运行配置（支持web-search-config、parser-engine-config、storage-engine-config、retrieval-config）
// @Tags         平台配置
// @Accept       json
// @Produce      json
// @Param        key      path      string  true  "配置键名"
// @Param        request  body      object  true  "配置值"
// @Success      200      {object}  map[string]interface{}  "更新成功"
// @Failure      400      {object}  errors.AppError         "不支持的键"
// @Security     Bearer
// @Router       /system/runtime-config/{key} [put]
func (h *KnowledgeDomainHandler) UpdatePlatformRuntimeConfig(c *gin.Context) {
	ctx := c.Request.Context()
	key := secutils.SanitizeForLog(c.Param("key"))

	switch key {
	case "web-search-config", "parser-engine-config", "storage-engine-config":
		if !dto.CanViewIntegrationSecrets(ctx) {
			c.Error(errors.NewForbiddenError("integration configuration requires admin access"))
			return
		}
	}

	switch key {
	case "web-search-config":
		h.updateKnowledgeDomainWebSearchConfigInternal(c)
		return
	case "parser-engine-config":
		h.updateKnowledgeDomainParserEngineConfigInternal(c)
		return
	case "storage-engine-config":
		h.updateKnowledgeDomainStorageEngineConfigInternal(c)
		return
	case "retrieval-config":
		h.updateKnowledgeDomainRetrievalConfigInternal(c)
		return
	default:
		logger.Info(ctx, "KV key not supported", "key", key)
		c.Error(errors.NewBadRequestError("unsupported key"))
		return
	}
}

// updateKnowledgeDomainWebSearchConfigInternal updates the platform web search config.
func (h *KnowledgeDomainHandler) updateKnowledgeDomainWebSearchConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()

	// Bind directly into the strong typed struct
	var cfg types.WebSearchConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform web search config").WithDetails(err.Error()))
		return
	}

	cfg = *types.MergeWebSearchConfigForUpdate(&cfg, runtimeConfig.WebSearchConfig)

	// Validate configuration
	if cfg.MaxResults < 1 || cfg.MaxResults > 50 {
		c.Error(errors.NewBadRequestError("max_results must be between 1 and 50"))
		return
	}

	runtimeConfig.WebSearchConfig = &cfg
	updatedRuntimeConfig, err := h.service.UpdatePlatformRuntimeConfig(ctx, runtimeConfig)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to update knowledgeDomain: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update knowledgeDomain web search config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.WebSearchConfigForResponse(updatedRuntimeConfig.WebSearchConfig, true),
		"message": "Web search configuration updated successfully",
	})
}

// GetKnowledgeDomainWebSearchConfig godoc
// @Summary      获取平台网络搜索配置
// @Description  获取平台级网络搜索配置
// @Tags         平台配置
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "网络搜索配置"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /system/runtime-config/web-search-config [get]
func (h *KnowledgeDomainHandler) GetKnowledgeDomainWebSearchConfig(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start getting platform web search config")
	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform web search config").WithDetails(err.Error()))
		return
	}

	logger.Info(ctx, "Platform web search config retrieved successfully")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.WebSearchConfigForResponse(runtimeConfig.WebSearchConfig, true),
	})
}

// GetKnowledgeDomainParserEngineConfig returns the platform parser engine config.
func (h *KnowledgeDomainHandler) GetKnowledgeDomainParserEngineConfig(c *gin.Context) {
	ctx := c.Request.Context()
	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform parser engine config").WithDetails(err.Error()))
		return
	}
	data := types.ParserEngineConfigForResponse(runtimeConfig.ParserEngineConfig, true)
	if data == nil {
		data = &types.ParserEngineConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateKnowledgeDomainParserEngineConfigInternal updates the platform parser engine config.
func (h *KnowledgeDomainHandler) updateKnowledgeDomainParserEngineConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()
	var cfg types.ParserEngineConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform parser engine config").WithDetails(err.Error()))
		return
	}
	merged := types.MergeParserEngineConfigForUpdate(&cfg, runtimeConfig.ParserEngineConfig)
	if err := validateParserEngineOutboundURLs(merged); err != nil {
		c.Error(errors.NewValidationError(err.Error()))
		return
	}
	runtimeConfig.ParserEngineConfig = merged
	updatedRuntimeConfig, err := h.service.UpdatePlatformRuntimeConfig(ctx, runtimeConfig)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update knowledgeDomain parser engine config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.ParserEngineConfigForResponse(updatedRuntimeConfig.ParserEngineConfig, true),
		"message": "解析引擎配置已更新",
	})
}

// GetKnowledgeDomainStorageEngineConfig returns the platform storage engine config.
func (h *KnowledgeDomainHandler) GetKnowledgeDomainStorageEngineConfig(c *gin.Context) {
	ctx := c.Request.Context()
	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform storage engine config").WithDetails(err.Error()))
		return
	}
	data := types.StorageEngineConfigForResponse(runtimeConfig.StorageEngineConfig, true)
	if data == nil {
		data = &types.StorageEngineConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateKnowledgeDomainStorageEngineConfigInternal updates the platform storage engine config.
func (h *KnowledgeDomainHandler) updateKnowledgeDomainStorageEngineConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()
	var cfg types.StorageEngineConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	if provider == "" {
		provider = firstAllowedStorageProvider()
	}
	if provider == "" {
		c.Error(errors.NewBadRequestError("No storage provider is allowed by STORAGE_ALLOW_LIST"))
		return
	}
	if !isStorageProviderAllowed(provider) {
		c.Error(errors.NewBadRequestError("Storage provider is not allowed by STORAGE_ALLOW_LIST"))
		return
	}
	cfg.DefaultProvider = provider
	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform storage engine config").WithDetails(err.Error()))
		return
	}
	merged := types.MergeStorageEngineConfigForUpdate(&cfg, runtimeConfig.StorageEngineConfig)
	runtimeConfig.StorageEngineConfig = merged
	updatedRuntimeConfig, err := h.service.UpdatePlatformRuntimeConfig(ctx, runtimeConfig)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update knowledgeDomain storage engine config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.StorageEngineConfigForResponse(updatedRuntimeConfig.StorageEngineConfig, true),
		"message": "存储引擎配置已更新",
	})
}

// GetPromptTemplates godoc
// @Summary      获取提示词模板
// @Description  获取系统配置的提示词模板列表
// @Tags         平台配置
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "提示词模板配置"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /system/runtime-config/prompt-templates [get]
func (h *KnowledgeDomainHandler) GetPromptTemplates(c *gin.Context) {
	// Return prompt templates from config.yaml
	templates := h.config.PromptTemplates
	if templates == nil {
		templates = &config.PromptTemplatesConfig{}
	}

	// Determine user language from context (set by Language middleware)
	lang, _ := types.LanguageFromContext(c.Request.Context())

	// Build a localized copy so the original config is never mutated
	localized := &config.PromptTemplatesConfig{
		SystemPrompt:         config.LocalizeTemplates(templates.SystemPrompt, lang),
		ContextTemplate:      config.LocalizeTemplates(templates.ContextTemplate, lang),
		Rewrite:              config.LocalizeTemplates(templates.Rewrite, lang),
		Fallback:             config.LocalizeTemplates(templates.Fallback, lang),
		GenerateSessionTitle: templates.GenerateSessionTitle,
		GenerateSummary:      templates.GenerateSummary,
		KeywordsExtraction:   templates.KeywordsExtraction,
		AgentSystemPrompt:    config.LocalizeTemplates(templates.AgentSystemPrompt, lang),
		IntentPrompts:        config.LocalizeTemplates(templates.IntentPrompts, lang),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    localized,
	})
}

// GetKnowledgeDomainRetrievalConfig returns the platform retrieval configuration.
func (h *KnowledgeDomainHandler) GetKnowledgeDomainRetrievalConfig(c *gin.Context) {
	ctx := c.Request.Context()
	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform retrieval config").WithDetails(err.Error()))
		return
	}
	data := runtimeConfig.RetrievalConfig
	if data == nil {
		data = &types.RetrievalConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateKnowledgeDomainRetrievalConfigInternal updates the platform retrieval configuration.
func (h *KnowledgeDomainHandler) updateKnowledgeDomainRetrievalConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()

	var cfg types.RetrievalConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	// Validate thresholds
	if cfg.VectorThreshold < 0 || cfg.VectorThreshold > 1 {
		c.Error(errors.NewBadRequestError("vector_threshold must be between 0 and 1"))
		return
	}
	if cfg.KeywordThreshold < 0 || cfg.KeywordThreshold > 1 {
		c.Error(errors.NewBadRequestError("keyword_threshold must be between 0 and 1"))
		return
	}
	if cfg.RerankThreshold < -10 || cfg.RerankThreshold > 10 {
		c.Error(errors.NewBadRequestError("rerank_threshold must be between -10 and 10"))
		return
	}
	if cfg.EmbeddingTopK < 0 || cfg.EmbeddingTopK > 200 {
		c.Error(errors.NewBadRequestError("embedding_top_k must be between 0 and 200"))
		return
	}
	if cfg.RerankTopK < 0 || cfg.RerankTopK > 200 {
		c.Error(errors.NewBadRequestError("rerank_top_k must be between 0 and 200"))
		return
	}

	runtimeConfig, err := h.service.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to get platform retrieval config").WithDetails(err.Error()))
		return
	}

	runtimeConfig.RetrievalConfig = &cfg
	updatedRuntimeConfig, err := h.service.UpdatePlatformRuntimeConfig(ctx, runtimeConfig)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update retrieval config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updatedRuntimeConfig.RetrievalConfig,
		"message": "Retrieval configuration updated successfully",
	})
}

func validateParserEngineOutboundURLs(cfg *types.ParserEngineConfig) error {
	if cfg == nil {
		return nil
	}
	if endpoint := strings.TrimSpace(cfg.MinerUEndpoint); endpoint != "" {
		if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
			return fmt.Errorf("mineru_endpoint failed SSRF validation: %v", err)
		}
	}
	if vlmURL := strings.TrimSpace(cfg.MinerUVLMServerURL); vlmURL != "" {
		if err := secutils.ValidateURLForSSRF(vlmURL); err != nil {
			return fmt.Errorf("mineru_vlm_server_url failed SSRF validation: %v", err)
		}
	}
	return nil
}
