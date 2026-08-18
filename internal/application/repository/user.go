package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrTokenNotFound      = errors.New("token not found")
	ErrCannotRevokeSelf   = errors.New("cannot revoke your own system admin privileges")
	ErrLastSystemAdmin    = errors.New("cannot revoke the last remaining system administrator")
	ErrUserNotSystemAdmin = errors.New("user is not a system administrator")
)

// userRepository implements user repository interface
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *gorm.DB) interfaces.UserRepository {
	return &userRepository{db: db}
}

// CreateUser creates a user
func (r *userRepository) CreateUser(ctx context.Context, user *types.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// GetUserByID gets a user by ID
func (r *userRepository) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUsersByIDs batch-fetches users by id with a single SELECT … WHERE id IN (…)
// and projects the result into a map keyed by user id. Returns an empty
// map for an empty input slice. Missing ids are silently absent from
// the result (consistent with the interface contract used by knowledgeDomain
// member hydration).
func (r *userRepository) GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error) {
	out := make(map[string]*types.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var users []*types.User
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

// GetUserByEmail gets a user by email
func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByEmployeeID gets a user by employee id.
func (r *userRepository) GetUserByEmployeeID(ctx context.Context, employeeID string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("employee_id = ?", employeeID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername gets a user by username
func (r *userRepository) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// UpdateUser updates a user
func (r *userRepository) UpdateUser(ctx context.Context, user *types.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// DeleteUser deletes a user
func (r *userRepository) DeleteUser(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.User{}).Error
}

// ListUsers lists all users with optional filters, paginated.
func (r *userRepository) ListUsers(ctx context.Context, offset, limit int, filter types.ListUsersFilter) ([]*types.User, int64, error) {
	var users []*types.User
	var total int64

	base := r.db.WithContext(ctx).Model(&types.User{})

	if filter.Account != "" {
		base = base.Where("LOWER(account) LIKE ?", "%"+strings.ToLower(filter.Account)+"%")
	}
	if filter.EmployeeName != "" {
		like := "%" + strings.ToLower(filter.EmployeeName) + "%"
		base = base.Where("LOWER(english_name) LIKE ? OR LOWER(chinese_name) LIKE ?", like, like)
	}
	if filter.IsKnowledgeOfficer != nil {
		base = base.Where("role_knowledge_officer = ?", *filter.IsKnowledgeOfficer)
	}
	if filter.IsOperator != nil {
		base = base.Where("is_system_admin = ?", *filter.IsOperator == 1)
	}
	if filter.Department != "" {
		like := "%" + strings.ToLower(filter.Department) + "%"
		base = base.Where("LOWER(department_code) LIKE ? OR LOWER(department_name) LIKE ?", like, like)
	}
	if filter.Status != nil {
		base = base.Where("status = ?", *filter.Status)
	}
	if filter.Query != "" {
		like := "%" + strings.ToLower(filter.Query) + "%"
		base = base.Where(
			"LOWER(username) LIKE ? OR LOWER(email) LIKE ? OR LOWER(account) LIKE ? OR LOWER(employee_id) LIKE ? OR LOWER(english_name) LIKE ? OR LOWER(chinese_name) LIKE ?",
			like, like, like, like, like, like,
		)
	}

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := base.Order("created_at DESC, id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// ListSystemAdmins lists users where is_system_admin = true.
//
// Walks idx_users_is_system_admin (created in migration 000052), so the
// query stays cheap even on a large users table — only the small subset
// of system admins is scanned. Returns total count alongside the page so
// the management UI can render pagination without a second roundtrip.
//
// Ordered by created_at DESC for stable, newest-first listing; ties are
// further broken by id to keep paging deterministic across boundaries.
// limit <= 0 means "no limit" (matches ListUsers semantics); callers in
// production pass a sane page size.
func (r *userRepository) ListSystemAdmins(ctx context.Context, offset, limit int) ([]*types.User, int64, error) {
	var users []*types.User
	var total int64

	base := r.db.WithContext(ctx).Model(&types.User{}).Where("is_system_admin = ?", true)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := base.Order("created_at DESC, id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if err := query.Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// RevokeSystemAdmin revokes system-admin privileges inside a transaction.
// It locks the current admin rows before counting so concurrent revokes
// cannot both observe "two admins" and leave the platform with zero.
//
// Return contract:
//   - (user, nil): revoke actually happened; user.IsSystemAdmin == false
//   - (user, ErrUserNotSystemAdmin): target was already not an admin;
//     no row was written. Caller should treat as idempotent success but
//     MUST distinguish it from a real revoke for audit purposes — the
//     surfaced `user` is the unchanged DB row.
//   - (nil, ErrCannotRevokeSelf | ErrLastSystemAdmin | ErrUserNotFound | …):
//     hard rejection; no row written.
func (r *userRepository) RevokeSystemAdmin(ctx context.Context, userID, actorID string) (*types.User, error) {
	if userID == actorID {
		return nil, ErrCannotRevokeSelf
	}

	var revoked *types.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locking := func(db *gorm.DB) *gorm.DB {
			switch tx.Dialector.Name() {
			case "postgres", "mysql":
				return db.Clauses(clause.Locking{Strength: "UPDATE"})
			default:
				return db
			}
		}
		var user types.User
		if err := locking(tx).
			Where("id = ?", userID).
			First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if !user.IsSystemAdmin {
			revoked = &user
			return ErrUserNotSystemAdmin
		}

		var admins []types.User
		if err := locking(tx).
			Where("is_system_admin = ?", true).
			Find(&admins).Error; err != nil {
			return err
		}
		if len(admins) <= 1 {
			return ErrLastSystemAdmin
		}

		user.IsSystemAdmin = false
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		revoked = &user
		return nil
	})
	// Propagate ErrUserNotSystemAdmin up to the handler alongside the
	// (unchanged) user row. The handler treats it as idempotent success
	// but emits an audit row with changed=false so a probing pattern
	// ("revoke every random user id we know") still leaves a trail.
	if errors.Is(err, ErrUserNotSystemAdmin) {
		return revoked, err
	}
	if err != nil {
		return nil, err
	}
	return revoked, nil
}

// SearchUsers searches users by username or email
func (r *userRepository) SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error) {
	var users []*types.User
	like := "%" + strings.ToLower(query) + "%"

	dbQuery := r.db.WithContext(ctx).
		Where("LOWER(username) LIKE ? OR LOWER(email) LIKE ?", like, like).
		Where("status = ?", types.UserStatusNormal).
		Order("username ASC")

	if limit > 0 {
		dbQuery = dbQuery.Limit(limit)
	} else {
		dbQuery = dbQuery.Limit(20) // default limit
	}

	if err := dbQuery.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// BatchUpdateUserRoles updates role_knowledge_officer for a batch of users.
// It only ever writes the knowledge-officer flag — the operator role is
// managed by the dedicated operator endpoints (BatchUpdateOperatorRole /
// UpdateOperatorRole) and is never modified here.
func (r *userRepository) BatchUpdateUserRoles(ctx context.Context, userIDs []string, patch types.KnowledgeOfficerRolesPatch) (int64, error) {
	updates := map[string]interface{}{}
	if patch.RoleKnowledgeOfficer != nil {
		updates["role_knowledge_officer"] = *patch.RoleKnowledgeOfficer
	}
	if len(updates) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Model(&types.User{}).
		Where("id IN ?", userIDs).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// UpdateUserRoles updates role_knowledge_officer for a single user.
// It only ever writes the knowledge-officer flag — the operator role is
// managed by the dedicated operator endpoints and is never modified here.
func (r *userRepository) UpdateUserRoles(ctx context.Context, userID string, patch types.KnowledgeOfficerRolesPatch) (int64, error) {
	updates := map[string]interface{}{}
	if patch.RoleKnowledgeOfficer != nil {
		updates["role_knowledge_officer"] = *patch.RoleKnowledgeOfficer
	}
	if len(updates) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Model(&types.User{}).
		Where("id = ?", userID).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// BatchUpdateOperatorRole grants/revokes the operator role for a batch of
// users by setting is_system_admin (1 = true, 0 = false). The users table no
// longer has a role_operator column. It only ever writes the operator flag —
// the knowledge-officer role and its knowledge-base bindings are never
// modified here.
func (r *userRepository) BatchUpdateOperatorRole(ctx context.Context, userIDs []string, patch types.OperatorRolesPatch) (int64, error) {
	updates := map[string]interface{}{}
	if patch.RoleOperator != nil {
		updates["is_system_admin"] = *patch.RoleOperator == 1
	}
	if len(updates) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Model(&types.User{}).
		Where("id IN ?", userIDs).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// UpdateOperatorRole grants/revokes the operator role for a single user by
// setting is_system_admin (1 = true, 0 = false). It only ever writes the
// operator flag — the knowledge-officer role and its knowledge-base bindings
// are never modified here.
func (r *userRepository) UpdateOperatorRole(ctx context.Context, userID string, patch types.OperatorRolesPatch) (int64, error) {
	updates := map[string]interface{}{}
	if patch.RoleOperator != nil {
		updates["is_system_admin"] = *patch.RoleOperator == 1
	}
	if len(updates) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Model(&types.User{}).
		Where("id = ?", userID).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// ---------------------------------------------------------------------------
// Workday worker batch-import helpers (used by POST /system/admin/users with
// external_worker_ids). The user table and the external_workers projection are
// managed by the same database, so these live here instead of injecting an
// extra repository into the user service.
// ---------------------------------------------------------------------------

// GetWorkdayWorkersByIDs loads external_workers by external_worker_id for a
// given provider. Unknown ids are silently absent from the result.
func (r *userRepository) GetWorkdayWorkersByIDs(ctx context.Context, provider string, ids []string) ([]*types.ExternalWorker, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var workers []*types.ExternalWorker
	err := r.db.WithContext(ctx).
		Where("provider = ? AND external_worker_id IN ?", strings.TrimSpace(provider), ids).
		Find(&workers).Error
	return workers, err
}

// GetUsersByEmails batch-fetches users by email (case-insensitive), returning
// a map keyed by the lower-cased email. Only non-deleted users are returned.
func (r *userRepository) GetUsersByEmails(ctx context.Context, emails []string) (map[string]*types.User, error) {
	out := make(map[string]*types.User)
	if len(emails) == 0 {
		return out, nil
	}
	normalized := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return out, nil
	}
	var users []*types.User
	if err := r.db.WithContext(ctx).
		Where("LOWER(email) IN ? AND deleted_at IS NULL", normalized).
		Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[strings.ToLower(u.Email)] = u
	}
	return out, nil
}

// ResolveWorkdayOrgUnit resolves the canonical org_units row (id/code/name)
// behind an external organization id. Missing projections or orgs return empty
// values without an error so a worker can still be imported without a known
// department.
func (r *userRepository) ResolveWorkdayOrgUnit(ctx context.Context, provider, externalOrgID string) (orgUnitID, code, name string, err error) {
	if strings.TrimSpace(externalOrgID) == "" {
		return "", "", "", nil
	}
	var projection types.ExternalOrgUnit
	err = r.db.WithContext(ctx).
		Where("provider = ? AND external_org_id = ? AND status = ?",
			strings.TrimSpace(provider), externalOrgID, types.OrgUnitStatusActive).
		First(&projection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || projection.OrgUnitID == nil {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	var unit types.OrgUnit
	err = r.db.WithContext(ctx).Where("id = ?", *projection.OrgUnitID).First(&unit).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	return unit.ID, unit.Code, unit.Name, nil
}

// CreateWorkdayLinkedUser creates a platform user inside a transaction,
// back-fills external_workers.user_id and establishes the user's primary
// workday org membership (when an org unit id is available). The transaction
// keeps the three writes atomic: a user never exists without its worker link.
func (r *userRepository) CreateWorkdayLinkedUser(ctx context.Context, user *types.User, worker *types.ExternalWorker, orgUnitID string) error {
	if worker == nil {
		return errors.New("worker is required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Model(&types.ExternalWorker{}).
			Where("id = ?", worker.ID).
			Update("user_id", user.ID).Error; err != nil {
			return err
		}
		if strings.TrimSpace(orgUnitID) != "" {
			now := time.Now()
			membership := &types.UserOrgMembership{
				UserID:    user.ID,
				OrgUnitID: orgUnitID,
				IsPrimary: true,
				Status:    types.OrgUnitStatusActive,
				Source:    types.OrgUnitSourceWorkday,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "org_unit_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"is_primary", "status", "source", "updated_at",
				}),
			}).Create(membership).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// authTokenRepository implements auth token repository interface
type authTokenRepository struct {
	db *gorm.DB
}

// NewAuthTokenRepository creates a new auth token repository
func NewAuthTokenRepository(db *gorm.DB) interfaces.AuthTokenRepository {
	return &authTokenRepository{db: db}
}

// CreateToken creates an auth token
func (r *authTokenRepository) CreateToken(ctx context.Context, token *types.AuthToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// GetTokenByValue gets a token by its value
func (r *authTokenRepository) GetTokenByValue(ctx context.Context, tokenValue string) (*types.AuthToken, error) {
	var token types.AuthToken
	if err := r.db.WithContext(ctx).Where("token = ?", tokenValue).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &token, nil
}

// GetTokensByUserID gets all tokens for a user
func (r *authTokenRepository) GetTokensByUserID(ctx context.Context, userID string) ([]*types.AuthToken, error) {
	var tokens []*types.AuthToken
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// UpdateToken updates a token
func (r *authTokenRepository) UpdateToken(ctx context.Context, token *types.AuthToken) error {
	return r.db.WithContext(ctx).Save(token).Error
}

// DeleteToken deletes a token
func (r *authTokenRepository) DeleteToken(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.AuthToken{}).Error
}

// DeleteExpiredTokens deletes all expired tokens
func (r *authTokenRepository) DeleteExpiredTokens(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < NOW()").Delete(&types.AuthToken{}).Error
}

// RevokeTokensByUserID revokes all tokens for a user
func (r *authTokenRepository) RevokeTokensByUserID(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Model(&types.AuthToken{}).Where("user_id = ?", userID).Update("is_revoked", true).Error
}
