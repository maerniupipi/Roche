package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/handler/dto"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/response"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

const oidcNonceCookieName = "roche_kap_oidc_nonce"
const oidcNonceCookieMaxAge = 600
const refreshTokenCookieName = "roche_kap_refresh_token"
const refreshTokenCookieMaxAge = 7 * 24 * 60 * 60
const refreshTokenCookiePath = "/api/v1/auth"

// AuthHandler implements HTTP request handlers for user authentication
// Provides functionality for user registration, login, logout, and token management
// through the REST API endpoints
type AuthHandler struct {
	userService interfaces.UserService
	configInfo  *config.Config
}

// NewAuthHandler creates a new auth handler instance with the provided services
// Parameters:
//   - userService: An implementation of the UserService interface for business logic
//
// Returns a pointer to the newly created AuthHandler
func NewAuthHandler(configInfo *config.Config, userService interfaces.UserService) *AuthHandler {
	return &AuthHandler{
		configInfo:  configInfo,
		userService: userService,
	}
}

// Register godoc
// @Summary      注册邮箱账号
// @Description  创建本地邮箱账号；仅在开发角色选择开关启用时接受 role
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        request  body      types.RegisterRequest  true  "注册参数"
// @Success      201      {object}  types.RegisterResponse
// @Failure      400      {object}  errors.AppError
// @Failure      403      {object}  errors.AppError
// @Failure      429      {object}  errors.AppError
// @Router       /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	ctx := c.Request.Context()
	if h.configInfo == nil || h.configInfo.Registration == nil || !h.configInfo.Registration.Enable {
		c.Error(errors.NewForbiddenError("Email registration is disabled"))
		return
	}

	var req types.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid registration parameters").WithDetails(err.Error()))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Username == "" || req.Email == "" || req.Password == "" {
		c.Error(errors.NewValidationError("Username, email and password are required"))
		return
	}

	user, err := h.userService.Register(ctx, &req)
	if err != nil {
		logger.Errorf(ctx, "Failed to register user: %v", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	response.Created(c, user)
}

// GetRegistrationConfig godoc
// @Summary      获取邮箱注册配置
// @Description  返回注册入口、默认角色以及开发角色选择是否启用
// @Tags         认证
// @Produce      json
// @Success      200  {object}  types.RegistrationConfigResponse
// @Router       /auth/registration/config [get]
func (h *AuthHandler) GetRegistrationConfig(c *gin.Context) {
	cfgResp := &types.RegistrationConfigResponse{
		Success:     true,
		DefaultRole: types.RegistrationRoleViewer,
		Roles:       []types.RegistrationRole{},
	}
	if h.configInfo != nil && h.configInfo.LocalAuth != nil {
		cfgResp.PasswordLoginEnabled = h.configInfo.LocalAuth.PasswordLoginEnable
	}
	if h.configInfo != nil && h.configInfo.Registration != nil {
		cfg := h.configInfo.Registration
		cfgResp.Enabled = cfg.Enable
		cfgResp.RoleSelectionEnabled = cfg.Enable && cfg.DevRoleSelection
		cfgResp.DefaultRole = types.RegistrationRole(cfg.DefaultRole)
		if cfgResp.RoleSelectionEnabled {
			cfgResp.Roles = []types.RegistrationRole{
				types.RegistrationRoleViewer,
				types.RegistrationRoleSystemAdmin,
			}
		}
	}
	response.Success(c, cfgResp)
}

// Login godoc
// @Summary      用户登录
// @Description  使用邮箱和密码登录，返回访问令牌，并通过 HttpOnly Cookie 写入刷新令牌
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        request  body      types.LoginRequest  true  "登录参数"
// @Success      200      {object}  dto.AuthLoginResponse
// @Failure      401      {object}  errors.AppError  "认证失败"
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	if h.configInfo == nil || h.configInfo.LocalAuth == nil || !h.configInfo.LocalAuth.PasswordLoginEnable {
		c.Error(errors.NewForbiddenError("Email/password login is disabled"))
		return
	}

	logger.Info(ctx, "Start user login")

	var req types.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse login request parameters", err)
		appErr := errors.NewValidationError("Invalid login parameters").WithDetails(err.Error())
		c.Error(appErr)
		return
	}
	email := secutils.SanitizeForLog(req.Email)

	// Validate required fields
	if req.Email == "" || req.Password == "" {
		logger.Error(ctx, "Missing required login fields")
		appErr := errors.NewValidationError("Email and password are required")
		c.Error(appErr)
		return
	}

	// Call service to authenticate user
	loginResp, err := h.userService.Login(ctx, &req)
	if err != nil {
		logger.Errorf(ctx, "Failed to login user: %v", err)
		appErr := errors.NewUnauthorizedError("Login failed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	// Check if login was successful
	if !loginResp.Success {
		logger.Warnf(ctx, "Login failed: %s", loginResp.Message)
		response.Error(c, http.StatusUnauthorized, http.StatusUnauthorized, loginResp.Message)
		return
	}

	logger.Infof(ctx, "User logged in successfully, email: %s", email)
	h.setRefreshTokenCookie(c, loginResp.RefreshToken)
	response.Success(c, dto.NewAuthLoginResponse(loginResp))
}

// GetOIDCAuthorizationURL godoc
// @Summary      获取 OIDC 授权地址
// @Description  根据后端 OIDC 配置生成第三方登录跳转地址
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        redirect_uri  query     string  true  "OIDC 回调地址"
// @Success      200           {object}  types.OIDCAuthURLResponse
// @Failure      400           {object}  errors.AppError  "参数错误"
// @Failure      403           {object}  errors.AppError  "OIDC 未启用或配置不可用"
// @Router       /auth/oidc/url [get]
func (h *AuthHandler) GetOIDCAuthorizationURL(c *gin.Context) {
	ctx := c.Request.Context()
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	if redirectURI == "" {
		appErr := errors.NewValidationError("redirect_uri is required")
		c.Error(appErr)
		return
	}
	if err := secutils.ValidateAllowedRedirectURI(redirectURI, os.Getenv("AUTH_ALLOWED_REDIRECT_URIS")); err != nil {
		appErr := errors.NewValidationError("redirect_uri is not allowed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	resp, err := h.userService.GetOIDCAuthorizationURL(ctx, redirectURI)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate OIDC authorization URL: %v", err)
		appErr := errors.NewForbiddenError("OIDC authorization unavailable").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	// Bind the state nonce to this browser so an attacker cannot replay
	// their own authorization code into a victim's callback.
	if resp.Nonce != "" {
		secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(oidcNonceCookieName, resp.Nonce, oidcNonceCookieMaxAge, "/", "", secure, true)
	}

	response.Success(c, resp)
}

// GetOIDCConfig godoc
// @Summary      获取 OIDC 登录配置
// @Description  返回 OIDC 是否启用以及身份提供商显示名称，供前端决定是否显示企业登录入口
// @Tags         认证
// @Accept       json
// @Produce      json
// @Success      200  {object}  types.OIDCConfigResponse
// @Router       /auth/oidc/config [get]
func (h *AuthHandler) GetOIDCConfig(c *gin.Context) {
	providerDisplayName := ""
	enabled := false

	if h.configInfo != nil && h.configInfo.OIDCAuth != nil {
		enabled = h.configInfo.OIDCAuth.Enable
		providerDisplayName = strings.TrimSpace(h.configInfo.OIDCAuth.ProviderDisplayName)
	}

	response.Success(c, gin.H{
		"enabled":               enabled,
		"provider_display_name": providerDisplayName,
	})
}

// OIDCRedirectCallback godoc
// @Summary      OIDC 登录回调
// @Description  接收 OIDC Provider 回调，由后端完成授权码交换，然后重定向至前端登录页
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        code   query string false "OIDC 授权码"
// @Param        state  query string false "OIDC 状态参数"
// @Param        error  query string false "OIDC 错误码"
// @Success      302
// @Router       /auth/oidc/callback [get]
func (h *AuthHandler) OIDCRedirectCallback(c *gin.Context) {
	ctx := c.Request.Context()
	frontendRedirectURI := "/"

	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		redirectURL := frontendRedirectURI + "#oidc_error=" + urlQueryEscape(providerError)
		if description := strings.TrimSpace(c.Query("error_description")); description != "" {
			redirectURL += "&oidc_error_description=" + urlQueryEscape(description)
		}
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	state := strings.TrimSpace(c.Query("state"))
	decodedState, err := decodeOIDCState(state, c.Request)
	if err != nil {
		logger.Errorf(ctx, "Failed to decode OIDC state: %v", err)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("invalid_state"))
		return
	}
	frontendRedirectURI = oidcFrontendRedirectURI(decodedState.RedirectURI)
	// One-time use: clear the binding cookie as soon as it is checked.
	c.SetCookie(oidcNonceCookieName, "", -1, "/", "", false, true)

	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("missing_code"))
		return
	}

	resp, err := h.userService.LoginWithOIDC(
		ctx,
		code,
		strings.TrimSpace(decodedState.RedirectURI),
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to complete OIDC login via redirect callback: %v", err)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("login_failed")+"&oidc_error_description="+urlQueryEscape(err.Error()))
		return
	}
	if !resp.Success {
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("login_failed")+"&oidc_error_description="+urlQueryEscape(resp.Message))
		return
	}
	h.setRefreshTokenCookie(c, resp.RefreshToken)

	payload, err := encodeOIDCCallbackPayload(resp)
	if err != nil {
		logger.Errorf(ctx, "Failed to encode OIDC callback payload: %v", err)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("payload_encode_failed"))
		return
	}

	c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_result="+urlQueryEscape(payload))
}

func oidcFrontendRedirectURI(callbackURI string) string {
	parsed, err := url.Parse(strings.TrimSpace(callbackURI))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "/"
	}
	if parsed.Path == "/api/v1/auth/oidc/callback" {
		parsed.Path = "/"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func encodeOIDCCallbackPayload(resp *types.OIDCCallbackResponse) (string, error) {
	payload, err := json.Marshal(dto.NewAuthOIDCCallbackResponse(resp))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

type oidcStatePayload struct {
	Nonce       string
	RedirectURI string
}

func decodeOIDCState(raw string, req *http.Request) (*oidcStatePayload, error) {
	payload, err := secutils.VerifyOIDCState(raw)
	if err != nil {
		return nil, err
	}
	cookieNonce, err := req.Cookie(oidcNonceCookieName)
	if err != nil || cookieNonce == nil || strings.TrimSpace(cookieNonce.Value) == "" {
		return nil, errors.NewValidationError("oidc nonce cookie missing")
	}
	if cookieNonce.Value != payload.Nonce {
		return nil, errors.NewValidationError("oidc nonce mismatch")
	}
	return &oidcStatePayload{
		Nonce:       payload.Nonce,
		RedirectURI: strings.TrimSpace(payload.RedirectURI),
	}, nil
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"#", "%23",
		"&", "%26",
		"+", "%2B",
		"=", "%3D",
		"?", "%3F",
	)
	return replacer.Replace(value)
}

const samlNonceCookieName = "roche_kap_saml_nonce"
const samlNonceCookieMaxAge = 600

// GetSAMLConfig godoc
// @Summary      获取 SAML 登录配置
// @Description  返回 SAML 是否启用以及身份提供商显示名称，供前端决定是否显示企业登录入口
// @Tags         认证
// @Accept       json
// @Produce      json
// @Success      200  {object}  types.SAMLConfigResponse
// @Router       /auth/saml/config [get]
func (h *AuthHandler) GetSAMLConfig(c *gin.Context) {
	providerDisplayName := ""
	enabled := false

	if h.configInfo != nil && h.configInfo.SAMLAuth != nil {
		enabled = h.configInfo.SAMLAuth.Enable
		providerDisplayName = strings.TrimSpace(h.configInfo.SAMLAuth.ProviderDisplayName)
	}

	response.Success(c, gin.H{
		"enabled":               enabled,
		"provider_display_name": providerDisplayName,
	})
}

// GetSAMLAuthorizationURL godoc
// @Summary      获取 SAML 授权地址
// @Description  根据后端 SAML 配置生成 SP 发起的单点登录跳转地址
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        redirect_uri  query     string  true  "登录成功后前端回调地址"
// @Success      200           {object}  types.SAMLAuthURLResponse
// @Failure      400           {object}  errors.AppError  "参数错误"
// @Failure      403           {object}  errors.AppError  "SAML 未启用或配置不可用"
// @Router       /auth/saml/url [get]
func (h *AuthHandler) GetSAMLAuthorizationURL(c *gin.Context) {
	ctx := c.Request.Context()
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	if redirectURI == "" {
		appErr := errors.NewValidationError("redirect_uri is required")
		c.Error(appErr)
		return
	}
	if err := secutils.ValidateAllowedRedirectURI(redirectURI, os.Getenv("AUTH_ALLOWED_REDIRECT_URIS")); err != nil {
		appErr := errors.NewValidationError("redirect_uri is not allowed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	resp, err := h.userService.GetSAMLAuthorizationURL(ctx, redirectURI)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate SAML authorization URL: %v", err)
		appErr := errors.NewForbiddenError("SAML authorization unavailable").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	// Bind the state nonce to this browser so an attacker cannot replay
	// their own assertion into a victim's ACS callback.
	if resp.Nonce != "" {
		setSAMLNonceCookie(c, resp.Nonce, samlNonceCookieMaxAge)
	}

	response.Success(c, resp)
}

// SAMLAcs godoc
// @Summary      SAML 断言消费回调
// @Description  接收 IdP 回调的 SAMLResponse（HTTP-Redirect 或 HTTP-POST 绑定），
// @Description  由后端校验断言、自动开通用户并重定向至前端登录页
// @Tags         认证
// @Accept       application/x-www-form-urlencoded
// @Produce      json
// @Param        SAMLResponse  query     string  false  "SAML 响应（Redirect 绑定）"
// @Param        RelayState    query     string  false  "SAML 中继状态"
// @Success      302
// @Router       /auth/saml/acs [get]
func (h *AuthHandler) SAMLAcs(c *gin.Context) {
	ctx := c.Request.Context()
	frontendRedirectURI := "/"

	relayState := strings.TrimSpace(c.Query("relay_state"))
	if relayState == "" {
		relayState = strings.TrimSpace(c.Query("RelayState"))
	}
	if relayState == "" {
		relayState = strings.TrimSpace(c.PostForm("RelayState"))
	}

	var decodedState *samlStatePayload
	var err error
	if relayState == "" && h.configInfo != nil && h.configInfo.SAMLAuth != nil && h.configInfo.SAMLAuth.AllowIDPInitiated {
		decodedState = &samlStatePayload{RedirectURI: frontendRedirectURI}
	} else {
		decodedState, err = decodeSAMLState(relayState, c.Request)
		if err != nil {
			logger.Errorf(ctx, "Failed to decode SAML relay state: %v", err)
			c.Redirect(http.StatusFound, frontendRedirectURI+"#saml_error="+urlQueryEscape("invalid_state"))
			return
		}
		// One-time use: clear the binding cookie as soon as it is checked.
		setSAMLNonceCookie(c, "", -1)
	}
	frontendRedirectURI = strings.TrimSpace(decodedState.RedirectURI)

	resp, err := h.userService.LoginWithSAML(
		ctx,
		c.Request,
		strings.TrimSpace(decodedState.RedirectURI),
		strings.TrimSpace(decodedState.RequestID),
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to complete SAML login: %v", err)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#saml_error="+urlQueryEscape("login_failed")+"&saml_error_description="+urlQueryEscape(err.Error()))
		return
	}
	if !resp.Success {
		c.Redirect(http.StatusFound, frontendRedirectURI+"#saml_error="+urlQueryEscape("login_failed")+"&saml_error_description="+urlQueryEscape(resp.Message))
		return
	}
	h.setRefreshTokenCookie(c, resp.RefreshToken)

	payload, err := encodeSAMLCallbackPayload(resp)
	if err != nil {
		logger.Errorf(ctx, "Failed to encode SAML callback payload: %v", err)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#saml_error="+urlQueryEscape("payload_encode_failed"))
		return
	}

	c.Redirect(http.StatusFound, frontendRedirectURI+"#saml_result="+urlQueryEscape(payload))
}

// GetSAMLMetadata godoc
// @Summary      获取 SP 元数据
// @Description  返回本服务的 SAML Service Provider 元数据 XML，供 IdP 侧注册
// @Tags         认证
// @Accept       xml
// @Produce      xml
// @Success      200  {string}  string  "SAML 元数据 XML"
// @Failure      403  {object}  errors.AppError  "SAML 未启用或配置不可用"
// @Router       /auth/saml/metadata [get]
func (h *AuthHandler) GetSAMLMetadata(c *gin.Context) {
	ctx := c.Request.Context()
	metadata, err := h.userService.GetSAMLMetadata(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate SAML metadata: %v", err)
		appErr := errors.NewForbiddenError("SAML metadata unavailable").WithDetails(err.Error())
		c.Error(appErr)
		return
	}
	c.Data(http.StatusOK, "application/samlmetadata+xml", metadata)
}

func encodeSAMLCallbackPayload(resp *types.SAMLCallbackResponse) (string, error) {
	payload, err := json.Marshal(dto.NewAuthSAMLCallbackResponse(resp))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

type samlStatePayload struct {
	Nonce       string
	RedirectURI string
	RequestID   string
}

func decodeSAMLState(raw string, req *http.Request) (*samlStatePayload, error) {
	payload, err := secutils.VerifySSOState(raw)
	if err != nil {
		return nil, err
	}
	cookieNonce, err := req.Cookie(samlNonceCookieName)
	if err != nil || cookieNonce == nil || strings.TrimSpace(cookieNonce.Value) == "" {
		return nil, errors.NewValidationError("saml nonce cookie missing")
	}
	if cookieNonce.Value != payload.Nonce {
		return nil, errors.NewValidationError("saml nonce mismatch")
	}
	return &samlStatePayload{
		Nonce:       payload.Nonce,
		RedirectURI: strings.TrimSpace(payload.RedirectURI),
		RequestID:   strings.TrimSpace(payload.RequestID),
	}, nil
}

// Logout godoc
// @Summary      用户登出
// @Description  撤销当前用户的有效令牌并退出登录
// @Tags         认证
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "登出成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()
	defer h.clearRefreshTokenCookie(c)

	logger.Info(ctx, "Start user logout")

	// Extract token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		logger.Error(ctx, "Missing Authorization header")
		appErr := errors.NewValidationError("Authorization header is required")
		c.Error(appErr)
		return
	}

	// Parse Bearer token
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		logger.Error(ctx, "Invalid Authorization header format")
		appErr := errors.NewValidationError("Invalid Authorization header format")
		c.Error(appErr)
		return
	}

	token := tokenParts[1]

	// Revoke every outstanding session for this user so refresh tokens
	// cannot keep working after logout.
	err := h.userService.Logout(ctx, token)
	if err != nil {
		logger.Errorf(ctx, "Failed to revoke token: %v", err)
		appErr := errors.NewInternalServerError("Logout failed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	logger.Info(ctx, "User logged out successfully")
	response.SuccessWithMsg(c, "Logout successful", nil)
}

// RefreshToken godoc
// @Summary      刷新令牌
// @Description  使用 HttpOnly Cookie 中的刷新令牌轮换会话；响应体只返回新的访问令牌
// @Tags         认证
// @Accept       json
// @Produce      json
// @Success      200      {object}  map[string]interface{}       "刷新成功"
// @Failure      401      {object}  errors.AppError              "刷新令牌无效或已过期"
// @Router       /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start token refresh")

	refreshToken, err := c.Cookie(refreshTokenCookieName)
	if err != nil || strings.TrimSpace(refreshToken) == "" {
		logger.Warn(ctx, "Refresh token cookie is missing")
		h.clearRefreshTokenCookie(c)
		appErr := errors.NewUnauthorizedError("Refresh token cookie is missing")
		c.Error(appErr)
		return
	}

	// Call service to refresh token
	accessToken, newRefreshToken, err := h.userService.RefreshToken(ctx, refreshToken)
	if err != nil {
		logger.Errorf(ctx, "Failed to refresh token: %v", err)
		h.clearRefreshTokenCookie(c)
		appErr := errors.NewUnauthorizedError("Token refresh failed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	logger.Info(ctx, "Token refreshed successfully")
	h.setRefreshTokenCookie(c, newRefreshToken)
	response.Success(c, gin.H{
		"access_token": accessToken,
	})
}

func (h *AuthHandler) setRefreshTokenCookie(c *gin.Context, token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    token,
		Path:     refreshTokenCookiePath,
		MaxAge:   refreshTokenCookieMaxAge,
		HttpOnly: true,
		Secure:   refreshCookieSecure(c.Request),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearRefreshTokenCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     refreshTokenCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   refreshCookieSecure(c.Request),
		SameSite: http.SameSiteLaxMode,
	})
}

func isHTTPSRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	return req.TLS != nil || strings.EqualFold(strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")), "https")
}

func refreshCookieSecure(req *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_REFRESH_COOKIE_SECURE")), "true") || isHTTPSRequest(req)
}

func setSAMLNonceCookie(c *gin.Context, value string, maxAge int) {
	secure := isHTTPSRequest(c.Request)
	sameSite := http.SameSiteLaxMode
	if secure {
		// SAML HTTP-POST responses normally arrive from a different site.
		// Browsers only include this nonce cookie when it is SameSite=None.
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     samlNonceCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

// GetCurrentUser godoc
// @Summary      获取当前用户信息
// @Description  获取当前登录用户的身份、系统管理员标记和知识域管理员标记
// @Tags         认证
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "当前用户信息"
// @Failure      401  {object}  errors.AppError         "未认证"
// @Security     Bearer
// @Router       /auth/me [get]
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	ctx := c.Request.Context()
	user, err := h.userService.GetCurrentUser(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to get current user: %v", err)
		c.Error(errors.NewUnauthorizedError("Failed to get user information").WithDetails(err.Error()))
		return
	}
	response.Success(c, gin.H{"user": user.ToUserInfo()})
}

// updateMyPreferencesRequest is the body for PUT /auth/me/preferences.
// Fields are pointers so the handler can distinguish "key not present"
// (preserve existing value) from "explicit false". See
// types.UserPreferences for the persistence-layer counterpart.
type updateMyPreferencesRequest struct {
	EnableMemory *bool `json:"enable_memory"`
}

// UpdateMyPreferences godoc
// @Summary      更新当前用户偏好
// @Description  按补丁语义合并用户偏好，只覆盖请求体中出现的字段
// @Description  偏好保存在 users.preferences，可跨浏览器同步
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        request  body      updateMyPreferencesRequest  true  "Preferences patch"
// @Success      200      {object}  map[string]interface{}      "更新成功"
// @Failure      400      {object}  errors.AppError             "请求参数错误"
// @Failure      401      {object}  errors.AppError             "未认证"
// @Security     Bearer
// @Router       /auth/me/preferences [put]
func (h *AuthHandler) UpdateMyPreferences(c *gin.Context) {
	ctx := c.Request.Context()

	user, err := h.userService.GetCurrentUser(ctx)
	if err != nil {
		appErr := errors.NewUnauthorizedError("Failed to get user information").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	var req updateMyPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.NewValidationError("Invalid preferences request").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	patch := types.UserPreferences{EnableMemory: req.EnableMemory}
	prefs, err := h.userService.UpdateUserPreferences(ctx, user.ID, patch)
	if err != nil {
		logger.Errorf(ctx, "Failed to update preferences for user %s: %v", user.Email, err)
		appErr := errors.NewBadRequestError("Failed to update preferences").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	response.Success(c, prefs)
}

// GetConfidentialityAck godoc
// @Summary      查询当前用户保密确认申明状态
// @Description  返回当前登录用户是否已确认保密申明。acknowledged=false 时客户端应弹窗，确认后调用 POST /auth/me/confidentiality-ack。
// @Description  状态按用户持久化，一次确认后退出再登录不会再次弹出。
// @Tags         认证
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "acknowledged 状态及确认时间"
// @Failure      401  {object}  errors.AppError         "未认证"
// @Security     Bearer
// @Router       /auth/me/confidentiality-ack [get]
func (h *AuthHandler) GetConfidentialityAck(c *gin.Context) {
	ctx := c.Request.Context()
	user, err := h.userService.GetCurrentUser(ctx)
	if err != nil {
		c.Error(errors.NewUnauthorizedError("Failed to get user information").WithDetails(err.Error()))
		return
	}
	acknowledged, acknowledgedAt, err := h.userService.GetConfidentialityAck(ctx, user.ID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get confidentiality ack for user %s: %v", user.ID, err)
		c.Error(errors.NewInternalServerError("Failed to get confidentiality ack").WithDetails(err.Error()))
		return
	}
	response.Success(c, gin.H{
		"acknowledged":     acknowledged,
		"acknowledged_at":  acknowledgedAt,
	})
}

// AcknowledgeConfidentiality godoc
// @Summary      确认保密申明
// @Description  标记当前用户已确认保密申明，写入确认时间戳。幂等：再次调用只刷新时间戳，不会清除状态。
// @Tags         认证
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "确认成功，返回 acknowledged_at"
// @Failure      401  {object}  errors.AppError         "未认证"
// @Security     Bearer
// @Router       /auth/me/confidentiality-ack [post]
func (h *AuthHandler) AcknowledgeConfidentiality(c *gin.Context) {
	ctx := c.Request.Context()
	user, err := h.userService.GetCurrentUser(ctx)
	if err != nil {
		c.Error(errors.NewUnauthorizedError("Failed to get user information").WithDetails(err.Error()))
		return
	}
	acknowledgedAt, err := h.userService.AcknowledgeConfidentiality(ctx, user.ID)
	if err != nil {
		logger.Errorf(ctx, "Failed to acknowledge confidentiality for user %s: %v", user.ID, err)
		c.Error(errors.NewInternalServerError("Failed to acknowledge confidentiality").WithDetails(err.Error()))
		return
	}
	response.Success(c, gin.H{
		"acknowledged":     true,
		"acknowledged_at":  acknowledgedAt,
	})
}

// ChangePassword godoc
// @Summary      修改密码
// @Description  修改当前登录用户的本地账号密码
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        request  body      object{old_password=string,new_password=string}  true  "旧密码和新密码"
// @Success      200      {object}  map[string]interface{}                           "修改成功"
// @Failure      400      {object}  errors.AppError                                  "请求参数错误"
// @Security     Bearer
// @Router       /auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	if h.configInfo == nil || h.configInfo.LocalAuth == nil || !h.configInfo.LocalAuth.PasswordLoginEnable {
		c.Error(errors.NewForbiddenError("Email/password login is disabled"))
		return
	}
	ctx := c.Request.Context()

	logger.Info(ctx, "Start password change")

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse password change request", err)
		appErr := errors.NewValidationError("Invalid password change request").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	// Get current user
	user, err := h.userService.GetCurrentUser(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to get current user: %v", err)
		appErr := errors.NewUnauthorizedError("Failed to get user information").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	// Change password
	err = h.userService.ChangePassword(ctx, user.ID, req.OldPassword, req.NewPassword)
	if err != nil {
		logger.Errorf(ctx, "Failed to change password: %v", err)
		appErr := errors.NewBadRequestError("Password change failed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	logger.Infof(ctx, "Password changed successfully for user: %s", user.Email)
	response.SuccessWithMsg(c, "Password changed successfully", nil)
}

// AutoSetup godoc
// @Summary      Initialize a lite-edition administrator session
// @Description  Development bootstrap available only in lite edition. It is forbidden in the standard server edition.
// @Tags         Authentication
// @Produce      json
// @Success      200 {object} dto.AuthLoginResponse
// @Failure      403 {object} errors.AppError
// @Router       /auth/auto-setup [post]
func (h *AuthHandler) AutoSetup(c *gin.Context) {
	ctx := c.Request.Context()
	if Edition != "lite" {
		c.Error(errors.NewForbiddenError("auto-setup is only available in lite edition"))
		return
	}

	const defaultEmail = "admin@rochekap.local"
	user, _ := h.userService.GetUserByEmail(ctx, defaultEmail)
	if user == nil {
		randomBytes := make([]byte, 24)
		if _, err := rand.Read(randomBytes); err != nil {
			c.Error(errors.NewInternalServerError("auto-setup failed: unable to generate credentials"))
			return
		}
		_, err := h.userService.Register(ctx, &types.RegisterRequest{
			Username:              fmt.Sprintf("user_%s", base64.RawURLEncoding.EncodeToString(randomBytes[:6])),
			Email:                 defaultEmail,
			Password:              base64.RawURLEncoding.EncodeToString(randomBytes),
			Role:                  types.RegistrationRoleSystemAdmin,
			TrustedRoleAssignment: true,
		})
		if err != nil {
			c.Error(errors.NewInternalServerError("auto-setup failed").WithDetails(err.Error()))
			return
		}
		user, _ = h.userService.GetUserByEmail(ctx, defaultEmail)
	}
	if user == nil {
		c.Error(errors.NewInternalServerError("auto-setup failed: user not found"))
		return
	}

	accessToken, refreshToken, err := h.userService.GenerateTokens(ctx, user)
	if err != nil {
		c.Error(errors.NewInternalServerError("auto-setup failed").WithDetails(err.Error()))
		return
	}
	h.setRefreshTokenCookie(c, refreshToken)
	response.Success(c, dto.NewAuthLoginResponse(&types.LoginResponse{
		Success: true, Message: "Auto-setup successful", User: user,
		Token: accessToken, RefreshToken: refreshToken,
	}))
}

// ValidateToken godoc
// @Summary      验证访问令牌
// @Description  验证当前访问令牌是否有效并返回用户信息
// @Tags         认证
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "令牌有效"
// @Failure      401  {object}  errors.AppError         "令牌无效或已过期"
// @Security     Bearer
// @Router       /auth/validate [get]
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start token validation")

	// Extract token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		logger.Error(ctx, "Missing Authorization header")
		appErr := errors.NewValidationError("Authorization header is required")
		c.Error(appErr)
		return
	}

	// Parse Bearer token
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		logger.Error(ctx, "Invalid Authorization header format")
		appErr := errors.NewValidationError("Invalid Authorization header format")
		c.Error(appErr)
		return
	}

	token := tokenParts[1]

	// Validate token
	user, err := h.userService.ValidateToken(ctx, token)
	if err != nil {
		logger.Errorf(ctx, "Failed to validate token: %v", err)
		appErr := errors.NewUnauthorizedError("Token validation failed").WithDetails(err.Error())
		c.Error(appErr)
		return
	}

	logger.Infof(ctx, "Token validated successfully for user: %s", user.Email)
	response.Success(c, gin.H{"user": user.ToUserInfo()})
}
