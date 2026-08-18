package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type AdminAnswerRecordHandler struct {
	service interfaces.AdminAnswerRecordService
}

func NewAdminAnswerRecordHandler(service interfaces.AdminAnswerRecordService) *AdminAnswerRecordHandler {
	return &AdminAnswerRecordHandler{service: service}
}

type adminAnswerRecordListQuery struct {
	types.Pagination
	Channel    string `form:"channel"`
	Username   string `form:"username"`
	Feedback   string `form:"feedback"`
	IsFallback *bool  `form:"is_fallback"`
	StartTime  string `form:"start_time"`
	EndTime    string `form:"end_time"`
}

// List godoc
// @Summary      查询管理端用户问答记录
// @Description  按一次问答一行返回全体用户记录，包含知识库名称和完整反馈详情
// @Tags         用户问答记录
// @Produce      json
// @Param        page        query  int     false  "页码"
// @Param        page_size   query  int     false  "每页条数"
// @Param        channel      query  string  false  "应用端：web/app" Enums(web,app)
// @Param        is_fallback  query  bool    false  "是否触发兜底回答；仅用于筛选，不增加列表字段"
// @Param        username    query  string  false  "用户名、姓名或邮箱模糊搜索"
// @Param        feedback    query  string  false  "反馈：like/dislike/none"
// @Param        start_time  query  string  false  "开始时间，RFC3339 或 YYYY-MM-DD"
// @Param        end_time    query  string  false  "结束时间，RFC3339 或 YYYY-MM-DD"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /system/admin/answer-records [get]
func (h *AdminAnswerRecordHandler) List(c *gin.Context) {
	query, err := bindAdminAnswerRecordQuery(c)
	if err != nil {
		c.Error(err)
		return
	}
	result, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// Export godoc
// @Summary      导出管理端用户问答记录
// @Description  使用与列表相同的筛选条件导出 UTF-8 CSV
// @Tags         用户问答记录
// @Produce      text/csv
// @Success      200  {file}  binary
// @Security     Bearer
// @Router       /system/admin/answer-records/export [get]
func (h *AdminAnswerRecordHandler) Export(c *gin.Context) {
	query, err := bindAdminAnswerRecordQuery(c)
	if err != nil {
		c.Error(err)
		return
	}
	content, err := h.service.Export(c.Request.Context(), query)
	if err != nil {
		c.Error(err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="answer-records.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", content)
}

func bindAdminAnswerRecordQuery(c *gin.Context) (*types.AdminAnswerRecordQuery, error) {
	var request adminAnswerRecordListQuery
	if err := c.ShouldBindQuery(&request); err != nil {
		return nil, apperrors.NewBadRequestError("invalid answer record query").WithDetails(err.Error())
	}
	startTime, err := parseAnswerRecordTime(request.StartTime, false)
	if err != nil {
		return nil, apperrors.NewBadRequestError("invalid start_time; use RFC3339 or YYYY-MM-DD")
	}
	endTime, err := parseAnswerRecordTime(request.EndTime, true)
	if err != nil {
		return nil, apperrors.NewBadRequestError("invalid end_time; use RFC3339 or YYYY-MM-DD")
	}
	return &types.AdminAnswerRecordQuery{
		Channel: request.Channel, Username: request.Username, Feedback: request.Feedback,
		IsFallback: request.IsFallback, StartTime: startTime, EndTime: endTime,
		Page: request.Page, PageSize: request.PageSize,
	}, nil
}

func parseAnswerRecordTime(value string, endOfDay bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return &parsed, nil
}
