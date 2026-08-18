package authserver

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/config"
	authhandler "roche.local/knowledge-agent-platform/internal/handler"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// NewRouter builds the authentication-only HTTP surface. Business, RAG,
// storage and model routes are intentionally absent from this process.
func NewRouter(
	cfg *config.Config,
	userService interfaces.UserService,
	db *sql.DB,
	internalSecret string,
) *gin.Engine {
	r := gin.New()
	r.ContextWithFallback = true
	r.SetTrustedProxies(trustedProxies()) //nolint:errcheck -- invalid entries fall back to direct peer IP

	corsHandler := cors.New(cors.Config{
		AllowOrigins:     allowedOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
	r.Use(func(c *gin.Context) {
		// SAML HTTP-POST binding is a top-level browser form submission from
		// the IdP, not a cross-origin API request. Browser POSTs include the
		// IdP Origin, which must not be rejected by the API CORS allowlist.
		// The ACS handler independently validates the signed assertion,
		// request ID, one-time RelayState and browser-bound nonce.
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/v1/auth/saml/acs" {
			c.Next()
			return
		}
		corsHandler(c)
	})
	r.Use(middleware.RequestID())
	r.Use(middleware.Language())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.ErrorHandler())
	r.Use(securityHeaders())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "auth-service"})
	})
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	validator := NewInternalTokenValidator(userService, internalSecret)
	r.GET("/internal/v1/auth/validate", validator.Validate)

	handler := authhandler.NewAuthHandler(cfg, userService)
	api := r.Group("/api/v1")
	registerPublicAuthRoutes(api, handler)

	protected := api.Group("")
	protected.Use(middleware.Auth(userService))
	registerProtectedAuthRoutes(protected, handler)
	return r
}

func registerPublicAuthRoutes(r *gin.RouterGroup, handler *authhandler.AuthHandler) {
	publicAuthRL := middleware.PublicAuthRateLimit()
	r.GET("/auth/registration/config", handler.GetRegistrationConfig)
	r.POST("/auth/register", publicAuthRL, handler.Register)
	r.POST("/auth/login", handler.Login)
	r.GET("/auth/oidc/config", handler.GetOIDCConfig)
	r.GET("/auth/oidc/url", handler.GetOIDCAuthorizationURL)
	r.GET("/auth/oidc/callback", handler.OIDCRedirectCallback)
	r.GET("/auth/saml/config", handler.GetSAMLConfig)
	r.GET("/auth/saml/url", handler.GetSAMLAuthorizationURL)
	r.GET("/auth/saml/acs", handler.SAMLAcs)
	r.POST("/auth/saml/acs", handler.SAMLAcs)
	r.GET("/auth/saml/metadata", handler.GetSAMLMetadata)
	r.POST("/auth/refresh", handler.RefreshToken)
	// Kept for local/lite compatibility. The standard edition handler rejects it.
	r.POST("/auth/auto-setup", handler.AutoSetup)
}

func registerProtectedAuthRoutes(r *gin.RouterGroup, handler *authhandler.AuthHandler) {
	r.GET("/auth/validate", handler.ValidateToken)
	r.POST("/auth/logout", handler.Logout)
	r.GET("/auth/me", handler.GetCurrentUser)
	r.PUT("/auth/me/preferences", handler.UpdateMyPreferences)
	r.GET("/auth/me/confidentiality-ack", handler.GetConfidentialityAck)
	r.POST("/auth/me/confidentiality-ack", handler.AcknowledgeConfidentiality)
	r.POST("/auth/change-password", handler.ChangePassword)
}

func allowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_ORIGINS"))
	if raw == "" {
		return []string{"http://localhost", "http://localhost:5173"}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func trustedProxies() []string {
	raw := strings.TrimSpace(os.Getenv("AUTH_TRUSTED_PROXIES"))
	if raw == "" {
		return []string{"127.0.0.1", "::1"}
	}
	return strings.Split(raw, ",")
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}
