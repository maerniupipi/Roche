package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	apprepo "roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/application/service"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type KnowledgeDomainAdminHandler struct {
	adminService interfaces.KnowledgeDomainAdminService
	userService  interfaces.UserService
}

func NewKnowledgeDomainAdminHandler(
	adminService interfaces.KnowledgeDomainAdminService,
	userService interfaces.UserService,
) *KnowledgeDomainAdminHandler {
	return &KnowledgeDomainAdminHandler{adminService: adminService, userService: userService}
}

func parseKnowledgeDomainIDFromPath(c *gin.Context) (uint64, bool) {
	raw := strings.TrimSpace(c.Param("id"))
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		c.Error(apperrors.NewValidationError("knowledgeDomain id must be a positive integer"))
		return 0, false
	}
	return value, true
}

// List godoc
// @Summary      List knowledge-domain administrators
// @Tags         Knowledge Domains
// @Produce      json
// @Param        id path int true "Knowledge domain ID"
// @Param        q query string false "Email or display-name search"
// @Param        page query int false "Page number" default(1)
// @Param        page_size query int false "Page size" default(20)
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /knowledge-domains/{id}/administrators [get]
func (h *KnowledgeDomainAdminHandler) List(c *gin.Context) {
	domainID, ok := parseKnowledgeDomainIDFromPath(c)
	if !ok {
		return
	}
	page, pageSize, ok := parseListPagination(c)
	if !ok {
		return
	}
	admins, total, err := h.adminService.ListPage(c.Request.Context(), domainID, c.Query("q"), page, pageSize)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list knowledgeDomain administrators").WithDetails(err.Error()))
		return
	}

	ids := make([]string, 0, len(admins))
	for _, admin := range admins {
		ids = append(ids, admin.UserID)
	}
	users, _ := h.userService.GetUsersByIDs(c.Request.Context(), ids)
	rows := make([]types.KnowledgeDomainAdminResponse, 0, len(admins))
	for _, admin := range admins {
		row := types.KnowledgeDomainAdminResponse{
			UserID: admin.UserID, Status: admin.Status, GrantedBy: admin.GrantedBy, CreatedAt: admin.CreatedAt,
		}
		if user := users[admin.UserID]; user != nil {
			row.Email, row.Username, row.Avatar = user.Email, user.Username, user.Avatar
		}
		rows = append(rows, row)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"administrators": rows, "total": total, "page": page, "page_size": pageSize,
	}})
}

// Grant godoc
// @Summary      Grant knowledge-domain administrator
// @Tags         Knowledge Domains
// @Accept       json
// @Produce      json
// @Param        id path int true "Knowledge domain ID"
// @Param        request body object{email=string} true "Target user email"
// @Success      201 {object} map[string]interface{}
// @Failure      404 {object} apperrors.AppError
// @Security     Bearer
// @Router       /knowledge-domains/{id}/administrators [post]
func (h *KnowledgeDomainAdminHandler) Grant(c *gin.Context) {
	domainID, ok := parseKnowledgeDomainIDFromPath(c)
	if !ok {
		return
	}
	var request struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.Error(apperrors.NewValidationError("invalid request body").WithDetails(err.Error()))
		return
	}
	user, err := h.userService.GetUserByEmail(c.Request.Context(), strings.TrimSpace(strings.ToLower(request.Email)))
	if err != nil {
		if errors.Is(err, apprepo.ErrUserNotFound) {
			c.Error(apperrors.NewNotFoundError("user not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to look up user").WithDetails(err.Error()))
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	admin, err := h.adminService.Grant(c.Request.Context(), user.ID, domainID, actorID)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to grant knowledgeDomain administrator").WithDetails(err.Error()))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": admin})
}

// Revoke godoc
// @Summary      Revoke knowledge-domain administrator
// @Tags         Knowledge Domains
// @Produce      json
// @Param        id path int true "Knowledge domain ID"
// @Param        user_id path string true "User ID"
// @Success      200 {object} map[string]interface{}
// @Failure      404 {object} apperrors.AppError
// @Security     Bearer
// @Router       /knowledge-domains/{id}/administrators/{user_id} [delete]
func (h *KnowledgeDomainAdminHandler) Revoke(c *gin.Context) {
	domainID, ok := parseKnowledgeDomainIDFromPath(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.Param("user_id"))
	if userID == "" {
		c.Error(apperrors.NewValidationError("user_id is required"))
		return
	}
	if err := h.adminService.Revoke(c.Request.Context(), userID, domainID); err != nil {
		if errors.Is(err, service.ErrKnowledgeDomainAdminNotFound) {
			c.Error(apperrors.NewNotFoundError(err.Error()))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to revoke knowledgeDomain administrator").WithDetails(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
