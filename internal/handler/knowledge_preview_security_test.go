package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type previewKnowledgeServiceStub struct {
	interfaces.KnowledgeService
	filename string
}

type previewKnowledgeBaseServiceStub struct {
	interfaces.KnowledgeBaseService
}

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	ch := make(chan bool, 1)
	return ch
}

func (s *previewKnowledgeServiceStub) GetKnowledgeByIDOnly(context.Context, string) (*types.Knowledge, error) {
	return &types.Knowledge{ID: "k1", KnowledgeDomainID: 42, KnowledgeBaseID: "kb1"}, nil
}

func (s *previewKnowledgeServiceStub) GetKnowledgeFile(context.Context, string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(`<html><script>alert(1)</script></html>`)), s.filename, nil
}

func (s *previewKnowledgeBaseServiceStub) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{ID: "kb1", KnowledgeDomainID: 42}, nil
}

func (s *previewKnowledgeBaseServiceStub) CanReadKnowledgeBase(context.Context, *types.KnowledgeBase) (bool, error) {
	return true, nil
}

func (s *previewKnowledgeBaseServiceStub) CanManageKnowledgeBase(context.Context, *types.KnowledgeBase) (bool, error) {
	return true, nil
}

func (s *previewKnowledgeBaseServiceStub) CanReadKnowledge(context.Context, *types.Knowledge) (bool, error) {
	return true, nil
}

func TestPreviewKnowledgeFileForcesActiveContentDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		c.Set(types.KnowledgeDomainIDContextKey.String(), uint64(42))
		c.Next()
	})

	h := &KnowledgeHandler{
		kgService: &previewKnowledgeServiceStub{filename: "payload.html"},
		kbService: &previewKnowledgeBaseServiceStub{},
	}
	router.GET("/knowledge/:id/preview", h.PreviewKnowledgeFile)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k1/preview", nil)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	router.ServeHTTP(w, req)

	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body=%s", got, want, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
}
