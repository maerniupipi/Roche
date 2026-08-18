package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestIsSAMLDevSystemAdmin(t *testing.T) {
	cfg := &config.Config{
		Registration: &config.RegistrationConfig{DevRoleSelection: true},
		SAMLAuth: &config.SAMLAuthConfig{
			DevSystemAdminEmails: []string{"developer001@rochekap.local"},
		},
	}

	if !isSAMLDevSystemAdmin(cfg, " Developer001@RocheKAP.Local ") {
		t.Fatal("configured development administrator was not recognized")
	}
	if isSAMLDevSystemAdmin(cfg, "developer011@rochekap.local") {
		t.Fatal("ordinary development user was recognized as a system administrator")
	}
	cfg.Registration.DevRoleSelection = false
	if isSAMLDevSystemAdmin(cfg, "developer001@rochekap.local") {
		t.Fatal("development administrator bootstrap remained active after the development guard was disabled")
	}
}

func TestProvisionSAMLUserPersistsDevelopmentSystemAdmin(t *testing.T) {
	state := &oidcFlowState{}
	svc := &userService{
		userRepo: &oidcFlowUserRepo{state: state},
		ssoRepo:  &oidcFlowSSORepo{state: state},
		config: &config.Config{
			Registration: &config.RegistrationConfig{DevRoleSelection: true},
			SAMLAuth: &config.SAMLAuthConfig{
				AutoProvision:        true,
				DevSystemAdminEmails: []string{"developer001@rochekap.local"},
			},
		},
	}

	user, err := svc.provisionSAMLUser(context.Background(), &types.SAMLUserInfo{
		Subject:  "developer001@rochekap.local",
		Username: "developer001",
		Email:    "developer001@rochekap.local",
	}, "urn:test:idp")
	if err != nil {
		t.Fatalf("provisionSAMLUser() error = %v", err)
	}
	if !user.IsSystemAdmin || state.user == nil || !state.user.IsSystemAdmin {
		t.Fatalf("development administrator was not persisted as a system administrator: %+v", state.user)
	}
}

func TestLoadSPKeyPairAllowsDevelopmentEphemeralCertificate(t *testing.T) {
	key, cert, err := loadSPKeyPair(&config.SAMLAuthConfig{AllowEphemeralSPCert: true})
	if err != nil {
		t.Fatalf("loadSPKeyPair() error = %v", err)
	}
	if key == nil || cert == nil {
		t.Fatal("loadSPKeyPair() returned an incomplete key pair")
	}
}

func TestGetSAMLAuthorizationURLWithDevelopmentEphemeralCertificate(t *testing.T) {
	t.Setenv("JWT_SECRET", "saml-development-test-secret")
	svc := &userService{config: &config.Config{SAMLAuth: &config.SAMLAuthConfig{
		Enable:               true,
		IdPMetadata:          `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.test/metadata"><IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"><SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.test/sso"/></IDPSSODescriptor></EntityDescriptor>`,
		SPEntityID:           "urn:test:sp",
		ACSUrl:               "https://app.example.test/api/v1/auth/saml/acs",
		ProviderDisplayName:  "Mock SAML",
		AllowEphemeralSPCert: true,
	}}}

	resp, err := svc.GetSAMLAuthorizationURL(context.Background(), "https://app.example.test/admin/")
	if err != nil {
		t.Fatalf("GetSAMLAuthorizationURL() error = %v", err)
	}
	if resp == nil || resp.AuthorizationURL == "" || resp.RelayState == "" || resp.Nonce == "" {
		t.Fatalf("GetSAMLAuthorizationURL() returned an incomplete response: %+v", resp)
	}
}

func TestGetSAMLAuthorizationURLRetriesAfterMetadataFailure(t *testing.T) {
	withOIDCSSRFWhitelist(t, "127.0.0.1")
	attempts := 0
	metadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		_, _ = w.Write([]byte(`<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.test/metadata"><IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"><SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.test/sso"/></IDPSSODescriptor></EntityDescriptor>`))
	}))
	defer metadataServer.Close()

	svc := &userService{config: &config.Config{SAMLAuth: &config.SAMLAuthConfig{
		Enable:               true,
		IdPMetadataURL:       metadataServer.URL,
		SPEntityID:           "urn:test:sp",
		ACSUrl:               "https://app.example.test/api/v1/auth/saml/acs",
		AllowEphemeralSPCert: true,
	}}}
	if _, err := svc.GetSAMLAuthorizationURL(context.Background(), "https://app.example.test/"); err == nil {
		t.Fatal("first GetSAMLAuthorizationURL() unexpectedly succeeded")
	}
	if _, err := svc.GetSAMLAuthorizationURL(context.Background(), "https://app.example.test/"); err != nil {
		t.Fatalf("second GetSAMLAuthorizationURL() did not recover: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("metadata requests = %d, want 2", attempts)
	}
}
