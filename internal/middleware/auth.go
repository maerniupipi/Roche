package middleware

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var noAuthAPI = map[string][]string{
	"/health":                    {"GET"},
	"/swagger/*":                 {"GET", "HEAD"},
	"/api/v1/mcp-oauth/callback": {"GET"},
	"/api/v1/files/presigned":    {"GET", "HEAD"},
}

// noAuthPattern matches dynamic paths that skip authentication. Unlike
// noAuthAPI (exact or trailing-* prefix match), these are anchored regexps
// against the full request path.
type noAuthPattern struct {
	pattern *regexp.Regexp
	methods []string
}

var noAuthAPIPatterns = []noAuthPattern{
	// Data-source sync is intentionally open: it is triggered by the
	// system every day at 00:00 as well as by external schedulers, so it
	// must not require a user JWT.
	{pattern: regexp.MustCompile(`^/api/v1/datasource/[^/]+/sync$`), methods: []string{"POST"}},
}

func isNoAuthAPI(path, method string) bool {
	for api, methods := range noAuthAPI {
		if strings.HasSuffix(api, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(api, "*")) && slices.Contains(methods, method) {
				return true
			}
			continue
		}
		if path == api && slices.Contains(methods, method) {
			return true
		}
	}
	for _, p := range noAuthAPIPatterns {
		if p.pattern.MatchString(path) && slices.Contains(p.methods, method) {
			return true
		}
	}
	return false
}

// Auth authenticates the platform user. Knowledge-domain and resource access
// are resolved later from the requested knowledge base or document and the
// explicit grant tables; authentication carries no active-space state.
func Auth(userService interfaces.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || isNoAuthAPI(c.Request.URL.Path, c.Request.Method) {
			c.Next()
			return
		}

		// If a user is already set in the request context (e.g. by
		// InternalServiceAuth), skip JWT validation.
		if _, ok := types.UserIDFromContext(c.Request.Context()); ok {
			c.Next()
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: missing authentication"})
			c.Abort()
			return
		}

		user, err := userService.ValidateToken(c.Request.Context(), strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")))
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid authentication"})
			c.Abort()
			return
		}

		principal := types.Principal{Type: types.PrincipalWebUser, ID: user.ID}
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.UserContextKey, user)
		ctx = context.WithValue(ctx, types.UserIDContextKey, user.ID)
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, user.IsSystemAdmin)
		ctx = context.WithValue(ctx, types.KnowledgeOfficerContextKey, user.RoleKnowledgeOfficer)
		ctx = types.WithPrincipal(ctx, principal)

		c.Set(types.UserContextKey.String(), user)
		c.Set(types.UserIDContextKey.String(), user.ID)
		c.Set(types.SystemAdminContextKey.String(), user.IsSystemAdmin)
		c.Set(types.KnowledgeOfficerContextKey.String(), user.RoleKnowledgeOfficer)
		c.Set(types.PrincipalContextKey.String(), principal)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
