package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// AuditLogHandler exposes audit events scoped to one knowledgeDomain.
type AuditLogHandler struct {
	auditService interfaces.AuditLogService
}

// NewAuditLogHandler constructs the handler.
func NewAuditLogHandler(auditService interfaces.AuditLogService) *AuditLogHandler {
	return &AuditLogHandler{auditService: auditService}
}

// auditLogListResponse is the response envelope for ListKnowledgeDomainAuditLog.
// Uses a data array plus an opaque cursor (here
// the integer id of the last entry, or 0 if no more rows).
type auditLogListResponse struct {
	Success    bool              `json:"success"`
	Data       []*types.AuditLog `json:"data"`
	NextCursor uint64            `json:"next_cursor"`
}

// ListKnowledgeDomainAuditLog godoc
// @Summary      获取知识域审计日志
// @Description  返回该知识域最近的审计事件，按 id 倒序。游标分页：将上次响应的 next_cursor 作为下一次请求的 after_id。
// @Tags         审计日志
// @Produce      json
// @Param        id        path   string  true   "知识域ID"
// @Param        after_id  query  int     false  "游标：返回 id 小于此值的记录（默认从最新开始）"
// @Param        limit     query  int     false  "页大小，1-100，默认 50"
// @Param        action    query  string  false  "按 action 精确过滤（如 rbac.member_added / rbac.access_denied）"
// @Param        outcome   query  string  false  "按 outcome 精确过滤（success / denied）"
// @Param        actor     query  string  false  "按 actor_user_id 精确过滤"
// @Success      200  {object}  auditLogListResponse
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledge-domains/{id}/audit-log [get]
func (h *AuditLogHandler) ListKnowledgeDomainAuditLog(c *gin.Context) {
	ctx := c.Request.Context()

	// after_id cursor — invalid values are tolerated (treated as "from
	// the top") so a misconfigured client doesn't see a hard 400 on
	// the empty / first request. Tighter validation belongs at the
	// frontend.
	var afterID uint64
	if raw := c.Query("after_id"); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			afterID = v
		}
	}
	limit := 0 // 0 lets the repository pick its default (50)
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}

	q := &interfaces.AuditLogQuery{
		AfterID:     afterID,
		Limit:       limit,
		Action:      types.AuditAction(c.Query("action")),
		Outcome:     types.AuditOutcome(c.Query("outcome")),
		ActorUserID: c.Query("actor"),
	}

	entries, err := h.auditService.List(ctx, q)
	if err != nil {
		logger.Error(ctx, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// next_cursor is the smallest id in the page (since rows are sorted
	// id DESC). Empty page ⇒ 0, telling the client there's nothing
	// older to fetch.
	var nextCursor uint64
	if n := len(entries); n > 0 {
		nextCursor = entries[n-1].ID
	}

	c.JSON(http.StatusOK, auditLogListResponse{
		Success:    true,
		Data:       entries,
		NextCursor: nextCursor,
	})
}

// ListSystemAuditLog godoc
// @Summary      获取平台审计日志
// @Description  返回平台级审计事件，覆盖 系统.设置变更 / 系统.管理员提升 / 系统.管理员撤销 等 SystemAdmin 操作。按 id 倒序的游标分页。
// @Tags         审计日志
// @Produce      json
// @Param        after_id  query  int     false  "游标：返回 id 小于此值的记录（默认从最新开始）"
// @Param        limit     query  int     false  "页大小，1-100，默认 50"
// @Param        action    query  string  false  "按 action 精确过滤（如 system.setting_changed）"
// @Param        outcome   query  string  false  "按 outcome 精确过滤（success / denied）"
// @Param        actor     query  string  false  "按 actor_user_id 精确过滤"
// @Success      200  {object}  auditLogListResponse
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /system/admin/audit-log [get]
//
// Mounted on /api/v1/system/admin/audit-log under the SystemAdmin()
// guard. The audit_logs table is flat — no knowledge_domain_id column —
// so this endpoint returns the platform-wide feed (see audit_log.go for
// the action constants).
func (h *AuditLogHandler) ListSystemAuditLog(c *gin.Context) {
	ctx := c.Request.Context()

	// Cursor / page-size parsing mirrors ListKnowledgeDomainAuditLog so the
	// frontend can share the same call shape; tolerant of garbage
	// because the empty / first request shouldn't bounce.
	var afterID uint64
	if raw := c.Query("after_id"); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			afterID = v
		}
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}

	q := &interfaces.AuditLogQuery{
		AfterID:     afterID,
		Limit:       limit,
		Action:      types.AuditAction(c.Query("action")),
		Outcome:     types.AuditOutcome(c.Query("outcome")),
		ActorUserID: c.Query("actor"),
	}

	entries, err := h.auditService.List(ctx, q)
	if err != nil {
		logger.Error(ctx, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	var nextCursor uint64
	if n := len(entries); n > 0 {
		nextCursor = entries[n-1].ID
	}

	c.JSON(http.StatusOK, auditLogListResponse{
		Success:    true,
		Data:       entries,
		NextCursor: nextCursor,
	})
}

// GetUserAuditLog godoc
// @Summary      获取用户审计日志
// @Description  返回指定用户产生的审计事件（按 actor_user_id 过滤），按 id 倒序游标分页。用于用户详情页下方的审计日志列表。
// @Tags         审计日志
// @Produce      json
// @Param        id        path   string  true   "用户ID"
// @Param        after_id  query  int     false  "游标：返回 id 小于此值的记录（默认从最新开始）"
// @Param        limit     query  int     false  "页大小，1-100，默认 50"
// @Param        action    query  string  false  "按 action 精确过滤"
// @Param        outcome   query  string  false  "按 outcome 精确过滤（success / denied）"
// @Success      200  {object}  auditLogListResponse
// @Failure      400  {object}  errors.AppError
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /system/admin/users/{id}/audit-log [get]
func (h *AuditLogHandler) GetUserAuditLog(c *gin.Context) {
	ctx := c.Request.Context()

	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		c.Error(errors.NewBadRequestError("user id is required"))
		return
	}

	var afterID uint64
	if raw := c.Query("after_id"); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			afterID = v
		}
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}

	q := &interfaces.AuditLogQuery{
		AfterID:     afterID,
		Limit:       limit,
		Action:      types.AuditAction(c.Query("action")),
		Outcome:     types.AuditOutcome(c.Query("outcome")),
		ActorUserID: userID,
	}

	entries, err := h.auditService.List(ctx, q)
	if err != nil {
		logger.Error(ctx, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	var nextCursor uint64
	if n := len(entries); n > 0 {
		nextCursor = entries[n-1].ID
	}

	c.JSON(http.StatusOK, auditLogListResponse{
		Success:    true,
		Data:       entries,
		NextCursor: nextCursor,
	})
}
