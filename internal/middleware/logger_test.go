package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	appLogger "roche.local/knowledge-agent-platform/internal/logger"
)

func TestSanitizeBody(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "camelCase apiKey",
			in:   `{"modelName":"gpt-5.2","apiKey":"sk-secret-123","provider":"azure_openai"}`,
			want: `{"modelName":"gpt-5.2","apiKey":"***","provider":"azure_openai"}`,
		},
		{
			name: "snake_case api_key",
			in:   `{"api_key":"sk-secret-123"}`,
			want: `{"api_key":"***"}`,
		},
		{
			name: "PascalCase APIKey",
			in:   `{"APIKey":"sk-secret-123"}`,
			want: `{"APIKey":"***"}`,
		},
		{
			name: "secretKey camelCase",
			in:   `{"secretKey":"abc","accessKeyId":"id"}`,
			want: `{"secretKey":"***","accessKeyId":"id"}`,
		},
		{
			name: "refreshToken / accessToken camelCase",
			in:   `{"refreshToken":"rt","accessToken":"at"}`,
			want: `{"refreshToken":"***","accessToken":"***"}`,
		},
		{
			name: "password and token preserved as masked",
			in:   `{"password":"p","token":"t"}`,
			want: `{"password":"***","token":"***"}`,
		},
		{
			name: "SAML authorization material",
			in:   `{"authorization_url":"https://idp.example/sso?SAMLRequest=request&RelayState=state","relay_state":"state","nonce":"nonce"}`,
			want: `{"authorization_url":"***","relay_state":"***","nonce":"***"}`,
		},
		{
			name: "extra whitespace around colon",
			in:   `{"apiKey"  :   "leak"}`,
			want: `{"apiKey":"***"}`,
		},
		{
			name: "non sensitive fields untouched",
			in:   `{"baseUrl":"https://example.com","modelName":"gpt"}`,
			want: `{"baseUrl":"https://example.com","modelName":"gpt"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeBody(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeBody(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoggerRedactsSAMLAssertion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput bytes.Buffer
	appLogger.SetOutput(&logOutput)
	t.Cleanup(appLogger.ConfigureFromEnv)

	router := gin.New()
	router.Use(Logger())
	router.POST(samlACSPath, func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if got := string(body); !strings.Contains(got, "signed-assertion-secret") {
			t.Fatalf("ACS handler did not receive the original request body: %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	body := "SAMLResponse=signed-assertion-secret&RelayState=relay-secret"
	req := httptest.NewRequest(
		http.MethodPost,
		samlACSPath+"?SAMLResponse=query-secret&RelayState=query-relay-secret",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: got %d, want %d", recorder.Code, http.StatusNoContent)
	}
	logged := logOutput.String()
	if !strings.Contains(logged, "SAML assertion redacted") {
		t.Fatalf("log does not contain the SAML redaction marker: %s", logged)
	}
	for _, secret := range []string{
		"signed-assertion-secret",
		"relay-secret",
		"query-secret",
		"query-relay-secret",
	} {
		if strings.Contains(logged, secret) {
			t.Fatalf("SAML secret %q leaked into request log: %s", secret, logged)
		}
	}
}
