package middleware

import (
	"context"
	"os"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
)

const internalServiceTokenHeader = "X-Internal-Service-Token"

// resolveInternalToken returns the configured internal service token,
// preferring the config struct but falling back to the env var.
func resolveInternalToken(cfg *config.Config) string {
	if cfg != nil && cfg.InternalService != nil && cfg.InternalService.Token != "" {
		return cfg.InternalService.Token
	}
	return os.Getenv("INTERNAL_SERVICE_TOKEN")
}

// InternalServiceAuth inspects the X-Internal-Service-Token header. When the
// header value matches the configured shared secret, a synthetic system‑admin
// user is injected into the request context so that downstream RBAC guards
// (KBAccessWrite, SystemAdmin, etc.) pass through without a real JWT.
//
// When the header is absent or mismatched the middleware is a no‑op —
// the regular JWT authentication pipeline takes over.
func InternalServiceAuth(cfg *config.Config) gin.HandlerFunc {
	token := resolveInternalToken(cfg)
	if token == "" {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if c.GetHeader(internalServiceTokenHeader) != token {
			c.Next()
			return
		}

		u := &types.User{
			ID:            "internal-service",
			IsSystemAdmin: true,
		}
		principal := types.Principal{Type: types.PrincipalWebUser, ID: u.ID}

		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.UserContextKey, u)
		ctx = context.WithValue(ctx, types.UserIDContextKey, u.ID)
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, u.IsSystemAdmin)
		ctx = context.WithValue(ctx, types.KnowledgeOfficerContextKey, u.RoleKnowledgeOfficer)
		ctx = types.WithPrincipal(ctx, principal)

		c.Set(types.UserContextKey.String(), u)
		c.Set(types.UserIDContextKey.String(), u.ID)
		c.Set(types.SystemAdminContextKey.String(), u.IsSystemAdmin)
		c.Set(types.KnowledgeOfficerContextKey.String(), u.RoleKnowledgeOfficer)
		c.Set(types.PrincipalContextKey.String(), principal)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
