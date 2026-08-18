package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
)

type enterpriseKBLookupStub struct {
	kb        *types.KnowledgeBase
	canRead   bool
	canManage bool
}

func (s *enterpriseKBLookupStub) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func (s *enterpriseKBLookupStub) CanReadKnowledgeBase(context.Context, *types.KnowledgeBase) (bool, error) {
	return s.canRead, nil
}

func (s *enterpriseKBLookupStub) CanManageKnowledgeBase(context.Context, *types.KnowledgeBase) (bool, error) {
	return s.canManage, nil
}

func runEnterpriseKBGuard(
	t *testing.T,
	lookup *enterpriseKBLookupStub,
	required types.KnowledgeBasePermissionLevel,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: lookup.kb.ID}}
	c.Request = httptest.NewRequest(http.MethodGet, "/knowledge-bases/"+lookup.kb.ID, nil)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), types.UserIDContextKey, "user-1"))

	RequireKBAccess(KBIDFromParam("id"), required, lookup)(c)
	return c, recorder
}

func TestRequireKBAccessUsesEnterpriseReadPermission(t *testing.T) {
	lookup := &enterpriseKBLookupStub{
		kb:      &types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1},
		canRead: true,
	}
	c, _ := runEnterpriseKBGuard(t, lookup, types.KnowledgeBasePermissionRead)
	require.False(t, c.IsAborted())
	access, ok := KBAccessFromContext(c)
	require.True(t, ok)
	require.Equal(t, types.KnowledgeBasePermissionRead, access.Permission)
}

func TestRequireKBAccessRejectsMissingManagePermission(t *testing.T) {
	lookup := &enterpriseKBLookupStub{
		kb:        &types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1},
		canRead:   true,
		canManage: false,
	}
	c, _ := runEnterpriseKBGuard(t, lookup, types.KnowledgeBasePermissionManage)
	require.True(t, c.IsAborted())
}

func TestRequireKBAccessDoesNotAllowForeignKnowledgeDomainFallback(t *testing.T) {
	lookup := &enterpriseKBLookupStub{
		kb:      &types.KnowledgeBase{ID: "kb-foreign", KnowledgeDomainID: 2},
		canRead: false,
	}
	c, _ := runEnterpriseKBGuard(t, lookup, types.KnowledgeBasePermissionRead)
	require.True(t, c.IsAborted())
}
