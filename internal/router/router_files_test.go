package router

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var _ interfaces.FileService = (*stubFileService)(nil)

type stubFileService struct {
	getFile func(ctx context.Context, filePath string) (io.ReadCloser, error)
}

func (s *stubFileService) CheckConnectivity(ctx context.Context) error {
	return nil
}

func (s *stubFileService) SaveFile(ctx context.Context, file *multipart.FileHeader, knowledgeDomainID uint64, knowledgeID string) (string, error) {
	panic("unexpected call to SaveFile")
}

func (s *stubFileService) SaveBytes(ctx context.Context, data []byte, knowledgeDomainID uint64, fileName string, temp bool) (string, error) {
	panic("unexpected call to SaveBytes")
}

func (s *stubFileService) GetFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	if s.getFile == nil {
		panic("unexpected call to GetFile")
	}
	return s.getFile(ctx, filePath)
}

func (s *stubFileService) GetFileURL(ctx context.Context, filePath string) (string, error) {
	panic("unexpected call to GetFileURL")
}

func (s *stubFileService) DeleteFile(ctx context.Context, filePath string) error {
	panic("unexpected call to DeleteFile")
}

func (s *stubFileService) CopyFile(ctx context.Context, srcPath string, knowledgeDomainID uint64, knowledgeID string) (string, error) {
	panic("unexpected call to CopyFile")
}

func TestServeFilesFallsBackToGlobalFileService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	engine := gin.New()
	var requestedPath string
	serveFiles(engine, &stubFileService{
		getFile: func(ctx context.Context, filePath string) (io.ReadCloser, error) {
			requestedPath = filePath
			return io.NopCloser(strings.NewReader("fallback-body")), nil
		},
	})

	filePath := "local://42/docs/example.txt"
	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(filePath), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.KnowledgeDomainInfoContextKey, &types.KnowledgeDomain{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if requestedPath != filePath {
		t.Fatalf("requested path = %q, want %q", requestedPath, filePath)
	}
	if body := recorder.Body.String(); body != "fallback-body" {
		t.Fatalf("body = %q, want %q", body, "fallback-body")
	}
}

func TestServeFilesDoesNotFallbackWhenProviderDoesNotMatchGlobalStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "minio")

	engine := gin.New()
	serveFiles(engine, &stubFileService{
		getFile: func(ctx context.Context, filePath string) (io.ReadCloser, error) {
			t.Fatalf("GetFile should not be called for mismatched provider, got %q", filePath)
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape("local://42/docs/example.txt"), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.KnowledgeDomainInfoContextKey, &types.KnowledgeDomain{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestServeFilesRejectsCrossKnowledgeDomainPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	engine := gin.New()
	serveFiles(engine, &stubFileService{
		getFile: func(ctx context.Context, filePath string) (io.ReadCloser, error) {
			t.Fatalf("GetFile should not be called for cross-knowledgeDomain path, got %q", filePath)
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape("local://7/knowledge/secret.pdf"), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.KnowledgeDomainInfoContextKey, &types.KnowledgeDomain{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusForbidden; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestServeFilesRejectsPathWithoutKnowledgeDomainSegment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	engine := gin.New()
	serveFiles(engine, &stubFileService{
		getFile: func(ctx context.Context, filePath string) (io.ReadCloser, error) {
			t.Fatalf("GetFile should not be called without knowledgeDomain segment, got %q", filePath)
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape("local://docs/example.txt"), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.KnowledgeDomainInfoContextKey, &types.KnowledgeDomain{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusForbidden; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestServeFilesForcesActiveContentDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	engine := gin.New()
	serveFiles(engine, &stubFileService{
		getFile: func(_ context.Context, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(`<svg onload="alert(1)"></svg>`)), nil
		},
	})

	filePath := "local://42/docs/payload.svg"
	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(filePath), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.KnowledgeDomainInfoContextKey, &types.KnowledgeDomain{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
