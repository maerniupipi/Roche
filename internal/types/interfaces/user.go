package interfaces

import (
	"context"
	"net/http"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
)

// UserService defines the user service interface
type UserService interface {
	// Register creates a new user account
	Register(ctx context.Context, req *types.RegisterRequest) (*types.User, error)
	// Login authenticates a user and returns tokens
	Login(ctx context.Context, req *types.LoginRequest) (*types.LoginResponse, error)
	// GetOIDCAuthorizationURL builds the third-party OIDC authorization URL
	GetOIDCAuthorizationURL(ctx context.Context, redirectURI string) (*types.OIDCAuthURLResponse, error)
	// LoginWithOIDC exchanges the callback code, auto-provisions users if needed, and completes login
	LoginWithOIDC(ctx context.Context, code, redirectURI string) (*types.OIDCCallbackResponse, error)
	// GetSAMLAuthorizationURL builds the SAML SP-initiated single sign-on URL
	// for the configured IdP. redirectURI is embedded in the signed RelayState
	// so the ACS knows where to send the browser after login.
	GetSAMLAuthorizationURL(ctx context.Context, redirectURI string) (*types.SAMLAuthURLResponse, error)
	// LoginWithSAML validates the SAMLResponse posted by the IdP against the
	// configured IdP metadata, resolves/provisions the local user and returns
	// local login tokens. req carries the SAMLResponse (query or form) and
	// must be the original ACS request.
	LoginWithSAML(ctx context.Context, req *http.Request, redirectURI, requestID string) (*types.SAMLCallbackResponse, error)
	// GetSAMLMetadata returns the SAML service-provider metadata XML used to
	// register this application with the IdP.
	GetSAMLMetadata(ctx context.Context) ([]byte, error)
	// GetUserByID gets a user by ID
	GetUserByID(ctx context.Context, id string) (*types.User, error)
	// GetUserDetail gets a user by ID and assembles the full admin detail
	// view: profile, knowledge-domain admin scope, and active refresh tokens.
	GetUserDetail(ctx context.Context, id string) (*types.UserDetailResponse, error)
	// GetUsersByIDs batch-fetches users by id, returning a map keyed by
	// user id. Missing ids are simply absent from the result; the call
	// is not an error when some ids resolve to no row. Used on hot list
	// endpoints (knowledgeDomain members, audit logs) to avoid N+1 queries.
	GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error)
	// GetUserByEmail gets a user by email
	GetUserByEmail(ctx context.Context, email string) (*types.User, error)
	// GetUserByEmployeeID gets a user by employee id.
	GetUserByEmployeeID(ctx context.Context, employeeID string) (*types.User, error)
	// GetUserByUsername gets a user by username
	GetUserByUsername(ctx context.Context, username string) (*types.User, error)
	// UpdateUser updates user information
	UpdateUser(ctx context.Context, user *types.User) error
	// DeleteUser deletes a user
	DeleteUser(ctx context.Context, id string) error
	// ChangePassword changes user password
	ChangePassword(ctx context.Context, userID string, oldPassword, newPassword string) error
	// ValidatePassword validates user password
	ValidatePassword(ctx context.Context, userID string, password string) error
	// GenerateTokens generates access and refresh tokens for user
	GenerateTokens(ctx context.Context, user *types.User) (accessToken, refreshToken string, err error)
	// ValidateToken validates an access token and returns its active user.
	// Knowledge access is resolved from the target resource, never from JWT scope.
	ValidateToken(ctx context.Context, token string) (*types.User, error)
	// RefreshToken refreshes access token using refresh token
	RefreshToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error)
	// RevokeToken revokes a token
	RevokeToken(ctx context.Context, token string) error
	// Logout revokes every outstanding access/refresh token for the user
	// identified by the presented JWT.
	Logout(ctx context.Context, token string) error
	// GetCurrentUser gets current user from context
	GetCurrentUser(ctx context.Context) (*types.User, error)
	// SearchUsers searches users by username or email
	SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error)
	// ListSystemAdmins lists users with IsSystemAdmin=true.
	// Returns the page of admins plus the total count (for pagination UI);
	// callers pass offset/limit to page through results. Used by the
	// /api/v1/system/admin/list endpoint, gated to SystemAdmin callers.
	ListSystemAdmins(ctx context.Context, offset, limit int) ([]*types.User, int64, error)
	// RevokeSystemAdmin removes system-admin privileges with the
	// last-admin/self-revoke checks performed atomically.
	RevokeSystemAdmin(ctx context.Context, userID, actorID string) (*types.User, error)
	// UpdateUserPreferences partially updates the calling user's
	// preferences blob (PATCH semantics: only keys present in `patch`
	// overwrite existing values). Returns the updated, persisted prefs.
	UpdateUserPreferences(ctx context.Context, userID string, patch types.UserPreferences) (types.UserPreferences, error)
	// GetConfidentialityAck returns the current user's confidentiality
	// acknowledgement state. acknowledged=false means the user has not
	// yet confirmed the confidentiality statement and the client should
	// keep showing the acknowledgement dialog.
	GetConfidentialityAck(ctx context.Context, userID string) (acknowledged bool, acknowledgedAt *time.Time, err error)
	// AcknowledgeConfidentiality stamps the current time as the user's
	// confidentiality acknowledgement. Idempotent: calling it again after
	// the first acknowledgement refreshes the timestamp but never clears
	// it. Returns the persisted timestamp.
	AcknowledgeConfidentiality(ctx context.Context, userID string) (*time.Time, error)
	// BanUser bans a user (sets status=banned, revokes tokens, adds to blacklist table).
	BanUser(ctx context.Context, userID, actorID string) (*types.User, error)
	// UnbanUser unbans a user (sets status=normal, clears ban fields, removes from blacklist table).
	UnbanUser(ctx context.Context, userID, actorID string) (*types.User, error)
	// OfflineUser forces a user offline by revoking every outstanding access
	// and refresh token without changing the account status.
	OfflineUser(ctx context.Context, userID, actorID string) (*types.User, error)
	// BatchUpdateUserRoles updates role_knowledge_officer for a batch of users
	// and (de)bind the knowledge-base officer scope. This endpoint never
	// touches the operator role.
	// Returns the count of affected users.
	BatchUpdateUserRoles(ctx context.Context, userIDs []string, patch types.KnowledgeOfficerRolesPatch, actorID string) (int64, error)
	// UpdateUserRoles updates role_knowledge_officer for a single user and
	// (de)bind the knowledge-base officer scope. This endpoint never touches
	// the operator role.
	UpdateUserRoles(ctx context.Context, userID string, patch types.KnowledgeOfficerRolesPatch, actorID string) (int64, error)
	// BatchUpdateOperatorRole grants/revokes the operator role for a batch of
	// users by setting is_system_admin (1 = true, 0 = false). The users table
	// no longer has a role_operator column. Returns the count of affected users.
	BatchUpdateOperatorRole(ctx context.Context, userIDs []string, patch types.OperatorRolesPatch, actorID string) (int64, error)
	// UpdateOperatorRole grants/revokes the operator role for a single user by
	// setting is_system_admin (1 = true, 0 = false).
	UpdateOperatorRole(ctx context.Context, userID string, patch types.OperatorRolesPatch, actorID string) (int64, error)
	// ListUsers lists all users with optional filters, paginated.
	ListUsers(ctx context.Context, offset, limit int, filter types.ListUsersFilter) ([]*types.User, int64, error)
	// CreateUser creates a user by an administrator. Only user_id and email
	// are required; username and the initial password are derived/generated
	// server-side.
	CreateUser(ctx context.Context, req *types.CreateUserRequest) (*types.User, error)
}

// UserRepository defines the user repository interface
type UserRepository interface {
	// CreateUser creates a user
	CreateUser(ctx context.Context, user *types.User) error
	// GetUserByID gets a user by ID
	GetUserByID(ctx context.Context, id string) (*types.User, error)
	// GetUsersByIDs batch-fetches users by id, returning a map keyed by
	// user id. Missing ids are simply absent from the result.
	GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error)
	// GetUserByEmail gets a user by email
	GetUserByEmail(ctx context.Context, email string) (*types.User, error)
	// GetUserByEmployeeID gets a user by employee id.
	GetUserByEmployeeID(ctx context.Context, employeeID string) (*types.User, error)
	// GetUserByUsername gets a user by username
	GetUserByUsername(ctx context.Context, username string) (*types.User, error)
	// UpdateUser updates a user
	UpdateUser(ctx context.Context, user *types.User) error
	// DeleteUser deletes a user
	DeleteUser(ctx context.Context, id string) error
	// ListUsers lists all users with optional filters, paginated.
	ListUsers(ctx context.Context, offset, limit int, filter types.ListUsersFilter) ([]*types.User, int64, error)
	// ListSystemAdmins lists users where is_system_admin = true.
	// Walks the partial-friendly idx_users_is_system_admin index. Returns
	// the slice plus the total count for pagination metadata. Used by
	// the system-admin management endpoint.
	ListSystemAdmins(ctx context.Context, offset, limit int) ([]*types.User, int64, error)
	// RevokeSystemAdmin removes system-admin privileges with the
	// last-admin/self-revoke checks performed atomically.
	RevokeSystemAdmin(ctx context.Context, userID, actorID string) (*types.User, error)
	// SearchUsers searches users by username or email
	SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error)
	// BatchUpdateUserRoles updates role_knowledge_officer for a batch of users.
	// It never touches the operator role.
	BatchUpdateUserRoles(ctx context.Context, userIDs []string, patch types.KnowledgeOfficerRolesPatch) (int64, error)
	// UpdateUserRoles updates role_knowledge_officer for a single user.
	// It never touches the operator role.
	UpdateUserRoles(ctx context.Context, userID string, patch types.KnowledgeOfficerRolesPatch) (int64, error)
	// BatchUpdateOperatorRole grants/revokes the operator role for a batch of
	// users by setting is_system_admin (1 = true, 0 = false).
	BatchUpdateOperatorRole(ctx context.Context, userIDs []string, patch types.OperatorRolesPatch) (int64, error)
	// UpdateOperatorRole grants/revokes the operator role for a single user by
	// setting is_system_admin (1 = true, 0 = false).
	UpdateOperatorRole(ctx context.Context, userID string, patch types.OperatorRolesPatch) (int64, error)
	// GetWorkdayWorkersByIDs loads external_workers by external_worker_id for
	// a given provider. Unknown ids are silently absent from the result.
	GetWorkdayWorkersByIDs(ctx context.Context, provider string, ids []string) ([]*types.ExternalWorker, error)
	// GetUsersByEmails batch-fetches users by email (case-insensitive),
	// returning a map keyed by the lower-cased email.
	GetUsersByEmails(ctx context.Context, emails []string) (map[string]*types.User, error)
	// ResolveWorkdayOrgUnit resolves the canonical org_units row (id/code/name)
	// behind an external organization id.
	ResolveWorkdayOrgUnit(ctx context.Context, provider, externalOrgID string) (orgUnitID, code, name string, err error)
	// CreateWorkdayLinkedUser creates a user, back-fills external_workers.user_id
	// and establishes the primary workday org membership in one transaction.
	CreateWorkdayLinkedUser(ctx context.Context, user *types.User, worker *types.ExternalWorker, orgUnitID string) error
}

// AuthTokenRepository defines the auth token repository interface
type AuthTokenRepository interface {
	// CreateToken creates an auth token
	CreateToken(ctx context.Context, token *types.AuthToken) error
	// GetTokenByValue gets a token by its value
	GetTokenByValue(ctx context.Context, tokenValue string) (*types.AuthToken, error)
	// GetTokensByUserID gets all tokens for a user
	GetTokensByUserID(ctx context.Context, userID string) ([]*types.AuthToken, error)
	// UpdateToken updates a token
	UpdateToken(ctx context.Context, token *types.AuthToken) error
	// DeleteToken deletes a token
	DeleteToken(ctx context.Context, id string) error
	// DeleteExpiredTokens deletes all expired tokens
	DeleteExpiredTokens(ctx context.Context) error
	// RevokeTokensByUserID revokes all tokens for a user
	RevokeTokensByUserID(ctx context.Context, userID string) error
}

// BlacklistEntryRepository defines the blacklist repository interface.
// It manages the independent blacklist table used as a defense-in-depth
// check layer across all access points.
type BlacklistEntryRepository interface {
	// Add adds a user to the blacklist (idempotent).
	Add(ctx context.Context, entry *types.BlacklistEntry) error
	// Remove removes a user from the blacklist (idempotent).
	Remove(ctx context.Context, userID string) error
	// IsBlacklisted checks whether a user is in the blacklist.
	IsBlacklisted(ctx context.Context, userID string) (bool, error)
	// GetByUserID returns the blacklist entry for a user, or nil if not found.
	GetByUserID(ctx context.Context, userID string) (*types.BlacklistEntry, error)
}

