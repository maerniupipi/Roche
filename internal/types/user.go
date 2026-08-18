package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// UserPreferences holds per-user UI/feature preferences persisted server-side
// so they sync across devices/browsers. Fields are pointers so we can
// distinguish "client didn't send this key" (leave existing value alone)
// from "client explicitly set false" — the partial-update merge in
// UpdateUserPreferences relies on this.
//
// Adding a new preference key:
//  1. Add a *T field below + JSON tag (snake_case, must match the front-end key).
//  2. Extend the merge logic in service.UserService.UpdateUserPreferences.
//  3. Surface the new knob in the frontend settings store.
//
// No DB DDL is required — preferences is a single jsonb column.
type UserPreferences struct {
	// EnableMemory mirrors the "开启记忆功能" switch in General Settings.
	// nil  = preference never set (treat as feature default = false)
	// *false / *true = user explicitly set the toggle.
	EnableMemory *bool `json:"enable_memory,omitempty"`
}

// Value implements driver.Valuer so GORM persists UserPreferences as
// JSON text (Postgres jsonb column / SQLite TEXT). Empty struct serialises
// to "{}", matching the NOT NULL DEFAULT '{}' column constraint.
func (p UserPreferences) Value() (driver.Value, error) {
	return json.Marshal(p)
}

// Scan implements sql.Scanner so GORM can hydrate UserPreferences back
// from the underlying column. Accept []byte (Postgres jsonb / SQLite blob)
// and string (some drivers hand TEXT as string) for portability.
func (p *UserPreferences) Scan(value interface{}) error {
	if value == nil {
		*p = UserPreferences{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("UserPreferences.Scan: unsupported type")
	}
	if len(data) == 0 {
		*p = UserPreferences{}
		return nil
	}
	return json.Unmarshal(data, p)
}

// User status constants
const (
	UserStatusNormal   = 0 // 正常/在职
	UserStatusBanned   = 1 // 已拉黑
	UserStatusResigned = 2 // 已离职
)

// User provider constants record where an account originated from.
const (
	// UserProviderWhiteList marks accounts pre-created by an administrator
	// through POST /system/admin/users (the whitelist flow).
	UserProviderWhiteList = "white_list"
	// UserProviderWorkday marks accounts provisioned or managed by the
	// Workday directory synchronization.
	UserProviderWorkday = "workday"
)

// Role flag constants
const (
	RoleFlagFalse = 0 // 否
	RoleFlagTrue  = 1 // 是
)

// User represents a user in the system
type User struct {
	// Unique identifier of the user
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// Username of the user
	Username string `json:"username"   gorm:"type:varchar(100);uniqueIndex;not null"`
	// Email address of the user. Nullable since a user may be pre-created from
	// an employee id only; SSO login back-fills the email on first sign-in.
	Email string `json:"email,omitempty" gorm:"type:varchar(255);uniqueIndex"`
	// Hashed password of the user
	PasswordHash string `json:"-"          gorm:"type:varchar(255);not null"`
	// Avatar URL of the user
	Avatar string `json:"avatar"     gorm:"type:varchar(500)"`
	// Whether the user is a system administrator (independent of knowledgeDomain roles)
	IsSystemAdmin bool `json:"is_system_admin" gorm:"default:false;index"`
	// IsKnowledgeDomainAdmin is computed from knowledge_domain_admins for API
	// responses. It is not persisted on users because domain administration is
	// a many-to-many assignment.
	IsKnowledgeDomainAdmin bool `json:"is_knowledge_domain_admin" gorm:"-"`
	// User status: 0=正常/在职, 1=已拉黑, 2=已离职
	Status int `json:"status" gorm:"type:smallint;default:0"`
	// Provider records where the account came from: "white_list" for accounts
	// pre-created by an administrator via the whitelist flow, "workday" for
	// accounts provisioned or managed by the Workday directory sync. Empty
	// for self-registered users.
	Provider string `json:"provider,omitempty" gorm:"type:varchar(32);index"`
	// Time the user was banned
	BannedAt *time.Time `json:"banned_at,omitempty"`
	// Admin who banned the user
	BannedBy *string `json:"banned_by,omitempty" gorm:"type:varchar(36)"`
	// Knowledge officer role: 0=否, 1=是
	RoleKnowledgeOfficer int `json:"role_knowledge_officer" gorm:"column:role_knowledge_officer;type:smallint;default:0"`
	// Employee ID (HR system worker ID).
	EmployeeID string `json:"employee_id,omitempty" gorm:"type:varchar(64);index"`
	// Account / AD account shown on the user-management page.
	Account string `json:"account,omitempty" gorm:"type:varchar(100);index"`
	// English name.
	EnglishName string `json:"english_name,omitempty" gorm:"type:varchar(100)"`
	// Chinese name.
	ChineseName string `json:"chinese_name,omitempty" gorm:"type:varchar(100)"`
	// Department code.
	DepartmentCode string `json:"department_code,omitempty" gorm:"type:varchar(64);index"`
	// Department name.
	DepartmentName string `json:"department_name,omitempty" gorm:"type:varchar(255)"`
	// Per-user UI/feature preferences (memory toggle, future knobs).
	// Stored as JSON (jsonb on Postgres, TEXT on SQLite) via the
	// driver.Valuer / sql.Scanner methods on UserPreferences.
	Preferences UserPreferences `json:"preferences" gorm:"type:jsonb;not null;default:'{}'"`
	// ConfidentialityAcknowledgedAt records when the user accepted the
	// confidentiality acknowledgement. NULL means the user has not yet
	// acknowledged; the Web client is expected to show the acknowledgement
	// dialog until the user confirms, then call POST /auth/me/confidentiality-ack
	// to stamp this timestamp. Once set it is never cleared, so the dialog
	// does not reappear on subsequent logins.
	ConfidentialityAcknowledgedAt *time.Time `json:"confidentiality_acknowledged_at,omitempty" gorm:"index"`
	// Creation time of the user
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the user
	UpdatedAt time.Time `json:"updated_at"`
	// Deletion time of the user
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// IsBanned returns true if the user status is banned.
func (u *User) IsBanned() bool {
	return u.Status == UserStatusBanned
}

// AuthToken represents an authentication token
type AuthToken struct {
	// Unique identifier of the token
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// User ID that owns this token
	UserID string `json:"user_id"    gorm:"type:varchar(36);index;not null"`
	// Token value (JWT or other format)
	Token string `json:"token"      gorm:"type:text;not null"`
	// Token type (access_token, refresh_token)
	TokenType string `json:"token_type" gorm:"type:varchar(50);not null"`
	// Token expiration time
	ExpiresAt time.Time `json:"expires_at"`
	// Whether the token is revoked
	IsRevoked bool `json:"is_revoked" gorm:"default:false"`
	// Creation time of the token
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the token
	UpdatedAt time.Time `json:"updated_at"`
	// LastUsedAt records when the refresh token was last used to rotate a
	// session. Access tokens do not update this field.
	LastUsedAt *time.Time `json:"last_used_at,omitempty" gorm:"type:datetime"`

	// Association relationship
	User *User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type OIDCAuthURLResponse struct {
	Success             bool   `json:"success"`
	ProviderDisplayName string `json:"provider_display_name,omitempty"`
	AuthorizationURL    string `json:"authorization_url,omitempty"`
	State               string `json:"state,omitempty"`
	// Nonce is bound to an HttpOnly cookie on /auth/oidc/url and verified
	// on callback; omitted from JSON so clients cannot replay it alone.
	Nonce string `json:"-"`
}

type OIDCConfigResponse struct {
	Success             bool   `json:"success"`
	Enabled             bool   `json:"enabled"`
	ProviderDisplayName string `json:"provider_display_name,omitempty"`
}

type OIDCCallbackResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	User         *User  `json:"user,omitempty"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IsNewUser    bool   `json:"is_new_user,omitempty"`
}

type OIDCUserInfo struct {
	Subject    string                 `json:"subject,omitempty"`
	Username   string                 `json:"username,omitempty"`
	Email      string                 `json:"email,omitempty"`
	EmployeeID string                 `json:"employee_id,omitempty"`
	Claims     map[string]interface{} `json:"claims,omitempty"`
}

// SAMLAuthURLResponse is returned by GET /auth/saml/url. The browser follows
// AuthorizationURL to the IdP; RelayState is echoed back by the IdP on the
// ACS callback and must be returned unchanged.
type SAMLAuthURLResponse struct {
	Success             bool   `json:"success"`
	ProviderDisplayName string `json:"provider_display_name,omitempty"`
	AuthorizationURL    string `json:"authorization_url,omitempty"`
	// RelayState is the signed round-trip token echoed by the IdP. It is
	// returned in the body (unlike the OIDC nonce) because the IdP, not the
	// browser, reflects it back to the ACS.
	RelayState string `json:"relay_state,omitempty"`
	// Nonce is bound to an HttpOnly cookie on /auth/saml/url and verified on
	// ACS; omitted from JSON so clients cannot replay it alone.
	Nonce string `json:"-"`
}

type SAMLConfigResponse struct {
	Success             bool   `json:"success"`
	Enabled             bool   `json:"enabled"`
	ProviderDisplayName string `json:"provider_display_name,omitempty"`
}

type SAMLCallbackResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	User         *User  `json:"user,omitempty"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IsNewUser    bool   `json:"is_new_user,omitempty"`
}

// SAMLUserInfo carries the identity claims extracted from a validated SAML
// assertion. Attributes is keyed by the configured attribute names.
type SAMLUserInfo struct {
	Subject    string                 `json:"subject,omitempty"`
	Username   string                 `json:"username,omitempty"`
	Email      string                 `json:"email,omitempty"`
	EmployeeID string                 `json:"employee_id,omitempty"`
	Attributes map[string]string      `json:"attributes,omitempty"`
	RawAssert  map[string]interface{} `json:"-"`
}

// RegistrationRole is the development registration selector exposed by the
// email/password flow. system_admin maps only to User.IsSystemAdmin=true.
type RegistrationRole string

const (
	RegistrationRoleViewer      RegistrationRole = "viewer"
	RegistrationRoleSystemAdmin RegistrationRole = "system_admin"
)

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string           `json:"username" binding:"required,min=2,max=50"`
	Email    string           `json:"email"    binding:"required,email"`
	Password string           `json:"password" binding:"required,min=8,max=32"`
	Role     RegistrationRole `json:"role,omitempty" binding:"omitempty,oneof=viewer system_admin"`
	// TrustedRoleAssignment is available only to in-process bootstrap callers;
	// JSON clients cannot set it.
	TrustedRoleAssignment bool `json:"-"`
}

type RegistrationConfigResponse struct {
	Success              bool               `json:"success"`
	PasswordLoginEnabled bool               `json:"password_login_enabled"`
	Enabled              bool               `json:"enabled"`
	RoleSelectionEnabled bool               `json:"role_selection_enabled"`
	DefaultRole          RegistrationRole   `json:"default_role"`
	Roles                []RegistrationRole `json:"roles"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	User         *User  `json:"user,omitempty"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	User    *User  `json:"user,omitempty"`
}

// UserInfo represents user information for API responses
type UserInfo struct {
	ID                     string          `json:"id"`
	Username               string          `json:"username"`
	Email                  string          `json:"email"`
	Avatar                 string          `json:"avatar"`
	IsSystemAdmin          bool            `json:"is_system_admin"`
	IsKnowledgeDomainAdmin bool            `json:"is_knowledge_domain_admin"`
	Status                 int             `json:"status"`
	Provider               string          `json:"provider,omitempty"`
	BannedAt               *time.Time      `json:"banned_at,omitempty"`
	BannedBy               *string         `json:"banned_by,omitempty"`
	RoleKnowledgeOfficer   int             `json:"role_knowledge_officer"`
	EmployeeID             string          `json:"employee_id,omitempty"`
	Account                string          `json:"account,omitempty"`
	EnglishName            string          `json:"english_name,omitempty"`
	ChineseName            string          `json:"chinese_name,omitempty"`
	DepartmentCode         string          `json:"department_code,omitempty"`
	DepartmentName         string          `json:"department_name,omitempty"`
	Preferences            UserPreferences `json:"preferences"`
	ConfidentialityAcknowledgedAt *time.Time `json:"confidentiality_acknowledged_at,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// ToUserInfo converts User to UserInfo (without sensitive data)
func (u *User) ToUserInfo() *UserInfo {
	return &UserInfo{
		ID:                     u.ID,
		Username:               u.Username,
		Email:                  u.Email,
		Avatar:                 u.Avatar,
		IsSystemAdmin:          u.IsSystemAdmin,
		IsKnowledgeDomainAdmin: u.IsKnowledgeDomainAdmin,
		Status:                 u.Status,
		Provider:               u.Provider,
		BannedAt:               u.BannedAt,
		BannedBy:               u.BannedBy,
		RoleKnowledgeOfficer:   u.RoleKnowledgeOfficer,
		EmployeeID:             u.EmployeeID,
		Account:                u.Account,
		EnglishName:            u.EnglishName,
		ChineseName:            u.ChineseName,
		DepartmentCode:               u.DepartmentCode,
		DepartmentName:               u.DepartmentName,
		Preferences:                  u.Preferences,
		ConfidentialityAcknowledgedAt: u.ConfidentialityAcknowledgedAt,
		CreatedAt:                    u.CreatedAt,
		UpdatedAt:                    u.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// User management request / response types
// ---------------------------------------------------------------------------

// BanUserRequest represents a request to ban a user
type BanUserRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// UnbanUserRequest represents a request to unban a user
type UnbanUserRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// OfflineUserRequest represents a request to force a user offline
type OfflineUserRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// UserBanActionResponse is the unified response for user ban/unban actions.
// Status is either "success" or "failed"; Code mirrors the HTTP status code.
type UserBanActionResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ActionResponse is the generic code/message/status response for simple
// admin action endpoints (role updates etc.).
// Status is either "success" or "failed"; Code mirrors the HTTP status code.
type ActionResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// KnowledgeOfficerRolesPatch represents a partial update of the knowledge
// officer role. It is handled by the dedicated knowledge-officer endpoints
// (/users/roles and /users/roles/single) and never touches the operator
// role — operator changes are made through the separate operator endpoints.
type KnowledgeOfficerRolesPatch struct {
	// RoleKnowledgeOfficer is 0 or 1; nil means don't change
	RoleKnowledgeOfficer *int `json:"role_knowledge_officer"`
	// KnowledgeDomainID is required when setting role_knowledge_officer=1
	KnowledgeDomainID *uint64 `json:"knowledge_domain_id,omitempty"`
	// KnowledgeBaseIDs are the knowledge bases the officer can manage;
	// required when role_knowledge_officer=1
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
}

// OperatorRolesPatch represents a partial update of the operator role.
// It is handled by the dedicated operator endpoints (/users/roles/operator
// and /users/roles/operator/single) and never touches the knowledge officer
// role or any knowledge-base binding.
//
// The users table no longer has a role_operator column: setting the operator
// role to 1 maps to is_system_admin=true and 0 maps to is_system_admin=false.
type OperatorRolesPatch struct {
	// RoleOperator is 0 or 1; nil means don't change.
	// 1 = grant operator (is_system_admin=true), 0 = revoke (is_system_admin=false).
	RoleOperator *int `json:"role_operator"`
}

// UpdateUserRolesRequest represents a single-user knowledge-officer role
// update request.
type UpdateUserRolesRequest struct {
	UserID string                     `json:"user_id" binding:"required"`
	Roles  KnowledgeOfficerRolesPatch `json:"roles"   binding:"required"`
}

// BatchUpdateUserRolesRequest represents a batch knowledge-officer role
// update request.
type BatchUpdateUserRolesRequest struct {
	UserIDs []string                   `json:"user_ids" binding:"required,min=1"`
	Roles   KnowledgeOfficerRolesPatch `json:"roles"    binding:"required"`
}

// UpdateOperatorRoleRequest represents a single-user operator role update
// request.
type UpdateOperatorRoleRequest struct {
	UserID string            `json:"user_id" binding:"required"`
	Roles  OperatorRolesPatch `json:"roles"   binding:"required"`
}

// BatchUpdateOperatorRoleRequest represents a batch operator role update
// request.
type BatchUpdateOperatorRoleRequest struct {
	UserIDs []string          `json:"user_ids" binding:"required,min=1"`
	Roles   OperatorRolesPatch `json:"roles"    binding:"required"`
}

// ListUsersFilter represents the filters supported by the user-management page.
type ListUsersFilter struct {
	// Account filters by users.account (case-insensitive prefix/like).
	Account string `form:"account"`
	// EmployeeName filters by english_name or chinese_name (case-insensitive like).
	EmployeeName string `form:"employee_name"`
	// IsKnowledgeOfficer filters by role_knowledge_officer (0 or 1).
	IsKnowledgeOfficer *int `form:"is_knowledge_officer"`
	// IsOperator filters by operator status, which is stored as
	// is_system_admin (1 = operator, 0 = not).
	IsOperator *int `form:"is_operator"`
	// Department filters by department_code or department_name (case-insensitive like).
	Department string `form:"department"`
	// Status filters by users.status: 0=normal/active, 1=banned, 2=resigned.
	Status *int `form:"status"`
	// Query is a free-text search across username, email, account, employee_id,
	// english_name and chinese_name.
	Query string `form:"query"`
}

// ListUsersRequest represents paginated user listing.
type ListUsersRequest struct {
	Offset int             `form:"offset"`
	Limit  int             `form:"limit"`
	Filter ListUsersFilter `form:"filter"`
}

// ListUsersResponse represents a paginated list of users
type ListUsersResponse struct {
	Total int64       `json:"total"`
	Users []*UserInfo `json:"users"`
}

// CreateUserRequest represents a request for an administrator to pre-create a
// local user. The admin provides a single identifier in `id_or_email`; the
// server decides whether it is an email address (stored in users.email) or an
// employee ID (stored in users.employee_id) and persists it into the
// corresponding column. The created account is tagged provider="white_list".
// username, password and remaining profile fields are derived/generated
// server-side.
type CreateUserRequest struct {
	// IDOrEmail is the single identifier entered by the admin. An input
	// containing '@' or '.com' is treated as an email address (users.email);
	// anything else is stored as users.employee_id. The other column remains
	// NULL until the user logs in via SSO and back-fills the missing fields.
	IDOrEmail string `json:"id_or_email" binding:"required,max=255"`
}

// CreateUserResponse represents the response after an administrator creates a user.
// It reports the action status only; no user object is included.
type CreateUserResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ExternalWorkerImportStatus describes the outcome of one worker during a
// batch import from external_workers into users.
type ExternalWorkerImportStatus string

const (
	ExternalWorkerImportCreated ExternalWorkerImportStatus = "created"
	ExternalWorkerImportSkipped ExternalWorkerImportStatus = "skipped"
)

// ExternalWorkerImportResult describes the outcome of a single external worker
// during batch import.
type ExternalWorkerImportResult struct {
	ExternalWorkerID string                     `json:"external_worker_id"`
	Email            string                     `json:"email"`
	Status           ExternalWorkerImportStatus `json:"status"`
	Reason           string                     `json:"reason,omitempty"`
	UserID           string                     `json:"user_id,omitempty"`
}

// CreateUsersBatchResponse is returned by the batch-import mode of
// POST /system/admin/users when external_worker_ids is provided.
type CreateUsersBatchResponse struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message,omitempty"`
	Total   int                          `json:"total"`
	Created int                          `json:"created"`
	Skipped int                          `json:"skipped"`
	Users   []*UserInfo                  `json:"users,omitempty"`
	Results []ExternalWorkerImportResult `json:"results,omitempty"`
}

// UserKnowledgeScope describes the knowledge-domain admin scope of a user.
// It groups one knowledge domain with the list of knowledge bases that
// belong to that domain.
type UserKnowledgeScope struct {
	KnowledgeDomainID   uint64   `json:"knowledge_domain_id"`
	KnowledgeDomainName string   `json:"knowledge_domain_name"`
	KnowledgeBaseNames  []string `json:"knowledge_base_names"`
}

// UserRefreshToken describes a single refresh token for the user detail page.
type UserRefreshToken struct {
	JTI        string     `json:"jti"`
	Status     string     `json:"status"`
	IssuedAt   time.Time  `json:"issued_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

// UserDetailResponse is returned by GET /system/admin/users/:id and contains
// everything shown on the user management detail page: basic profile, role
// flags, knowledge-domain admin scope, and active refresh tokens.
type UserDetailResponse struct {
	ID                     string               `json:"id"`
	Username               string               `json:"username"`
	Email                  string               `json:"email"`
	Avatar                 string               `json:"avatar"`
	Status                 int                  `json:"status"`
	Provider               string               `json:"provider,omitempty"`
	IsSystemAdmin          bool                 `json:"is_system_admin"`
	IsKnowledgeDomainAdmin bool                 `json:"is_knowledge_domain_admin"`
	RoleKnowledgeOfficer   int                  `json:"role_knowledge_officer"`
	EmployeeID             string               `json:"employee_id,omitempty"`
	Account                string               `json:"account,omitempty"`
	EnglishName            string               `json:"english_name,omitempty"`
	ChineseName            string               `json:"chinese_name,omitempty"`
	DepartmentCode         string               `json:"department_code,omitempty"`
	DepartmentName         string               `json:"department_name,omitempty"`
	KnowledgeScopes        []UserKnowledgeScope `json:"knowledge_scopes"`
	RefreshTokens          []UserRefreshToken   `json:"refresh_tokens"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Blacklist types
// ---------------------------------------------------------------------------

// BlacklistEntry represents a blacklisted user entry.
// When a user is banned, a row is inserted into this table;
// when unbanned, the row is removed.
// This table is checked at every layer (login, token refresh, middleware)
// as a defense-in-depth measure.
type BlacklistEntry struct {
	// Unique identifier of the blacklist entry
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// User ID that is blacklisted (unique constraint)
	UserID string `json:"user_id"    gorm:"type:varchar(36);uniqueIndex;not null"`
	// Time the user was banned
	BannedAt time.Time `json:"banned_at"`
	// Admin who banned the user
	BannedBy string `json:"banned_by,omitempty" gorm:"type:varchar(36)"`
	// Optional reason for banning
	Reason string `json:"reason,omitempty" gorm:"type:text;default:''"`
	// Creation time of the entry
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the entry
	UpdatedAt time.Time `json:"updated_at"`
}

