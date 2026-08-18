package dto

import "roche.local/knowledge-agent-platform/internal/types"

// AuthLoginResponse is the HTTP-safe login response shape.
type AuthLoginResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	User    *types.User `json:"user,omitempty"`
	Token   string      `json:"token,omitempty"`
}

// AuthOIDCCallbackResponse is the HTTP-safe OIDC callback payload shape.
type AuthOIDCCallbackResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	User      *types.User `json:"user,omitempty"`
	Token     string      `json:"token,omitempty"`
	IsNewUser bool        `json:"is_new_user,omitempty"`
}

// NewAuthLoginResponse converts a service-layer login response for HTTP output.
func NewAuthLoginResponse(resp *types.LoginResponse) *AuthLoginResponse {
	if resp == nil {
		return nil
	}
	return &AuthLoginResponse{
		Success: resp.Success,
		Message: resp.Message,
		User:    resp.User,
		Token:   resp.Token,
	}
}

// NewAuthOIDCCallbackResponse converts an OIDC callback response for HTTP output.
func NewAuthOIDCCallbackResponse(resp *types.OIDCCallbackResponse) *AuthOIDCCallbackResponse {
	if resp == nil {
		return nil
	}
	return &AuthOIDCCallbackResponse{
		Success:   resp.Success,
		Message:   resp.Message,
		User:      resp.User,
		Token:     resp.Token,
		IsNewUser: resp.IsNewUser,
	}
}

// NewAuthSAMLCallbackResponse converts a SAML callback response for HTTP
// output. The callback payload shape is identical to the OIDC one.
func NewAuthSAMLCallbackResponse(resp *types.SAMLCallbackResponse) *AuthOIDCCallbackResponse {
	if resp == nil {
		return nil
	}
	return &AuthOIDCCallbackResponse{
		Success:   resp.Success,
		Message:   resp.Message,
		User:      resp.User,
		Token:     resp.Token,
		IsNewUser: resp.IsNewUser,
	}
}
