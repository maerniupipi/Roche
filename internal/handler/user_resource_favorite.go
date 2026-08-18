package handler

import (
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/application/service"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// UserResourceFavoriteHandler exposes the authenticated user's favorites.
// Callers cannot supply a user ID; ownership always comes from the JWT.
type UserResourceFavoriteHandler struct {
	service interfaces.UserResourceFavoriteService
}

func NewUserResourceFavoriteHandler(svc interfaces.UserResourceFavoriteService) *UserResourceFavoriteHandler {
	return &UserResourceFavoriteHandler{service: svc}
}

func favoriteUserID(c *gin.Context) (string, bool) {
	uidVal, ok := c.Get(types.UserIDContextKey.String())
	if !ok {
		_ = c.Error(apperrors.NewUnauthorizedError("user ID not found"))
		return "", false
	}
	userID, _ := uidVal.(string)
	if userID == "" {
		_ = c.Error(apperrors.NewUnauthorizedError("user ID not found"))
		return "", false
	}
	return userID, true
}

// ListFavorites godoc
// @Summary      List my favorites
// @Description  Lists this user's starred resources for a given type
// @Tags         User
// @Param        type  query     string  true  "Resource type (kb | agent)"
// @Success      200   {object}  map[string]interface{}
// @Router       /user/favorites [get]
func (h *UserResourceFavoriteHandler) ListFavorites(c *gin.Context) {
	ctx := c.Request.Context()
	userID, ok := favoriteUserID(c)
	if !ok {
		return
	}
	list, err := h.service.List(ctx, userID, c.Query("type"))
	if err != nil {
		if stderrors.Is(err, service.ErrFavoriteInvalidType) {
			_ = c.Error(apperrors.NewBadRequestError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

type AddFavoriteRequest struct {
	ResourceType string `json:"type"`
	ResourceID   string `json:"id"`
}

// AddFavorite godoc
// @Summary      Star a resource
// @Tags         User
// @Param        body  body      AddFavoriteRequest  true  "Type + id"
// @Success      200   {object}  map[string]interface{}
// @Router       /user/favorites [post]
func (h *UserResourceFavoriteHandler) AddFavorite(c *gin.Context) {
	ctx := c.Request.Context()
	userID, ok := favoriteUserID(c)
	if !ok {
		return
	}
	var req AddFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	if err := h.service.Add(ctx, userID, req.ResourceType, req.ResourceID); err != nil {
		if stderrors.Is(err, service.ErrFavoriteInvalidType) || stderrors.Is(err, service.ErrFavoriteEmptyID) {
			_ = c.Error(apperrors.NewBadRequestError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RemoveFavorite godoc
// @Summary      Unstar a resource
// @Tags         User
// @Param        type  path      string  true  "Resource type"
// @Param        id    path      string  true  "Resource id"
// @Success      200   {object}  map[string]interface{}
// @Router       /user/favorites/{type}/{id} [delete]
func (h *UserResourceFavoriteHandler) RemoveFavorite(c *gin.Context) {
	ctx := c.Request.Context()
	userID, ok := favoriteUserID(c)
	if !ok {
		return
	}
	resourceType := c.Param("type")
	resourceID := c.Param("id")
	if err := h.service.Remove(ctx, userID, resourceType, resourceID); err != nil {
		if stderrors.Is(err, service.ErrFavoriteInvalidType) || stderrors.Is(err, service.ErrFavoriteEmptyID) {
			_ = c.Error(apperrors.NewBadRequestError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
