package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type UnifiedQAObservationHandler struct {
	service interfaces.UnifiedQAObservationService
}

func NewUnifiedQAObservationHandler(service interfaces.UnifiedQAObservationService) *UnifiedQAObservationHandler {
	return &UnifiedQAObservationHandler{service: service}
}

func (h *UnifiedQAObservationHandler) GetRun(c *gin.Context) {
	observation, err := h.service.GetRunObservation(c.Request.Context(), c.Param("id"))
	if err != nil {
		if _, ok := err.(*apperrors.AppError); ok {
			c.Error(err)
		} else {
			c.Error(apperrors.NewInternalServerError(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": observation})
}
