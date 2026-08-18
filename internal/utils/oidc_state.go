package utils

import (
	"errors"
)

// OIDCStatePayload is the signed OIDC authorization state carried in the
// redirect URL and validated on callback.
type OIDCStatePayload struct {
	Nonce       string `json:"nonce"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	IssuedAt    int64  `json:"iat"`
}

// SignOIDCState returns a tamper-evident state token: base64url(payload).base64url(hmac).
// It shares the HMAC mechanism with the SAML relay state (see SignSSOState).
func SignOIDCState(payload *OIDCStatePayload) (string, error) {
	if payload == nil {
		return "", errors.New("oidc state payload is required")
	}
	return SignSSOState(&SSOStatePayload{
		Nonce:       payload.Nonce,
		RedirectURI: payload.RedirectURI,
		IssuedAt:    payload.IssuedAt,
	})
}

// VerifyOIDCState validates the HMAC and freshness of a state token.
func VerifyOIDCState(raw string) (*OIDCStatePayload, error) {
	payload, err := VerifySSOState(raw)
	if err != nil {
		return nil, err
	}
	return &OIDCStatePayload{
		Nonce:       payload.Nonce,
		RedirectURI: payload.RedirectURI,
		IssuedAt:    payload.IssuedAt,
	}, nil
}
