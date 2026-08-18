package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const publicAuthLimiterTTL = 15 * time.Minute

type publicAuthVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// PublicAuthRateLimit limits costly unauthenticated account operations per
// client IP. Entries expire opportunistically so long-running servers do not
// retain every address they have ever observed.
func PublicAuthRateLimit() gin.HandlerFunc {
	var mu sync.Mutex
	visitors := make(map[string]*publicAuthVisitor)
	lastCleanup := time.Now()

	return func(c *gin.Context) {
		now := time.Now()
		clientIP := c.ClientIP()

		mu.Lock()
		if now.Sub(lastCleanup) >= publicAuthLimiterTTL {
			for key, visitor := range visitors {
				if now.Sub(visitor.lastSeen) >= publicAuthLimiterTTL {
					delete(visitors, key)
				}
			}
			lastCleanup = now
		}

		visitor := visitors[clientIP]
		if visitor == nil {
			// Ten requests per minute with a short burst keeps manual testing
			// responsive while bounding password-hashing work from one client.
			visitor = &publicAuthVisitor{
				limiter: rate.NewLimiter(rate.Every(6*time.Second), 5),
			}
			visitors[clientIP] = visitor
		}
		visitor.lastSeen = now
		allowed := visitor.limiter.Allow()
		mu.Unlock()

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "Too many registration attempts; please try again later",
			})
			return
		}
		c.Next()
	}
}
