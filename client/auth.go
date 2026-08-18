package client

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is returned by POST /api/v1/auth/login.
// Authorization is user-scoped; no active knowledge domain is encoded in the
// response or the access token.
type LoginResponse struct {
	Success      bool      `json:"success"`
	Message      string    `json:"message,omitempty"`
	User         *AuthUser `json:"user,omitempty"`
	Token        string    `json:"token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
}

// AuthUser is the principal returned by /auth/login and /auth/me.
type AuthUser struct {
	ID                     string    `json:"id"`
	Username               string    `json:"username"`
	Email                  string    `json:"email"`
	Avatar                 string    `json:"avatar,omitempty"`
	IsActive               bool      `json:"is_active"`
	IsSystemAdmin          bool      `json:"is_system_admin,omitempty"`
	IsKnowledgeDomainAdmin bool      `json:"is_knowledge_domain_admin,omitempty"`
	CreatedAt              time.Time `json:"created_at,omitempty"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

type CurrentUserResponse struct {
	Success bool `json:"success"`
	Data    struct {
		User *AuthUser `json:"user,omitempty"`
	} `json:"data"`
}

const (
	PathAuthLogin   = "/api/v1/auth/login"
	PathAuthRefresh = "/api/v1/auth/refresh"
)

type RefreshTokenResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Client) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, PathAuthLogin, req, nil)
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	var out LoginResponse
	if err := parseResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetCurrentUser(ctx context.Context) (*CurrentUserResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	var out CurrentUserResponse
	if err := parseResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenResponse, error) {
	body := struct {
		RefreshToken string `json:"refreshToken"`
	}{RefreshToken: refreshToken}
	resp, err := c.doRequest(ctx, http.MethodPost, PathAuthRefresh, body, nil)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	var out RefreshTokenResponse
	if err := parseResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
