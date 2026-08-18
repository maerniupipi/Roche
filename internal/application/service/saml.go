package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/google/uuid"
	dsig "github.com/russellhaering/goxmldsig"
	"golang.org/x/crypto/bcrypt"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// samlSPManager owns the lazily-initialised SAML service provider. It is
// built once per process from the SAML auth configuration (IdP metadata +
// SP keypair) and reused across requests.
type samlSPManager struct {
	cfg    *config.SAMLAuthConfig
	sp     *saml.ServiceProvider
	issuer string
}

// getSAMLSManager returns the cached SAML SP manager, building it on first
// successful use. Building fetches/parses the IdP metadata and loads the SP
// keypair. A failed build remains retryable because IdP/network startup order
// must not permanently disable SAML for the lifetime of the Auth Service.
func (s *userService) getSAMLSManager(ctx context.Context) (*samlSPManager, error) {
	s.samlMgrMu.Lock()
	defer s.samlMgrMu.Unlock()
	if s.samlMgr != nil {
		return s.samlMgr, nil
	}
	manager, err := buildSAMLSManager(ctx, s.config)
	if err != nil {
		return nil, err
	}
	s.samlMgr = manager
	return manager, nil
}

func buildSAMLSManager(ctx context.Context, cfg *config.Config) (*samlSPManager, error) {
	if cfg == nil || cfg.SAMLAuth == nil || !cfg.SAMLAuth.Enable {
		return nil, errors.New("SAML login is disabled")
	}
	c := cfg.SAMLAuth
	manager := &samlSPManager{cfg: c}

	metadata, err := manager.fetchIDPMetadata(ctx)
	if err != nil {
		return nil, err
	}
	idpEntity, err := samlsp.ParseMetadata(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IdP metadata: %w", err)
	}
	if len(idpEntity.IDPSSODescriptors) == 0 {
		return nil, errors.New("IdP metadata contains no IDPSSODescriptor")
	}

	key, cert, err := loadSPKeyPair(c)
	if err != nil {
		return nil, err
	}

	acsURL, err := url.Parse(strings.TrimSpace(c.ACSUrl))
	if err != nil {
		return nil, fmt.Errorf("invalid saml_auth.acs_url: %w", err)
	}
	if acsURL.Scheme == "" || acsURL.Host == "" {
		return nil, errors.New("saml_auth.acs_url must be an absolute URL")
	}
	metadataURL := deriveSAMLMetadataURL(acsURL)

	sp := &saml.ServiceProvider{
		EntityID:           strings.TrimSpace(c.SPEntityID),
		Key:                key,
		Certificate:        cert,
		MetadataURL:        *metadataURL,
		AcsURL:             *acsURL,
		IDPMetadata:        idpEntity,
		AuthnNameIDFormat:  saml.EmailAddressNameIDFormat,
		AllowIDPInitiated:  c.AllowIDPInitiated,
		DefaultRedirectURI: "/",
	}
	// SAML request signing is driven by SignatureMethod: when set, outgoing
	// AuthnRequests carry a SigAlg + Signature (redirect binding) or an
	// enveloped XML signature (POST binding).
	if c.SignRequest {
		sp.SignatureMethod = dsig.RSASHA256SignatureMethod
	}
	manager.sp = sp
	manager.issuer = strings.TrimSpace(idpEntity.EntityID)
	if manager.issuer == "" {
		manager.issuer = strings.TrimSpace(c.SPEntityID)
	}
	return manager, nil
}

// deriveSAMLMetadataURL turns the ACS URL
// (https://host/api/v1/auth/saml/acs) into the SP metadata URL
// (https://host/api/v1/auth/saml/metadata).
func deriveSAMLMetadataURL(acsURL *url.URL) *url.URL {
	u := *acsURL
	path := strings.TrimSuffix(u.Path, "/")
	if strings.HasSuffix(path, "/acs") {
		u.Path = strings.TrimSuffix(path, "acs") + "metadata"
	} else {
		u.Path = path + "/metadata"
	}
	return &u
}

// fetchIDPMetadata returns the raw IdP metadata XML from the configured
// source: a remote URL (SSRF-validated), a local file, or embedded text.
func (m *samlSPManager) fetchIDPMetadata(ctx context.Context) ([]byte, error) {
	cfg := m.cfg
	switch {
	case strings.TrimSpace(cfg.IdPMetadataURL) != "":
		if err := secutils.ValidateURLForSSRF(cfg.IdPMetadataURL); err != nil {
			return nil, fmt.Errorf("SAML IdP metadata URL failed SSRF validation: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(cfg.IdPMetadataURL), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create IdP metadata request: %w", err)
		}
		resp, err := newOIDCHTTPClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch IdP metadata: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
			return nil, fmt.Errorf("IdP metadata request failed: status=%d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	case strings.TrimSpace(cfg.IdPMetadataFile) != "":
		return os.ReadFile(strings.TrimSpace(cfg.IdPMetadataFile))
	case strings.TrimSpace(cfg.IdPMetadata) != "":
		return []byte(cfg.IdPMetadata), nil
	default:
		return nil, errors.New("SAML idp_metadata_url, idp_metadata_file or idp_metadata is required")
	}
}

// loadSPKeyPair loads the SP certificate/key from files or raw PEM, or
// generates an ephemeral self-signed keypair when neither is configured
// (development only — the metadata changes on every restart).
func loadSPKeyPair(cfg *config.SAMLAuthConfig) (*rsa.PrivateKey, *x509.Certificate, error) {
	var certPEM, keyPEM []byte
	switch {
	case strings.TrimSpace(cfg.SPCertFile) != "" && strings.TrimSpace(cfg.SPKeyFile) != "":
		var err error
		if certPEM, err = os.ReadFile(strings.TrimSpace(cfg.SPCertFile)); err != nil {
			return nil, nil, fmt.Errorf("failed to read SAML SP certificate: %w", err)
		}
		if keyPEM, err = os.ReadFile(strings.TrimSpace(cfg.SPKeyFile)); err != nil {
			return nil, nil, fmt.Errorf("failed to read SAML SP key: %w", err)
		}
	case strings.TrimSpace(cfg.SPCert) != "" && strings.TrimSpace(cfg.SPKey) != "":
		certPEM = []byte(cfg.SPCert)
		keyPEM = []byte(cfg.SPKey)
	}

	if len(certPEM) > 0 && len(keyPEM) > 0 {
		block, _ := pem.Decode(certPEM)
		if block == nil {
			return nil, nil, errors.New("failed to decode SAML SP certificate PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse SAML SP certificate: %w", err)
		}
		key, err := parseSPPrivateKey(keyPEM)
		if err != nil {
			return nil, nil, err
		}
		return key, cert, nil
	}

	if !cfg.AllowEphemeralSPCert {
		return nil, nil, errors.New("stable SAML SP certificate/key are required")
	}
	logger.Warn(context.Background(), "SAML development mode: generating an ephemeral self-signed SP keypair; metadata will change on restart")
	return generateEphemeralSPKeyPair()
}

func parseSPPrivateKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("failed to decode SAML SP private key PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("unsupported SAML SP private key format")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("SAML SP private key must be an RSA key")
	}
	return key, nil
}

func generateEphemeralSPKeyPair() (*rsa.PrivateKey, *x509.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate SAML SP key: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "rochekap-saml-sp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create SAML SP certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse SAML SP certificate: %w", err)
	}
	return key, cert, nil
}

// GetSAMLAuthorizationURL builds the SP-initiated single sign-on URL for the
// configured IdP. The front-end redirect URI is embedded in the signed
// RelayState so the ACS can route the browser back after login.
func (s *userService) GetSAMLAuthorizationURL(
	ctx context.Context,
	redirectURI string,
) (*types.SAMLAuthURLResponse, error) {
	if strings.TrimSpace(redirectURI) == "" {
		return nil, errors.New("redirect_uri is required")
	}
	manager, err := s.getSAMLSManager(ctx)
	if err != nil {
		return nil, err
	}
	nonce, err := generateRandomString(24)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	authRequest, err := manager.sp.MakeAuthenticationRequest(
		manager.sp.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create SAML authentication request: %w", err)
	}
	relayState, err := secutils.SignSSOState(&secutils.SSOStatePayload{
		Nonce:       nonce,
		RedirectURI: strings.TrimSpace(redirectURI),
		RequestID:   authRequest.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode SAML relay state: %w", err)
	}

	authURL, err := authRequest.Redirect(relayState, manager.sp)
	if err != nil {
		return nil, fmt.Errorf("failed to create SAML authentication request: %w", err)
	}

	return &types.SAMLAuthURLResponse{
		Success:             true,
		ProviderDisplayName: manager.cfg.ProviderDisplayName,
		AuthorizationURL:    authURL.String(),
		RelayState:          relayState,
		Nonce:               nonce,
	}, nil
}

// LoginWithSAML validates the SAMLResponse carried by req (query parameter
// for the redirect binding, form field for the POST binding), resolves or
// provisions the local user and returns local login tokens.
func (s *userService) LoginWithSAML(
	ctx context.Context,
	req *http.Request,
	redirectURI string,
	requestID string,
) (*types.SAMLCallbackResponse, error) {
	if req == nil {
		return nil, errors.New("SAML response request is required")
	}
	manager, err := s.getSAMLSManager(ctx)
	if err != nil {
		return nil, err
	}

	possibleRequestIDs := make([]string, 0, 1)
	if strings.TrimSpace(requestID) != "" {
		possibleRequestIDs = append(possibleRequestIDs, strings.TrimSpace(requestID))
	} else if manager.cfg.AllowIDPInitiated {
		possibleRequestIDs = append(possibleRequestIDs, "")
	} else {
		return nil, errors.New("SAML request ID is required for SP-initiated login")
	}
	assertion, err := manager.sp.ParseResponse(req, possibleRequestIDs)
	if err != nil {
		return nil, fmt.Errorf("invalid SAML response: %w", err)
	}

	info := extractSAMLUserInfo(manager.cfg, assertion)
	if strings.TrimSpace(info.Email) == "" && strings.TrimSpace(info.Username) == "" && strings.TrimSpace(info.EmployeeID) == "" {
		return nil, errors.New("SAML assertion did not include a usable email, username or employee id")
	}
	if strings.TrimSpace(info.Subject) == "" {
		info.Subject = strings.TrimSpace(info.Email)
	}
	if strings.TrimSpace(info.Subject) == "" {
		info.Subject = strings.TrimSpace(info.EmployeeID)
	}
	if strings.TrimSpace(info.Subject) == "" {
		return nil, errors.New("SAML assertion did not include a stable subject")
	}

	issuer := manager.issuer
	user, isNewUser, err := s.findOrProvisionSAMLUser(ctx, issuer, info)
	if err != nil {
		return nil, err
	}

	if user.Status != types.UserStatusNormal {
		return &types.SAMLCallbackResponse{Success: false, Message: "Account is disabled"}, nil
	}
	if user.IsBanned() {
		return &types.SAMLCallbackResponse{Success: false, Message: "Account is banned"}, nil
	}
	if !isNewUser {
		now := time.Now()
		if err := s.ssoRepo.Upsert(ctx, &types.SSOIdentity{
			UserID: user.ID, Provider: "saml", Issuer: issuer, Subject: info.Subject,
			CreatedAt: now, UpdatedAt: now, LastLoginAt: &now,
		}); err != nil {
			return nil, fmt.Errorf("failed to persist SSO identity: %w", err)
		}
	}
	s.enrichKnowledgeDomainAdminFlag(ctx, user)

	accessToken, refreshToken, err := s.GenerateTokens(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate local tokens: %w", err)
	}

	s.recordLoginSuccess(ctx, user, "saml")
	return &types.SAMLCallbackResponse{
		Success:      true,
		Message:      "Login successful",
		User:         user,
		Token:        accessToken,
		RefreshToken: refreshToken,
		IsNewUser:    isNewUser,
	}, nil
}

// GetSAMLMetadata returns the SP metadata XML used to register this
// application as a SAML service provider with the IdP.
func (s *userService) GetSAMLMetadata(ctx context.Context) ([]byte, error) {
	manager, err := s.getSAMLSManager(ctx)
	if err != nil {
		return nil, err
	}
	return xml.MarshalIndent(manager.sp.Metadata(), "", "  ")
}

// findOrProvisionSAMLUser locates an existing local account for a SAML
// assertion. It tries, in order: the persisted SSO identity (subject), the
// email attribute, then the employee id attribute. When a pre-created account
// is matched by email or employee id, missing fields are back-filled from the
// SAML assertion. If no account is found, a new one is provisioned when
// AutoProvision is enabled.
func (s *userService) findOrProvisionSAMLUser(
	ctx context.Context,
	issuer string,
	info *types.SAMLUserInfo,
) (*types.User, bool, error) {
	identity, err := s.ssoRepo.GetBySubject(ctx, "saml", issuer, info.Subject)
	if err != nil {
		return nil, false, fmt.Errorf("failed to query SSO identity: %w", err)
	}
	if identity != nil {
		user, err := s.userRepo.GetUserByID(ctx, identity.UserID)
		if err != nil && !isUserLookupNotFound(err) {
			return nil, false, fmt.Errorf("failed to query user: %w", err)
		}
		if user != nil {
			return user, false, nil
		}
	}

	if info.Email != "" {
		user, err := s.userRepo.GetUserByEmail(ctx, info.Email)
		if err != nil && !isUserLookupNotFound(err) {
			return nil, false, fmt.Errorf("failed to query user: %w", err)
		}
		if user != nil {
			if _, err := s.backfillUserFromSAML(ctx, user, info); err != nil {
				return nil, false, err
			}
			return user, false, nil
		}
	}

	if info.EmployeeID != "" {
		user, err := s.userRepo.GetUserByEmployeeID(ctx, info.EmployeeID)
		if err != nil && !isUserLookupNotFound(err) {
			return nil, false, fmt.Errorf("failed to query user: %w", err)
		}
		if user != nil {
			if _, err := s.backfillUserFromSAML(ctx, user, info); err != nil {
				return nil, false, err
			}
			return user, false, nil
		}
	}

	user, err := s.provisionSAMLUser(ctx, info, issuer)
	if err != nil {
		return nil, false, err
	}
	return user, true, nil
}

// backfillUserFromSAML populates empty profile fields on a pre-created local
// account from a SAML assertion.
func (s *userService) backfillUserFromSAML(
	ctx context.Context,
	user *types.User,
	info *types.SAMLUserInfo,
) (bool, error) {
	changed := false
	if user.Email == "" && info.Email != "" {
		existing, _ := s.userRepo.GetUserByEmail(ctx, info.Email)
		if existing != nil && existing.ID != user.ID {
			return false, errors.New("SAML email is already bound to another user")
		}
		user.Email = info.Email
		changed = true
	}
	if user.EmployeeID == "" && info.EmployeeID != "" {
		user.EmployeeID = info.EmployeeID
		changed = true
	}
	if user.EnglishName == "" && info.Username != "" {
		user.EnglishName = info.Username
		changed = true
	}
	if !changed {
		return false, nil
	}
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return false, fmt.Errorf("failed to back-fill user from SAML assertion: %w", err)
	}
	return true, nil
}

// provisionSAMLUser auto-provisions a local account for a first-time SAML
// user. The account is created inactive-password (random, unusable) and the
// SSO identity row links it to the IdP subject.
func (s *userService) provisionSAMLUser(
	ctx context.Context,
	info *types.SAMLUserInfo,
	issuer string,
) (*types.User, error) {
	if s.config == nil || s.config.SAMLAuth == nil || !s.config.SAMLAuth.AutoProvision {
		return nil, errors.New("SSO account is not provisioned; contact an enterprise administrator")
	}
	username := s.generateOIDCUsername(ctx, &types.OIDCUserInfo{Username: info.Username, Email: info.Email})
	randomPassword, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password for SAML user: %w", err)
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash SAML fallback password: %w", err)
	}

	now := time.Now()
	user := &types.User{
		ID: uuid.New().String(), Username: username, Email: info.Email,
		PasswordHash:  string(hashedPassword),
		IsSystemAdmin: isSAMLDevSystemAdmin(s.config, info.Email),
		CreatedAt:     now, UpdatedAt: now,
	}
	identity := &types.SSOIdentity{
		UserID: user.ID, Provider: "saml", Issuer: issuer, Subject: info.Subject,
		CreatedAt: now, UpdatedAt: now, LastLoginAt: &now,
	}
	if err := s.ssoRepo.CreateEnterpriseUser(ctx, user, identity); err != nil {
		return nil, fmt.Errorf("failed to auto-provision SAML user: %w", err)
	}
	return user, nil
}

func isSAMLDevSystemAdmin(cfg *config.Config, email string) bool {
	if cfg == nil || cfg.Registration == nil || !cfg.Registration.DevRoleSelection || cfg.SAMLAuth == nil {
		return false
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	for _, allowedEmail := range cfg.SAMLAuth.DevSystemAdminEmails {
		if normalizedEmail == strings.ToLower(strings.TrimSpace(allowedEmail)) {
			return true
		}
	}
	return false
}

// extractSAMLUserInfo maps a validated SAML assertion to local identity
// claims. Attribute lookup is case-insensitive and falls back across the
// common OID/SAML attribute names when the configured mapping is absent.
func extractSAMLUserInfo(cfg *config.SAMLAuthConfig, assertion *saml.Assertion) *types.SAMLUserInfo {
	info := &types.SAMLUserInfo{Attributes: map[string]string{}}
	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		info.Subject = strings.TrimSpace(assertion.Subject.NameID.Value)
	}

	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			var values []string
			for _, v := range attr.Values {
				if val := strings.TrimSpace(v.Value); val != "" {
					values = append(values, val)
				}
			}
			if len(values) == 0 {
				continue
			}
			joined := strings.Join(values, ",")
			info.Attributes[strings.ToLower(attr.Name)] = joined
			if attr.FriendlyName != "" {
				info.Attributes[strings.ToLower(attr.FriendlyName)] = joined
			}
		}
	}

	mapping := cfg.UserInfoMapping
	if mapping == nil {
		mapping = &config.SAMLUserInfoMapping{Subject: "subject", Username: "username", Email: "email"}
	}

	info.Email = samlAttributeValue(info.Attributes,
		mapping.Email, "email", "mail",
		"edupersonprincipalname",
		"urn:oid:0.9.2342.19200300.100.1.3",
		"urn:oid:1.3.6.1.4.1.5923.1.1.1.6")
	info.EmployeeID = samlAttributeValue(info.Attributes,
		mapping.EmployeeID, "employee_id", "employeeid", "emp_id", "workforceid",
		"urn:oid:2.16.840.1.113730.3.1.3",
		"urn:oid:2.16.840.1.113730.3.1.604")
	info.Username = samlAttributeValue(info.Attributes,
		mapping.Username, "username", "name", "displayname",
		"urn:oid:2.16.840.1.113730.3.1.241",
		"uid", "urn:oid:0.9.2342.19200300.100.1.1")
	if subj := samlAttributeValue(info.Attributes,
		mapping.Subject, "subject", "uid", "urn:oid:0.9.2342.19200300.100.1.1"); subj != "" {
		info.Subject = subj
	}
	return info
}

// samlAttributeValue returns the first non-empty attribute value among the
// candidate keys (case-insensitive).
func samlAttributeValue(attrs map[string]string, keys ...string) string {
	for _, k := range keys {
		k = strings.TrimSpace(strings.ToLower(k))
		if k == "" {
			continue
		}
		if v, ok := attrs[k]; ok {
			if val := strings.TrimSpace(v); val != "" {
				return val
			}
		}
	}
	return ""
}
