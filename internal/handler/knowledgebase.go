package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/errors"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/tracing/langfuse"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	"roche.local/knowledge-agent-platform/internal/utils"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// KnowledgeBaseHandler defines the HTTP handler for knowledge base operations
type KnowledgeBaseHandler struct {
	cfg                *config.Config
	service            interfaces.KnowledgeBaseService
	knowledgeService   interfaces.KnowledgeService
	asynqClient        interfaces.TaskEnqueuer
	vectorStoreService interfaces.VectorStoreService // enriches KB responses with bound store display
	// userService 仅在 list 类接口里用于批量回填 creator_name；
	// 真正的鉴权由 RBAC 中间件 + Lookup 完成，这里不参与决策。
	userService interfaces.UserService
}

// NewKnowledgeBaseHandler creates a new knowledge base handler instance
func NewKnowledgeBaseHandler(
	cfg *config.Config,
	service interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
	asynqClient interfaces.TaskEnqueuer,
	vectorStoreService interfaces.VectorStoreService,
	userService interfaces.UserService,
) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{
		cfg:                cfg,
		service:            service,
		knowledgeService:   knowledgeService,
		asynqClient:        asynqClient,
		vectorStoreService: vectorStoreService,
		userService:        userService,
	}
}

// buildKBResponse turns a knowledge base into a JSON-ready response shape,
// merging the bound vector store's display metadata and any caller-supplied
// extras (e.g., my_permission for directly granted KBs). Returns the kb pointer
// unchanged on serialization failure so the request still succeeds.
//
// The map-merge approach (rather than a wrapper struct embedding the kb)
// is deliberate: KnowledgeBase has a custom MarshalJSON, and embedding
// would promote it to any wrapper struct and silently swallow the extra
// fields. The same pattern is already used by GetKnowledgeBase to add the
// my_permission field for directly granted knowledge bases.
func buildKBResponse(
	kb *types.KnowledgeBase,
	storeView types.StoreDisplay,
	extras map[string]interface{},
) interface{} {
	b, err := json.Marshal(kb)
	if err != nil {
		return kb
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return kb
	}
	if storeView.Name != "" {
		m["vector_store_name"] = storeView.Name
	}
	if storeView.Source != "" {
		m["vector_store_source"] = storeView.Source
	}
	if storeView.EngineType != "" {
		m["vector_store_engine_type"] = storeView.EngineType
	}
	if storeView.Status != "" {
		m["vector_store_status"] = storeView.Status
	}
	for k, v := range extras {
		m[k] = v
	}
	return m
}

// buildKBListResponse turns a slice of knowledge bases into a JSON-ready
// slice that mirrors the single-KB enrichment in buildKBResponse. Store
// views are batch-resolved once via BatchResolveStoreView to keep the
// list endpoint O(1) in vector-store service calls — the per-KB
// resolveKBStoreView would otherwise be N+1.
//
// Binding semantics match the single-KB path:
//   - KB has no binding         → DefaultStoreDisplay()
//   - KB is bound to a store    → look up in the batch result; misses
//     fall back to UnavailableStoreDisplay()
//
// Resolver failures degrade gracefully: every bound KB renders
// as unavailable instead of breaking the list response.
func (h *KnowledgeBaseHandler) buildKBListResponse(
	ctx context.Context, kbs []*types.KnowledgeBase,
) []interface{} {
	defaultView := h.envDefaultStoreView(ctx)
	storeViews := h.batchResolveKBStoreViews(ctx, kbs)
	out := make([]interface{}, 0, len(kbs))
	for _, kb := range kbs {
		var view types.StoreDisplay
		switch {
		case !kb.HasVectorStore():
			view = defaultView
		default:
			v, ok := storeViews[*kb.VectorStoreID]
			if !ok || v.Source == "" {
				view = types.UnavailableStoreDisplay()
			} else {
				view = v
			}
		}
		out = append(out, buildKBResponse(kb, view, nil))
	}
	return out
}

func (h *KnowledgeBaseHandler) envDefaultStoreView(ctx context.Context) types.StoreDisplay {
	if h.vectorStoreService == nil {
		return types.DefaultStoreDisplay()
	}
	return h.vectorStoreService.EnvDefaultStoreView(ctx)
}

// batchResolveKBStoreViews collects the unique store IDs across
// the KB slice and resolves them in one BatchResolveStoreView call.
func (h *KnowledgeBaseHandler) batchResolveKBStoreViews(
	ctx context.Context, kbs []*types.KnowledgeBase,
) map[string]types.StoreDisplay {
	if h.vectorStoreService == nil {
		return nil
	}
	storeIDs := make([]string, 0)
	seen := make(map[string]bool)
	for _, kb := range kbs {
		if !kb.HasVectorStore() {
			continue
		}
		sid := *kb.VectorStoreID
		if !seen[sid] {
			seen[sid] = true
			storeIDs = append(storeIDs, sid)
		}
	}
	if len(storeIDs) == 0 {
		return nil
	}
	views, err := h.vectorStoreService.BatchResolveStoreView(ctx, 0, storeIDs)
	if err != nil {
		logger.WarnWithFields(ctx, logger.Fields{
			"store_count": len(storeIDs),
		}, "[kb.list] batch store view resolve failed; rendering affected stores as unavailable")
		return nil
	}
	return views
}

// resolveKBStoreView returns the store display payload to embed in the KB
// response. It applies two policies on top of the service-level resolver:
//
//   - When the KB does not have a DB-managed vector store binding,
//     the env-fallback display is returned without touching the service.
//   - When the caller has read-only access through a direct grant, the underlying
//     store's name and engine are suppressed so operator-chosen names do
//     not leak across knowledgeDomains. The Source value is set to "shared".
//
// On resolution error, an unavailable display is returned and the failure
// is logged for ops; the request itself still succeeds.
func (h *KnowledgeBaseHandler) resolveKBStoreView(
	ctx context.Context, kb *types.KnowledgeBase,
) types.StoreDisplay {
	if !kb.HasVectorStore() {
		return h.envDefaultStoreView(ctx)
	}
	if h.vectorStoreService == nil {
		return types.UnavailableStoreDisplay()
	}
	view, err := h.vectorStoreService.ResolveStoreView(ctx, 0, *kb.VectorStoreID)
	if err != nil {
		logger.WarnWithFields(ctx, logger.Fields{
			"kb_id":               secutils.SanitizeForLog(kb.ID),
			"knowledge_domain_id": kb.KnowledgeDomainID,
		}, "[kb.view] vector store resolve failed; returning unavailable")
		return types.UnavailableStoreDisplay()
	}
	return view
}

// HybridSearch godoc
// @Summary      混合搜索
// @Description  在知识库中执行向量和关键词混合搜索。推荐使用 POST；GET 携带 JSON 请求体仍受支持（兼容旧客户端）。
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id       path      string             true  "知识库ID"
// @Param        request  body      types.SearchParams true  "搜索参数"
// @Success      200      {object}  map[string]interface{}  "搜索结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/hybrid-search [post]
// @Router       /knowledge-bases/{id}/hybrid-search [get]
func (h *KnowledgeBaseHandler) HybridSearch(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start hybrid search")

	// Validate and check permission for knowledge base access
	_, id, effectiveKnowledgeDomainID, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Parse request body
	var req types.SearchParams
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	logger.Infof(ctx, "Executing hybrid search, knowledge base ID: %s, query: %s, effectiveKnowledgeDomainID: %d",
		secutils.SanitizeForLog(id), secutils.SanitizeForLog(req.QueryText), effectiveKnowledgeDomainID)

	// Execute hybrid search with default search parameters.
	results, err := h.service.HybridSearch(ctx, id, req)
	if err != nil {
		// Service-layer typed AppErrors (e.g. ErrVectorStoreBindingInvalid,
		// ErrVectorStoreUnavailable, BadRequest from multi-store fan-out)
		// must reach the client with their original code rather than be
		// downgraded to InternalServerError. Mirrors the pattern used in
		// CreateKnowledgeBase.
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Hybrid search completed, knowledge base ID: %s, result count: %d",
		secutils.SanitizeForLog(id), len(results))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}

// CreateKnowledgeBase godoc
// @Summary      创建知识库
// @Description  创建新的知识库
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        request  body      types.KnowledgeBase  true  "知识库信息"
// @Success      201      {object}  map[string]interface{}  "创建的知识库"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases [post]
func (h *KnowledgeBaseHandler) CreateKnowledgeBase(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start creating knowledge base")

	// Parse request body
	var req types.KnowledgeBase
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	if err := validateExtractConfig(req.ExtractConfig); err != nil {
		logger.Error(ctx, "Invalid extract configuration", err)
		c.Error(err)
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.GetStorageProvider()))
	if provider != "" && !isStorageProviderAllowed(provider) {
		c.Error(apperrors.NewBadRequestError("Storage provider is not allowed by STORAGE_ALLOW_LIST"))
		return
	}

	logger.Infof(ctx, "Creating knowledge base, name: %s", secutils.SanitizeForLog(req.Name))
	// Create knowledge base using the service
	kb, err := h.service.CreateKnowledgeBase(ctx, &req)
	if err != nil {
		// Surface typed AppErrors (notably the 400-class codes
		// ErrVectorStoreBindingInvalid and ErrVectorStoreUnavailable
		// returned by validateVectorStoreBinding) instead of wrapping them
		// into a generic 500. The middleware renders the original code and
		// HTTP status verbatim. Falls through to 500 only for raw infra
		// errors that the service did not classify.
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge base created successfully, ID: %s, name: %s",
		secutils.SanitizeForLog(kb.ID), secutils.SanitizeForLog(kb.Name))
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    buildKBResponse(kb, h.resolveKBStoreView(ctx, kb), nil),
	})
}

// validateAndGetKnowledgeBase validates request parameters and retrieves the knowledge base.
// Returns the knowledge base, knowledge base ID, effective knowledgeDomain ID for embedding, permission level, and any errors encountered
// effectiveKnowledgeDomainID is the active enterprise knowledgeDomain ID.
func (h *KnowledgeBaseHandler) validateAndGetKnowledgeBase(
	c *gin.Context,
) (*types.KnowledgeBase, string, uint64, types.KnowledgeBasePermissionLevel, error) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		return nil, "", 0, "", apperrors.NewBadRequestError("Knowledge base ID cannot be empty")
	}
	if access, ok := middleware.KBAccessFromContext(c); ok && access.KnowledgeBase != nil && access.KnowledgeBase.ID == id {
		return access.KnowledgeBase, id, access.EffectiveKnowledgeDomainID, access.Permission, nil
	}

	kb, err := h.service.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		if stderrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			return nil, id, 0, "", apperrors.NewNotFoundError("knowledge base not found")
		}
		return nil, id, 0, "", apperrors.NewInternalServerError("cannot load knowledge base")
	}
	checker, ok := h.service.(knowledgeBaseAccessChecker)
	if !ok {
		return nil, id, 0, "", apperrors.NewInternalServerError("knowledge base authorization is unavailable")
	}
	canManage, accessErr := checker.CanManageKnowledgeBase(ctx, kb)
	if accessErr != nil {
		return nil, id, 0, "", apperrors.NewInternalServerError("cannot verify knowledge base access")
	}
	if canManage {
		return kb, id, kb.KnowledgeDomainID, types.KnowledgeBasePermissionManage, nil
	}
	canRead, accessErr := checker.CanReadKnowledgeBase(ctx, kb)
	if accessErr != nil {
		return nil, id, 0, "", apperrors.NewInternalServerError("cannot verify knowledge base access")
	}
	if !canRead {
		return nil, id, 0, "", apperrors.NewForbiddenError("No permission to operate")
	}
	return kb, id, kb.KnowledgeDomainID, types.KnowledgeBasePermissionRead, nil
}

// GetKnowledgeBase godoc
// @Summary      获取知识库详情
// @Description  根据ID获取知识库详情。
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id         path      string  true   "知识库ID"
// @Success      200  {object}  map[string]interface{}  "知识库详情"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "知识库不存在"
// @Security     Bearer
// @Router       /knowledge-bases/{id} [get]
func (h *KnowledgeBaseHandler) GetKnowledgeBase(c *gin.Context) {
	// Validate and get the knowledge base
	kb, _, _, permission, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	// Fill counts (knowledge_count, chunk_count, is_processing) so hover/detail shows correct numbers
	if fillErr := h.service.FillKnowledgeBaseCounts(c.Request.Context(), kb); fillErr != nil {
		logger.Warnf(c.Request.Context(), "Failed to fill KB counts for %s: %v", kb.ID, fillErr)
	}
	storeView := h.resolveKBStoreView(c.Request.Context(), kb)
	var extras map[string]interface{}
	if permission != "" {
		extras = map[string]interface{}{"my_permission": permission}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": buildKBResponse(kb, storeView, extras)})
}

// ListKnowledgeBases godoc
// @Summary      获取知识库列表
// @Description  获取当前用户通过显式整库或单文档授权可访问的知识库。
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "知识库列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Router       /knowledge-bases [get]
func (h *KnowledgeBaseHandler) ListKnowledgeBases(c *gin.Context) {
	ctx := c.Request.Context()

	// Get all knowledge bases for this knowledgeDomain
	kbs, err := h.service.ListKnowledgeBases(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	// Optional creator filter — drives the [All | Mine | Others] segmented
	// control on the list page. We filter in-process rather than pushing
	// down into SQL because the knowledgeDomain-bounded KB list is small (typically
	// <100 rows) and adding a creator predicate to ListKnowledgeBases would
	// ripple through every other caller (chat pipeline, agent editor, …).
	// Rows with empty CreatorID predate the RBAC migration (PR 5); we treat
	// them as "not anyone in particular" so they never appear under "mine"
	// or "others" — they fall out of both filters cleanly.
	creatorFilter := strings.ToLower(strings.TrimSpace(c.Query("creator")))
	if creatorFilter == "mine" || creatorFilter == "others" {
		callerUserID, _ := c.Get(types.UserIDContextKey.String())
		callerUserIDStr, _ := callerUserID.(string)
		filtered := make([]*types.KnowledgeBase, 0, len(kbs))
		for _, kb := range kbs {
			if kb.CreatorID == "" {
				continue
			}
			if creatorFilter == "mine" && kb.CreatorID == callerUserIDStr {
				filtered = append(filtered, kb)
			} else if creatorFilter == "others" && kb.CreatorID != callerUserIDStr {
				filtered = append(filtered, kb)
			}
		}
		kbs = filtered
	}
	// 批量回填 creator_name，让前端列表能区分「我创建」与「同知识域其他成员创建」。
	// 仅在 list 接口里回填，详情 / 编辑场景不依赖这个字段；解析失败（用户已删除、
	// CreatorID 为空的老数据）就让字段为空，前端按 fallback 渲染。
	enrichKBCreatorNames(ctx, h.userService, kbs)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.buildKBListResponse(ctx, kbs),
	})
}

// enrichKBCreatorNames 把 KB 列表里的 CreatorID 批量解析成展示名（username
// 优先，退化到 email）。任意一步失败都吞掉错误：creator_name 缺失只会影响
// 卡片右下角的徽章展示，不该影响列表本身可用。
func enrichKBCreatorNames(ctx context.Context, userSvc interfaces.UserService, kbs []*types.KnowledgeBase) {
	if userSvc == nil || len(kbs) == 0 {
		return
	}
	idSet := make(map[string]struct{}, len(kbs))
	for _, kb := range kbs {
		if kb.CreatorID != "" {
			idSet[kb.CreatorID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	users, err := userSvc.GetUsersByIDs(ctx, ids)
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve KB creator names: %v", err)
		return
	}
	for _, kb := range kbs {
		if kb.CreatorID == "" {
			continue
		}
		u, ok := users[kb.CreatorID]
		if !ok || u == nil {
			continue
		}
		kb.CreatorName = pickUserDisplayName(u)
	}
}

// pickUserDisplayName picks the field most users will recognise: Username
// if present (it's required at registration), Email as a fallback. Used by
// both KB and Agent list enrichment so the badge text stays consistent.
func pickUserDisplayName(u *types.User) string {
	if u == nil {
		return ""
	}
	if u.Username != "" {
		return u.Username
	}
	return u.Email
}

// TogglePinKnowledgeBase godoc
// @Summary      置顶/取消置顶知识库
// @Description  切换知识库的置顶状态
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id  path      string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}  "更新后的知识库"
// @Failure      404  {object}  errors.AppError         "知识库不存在"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/pin [put]
func (h *KnowledgeBaseHandler) TogglePinKnowledgeBase(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	if id == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base ID is required"))
		return
	}

	kb, err := h.service.TogglePinKnowledgeBase(ctx, id)
	if err != nil {
		if stderrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(apperrors.NewNotFoundError("knowledge base not found"))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    buildKBResponse(kb, h.resolveKBStoreView(ctx, kb), nil),
	})
}

// TogglePublicKnowledgeBase godoc
// @Summary      设置知识库公开/不公开
// @Description  切换知识库的全公司开放状态，公开后所有认证用户默认可读，但可通过 ACL 单独停用特定用户
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "知识库ID"
// @Param        request  body      map[string]bool         true  "is_public 字段"
// @Success      200      {object}  map[string]interface{}  "更新后的知识库"
// @Failure      404      {object}  errors.AppError         "知识库不存在"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/public [put]
func (h *KnowledgeBaseHandler) TogglePublicKnowledgeBase(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	if id == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base ID is required"))
		return
	}

	var req struct {
		IsPublic bool `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	kb, err := h.service.TogglePublicKnowledgeBase(ctx, id, req.IsPublic)
	if err != nil {
		if stderrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(apperrors.NewNotFoundError("knowledge base not found"))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    buildKBResponse(kb, h.resolveKBStoreView(ctx, kb), nil),
	})
}

// UpdateKnowledgeBaseRequest defines the request body structure for updating a knowledge base
type UpdateKnowledgeBaseRequest struct {
	Name        string                     `json:"name"        binding:"required"`
	Description string                     `json:"description"`
	Config      *types.KnowledgeBaseConfig `json:"config"`
}

// UpdateKnowledgeBase godoc
// @Summary      更新知识库
// @Description  更新知识库的名称、描述和配置
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id       path      string                     true  "知识库ID"
// @Param        request  body      UpdateKnowledgeBaseRequest true  "更新请求"
// @Success      200      {object}  map[string]interface{}     "更新后的知识库"
// @Failure      400      {object}  errors.AppError            "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases/{id} [put]
func (h *KnowledgeBaseHandler) UpdateKnowledgeBase(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start updating knowledge base")

	// Validate and get the knowledge base
	_, id, _, permission, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Only admin/editor can update knowledge base
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(apperrors.NewForbiddenError("No permission to update knowledge base"))
		return
	}

	// Parse request body
	var req UpdateKnowledgeBaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	logger.Infof(ctx, "Updating knowledge base, ID: %s, name: %s",
		secutils.SanitizeForLog(id), secutils.SanitizeForLog(req.Name))

	// Update the knowledge base
	kb, err := h.service.UpdateKnowledgeBase(ctx, id, req.Name, req.Description, req.Config)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge base updated successfully, ID: %s",
		secutils.SanitizeForLog(id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    buildKBResponse(kb, h.resolveKBStoreView(ctx, kb), nil),
	})
}

// DeleteKnowledgeBase godoc
// @Summary      删除知识库
// @Description  删除指定的知识库及其所有内容
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}  "删除成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases/{id} [delete]
func (h *KnowledgeBaseHandler) DeleteKnowledgeBase(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start deleting knowledge base")

	// Validate and get the knowledge base
	kb, id, _, permission, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Deletion requires effective management permission in the KB's department.
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(apperrors.NewForbiddenError("Knowledge base management permission required"))
		return
	}

	logger.Infof(ctx, "Deleting knowledge base, ID: %s, name: %s",
		secutils.SanitizeForLog(id), secutils.SanitizeForLog(kb.Name))

	// Delete the knowledge base
	if err := h.service.DeleteKnowledgeBase(ctx, id); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge base deleted successfully, ID: %s",
		secutils.SanitizeForLog(id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge base deleted successfully",
	})
}

type CopyKnowledgeBaseRequest struct {
	TaskID   string `json:"task_id"`
	SourceID string `json:"source_id" binding:"required"`
	TargetID string `json:"target_id"`
}

// CopyKnowledgeBaseResponse defines the response for copy knowledge base
type CopyKnowledgeBaseResponse struct {
	TaskID   string `json:"task_id"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Message  string `json:"message"`
}

// CopyKnowledgeBase godoc
// @Summary      复制知识库
// @Description  将一个知识库的内容复制到另一个知识库（异步任务）
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        request  body      CopyKnowledgeBaseRequest   true  "复制请求"
// @Success      200      {object}  map[string]interface{}     "任务ID"
// @Failure      400      {object}  errors.AppError            "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases/copy [post]
func (h *KnowledgeBaseHandler) CopyKnowledgeBase(c *gin.Context) {
	ctx := c.Request.Context()
	var req CopyKnowledgeBaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Validate source knowledge base and management permission.
	sourceKB, err := h.service.GetKnowledgeBaseByID(ctx, req.SourceID)
	if err != nil {
		if stderrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(errors.NewNotFoundError("Source knowledge base not found"))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	checker, ok := h.service.(knowledgeBaseAccessChecker)
	if !ok {
		c.Error(apperrors.NewInternalServerError("knowledge base authorization is unavailable"))
		return
	}
	canManage, err := checker.CanManageKnowledgeBase(ctx, sourceKB)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("cannot verify source knowledge base access"))
		return
	}
	if !canManage {
		c.Error(errors.NewForbiddenError("No permission to copy this knowledge base"))
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, sourceKB.KnowledgeDomainID)

	// If target_id provided, validate target belongs to caller's knowledgeDomain
	// and run the pre-flight defenses synchronously so a mismatched
	// clone is rejected with 400 before the task is enqueued. The same
	// checks are re-applied inside the async worker (service.CopyKnowledgeBase)
	// as defense in depth.
	if req.TargetID != "" {
		targetKB, err := h.service.GetKnowledgeBaseByID(ctx, req.TargetID)
		if err != nil {
			if stderrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
				c.Error(errors.NewNotFoundError("Target knowledge base not found"))
				return
			}
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}
		canManageTarget, accessErr := checker.CanManageKnowledgeBase(ctx, targetKB)
		if accessErr != nil {
			c.Error(apperrors.NewInternalServerError("cannot verify target knowledge base access"))
			return
		}
		if !canManageTarget {
			c.Error(errors.NewForbiddenError("No permission to copy to this knowledge base"))
			return
		}
		if targetKB.KnowledgeDomainID != sourceKB.KnowledgeDomainID {
			c.Error(apperrors.NewBadRequestError("copying across knowledgeDomains is not supported"))
			return
		}
		// Pre-flight defense 1: embedding model must match.
		// Without this check the async clone would run with incompatible
		// vector spaces and produce semantically broken results.
		if sourceKB.EmbeddingModelID != targetKB.EmbeddingModelID {
			c.Error(apperrors.NewBadRequestError(
				"source and target knowledge bases use different embedding models; " +
					"clone into a target with the same embedding model"))
			return
		}
		// Pre-flight defense 2: vector store binding must match.
		// Cross-store cloning would require copying physical vector data
		// between stores, which is not yet supported.
		if !sourceKB.SharesStoreWith(targetKB) {
			c.Error(apperrors.NewBadRequestError(
				"source and target knowledge bases are bound to different vector stores; " +
					"cross-store cloning is not yet supported"))
			return
		}
		// Pre-flight defense 3: storage backend must match — only meaningful
		// when the knowledgeDomain has a StorageEngineConfig. Without it,
		// resolveFileService ignores per-KB provider pins and routes ALL KBs to
		// the global storage service, so a clone can never span two real
		// backends and the pins must NOT be used to reject (false positive).
		// When a knowledgeDomain config exists, pins are honored, so reject a genuine
		// cross-backend clone before enqueueing.
		if knowledgeDomain, _ := ctx.Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain); knowledgeDomain != nil && knowledgeDomain.StorageEngineConfig != nil {
			knowledgeDomainDefault := knowledgeDomain.StorageEngineConfig.DefaultProvider
			srcProvider := sourceKB.EffectiveStorageProvider(knowledgeDomainDefault)
			dstProvider := targetKB.EffectiveStorageProvider(knowledgeDomainDefault)
			if srcProvider != "" && dstProvider != "" && srcProvider != dstProvider {
				c.Error(apperrors.NewBadRequestError(
					"source and target knowledge bases use different storage backends (" +
						srcProvider + " vs " + dstProvider + "); cross-storage-backend cloning is not supported"))
				return
			}
		}
	}

	// Generate task ID if not provided
	taskID := req.TaskID
	if taskID == "" {
		taskID = utils.GenerateTaskID("kb_clone", sourceKB.KnowledgeDomainID, req.SourceID)
	}

	// Create KB clone payload
	payload := types.KBClonePayload{
		KnowledgeDomainID: sourceKB.KnowledgeDomainID,
		TaskID:            taskID,
		SourceID:          req.SourceID,
		TargetID:          req.TargetID,
	}
	langfuse.InjectTracing(ctx, &payload)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal KB clone payload: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to create task"))
		return
	}

	// Enqueue KB clone task to Asynq
	task := asynq.NewTask(types.TypeKBClone, payloadBytes,
		asynq.TaskID(taskID), asynq.Queue("default"), asynq.MaxRetry(3))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue KB clone task: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to enqueue task"))
		return
	}

	logger.Infof(ctx, "KB clone task enqueued: %s, asynq task ID: %s, source: %s, target: %s",
		taskID, info.ID, secutils.SanitizeForLog(req.SourceID), secutils.SanitizeForLog(req.TargetID))

	// Save initial progress to Redis so frontend can query immediately
	initialProgress := &types.KBCloneProgress{
		TaskID:    taskID,
		SourceID:  req.SourceID,
		TargetID:  req.TargetID,
		Status:    types.KBCloneStatusPending,
		Progress:  0,
		Message:   "Task queued, waiting to start...",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	if err := h.knowledgeService.SaveKBCloneProgress(ctx, initialProgress); err != nil {
		logger.Warnf(ctx, "Failed to save initial KB clone progress: %v", err)
		// Don't fail the request, task is already enqueued
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": CopyKnowledgeBaseResponse{
			TaskID:   taskID,
			SourceID: req.SourceID,
			TargetID: req.TargetID,
			Message:  "Knowledge base copy task started",
		},
	})
}

// GetKBCloneProgress godoc
// @Summary      获取知识库复制进度
// @Description  获取知识库复制任务的进度
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        task_id  path      string  true  "任务ID"
// @Success      200      {object}  map[string]interface{}  "进度信息"
// @Failure      404      {object}  errors.AppError         "任务不存在"
// @Security     Bearer
// @Router       /knowledge-bases/copy/progress/{task_id} [get]
func (h *KnowledgeBaseHandler) GetKBCloneProgress(c *gin.Context) {
	ctx := c.Request.Context()

	taskID := c.Param("task_id")
	if taskID == "" {
		logger.Error(ctx, "Task ID is empty")
		c.Error(apperrors.NewBadRequestError("Task ID cannot be empty"))
		return
	}
	if err := requireTaskProgressKnowledgeDomain(ctx, taskID); err != nil {
		c.Error(err)
		return
	}

	progress, err := h.knowledgeService.GetKBCloneProgress(ctx, taskID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    progress,
	})
}

// validateExtractConfig validates the graph configuration parameters
func validateExtractConfig(config *types.ExtractConfig) error {
	if config == nil {
		return nil
	}
	if !config.Enabled {
		*config = types.ExtractConfig{Enabled: false}
		return nil
	}
	// Validate text field
	if config.Text == "" {
		return apperrors.NewBadRequestError("text cannot be empty")
	}

	// Validate tags field
	if len(config.Tags) == 0 {
		return apperrors.NewBadRequestError("tags cannot be empty")
	}
	for i, tag := range config.Tags {
		if tag == "" {
			return apperrors.NewBadRequestError("tag cannot be empty at index " + strconv.Itoa(i))
		}
	}

	// Validate nodes
	if len(config.Nodes) == 0 {
		return apperrors.NewBadRequestError("nodes cannot be empty")
	}
	nodeNames := make(map[string]bool)
	for i, node := range config.Nodes {
		if node.Name == "" {
			return apperrors.NewBadRequestError("node name cannot be empty at index " + strconv.Itoa(i))
		}
		// Check for duplicate node names
		if nodeNames[node.Name] {
			return apperrors.NewBadRequestError("duplicate node name: " + node.Name)
		}
		nodeNames[node.Name] = true
	}

	if len(config.Relations) == 0 {
		return apperrors.NewBadRequestError("relations cannot be empty")
	}
	// Validate relations
	for i, relation := range config.Relations {
		if relation.Node1 == "" {
			return apperrors.NewBadRequestError("relation node1 cannot be empty at index " + strconv.Itoa(i))
		}
		if relation.Node2 == "" {
			return apperrors.NewBadRequestError("relation node2 cannot be empty at index " + strconv.Itoa(i))
		}
		if relation.Type == "" {
			return apperrors.NewBadRequestError("relation type cannot be empty at index " + strconv.Itoa(i))
		}
		// Check if referenced nodes exist
		if !nodeNames[relation.Node1] {
			return apperrors.NewBadRequestError("relation references non-existent node1: " + relation.Node1)
		}
		if !nodeNames[relation.Node2] {
			return apperrors.NewBadRequestError("relation references non-existent node2: " + relation.Node2)
		}
	}

	return nil
}

// ListMoveTargets returns knowledge bases eligible as move targets for the given source KB.
// Filters: same Type, same EmbeddingModelID, different ID, not temporary.
//
// ListMoveTargets godoc
// @Summary      获取可移动目标知识库列表
// @Description  返回与源知识库 Type 一致、EmbeddingModelID 一致、非临时且不是自身的目标知识库列表
// @Tags         知识库
// @Produce      json
// @Param        id   path      string                  true  "源知识库 ID"
// @Success      200  {object}  map[string]interface{}  "可移动目标列表"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "知识库不存在"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/move-targets [get]
func (h *KnowledgeBaseHandler) ListMoveTargets(c *gin.Context) {
	ctx := c.Request.Context()

	sourceKBID := c.Param("id")
	if sourceKBID == "" {
		c.Error(apperrors.NewBadRequestError("Knowledge base ID is required"))
		return
	}

	// Get source knowledge base
	sourceKB, err := h.service.GetKnowledgeBaseByID(ctx, sourceKBID)
	if err != nil {
		if stderrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(errors.NewNotFoundError("Source knowledge base not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	checker, ok := h.service.(knowledgeBaseAccessChecker)
	if !ok {
		c.Error(apperrors.NewInternalServerError("knowledge base authorization is unavailable"))
		return
	}
	canManage, accessErr := checker.CanManageKnowledgeBase(ctx, sourceKB)
	if accessErr != nil {
		c.Error(apperrors.NewInternalServerError("cannot verify knowledge base access"))
		return
	}
	if !canManage {
		c.Error(errors.NewForbiddenError("No permission to manage this knowledge base"))
		return
	}

	// Get all knowledge bases
	allKBs, err := h.service.ListKnowledgeBases(ctx)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// Filter eligible targets
	targets := make([]*types.KnowledgeBase, 0)
	for _, kb := range allKBs {
		if kb.ID == sourceKBID {
			continue
		}
		if kb.IsTemporary {
			continue
		}
		if kb.Type != sourceKB.Type {
			continue
		}
		if kb.EmbeddingModelID != sourceKB.EmbeddingModelID {
			continue
		}
		targets = append(targets, kb)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    targets,
	})
}
