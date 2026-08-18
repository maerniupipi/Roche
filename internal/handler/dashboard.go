package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/application/service"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/response"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// Concrete envelopes keep Swagger generation compatible with the response
// package's interface{} data field. They are documentation-only types.
type dashboardKnowledgeBaseStatsResponse struct {
	Code int                               `json:"code"`
	Msg  string                            `json:"msg"`
	Data types.DashboardKnowledgeBaseStats `json:"data"`
}

type dashboardChatStatsResponse struct {
	Code int                      `json:"code"`
	Msg  string                   `json:"msg"`
	Data types.DashboardChatStats `json:"data"`
}

type dashboardOverviewResponse struct {
	Code int                     `json:"code"`
	Msg  string                  `json:"msg"`
	Data types.DashboardOverview `json:"data"`
}

// DashboardHandler serves analytics for the platform dashboard.
type DashboardHandler struct {
	svc      interfaces.DashboardService
	statsSvc *service.DashboardStatsService
}

// NewDashboardHandler creates a new dashboard handler.
func NewDashboardHandler(svc interfaces.DashboardService, statsSvc *service.DashboardStatsService) *DashboardHandler {
	return &DashboardHandler{svc: svc, statsSvc: statsSvc}
}

// GetKnowledgeBaseStats returns document lifecycle counters for a knowledge base.
//
// @Summary      知识库文件统计
// @Description  返回指定知识库（或全部）的已上架、上传成功/失败、预约上架、已下架、归档文件数
// @Tags         dashboard
// @Produce      json
// @Param        knowledge_base_id  query  string  false  "知识库ID，不传表示全部"
// @Success      200  {object}  dashboardKnowledgeBaseStatsResponse
// @Security     Bearer
// @Router       /dashboard/knowledge-base-stats [get]
func (h *DashboardHandler) GetKnowledgeBaseStats(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := c.Query("knowledge_base_id")

	stats, err := h.svc.GetKnowledgeBaseStats(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"knowledge_base_id": kbID})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	response.Success(c, stats)
}

// GetChatStats returns conversation metrics over a date range.
//
// @Summary      问答统计
// @Description  返回指定日期范围内的平均响应时长、每日问答总数、提问人数、满意度
// @Tags         dashboard
// @Produce      json
// @Param        knowledge_domain_id  query  uint64  false  "知识域ID"
// @Param        start_date           query  string  true   "开始日期 YYYY-MM-DD"
// @Param        end_date             query  string  true   "结束日期 YYYY-MM-DD"
// @Success      200  {object}  dashboardChatStatsResponse
// @Security     Bearer
// @Router       /dashboard/chat-stats [get]
func (h *DashboardHandler) GetChatStats(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, start, end, ok := parseDashboardDateRange(c)
	if !ok {
		return
	}

	stats, err := h.svc.GetChatStats(ctx, knowledgeDomainID, start, end)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"knowledge_domain_id": knowledgeDomainID})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	response.Success(c, stats)
}

// GetOverview returns cross-domain aggregated analytics.
//
// @Summary      数据总览
// @Description  返回问题领域分布、跨领域回答占比、热门文档、产品反馈、提问用户榜、有效回答率、兜底问题
// @Tags         dashboard
// @Produce      json
// @Param        knowledge_domain_id  query  uint64  false  "知识域ID"
// @Param        start_date           query  string  true   "开始日期 YYYY-MM-DD"
// @Param        end_date             query  string  true   "结束日期 YYYY-MM-DD"
// @Success      200  {object}  dashboardOverviewResponse
// @Security     Bearer
// @Router       /dashboard/overview [get]
func (h *DashboardHandler) GetOverview(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeDomainID, start, end, ok := parseDashboardDateRange(c)
	if !ok {
		return
	}

	overview, err := h.svc.GetOverview(ctx, knowledgeDomainID, start, end)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"knowledge_domain_id": knowledgeDomainID})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	response.Success(c, overview)
}

// RecomputeStats recomputes one UTC day (default: yesterday) into
// dashboard_daily_stats and overwrites the existing rows. It exists so ops can
// trigger a manual recompute when source data changed after a day was already
// aggregated (e.g. deleted messages or adjusted knowledge lifecycle states).
//
// @Summary      手动重算仪表盘日聚合
// @Description  重新计算指定日期（默认昨天，UTC）的仪表盘聚合数据并覆盖写入 dashboard_daily_stats，供运维修正历史聚合
// @Tags         dashboard
// @Produce      json
// @Param        date  query  string  false  "日期 YYYY-MM-DD，默认昨天（UTC）"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /dashboard/stats/recompute [post]
func (h *DashboardHandler) RecomputeStats(c *gin.Context) {
	ctx := c.Request.Context()
	if h.statsSvc == nil {
		c.Error(errors.NewInternalServerError("dashboard stats service unavailable"))
		return
	}

	day := time.Now().UTC().Truncate(24*time.Hour).Add(-24 * time.Hour)
	if raw := c.Query("date"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			c.Error(errors.NewBadRequestError("invalid date format, expected YYYY-MM-DD"))
			return
		}
		day = parsed
	}

	dateKey := day.Format("2006-01-02")
	if err := h.statsSvc.ComputeDay(ctx, day); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"stat_date": dateKey})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	logger.Infof(ctx, "[Dashboard] manual recompute of %s completed", dateKey)
	response.Success(c, gin.H{"date": dateKey, "recomputed": true})
}

func parseDashboardDateRange(c *gin.Context) (uint64, time.Time, time.Time, bool) {
	knowledgeDomainID := uint64(0)
	if raw := c.Query("knowledge_domain_id"); raw != "" {
		if v, err := parseUint64(raw); err == nil {
			knowledgeDomainID = v
		}
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" || endDate == "" {
		c.Error(errors.NewBadRequestError("start_date and end_date are required"))
		return 0, time.Time{}, time.Time{}, false
	}

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		c.Error(errors.NewBadRequestError("invalid start_date format"))
		return 0, time.Time{}, time.Time{}, false
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		c.Error(errors.NewBadRequestError("invalid end_date format"))
		return 0, time.Time{}, time.Time{}, false
	}
	end = end.Add(24 * time.Hour).Add(-time.Nanosecond)

	return knowledgeDomainID, start, end, true
}

func parseUint64(s string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
