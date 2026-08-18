// Command mock-saml-idp runs a development-only SAML 2.0 identity provider.
// It exercises the same SP-initiated login flow used with PingIdentity while
// keeping local and server-development environments self-contained.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlidp"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultListenAddress        = "0.0.0.0:8091"
	defaultPublicURL            = "http://127.0.0.1:8091"
	defaultUsername             = "admin"
	defaultPassword             = "Admin123!"
	defaultEmail                = "admin@rochekap.local"
	defaultDeveloperCount       = 100
	defaultDeveloperPassword    = "Dev12345!"
	defaultDeveloperEmailDomain = "rochekap.local"
)

type mockUserConfig struct {
	username   string
	password   string
	email      string
	commonName string
	givenName  string
	surname    string
}

type serviceProviderConfig struct {
	name     string
	entityID string
	acsURL   string
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		response, err := http.Get("http://127.0.0.1:8091/healthz")
		if err != nil || response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}

	publicURL, err := url.Parse(strings.TrimRight(env("MOCK_SAML_PUBLIC_URL", defaultPublicURL), "/"))
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
		log.Fatalf("invalid MOCK_SAML_PUBLIC_URL: %q", env("MOCK_SAML_PUBLIC_URL", defaultPublicURL))
	}

	key, cert, err := generateSigningKeyPair()
	if err != nil {
		log.Fatalf("generate mock IdP signing key: %v", err)
	}

	store := &samlidp.MemoryStore{}
	users, err := configuredUsers()
	if err != nil {
		log.Fatalf("configure mock SAML users: %v", err)
	}
	for _, user := range users {
		if err := addMockUser(store, user); err != nil {
			log.Fatalf("create mock SAML user %s: %v", user.username, err)
		}
	}

	for _, spConfig := range configuredServiceProviders() {
		if err := addServiceProvider(store, spConfig); err != nil {
			log.Fatalf("register %s service provider: %v", spConfig.name, err)
		}
	}

	idp, err := samlidp.New(samlidp.Options{
		URL:         *publicURL,
		Key:         key,
		Signer:      key,
		Certificate: cert,
		Store:       store,
	})
	if err != nil {
		log.Fatalf("create mock SAML IdP: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", idp)

	listenAddress := env("MOCK_SAML_LISTEN_ADDRESS", defaultListenAddress)
	log.Printf("Mock SAML IdP listening on %s (public URL %s)", listenAddress, publicURL.String())
	log.Printf("Primary development account: %s / %s (%s)", users[0].username, users[0].password, users[0].email)
	if len(users) > 1 {
		log.Printf("Additional development accounts: developer001..developer%03d / %s", len(users)-1, users[1].password)
	}
	if err := http.ListenAndServe(listenAddress, mux); err != nil {
		log.Fatal(err)
	}
}

func configuredUsers() ([]mockUserConfig, error) {
	count, err := strconv.Atoi(env("MOCK_SAML_DEVELOPER_COUNT", strconv.Itoa(defaultDeveloperCount)))
	if err != nil || count < 0 || count > 500 {
		return nil, fmt.Errorf("MOCK_SAML_DEVELOPER_COUNT must be an integer between 0 and 500")
	}

	users := []mockUserConfig{{
		username:   env("MOCK_SAML_USERNAME", defaultUsername),
		password:   env("MOCK_SAML_PASSWORD", defaultPassword),
		email:      env("MOCK_SAML_EMAIL", defaultEmail),
		commonName: env("MOCK_SAML_DISPLAY_NAME", "Mock Development User"),
		givenName:  "Mock",
		surname:    "User",
	}}
	developerPassword := env("MOCK_SAML_DEVELOPER_PASSWORD", defaultDeveloperPassword)
	emailDomain := env("MOCK_SAML_DEVELOPER_EMAIL_DOMAIN", defaultDeveloperEmailDomain)
	for i := 1; i <= count; i++ {
		username := fmt.Sprintf("developer%03d", i)
		users = append(users, mockUserConfig{
			username:   username,
			password:   developerPassword,
			email:      fmt.Sprintf("%s@%s", username, emailDomain),
			commonName: fmt.Sprintf("Mock Developer %03d", i),
			givenName:  "Developer",
			surname:    fmt.Sprintf("%03d", i),
		})
	}

	usernames := make(map[string]struct{}, len(users))
	emails := make(map[string]struct{}, len(users))
	for _, user := range users {
		if user.username == "" || user.password == "" || user.email == "" {
			return nil, fmt.Errorf("mock SAML usernames, passwords and emails must not be empty")
		}
		if _, exists := usernames[user.username]; exists {
			return nil, fmt.Errorf("duplicate mock SAML username %q", user.username)
		}
		if _, exists := emails[strings.ToLower(user.email)]; exists {
			return nil, fmt.Errorf("duplicate mock SAML email %q", user.email)
		}
		usernames[user.username] = struct{}{}
		emails[strings.ToLower(user.email)] = struct{}{}
	}
	return users, nil
}

func addMockUser(store samlidp.Store, cfg mockUserConfig) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash mock password: %w", err)
	}
	return store.Put("/users/"+cfg.username, &samlidp.User{
		HashedPassword: hashedPassword,
		Email:          cfg.email,
		CommonName:     cfg.commonName,
		GivenName:      cfg.givenName,
		Surname:        cfg.surname,
		Groups:         []string{"developers"},
	})
}

func configuredServiceProviders() []serviceProviderConfig {
	configs := []serviceProviderConfig{
		{
			name:     "local",
			entityID: env("MOCK_SAML_LOCAL_SP_ENTITY_ID", "urn:rochekap:local:sp"),
			acsURL:   env("MOCK_SAML_LOCAL_ACS_URL", "http://127.0.0.1:8088/api/v1/auth/saml/acs"),
		},
		{
			name:     "internal",
			entityID: strings.TrimSpace(os.Getenv("MOCK_SAML_INTERNAL_SP_ENTITY_ID")),
			acsURL:   strings.TrimSpace(os.Getenv("MOCK_SAML_INTERNAL_ACS_URL")),
		},
		{
			name:     "external",
			entityID: strings.TrimSpace(os.Getenv("MOCK_SAML_EXTERNAL_SP_ENTITY_ID")),
			acsURL:   strings.TrimSpace(os.Getenv("MOCK_SAML_EXTERNAL_ACS_URL")),
		},
	}

	result := make([]serviceProviderConfig, 0, len(configs))
	seen := make(map[string]struct{}, len(configs))
	for _, cfg := range configs {
		if cfg.entityID == "" || cfg.acsURL == "" {
			continue
		}
		if _, exists := seen[cfg.entityID]; exists {
			continue
		}
		seen[cfg.entityID] = struct{}{}
		result = append(result, cfg)
	}
	return result
}

func addServiceProvider(store samlidp.Store, cfg serviceProviderConfig) error {
	acsURL, err := url.Parse(cfg.acsURL)
	if err != nil || acsURL.Scheme == "" || acsURL.Host == "" {
		return fmt.Errorf("invalid ACS URL %q", cfg.acsURL)
	}
	metadataURL := *acsURL
	metadataURL.Path = strings.TrimSuffix(metadataURL.Path, "/acs") + "/metadata"
	sp := &saml.ServiceProvider{
		EntityID:    cfg.entityID,
		AcsURL:      *acsURL,
		MetadataURL: metadataURL,
	}
	return store.Put("/services/"+cfg.name, &samlidp.Service{
		Name:     cfg.name,
		Metadata: *sp.Metadata(),
	})
}

func generateSigningKeyPair() (*rsa.PrivateKey, *x509.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "RocheKAP Development Mock SAML IdP"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return key, cert, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
