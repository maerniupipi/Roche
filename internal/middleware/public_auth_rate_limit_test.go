package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicAuthRateLimitRejectsBurstBeyondLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/register", PublicAuthRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/register", nil)
		req.RemoteAddr = "192.0.2.10:12345"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if i < 5 && resp.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d, want %d", i+1, resp.Code, http.StatusNoContent)
		}
		if i == 5 && resp.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d status = %d, want %d", i+1, resp.Code, http.StatusTooManyRequests)
		}
	}
}
