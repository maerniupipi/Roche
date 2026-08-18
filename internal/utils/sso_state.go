package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const ssoStateMaxAge = 10 * time.Minute

var (
	ssoStateSecretOnce sync.Once
	ssoStateSecret     string
)

// ssoStateSigningKey returns the HMAC key used to sign OIDC and SAML state
// tokens. It prefers JWT_SECRET so all signature-bearing artifacts share one
// operator-supplied secret; otherwise a random per-process key is used.
func ssoStateSigningKey() string {
	ssoStateSecretOnce.Do(func() {
		if envSecret := strings.TrimSpace(os.Getenv("JWT_SECRET")); envSecret != "" {
			ssoStateSecret = envSecret
			return
		}
		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			panic(fmt.Sprintf("failed to generate SSO state signing key: %v", err))
		}
		ssoStateSecret = base64.StdEncoding.EncodeToString(randomBytes)
	})
	return ssoStateSecret
}

// SSOStatePayload is the tamper-evident round-trip state shared by the OIDC
// and SAML single sign-on flows. It binds a browser nonce and the front-end
// redirect URI to the state token that is echoed back by the identity
// provider on the callback.
type SSOStatePayload struct {
	Nonce       string `json:"nonce"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	// RequestID binds a SAML response to the exact AuthnRequest that started
	// the browser flow. OIDC leaves this field empty.
	RequestID string `json:"request_id,omitempty"`
	IssuedAt  int64  `json:"iat"`
}

// SignSSOState returns a tamper-evident state token:
// base64url(payload).base64url(hmac-sha256(payload)).
func SignSSOState(payload *SSOStatePayload) (string, error) {
	if payload == nil {
		return "", errors.New("sso state payload is required")
	}
	if strings.TrimSpace(payload.Nonce) == "" {
		return "", errors.New("sso state nonce is required")
	}
	if strings.TrimSpace(payload.RedirectURI) == "" {
		return "", errors.New("sso state redirect_uri is required")
	}
	if payload.IssuedAt == 0 {
		payload.IssuedAt = time.Now().Unix()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal sso state: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(ssoStateSigningKey()))
	mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifySSOState validates the HMAC and freshness of a state token and
// returns the decoded payload.
func VerifySSOState(raw string) (*SSOStatePayload, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid sso state format")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode sso state payload: %w", err)
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode sso state signature: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(ssoStateSigningKey()))
	mac.Write(payloadBytes)
	if !hmac.Equal(mac.Sum(nil), sigBytes) {
		return nil, errors.New("sso state signature mismatch")
	}
	var payload SSOStatePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal sso state: %w", err)
	}
	if strings.TrimSpace(payload.RedirectURI) == "" {
		return nil, errors.New("state.redirect_uri is required")
	}
	if payload.IssuedAt == 0 {
		return nil, errors.New("state.iat is required")
	}
	issuedAt := time.Unix(payload.IssuedAt, 0)
	if time.Since(issuedAt) > ssoStateMaxAge || time.Until(issuedAt) > time.Minute {
		return nil, errors.New("sso state expired or invalid timestamp")
	}
	return &payload, nil
}
