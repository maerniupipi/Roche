package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// SuggestedQuestionHandler exposes the three global homepage questions.
type SuggestedQuestionHandler struct {
	service interfaces.SuggestedQuestionService
}

func NewSuggestedQuestionHandler(service interfaces.SuggestedQuestionService) *SuggestedQuestionHandler {
	return &SuggestedQuestionHandler{service: service}
}

// List godoc
// @Summary      获取前台推荐问题
// @Description  返回全局配置的三个推荐问题，按 sort_order 升序排列
// @Tags         推荐问题
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /suggested-questions [get]
func (h *SuggestedQuestionHandler) List(c *gin.Context) {
	questions, err := h.service.ListSuggestedQuestions(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"questions": questions}})
}

type suggestedQuestionConfigRequest struct {
	Items []suggestedQuestionConfigRequestItem `json:"items" binding:"required,len=3,dive"`
}

type suggestedQuestionConfigRequestItem struct {
	ID           string                            `json:"id"`
	Question     string                            `json:"question" binding:"required"`
	AnswerMode   types.SuggestedQuestionAnswerMode `json:"answer_mode" binding:"required,oneof=generated custom"`
	CustomAnswer string                            `json:"custom_answer"`
	SortOrder    int                               `json:"sort_order" binding:"required,min=1,max=3"`
}

// Configure godoc
// @Summary      保存三个全局推荐问题
// @Description  系统管理员整体替换三个推荐问题；支持系统生成回答或自定义答案
// @Tags         推荐问题
// @Accept       json
// @Produce      json
// @Param        request  body  suggestedQuestionConfigRequest  true  "三个推荐问题"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /suggested-questions/config [put]
func (h *SuggestedQuestionHandler) Configure(c *gin.Context) {
	var request suggestedQuestionConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid suggested question configuration").WithDetails(err.Error()))
		return
	}
	items := make([]types.SuggestedQuestionConfigItem, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, types.SuggestedQuestionConfigItem{
			ID: item.ID, Question: item.Question, AnswerMode: item.AnswerMode,
			CustomAnswer: item.CustomAnswer, SortOrder: item.SortOrder,
		})
	}
	_, err := h.service.ConfigureSuggestedQuestions(c.Request.Context(), items)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "推荐问题配置保存成功"})
}
