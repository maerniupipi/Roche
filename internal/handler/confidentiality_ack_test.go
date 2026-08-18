package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// confidentialityAckMockUserService is a minimal stub of interfaces.UserService
// that only implements the methods touched by the confidentiality-ack
// handlers. Any other method panics so we know if a test strays.
type confidentialityAckMockUserService struct {
	currentUser        *types.User
	currentUserErr     error
	acknowledged       bool
	acknowledgedAt     *time.Time
	getAckErr          error
	ackErr             error
	lastAckUserID      string
	getAckCalls        int
	ackCalls           int
}

func (m *confidentialityAckMockUserService) GetCurrentUser(ctx context.Context) (*types.User, error) {
	if m.currentUserErr != nil {
		return nil, m.currentUserErr
	}
	return m.currentUser, nil
}

func (m *confidentialityAckMockUserService) GetConfidentialityAck(_ context.Context, userID string) (bool, *time.Time, error) {
	m.getAckCalls++
	m.lastAckUserID = userID
	if m.getAckErr != nil {
		return false, nil, m.getAckErr
	}
	return m.acknowledged, m.acknowledgedAt, nil
}

func (m *confidentialityAckMockUserService) AcknowledgeConfidentiality(_ context.Context, userID string) (*time.Time, error) {
	m.ackCalls++
	m.lastAckUserID = userID
	if m.ackErr != nil {
		return nil, m.ackErr
	}
	return m.acknowledgedAt, nil
}

// The following methods are not used by the confidentiality-ack handlers and
// intentionally panic to surface any unexpected call.
func (m *confidentialityAckMockUserService) Register(context.Context, *types.RegisterRequest) (*types.User, error) {
	panic("unexpected Register")
}
func (m *confidentialityAckMockUserService) Login(context.Context, *types.LoginRequest) (*types.LoginResponse, error) {
	panic("unexpected Login")
}
func (m *confidentialityAckMockUserService) GetOIDCAuthorizationURL(context.Context, string) (*types.OIDCAuthURLResponse, error) {
	panic("unexpected GetOIDCAuthorizationURL")
}
func (m *confidentialityAckMockUserService) LoginWithOIDC(context.Context, string, string) (*types.OIDCCallbackResponse, error) {
	panic("unexpected LoginWithOIDC")
}
func (m *confidentialityAckMockUserService) GetSAMLAuthorizationURL(context.Context, string) (*types.SAMLAuthURLResponse, error) {
	panic("unexpected GetSAMLAuthorizationURL")
}
func (m *confidentialityAckMockUserService) LoginWithSAML(context.Context, *http.Request, string, string) (*types.SAMLCallbackResponse, error) {
	panic("unexpected LoginWithSAML")
}
func (m *confidentialityAckMockUserService) GetSAMLMetadata(context.Context) ([]byte, error) {
	panic("unexpected GetSAMLMetadata")
}
func (m *confidentialityAckMockUserService) GetUserByID(context.Context, string) (*types.User, error) {
	panic("unexpected GetUserByID")
}
func (m *confidentialityAckMockUserService) GetUserDetail(context.Context, string) (*types.UserDetailResponse, error) {
	panic("unexpected GetUserDetail")
}
func (m *confidentialityAckMockUserService) GetUsersByIDs(context.Context, []string) (map[string]*types.User, error) {
	panic("unexpected GetUsersByIDs")
}
func (m *confidentialityAckMockUserService) GetUserByEmail(context.Context, string) (*types.User, error) {
	panic("unexpected GetUserByEmail")
}
func (m *confidentialityAckMockUserService) GetUserByEmployeeID(context.Context, string) (*types.User, error) {
	panic("unexpected GetUserByEmployeeID")
}
func (m *confidentialityAckMockUserService) GetUserByUsername(context.Context, string) (*types.User, error) {
	panic("unexpected GetUserByUsername")
}
func (m *confidentialityAckMockUserService) UpdateUser(context.Context, *types.User) error {
	panic("unexpected UpdateUser")
}
func (m *confidentialityAckMockUserService) DeleteUser(context.Context, string) error {
	panic("unexpected DeleteUser")
}
func (m *confidentialityAckMockUserService) ChangePassword(context.Context, string, string, string) error {
	panic("unexpected ChangePassword")
}
func (m *confidentialityAckMockUserService) ValidatePassword(context.Context, string, string) error {
	panic("unexpected ValidatePassword")
}
func (m *confidentialityAckMockUserService) GenerateTokens(context.Context, *types.User) (string, string, error) {
	panic("unexpected GenerateTokens")
}
func (m *confidentialityAckMockUserService) ValidateToken(context.Context, string) (*types.User, error) {
	panic("unexpected ValidateToken")
}
func (m *confidentialityAckMockUserService) RefreshToken(context.Context, string) (string, string, error) {
	panic("unexpected RefreshToken")
}
func (m *confidentialityAckMockUserService) RevokeToken(context.Context, string) error {
	panic("unexpected RevokeToken")
}
func (m *confidentialityAckMockUserService) Logout(context.Context, string) error {
	panic("unexpected Logout")
}
func (m *confidentialityAckMockUserService) SearchUsers(context.Context, string, int) ([]*types.User, error) {
	panic("unexpected SearchUsers")
}
func (m *confidentialityAckMockUserService) ListSystemAdmins(context.Context, int, int) ([]*types.User, int64, error) {
	panic("unexpected ListSystemAdmins")
}
func (m *confidentialityAckMockUserService) RevokeSystemAdmin(context.Context, string, string) (*types.User, error) {
	panic("unexpected RevokeSystemAdmin")
}
func (m *confidentialityAckMockUserService) UpdateUserPreferences(context.Context, string, types.UserPreferences) (types.UserPreferences, error) {
	panic("unexpected UpdateUserPreferences")
}
func (m *confidentialityAckMockUserService) BanUser(context.Context, string, string) (*types.User, error) {
	panic("unexpected BanUser")
}
func (m *confidentialityAckMockUserService) UnbanUser(context.Context, string, string) (*types.User, error) {
	panic("unexpected UnbanUser")
}
func (m *confidentialityAckMockUserService) OfflineUser(context.Context, string, string) (*types.User, error) {
	panic("unexpected OfflineUser")
}
func (m *confidentialityAckMockUserService) BatchUpdateUserRoles(context.Context, []string, types.KnowledgeOfficerRolesPatch, string) (int64, error) {
	panic("unexpected BatchUpdateUserRoles")
}
func (m *confidentialityAckMockUserService) UpdateUserRoles(context.Context, string, types.KnowledgeOfficerRolesPatch, string) (int64, error) {
	panic("unexpected UpdateUserRoles")
}
func (m *confidentialityAckMockUserService) BatchUpdateOperatorRole(context.Context, []string, types.OperatorRolesPatch, string) (int64, error) {
	panic("unexpected BatchUpdateOperatorRole")
}
func (m *confidentialityAckMockUserService) UpdateOperatorRole(context.Context, string, types.OperatorRolesPatch, string) (int64, error) {
	panic("unexpected UpdateOperatorRole")
}
func (m *confidentialityAckMockUserService) ListUsers(context.Context, int, int, types.ListUsersFilter) ([]*types.User, int64, error) {
	panic("unexpected ListUsers")
}
func (m *confidentialityAckMockUserService) CreateUser(context.Context, *types.CreateUserRequest) (*types.User, error) {
	panic("unexpected CreateUser")
}

// Ensure the mock satisfies the interface so a signature drift between the
// test stub and the real interface is caught at compile time.
var _ interfaces.UserService = (*confidentialityAckMockUserService)(nil)

// newConfidentialityAckHandler builds an AuthHandler backed by the given mock
// and returns a *gin.Engine wired with the two confidentiality-ack routes plus
// the errorCapture middleware so c.Error(...) gets a JSON response.
func newConfidentialityAckHandler(t *testing.T, svc *confidentialityAckMockUserService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	h := NewAuthHandler(nil, svc)
	r.GET("/api/v1/auth/me/confidentiality-ack", h.GetConfidentialityAck)
	r.POST("/api/v1/auth/me/confidentiality-ack", h.AcknowledgeConfidentiality)
	return r
}

func doConfidentialityAckRequest(t *testing.T, r *gin.Engine, method string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/auth/me/confidentiality-ack", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGetConfidentialityAck_NotAcknowledged(t *testing.T) {
	svc := &confidentialityAckMockUserService{
		currentUser:    &types.User{ID: "u1", Username: "alice"},
		acknowledged:   false,
		acknowledgedAt: nil,
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodGet)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data struct {
			Acknowledged   bool       `json:"acknowledged"`
			AcknowledgedAt *time.Time `json:"acknowledged_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Code)
	assert.False(t, body.Data.Acknowledged)
	assert.Nil(t, body.Data.AcknowledgedAt)
	assert.Equal(t, "u1", svc.lastAckUserID)
	assert.Equal(t, 1, svc.getAckCalls)
}

func TestGetConfidentialityAck_Acknowledged(t *testing.T) {
	ts := time.Date(2026, 8, 17, 14, 5, 30, 0, time.UTC)
	svc := &confidentialityAckMockUserService{
		currentUser:    &types.User{ID: "u2", Username: "bob"},
		acknowledged:   true,
		acknowledgedAt: &ts,
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodGet)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Code int            `json:"code"`
		Data struct {
			Acknowledged   bool       `json:"acknowledged"`
			AcknowledgedAt *time.Time `json:"acknowledged_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Data.Acknowledged)
	require.NotNil(t, body.Data.AcknowledgedAt)
	assert.Equal(t, ts.UTC(), body.Data.AcknowledgedAt.UTC())
}

func TestAcknowledgeConfidentiality_Success(t *testing.T) {
	ts := time.Date(2026, 8, 17, 14, 6, 0, 0, time.UTC)
	svc := &confidentialityAckMockUserService{
		currentUser:    &types.User{ID: "u3", Username: "carol"},
		acknowledgedAt: &ts,
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodPost)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Code int            `json:"code"`
		Data struct {
			Acknowledged   bool       `json:"acknowledged"`
			AcknowledgedAt *time.Time `json:"acknowledged_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Code)
	assert.True(t, body.Data.Acknowledged)
	require.NotNil(t, body.Data.AcknowledgedAt)
	assert.Equal(t, ts.UTC(), body.Data.AcknowledgedAt.UTC())
	assert.Equal(t, 1, svc.ackCalls)
	assert.Equal(t, "u3", svc.lastAckUserID)
}

func TestGetConfidentialityAck_Unauthenticated(t *testing.T) {
	svc := &confidentialityAckMockUserService{
		currentUserErr: errors.New("no user in context"),
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodGet)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetConfidentialityAck_ServiceError(t *testing.T) {
	svc := &confidentialityAckMockUserService{
		currentUser: &types.User{ID: "u4", Username: "dave"},
		getAckErr:   errors.New("db down"),
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodGet)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAcknowledgeConfidentiality_ServiceError(t *testing.T) {
	svc := &confidentialityAckMockUserService{
		currentUser: &types.User{ID: "u5", Username: "erin"},
		ackErr:      errors.New("db down"),
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodPost)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAcknowledgeConfidentiality_Unauthenticated(t *testing.T) {
	svc := &confidentialityAckMockUserService{
		currentUserErr: apperrors.NewUnauthorizedError("no user in context"),
	}
	r := newConfidentialityAckHandler(t, svc)
	w := doConfidentialityAckRequest(t, r, http.MethodPost)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
