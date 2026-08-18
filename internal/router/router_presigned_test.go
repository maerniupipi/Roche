package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// stubKnowledgeDomainService is a minimal stub of interfaces.KnowledgeDomainService that only
// implements GetKnowledgeDomainByID. Every other method panics — those code paths
// must not be exercised by presigned routes; the panic surfaces test bugs
// instead of silently returning zero values.
type stubKnowledgeDomainService struct {
	get func(ctx context.Context, id uint64) (*types.KnowledgeDomain, error)
}

var _ interfaces.KnowledgeDomainService = (*stubKnowledgeDomainService)(nil)

func (s *stubKnowledgeDomainService) GetKnowledgeDomainByID(ctx context.Context, id uint64) (*types.KnowledgeDomain, error) {
	if s.get == nil {
		return nil, os.ErrNotExist
	}
	return s.get(ctx, id)
}

func (s *stubKnowledgeDomainService) CreateKnowledgeDomain(context.Context, *types.KnowledgeDomain) (*types.KnowledgeDomain, error) {
	panic("unexpected")
}

func (s *stubKnowledgeDomainService) GetKnowledgeDomainsByIDs(context.Context, []uint64) (map[uint64]*types.KnowledgeDomain, error) {
	panic("unexpected")
}
func (s *stubKnowledgeDomainService) ListKnowledgeDomains(context.Context) ([]*types.KnowledgeDomain, error) {
	panic("unexpected")
}

func (s *stubKnowledgeDomainService) UpdateKnowledgeDomain(context.Context, *types.KnowledgeDomain) (*types.KnowledgeDomain, error) {
	panic("unexpected")
}
func (s *stubKnowledgeDomainService) GetPlatformRuntimeConfig(context.Context) (*types.PlatformRuntimeConfig, error) {
	panic("unexpected")
}
func (s *stubKnowledgeDomainService) UpdatePlatformRuntimeConfig(
	context.Context,
	*types.PlatformRuntimeConfig,
) (*types.PlatformRuntimeConfig, error) {
	panic("unexpected")
}
func (s *stubKnowledgeDomainService) DeleteKnowledgeDomain(context.Context, uint64) error {
	panic("unexpected")
}
func (s *stubKnowledgeDomainService) ListAllKnowledgeDomains(context.Context) ([]*types.KnowledgeDomain, error) {
	panic("unexpected")
}
func (s *stubKnowledgeDomainService) BulkSetStorageQuota(context.Context, int64) (int64, error) {
	panic("unexpected")
}

func (s *stubKnowledgeDomainService) SearchKnowledgeDomains(context.Context, string, uint64, int, int) ([]*types.KnowledgeDomain, int64, error) {
	panic("unexpected")
}

func (s *stubKnowledgeDomainService) GetKnowledgeDomainByIDForUser(context.Context, uint64, string) (*types.KnowledgeDomain, error) {
	panic("unexpected")
}

// setupPresignedTestServer wires presignedFileHandler with a real local file
// service rooted at a temp dir, returning the engine, baseDir, and the
// presigned URL generator helper.
func setupPresignedTestServer(t *testing.T) (engine *gin.Engine, baseDir string, signURL func(filePath string, knowledgeDomainID uint64, ttl time.Duration) string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("SYSTEM_AES_KEY", "rochekap-test-aes-key-32bytes!!!")

	baseDir = t.TempDir()

	knowledgeDomain := &types.KnowledgeDomain{
		ID: 1,
		StorageEngineConfig: &types.StorageEngineConfig{
			DefaultProvider: "local",
			Local:           &types.LocalEngineConfig{},
		},
	}
	stubTS := &stubKnowledgeDomainService{
		get: func(_ context.Context, id uint64) (*types.KnowledgeDomain, error) {
			if id == knowledgeDomain.ID {
				return knowledgeDomain, nil
			}
			return nil, os.ErrNotExist
		},
	}

	engine = gin.New()
	handler := presignedFileHandler(stubTS, baseDir)
	engine.GET("/api/v1/files/presigned", handler)
	engine.HEAD("/api/v1/files/presigned", handler)

	signURL = func(filePath string, knowledgeDomainID uint64, ttl time.Duration) string {
		signed, err := secutils.SignFileURL("https://rochekap.example.com", filePath, knowledgeDomainID, ttl)
		if err != nil {
			t.Fatalf("SignFileURL: %v", err)
		}
		// Re-parse so we only keep the query part: the test server is on
		// a different host than the canonical signing baseURL.
		u, _ := url.Parse(signed)
		return "/api/v1/files/presigned?" + u.RawQuery
	}

	return engine, baseDir, signURL
}

// writeTestFile creates baseDir/<relPath> with the given content and returns
// the matching `local://<relPath>` storage path.
func writeTestFile(t *testing.T, baseDir, relPath, content string) string {
	t.Helper()
	full := filepath.Join(baseDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return "local://" + relPath
}

func TestPresignedFile_HEAD_Returns200WithoutBody(t *testing.T) {
	engine, baseDir, signURL := setupPresignedTestServer(t)
	storagePath := writeTestFile(t, baseDir, "1/img.png", "PNG-BYTES")

	req := httptest.NewRequest(http.MethodHead, signURL(storagePath, 1, time.Hour), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d (body=%q)", got, want, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	// HEAD must not stream the body — protects backing storage from a
	// full read on every IM preview probe.
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD response body should be empty, got %d bytes", w.Body.Len())
	}
}

func TestPresignedFile_GET_ReturnsContent(t *testing.T) {
	engine, baseDir, signURL := setupPresignedTestServer(t)
	storagePath := writeTestFile(t, baseDir, "1/img.png", "PNG-BYTES")

	req := httptest.NewRequest(http.MethodGet, signURL(storagePath, 1, time.Hour), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got := w.Body.String(); got != "PNG-BYTES" {
		t.Fatalf("body = %q, want %q", got, "PNG-BYTES")
	}
}

func TestPresignedFile_ForcesActiveContentDownload(t *testing.T) {
	engine, baseDir, signURL := setupPresignedTestServer(t)
	storagePath := writeTestFile(t, baseDir, "1/payload.svg", `<svg onload="alert(1)"></svg>`)

	req := httptest.NewRequest(http.MethodGet, signURL(storagePath, 1, time.Hour), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestPresignedFile_InvalidSig_403(t *testing.T) {
	engine, _, _ := setupPresignedTestServer(t)

	// Hand-craft a URL with a tampered signature.
	q := url.Values{}
	q.Set("file_path", "local://1/img.png")
	q.Set("knowledge_domain_id", "1")
	q.Set("expires", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	q.Set("sig", "deadbeefdeadbeef")

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/files/presigned?"+q.Encode(), nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if got, want := w.Code, http.StatusForbidden; got != want {
				t.Fatalf("status = %d, want %d", got, want)
			}
		})
	}
}

func TestPresignedFile_MissingParams_400(t *testing.T) {
	engine, _, _ := setupPresignedTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/presigned?file_path=local%3A%2F%2F1%2Fimg.png", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got, want := w.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(w.Body.String(), "missing required parameters") {
		t.Fatalf("body = %q, want missing-params error", w.Body.String())
	}
}

func TestPresignedFile_MissingFile_404(t *testing.T) {
	engine, _, signURL := setupPresignedTestServer(t)

	// A legitimately signed URL pointing at a file that does not exist.
	req := httptest.NewRequest(http.MethodGet, signURL("local://1/nope.png", 1, time.Hour), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got, want := w.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
