package middleware

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/config"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

func IsCrossKnowledgeDomainSuperuser(ctx context.Context, _ *config.Config) bool {
	u, ok := ctx.Value(types.UserContextKey).(*types.User)
	return ok && u != nil && u.IsSystemAdmin
}

func RequireCrossKnowledgeDomainAccess(_ *config.Config) gin.HandlerFunc {
	return requireSystemAdministrator
}

// Domain management is no longer tied to an active-domain value in the JWT.
// Until a route installs the explicit domain-administrator guard, this legacy
// route guard is deliberately restricted to system administrators.
func RequirePathKnowledgeDomainMatch(_ *config.Config) gin.HandlerFunc {
	return requireSystemAdministrator
}

// RequireKnowledgeDomainAdmin allows system administrators and explicit
// administrators of the knowledge domain addressed by the path parameter.
func RequireKnowledgeDomainAdmin(
	param string,
	adminService interfaces.KnowledgeDomainAdminService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if types.IsSystemAdminFromContext(ctx) {
			c.Next()
			return
		}

		userID, ok := types.UserIDFromContext(ctx)
		if !ok || strings.TrimSpace(userID) == "" {
			_ = c.Error(apperrors.NewUnauthorizedError("Unauthorized"))
			c.Abort()
			return
		}
		domainID, err := strconv.ParseUint(strings.TrimSpace(c.Param(param)), 10, 64)
		if err != nil || domainID == 0 {
			_ = c.Error(apperrors.NewBadRequestError("invalid knowledge domain id"))
			c.Abort()
			return
		}
		if adminService == nil {
			_ = c.Error(apperrors.NewServiceUnavailableError("knowledge domain authorization is unavailable"))
			c.Abort()
			return
		}
		allowed, err := adminService.IsAdmin(ctx, userID, domainID)
		if err != nil {
			logger.Errorf(ctx, "[rbac] knowledge domain administrator lookup failed: %v", err)
			_ = c.Error(apperrors.NewServiceUnavailableError("cannot verify knowledge domain access"))
			c.Abort()
			return
		}
		if !allowed {
			_ = c.Error(apperrors.NewForbiddenError("Knowledge domain administrator permission is required"))
			c.Abort()
			return
		}

		c.Set(types.KnowledgeDomainIDContextKey.String(), domainID)
		c.Request = c.Request.WithContext(context.WithValue(ctx, types.KnowledgeDomainIDContextKey, domainID))
		c.Next()
	}
}

func requireSystemAdministrator(c *gin.Context) {
	ctx := c.Request.Context()
	u, _ := ctx.Value(types.UserContextKey).(*types.User)
	if u != nil && u.IsSystemAdmin {
		c.Next()
		return
	}
	uid, _ := types.UserIDFromContext(ctx)
	logger.Warnf(ctx, "[rbac] system administrator route blocked: user=%s path=%s", uid, c.Request.URL.Path)
	_ = c.Error(apperrors.NewForbiddenError("System administrator permission is required"))
	c.Abort()
}
