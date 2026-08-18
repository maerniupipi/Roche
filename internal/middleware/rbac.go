package middleware

import (
	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/response"
	"roche.local/knowledge-agent-platform/internal/types"
)

// RequireSystemAdmin guards platform-wide administration. It is always
// enforced; the clean enterprise model has no RBAC rollout or fail-open mode.
func RequireSystemAdmin(_ *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if types.IsSystemAdminFromContext(ctx) {
			c.Next()
			return
		}

		userID, _ := types.UserIDFromContext(ctx)
		logger.Warnf(ctx,
			"[access] system administrator required: user=%s path=%s",
			userID, c.Request.URL.Path)
		if svc := AuditServiceFromContext(c); svc != nil {
			_ = svc.LogDenied(
				ctx,
				c,
				userID,
				"user",
				"system_admin",
			)
		}
		response.Forbidden(c, "system administrator required")
		c.Abort()
	}
}

// RequireKnowledgeOfficer guards routes that require knowledge officer role.
// SystemAdmin also passes (super-user).
func RequireKnowledgeOfficer(_ *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if types.IsSystemAdminFromContext(ctx) || types.IsKnowledgeOfficerFromContext(ctx) {
			c.Next()
			return
		}

		userID, _ := types.UserIDFromContext(ctx)
		logger.Warnf(ctx,
			"[access] knowledge officer required: user=%s path=%s",
			userID, c.Request.URL.Path)
		if svc := AuditServiceFromContext(c); svc != nil {
			_ = svc.LogDenied(
				ctx,
				c,
				userID,
				"user",
				"knowledge_officer",
			)
		}
		response.Forbidden(c, "knowledge officer required")
		c.Abort()
	}
}

// RequireAdminBackend guards admin-backend routes. Both system_admin and
// knowledge_officer (with role_knowledge_officer=1) are allowed.
// Regular viewers receive 403.
func RequireAdminBackend(_ *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if types.IsSystemAdminFromContext(ctx) || types.IsKnowledgeOfficerFromContext(ctx) {
			c.Next()
			return
		}

		userID, _ := types.UserIDFromContext(ctx)
		logger.Warnf(ctx,
			"[access] admin backend required: user=%s path=%s",
			userID, c.Request.URL.Path)
		if svc := AuditServiceFromContext(c); svc != nil {
			_ = svc.LogDenied(
				ctx,
				c,
				userID,
				"user",
				"admin_backend",
			)
		}
		response.Forbidden(c, "admin backend access requires system_admin or knowledge_officer role")
		c.Abort()
	}
}
