package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	apprepo "roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

var (
	jwtSecretOnce sync.Once
	jwtSecret     string
	// emailHintRegex matches any input that contains '@' or '.com'
	// (case-insensitive). The admin "create user" flow uses it to decide that
	// such an input is an email address (stored in users.email); every other
	// input — including alphanumeric employee ids such as E10002 — is stored
	// as users.employee_id. Go's regexp (RE2) has no lookaround, so "does NOT
	// contain X" is expressed as the negation of this match instead of a
	// negative lookahead.
	emailHintRegex = regexp.MustCompile(`(?i)(@|\.com)`)
)

// getJwtSecret retrieves the JWT secret from the environment, falling back to a securely generated random secret.
func getJwtSecret() string {
	jwtSecretOnce.Do(func() {
		if envSecret := strings.TrimSpace(os.Getenv("JWT_SECRET")); envSecret != "" {
			jwtSecret = envSecret
			return
		}

		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			panic(fmt.Sprintf("failed to generate JWT secret: %v", err))
		}
		jwtSecret = base64.StdEncoding.EncodeToString(randomBytes)
	})

	return jwtSecret
}

// userService implements the UserService interface
type userService struct {
	userRepo     interfaces.UserRepository
	tokenRepo    interfaces.AuthTokenRepository
	ssoRepo      interfaces.SSOIdentityRepository
	domainAdmins interfaces.KnowledgeDomainAdminService
	kdRepo       interfaces.KnowledgeDomainRepository
	kbRepo       interfaces.KnowledgeBaseRepository
	eaRepo       interfaces.EnterpriseAccessRepository
	config       *config.Config

	// businessAudit records auth lifecycle events (login/logout) to
	// the audit_logs table.
	businessAudit *BusinessAuditRecorder

	// Lazily-initialised SAML service provider (see saml.go). Failed
	// initialisation attempts are not cached, so a temporarily unavailable IdP
	// does not disable SAML until this process is restarted.
	samlMgrMu sync.Mutex
	samlMgr   *samlSPManager
}

// NewUserService creates a new user service instance
func NewUserService(
	configInfo *config.Config,
	userRepo interfaces.UserRepository,
	tokenRepo interfaces.AuthTokenRepository,
	ssoRepo interfaces.SSOIdentityRepository,
	domainAdmins interfaces.KnowledgeDomainAdminService,
	kdRepo interfaces.KnowledgeDomainRepository,
	kbRepo interfaces.KnowledgeBaseRepository,
	eaRepo interfaces.EnterpriseAccessRepository,
	businessAudit *BusinessAuditRecorder,
) interfaces.UserService {
	return &userService{
		userRepo:      userRepo,
		tokenRepo:     tokenRepo,
		ssoRepo:       ssoRepo,
		domainAdmins:  domainAdmins,
		kdRepo:        kdRepo,
		kbRepo:        kbRepo,
		eaRepo:        eaRepo,
		config:        configInfo,
		businessAudit: businessAudit,
	}
}

// Register creates a new user account
func (s *userService) Register(ctx context.Context, req *types.RegisterRequest) (*types.User, error) {
	logger.Info(ctx, "Start user registration")

	if s.config == nil || s.config.Registration == nil || !s.config.Registration.Enable {
		return nil, errors.New("email registration is disabled")
	}

	// Validate input
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, errors.New("username, email and password are required")
	}
	systemAdmin, err := s.resolveRegistrationAccess(req.Role, req.TrustedRoleAssignment)
	if err != nil {
		return nil, err
	}

	// Check if user already exists
	existingUser, _ := s.userRepo.GetUserByEmail(ctx, req.Email)
	if existingUser != nil {
		return nil, errors.New("user with this email already exists")
	}

	existingUser, _ = s.userRepo.GetUserByUsername(ctx, req.Username)
	if existingUser != nil {
		return nil, errors.New("user with this username already exists")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf(ctx, "Failed to hash password: %v", err)
		return nil, errors.New("failed to process password")
	}

	now := time.Now()
	user := &types.User{
		ID:            uuid.New().String(),
		Username:      req.Username,
		Email:         req.Email,
		PasswordHash:  string(hashedPassword),
		IsSystemAdmin: systemAdmin,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		logger.Errorf(ctx, "Failed to create user: %v", err)
		return nil, errors.New("failed to create user")
	}

	logger.Info(ctx, "User registered successfully")
	if s.businessAudit != nil {
		s.businessAudit.RecordUserCreated(ctx, user.ID, user.Email, user.Username, "email_password")
	}
	return user, nil
}

func (s *userService) resolveRegistrationAccess(
	raw types.RegistrationRole,
	trusted bool,
) (bool, error) {
	cfg := s.config.Registration
	requested := types.RegistrationRole(strings.ToLower(strings.TrimSpace(string(raw))))
	if requested == "" {
		requested = types.RegistrationRole(cfg.DefaultRole)
	} else if !trusted && !cfg.DevRoleSelection {
		return false, errors.New("registration role selection is disabled")
	}

	switch requested {
	case types.RegistrationRoleViewer:
		return false, nil
	case types.RegistrationRoleSystemAdmin:
		return true, nil
	default:
		return false, errors.New("invalid registration role")
	}
}

// Login authenticates a user and returns tokens
func (s *userService) Login(ctx context.Context, req *types.LoginRequest) (*types.LoginResponse, error) {
	logger.Info(ctx, "Start user login")
	// Get user by email
	user, err := s.userRepo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user by email: %v", err)
		return &types.LoginResponse{
			Success: false,
			Message: "Invalid email or password",
		}, nil
	}
	if user == nil {
		logger.Warn(ctx, "User not found for email")
		s.recordLoginFailed(ctx, req.Email, "email_password", "user_not_found")
		return &types.LoginResponse{
			Success: false,
			Message: "Invalid email or password",
		}, nil
	}

	// Check if user is active
	if user.Status != types.UserStatusNormal {
		logger.Warn(ctx, "User account is disabled")
		s.recordLoginFailed(ctx, req.Email, "email_password", "account_disabled")
		return &types.LoginResponse{
			Success: false,
			Message: "Account is disabled",
		}, nil
	}

	// Check if user is banned
	if user.IsBanned() {
		logger.Warn(ctx, "User account is banned")
		s.recordLoginFailed(ctx, req.Email, "email_password", "account_banned")
		return &types.LoginResponse{
			Success: false,
			Message: "Account is banned",
		}, nil
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		logger.Warn(ctx, "Password verification failed")
		s.recordLoginFailed(ctx, req.Email, "email_password", "wrong_password")
		return &types.LoginResponse{
			Success: false,
			Message: "Invalid email or password",
		}, nil
	}
	logger.Info(ctx, "Password verification successful")
	s.enrichKnowledgeDomainAdminFlag(ctx, user)

	// Generate user-scoped platform tokens. Knowledge access is evaluated
	// from explicit grants at request time and is not encoded in the JWT.
	logger.Info(ctx, "Generating tokens")
	accessToken, refreshToken, err := s.GenerateTokens(ctx, user)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate tokens: %v", err)
		return &types.LoginResponse{
			Success: false,
			Message: "Login failed",
		}, nil
	}
	logger.Info(ctx, "Tokens generated successfully")

	logger.Info(ctx, "User logged in successfully")
	s.recordLoginSuccess(ctx, user, "email_password")
	return &types.LoginResponse{
		Success:      true,
		Message:      "Login successful",
		User:         user,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// GetOIDCAuthorizationURL builds the OIDC authorization URL.
func (s *userService) GetOIDCAuthorizationURL(
	ctx context.Context,
	redirectURI string,
) (*types.OIDCAuthURLResponse, error) {
	cfg, err := s.getOIDCConfig(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(redirectURI) == "" {
		return nil, errors.New("redirect_uri is required")
	}
	nonce, err := generateRandomString(24)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	statePayload := &secutils.OIDCStatePayload{
		Nonce:       nonce,
		RedirectURI: strings.TrimSpace(redirectURI),
	}
	state, err := secutils.SignOIDCState(statePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode OIDC state: %w", err)
	}

	query := url.Values{}
	query.Set("response_type", "code")
	query.Set("client_id", cfg.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", strings.Join(cfg.Scopes, " "))
	query.Set("state", state)

	authURL := cfg.AuthorizationEndpoint
	if strings.Contains(authURL, "?") {
		authURL += "&" + query.Encode()
	} else {
		authURL += "?" + query.Encode()
	}

	return &types.OIDCAuthURLResponse{
		Success:             true,
		ProviderDisplayName: cfg.ProviderDisplayName,
		AuthorizationURL:    authURL,
		State:               state,
		Nonce:               nonce,
	}, nil
}

// LoginWithOIDC exchanges code for tokens, loads user info, provisions user if needed, and returns local login tokens.
func (s *userService) LoginWithOIDC(
	ctx context.Context,
	code,
	redirectURI string,
) (*types.OIDCCallbackResponse, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("code is required")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return nil, errors.New("redirect_uri is required")
	}

	cfg, err := s.getOIDCConfig(ctx)
	if err != nil {
		return nil, err
	}
	tokenResp, err := s.exchangeOIDCCode(ctx, cfg, code, redirectURI)
	if err != nil {
		return nil, err
	}

	userInfo, err := s.resolveOIDCUserInfo(ctx, cfg, tokenResp)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(userInfo.Email) == "" && strings.TrimSpace(userInfo.EmployeeID) == "" {
		return nil, errors.New("OIDC provider did not return email or employee id")
	}

	if strings.TrimSpace(userInfo.Subject) == "" {
		return nil, errors.New("OIDC provider did not return a stable subject")
	}
	issuer := oidcIdentityIssuer(cfg)
	user, isNewUser, err := s.findOrProvisionOIDCUser(ctx, issuer, userInfo)
	if err != nil {
		return nil, err
	}

	if user.Status != types.UserStatusNormal {
		return &types.OIDCCallbackResponse{Success: false, Message: "Account is disabled"}, nil
	}
	if user.IsBanned() {
		return &types.OIDCCallbackResponse{Success: false, Message: "Account is banned"}, nil
	}
	if !isNewUser {
		now := time.Now()
		if err := s.ssoRepo.Upsert(ctx, &types.SSOIdentity{
			UserID: user.ID, Provider: "oidc", Issuer: issuer, Subject: userInfo.Subject,
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

	s.recordLoginSuccess(ctx, user, "oidc")
	return &types.OIDCCallbackResponse{
		Success:      true,
		Message:      "Login successful",
		User:         user,
		Token:        accessToken,
		RefreshToken: refreshToken,
		IsNewUser:    isNewUser,
	}, nil
}

// findOrProvisionOIDCUser locates an existing local account for an OIDC
// callback. It tries, in order: the persisted SSO identity (subject), the
// email claim, then the employee id claim. When a pre-created account is
// matched by email or employee id, missing fields are back-filled from the
// OIDC claims. If no account is found, a new one is provisioned when
// AutoProvision is enabled.
func (s *userService) findOrProvisionOIDCUser(
	ctx context.Context,
	issuer string,
	info *types.OIDCUserInfo,
) (*types.User, bool, error) {
	identity, err := s.ssoRepo.GetBySubject(ctx, "oidc", issuer, info.Subject)
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
			if _, err := s.backfillUserFromOIDC(ctx, user, info); err != nil {
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
			if _, err := s.backfillUserFromOIDC(ctx, user, info); err != nil {
				return nil, false, err
			}
			return user, false, nil
		}
	}

	user, err := s.provisionOIDCUser(ctx, info, issuer)
	if err != nil {
		return nil, false, err
	}
	return user, true, nil
}

// backfillUserFromOIDC populates empty profile fields on a pre-created local
// account from OIDC claims. This lets an admin create a user with only an
// employee id or email; the first SSO login completes the record.
func (s *userService) backfillUserFromOIDC(
	ctx context.Context,
	user *types.User,
	info *types.OIDCUserInfo,
) (bool, error) {
	changed := false
	if user.Email == "" && info.Email != "" {
		existing, _ := s.userRepo.GetUserByEmail(ctx, info.Email)
		if existing != nil && existing.ID != user.ID {
			return false, errors.New("OIDC email is already bound to another user")
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
		return false, fmt.Errorf("failed to back-fill user from OIDC claims: %w", err)
	}
	return true, nil
}

// GetUserByID gets a user by ID
func (s *userService) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, id)
	if err == nil {
		s.enrichKnowledgeDomainAdminFlag(ctx, user)
	}
	return user, err
}

// GetUserDetail returns the full user admin detail view: profile, knowledge
// domain admin scope, and active refresh tokens.
func (s *userService) GetUserDetail(ctx context.Context, id string) (*types.UserDetailResponse, error) {
	user, err := s.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.enrichKnowledgeDomainAdminFlag(ctx, user)

	resp := &types.UserDetailResponse{
		ID:                     user.ID,
		Username:               user.Username,
		Email:                  user.Email,
		Avatar:                 user.Avatar,
		Status:                 user.Status,
		IsSystemAdmin:          user.IsSystemAdmin,
		IsKnowledgeDomainAdmin: user.IsKnowledgeDomainAdmin,
		RoleKnowledgeOfficer:   user.RoleKnowledgeOfficer,
		EmployeeID:             user.EmployeeID,
		Account:                user.Account,
		EnglishName:            user.EnglishName,
		ChineseName:            user.ChineseName,
		DepartmentCode:         user.DepartmentCode,
		DepartmentName:         user.DepartmentName,
		CreatedAt:              user.CreatedAt,
		UpdatedAt:              user.UpdatedAt,
	}

	if user.IsKnowledgeDomainAdmin && s.domainAdmins != nil && s.kdRepo != nil && s.kbRepo != nil {
		resp.KnowledgeScopes, err = s.buildKnowledgeScopes(ctx, user.ID)
		if err != nil {
			logger.Warnf(ctx, "Failed to build knowledge scopes for user %s: %v", user.ID, err)
		}
	}

	resp.RefreshTokens, err = s.buildRefreshTokens(ctx, user.ID)
	if err != nil {
		logger.Warnf(ctx, "Failed to load refresh tokens for user %s: %v", user.ID, err)
	}

	return resp, nil
}

// buildKnowledgeScopes loads every knowledge domain the user administrates
// and the list of knowledge-base names inside each domain.
func (s *userService) buildKnowledgeScopes(ctx context.Context, userID string) ([]types.UserKnowledgeScope, error) {
	domainIDs, err := s.domainAdmins.ListDomainIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(domainIDs) == 0 {
		return []types.UserKnowledgeScope{}, nil
	}

	scopes := make([]types.UserKnowledgeScope, 0, len(domainIDs))
	for _, domainID := range domainIDs {
		domain, err := s.kdRepo.GetKnowledgeDomainByID(ctx, domainID)
		if err != nil {
			logger.Warnf(ctx, "Failed to load knowledge domain %d for user detail: %v", domainID, err)
			continue
		}
		if domain == nil {
			continue
		}
		kbs, err := s.kbRepo.ListKnowledgeBasesByKnowledgeDomainID(ctx, domainID)
		if err != nil {
			logger.Warnf(ctx, "Failed to list knowledge bases for domain %d: %v", domainID, err)
			continue
		}
		names := make([]string, 0, len(kbs))
		for _, kb := range kbs {
			if kb != nil {
				names = append(names, kb.Name)
			}
		}
		scopes = append(scopes, types.UserKnowledgeScope{
			KnowledgeDomainID:   domain.ID,
			KnowledgeDomainName: domain.Name,
			KnowledgeBaseNames:  names,
		})
	}
	return scopes, nil
}

// buildRefreshTokens returns the refresh-token entries for a user, tagged with
// their current status (active / revoked / expired).
func (s *userService) buildRefreshTokens(ctx context.Context, userID string) ([]types.UserRefreshToken, error) {
	tokens, err := s.tokenRepo.GetTokensByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	out := make([]types.UserRefreshToken, 0)
	for _, t := range tokens {
		if t == nil || t.TokenType != "refresh_token" {
			continue
		}
		status := "active"
		if t.IsRevoked {
			status = "revoked"
		} else if t.ExpiresAt.Before(now) {
			status = "expired"
		}
		out = append(out, types.UserRefreshToken{
			JTI:        t.ID,
			Status:     status,
			IssuedAt:   t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
			ExpiresAt:  t.ExpiresAt,
		})
	}
	return out, nil
}

// GetUsersByIDs proxies to the repository batch fetch. Returns an empty
// map for an empty input; missing ids are absent from the result.
func (s *userService) GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error) {
	return s.userRepo.GetUsersByIDs(ctx, ids)
}

// GetUserByEmail gets a user by email
func (s *userService) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	return s.userRepo.GetUserByEmail(ctx, email)
}

// GetUserByUsername gets a user by username
func (s *userService) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	return s.userRepo.GetUserByUsername(ctx, username)
}

// GetUserByEmployeeID gets a user by employee id
func (s *userService) GetUserByEmployeeID(ctx context.Context, employeeID string) (*types.User, error) {
	return s.userRepo.GetUserByEmployeeID(ctx, employeeID)
}

// UpdateUser updates user information
func (s *userService) UpdateUser(ctx context.Context, user *types.User) error {
	user.UpdatedAt = time.Now()
	return s.userRepo.UpdateUser(ctx, user)
}

// ListSystemAdmins lists users with IsSystemAdmin=true. Thin pass-through
// to the repository; the handler enforces SystemAdmin gating, so the
// service does not duplicate the role check here.
func (s *userService) ListSystemAdmins(
	ctx context.Context, offset, limit int,
) ([]*types.User, int64, error) {
	return s.userRepo.ListSystemAdmins(ctx, offset, limit)
}

// RevokeSystemAdmin removes system-admin privileges through the
// repository's transactional guard so concurrent revokes cannot remove
// the final administrator.
func (s *userService) RevokeSystemAdmin(ctx context.Context, userID, actorID string) (*types.User, error) {
	user, err := s.userRepo.RevokeSystemAdmin(ctx, userID, actorID)
	if err != nil {
		return nil, err
	}
	if s.businessAudit != nil && user != nil {
		s.businessAudit.RecordSystemAdminRevoked(ctx, actorID, user.ID, user.Email, user.Username, true)
	}
	return user, nil
}

// UpdateUserPreferences applies a partial update over the user's
// preferences blob. PATCH semantics: only keys present in `patch`
// (non-nil pointer fields) replace the existing value; everything else
// is preserved. This lets the front-end PUT only the toggle that
// changed without having to read-modify-write the whole struct, and
// also makes the endpoint forward-compatible 閳?older clients that
// don't know about newer keys won't accidentally erase them.
func (s *userService) UpdateUserPreferences(
	ctx context.Context,
	userID string,
	patch types.UserPreferences,
) (types.UserPreferences, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return types.UserPreferences{}, err
	}

	merged := user.Preferences
	if patch.EnableMemory != nil {
		v := *patch.EnableMemory
		merged.EnableMemory = &v
	}

	user.Preferences = merged
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return types.UserPreferences{}, err
	}
	return merged, nil
}

// GetConfidentialityAck reports the calling user's confidentiality
// acknowledgement state. acknowledged is false when the timestamp is NULL
// (the user has never confirmed) and true otherwise.
func (s *userService) GetConfidentialityAck(
	ctx context.Context,
	userID string,
) (bool, *time.Time, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return false, nil, err
	}
	return user.ConfidentialityAcknowledgedAt != nil, user.ConfidentialityAcknowledgedAt, nil
}

// AcknowledgeConfidentiality stamps the current time as the user's
// confidentiality acknowledgement. It is idempotent: a second call updates
// the timestamp but never clears it. The persisted timestamp is returned so
// the handler can echo it back to the client.
func (s *userService) AcknowledgeConfidentiality(
	ctx context.Context,
	userID string,
) (*time.Time, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user.ConfidentialityAcknowledgedAt = &now
	user.UpdatedAt = now
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user.ConfidentialityAcknowledgedAt, nil
}

// DeleteUser deletes a user
func (s *userService) DeleteUser(ctx context.Context, id string) error {
	// Fetch the target first so the audit trail can carry email/username
	// even though the row is removed afterwards.
	target, _ := s.userRepo.GetUserByID(ctx, id)
	if err := s.userRepo.DeleteUser(ctx, id); err != nil {
		return err
	}
	if s.businessAudit != nil {
		email, username := "", ""
		if target != nil {
			email, username = target.Email, target.Username
		}
		s.businessAudit.RecordUserDeleted(ctx, auditActor(ctx), id, email, username)
	}
	return nil
}

// ChangePassword changes user password
func (s *userService) ChangePassword(ctx context.Context, userID string, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify old password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword))
	if err != nil {
		return errors.New("invalid old password")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hashedPassword)
	user.UpdatedAt = time.Now()

	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return err
	}

	// Invalidate every outstanding session so a stolen token cannot
	// survive a password rotation.
	if err := s.tokenRepo.RevokeTokensByUserID(ctx, userID); err != nil {
		return err
	}
	if s.businessAudit != nil {
		s.businessAudit.RecordUserPasswordChanged(ctx, userID)
	}
	return nil
}

// ValidatePassword validates user password
func (s *userService) ValidatePassword(ctx context.Context, userID string, password string) error {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
}

// GenerateTokens creates a user-scoped access token and a rotating refresh token.
// Knowledge access is resolved from grants when a resource is queried; it is not
// encoded into the authentication token.
func (s *userService) GenerateTokens(
	ctx context.Context,
	user *types.User,
) (accessToken, refreshToken string, err error) {
	now := time.Now()
	accessID := uuid.New().String()
	refreshID := uuid.New().String()
	accessExp := now.Add(24 * time.Hour)
	refreshExp := now.Add(7 * 24 * time.Hour)

	accessClaims := jwt.MapClaims{
		"user_id":                user.ID,
		"email":                  user.Email,
		"is_system_admin":        user.IsSystemAdmin,
		"role_knowledge_officer": user.RoleKnowledgeOfficer,
		"jti":                    accessID,
		"exp":                    accessExp.Unix(),
		"iat":                    now.Unix(),
		"type":                   "access",
	}

	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessTokenObj.SignedString([]byte(getJwtSecret()))
	if err != nil {
		return "", "", err
	}

	refreshClaims := jwt.MapClaims{
		"user_id": user.ID,
		"jti":     refreshID,
		"exp":     refreshExp.Unix(),
		"iat":     now.Unix(),
		"type":    "refresh",
	}
	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshTokenObj.SignedString([]byte(getJwtSecret()))
	if err != nil {
		return "", "", err
	}

	accessRecord := &types.AuthToken{
		ID: accessID, UserID: user.ID, Token: accessToken,
		TokenType: "access_token", ExpiresAt: accessExp,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tokenRepo.CreateToken(ctx, accessRecord); err != nil {
		return "", "", fmt.Errorf("persist access token: %w", err)
	}

	refreshRecord := &types.AuthToken{
		ID: refreshID, UserID: user.ID, Token: refreshToken,
		TokenType: "refresh_token", ExpiresAt: refreshExp,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tokenRepo.CreateToken(ctx, refreshRecord); err != nil {
		// Avoid leaving a usable half-created session when the refresh token
		// cannot be persisted. The primary persistence error is returned even
		// if this best-effort cleanup also fails.
		_ = s.tokenRepo.DeleteToken(ctx, accessRecord.ID)
		return "", "", fmt.Errorf("persist refresh token: %w", err)
	}
	return accessToken, refreshToken, nil
}

// ValidateToken validates an access token and returns the current user.
func (s *userService) ValidateToken(ctx context.Context, tokenString string) (*types.User, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJwtSecret()), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}
	userID, ok := claims["user_id"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return nil, errors.New("invalid user ID in token")
	}
	if isRefreshTokenClaims(claims) {
		return nil, errors.New("refresh token cannot be used as access token")
	}

	tokenRecord, err := s.tokenRepo.GetTokenByValue(ctx, tokenString)
	if err != nil || tokenRecord == nil || tokenRecord.IsRevoked {
		return nil, errors.New("token is revoked")
	}
	if tokenRecord.TokenType == "refresh_token" {
		return nil, errors.New("refresh token cannot be used as access token")
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != types.UserStatusNormal {
		return nil, errors.New("account is disabled")
	}
	if user.IsBanned() {
		return nil, errors.New("account is banned")
	}
	return user, nil
}
func isRefreshTokenClaims(claims jwt.MapClaims) bool {
	tokenType, ok := claims["type"].(string)
	return ok && tokenType == "refresh"
}

func userIDFromSignedToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJwtSecret()), nil
	}, jwt.WithoutClaimsValidation())
	if err != nil || token == nil || !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	userID, ok := claims["user_id"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return "", errors.New("invalid user ID in token")
	}
	return userID, nil
}

// RefreshToken refreshes access token using refresh token
func (s *userService) RefreshToken(
	ctx context.Context,
	refreshTokenString string,
) (accessToken, newRefreshToken string, err error) {
	token, err := jwt.Parse(refreshTokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJwtSecret()), nil
	})

	if err != nil || !token.Valid {
		return "", "", errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", errors.New("invalid token claims")
	}

	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != "refresh" {
		return "", "", errors.New("not a refresh token")
	}

	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", "", errors.New("invalid user ID in token")
	}

	// Check if token is revoked
	tokenRecord, err := s.tokenRepo.GetTokenByValue(ctx, refreshTokenString)
	if err != nil || tokenRecord == nil || tokenRecord.IsRevoked {
		return "", "", errors.New("refresh token is revoked")
	}
	if tokenRecord.TokenType != "refresh_token" {
		return "", "", errors.New("not a refresh token")
	}

	// Get user
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return "", "", err
	}

	// Record that this refresh token was used to rotate the session, then
	// revoke it so the same token cannot be used again.
	now := time.Now()
	tokenRecord.LastUsedAt = &now
	tokenRecord.UpdatedAt = now
	tokenRecord.IsRevoked = true
	if err := s.tokenRepo.UpdateToken(ctx, tokenRecord); err != nil {
		return "", "", fmt.Errorf("revoke rotated refresh token: %w", err)
	}

	// Generate new tokens
	return s.GenerateTokens(ctx, user)
}

// Logout invalidates every outstanding session for the user identified by
// the presented JWT. Access and refresh tokens are both accepted so clients
// can end the session without refreshing first; expired tokens are allowed
// so logout still works after the access token TTL.
func (s *userService) Logout(ctx context.Context, tokenString string) error {
	userID, err := userIDFromSignedToken(tokenString)
	if err != nil {
		return err
	}
	if err := s.tokenRepo.RevokeTokensByUserID(ctx, userID); err != nil {
		return err
	}
	// Record logout audit — best-effort, failure must not break the flow.
	email := ""
	if u, _ := s.userRepo.GetUserByID(ctx, userID); u != nil {
		email = u.Email
	}
	s.recordLogout(ctx, userID, email)
	return nil
}

// RevokeToken revokes a token
func (s *userService) RevokeToken(ctx context.Context, tokenString string) error {
	tokenRecord, err := s.tokenRepo.GetTokenByValue(ctx, tokenString)
	if err != nil {
		return err
	}

	tokenRecord.IsRevoked = true
	tokenRecord.UpdatedAt = time.Now()

	return s.tokenRepo.UpdateToken(ctx, tokenRecord)
}

// GetCurrentUser gets current user from context
func (s *userService) GetCurrentUser(ctx context.Context) (*types.User, error) {
	user, ok := ctx.Value(types.UserContextKey).(*types.User)
	if !ok {
		return nil, errors.New("user not found in context")
	}

	s.enrichKnowledgeDomainAdminFlag(ctx, user)
	return user, nil
}

func (s *userService) enrichKnowledgeDomainAdminFlag(ctx context.Context, user *types.User) {
	if user == nil || user.IsSystemAdmin || s.domainAdmins == nil {
		return
	}
	ids, err := s.domainAdmins.ListDomainIDs(ctx, user.ID)
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve knowledge-domain administration for user %s: %v", user.ID, err)
		return
	}
	user.IsKnowledgeDomainAdmin = len(ids) > 0
}

// SearchUsers searches users by username or email
func (s *userService) SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error) {
	if query == "" {
		return []*types.User{}, nil
	}
	return s.userRepo.SearchUsers(ctx, query, limit)
}

type oidcDiscoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
}

func newOIDCHTTPClient() *http.Client {
	cfg := secutils.DefaultSSRFSafeHTTPClientConfig()
	cfg.Timeout = 30 * time.Second
	return secutils.NewSSRFSafeHTTPClient(cfg)
}

func validateOIDCEndpoint(label, endpoint string, required bool) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		if required {
			return fmt.Errorf("OIDC %s endpoint is required", label)
		}
		return nil
	}
	if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
		return fmt.Errorf("OIDC %s endpoint failed SSRF validation: %w", label, err)
	}
	return nil
}

func validateOIDCEndpoints(cfg *config.OIDCAuthConfig) error {
	if err := validateOIDCEndpoint("authorization", cfg.AuthorizationEndpoint, true); err != nil {
		return err
	}
	if err := validateOIDCEndpoint("token", cfg.TokenEndpoint, true); err != nil {
		return err
	}
	if err := validateOIDCEndpoint("userinfo", cfg.UserInfoEndpoint, true); err != nil {
		return err
	}
	return nil
}

func (s *userService) getOIDCConfig(ctx context.Context) (*config.OIDCAuthConfig, error) {
	if s.config == nil || s.config.OIDCAuth == nil || !s.config.OIDCAuth.Enable {
		return nil, errors.New("OIDC login is disabled")
	}
	cfg := *s.config.OIDCAuth
	if cfg.UserInfoMapping == nil {
		cfg.UserInfoMapping = &config.OIDCUserInfoMapping{Username: "name", Email: "email"}
	}
	if err := s.populateOIDCEndpoints(ctx, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *userService) populateOIDCEndpoints(ctx context.Context, cfg *config.OIDCAuthConfig) error {
	if strings.TrimSpace(cfg.AuthorizationEndpoint) != "" && strings.TrimSpace(cfg.TokenEndpoint) != "" {
		return validateOIDCEndpoints(cfg)
	}
	if strings.TrimSpace(cfg.DiscoveryURL) == "" {
		return errors.New("OIDC discovery_url or explicit endpoints are required")
	}
	if err := validateOIDCEndpoint("discovery", cfg.DiscoveryURL, true); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.DiscoveryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create OIDC discovery request: %w", err)
	}

	resp, err := newOIDCHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to load OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("OIDC discovery request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var doc oidcDiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("failed to decode OIDC discovery document: %w", err)
	}
	if cfg.AuthorizationEndpoint == "" {
		cfg.AuthorizationEndpoint = doc.AuthorizationEndpoint
	}
	if cfg.TokenEndpoint == "" {
		cfg.TokenEndpoint = doc.TokenEndpoint
	}
	if cfg.UserInfoEndpoint == "" {
		cfg.UserInfoEndpoint = doc.UserInfoEndpoint
	}
	if cfg.AuthorizationEndpoint == "" || cfg.TokenEndpoint == "" {
		return errors.New("OIDC discovery document missing required endpoints")
	}
	return validateOIDCEndpoints(cfg)
}

func (s *userService) exchangeOIDCCode(ctx context.Context, cfg *config.OIDCAuthConfig, code, redirectURI string) (*oidcTokenResponse, error) {
	if err := validateOIDCEndpoint("token", cfg.TokenEndpoint, true); err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := newOIDCHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange OIDC code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("OIDC token exchange failed: status=%d", resp.StatusCode)
	}

	var tokenResp oidcTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode OIDC token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" && strings.TrimSpace(tokenResp.IDToken) == "" {
		return nil, errors.New("OIDC token response missing access_token and id_token")
	}
	return &tokenResp, nil
}

func (s *userService) resolveOIDCUserInfo(ctx context.Context, cfg *config.OIDCAuthConfig, tokenResp *oidcTokenResponse) (*types.OIDCUserInfo, error) {
	// A base64-decoded ID token is not authenticated. Until this adapter
	// performs full signature/issuer/audience/nonce verification, identity
	// claims must come from the protected UserInfo endpoint.
	if strings.TrimSpace(cfg.UserInfoEndpoint) == "" {
		return nil, errors.New("OIDC userinfo endpoint is required")
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, errors.New("OIDC access_token is required for userinfo")
	}
	claims, err := s.fetchOIDCUserInfo(ctx, cfg.UserInfoEndpoint, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC userinfo: %w", err)
	}

	info := &types.OIDCUserInfo{Claims: claims}
	if sub, _ := claims["sub"].(string); sub != "" {
		info.Subject = sub
	}
	info.Username = extractClaimAsString(claims, cfg.UserInfoMapping.Username)
	info.Email = extractClaimAsString(claims, cfg.UserInfoMapping.Email)
	info.EmployeeID = extractClaimAsString(claims, cfg.UserInfoMapping.EmployeeID)
	if info.Username == "" {
		info.Username = extractClaimAsString(claims, "preferred_username")
	}
	if info.Username == "" {
		info.Username = extractClaimAsString(claims, "name")
	}
	if info.Username == "" && info.Email != "" {
		info.Username = strings.Split(info.Email, "@")[0]
	}
	return info, nil
}

func (s *userService) fetchOIDCUserInfo(ctx context.Context, endpoint, accessToken string) (map[string]interface{}, error) {
	if err := validateOIDCEndpoint("userinfo", endpoint, true); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := newOIDCHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("userinfo request failed: status=%d", resp.StatusCode)
	}

	var claims map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func (s *userService) provisionOIDCUser(
	ctx context.Context,
	info *types.OIDCUserInfo,
	issuer string,
) (*types.User, error) {
	if s.config == nil || s.config.OIDCAuth == nil || !s.config.OIDCAuth.AutoProvision {
		return nil, errors.New("SSO account is not provisioned; contact an enterprise administrator")
	}
	username := s.generateOIDCUsername(ctx, info)
	randomPassword, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password for OIDC user: %w", err)
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash OIDC fallback password: %w", err)
	}

	now := time.Now()
	user := &types.User{
		ID: uuid.New().String(), Username: username, Email: info.Email,
		PasswordHash: string(hashedPassword),
		CreatedAt:    now, UpdatedAt: now,
	}
	identity := &types.SSOIdentity{
		UserID: user.ID, Provider: "oidc", Issuer: issuer, Subject: info.Subject,
		CreatedAt: now, UpdatedAt: now, LastLoginAt: &now,
	}
	if err := s.ssoRepo.CreateEnterpriseUser(ctx, user, identity); err != nil {
		return nil, fmt.Errorf("failed to auto-provision OIDC user: %w", err)
	}
	return user, nil
}
func oidcIdentityIssuer(cfg *config.OIDCAuthConfig) string {
	if cfg == nil {
		return "oidc"
	}
	for _, candidate := range []string{cfg.IssuerURL, cfg.DiscoveryURL, cfg.AuthorizationEndpoint} {
		if value := strings.TrimSpace(candidate); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return "oidc"
}

func (s *userService) generateOIDCUsername(ctx context.Context, info *types.OIDCUserInfo) string {
	base := sanitizeUsernameCandidate(info.Username)
	if base == "" {
		base = sanitizeUsernameCandidate(strings.Split(info.Email, "@")[0])
	}
	if base == "" {
		base = "oidc-user"
	}

	candidate := base
	for i := 0; i < 20; i++ {
		existing, err := s.userRepo.GetUserByUsername(ctx, candidate)
		if isUserLookupNotFound(err) || (err == nil && existing == nil) {
			return candidate
		}
		if err != nil && !isUserLookupNotFound(err) {
			logger.Warnf(ctx, "Failed to check existing OIDC username %q: %v", candidate, err)
		}
		candidate = fmt.Sprintf("%s-%d", base, i+1)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func generateRandomString(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func extractClaimAsString(claims map[string]interface{}, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	value, ok := claims[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func sanitizeUsernameCandidate(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-._")
	if len(result) > 50 {
		result = strings.Trim(result[:50], "-._")
	}
	return result
}

func isUserLookupNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, apprepo.ErrUserNotFound) || strings.Contains(strings.ToLower(err.Error()), "user not found")
}

// BanUser bans a user (sets status=banned, revokes tokens).
func (s *userService) BanUser(ctx context.Context, userID, actorID string) (*types.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.IsBanned() {
		return user, nil // idempotent
	}

	now := time.Now()
	user.Status = types.UserStatusBanned
	user.BannedAt = &now
	user.BannedBy = &actorID
	user.UpdatedAt = now

	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	// Revoke all tokens so the banned user cannot continue using existing sessions.
	_ = s.tokenRepo.RevokeTokensByUserID(ctx, userID)

	logger.Infof(ctx, "User %s banned by %s", userID, actorID)
	if s.businessAudit != nil {
		s.businessAudit.RecordUserBanned(ctx, actorID, user.ID, user.Email, user.Username, "")
	}
	return user, nil
}

// OfflineUser forces a user offline by revoking every outstanding access and
// refresh token. Unlike BanUser it does not change the account status or ban
// fields, so the user may sign in again immediately afterwards.
func (s *userService) OfflineUser(ctx context.Context, userID, actorID string) (*types.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if err := s.tokenRepo.RevokeTokensByUserID(ctx, userID); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "User %s forced offline by %s", userID, actorID)
	if s.businessAudit != nil {
		s.businessAudit.RecordUserOfflined(ctx, actorID, user.ID, user.Email, user.Username)
	}
	return user, nil
}

// UnbanUser unbans a user (sets status=normal, clears ban fields).
func (s *userService) UnbanUser(ctx context.Context, userID, actorID string) (*types.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status == types.UserStatusNormal {
		return user, nil // idempotent
	}

	now := time.Now()
	user.Status = types.UserStatusNormal
	user.BannedAt = nil
	user.BannedBy = nil
	user.UpdatedAt = now

	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "User %s unbanned by %s", userID, actorID)
	if s.businessAudit != nil {
		s.businessAudit.RecordUserUnbanned(ctx, actorID, user.ID, user.Email, user.Username)
	}
	return user, nil
}

// BatchUpdateUserRoles updates role_knowledge_officer for a batch of users.
// When setting knowledge officer (role_knowledge_officer=1), it also binds the
// user to the specified knowledge bases as an officer. The operator role is
// never modified here — it is handled by BatchUpdateOperatorRole.
func (s *userService) BatchUpdateUserRoles(ctx context.Context, userIDs []string, patch types.KnowledgeOfficerRolesPatch, actorID string) (int64, error) {
	affected, err := s.userRepo.BatchUpdateUserRoles(ctx, userIDs, patch)
	if err != nil {
		return 0, err
	}

	// Handle knowledge base officer bindings when role_knowledge_officer is set
	if patch.RoleKnowledgeOfficer != nil {
		if *patch.RoleKnowledgeOfficer == 1 && len(patch.KnowledgeBaseIDs) > 0 && patch.KnowledgeDomainID != nil {
			for _, userID := range userIDs {
				if err := s.bindKnowledgeOfficer(ctx, userID, *patch.KnowledgeDomainID, patch.KnowledgeBaseIDs, actorID); err != nil {
					logger.Warnf(ctx, "Failed to bind knowledge officer for user %s: %v", userID, err)
				}
			}
		} else if *patch.RoleKnowledgeOfficer == 0 {
			for _, userID := range userIDs {
				if err := s.unbindKnowledgeOfficer(ctx, userID); err != nil {
					logger.Warnf(ctx, "Failed to unbind knowledge officer for user %s: %v", userID, err)
				}
			}
		}
	}

	logger.Infof(ctx, "Batch role update by %s: %d users affected", actorID, affected)
	if s.businessAudit != nil && affected > 0 {
		rolesChanged := map[string]interface{}{}
		if patch.RoleKnowledgeOfficer != nil {
			rolesChanged["role_knowledge_officer"] = *patch.RoleKnowledgeOfficer
		}
		s.businessAudit.RecordUserRolesUpdated(ctx, actorID, int(affected), rolesChanged)
	}
	return affected, nil
}

// UpdateUserRoles updates role_knowledge_officer for a single user.
// When setting knowledge officer (role_knowledge_officer=1), it also binds the
// user to the specified knowledge bases as an officer. The operator role is
// never modified here — it is handled by UpdateOperatorRole.
func (s *userService) UpdateUserRoles(ctx context.Context, userID string, patch types.KnowledgeOfficerRolesPatch, actorID string) (int64, error) {
	affected, err := s.userRepo.UpdateUserRoles(ctx, userID, patch)
	if err != nil {
		return 0, err
	}

	// Handle knowledge base officer bindings when role_knowledge_officer is set
	if patch.RoleKnowledgeOfficer != nil {
		if *patch.RoleKnowledgeOfficer == 1 && len(patch.KnowledgeBaseIDs) > 0 && patch.KnowledgeDomainID != nil {
			if err := s.bindKnowledgeOfficer(ctx, userID, *patch.KnowledgeDomainID, patch.KnowledgeBaseIDs, actorID); err != nil {
				logger.Warnf(ctx, "Failed to bind knowledge officer for user %s: %v", userID, err)
			}
		} else if *patch.RoleKnowledgeOfficer == 0 {
			if err := s.unbindKnowledgeOfficer(ctx, userID); err != nil {
				logger.Warnf(ctx, "Failed to unbind knowledge officer for user %s: %v", userID, err)
			}
		}
	}

	logger.Infof(ctx, "Role update for user %s by %s: affected=%d", userID, actorID, affected)
	if s.businessAudit != nil && affected > 0 {
		rolesChanged := map[string]interface{}{}
		if patch.RoleKnowledgeOfficer != nil {
			rolesChanged["role_knowledge_officer"] = *patch.RoleKnowledgeOfficer
		}
		s.businessAudit.RecordUserRolesUpdated(ctx, actorID, int(affected), rolesChanged)
	}
	return affected, nil
}

// BatchUpdateOperatorRole grants/revokes the operator role for a batch of
// users by setting is_system_admin (1 = true, 0 = false). It never touches
// the knowledge-officer role or its knowledge-base bindings.
func (s *userService) BatchUpdateOperatorRole(ctx context.Context, userIDs []string, patch types.OperatorRolesPatch, actorID string) (int64, error) {
	affected, err := s.userRepo.BatchUpdateOperatorRole(ctx, userIDs, patch)
	if err != nil {
		return 0, err
	}

	logger.Infof(ctx, "Batch operator role update by %s: %d users affected", actorID, affected)
	if s.businessAudit != nil && affected > 0 && patch.RoleOperator != nil {
		rolesChanged := map[string]interface{}{
			"is_system_admin": *patch.RoleOperator,
			"batch":           true,
		}
		s.businessAudit.RecordUserRolesUpdated(ctx, actorID, int(affected), rolesChanged)
	}
	return affected, nil
}

// UpdateOperatorRole grants/revokes the operator role for a single user by
// setting is_system_admin (1 = true, 0 = false). It never touches the
// knowledge-officer role or its knowledge-base bindings.
func (s *userService) UpdateOperatorRole(ctx context.Context, userID string, patch types.OperatorRolesPatch, actorID string) (int64, error) {
	affected, err := s.userRepo.UpdateOperatorRole(ctx, userID, patch)
	if err != nil {
		return 0, err
	}

	logger.Infof(ctx, "Operator role update for user %s by %s: affected=%d", userID, actorID, affected)
	if s.businessAudit != nil && affected > 0 && patch.RoleOperator != nil {
		target, uerr := s.userRepo.GetUserByID(ctx, userID)
		email, username := "", ""
		if uerr == nil && target != nil {
			email, username = target.Email, target.Username
		}
		if *patch.RoleOperator == 1 {
			s.businessAudit.RecordSystemAdminPromoted(ctx, actorID, userID, email, username, false)
		} else {
			s.businessAudit.RecordSystemAdminRevoked(ctx, actorID, userID, email, username, true)
		}
	}
	return affected, nil
}

// bindKnowledgeOfficer binds a user to the given knowledge bases as an officer
// and grants manage permission via knowledge_resource_grants.
func (s *userService) bindKnowledgeOfficer(ctx context.Context, userID string, domainID uint64, kbIDs []string, grantedBy string) error {
	for _, kbID := range kbIDs {
		if err := s.eaRepo.AddKnowledgeBaseOfficer(ctx, kbID, userID, grantedBy); err != nil {
			logger.Warnf(ctx, "AddKnowledgeBaseOfficer failed for user %s kb %s: %v", userID, kbID, err)
		}
		grant := &types.KnowledgeResourceGrant{
			KnowledgeDomainID: domainID,
			KnowledgeBaseID:   kbID,
			ResourceType:      types.KnowledgeResourceKnowledgeBase,
			ResourceID:        kbID,
			SubjectType:       types.GrantSubjectUser,
			SubjectID:         userID,
			Permission:        types.KnowledgeBasePermissionManage,
			Effect:            types.GrantEffectAllow,
			InheritToChildren: true,
			GrantedBy:         &grantedBy,
		}
		if err := s.eaRepo.UpsertResourceGrant(ctx, grant); err != nil {
			logger.Warnf(ctx, "UpsertResourceGrant failed for user %s kb %s: %v", userID, kbID, err)
		}
	}
	return nil
}

// unbindKnowledgeOfficer removes all knowledge base officer bindings and
// related resource grants for the user.
func (s *userService) unbindKnowledgeOfficer(ctx context.Context, userID string) error {
	kbIDs, err := s.eaRepo.ListOfficerKnowledgeBaseIDs(ctx, userID)
	if err != nil {
		logger.Warnf(ctx, "ListOfficerKnowledgeBaseIDs failed for user %s: %v", userID, err)
	}
	for _, kbID := range kbIDs {
		if err := s.eaRepo.RemoveKnowledgeBaseOfficer(ctx, kbID, userID); err != nil {
			logger.Warnf(ctx, "RemoveKnowledgeBaseOfficer failed for user %s kb %s: %v", userID, kbID, err)
		}
	}
	return nil
}

// ListUsers lists all users with optional filters, paginated.
func (s *userService) ListUsers(ctx context.Context, offset, limit int, filter types.ListUsersFilter) ([]*types.User, int64, error) {
	return s.userRepo.ListUsers(ctx, offset, limit, filter)
}

// CreateUser creates a local user from the admin panel. The admin provides a
// single identifier in id_or_email; the server decides whether it is a numeric
// employee id (persisted in users.employee_id) or an email address (persisted
// in users.email). The other identifier remains empty until the user signs in
// via SSO and back-fills it. Default status is active/on-duty and default
// role flags are off.
func (s *userService) CreateUser(ctx context.Context, req *types.CreateUserRequest) (*types.User, error) {
	logger.Info(ctx, "Start admin user creation")

	raw := strings.TrimSpace(req.IDOrEmail)
	if raw == "" {
		return nil, errors.New("id_or_email is required")
	}

	// The admin input that does NOT contain '@' or '.com' — including
	// alphanumeric employee ids such as E10002 — is stored in users.employee_id.
	// Any other input is treated as an email address (users.email) and is
	// lower-cased before persistence.
	var email, employeeID string
	if !emailHintRegex.MatchString(raw) {
		employeeID = raw
	} else {
		email = strings.ToLower(raw)
	}

	// Derive the username. For email input use the local part; for a numeric
	// employee id use the id itself. Then ensure uniqueness by appending a
	// numeric suffix if necessary.
	baseUsername := usernameFromIdentifier(email, employeeID)
	if baseUsername == "" {
		return nil, errors.New("cannot derive username from id_or_email")
	}
	username := s.ensureUniqueUsername(ctx, baseUsername)

	if email != "" {
		if existingUser, _ := s.userRepo.GetUserByEmail(ctx, email); existingUser != nil {
			return nil, errors.New("user with this email already exists")
		}
	}
	if employeeID != "" {
		if existingUser, _ := s.userRepo.GetUserByEmployeeID(ctx, employeeID); existingUser != nil {
			return nil, errors.New("user with this employee id already exists")
		}
	}

	// Generate a random initial password; the user is expected to sign in
	// through SSO or to have the password reset by an administrator.
	password, err := generateRandomPassword()
	if err != nil {
		return nil, errors.New("failed to generate password")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf(ctx, "Failed to hash password: %v", err)
		return nil, errors.New("failed to process password")
	}

	now := time.Now()
	user := &types.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		EmployeeID:   employeeID,
		Provider:     types.UserProviderWhiteList,
		Status:       types.UserStatusNormal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		logger.Errorf(ctx, "Failed to create user: %v", err)
		return nil, errors.New("failed to create user")
	}

	logger.Info(ctx, "Admin created user successfully")
	if s.businessAudit != nil {
		s.businessAudit.RecordUserCreated(ctx, user.ID, user.Email, user.Username, types.UserProviderWhiteList)
	}
	return user, nil
}

// usernameFromIdentifier derives a default username from whichever identifier
// was supplied by the admin. Email inputs use the local part; numeric employee
// ids use the id itself.
func usernameFromIdentifier(email, employeeID string) string {
	if email != "" {
		return usernameFromEmail(email)
	}
	return sanitizeUsernameCandidate(employeeID)
}

// ensureUniqueUsername appends a numeric suffix to base when the username is
// already taken. This mirrors the OIDC username generation logic but accepts
// an explicit base candidate.
func (s *userService) ensureUniqueUsername(ctx context.Context, base string) string {
	candidate := base
	for i := 0; i < 20; i++ {
		existing, err := s.userRepo.GetUserByUsername(ctx, candidate)
		if isUserLookupNotFound(err) || (err == nil && existing == nil) {
			return candidate
		}
		if err != nil && !isUserLookupNotFound(err) {
			logger.Warnf(ctx, "Failed to check existing username %q: %v", candidate, err)
		}
		candidate = fmt.Sprintf("%s-%d", base, i+1)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

// usernameFromEmail returns the local part of an email address (the part
// before '@'), lower-cased, to be used as the default username.
func usernameFromEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(email, "@")
	if at <= 0 {
		return email
	}
	return email[:at]
}

// generateRandomPassword returns a cryptographically random 32-character
// password used as the initial password of imported users.
func generateRandomPassword() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// ------------------------------------------------------------------
// audit helpers (best-effort, never break business logic)
// ------------------------------------------------------------------

func (s *userService) recordLoginSuccess(ctx context.Context, user *types.User, method string) {
	if s.businessAudit != nil {
		s.businessAudit.RecordLogin(ctx, user.ID, user.Email, method, "")
	}
}

func (s *userService) recordLoginFailed(ctx context.Context, email, method, reason string) {
	if s.businessAudit != nil {
		s.businessAudit.RecordLoginFailed(ctx, email, method, reason, "")
	}
}

func (s *userService) recordLogout(ctx context.Context, userID, email string) {
	if s.businessAudit != nil {
		s.businessAudit.RecordLogout(ctx, userID, email)
	}
}
