package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenCookieIsHttpOnlyAndSameSiteLax(t *testing.T) {
	t.Setenv("AUTH_REFRESH_COOKIE_SECURE", "false")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login", nil)

	(&AuthHandler{}).setRefreshTokenCookie(ctx, "refresh-token")

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, refreshTokenCookieName, cookie.Name)
	assert.Equal(t, "refresh-token", cookie.Value)
	assert.Equal(t, refreshTokenCookiePath, cookie.Path)
	assert.Equal(t, refreshTokenCookieMaxAge, cookie.MaxAge)
	assert.True(t, cookie.HttpOnly)
	assert.False(t, cookie.Secure)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
}

func TestRefreshTokenCookieCanBeForcedSecure(t *testing.T) {
	t.Setenv("AUTH_REFRESH_COOKIE_SECURE", "true")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login", nil)

	(&AuthHandler{}).setRefreshTokenCookie(ctx, "refresh-token")

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure)
}

func TestClearRefreshTokenCookieUsesSameScope(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/logout", nil)

	(&AuthHandler{}).clearRefreshTokenCookie(ctx)

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, refreshTokenCookieName, cookies[0].Name)
	assert.Equal(t, refreshTokenCookiePath, cookies[0].Path)
	assert.Less(t, cookies[0].MaxAge, 0)
	assert.True(t, cookies[0].HttpOnly)
}

func TestSAMLNonceCookieAllowsCrossSitePostOverHTTPS(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/auth/saml/url", nil)
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	setSAMLNonceCookie(ctx, "nonce", samlNonceCookieMaxAge)

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, samlNonceCookieName, cookies[0].Name)
	assert.Equal(t, "nonce", cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)
	assert.True(t, cookies[0].Secure)
	assert.Equal(t, http.SameSiteNoneMode, cookies[0].SameSite)
}

func TestSAMLNonceCookieSupportsHTTPDevelopment(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/auth/saml/url", nil)

	setSAMLNonceCookie(ctx, "nonce", samlNonceCookieMaxAge)

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.False(t, cookies[0].Secure)
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
}

func TestClearSAMLNonceCookieUsesRequestSecurityScope(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/saml/acs", nil)
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	setSAMLNonceCookie(ctx, "", -1)

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, samlNonceCookieName, cookies[0].Name)
	assert.Less(t, cookies[0].MaxAge, 0)
	assert.True(t, cookies[0].Secure)
	assert.Equal(t, http.SameSiteNoneMode, cookies[0].SameSite)
}
