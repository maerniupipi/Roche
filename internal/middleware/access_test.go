package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/config"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestIsCrossKnowledgeDomainSuperuser(t *testing.T) {
	root := context.WithValue(context.Background(), types.UserContextKey,
		&types.User{ID: "root", IsSystemAdmin: true})
	if !IsCrossKnowledgeDomainSuperuser(root, nil) {
		t.Fatal("system administrator must have cross-knowledgeDomain access")
	}
	normal := context.WithValue(context.Background(), types.UserContextKey,
		&types.User{ID: "viewer"})
	if IsCrossKnowledgeDomainSuperuser(normal, nil) {
		t.Fatal("normal user must not have cross-knowledgeDomain access")
	}
	if IsCrossKnowledgeDomainSuperuser(context.Background(), nil) {
		t.Fatal("missing user must not have cross-knowledgeDomain access")
	}
}

func runCrossKnowledgeDomainHandler(user *types.User) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	r.Use(func(c *gin.Context) {
		if user != nil {
			ctx := context.WithValue(c.Request.Context(), types.UserContextKey, user)
			ctx = context.WithValue(ctx, types.UserIDContextKey, user.ID)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})
	r.GET("/cross", RequireCrossKnowledgeDomainAccess(nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cross", nil))
	return w
}

func TestRequireCrossKnowledgeDomainAccess(t *testing.T) {
	if got := runCrossKnowledgeDomainHandler(&types.User{ID: "root", IsSystemAdmin: true}).Code; got != http.StatusOK {
		t.Fatalf("system administrator status = %d, want 200", got)
	}
	for _, user := range []*types.User{{ID: "viewer"}, nil} {
		w := runCrossKnowledgeDomainHandler(user)
		assertEnvelope(t, w, http.StatusForbidden, apperrors.ErrForbidden)
	}
}

func runPathKnowledgeDomainHandler(ctxKnowledgeDomainID uint64, user *types.User, urlPath string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		if ctxKnowledgeDomainID != 0 {
			ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, ctxKnowledgeDomainID)
		}
		if user != nil {
			ctx = context.WithValue(ctx, types.UserContextKey, user)
			ctx = context.WithValue(ctx, types.UserIDContextKey, user.ID)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/knowledge-domains/:id", RequirePathKnowledgeDomainMatch(&config.Config{}), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, urlPath, nil))
	return w
}

func TestRequirePathKnowledgeDomainMatch(t *testing.T) {
	assertEnvelope(t, runPathKnowledgeDomainHandler(7, nil, "/knowledge-domains/7"), http.StatusForbidden, apperrors.ErrForbidden)
	assertEnvelope(t, runPathKnowledgeDomainHandler(7, &types.User{ID: "regular"}, "/knowledge-domains/7"), http.StatusForbidden, apperrors.ErrForbidden)
	if got := runPathKnowledgeDomainHandler(7, &types.User{ID: "root", IsSystemAdmin: true}, "/knowledge-domains/9").Code; got != http.StatusOK {
		t.Fatalf("system administrator mismatch status = %d, want 200", got)
	}
}

func assertEnvelope(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode apperrors.ErrorCode) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status: got %d, want %d (body=%s)", w.Code, wantStatus, w.Body.String())
	}
	var body struct {
		Code apperrors.ErrorCode `json:"code"`
		Msg  string              `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the standard envelope: %v (body=%s)", err, w.Body.String())
	}
	if body.Code != wantCode || body.Msg == "" {
		t.Fatalf("unexpected envelope: %s", w.Body.String())
	}
}
