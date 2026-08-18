package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/application/service"
	"roche.local/knowledge-agent-platform/internal/authserver"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/logger"
)

func main() {
	ctx := context.Background()
	production := strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_SERVICE_ENV")), "production")
	if strings.EqualFold(os.Getenv("GIN_MODE"), "release") {
		gin.SetMode(gin.ReleaseMode)
	}

	internalSecret := strings.TrimSpace(os.Getenv("AUTH_SERVICE_INTERNAL_SECRET"))
	if len(internalSecret) < 32 {
		logger.Fatalf(ctx, "AUTH_SERVICE_INTERNAL_SECRET must contain at least 32 characters")
	}
	if production && (len(internalSecret) < 48 || strings.Contains(strings.ToLower(internalSecret), "change_me")) {
		logger.Fatalf(ctx, "production AUTH_SERVICE_INTERNAL_SECRET must be a random value containing at least 48 characters")
	}
	if production && !strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_REFRESH_COOKIE_SECURE")), "true") {
		logger.Fatalf(ctx, "AUTH_REFRESH_COOKIE_SECURE must be true in production")
	}
	if strings.TrimSpace(os.Getenv("AUTH_ALLOWED_REDIRECT_URIS")) == "" {
		logger.Fatalf(ctx, "AUTH_ALLOWED_REDIRECT_URIS must contain at least one exact post-login URI")
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf(ctx, "load auth service configuration: %v", err)
	}
	if production {
		validateProductionSAMLConfig(ctx, cfg)
	}
	db, err := authserver.OpenDatabase(ctx)
	if err != nil {
		logger.Fatalf(ctx, "%v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		logger.Fatalf(ctx, "get auth database pool: %v", err)
	}
	defer sqlDB.Close()

	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewAuthTokenRepository(db)
	ssoRepo := repository.NewSSOIdentityRepository(db)
	domainAdminRepo := repository.NewKnowledgeDomainAdminRepository(db)
	kdRepo := repository.NewKnowledgeDomainRepository(db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)
	eaRepo := repository.NewEnterpriseAccessRepository(db)

	// Audit wiring: unlike the main app (which assembles its audit
	// components via the DI container), the auth service wires them by
	// hand. The recorder must NOT be nil — auth lifecycle events
	// (login / login-failed / logout / user management / grants) are
	// written through it to the shared audit_logs table. Audit failures
	// stay best-effort: they are logged but never break the auth flow.
	auditRepo := repository.NewAuditLogRepository(db)
	auditSvc := service.NewAuditLogService(auditRepo)
	businessAudit := service.NewBusinessAuditRecorder(auditSvc)
	domainAdminService := service.NewKnowledgeDomainAdminService(domainAdminRepo, businessAudit)
	userService := service.NewUserService(cfg, userRepo, tokenRepo, ssoRepo, domainAdminService, kdRepo, kbRepo, eaRepo, businessAudit)
	router := authserver.NewRouter(cfg, userService, sqlDB, internalSecret)

	addr := fmt.Sprintf("%s:%s", env("AUTH_SERVICE_HOST", "0.0.0.0"), env("AUTH_SERVICE_PORT", "8081"))
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		logger.Infof(ctx, "Auth Service listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf(ctx, "Auth Service failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Errorf(ctx, "Auth Service shutdown failed: %v", err)
	}
}

func validateProductionSAMLConfig(ctx context.Context, cfg *config.Config) {
	if cfg.SAMLAuth == nil || !cfg.SAMLAuth.Enable {
		logger.Fatalf(ctx, "SAML authentication must be enabled for the production Auth Service")
	}
	if cfg.SAMLAuth.AllowEphemeralSPCert {
		logger.Fatalf(ctx, "ephemeral SAML SP certificates are forbidden in production")
	}

	zone := strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_SERVICE_ZONE")))
	if zone != "internal" && zone != "external" {
		logger.Fatalf(ctx, "AUTH_SERVICE_ZONE must be internal or external in production")
	}
	internalEntityID := strings.TrimSpace(os.Getenv("SAML_INTERNAL_SP_ENTITY_ID"))
	externalEntityID := strings.TrimSpace(os.Getenv("SAML_EXTERNAL_SP_ENTITY_ID"))
	internalACS := strings.TrimSpace(os.Getenv("SAML_INTERNAL_ACS_URL"))
	externalACS := strings.TrimSpace(os.Getenv("SAML_EXTERNAL_ACS_URL"))
	if internalEntityID == "" || externalEntityID == "" || internalEntityID == externalEntityID {
		logger.Fatalf(ctx, "production internal/external SAML SP entity IDs must be present and different")
	}
	if internalACS == "" || externalACS == "" || internalACS == externalACS {
		logger.Fatalf(ctx, "production internal/external SAML ACS URLs must be present and different")
	}

	expectedEntityID, expectedACS := internalEntityID, internalACS
	if zone == "external" {
		expectedEntityID, expectedACS = externalEntityID, externalACS
	}
	if cfg.SAMLAuth.SPEntityID != expectedEntityID || cfg.SAMLAuth.ACSUrl != expectedACS {
		logger.Fatalf(ctx, "SAML configuration does not match AUTH_SERVICE_ZONE=%s", zone)
	}
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
