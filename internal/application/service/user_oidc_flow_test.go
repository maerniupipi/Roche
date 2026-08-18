package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type oidcFlowState struct {
	user     *types.User
	identity *types.SSOIdentity
	tokens   []*types.AuthToken
}

type oidcFlowUserRepo struct {
	interfaces.UserRepository
	state *oidcFlowState
}

func (r *oidcFlowUserRepo) GetUserByID(_ context.Context, id string) (*types.User, error) {
	if r.state.user != nil && r.state.user.ID == id {
		return r.state.user, nil
	}
	return nil, errors.New("user not found")
}

func (r *oidcFlowUserRepo) GetUserByEmail(_ context.Context, email string) (*types.User, error) {
	if r.state.user != nil && r.state.user.Email == email {
		return r.state.user, nil
	}
	return nil, errors.New("user not found")
}

func (r *oidcFlowUserRepo) GetUserByUsername(_ context.Context, username string) (*types.User, error) {
	if r.state.user != nil && r.state.user.Username == username {
		return r.state.user, nil
	}
	return nil, errors.New("user not found")
}

func (r *oidcFlowUserRepo) UpdateUser(_ context.Context, user *types.User) error {
	r.state.user = user
	return nil
}

type oidcFlowTokenRepo struct {
	interfaces.AuthTokenRepository
	state *oidcFlowState
}

func (r *oidcFlowTokenRepo) CreateToken(_ context.Context, token *types.AuthToken) error {
	r.state.tokens = append(r.state.tokens, token)
	return nil
}

type oidcFlowSSORepo struct {
	interfaces.SSOIdentityRepository
	state *oidcFlowState
}

func (r *oidcFlowSSORepo) GetBySubject(_ context.Context, provider, issuer, subject string) (*types.SSOIdentity, error) {
	identity := r.state.identity
	if identity != nil && identity.Provider == provider && identity.Issuer == issuer && identity.Subject == subject {
		return identity, nil
	}
	return nil, nil
}

func (r *oidcFlowSSORepo) Upsert(_ context.Context, identity *types.SSOIdentity) error {
	r.state.identity = identity
	return nil
}

func (r *oidcFlowSSORepo) CreateEnterpriseUser(
	_ context.Context,
	user *types.User,
	identity *types.SSOIdentity,
) error {
	r.state.user = user
	r.state.identity = identity
	return nil
}

func TestLoginWithOIDCAutoProvisionsRegularUser(t *testing.T) {
	withOIDCSSRFWhitelist(t, "127.0.0.1")

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("code") != "mock-code" || r.Form.Get("client_secret") != "mock-secret" {
				t.Fatalf("unexpected token request: %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "mock-access-token"})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer mock-access-token" {
				t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":   "mock-subject-001",
				"email": "user@rochekap.local",
				"name":  "Mock User",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	state := &oidcFlowState{}
	svc := &userService{
		userRepo:  &oidcFlowUserRepo{state: state},
		tokenRepo: &oidcFlowTokenRepo{state: state},
		ssoRepo:   &oidcFlowSSORepo{state: state},
		config: &config.Config{OIDCAuth: &config.OIDCAuthConfig{
			Enable:                true,
			IssuerURL:             provider.URL,
			AuthorizationEndpoint: provider.URL + "/authorize",
			TokenEndpoint:         provider.URL + "/token",
			UserInfoEndpoint:      provider.URL + "/userinfo",
			ClientID:              "mock-client",
			ClientSecret:          "mock-secret",
			AutoProvision:         true,
			UserInfoMapping:       &config.OIDCUserInfoMapping{Username: "name", Email: "email"},
		}},
	}

	response, err := svc.LoginWithOIDC(
		context.Background(),
		"mock-code",
		"http://127.0.0.1:5173/api/v1/auth/oidc/callback",
	)
	if err != nil {
		t.Fatalf("LoginWithOIDC: %v", err)
	}
	if !response.Success || response.User == nil || response.User.Email != "user@rochekap.local" {
		t.Fatalf("unexpected OIDC response: %+v", response)
	}
	if state.identity == nil || state.identity.Subject != "mock-subject-001" || state.identity.UserID != response.User.ID {
		t.Fatalf("SSO identity was not persisted: %+v", state.identity)
	}
	if state.user.IsSystemAdmin {
		t.Fatal("OIDC auto-provisioning must create a regular user")
	}
	if len(state.tokens) != 2 || response.Token == "" || response.RefreshToken == "" {
		t.Fatalf("local token issuance failed: token records=%d", len(state.tokens))
	}
}

func TestProvisionOIDCUserDoesNotGrantAdministrativeAccess(t *testing.T) {
	state := &oidcFlowState{}
	svc := &userService{
		userRepo: &oidcFlowUserRepo{state: state},
		ssoRepo:  &oidcFlowSSORepo{state: state},
		config: &config.Config{OIDCAuth: &config.OIDCAuthConfig{
			Enable: true, AutoProvision: true,
		}},
	}

	user, err := svc.provisionOIDCUser(context.Background(), &types.OIDCUserInfo{
		Subject: "root-subject", Email: "root@rochekap.local", Username: "Mock Root",
	}, "mock-issuer")
	if err != nil {
		t.Fatalf("provisionOIDCUser: %v", err)
	}
	if user.IsSystemAdmin {
		t.Fatal("OIDC auto-provisioning must not grant system administrator")
	}
	if state.identity == nil || state.identity.UserID != user.ID {
		t.Fatalf("identity was not persisted: %+v", state.identity)
	}
}
