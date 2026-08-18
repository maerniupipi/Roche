package authserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/types"
)

type fakeTokenValidator struct {
	user *types.User
	err  error
}

func (f fakeTokenValidator) ValidateToken(context.Context, string) (*types.User, error) {
	return f.user, f.err
}

func TestInternalTokenValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "01234567890123456789012345678901"

	tests := []struct {
		name       string
		validator  fakeTokenValidator
		secret     string
		auth       string
		wantStatus int
		wantUserID string
	}{
		{name: "rejects missing gateway secret", validator: fakeTokenValidator{}, auth: "Bearer token", wantStatus: http.StatusForbidden},
		{name: "rejects missing bearer token", validator: fakeTokenValidator{}, secret: secret, wantStatus: http.StatusUnauthorized},
		{name: "rejects invalid token", validator: fakeTokenValidator{err: errors.New("invalid")}, secret: secret, auth: "Bearer invalid", wantStatus: http.StatusUnauthorized},
		{name: "returns trusted identity headers", validator: fakeTokenValidator{user: &types.User{ID: "user-1", Email: "user@example.com", IsSystemAdmin: true, RoleKnowledgeOfficer: 2}}, secret: secret, auth: "Bearer valid", wantStatus: http.StatusNoContent, wantUserID: "user-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			handler := NewInternalTokenValidator(tt.validator, secret)
			router.GET("/validate", handler.Validate)

			req := httptest.NewRequest(http.MethodGet, "/validate", nil)
			if tt.secret != "" {
				req.Header.Set(internalSecretHeader, tt.secret)
			}
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.Code, tt.wantStatus)
			}
			if got := resp.Header().Get("X-Authenticated-User-ID"); got != tt.wantUserID {
				t.Fatalf("X-Authenticated-User-ID = %q, want %q", got, tt.wantUserID)
			}
		})
	}
}
