package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type ExchangeRateHandler struct {
	service interfaces.ExchangeRateService
}

func NewExchangeRateHandler(service interfaces.ExchangeRateService) *ExchangeRateHandler {
	return &ExchangeRateHandler{service: service}
}

// Get godoc
// @Summary      获取人民币与瑞士法郎汇率配置
// @Description  未配置时返回 Func033 默认值 6 RMB = 1 CHF，is_default 为 true
// @Tags         汇率配置
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /exchange-rate [get]
func (h *ExchangeRateHandler) Get(c *gin.Context) {
	rate, err := h.service.GetExchangeRate(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rate})
}

type exchangeRateConfigRequest struct {
	RMBAmount float64 `json:"rmb_amount" binding:"required,gt=0"`
	CHFAmount float64 `json:"chf_amount" binding:"required,gt=0"`
}

// Configure godoc
// @Summary      保存人民币与瑞士法郎汇率配置
// @Description  系统管理员配置换算比例，例如 rmb_amount=6、chf_amount=1
// @Tags         汇率配置
// @Accept       json
// @Produce      json
// @Param        request  body  exchangeRateConfigRequest  true  "汇率比例"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /exchange-rate/config [put]
func (h *ExchangeRateHandler) Configure(c *gin.Context) {
	var request exchangeRateConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid exchange rate configuration").WithDetails(err.Error()))
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	rate, err := h.service.ConfigureExchangeRate(c.Request.Context(), types.ExchangeRateConfig{
		RMBAmount: request.RMBAmount,
		CHFAmount: request.CHFAmount,
	}, actorID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "汇率配置保存成功",
		"data":    rate,
	})
}
