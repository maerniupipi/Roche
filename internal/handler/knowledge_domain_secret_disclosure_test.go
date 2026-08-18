package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
)

type stubKnowledgeDomainService struct {
	knowledgeDomain *types.KnowledgeDomain
	runtimeConfig   *types.PlatformRuntimeConfig
}

func (s *stubKnowledgeDomainService) UpdateKnowledgeDomain(_ context.Context, knowledgeDomain *types.KnowledgeDomain) (*types.KnowledgeDomain, error) {
	s.knowledgeDomain = knowledgeDomain
	return knowledgeDomain, nil
}
func (s *stubKnowledgeDomainService) GetPlatformRuntimeConfig(context.Context) (*types.PlatformRuntimeConfig, error) {
	return s.runtimeConfig, nil
}
func (s *stubKnowledgeDomainService) UpdatePlatformRuntimeConfig(
	_ context.Context,
	config *types.PlatformRuntimeConfig,
) (*types.PlatformRuntimeConfig, error) {
	s.runtimeConfig = config
	return config, nil
}

func (s *stubKnowledgeDomainService) CreateKnowledgeDomain(context.Context, *types.KnowledgeDomain) (*types.KnowledgeDomain, error) {
	return nil, nil
}
func (s *stubKnowledgeDomainService) GetKnowledgeDomainByID(context.Context, uint64) (*types.KnowledgeDomain, error) {
	return s.knowledgeDomain, nil
}
func (s *stubKnowledgeDomainService) GetKnowledgeDomainsByIDs(context.Context, []uint64) (map[uint64]*types.KnowledgeDomain, error) {
	return map[uint64]*types.KnowledgeDomain{s.knowledgeDomain.ID: s.knowledgeDomain}, nil
}
func (s *stubKnowledgeDomainService) DeleteKnowledgeDomain(context.Context, uint64) error { return nil }
func (s *stubKnowledgeDomainService) ListKnowledgeDomains(context.Context) ([]*types.KnowledgeDomain, error) {
	return []*types.KnowledgeDomain{s.knowledgeDomain}, nil
}
func (s *stubKnowledgeDomainService) ListAllKnowledgeDomains(context.Context) ([]*types.KnowledgeDomain, error) {
	return nil, nil
}
func (s *stubKnowledgeDomainService) SearchKnowledgeDomains(context.Context, string, uint64, int, int) ([]*types.KnowledgeDomain, int64, error) {
	return nil, 0, nil
}
func (s *stubKnowledgeDomainService) BulkSetStorageQuota(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *stubKnowledgeDomainService) GetKnowledgeDomainByIDForUser(context.Context, uint64, string) (*types.KnowledgeDomain, error) {
	return s.knowledgeDomain, nil
}
func newKnowledgeDomainHandlerTestEngine(t *testing.T, systemAdmin bool, knowledgeDomain *types.KnowledgeDomain) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	runtimeConfig := &types.PlatformRuntimeConfig{
		ID:                  1,
		WebSearchConfig:     knowledgeDomain.WebSearchConfig,
		ParserEngineConfig:  knowledgeDomain.ParserEngineConfig,
		StorageEngineConfig: knowledgeDomain.StorageEngineConfig,
		RetrievalConfig:     knowledgeDomain.RetrievalConfig,
	}
	h := &KnowledgeDomainHandler{service: &stubKnowledgeDomainService{
		knowledgeDomain: knowledgeDomain,
		runtimeConfig:   runtimeConfig,
	}}

	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, systemAdmin)
		ctx = context.WithValue(ctx, types.UserIDContextKey, "test-user")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/knowledge-domains", h.ListKnowledgeDomains)
	r.GET("/knowledge-domains/:id", h.GetKnowledgeDomain)
	r.GET("/system/runtime-config/:key", h.GetPlatformRuntimeConfig)
	r.PUT("/system/runtime-config/:key", h.UpdatePlatformRuntimeConfig)
	return r
}

func TestListKnowledgeDomainsDoesNotLeakPlatformSecrets(t *testing.T) {
	knowledgeDomain := secretKnowledgeDomainFixture()
	engine := newKnowledgeDomainHandlerTestEngine(t, true, knowledgeDomain)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-domains", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.NotContains(t, body, "knowledgeDomain-api-key-123")
	assert.NotContains(t, body, "parser-secret-123")
}

func TestGetKnowledgeDomainViewerDoesNotLeakSecrets(t *testing.T) {
	knowledgeDomain := secretKnowledgeDomainFixture()
	engine := newKnowledgeDomainHandlerTestEngine(t, false, knowledgeDomain)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-domains/42", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "knowledgeDomain-api-key-123")
}

func TestGetKnowledgeDomainKVViewerForbiddenForSecretKeys(t *testing.T) {
	knowledgeDomain := secretKnowledgeDomainFixture()
	engine := newKnowledgeDomainHandlerTestEngine(t, false, knowledgeDomain)

	for _, key := range []string{"web-search-config", "parser-engine-config", "storage-engine-config"} {
		t.Run(key, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/system/runtime-config/"+key, nil)
			engine.ServeHTTP(rec, req)
			require.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func TestGetKnowledgeDomainKVAdminReturnsRedactedSecrets(t *testing.T) {
	knowledgeDomain := secretKnowledgeDomainFixture()
	engine := newKnowledgeDomainHandlerTestEngine(t, true, knowledgeDomain)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/runtime-config/parser-engine-config", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Success bool                     `json:"success"`
		Data    types.ParserEngineConfig `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, types.RedactedSecretPlaceholder, payload.Data.MinerUAPIKey)
	assert.NotContains(t, rec.Body.String(), "parser-secret-123")
}

func secretKnowledgeDomainFixture() *types.KnowledgeDomain {
	return &types.KnowledgeDomain{
		ID:   42,
		Name: "knowledgeDomain",
		WebSearchConfig: &types.WebSearchConfig{
			APIKey: "legacy-search-secret-999",
		},
		ParserEngineConfig: &types.ParserEngineConfig{
			MinerUAPIKey: "parser-secret-123",
		},
		StorageEngineConfig: &types.StorageEngineConfig{
			MinIO: &types.MinIOEngineConfig{
				SecretAccessKey: "minio-secret-789",
			},
		},
	}
}

func TestGetKnowledgeDomainKVViewerAllowedForNonSecretKey(t *testing.T) {
	knowledgeDomain := secretKnowledgeDomainFixture()
	knowledgeDomain.RetrievalConfig = &types.RetrievalConfig{}
	engine := newKnowledgeDomainHandlerTestEngine(t, false, knowledgeDomain)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/runtime-config/retrieval-config", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestPutKnowledgeDomainParserConfigAdminPreservesRedactedSecrets(t *testing.T) {
	knowledgeDomain := secretKnowledgeDomainFixture()
	service := &stubKnowledgeDomainService{
		knowledgeDomain: knowledgeDomain,
		runtimeConfig: &types.PlatformRuntimeConfig{
			ID:                 1,
			ParserEngineConfig: knowledgeDomain.ParserEngineConfig,
		},
	}
	gin.SetMode(gin.TestMode)
	h := &KnowledgeDomainHandler{service: service}
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.SystemAdminContextKey, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	engine.PUT("/system/runtime-config/:key", h.UpdatePlatformRuntimeConfig)

	body := `{"mineru_api_key":"***","mineru_endpoint":"https://example.com/mineru"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/system/runtime-config/parser-engine-config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, service.runtimeConfig.ParserEngineConfig)
	assert.Equal(t, "parser-secret-123", service.runtimeConfig.ParserEngineConfig.MinerUAPIKey)
	assert.Equal(t, "https://example.com/mineru", service.runtimeConfig.ParserEngineConfig.MinerUEndpoint)
}
