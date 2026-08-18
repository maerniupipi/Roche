package authserver_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlidp"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/html"

	"roche.local/knowledge-agent-platform/internal/application/service"
	"roche.local/knowledge-agent-platform/internal/authserver"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

type samlFlowUserRepo struct {
	interfaces.UserRepository
	mu    sync.Mutex
	byID  map[string]*types.User
	byKey map[string]*types.User
}

func newSAMLFlowUserRepo() *samlFlowUserRepo {
	return &samlFlowUserRepo{
		byID:  map[string]*types.User{},
		byKey: map[string]*types.User{},
	}
}

func (r *samlFlowUserRepo) GetUserByID(_ context.Context, id string) (*types.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if user := r.byID[id]; user != nil {
		return user, nil
	}
	return nil, fmt.Errorf("user not found")
}

func (r *samlFlowUserRepo) GetUserByEmail(_ context.Context, email string) (*types.User, error) {
	return r.getByKey(strings.ToLower(strings.TrimSpace(email)))
}

func (r *samlFlowUserRepo) GetUserByUsername(_ context.Context, username string) (*types.User, error) {
	return r.getByKey("username:" + strings.ToLower(strings.TrimSpace(username)))
}

func (r *samlFlowUserRepo) getByKey(key string) (*types.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if user := r.byKey[key]; user != nil {
		return user, nil
	}
	return nil, fmt.Errorf("user not found")
}

func (r *samlFlowUserRepo) put(user *types.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[user.ID] = user
	r.byKey[strings.ToLower(user.Email)] = user
	r.byKey["username:"+strings.ToLower(user.Username)] = user
}

type samlFlowTokenRepo struct {
	interfaces.AuthTokenRepository
	mu     sync.Mutex
	byText map[string]*types.AuthToken
}

func newSAMLFlowTokenRepo() *samlFlowTokenRepo {
	return &samlFlowTokenRepo{byText: map[string]*types.AuthToken{}}
}

func (r *samlFlowTokenRepo) CreateToken(_ context.Context, token *types.AuthToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byText[token.Token] = token
	return nil
}

func (r *samlFlowTokenRepo) GetTokenByValue(_ context.Context, value string) (*types.AuthToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token := r.byText[value]; token != nil {
		return token, nil
	}
	return nil, fmt.Errorf("token not found")
}

func (r *samlFlowTokenRepo) UpdateToken(_ context.Context, token *types.AuthToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byText[token.Token] = token
	return nil
}

func (r *samlFlowTokenRepo) DeleteToken(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for value, token := range r.byText {
		if token.ID == id {
			delete(r.byText, value)
			break
		}
	}
	return nil
}

type samlFlowSSORepo struct {
	interfaces.SSOIdentityRepository
	mu       sync.Mutex
	users    *samlFlowUserRepo
	identity *types.SSOIdentity
}

func (r *samlFlowSSORepo) GetBySubject(_ context.Context, provider, issuer, subject string) (*types.SSOIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.identity != nil && r.identity.Provider == provider && r.identity.Issuer == issuer && r.identity.Subject == subject {
		return r.identity, nil
	}
	return nil, nil
}

func (r *samlFlowSSORepo) Upsert(_ context.Context, identity *types.SSOIdentity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.identity = identity
	return nil
}

func (r *samlFlowSSORepo) CreateEnterpriseUser(_ context.Context, user *types.User, identity *types.SSOIdentity) error {
	r.users.put(user)
	r.mu.Lock()
	r.identity = identity
	r.mu.Unlock()
	return nil
}

type htmlForm struct {
	action string
	values url.Values
}

func parseHTMLForm(t *testing.T, response *http.Response) htmlForm {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read HTML form: %v", err)
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("parse HTML form: %v", err)
	}
	form := htmlForm{values: url.Values{}}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "form" && form.action == "" {
			for _, attr := range node.Attr {
				if attr.Key == "action" {
					form.action = attr.Val
				}
			}
		}
		if node.Type == html.ElementNode && node.Data == "input" {
			var name, value string
			for _, attr := range node.Attr {
				switch attr.Key {
				case "name":
					name = attr.Val
				case "value":
					value = attr.Val
				}
			}
			if name != "" {
				form.values.Set(name, value)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	if form.action == "" {
		t.Fatalf("HTML response contains no form action: %s", body)
	}
	return form
}

func resolveFormAction(t *testing.T, baseURL, action string) string {
	t.Helper()
	base, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := url.Parse(action)
	if err != nil {
		t.Fatal(err)
	}
	return base.ResolveReference(reference).String()
}

func generateIDPKeyPair(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "SAML flow test IdP"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

func TestSAMLBrowserLoginEndToEnd(t *testing.T) {
	t.Setenv("JWT_SECRET", "saml-flow-test-jwt-secret")
	t.Setenv("AUTH_REFRESH_COOKIE_SECURE", "false")
	t.Setenv("SSRF_WHITELIST", "127.0.0.1")
	t.Setenv("SSRF_WHITELIST_EXTRA", "")
	secutils.ResetSSRFWhitelistForTest()
	t.Cleanup(secutils.ResetSSRFWhitelistForTest)

	idpServer := httptest.NewUnstartedServer(nil)
	authServer := httptest.NewUnstartedServer(nil)
	idpBaseURL := "http://" + idpServer.Listener.Addr().String()
	authBaseURL := "http://" + authServer.Listener.Addr().String()
	frontendURL := "http://frontend.example.test/"
	t.Setenv("AUTH_ALLOWED_REDIRECT_URIS", frontendURL)

	acsURL, err := url.Parse(authBaseURL + "/api/v1/auth/saml/acs")
	if err != nil {
		t.Fatal(err)
	}
	metadataURL := *acsURL
	metadataURL.Path = "/api/v1/auth/saml/metadata"
	const spEntityID = "urn:rochekap:test:sp"
	sp := saml.ServiceProvider{EntityID: spEntityID, AcsURL: *acsURL, MetadataURL: metadataURL}

	store := &samlidp.MemoryStore{}
	password := "Admin123!"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("/users/admin", &samlidp.User{
		HashedPassword: hashedPassword,
		Email:          "admin@rochekap.local",
		CommonName:     "Mock Admin",
		GivenName:      "Mock",
		Surname:        "Admin",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("/services/test", &samlidp.Service{Name: "test", Metadata: *sp.Metadata()}); err != nil {
		t.Fatal(err)
	}
	idpURL, err := url.Parse(idpBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	idpKey, idpCert := generateIDPKeyPair(t)
	idp, err := samlidp.New(samlidp.Options{
		URL: *idpURL, Key: idpKey, Signer: idpKey, Certificate: idpCert, Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	idpServer.Config.Handler = idp
	idpServer.Start()
	defer idpServer.Close()

	userRepo := newSAMLFlowUserRepo()
	tokenRepo := newSAMLFlowTokenRepo()
	ssoRepo := &samlFlowSSORepo{users: userRepo}
	cfg := &config.Config{
		LocalAuth:    &config.LocalAuthConfig{PasswordLoginEnable: true},
		Registration: &config.RegistrationConfig{DevRoleSelection: true},
		SAMLAuth: &config.SAMLAuthConfig{
			Enable:               true,
			IdPMetadataURL:       idpBaseURL + "/metadata",
			SPEntityID:           spEntityID,
			ACSUrl:               acsURL.String(),
			ProviderDisplayName:  "Mock SAML",
			AutoProvision:        true,
			AllowEphemeralSPCert: true,
			DevSystemAdminEmails: []string{"admin@rochekap.local"},
		},
	}
	userService := service.NewUserService(cfg, userRepo, tokenRepo, ssoRepo, nil, nil, nil, nil, nil)
	authServer.Config.Handler = authserver.NewRouter(cfg, userService, nil, "test-internal-secret")
	authServer.Start()
	defer authServer.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	authURLResponse, err := client.Get(authBaseURL + "/api/v1/auth/saml/url?redirect_uri=" + url.QueryEscape(frontendURL))
	if err != nil {
		t.Fatal(err)
	}
	defer authURLResponse.Body.Close()
	if authURLResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(authURLResponse.Body)
		t.Fatalf("SAML URL status=%d body=%s", authURLResponse.StatusCode, body)
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			AuthorizationURL string `json:"authorization_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(authURLResponse.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || envelope.Data.AuthorizationURL == "" {
		t.Fatalf("invalid SAML URL response: %+v", envelope)
	}

	loginResponse, err := client.Get(envelope.Data.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	loginForm := parseHTMLForm(t, loginResponse)
	loginForm.values.Set("user", "admin")
	loginForm.values.Set("password", password)
	assertionResponse, err := client.PostForm(resolveFormAction(t, idpBaseURL, loginForm.action), loginForm.values)
	if err != nil {
		t.Fatal(err)
	}
	assertionForm := parseHTMLForm(t, assertionResponse)
	if assertionForm.values.Get("SAMLResponse") == "" || assertionForm.values.Get("RelayState") == "" {
		t.Fatalf("IdP response form is incomplete: %v", assertionForm.values)
	}

	acsRequest, err := http.NewRequest(
		http.MethodPost,
		resolveFormAction(t, authBaseURL, assertionForm.action),
		strings.NewReader(assertionForm.values.Encode()),
	)
	if err != nil {
		t.Fatal(err)
	}
	acsRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Browsers attach the IdP origin to a cross-origin SAML form POST. The
	// ACS endpoint must accept it even though regular API CORS remains limited
	// to the application frontend origin.
	acsRequest.Header.Set("Origin", idpBaseURL)
	acsResponse, err := client.Do(acsRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer acsResponse.Body.Close()
	if acsResponse.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(acsResponse.Body)
		t.Fatalf("ACS status=%d body=%s", acsResponse.StatusCode, body)
	}
	callbackURL, err := url.Parse(acsResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(callbackURL.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	encodedResult := fragment.Get("saml_result")
	if encodedResult == "" {
		t.Fatalf("ACS redirect contains no saml_result: %s", callbackURL.String())
	}
	callbackJSON, err := base64.RawURLEncoding.DecodeString(encodedResult)
	if err != nil {
		t.Fatal(err)
	}
	var callback struct {
		Success bool        `json:"success"`
		Token   string      `json:"token"`
		User    *types.User `json:"user"`
	}
	if err := json.Unmarshal(callbackJSON, &callback); err != nil {
		t.Fatal(err)
	}
	if !callback.Success || callback.Token == "" || callback.User == nil {
		t.Fatalf("invalid SAML callback payload: %s", callbackJSON)
	}
	if callback.User.Email != "admin@rochekap.local" || !callback.User.IsSystemAdmin {
		t.Fatalf("unexpected provisioned user: %+v", callback.User)
	}
	if ssoRepo.identity == nil || ssoRepo.identity.UserID != callback.User.ID {
		t.Fatalf("SSO identity was not linked to the provisioned user")
	}
	if len(tokenRepo.byText) != 2 {
		t.Fatalf("persisted token count=%d, want access + refresh", len(tokenRepo.byText))
	}
	authURL, err := url.Parse(authBaseURL + "/api/v1/auth/refresh")
	if err != nil {
		t.Fatal(err)
	}
	var refreshCookieFound, nonceCookieFound bool
	for _, cookie := range jar.Cookies(authURL) {
		switch cookie.Name {
		case "roche_kap_refresh_token":
			refreshCookieFound = cookie.Value != ""
		case "roche_kap_saml_nonce":
			nonceCookieFound = true
		}
	}
	if !refreshCookieFound {
		t.Fatal("ACS response did not persist the refresh-token cookie")
	}
	if nonceCookieFound {
		t.Fatal("ACS response did not clear the one-time SAML nonce cookie")
	}

	meRequest, err := http.NewRequest(http.MethodGet, authBaseURL+"/api/v1/auth/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	meRequest.Header.Set("Authorization", "Bearer "+callback.Token)
	meResponse, err := client.Do(meRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer meResponse.Body.Close()
	if meResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResponse.Body)
		t.Fatalf("/auth/me status=%d body=%s", meResponse.StatusCode, body)
	}
}
