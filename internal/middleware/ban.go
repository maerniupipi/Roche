package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// BanCheck is a middleware that checks whether the authenticated user is
// blacklisted. It runs after Auth middleware (which sets the user in context)
// and before any route handler. When the user is banned (status or blacklist
// table), the request is rejected with 403 Forbidden regardless of token
// validity.
//
// This is a defense-in-depth layer: even if ValidateToken succeeds (e.g.
// due to a race), the blacklist table provides an independent check.
func BanCheck(blacklistRepo interfaces.BlacklistEntryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// No user means the route is unauthenticated (noAuthAPI) —
		// pass through without checking.
		userVal, exists := c.Get(types.UserContextKey.String())
		if !exists {
			c.Next()
			return
		}

		user, ok := userVal.(*types.User)
		if !ok || user == nil {
			c.Next()
			return
		}

		// Banned status takes highest precedence.
		if user.IsBanned() {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Account is banned",
			})
			c.Abort()
			return
		}

		// Cross-check against the independent blacklist table.
		if blacklistRepo != nil {
			isBlacklisted, err := blacklistRepo.IsBlacklisted(c.Request.Context(), user.ID)
			if err == nil && isBlacklisted {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "Account is banned",
				})
				c.Abort()
				return
			}
		}

		// Non-normal status (e.g. resigned) is blocked.
		if user.Status != types.UserStatusNormal {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Account is disabled",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
