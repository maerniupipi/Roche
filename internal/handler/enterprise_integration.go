package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/application/service"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type EnterpriseIntegrationHandler struct {
	service interfaces.EnterpriseIntegrationService
}

func NewEnterpriseIntegrationHandler(
	integrationService interfaces.EnterpriseIntegrationService,
) *EnterpriseIntegrationHandler {
	return &EnterpriseIntegrationHandler{service: integrationService}
}

type triggerWorkdaySyncRequest struct {
	Mode types.IntegrationSyncMode `json:"mode"`
}

// TriggerWorkdaySync godoc
// @Summary      Trigger Workday synchronization
// @Description  Enqueues a full or incremental organization and worker synchronization.
// @Tags         Enterprise Integrations
// @Accept       json
// @Produce      json
// @Param        request body triggerWorkdaySyncRequest true "Synchronization mode"
// @Success      202 {object} map[string]interface{}
// @Failure      400 {object} apperrors.AppError
// @Failure      403 {object} apperrors.AppError
// @Failure      409 {object} apperrors.AppError
// @Security     Bearer
// @Router       /system/admin/integrations/workday/sync [post]
func (h *EnterpriseIntegrationHandler) TriggerWorkdaySync(c *gin.Context) {
	var req triggerWorkdaySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	if req.Mode == "" {
		req.Mode = types.IntegrationSyncModeIncremental
	}
	run, err := h.service.TriggerWorkdaySync(
		c.Request.Context(),
		req.Mode,
		traceIDFromRequest(c),
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": run})
}

// ListWorkdaySyncRuns godoc
// @Summary      List Workday synchronization runs
// @Tags         Enterprise Integrations
// @Produce      json
// @Param        offset query int false "Zero-based offset"
// @Param        limit query int false "Page size" default(20)
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} apperrors.AppError
// @Security     Bearer
// @Router       /system/admin/integrations/workday/runs [get]
func (h *EnterpriseIntegrationHandler) ListWorkdaySyncRuns(c *gin.Context) {
	offset := parseNonNegativeInt(c.Query("offset"), 0)
	limit := parseNonNegativeInt(c.Query("limit"), 20)
	runs, total, err := h.service.ListWorkdaySyncRuns(
		c.Request.Context(),
		offset,
		limit,
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runs,
		"total":   total,
	})
}

// GetWorkdaySyncRun godoc
// @Summary      Get a Workday synchronization run
// @Tags         Enterprise Integrations
// @Produce      json
// @Param        run_id path string true "Synchronization run ID"
// @Success      200 {object} map[string]interface{}
// @Failure      404 {object} apperrors.AppError
// @Security     Bearer
// @Router       /system/admin/integrations/workday/runs/{run_id} [get]
func (h *EnterpriseIntegrationHandler) GetWorkdaySyncRun(c *gin.Context) {
	run, err := h.service.GetWorkdaySyncRun(
		c.Request.Context(),
		c.Param("run_id"),
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}

// ListWorkdayOrgUnits godoc
// @Summary      List synchronized Workday organizations
// @Description  Returns the read-only Workday organization projection, separate from manually maintained groups.
// @Tags         Enterprise Integrations
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} apperrors.AppError
// @Security     Bearer
// @Router       /system/admin/integrations/workday/directory/org-units [get]
func (h *EnterpriseIntegrationHandler) ListWorkdayOrgUnits(c *gin.Context) {
	units, err := h.service.ListWorkdayOrgUnits(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": units})
}

// ListWorkdayWorkers godoc
// @Summary      List synchronized Workday workers
// @Description  Returns linked and unlinked workers from the read-only Workday projection.
// @Tags         Enterprise Integrations
// @Produce      json
// @Param        org_external_id query string false "Workday organization external ID"
// @Param        search query string false "Corporate email or external worker ID"
// @Param        offset query int false "Zero-based offset"
// @Param        limit query int false "Page size" default(200)
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} apperrors.AppError
// @Security     Bearer
// @Router       /system/admin/integrations/workday/directory/workers [get]
func (h *EnterpriseIntegrationHandler) ListWorkdayWorkers(c *gin.Context) {
	workers, total, err := h.service.ListWorkdayWorkers(
		c.Request.Context(),
		c.Query("org_external_id"),
		c.Query("search"),
		parseNonNegativeInt(c.Query("offset"), 0),
		parseNonNegativeInt(c.Query("limit"), 200),
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    workers,
		"total":   total,
	})
}

type workdayEventRequest struct {
	ExternalEventID string          `json:"external_event_id"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
}

// AcceptWorkdayEvent godoc
// @Summary      Accept an authenticated Workday event
// @Description  Records an idempotent event and schedules synchronization. This is an administrator API, not a public webhook.
// @Tags         Enterprise Integrations
// @Accept       json
// @Produce      json
// @Param        request body workdayEventRequest true "Workday event envelope"
// @Success      202 {object} map[string]interface{}
// @Failure      400 {object} apperrors.AppError
// @Failure      403 {object} apperrors.AppError
// @Security     Bearer
// @Router       /system/admin/integrations/workday/events [post]
func (h *EnterpriseIntegrationHandler) AcceptWorkdayEvent(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError("failed to read event"))
		return
	}
	var req workdayEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("invalid event envelope"))
		return
	}
	created, run, err := h.service.AcceptWorkdayEvent(
		c.Request.Context(),
		req.ExternalEventID,
		req.EventType,
		req.Payload,
		traceIDFromRequest(c),
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"success":   true,
		"accepted":  created,
		"duplicate": !created,
		"run":       run,
	})
}

func (h *EnterpriseIntegrationHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrWorkdayDisabled):
		_ = c.Error(apperrors.NewConflictError(err.Error()))
	case errors.Is(err, service.ErrInvalidSyncMode):
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
	case errors.Is(err, service.ErrIntegrationRunNotFound):
		_ = c.Error(apperrors.NewNotFoundError(err.Error()))
	default:
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
	}
}

func parseNonNegativeInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func traceIDFromRequest(c *gin.Context) string {
	for _, header := range []string{"X-Trace-ID", "X-Request-ID"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}
	return ""
}
