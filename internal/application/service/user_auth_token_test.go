package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"roche.local/knowledge-agent-platform/internal/types"
)

func init() {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-user-auth-token-tests")
}

type stubAuthTokenRepo struct {
	tokens         map[string]*types.AuthToken
	revokedUserIDs []string
	createCalls    int
	createErrAt    int
	updateErr      error
	deletedIDs     []string
}

func (s *stubAuthTokenRepo) CreateToken(_ context.Context, token *types.AuthToken) error {
	s.createCalls++
	if s.createErrAt == s.createCalls {
		return errors.New("token persistence failed")
	}
	if s.tokens == nil {
		s.tokens = map[string]*types.AuthToken{}
	}
	s.tokens[token.Token] = token
	return nil
}
func (s *stubAuthTokenRepo) GetTokenByValue(_ context.Context, tokenValue string) (*types.AuthToken, error) {
	token, ok := s.tokens[tokenValue]
	if !ok {
		return nil, errors.New("token not found")
	}
	return token, nil
}
func (s *stubAuthTokenRepo) GetTokensByUserID(context.Context, string) ([]*types.AuthToken, error) {
	return nil, nil
}
func (s *stubAuthTokenRepo) UpdateToken(context.Context, *types.AuthToken) error {
	return s.updateErr
}
func (s *stubAuthTokenRepo) DeleteToken(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return nil
}
func (s *stubAuthTokenRepo) DeleteExpiredTokens(context.Context) error { return nil }
func (s *stubAuthTokenRepo) RevokeTokensByUserID(_ context.Context, userID string) error {
	s.revokedUserIDs = append(s.revokedUserIDs, userID)
	return nil
}

type stubUserRepoForAuth struct {
	users map[string]*types.User
}

func (s *stubUserRepoForAuth) CreateUser(context.Context, *types.User) error { return nil }
func (s *stubUserRepoForAuth) GetUserByID(_ context.Context, id string) (*types.User, error) {
	user, ok := s.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}
func (s *stubUserRepoForAuth) GetUsersByIDs(context.Context, []string) (map[string]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByEmail(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByEmployeeID(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByUsername(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) UpdateUser(context.Context, *types.User) error { return nil }
func (s *stubUserRepoForAuth) DeleteUser(context.Context, string) error      { return nil }
func (s *stubUserRepoForAuth) ListUsers(context.Context, int, int, types.ListUsersFilter) ([]*types.User, int64, error) {
	return nil, 0, nil
}
func (s *stubUserRepoForAuth) BatchUpdateUserRoles(context.Context, []string, types.KnowledgeOfficerRolesPatch) (int64, error) {
	return 0, nil
}
func (s *stubUserRepoForAuth) UpdateUserRoles(context.Context, string, types.KnowledgeOfficerRolesPatch) (int64, error) {
	return 0, nil
}
func (s *stubUserRepoForAuth) BatchUpdateOperatorRole(context.Context, []string, types.OperatorRolesPatch) (int64, error) {
	return 0, nil
}
func (s *stubUserRepoForAuth) UpdateOperatorRole(context.Context, string, types.OperatorRolesPatch) (int64, error) {
	return 0, nil
}
func (s *stubUserRepoForAuth) ListSystemAdmins(context.Context, int, int) ([]*types.User, int64, error) {
	return nil, 0, nil
}
func (s *stubUserRepoForAuth) RevokeSystemAdmin(context.Context, string, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) SearchUsers(context.Context, string, int) ([]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetWorkdayWorkersByIDs(context.Context, string, []string) ([]*types.ExternalWorker, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUsersByEmails(context.Context, []string) (map[string]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) ResolveWorkdayOrgUnit(context.Context, string, string) (string, string, string, error) {
	return "", "", "", nil
}
func (s *stubUserRepoForAuth) CreateWorkdayLinkedUser(context.Context, *types.User, *types.ExternalWorker, string) error {
	return nil
}

func newAuthTestUserService(tokenRepo *stubAuthTokenRepo) *userService {
	return &userService{
		userRepo: &stubUserRepoForAuth{
			users: map[string]*types.User{
				"user-1": {ID: "user-1"},
			},
		},
		tokenRepo: tokenRepo,
	}
}

func signTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(getJwtSecret()))
	if err != nil {
		panic(err)
	}
	return signed
}

func TestValidateTokenRejectsRefreshToken(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[refreshJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     refreshJWT,
		TokenType: "refresh_token",
	}

	_, err := svc.ValidateToken(ctx, refreshJWT)
	if err == nil || err.Error() != "refresh token cannot be used as access token" {
		t.Fatalf("ValidateToken(refresh JWT) err = %v, want refresh rejection", err)
	}

	legacyRefresh := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[legacyRefresh] = &types.AuthToken{
		UserID:    "user-1",
		Token:     legacyRefresh,
		TokenType: "refresh_token",
	}

	_, err = svc.ValidateToken(ctx, legacyRefresh)
	if err == nil || err.Error() != "refresh token cannot be used as access token" {
		t.Fatalf("ValidateToken(legacy refresh in DB) err = %v, want refresh rejection", err)
	}
}

func TestRefreshTokenRejectsAccessTokenRecord(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[refreshJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     refreshJWT,
		TokenType: "access_token",
	}

	_, _, err := svc.RefreshToken(ctx, refreshJWT)
	if err == nil || err.Error() != "not a refresh token" {
		t.Fatalf("RefreshToken(access token record) err = %v, want not a refresh token", err)
	}
}

func TestLogoutRevokesAllUserTokens(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	expiredAccess := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	if err := svc.Logout(ctx, expiredAccess); err != nil {
		t.Fatalf("Logout(expired access token) err = %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("RevokeTokensByUserID calls = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestUserIDFromSignedTokenAcceptsExpiredToken(t *testing.T) {
	expired := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	userID, err := userIDFromSignedToken(expired)
	if err != nil {
		t.Fatalf("userIDFromSignedToken(expired) err = %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userIDFromSignedToken(expired) = %q, want user-1", userID)
	}
}

func TestGenerateTokensFailsWhenAccessTokenCannotBePersisted(t *testing.T) {
	tokenRepo := &stubAuthTokenRepo{createErrAt: 1}
	svc := newAuthTestUserService(tokenRepo)

	access, refresh, err := svc.GenerateTokens(context.Background(), &types.User{ID: "user-1"})
	if err == nil || err.Error() != "persist access token: token persistence failed" {
		t.Fatalf("GenerateTokens() err = %v, want access persistence error", err)
	}
	if access != "" || refresh != "" {
		t.Fatalf("GenerateTokens() returned unusable tokens: access=%q refresh=%q", access, refresh)
	}
}

func TestGenerateTokensCleansUpAccessTokenWhenRefreshPersistenceFails(t *testing.T) {
	tokenRepo := &stubAuthTokenRepo{createErrAt: 2}
	svc := newAuthTestUserService(tokenRepo)

	access, refresh, err := svc.GenerateTokens(context.Background(), &types.User{ID: "user-1"})
	if err == nil || err.Error() != "persist refresh token: token persistence failed" {
		t.Fatalf("GenerateTokens() err = %v, want refresh persistence error", err)
	}
	if access != "" || refresh != "" {
		t.Fatalf("GenerateTokens() returned partial session: access=%q refresh=%q", access, refresh)
	}
	if len(tokenRepo.deletedIDs) != 1 {
		t.Fatalf("DeleteToken() calls = %v, want one access-token cleanup", tokenRepo.deletedIDs)
	}
}

func TestRefreshTokenStopsWhenRotationCannotRevokeOldToken(t *testing.T) {
	now := time.Now()
	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     now.Add(time.Hour).Unix(),
	})
	tokenRepo := &stubAuthTokenRepo{
		tokens: map[string]*types.AuthToken{
			refreshJWT: {
				ID:        "refresh-1",
				UserID:    "user-1",
				Token:     refreshJWT,
				TokenType: "refresh_token",
				ExpiresAt: now.Add(time.Hour),
			},
		},
		updateErr: errors.New("database unavailable"),
	}
	svc := newAuthTestUserService(tokenRepo)

	access, refresh, err := svc.RefreshToken(context.Background(), refreshJWT)
	if err == nil || err.Error() != "revoke rotated refresh token: database unavailable" {
		t.Fatalf("RefreshToken() err = %v, want revoke error", err)
	}
	if access != "" || refresh != "" || tokenRepo.createCalls != 0 {
		t.Fatalf("RefreshToken() created a new session after revoke failed")
	}
}
