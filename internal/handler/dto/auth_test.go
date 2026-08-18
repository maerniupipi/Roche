package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestAuthLoginResponseDoesNotExposeRefreshToken(t *testing.T) {
	resp := NewAuthLoginResponse(&types.LoginResponse{
		Success:      true,
		User:         &types.User{ID: "user-1", Email: "user@example.com"},
		Token:        "access-token",
		RefreshToken: "refresh-token",
	})
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	s := string(body)
	assert.Contains(t, s, "user@example.com")
	assert.Contains(t, s, "access-token")
	assert.NotContains(t, s, "refresh-token")
	assert.NotContains(t, s, "refresh_token")
	assert.Contains(t, s, `"is_knowledge_domain_admin":false`)
	assert.NotContains(t, s, `"knowledge_domain_id"`)
	assert.NotContains(t, s, "memberships")
}

func TestAuthOIDCCallbackResponseContainsProvisioningFlag(t *testing.T) {
	resp := NewAuthOIDCCallbackResponse(&types.OIDCCallbackResponse{
		Success:   true,
		User:      &types.User{ID: "user-1"},
		IsNewUser: true,
	})
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"is_new_user":true`)
	assert.NotContains(t, string(body), "memberships")
}
