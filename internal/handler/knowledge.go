package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	goerrors "errors"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/application/service"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/tracing/langfuse"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	"roche.local/knowledge-agent-platform/internal/utils"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// KnowledgeHandler processes HTTP requests related to knowledge resources
type KnowledgeHandler struct {
	cfg         *config.Config
	kgService   interfaces.KnowledgeService
	kbService   interfaces.KnowledgeBaseService
	asynqClient interfaces.TaskEnqueuer
	spanRepo    repository.KnowledgeSpanRepository

	// businessAudit records knowledge document lifecycle events (download).
	businessAudit *service.BusinessAuditRecorder
}

type knowledgeBaseAccessChecker interface {
	CanReadKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error)
	CanManageKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error)
}

type knowledgeAccessChecker interface {
	CanReadKnowledge(ctx context.Context, knowledge *types.Knowledge) (bool, error)
}

// NewKnowledgeHandler creates a new knowledge handler instance
func NewKnowledgeHandler(
	cfg *config.Config,
	kgService interfaces.KnowledgeService,
	kbService interfaces.KnowledgeBaseService,
	asynqClient interfaces.TaskEnqueuer,
	spanRepo repository.KnowledgeSpanRepository,
	businessAudit *service.BusinessAuditRecorder,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		cfg:         cfg,
		kgService:   kgService,
		kbService:   kbService,
		asynqClient: asynqClient,
		spanRepo:    spanRepo,
		businessAudit: businessAudit,
	}
}

// requireKBOwnershipOrAdmin verifies effective KB management permission for
// routes whose KB id comes from the request body. Production services use the
// enterprise access checker.
func (h *KnowledgeHandler) requireKBOwnershipOrAdmin(c *gin.Context, kbID string) error {
	checker, ok := h.kbService.(knowledgeBaseAccessChecker)
	if !ok {
		return errors.NewInternalServerError("knowledge base authorization is unavailable")
	}
	kb, err := h.kbService.GetKnowledgeBaseByID(c.Request.Context(), kbID)
	if err != nil {
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			return errors.NewNotFoundError("knowledge base not found")
		}
		return errors.NewInternalServerError("cannot load knowledge base")
	}
	allowed, err := checker.CanManageKnowledgeBase(c.Request.Context(), kb)
	if err != nil {
		return errors.NewInternalServerError("cannot verify knowledge base access")
	}
	if !allowed {
		return errors.NewForbiddenError("No permission to operate on this knowledge base")
	}
	return nil
}

// validateKnowledgeBaseAccess validates access permissions to a knowledge base
// using the ":id" URL path parameter. It delegates to validateKnowledgeBaseAccessWithKBID.
func (h *KnowledgeHandler) validateKnowledgeBaseAccess(c *gin.Context) (*types.KnowledgeBase, string, uint64, types.KnowledgeBasePermissionLevel, error) {
	return h.validateKnowledgeBaseAccessWithKBID(c, secutils.SanitizeForLog(c.Param("id")))
}

func (h *KnowledgeHandler) validateKnowledgeBaseAccessWithKBID(
	c *gin.Context,
	kbID string,
) (*types.KnowledgeBase, string, uint64, types.KnowledgeBasePermissionLevel, error) {
	ctx := c.Request.Context()
	kbID = secutils.SanitizeForLog(kbID)
	if kbID == "" {
		return nil, "", 0, "", errors.NewBadRequestError("Knowledge base ID cannot be empty")
	}
	if access, ok := middleware.KBAccessFromContext(c); ok && access.KnowledgeBase != nil && access.KnowledgeBase.ID == kbID {
		return access.KnowledgeBase, kbID, access.EffectiveKnowledgeDomainID, access.Permission, nil
	}
	if h.kbService == nil {
		return nil, kbID, 0, "", errors.NewInternalServerError("knowledge base service is unavailable")
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			return nil, kbID, 0, "", errors.NewNotFoundError("knowledge base not found")
		}
		return nil, kbID, 0, "", errors.NewInternalServerError("cannot load knowledge base")
	}
	checker, ok := h.kbService.(knowledgeBaseAccessChecker)
	if !ok {
		return nil, kbID, 0, "", errors.NewInternalServerError("knowledge base authorization is unavailable")
	}
	canManage, accessErr := checker.CanManageKnowledgeBase(ctx, kb)
	if accessErr != nil {
		return nil, kbID, 0, "", errors.NewInternalServerError("cannot verify knowledge base access")
	}
	if canManage {
		return kb, kbID, kb.KnowledgeDomainID, types.KnowledgeBasePermissionManage, nil
	}
	canRead, accessErr := checker.CanReadKnowledgeBase(ctx, kb)
	if accessErr != nil {
		return nil, kbID, 0, "", errors.NewInternalServerError("cannot verify knowledge base access")
	}
	if !canRead {
		return nil, kbID, 0, "", errors.NewForbiddenError("Permission denied to access this knowledge base")
	}
	return kb, kbID, kb.KnowledgeDomainID, types.KnowledgeBasePermissionRead, nil
}

func (h *KnowledgeHandler) resolveKnowledgeAndValidateKBAccess(
	c *gin.Context,
	knowledgeID string,
	requiredPermission types.KnowledgeBasePermissionLevel,
) (*types.Knowledge, context.Context, error) {
	ctx := c.Request.Context()
	knowledge, err := h.kgService.GetKnowledgeByIDOnly(ctx, knowledgeID)
	if err != nil {
		return nil, ctx, errors.NewNotFoundError("Knowledge not found")
	}
	if h.kbService == nil {
		return nil, ctx, errors.NewInternalServerError("knowledge base authorization is unavailable")
	}
	_, _, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, knowledge.KnowledgeBaseID)
	if err != nil {
		return nil, ctx, err
	}
	if !permission.Allows(requiredPermission) {
		return nil, ctx, errors.NewForbiddenError("Permission denied to access this knowledge")
	}
	if requiredPermission == types.KnowledgeBasePermissionRead {
		if checker, ok := h.kbService.(knowledgeAccessChecker); ok {
			allowed, checkErr := checker.CanReadKnowledge(ctx, knowledge)
			if checkErr != nil {
				return nil, ctx, errors.NewInternalServerError("cannot verify knowledge access")
			}
			if !allowed {
				return nil, ctx, errors.NewForbiddenError("Permission denied to access this knowledge")
			}
		}
	}
	return knowledge, context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID), nil
}

// handleDuplicateKnowledgeError handles cases where duplicate knowledge is detected
// Returns true if the error was a duplicate error and was handled, false otherwise
func (h *KnowledgeHandler) handleDuplicateKnowledgeError(c *gin.Context,
	err error, knowledge *types.Knowledge, duplicateType string,
) bool {
	if dupErr, ok := err.(*types.DuplicateKnowledgeError); ok {
		ctx := c.Request.Context()
		logger.Warnf(ctx, "Detected duplicate %s: %s", duplicateType, secutils.SanitizeForLog(dupErr.Error()))
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": dupErr.Error(),
			"data":    knowledge, // knowledge contains the existing document
			"code":    fmt.Sprintf("duplicate_%s", duplicateType),
		})
		return true
	}
	return false
}

// enqueueKnowledgeListDelete enqueues an async batch-delete task for the
// given knowledge IDs and returns the asynq task ID.
func (h *KnowledgeHandler) enqueueKnowledgeListDelete(
	ctx context.Context, knowledgeDomainID uint64, ids []string,
) (string, error) {
	payload := types.KnowledgeListDeletePayload{
		KnowledgeDomainID: knowledgeDomainID,
		KnowledgeIDs:      ids,
	}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(types.TypeKnowledgeListDelete, payloadBytes,
		asynq.Queue("low"), asynq.MaxRetry(3))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("enqueue task: %w", err)
	}
	return info.ID, nil
}

// enqueueKnowledgeListReparse enqueues an async batch-reparse task for the
// given knowledge IDs and returns the asynq task ID.
func (h *KnowledgeHandler) enqueueKnowledgeListReparse(
	ctx context.Context, knowledgeDomainID uint64, ids []string, processConfig *types.KnowledgeProcessOverrides,
) (string, error) {
	payload := types.KnowledgeListReparsePayload{
		KnowledgeDomainID: knowledgeDomainID,
		KnowledgeIDs:      ids,
		ProcessConfig:     processConfig,
	}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(types.TypeKnowledgeListReparse, payloadBytes,
		asynq.Queue("low"), asynq.MaxRetry(3))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("enqueue task: %w", err)
	}
	return info.ID, nil
}

// CreateKnowledgeFromFile godoc
// @Summary      从文件创建知识
// @Description  上传文件并创建知识条目
// @Tags         知识管理
// @Accept       multipart/form-data
// @Produce      json
// @Param        id                path      string  true   "知识库ID"
// @Param        file              formData  file    true   "上传的文件"
// @Param        fileName          formData  string  false  "自定义文件名"
// @Param        folder_path       formData  string  false  "知识库内相对目录，例如 Policies/Finance"
// @Param        metadata          formData  string  false  "元数据JSON"
// @Param        enable_multimodel formData  bool    false  "启用多模态处理"
// @Param        tag_ids       formData  string  false  "分类ID列表，逗号分隔"
// @Param        process_config    formData  string  false  "处理配置JSON（KnowledgeProcessOverrides）"
// @Success      200               {object}  map[string]interface{}  "创建的知识"
// @Failure      400               {object}  errors.AppError         "请求参数错误"
// @Failure      409               {object}  map[string]interface{}  "文件重复"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/knowledge/file [post]
func (h *KnowledgeHandler) CreateKnowledgeFromFile(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start creating knowledge from file")

	// Creating knowledge requires effective KB management permission.
	_, kbID, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	// Check write permission
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(errors.NewForbiddenError("No permission to create knowledge"))
		return
	}

	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		logger.Error(ctx, "File upload failed", err)
		c.Error(errors.NewBadRequestError("File upload failed").WithDetails(err.Error()))
		return
	}

	// Validate file size — read MAX_FILE_SIZE_MB env (50MB default).
	// Deliberately not a runtime system_setting; see filesize.go for the
	// rationale (nginx / docreader / browser bundle all cache this at
	// container startup, so a UI knob would silently mismatch).
	maxSizeMB := utils.GetMaxFileSizeMB()
	maxSize := maxSizeMB * 1024 * 1024
	if file.Size > maxSize {
		logger.Error(ctx, "File size too large")
		c.Error(errors.NewBadRequestError(fmt.Sprintf("文件大小不能超过%dMB", maxSizeMB)))
		return
	}

	// Get custom filename and first-class folder placement.
	customFileName := c.PostForm("fileName")
	customFileName = secutils.SanitizeForLog(customFileName)
	folderPath := c.PostForm("folder_path")
	displayFileName := file.Filename
	displayFileName = secutils.SanitizeForLog(displayFileName)
	if customFileName != "" {
		displayFileName = customFileName
		logger.Infof(ctx, "Using custom filename: %s (original: %s)", customFileName, displayFileName)
	}

	logger.Infof(ctx, "File upload successful, filename: %s, size: %.2f KB", displayFileName, float64(file.Size)/1024)
	logger.Infof(ctx, "Creating knowledge, knowledge base ID: %s, filename: %s", kbID, displayFileName)

	// Parse metadata if provided
	var metadata map[string]string
	metadataStr := c.PostForm("metadata")
	if metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			logger.Error(ctx, "Failed to parse metadata", err)
			c.Error(errors.NewBadRequestError("Invalid metadata format").WithDetails(err.Error()))
			return
		}
		logger.Infof(ctx, "Received file metadata: %s", secutils.SanitizeForLog(fmt.Sprintf("%v", metadata)))
	}

	enableMultimodelForm := c.PostForm("enable_multimodel")
	var enableMultimodel *bool
	if enableMultimodelForm != "" {
		parseBool, err := strconv.ParseBool(enableMultimodelForm)
		if err != nil {
			logger.Error(ctx, "Failed to parse enable_multimodel", err)
			c.Error(errors.NewBadRequestError("Invalid enable_multimodel format").WithDetails(err.Error()))
			return
		}
		enableMultimodel = &parseBool
	}

	var processOverrides *types.KnowledgeProcessOverrides
	if raw := c.PostForm("process_config"); raw != "" {
		processOverrides = &types.KnowledgeProcessOverrides{}
		if err := json.Unmarshal([]byte(raw), processOverrides); err != nil {
			logger.Error(ctx, "Failed to parse process_config", err)
			c.Error(errors.NewBadRequestError("Invalid process_config format").WithDetails(err.Error()))
			return
		}
	}
	if enableMultimodel != nil && (processOverrides == nil || processOverrides.EnableMultimodel == nil) {
		if processOverrides == nil {
			processOverrides = &types.KnowledgeProcessOverrides{EnableMultimodel: enableMultimodel}
		} else {
			processOverrides.EnableMultimodel = enableMultimodel
		}
	}

	// 获取分类ID列表（如果提供），逗号分隔，用于知识多标签分类管理
	tagIDs := parseCommaSeparatedTagIDs(c.PostForm("tag_ids"))

	channel := c.PostForm("channel")

	// Create knowledge entry from the file
	knowledge, err := h.kgService.CreateKnowledgeFromFile(
		ctx,
		kbID,
		file,
		metadata,
		enableMultimodel,
		customFileName,
		folderPath,
		tagIDs,
		channel,
		processOverrides,
	)
	// Check for duplicate knowledge error
	if err != nil {
		if h.handleDuplicateKnowledgeError(c, err, knowledge, "file") {
			return
		}
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge created successfully, ID: %s, title: %s",
		secutils.SanitizeForLog(knowledge.ID),
		secutils.SanitizeForLog(knowledge.Title),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// CreateKnowledgeFromURL godoc
// @Summary      从URL创建知识
// @Description  从指定URL抓取内容并创建知识条目。当提供 file_name/file_type 或 URL 路径含已知文件扩展名时，自动切换为文件下载模式
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "知识库ID"
// @Param        request  body      object{url=string,file_name=string,file_type=string,enable_multimodel=bool,title=string,tag_ids=[]string}  true  "URL请求"
// @Success      201      {object}  map[string]interface{}  "创建的知识"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Failure      409      {object}  map[string]interface{}  "URL重复"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/knowledge/url [post]
func (h *KnowledgeHandler) CreateKnowledgeFromURL(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start creating knowledge from URL")

	// Creating knowledge requires effective KB management permission.
	_, kbID, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	// Check write permission
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(errors.NewForbiddenError("No permission to create knowledge"))
		return
	}

	// Parse URL from request body
	var req struct {
		URL              string                           `json:"url" binding:"required"`
		FileName         string                           `json:"file_name"`
		FileType         string                           `json:"file_type"`
		EnableMultimodel *bool                            `json:"enable_multimodel"`
		Title            string                           `json:"title"`
		TagIDs           []string                         `json:"tag_ids"`
		Channel          string                           `json:"channel"`
		ProcessConfig    *types.KnowledgeProcessOverrides `json:"process_config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse URL request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	logger.Infof(ctx, "Received URL request: %s, file_name: %s, file_type: %s",
		secutils.SanitizeForLog(req.URL),
		secutils.SanitizeForLog(req.FileName),
		secutils.SanitizeForLog(req.FileType),
	)

	// SSRF validation for user-supplied URL
	if err := secutils.ValidateURLForSSRF(req.URL); err != nil {
		logger.Warnf(ctx, "SSRF validation failed for knowledge URL: %v", err)
		c.Error(errors.NewBadRequestError(secutils.FormatSSRFError("URL", req.URL, err)))
		return
	}

	logger.Infof(ctx,
		"Creating knowledge from URL, knowledge base ID: %s, URL: %s",
		secutils.SanitizeForLog(kbID),
		secutils.SanitizeForLog(req.URL),
	)

	// Create knowledge entry from the URL
	knowledge, err := h.kgService.CreateKnowledgeFromURL(
		ctx, kbID, req.URL, req.FileName, req.FileType, req.EnableMultimodel, req.Title, req.TagIDs, req.Channel, req.ProcessConfig,
	)
	// Check for duplicate knowledge error
	if err != nil {
		if h.handleDuplicateKnowledgeError(c, err, knowledge, "url") {
			return
		}
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge created successfully from URL, ID: %s, title: %s",
		secutils.SanitizeForLog(knowledge.ID),
		secutils.SanitizeForLog(knowledge.Title),
	)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// CreateManualKnowledge godoc
// @Summary      手工创建知识
// @Description  手工录入Markdown格式的知识内容
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                       true  "知识库ID"
// @Param        request  body      types.ManualKnowledgePayload true  "手工知识内容"
// @Success      200      {object}  map[string]interface{}       "创建的知识"
// @Failure      400      {object}  errors.AppError              "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/knowledge/manual [post]
func (h *KnowledgeHandler) CreateManualKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start creating manual knowledge")

	// Creating knowledge requires effective KB management permission.
	_, kbID, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	// Check write permission
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(errors.NewForbiddenError("No permission to create knowledge"))
		return
	}

	var req types.ManualKnowledgePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse manual knowledge request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	knowledge, err := h.kgService.CreateKnowledgeFromManual(ctx, kbID, &req, req.Channel)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"kb_id": kbID,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Manual knowledge created successfully, knowledge ID: %s",
		secutils.SanitizeForLog(knowledge.ID))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// GetKnowledge godoc
// @Summary      获取知识详情
// @Description  根据ID获取知识条目详情
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "知识详情"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "知识不存在"
// @Security     Bearer
// @Router       /knowledge/{id} [get]
func (h *KnowledgeHandler) GetKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start retrieving knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Resolve knowledge and validate KB access (at least viewer)
	knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionRead)
	if err != nil {
		c.Error(err)
		return
	}

	// Re-fetch with knowledgeDomain-scoped service so tags and other joined fields are populated.
	if knowledge, err = h.kgService.GetKnowledgeByID(effCtx, id); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}

	logger.Infof(ctx, "Knowledge retrieved successfully, ID: %s, title: %s",
		secutils.SanitizeForLog(knowledge.ID), secutils.SanitizeForLog(knowledge.Title))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// GetKnowledgeSpans godoc
// @Summary      获取知识文档解析的 Span 树（含历史尝试）
// @Description  返回该知识在解析流水线的 trace tree（root → stage → subspan）：每段状态、耗时、input/output、错误码、langfuse_trace_id。支持 ?attempt=N 查看历史尝试；不传则返回最新尝试。前端用于渲染时间线 + 多模态/embedding 子节点 + 一键跳转 Langfuse。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id        path   string  true   "知识ID"
// @Param        attempt   query  int     false  "指定尝试号；省略=最新"
// @Success      200       {object}  map[string]interface{}
// @Router       /knowledge/{id}/spans [get]
// @Router       /knowledge/{id}/stages [get]
//
// Always returns the canonical 5-stage timeline; missing stage rows are
// synthesized as "pending" so the frontend timeline always renders five
// segments. Subspans (multimodal.image[i], generation.*) ride along under
// each stage as children when present.
func (h *KnowledgeHandler) GetKnowledgeSpans(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	knowledge, _, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionRead)
	if err != nil {
		c.Error(err)
		return
	}

	// Pick attempt: explicit ?attempt=N wins; otherwise pull the
	// latest attempt from the spans table. Lite-mode / fresh installs
	// with zero rows fall through to attempt=0, in which case we
	// return a placeholder tree (5 pending stages, no root, no
	// children) so the UI still renders.
	requestedAttempt := 0
	if v := strings.TrimSpace(c.Query("attempt")); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			requestedAttempt = n
		}
	}

	rows := []types.KnowledgeProcessingSpan{}
	currentAttempt := 0
	if h.spanRepo != nil {
		if requestedAttempt == 0 {
			latest, lerr := h.spanRepo.LatestAttempt(ctx, knowledge.ID)
			if lerr != nil {
				logger.Warnf(ctx, "spans LatestAttempt failed for %s: %v", knowledge.ID, lerr)
			} else {
				currentAttempt = latest
			}
		} else {
			currentAttempt = requestedAttempt
		}
		if currentAttempt > 0 {
			rows, err = h.spanRepo.ListByAttempt(ctx, knowledge.ID, currentAttempt)
			if err != nil {
				logger.Warnf(ctx, "spans ListByAttempt failed kid=%s attempt=%d: %v",
					knowledge.ID, currentAttempt, err)
				rows = nil
			}
		}
	}

	// Build tree: index by SpanID, then attach to parents. Stages
	// missing from the DB are synthesized as "pending" placeholders
	// under a synthetic (or real, if present) root so the timeline
	// always renders five segments. parse_status threads through so
	// pre-tracker historical knowledge (no rows but parse_status is
	// already terminal) renders as done/failed instead of pending —
	// otherwise legacy completed documents would forever look like
	// they're still waiting in the queue.
	tree, currentStageName, lastErr := buildSpanTree(knowledge.ID, currentAttempt, rows, knowledge.ParseStatus)

	resp := gin.H{
		"knowledge_id":    knowledge.ID,
		"parse_status":    knowledge.ParseStatus,
		"current_attempt": currentAttempt,
		"current_stage":   currentStageName,
		"trace":           tree,
	}
	if lastErr != nil {
		resp["last_error"] = gin.H{
			"stage":       lastErr.Name,
			"code":        lastErr.ErrorCode,
			"message":     lastErr.ErrorMessage,
			"finished_at": lastErr.FinishedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// buildSpanTree assembles a flat list of span rows into a parent-child
// tree rooted at the (knowledge, attempt)'s root span. Missing canonical
// stages are filled in with pending placeholders so the UI always renders
// the five timeline segments. Returns the root, the current_stage name
// (the running stage if any), and the most recent failed span if one
// exists.
//
// parseStatus is the knowledge.parse_status string. When the spans table
// has zero rows for this attempt (legacy data parsed before tracking, or
// a fresh knowledge before the pipeline starts), the placeholder status
// is inferred from parseStatus: completed → done, failed → failed,
// otherwise pending. Without this, every historical knowledge would
// render as "all 5 stages pending" forever despite having actually
// completed parsing.
func buildSpanTree(knowledgeID string, attempt int, rows []types.KnowledgeProcessingSpan, parseStatus string) (
	root *types.SpanTreeNode, currentStage string, lastFailure *types.KnowledgeProcessingSpan,
) {
	now := time.Now()
	// Build node lookup, identify root.
	nodes := make(map[string]*types.SpanTreeNode, len(rows))
	var rootRow *types.KnowledgeProcessingSpan
	stageRowByName := map[string]*types.KnowledgeProcessingSpan{}
	for i := range rows {
		r := rows[i]
		nodes[r.SpanID] = &types.SpanTreeNode{KnowledgeProcessingSpan: r}
		if r.Kind == types.SpanKindRoot && rootRow == nil {
			cp := r
			rootRow = &cp
		}
		if r.Kind == types.SpanKindStage {
			cp := r
			stageRowByName[r.Name] = &cp
		}
		if r.Status == types.SpanStatusRunning && r.Kind == types.SpanKindStage && currentStage == "" {
			currentStage = r.Name
		}
		if r.Status == types.SpanStatusFailed {
			cp := r
			lastFailure = &cp
		}
	}

	// Pick the synthesized stage status from parse_status. Without this,
	// historical knowledge that completed before span tracking was wired
	// would render as "5 pending stages" forever — the rows simply
	// weren't recorded, but parse_status correctly reads "completed".
	// The synthesized stages don't carry duration/timing data; they
	// just communicate the inferred terminal state.
	syntheticStatus := types.SpanStatusPending
	switch parseStatus {
	case types.ParseStatusCompleted:
		syntheticStatus = types.SpanStatusDone
	case types.ParseStatusFailed:
		syntheticStatus = types.SpanStatusFailed
	}

	// Synthesize root if no rows came back so the API contract stays
	// stable (frontend always expects a `trace` object).
	if rootRow == nil {
		root = &types.SpanTreeNode{KnowledgeProcessingSpan: types.KnowledgeProcessingSpan{
			KnowledgeID: knowledgeID,
			Attempt:     attempt,
			SpanID:      "",
			Name:        "knowledge_processing",
			Kind:        types.SpanKindRoot,
			Status:      syntheticStatus,
			CreatedAt:   now,
			UpdatedAt:   now,
		}}
	} else {
		root = nodes[rootRow.SpanID]
	}

	// Link real children to their parents. Walk `rows` (not the
	// `nodes` map!) so the append order matches the repo's stable
	// `ORDER BY id ASC`. Iterating the map directly would give
	// callers a different child ordering on every request — Go map
	// iteration is intentionally randomised — and the UI would
	// flicker subspans into a different order on each refresh.
	for i := range rows {
		r := &rows[i]
		n := nodes[r.SpanID]
		if n == nil || n == root {
			continue
		}
		if r.ParentSpanID == "" {
			// Real top-level row with no parent and not the root
			// itself — attach to root so it doesn't dangle.
			root.Children = append(root.Children, n)
			continue
		}
		parent, ok := nodes[r.ParentSpanID]
		if !ok {
			// Unknown parent (orphan); attach to root.
			root.Children = append(root.Children, n)
			continue
		}
		parent.Children = append(parent.Children, n)
	}

	// Synthesize missing stage rows as children of root so the timeline
	// always shows 5 segments. Status mirrors the synthesized root —
	// pending while the pipeline is still running, done/failed for
	// historical knowledge whose terminal state we know but whose
	// per-stage timing was never recorded. Appended in AllStages order
	// so the canonical stage layout is deterministic regardless of
	// which rows are missing.
	for _, name := range types.AllStages {
		if _, ok := stageRowByName[name]; ok {
			continue
		}
		placeholder := types.KnowledgeProcessingSpan{
			KnowledgeID: knowledgeID,
			Attempt:     attempt,
			Name:        name,
			Kind:        types.SpanKindStage,
			Status:      syntheticStatus,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		root.Children = append(root.Children, &types.SpanTreeNode{KnowledgeProcessingSpan: placeholder})
	}

	return root, currentStage, lastFailure
}

// ListKnowledge godoc
// @Summary      获取知识列表
// @Description  获取知识库下的知识列表，支持分页和筛选
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id         path      string  true   "知识库ID"
// @Param        page       query     int     false  "页码"
// @Param        page_size  query     int     false  "每页数量"
// @Param        tag_ids       query     string  false  "标签ID筛选，逗号分隔（OR语义）"
// @Param        keyword       query     string  false  "关键词搜索"
// @Param        file_type     query     string  false  "文件类型筛选"
// @Param        parse_status  query     string  false  "解析状态筛选 (pending/processing/completed/failed)"
// @Param        source        query     string  false  "来源/渠道筛选 (web/api/feishu/notion/yuque/wechat/...，或 manual/url 按 type 过滤)"
// @Param        start_time    query     string  false  "更新时间起点，RFC3339 格式"
// @Param        end_time      query     string  false  "更新时间终点，RFC3339 格式"
// @Success      200        {object}  map[string]interface{}  "知识列表"
// @Failure      400        {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/knowledge [get]
func (h *KnowledgeHandler) ListKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start retrieving knowledge list")

	// Validate access to the knowledge base (read access - any permission level)
	_, kbID, effectiveKnowledgeDomainID, _, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Keep the request context aligned with the authorized KB knowledgeDomain.
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	// Parse pagination parameters from query string
	var pagination types.Pagination
	if err := c.ShouldBindQuery(&pagination); err != nil {
		logger.Error(ctx, "Failed to parse pagination parameters", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	filter := types.KnowledgeListFilter{
		TagIDs:      parseCommaSeparatedTagIDs(c.Query("tag_ids")),
		Keyword:     c.Query("keyword"),
		FileType:    c.Query("file_type"),
		ParseStatus: c.Query("parse_status"),
		Source:      c.Query("source"),
	}
	if rawFolderID, exists := c.GetQuery("folder_id"); exists {
		filter.FilterByFolder = true
		if rawFolderID != "" && rawFolderID != "root" {
			filter.FolderID = rawFolderID
		}
	}
	var accessScope *types.KnowledgeBaseAccessScope
	if access, ok := middleware.KBAccessFromContext(c); ok {
		accessScope = access.Scope
	}
	if accessScope != nil && !accessScope.FullAccess {
		filter.AllowedKnowledgeIDs = append(
			[]string(nil),
			accessScope.KnowledgeIDs...,
		)
		if len(filter.AllowedKnowledgeIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"data":      []*types.Knowledge{},
				"total":     0,
				"page":      pagination.GetPage(),
				"page_size": pagination.GetPageSize(),
			})
			return
		}
	}
	if raw := c.Query("start_time"); raw != "" {
		t, err := parseFilterTime(raw)
		if err != nil {
			c.Error(errors.NewBadRequestError("invalid start_time: " + err.Error()))
			return
		}
		filter.UpdatedFrom = t
	}
	if raw := c.Query("end_time"); raw != "" {
		t, err := parseFilterTime(raw)
		if err != nil {
			c.Error(errors.NewBadRequestError("invalid end_time: " + err.Error()))
			return
		}
		filter.UpdatedTo = t
	}

	logger.Infof(
		ctx,
		"Retrieving knowledge list under knowledge base, kb_id=%s tag_ids=%s keyword=%s file_type=%s parse_status=%s source=%s start_time=%s end_time=%s page=%d page_size=%d effectiveKnowledgeDomainID=%d",
		secutils.SanitizeForLog(kbID),
		secutils.SanitizeForLog(strings.Join(filter.TagIDs, ",")),
		secutils.SanitizeForLog(filter.Keyword),
		secutils.SanitizeForLog(filter.FileType),
		secutils.SanitizeForLog(filter.ParseStatus),
		secutils.SanitizeForLog(filter.Source),
		secutils.SanitizeForLog(c.Query("start_time")),
		secutils.SanitizeForLog(c.Query("end_time")),
		pagination.Page,
		pagination.PageSize,
		effectiveKnowledgeDomainID,
	)

	// Retrieve paginated knowledge entries
	result, err := h.kgService.ListPagedKnowledgeByKnowledgeBaseID(ctx, kbID, &pagination, filter)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge list retrieved successfully, knowledge base ID: %s, total: %d",
		secutils.SanitizeForLog(kbID),
		result.Total,
	)
	if accessScope != nil {
		for _, item := range result.Data.([]*types.Knowledge) {
			switch {
			case accessScope.ManagesKnowledge(item.ID):
				item.MyPermission = types.KnowledgeBasePermissionManage
			case accessScope.AllowsKnowledge(item.ID):
				item.MyPermission = types.KnowledgeBasePermissionRead
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      result.Data,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// ListKnowledgeFolders returns the folder hierarchy visible to the current
// caller. Document-level readers only receive ancestors of authorized files.
// @Summary      获取知识库目录
// @Description  获取当前用户可见的知识库目录树
// @Tags         知识管理
// @Produce      json
// @Param        id  path  string  true  "知识库 ID"
// @Success      200  {object}  map[string]interface{}  "目录列表"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/knowledge/folders [get]
func (h *KnowledgeHandler) ListKnowledgeFolders(c *gin.Context) {
	ctx := c.Request.Context()
	_, kbID, effectiveKnowledgeDomainID, _, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	var accessScope *types.KnowledgeBaseAccessScope
	if access, ok := middleware.KBAccessFromContext(c); ok {
		accessScope = access.Scope
	}
	// Fetch the hierarchy once, then apply the already-resolved folder scope.
	// Filtering folders indirectly through visible documents would hide an
	// explicitly granted empty folder.
	folders, err := h.kgService.ListKnowledgeFolders(ctx, kbID, nil)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if accessScope != nil && !accessScope.FullAccess {
		allowedFolders := make(map[string]struct{}, len(accessScope.FolderIDs))
		for _, folderID := range accessScope.FolderIDs {
			allowedFolders[folderID] = struct{}{}
		}
		visible := make([]*types.KnowledgeFolder, 0, len(allowedFolders))
		for _, folder := range folders {
			if folder == nil {
				continue
			}
			if _, allowed := allowedFolders[folder.ID]; allowed {
				visible = append(visible, folder)
			}
		}
		folders = visible
	}
	if accessScope != nil {
		for _, folder := range folders {
			switch {
			case accessScope.ManagesFolder(folder.ID):
				folder.MyPermission = types.KnowledgeBasePermissionManage
			case accessScope.AllowsFolder(folder.ID):
				folder.MyPermission = types.KnowledgeBasePermissionRead
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    folders,
	})
}

// DeleteKnowledge godoc
// @Summary      删除知识
// @Description  根据ID异步删除知识条目。请求会被入队到与批量删除相同的异步管道（asynq）；
// @Description  接口返回 200 仅表示任务已提交（响应 data.task_id 为任务 ID），实际删除由后台 worker 完成。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "任务已提交，返回 task_id"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/{id} [delete]
func (h *KnowledgeHandler) DeleteKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionManage)
	if err != nil {
		c.Error(err)
		return
	}

	// Reuse the batch async pipeline so single-item delete shares the same
	// hardening (asynq retries, business-aware queue routing, marking-as-deleting
	// inside the worker) as BatchDeleteKnowledge / ClearKnowledgeBaseContents.
	effectiveKnowledgeDomainID, _ := effCtx.Value(types.KnowledgeDomainIDContextKey).(uint64)
	if effectiveKnowledgeDomainID == 0 {
		logger.Error(ctx, "Effective knowledgeDomain ID missing after access validation")
		c.Error(errors.NewInternalServerError("knowledgeDomain context unavailable"))
		return
	}

	logger.Infof(ctx, "Enqueuing knowledge delete, ID: %s", secutils.SanitizeForLog(id))
	taskID, err := h.enqueueKnowledgeListDelete(effCtx, effectiveKnowledgeDomainID, []string{id})
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue knowledge delete task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue delete task"))
		return
	}

	logger.Infof(ctx, "Knowledge delete task enqueued: %s, knowledge_id: %s", taskID, secutils.SanitizeForLog(id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Delete task submitted",
		"data": gin.H{
			"task_id": taskID,
		},
	})
}

// BatchDeleteKnowledgeRequest is the body schema for POST /knowledge/batch-delete.
type BatchDeleteKnowledgeRequest struct {
	KBID string   `json:"kb_id" binding:"required"`
	IDs  []string `json:"ids"  binding:"required"`
}

// BatchDeleteKnowledge godoc
// @Summary      批量删除知识
// @Description  按 ID 列表批量删除单个知识库下的多个知识条目
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      BatchDeleteKnowledgeRequest  true  "批量删除请求"
// @Success      200      {object}  map[string]interface{}       "删除成功"
// @Failure      400      {object}  errors.AppError              "请求参数错误"
// @Failure      403      {object}  errors.AppError              "权限不足"
// @Security     Bearer
// @Router       /knowledge/batch-delete [post]
func (h *KnowledgeHandler) BatchDeleteKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	var req BatchDeleteKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("Invalid request parameters: " + err.Error()))
		return
	}

	// Deduplicate and drop empty IDs.
	seen := make(map[string]struct{}, len(req.IDs))
	ids := make([]string, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.Error(errors.NewBadRequestError("ids cannot be empty"))
		return
	}
	const maxBatch = 200
	if len(ids) > maxBatch {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("too many ids (max %d per batch)", maxBatch)))
		return
	}

	// Validate effective KB management permission using the kb_id from the body.
	_, kbID, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, req.KBID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(errors.NewForbiddenError("No permission to delete knowledge"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	// Single batch fetch to validate that every id exists and belongs to the
	// requested KB. The service-layer DeleteKnowledgeList only enforces knowledgeDomain
	// scope, not KB scope, so the handler must guard against cross-KB deletion.
	knowledgeList, err := h.kgService.GetKnowledgeBatch(ctx, effectiveKnowledgeDomainID, ids)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if len(knowledgeList) != len(ids) {
		c.Error(errors.NewBadRequestError("One or more knowledge entries not found"))
		return
	}
	for _, k := range knowledgeList {
		if k.KnowledgeBaseID != kbID {
			c.Error(errors.NewBadRequestError(
				fmt.Sprintf("Knowledge %s does not belong to knowledge base %s",
					secutils.SanitizeForLog(k.ID), secutils.SanitizeForLog(kbID))))
			return
		}
	}

	taskID, err := h.enqueueKnowledgeListDelete(ctx, effectiveKnowledgeDomainID, ids)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue batch knowledge delete task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue batch delete task"))
		return
	}

	logger.Infof(ctx, "Batch knowledge delete task enqueued: %s, kb_id: %s, count: %d",
		taskID, secutils.SanitizeForLog(kbID), len(ids))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Batch delete task submitted",
		"data": gin.H{
			"task_id":       taskID,
			"deleted_count": len(ids),
		},
	})
}

// ClearKnowledgeBaseContents godoc
// @Summary      清空知识库内容
// @Description  删除知识库下的所有知识条目（异步任务）。知识库本身保留，仅清空其中的内容
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}  "清空任务已提交"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/knowledge [delete]
func (h *KnowledgeHandler) ClearKnowledgeBaseContents(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start clearing knowledge base contents")

	_, kbID, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Clearing contents requires effective management permission in the KB's department.
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(errors.NewForbiddenError("Knowledge base management permission required to clear contents"))
		return
	}

	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	knowledgeList, err := h.kgService.ListKnowledgeByKnowledgeBaseID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list knowledge entries").WithDetails(err.Error()))
		return
	}

	if len(knowledgeList) == 0 {
		logger.Infof(ctx, "Knowledge base %s is already empty", secutils.SanitizeForLog(kbID))
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Knowledge base is already empty",
			"data":    gin.H{"deleted_count": 0},
		})
		return
	}

	knowledgeIDs := make([]string, 0, len(knowledgeList))
	for _, knowledge := range knowledgeList {
		knowledgeIDs = append(knowledgeIDs, knowledge.ID)
	}

	taskID, err := h.enqueueKnowledgeListDelete(ctx, effectiveKnowledgeDomainID, knowledgeIDs)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue knowledge list delete task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue cleanup task"))
		return
	}

	logger.Infof(ctx, "Knowledge base contents clear task enqueued: %s, kb_id: %s, count: %d",
		taskID, secutils.SanitizeForLog(kbID), len(knowledgeIDs))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge base contents clear task submitted",
		"data":    gin.H{"deleted_count": len(knowledgeIDs)},
	})
}

// DownloadKnowledgeFile godoc
// @Summary      下载知识文件
// @Description  下载知识条目关联的原始文件
// @Tags         知识管理
// @Accept       json
// @Produce      application/octet-stream
// @Param        id   path      string  true  "知识ID"
// @Success      200  {file}    file    "文件内容"
// @Failure      400  {object}  errors.AppError  "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/{id}/download [get]
func (h *KnowledgeHandler) DownloadKnowledgeFile(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start downloading knowledge file")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionRead)
	if err != nil {
		c.Error(err)
		return
	}
	logger.Infof(ctx, "Retrieving knowledge file, ID: %s", secutils.SanitizeForLog(id))

	file, filename, err := h.kgService.GetKnowledgeFile(effCtx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(err.Error()))
		return
	}
	defer file.Close()

	// Record download audit (best-effort, must not block the response).
	if h.businessAudit != nil && knowledge != nil {
		h.businessAudit.RecordKnowledgeDownloaded(
			ctx,
			knowledge.ID,
			knowledge.Title,
			filename,
			0, // file size is not readily available here; 0 means unknown
			knowledge.KnowledgeBaseID,
		)
	}

	logger.Infof(
		ctx,
		"Knowledge file retrieved successfully, ID: %s, filename: %s",
		secutils.SanitizeForLog(id),
		secutils.SanitizeForLog(filename),
	)

	// Set response headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	c.Header("Content-Disposition", cd)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Expires", "0")
	c.Header("Cache-Control", "must-revalidate")
	c.Header("Pragma", "public")

	// Stream file content to response
	c.Stream(func(w io.Writer) bool {
		if _, err := io.Copy(w, file); err != nil {
			logger.Errorf(ctx, "Failed to send file: %v", err)
			return false
		}
		logger.Debug(ctx, "File sending completed")
		return false
	})
}

// mimeTypeByExt returns the MIME type for a given file extension.
func mimeTypeByExt(filename string) string {
	ct, _ := secutils.SafeContentTypeByFilename(filename)
	return ct
}

// PreviewKnowledgeFile godoc
// @Summary      预览知识文件
// @Description  返回知识条目关联的原始文件，Content-Type 根据文件类型设置，用于浏览器内嵌预览
// @Tags         知识管理
// @Accept       json
// @Produce      application/pdf,image/jpeg,image/png,text/plain
// @Param        id   path      string  true  "知识ID"
// @Success      200  {file}    file    "文件内容"
// @Failure      400  {object}  errors.AppError  "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/{id}/preview [get]
func (h *KnowledgeHandler) PreviewKnowledgeFile(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionRead)
	if err != nil {
		c.Error(err)
		return
	}

	file, filename, err := h.kgService.GetKnowledgeFile(effCtx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(err.Error()))
		return
	}
	defer file.Close()

	contentType, inline := secutils.SafeContentTypeByFilename(filename)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	disposition := "inline"
	if !inline {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	c.Header("Cache-Control", "private, max-age=3600")

	c.Stream(func(w io.Writer) bool {
		if _, err := io.Copy(w, file); err != nil {
			logger.Errorf(ctx, "Failed to stream preview: %v", err)
			return false
		}
		return false
	})
}

// GetKnowledgeBatchRequest defines parameters for batch knowledge retrieval
type GetKnowledgeBatchRequest struct {
	IDs  []string `form:"ids" binding:"required"` // List of knowledge IDs
	KBID string   `form:"kb_id"`                  // Optional: scope to this KB and validate access
}

// GetKnowledgeBatch godoc
// @Summary      批量获取知识
// @Description  根据ID列表批量获取知识条目。可选 kb_id：指定时按该知识库校验权限。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        ids       query     []string  true   "知识ID列表"
// @Param        kb_id     query     string   false  "可选，知识库ID"
// @Success      200       {object}  map[string]interface{}  "知识列表"
// @Failure      400       {object}  errors.AppError        "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/batch [get]
func (h *KnowledgeHandler) GetKnowledgeBatch(c *gin.Context) {
	ctx := c.Request.Context()

	var req GetKnowledgeBatchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	var knowledges []*types.Knowledge
	var err error
	var effectiveKnowledgeDomainID uint64

	// scopeKBID tracks the single KB the results must belong to (set by explicit kb_id).
	var scopeKBID string

	// Optional kb_id: validate KB access.
	if kbID := secutils.SanitizeForLog(req.KBID); kbID != "" {
		_, _, effID, _, accessErr := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
		if accessErr != nil {
			c.Error(accessErr)
			return
		}
		effectiveKnowledgeDomainID = effID
		scopeKBID = kbID
		ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

		logger.Infof(ctx, "Batch retrieving knowledge with kb_id, effective knowledgeDomain ID: %d, IDs count: %d",
			effectiveKnowledgeDomainID, len(req.IDs))

		knowledges, err = h.kgService.GetKnowledgeBatch(ctx, effectiveKnowledgeDomainID, req.IDs)
	} else {
		// No kb_id: resolve each document globally and authorize it against the
		// current user's direct or organization grants.
		knowledges = make([]*types.Knowledge, 0, len(req.IDs))
		for _, id := range req.IDs {
			knowledge, _, accessErr := h.resolveKnowledgeAndValidateKBAccess(
				c,
				id,
				types.KnowledgeBasePermissionRead,
			)
			if accessErr != nil {
				continue
			}
			knowledges = append(knowledges, knowledge)
		}
	}

	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve knowledge list").WithDetails(err.Error()))
		return
	}

	// Build the effective allowed-KB set from the explicit kb_id or readable KBs.
	var allowedKBSet map[string]bool
	if scopeKBID != "" {
		allowedKBSet = map[string]bool{scopeKBID: true}
	} else {
		readableKBs, listErr := h.kbService.ListKnowledgeBases(ctx)
		if listErr != nil {
			logger.ErrorWithFields(ctx, listErr, nil)
			c.Error(errors.NewInternalServerError("Failed to verify knowledge base access").WithDetails(listErr.Error()))
			return
		}
		allowedKBSet = make(map[string]bool, len(readableKBs))
		for _, id := range knowledgeBaseIDs(readableKBs) {
			allowedKBSet[id] = true
		}
	}
	if allowedKBSet != nil {
		knowledges = filterKnowledgesByKBAllowSet(knowledges, allowedKBSet)
	}

	logger.Infof(ctx, "Batch knowledge retrieval successful, requested count: %d, returned count: %d",
		len(req.IDs), len(knowledges))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledges,
	})
}

// UpdateKnowledge godoc
// @Summary      更新知识
// @Description  更新知识条目信息
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string          true  "知识ID"
// @Param        request  body      types.Knowledge true  "知识信息"
// @Success      200      {object}  map[string]interface{}  "更新成功"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/{id} [put]
func (h *KnowledgeHandler) UpdateKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionManage)
	if err != nil {
		c.Error(err)
		return
	}

	var knowledge types.Knowledge
	if err := c.ShouldBindJSON(&knowledge); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	knowledge.ID = id

	if err := h.kgService.UpdateKnowledge(effCtx, &knowledge); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge updated successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge chunk updated successfully",
	})
}

// UpdateManualKnowledge godoc
// @Summary      更新手工知识
// @Description  更新手工录入的Markdown知识内容
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                       true  "知识ID"
// @Param        request  body      types.ManualKnowledgePayload true  "手工知识内容"
// @Success      200      {object}  map[string]interface{}       "更新后的知识"
// @Failure      400      {object}  errors.AppError              "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/manual/{id} [put]
func (h *KnowledgeHandler) UpdateManualKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start updating manual knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionManage)
	if err != nil {
		c.Error(err)
		return
	}

	var req types.ManualKnowledgePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse manual knowledge update request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	knowledge, err := h.kgService.UpdateManualKnowledge(effCtx, id, &req)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Manual knowledge updated successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// ReparseKnowledge godoc
// @Summary      重新解析知识
// @Description  删除知识中现有的文档内容并重新解析，使用异步任务方式处理
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Param        body body      object  false  "可选的处理配置覆盖：{\"process_config\": KnowledgeProcessOverrides}"
// @Success      200  {object}  map[string]interface{}  "重新解析任务已提交"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Router       /knowledge/{id}/reparse [post]
func (h *KnowledgeHandler) ReparseKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start re-parsing knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Validate KB access with editor permission (reparse requires write access)
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionManage)
	if err != nil {
		c.Error(err)
		return
	}

	// Optional per-reparse parse config override. Empty body keeps the
	// overrides stored at upload time.
	var processOverrides *types.KnowledgeProcessOverrides
	if c.Request.ContentLength != 0 {
		var req struct {
			ProcessConfig *types.KnowledgeProcessOverrides `json:"process_config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Error(ctx, "Failed to parse reparse request body", err)
			c.Error(errors.NewBadRequestError("Invalid reparse request body").WithDetails(err.Error()))
			return
		}
		processOverrides = req.ProcessConfig
	}

	// Call service to reparse knowledge
	knowledge, err := h.kgService.ReparseKnowledge(effCtx, id, processOverrides)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge reparse task submitted successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge reparse task submitted",
		"data":    knowledge,
	})
}

// CancelKnowledgeParse godoc
// @Summary      取消知识解析
// @Description  取消进行中的知识解析任务。当前已写入的 chunk / 索引保留，可通过 reparse 接口重新触发解析。已完成 / 已失败 / 删除中的知识不支持取消。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "取消已提交"
// @Failure      400  {object}  errors.AppError         "状态不支持取消"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Failure      404  {object}  errors.AppError         "知识不存在"
// @Security     Bearer
// @Router       /knowledge/{id}/cancel-parse [post]
func (h *KnowledgeHandler) CancelKnowledgeParse(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start cancelling knowledge parse")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Editor permission — same gate as ReparseKnowledge / DeleteKnowledge.
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionManage)
	if err != nil {
		c.Error(err)
		return
	}

	knowledge, err := h.kgService.CancelKnowledgeParse(effCtx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge parse cancelled successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge parse cancelled",
		"data":    knowledge,
	})
}

type knowledgeTagBatchRequest struct {
	Updates map[string][]string `json:"updates" binding:"required,min=1"`
	KBID    string              `json:"kb_id"` // Optional: scope to this KB and validate manage access
}

// UpdateKnowledgeTagBatch godoc
// @Summary      批量更新知识标签
// @Description  批量更新知识条目的标签。可选 kb_id：指定时按该知识库校验管理权限
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      object  true  "标签更新请求（updates 必填，kb_id 可选）"
// @Success      200      {object}  map[string]interface{}  "更新成功"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/tags [put]
func (h *KnowledgeHandler) UpdateKnowledgeTagBatch(c *gin.Context) {
	ctx := c.Request.Context()

	// Ensure knowledgeDomain ID is in context (service reads it; may be missing if request context was not set by auth)
	var req knowledgeTagBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse knowledge tag batch request", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}
	// Resolve effective knowledgeDomain and the authorized KB scope.
	var authorizedKBID string
	if kbID := secutils.SanitizeForLog(req.KBID); kbID != "" {
		_, _, effID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
		if err != nil {
			c.Error(err)
			return
		}
		if permission != types.KnowledgeBasePermissionManage {
			c.Error(errors.NewForbiddenError("No permission to update knowledge tags"))
			return
		}
		authorizedKBID = kbID
		ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effID)
	} else if len(req.Updates) > 0 {
		// No kb_id: infer from first knowledge ID so shared-KB updates work without client sending kb_id
		var firstKnowledgeID string
		for id := range req.Updates {
			firstKnowledgeID = id
			break
		}
		if firstKnowledgeID != "" {
			knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, firstKnowledgeID, types.KnowledgeBasePermissionManage)
			if err != nil {
				c.Error(err)
				return
			}
			authorizedKBID = knowledge.KnowledgeBaseID
			ctx = effCtx
		}
	}
	if err := h.kgService.UpdateKnowledgeTagBatch(ctx, authorizedKBID, req.Updates); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// UpdateImageInfo godoc
// @Summary      更新图像信息
// @Description  更新知识分块的图像信息
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id        path      string  true  "知识ID"
// @Param        chunk_id  path      string  true  "分块ID"
// @Param        request   body      object{image_info=string}  true  "图像信息"
// @Success      200       {object}  map[string]interface{}     "更新成功"
// @Failure      400       {object}  errors.AppError            "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/image/{id}/{chunk_id} [put]
func (h *KnowledgeHandler) UpdateImageInfo(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start updating image info")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}
	chunkID := secutils.SanitizeForLog(c.Param("chunk_id"))
	if chunkID == "" {
		logger.Error(ctx, "Chunk ID is empty")
		c.Error(errors.NewBadRequestError("Chunk ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.KnowledgeBasePermissionManage)
	if err != nil {
		c.Error(err)
		return
	}

	var request struct {
		ImageInfo string `json:"image_info"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	logger.Infof(ctx, "Updating knowledge chunk, knowledge ID: %s, chunk ID: %s", id, chunkID)
	err = h.kgService.UpdateImageInfo(effCtx, id, chunkID, secutils.SanitizeForLog(request.ImageInfo))
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge chunk updated successfully, knowledge ID: %s, chunk ID: %s", id, chunkID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge chunk image updated successfully",
	})
}

// SearchKnowledge godoc
// @Summary      Search knowledge
// @Description  Search knowledge files by keyword. Pass recent=true without a keyword to browse recent files.
// @Tags         Knowledge
// @Accept       json
// @Produce      json
// @Param        keyword    query     string  false "Keyword to search"
// @Param        offset     query     int     false "Offset for pagination"
// @Param        limit      query     int     false "Limit for pagination (default 20)"
// @Param        file_types query     string  false "Comma-separated file extensions to filter (e.g., csv,xlsx)"
// @Param        recent     query     bool    false "Return recent files when keyword is empty"
// @Success      200         {object}  map[string]interface{}     "Search results"
// @Failure      400         {object}  errors.AppError            "Invalid request"
// @Security     Bearer
// @Router       /knowledge/search [get]
func (h *KnowledgeHandler) SearchKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	if userID, ok := c.Get(types.UserIDContextKey.String()); ok {
		ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
	}
	// Accept both ?keyword= (legacy / upstream name) and ?query= (what most
	// MCP / agent integrations send). Empty input is only valid for an explicit
	// recent-file browse request; ordinary callers still get a clear 400 instead
	// of silently receiving the same newest cards for every missing query.
	keyword := c.Query("keyword")
	if keyword == "" {
		keyword = c.Query("query")
	}
	recent, _ := strconv.ParseBool(c.DefaultQuery("recent", "false"))
	if strings.TrimSpace(keyword) == "" && !recent {
		c.Error(errors.NewBadRequestError("missing search keyword: pass ?keyword=... or ?query=..."))
		return
	}
	keyword = strings.TrimSpace(keyword)
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	var fileTypes []string
	if fileTypesStr := c.Query("file_types"); fileTypesStr != "" {
		for _, ft := range strings.Split(fileTypesStr, ",") {
			ft = strings.TrimSpace(ft)
			if ft != "" {
				fileTypes = append(fileTypes, ft)
			}
		}
	}

	// Default: directly readable knowledge bases in the current knowledgeDomain.
	knowledges, hasMore, total, err := h.kgService.SearchKnowledge(ctx, keyword, offset, limit, fileTypes)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to search knowledge").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"data":     knowledges,
		"has_more": hasMore,
		"total":    total,
	})
}

// MoveKnowledgeRequest defines the request for moving knowledge items
type MoveKnowledgeRequest struct {
	KnowledgeIDs []string `json:"knowledge_ids" binding:"required,min=1"`
	SourceKBID   string   `json:"source_kb_id"  binding:"required"`
	TargetKBID   string   `json:"target_kb_id"  binding:"required"`
	Mode         string   `json:"mode"          binding:"required,oneof=reuse_vectors reparse"`
}

// MoveKnowledgeResponse defines the response for move knowledge
type MoveKnowledgeResponse struct {
	TaskID         string `json:"task_id"`
	SourceKBID     string `json:"source_kb_id"`
	TargetKBID     string `json:"target_kb_id"`
	KnowledgeCount int    `json:"knowledge_count"`
	Message        string `json:"message"`
}

// MoveKnowledge moves knowledge items from one knowledge base to another (async task).
//
// MoveKnowledge godoc
// @Summary      移动知识到其他知识库
// @Description  将一条或多条知识从源知识库移动到目标知识库（异步），返回任务 ID 用于查询进度
// @Tags         知识
// @Accept       json
// @Produce      json
// @Param        request  body      handler.MoveKnowledgeRequest  true  "{source_kb_id, target_kb_id, knowledge_ids}"
// @Success      200      {object}  handler.MoveKnowledgeResponse  "任务信息"
// @Failure      400      {object}  errors.AppError                "请求参数错误"
// @Security     Bearer
// @Router       /knowledge/move [post]
func (h *KnowledgeHandler) MoveKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	var req MoveKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "MoveKnowledge: failed to parse request", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters: " + err.Error()))
		return
	}

	// Validate source != target
	if req.SourceKBID == req.TargetKBID {
		c.Error(errors.NewBadRequestError("Source and target knowledge base cannot be the same"))
		return
	}

	// Validate source KB
	sourceKB, err := h.kbService.GetKnowledgeBaseByID(ctx, req.SourceKBID)
	if err != nil {
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(errors.NewNotFoundError("Source knowledge base not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, req.SourceKBID); err != nil {
		c.Error(err)
		return
	}

	// Validate target KB
	targetKB, err := h.kbService.GetKnowledgeBaseByID(ctx, req.TargetKBID)
	if err != nil {
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(errors.NewNotFoundError("Target knowledge base not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, req.TargetKBID); err != nil {
		c.Error(err)
		return
	}

	// Validate type match
	if sourceKB.Type != targetKB.Type {
		c.Error(errors.NewBadRequestError("Source and target knowledge bases must be the same type"))
		return
	}
	if sourceKB.KnowledgeDomainID != targetKB.KnowledgeDomainID {
		c.Error(errors.NewBadRequestError("Moving documents across knowledgeDomains is not supported"))
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, sourceKB.KnowledgeDomainID)

	// Validate embedding model match
	if sourceKB.EmbeddingModelID != targetKB.EmbeddingModelID {
		c.Error(errors.NewBadRequestError("Source and target must use the same embedding model"))
		return
	}

	// reuse_vectors copies index entries directly between KBs, which only works
	// inside the same VectorStore backend. A cross-store reuse_vectors move would
	// route CopyIndices through the SOURCE store and then delete the source
	// indices, corrupting the vector data. Reject it and point the caller at
	// reparse mode, which re-indexes into the target store safely.
	if req.Mode == "reuse_vectors" && !sourceKB.SharesStoreWith(targetKB) {
		c.Error(errors.NewBadRequestError(
			"reuse_vectors move across different vector stores is not supported; " +
				"use reparse mode to move into a different store"))
		return
	}

	// Validate all knowledge IDs belong to source KB and are in completed status
	for _, kID := range req.KnowledgeIDs {
		knowledge, err := h.kgService.GetKnowledgeByID(ctx, kID)
		if err != nil {
			c.Error(errors.NewBadRequestError(fmt.Sprintf("Knowledge item %s not found", kID)))
			return
		}
		if knowledge.KnowledgeBaseID != req.SourceKBID {
			c.Error(errors.NewBadRequestError(fmt.Sprintf("Knowledge item %s does not belong to the source knowledge base", kID)))
			return
		}
		if knowledge.ParseStatus != types.ParseStatusCompleted {
			c.Error(errors.NewBadRequestError(fmt.Sprintf("Knowledge item %s is not in completed status (current: %s)", kID, knowledge.ParseStatus)))
			return
		}
	}

	// Generate task ID
	taskID := utils.GenerateTaskID("kg_move", sourceKB.KnowledgeDomainID, req.SourceKBID)

	// Create move payload
	payload := types.KnowledgeMovePayload{
		KnowledgeDomainID: sourceKB.KnowledgeDomainID,
		TaskID:            taskID,
		KnowledgeIDs:      req.KnowledgeIDs,
		SourceKBID:        req.SourceKBID,
		TargetKBID:        req.TargetKBID,
		Mode:              req.Mode,
	}
	langfuse.InjectTracing(ctx, &payload)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf(ctx, "MoveKnowledge: failed to marshal payload: %v", err)
		c.Error(errors.NewInternalServerError("Failed to create task"))
		return
	}

	// Enqueue move task
	task := asynq.NewTask(types.TypeKnowledgeMove, payloadBytes,
		asynq.TaskID(taskID), asynq.Queue("default"), asynq.MaxRetry(3))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "MoveKnowledge: failed to enqueue task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue task"))
		return
	}

	logger.Infof(ctx, "MoveKnowledge: task enqueued: %s, asynq_id: %s, source: %s, target: %s, count: %d",
		taskID, info.ID, secutils.SanitizeForLog(req.SourceKBID), secutils.SanitizeForLog(req.TargetKBID), len(req.KnowledgeIDs))

	// Save initial progress
	initialProgress := &types.KnowledgeMoveProgress{
		TaskID:     taskID,
		SourceKBID: req.SourceKBID,
		TargetKBID: req.TargetKBID,
		Status:     types.KBCloneStatusPending,
		Total:      len(req.KnowledgeIDs),
		Progress:   0,
		Message:    "Task queued, waiting to start...",
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
	}
	if err := h.kgService.SaveKnowledgeMoveProgress(ctx, initialProgress); err != nil {
		logger.Warnf(ctx, "MoveKnowledge: failed to save initial progress: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": MoveKnowledgeResponse{
			TaskID:         taskID,
			SourceKBID:     req.SourceKBID,
			TargetKBID:     req.TargetKBID,
			KnowledgeCount: len(req.KnowledgeIDs),
			Message:        "Knowledge move task started",
		},
	})
}

// GetKnowledgeMoveProgress retrieves the progress of a knowledge move task.
//
// GetKnowledgeMoveProgress godoc
// @Summary      获取知识移动进度
// @Description  按任务 ID 查询移动进度
// @Tags         知识
// @Produce      json
// @Param        task_id  path      string                       true  "移动任务 ID"
// @Success      200      {object}  types.KnowledgeMoveProgress  "进度信息"
// @Failure      404      {object}  errors.AppError              "任务不存在"
// @Security     Bearer
// @Router       /knowledge/move/progress/{task_id} [get]
func (h *KnowledgeHandler) GetKnowledgeMoveProgress(c *gin.Context) {
	ctx := c.Request.Context()

	taskID := c.Param("task_id")
	if taskID == "" {
		c.Error(errors.NewBadRequestError("Task ID cannot be empty"))
		return
	}
	if err := requireTaskProgressKnowledgeDomain(ctx, taskID); err != nil {
		c.Error(err)
		return
	}

	progress, err := h.kgService.GetKnowledgeMoveProgress(ctx, taskID)
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

// parseCommaSeparatedTagIDs splits a comma-separated string of tag IDs and
// filters out empty strings and the "__untagged__" sentinel value.
func parseCommaSeparatedTagIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "__untagged__" {
			continue
		}
		result = append(result, p)
	}
	return result
}

// parseFilterTime parses a query-string timestamp accepted by knowledge list
// filters. It supports RFC3339, RFC3339 with milliseconds, and the date-only
// "2006-01-02" form (interpreted at start of day in the local timezone).
func parseFilterTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

type batchReparseKnowledgeRequest struct {
	KBID          string                           `json:"kb_id" binding:"required"`
	IDs           []string                         `json:"ids" binding:"required"`
	ProcessConfig *types.KnowledgeProcessOverrides `json:"process_config,omitempty"`
}

// BatchReparseKnowledge godoc
// @Summary      批量重新解析知识
// @Description  按 ID 列表批量重新解析单个知识库下的多个知识条目
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      batchReparseKnowledgeRequest  true  "批量重解析请求"
// @Success      200      {object}  map[string]interface{}        "任务已提交"
// @Failure      400      {object}  errors.AppError               "请求参数错误"
// @Failure      403      {object}  errors.AppError               "权限不足"
// @Security     Bearer
// @Router       /knowledge/batch-reparse [post]
func (h *KnowledgeHandler) BatchReparseKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	var req batchReparseKnowledgeRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "failed to parse batch reparse knowledge request: %v", err)
		c.Error(errors.NewBadRequestError("invalid batch reparse knowledge request parameters"))
		return
	}

	seen := make(map[string]struct{}, len(req.IDs))
	ids := make([]string, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		c.Error(errors.NewBadRequestError("no knowledge IDs provided for batch reparse"))
		return
	}
	const maxBatch = 200
	if len(ids) > maxBatch {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("too many ids (max %d per batch)", maxBatch)))
		return
	}

	_, kbID, effectiveKnowledgeDomainID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, req.KBID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.KnowledgeBasePermissionManage {
		c.Error(errors.NewForbiddenError("no permission to reparse knowledge in this kb"))
		return
	}
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, effectiveKnowledgeDomainID)

	knowledgeList, err := h.kgService.GetKnowledgeBatch(ctx, effectiveKnowledgeDomainID, ids)
	if err != nil {
		logger.Errorf(ctx, "failed to get knowledge batch, kb_id: %s, size: %d, err: %v", kbID, len(ids), err)
		c.Error(errors.NewInternalServerError("failed to get knowledge batch"))
		return
	}
	if len(knowledgeList) != len(ids) {
		c.Error(errors.NewBadRequestError("some knowledge entries were not found"))
		return
	}
	for _, k := range knowledgeList {
		if k.KnowledgeBaseID != kbID {
			c.Error(errors.NewBadRequestError(
				fmt.Sprintf("Knowledge %s does not belong to knowledge base %s",
					secutils.SanitizeForLog(k.ID), secutils.SanitizeForLog(kbID))))
			return
		}
	}

	taskID, err := h.enqueueKnowledgeListReparse(ctx, effectiveKnowledgeDomainID, ids, req.ProcessConfig)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue batch knowledge reparse task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue batch reparse task"))
		return
	}

	logger.Infof(ctx, "Batch knowledge reparse task enqueued: %s, kb_id: %s, count: %d",
		taskID, secutils.SanitizeForLog(kbID), len(ids))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Batch reparse task submitted",
		"data": gin.H{
			"task_id":       taskID,
			"reparse_count": len(ids),
		},
	})
}

func filterKnowledgesByKBAllowSet(knowledges []*types.Knowledge, allowed map[string]bool) []*types.Knowledge {
	if allowed == nil {
		return knowledges
	}
	filtered := make([]*types.Knowledge, 0, len(knowledges))
	for _, k := range knowledges {
		if k != nil && allowed[k.KnowledgeBaseID] {
			filtered = append(filtered, k)
		}
	}
	return filtered
}

func knowledgeBaseIDs(kbs []*types.KnowledgeBase) []string {
	ids := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb != nil && kb.ID != "" {
			ids = append(ids, kb.ID)
		}
	}
	return ids
}
