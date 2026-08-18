package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type enterpriseIntegrationRepository struct {
	db *gorm.DB
}

func NewEnterpriseIntegrationRepository(db *gorm.DB) interfaces.EnterpriseIntegrationRepository {
	return &enterpriseIntegrationRepository{db: db}
}

func (r *enterpriseIntegrationRepository) CreateSyncRun(
	ctx context.Context,
	run *types.IntegrationSyncRun,
) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *enterpriseIntegrationRepository) GetSyncRun(
	ctx context.Context,
	runID string,
) (*types.IntegrationSyncRun, error) {
	var run types.IntegrationSyncRun
	err := r.db.WithContext(ctx).Where("id = ?", runID).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &run, err
}

func (r *enterpriseIntegrationRepository) ListSyncRuns(
	ctx context.Context,
	provider string,
	offset, limit int,
) ([]*types.IntegrationSyncRun, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := r.db.WithContext(ctx).Model(&types.IntegrationSyncRun{})
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []*types.IntegrationSyncRun
	err := query.Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&runs).Error
	return runs, total, err
}

func (r *enterpriseIntegrationRepository) ListExternalOrgUnits(
	ctx context.Context,
	provider string,
) ([]*types.ExternalOrgUnit, error) {
	var units []*types.ExternalOrgUnit
	err := r.db.WithContext(ctx).
		Where("provider = ?", strings.TrimSpace(provider)).
		Order("name ASC, external_org_id ASC").
		Find(&units).Error
	return units, err
}

func (r *enterpriseIntegrationRepository) ListExternalWorkers(
	ctx context.Context,
	provider, orgExternalID, search string,
	offset, limit int,
) ([]*types.ExternalWorker, int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	query := r.db.WithContext(ctx).Model(&types.ExternalWorker{}).
		Where("provider = ?", strings.TrimSpace(provider))
	if orgExternalID = strings.TrimSpace(orgExternalID); orgExternalID != "" {
		query = query.Where("primary_org_external_id = ?", orgExternalID)
	}
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"LOWER(corporate_email) LIKE LOWER(?) OR LOWER(external_worker_id) LIKE LOWER(?)",
			like,
			like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var workers []*types.ExternalWorker
	err := query.
		Order("corporate_email ASC, external_worker_id ASC").
		Offset(offset).
		Limit(limit).
		Find(&workers).Error
	return workers, total, err
}

func (r *enterpriseIntegrationRepository) LatestSuccessfulCursor(
	ctx context.Context,
	provider, connectionKey string,
) (types.JSON, error) {
	var run types.IntegrationSyncRun
	err := r.db.WithContext(ctx).
		Where(
			"provider = ? AND connection_key = ? AND status = ?",
			provider,
			connectionKey,
			types.IntegrationSyncSucceeded,
		).
		Order("created_at DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return jsonValue(map[string]string{}), nil
	}
	return run.CursorAfter, err
}

func (r *enterpriseIntegrationRepository) MarkSyncRunRunning(
	ctx context.Context,
	runID string,
) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&types.IntegrationSyncRun{}).
		Where("id = ? AND status <> ?", runID, types.IntegrationSyncSucceeded).
		Updates(map[string]any{
			"status":     types.IntegrationSyncRunning,
			"started_at": gorm.Expr("COALESCE(started_at, ?)", now),
		}).Error
}

func (r *enterpriseIntegrationRepository) ApplyOrgUnitPage(
	ctx context.Context,
	runID, provider string,
	items []types.WorkdayOrgUnitRecord,
	nextCursor string,
) (*types.WorkdaySyncCounters, error) {
	counters := &types.WorkdaySyncCounters{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for i := range items {
			item := normalizeOrgRecord(items[i])
			if item.ExternalID == "" || item.Name == "" {
				return fmt.Errorf("Workday organization requires external_id and name")
			}
			counters.OrgUnitsSeen++
			checksum, attributes, err := integrationRecordChecksum(item)
			if err != nil {
				return err
			}

			var projection types.ExternalOrgUnit
			err = tx.Where(
				"provider = ? AND external_org_id = ?",
				provider,
				item.ExternalID,
			).First(&projection).Error
			isNew := errors.Is(err, gorm.ErrRecordNotFound)
			if err != nil && !isNew {
				return err
			}
			if isNew {
				projection.ID = uuid.NewString()
				canonicalID := uuid.NewString()
				projection.OrgUnitID = &canonicalID
			}
			if isNew || projection.Checksum != checksum {
				counters.OrgUnitsChanged++
			}
			if projection.OrgUnitID == nil || *projection.OrgUnitID == "" {
				canonicalID := uuid.NewString()
				projection.OrgUnitID = &canonicalID
			}

			externalID := item.ExternalID
			canonical := &types.OrgUnit{
				ID:         *projection.OrgUnitID,
				Code:       item.Code,
				Name:       item.Name,
				Status:     item.Status,
				Source:     types.OrgUnitSourceWorkday,
				ExternalID: &externalID,
				Attributes: attributes,
			}
			if canonical.Code == "" {
				canonical.Code = "workday:" + item.ExternalID
			}
			if err := upsertWorkdayOrgUnit(tx, canonical); err != nil {
				return err
			}

			projection.Provider = provider
			projection.ExternalOrgID = item.ExternalID
			projection.ParentExternalOrgID = optionalString(item.ParentExternalID)
			projection.Name = item.Name
			projection.OrgType = item.OrgType
			projection.Status = item.Status
			projection.Attributes = attributes
			projection.Checksum = checksum
			projection.EffectiveFrom = item.EffectiveFrom
			projection.EffectiveTo = item.EffectiveTo
			projection.LastSeenAt = now
			projection.UpdatedAt = now
			if isNew {
				projection.CreatedAt = now
				if err := tx.Create(&projection).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&types.ExternalOrgUnit{}).
				Where("id = ?", projection.ID).
				Updates(projection).Error; err != nil {
				return err
			}
		}

		// Reconcile every projection after each page. Workday may return a child
		// before its parent on a later page.
		var projections []types.ExternalOrgUnit
		if err := tx.Where("provider = ?", provider).Find(&projections).Error; err != nil {
			return err
		}
		for i := range projections {
			child := projections[i]
			var parentID *string
			if child.ParentExternalOrgID != nil &&
				strings.TrimSpace(*child.ParentExternalOrgID) != "" {
				var parent types.ExternalOrgUnit
				err := tx.Where(
					"provider = ? AND external_org_id = ?",
					provider,
					*child.ParentExternalOrgID,
				).First(&parent).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if err == nil {
					parentID = parent.OrgUnitID
				}
			}
			if child.OrgUnitID != nil {
				if err := tx.Model(&types.OrgUnit{}).
					Where("id = ?", *child.OrgUnitID).
					Update("parent_id", parentID).Error; err != nil {
					return err
				}
			}
		}
		return updateRunCursor(tx, runID, "org_units", nextCursor)
	})
	return counters, err
}

func normalizeOrgRecord(item types.WorkdayOrgUnitRecord) types.WorkdayOrgUnitRecord {
	item.ExternalID = strings.TrimSpace(item.ExternalID)
	item.ParentExternalID = strings.TrimSpace(item.ParentExternalID)
	item.Code = strings.TrimSpace(item.Code)
	item.Name = strings.TrimSpace(item.Name)
	item.OrgType = strings.TrimSpace(item.OrgType)
	if !item.Status.IsValid() {
		item.Status = types.OrgUnitStatusActive
	}
	if item.Attributes == nil {
		item.Attributes = map[string]any{}
	}
	return item
}

func upsertWorkdayOrgUnit(tx *gorm.DB, unit *types.OrgUnit) error {
	now := time.Now()
	unit.UpdatedAt = now
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"code", "name", "status", "source", "external_id", "attributes", "updated_at",
		}),
	}).Create(unit).Error
}

func (r *enterpriseIntegrationRepository) ApplyWorkerPage(
	ctx context.Context,
	runID, provider string,
	items []types.WorkdayWorkerRecord,
	nextCursor string,
) (*types.WorkdaySyncCounters, error) {
	counters := &types.WorkdaySyncCounters{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for i := range items {
			item := normalizeWorkerRecord(items[i])
			if item.ExternalID == "" {
				return fmt.Errorf("Workday worker requires external_id")
			}
			counters.WorkersSeen++
			checksum, attributes, err := integrationRecordChecksum(item)
			if err != nil {
				return err
			}

			var projection types.ExternalWorker
			err = tx.Where(
				"provider = ? AND external_worker_id = ?",
				provider,
				item.ExternalID,
			).First(&projection).Error
			isNew := errors.Is(err, gorm.ErrRecordNotFound)
			if err != nil && !isNew {
				return err
			}
			if isNew {
				projection.ID = uuid.NewString()
			}
			if isNew || projection.Checksum != checksum {
				counters.WorkersChanged++
			}

		userID := projection.UserID
		if userID == nil && item.CorporateEmail != "" {
			var user types.User
			err := tx.Where(
				"LOWER(email) = ? AND deleted_at IS NULL",
				strings.ToLower(item.CorporateEmail),
			).First(&user).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				userID = &user.ID
			} else {
				// The directory is the source of truth: auto-provision the
				// platform account for active workers (status 0) and for
				// terminated workers (status 2) alike, so every external
				// worker is represented in users. The random unusable
				// password keeps authentication on SSO only.
				provisioned, provisionErr := upsertWorkdayUser(tx, item)
				if provisionErr != nil {
					return provisionErr
				}
				if provisioned != nil {
					userID = &provisioned.ID
				}
			}
		}
		// Keep the linked user's profile and employment status in sync with
		// the directory on every run (this is a full sync), except for
		// platform-banned users (status=1), which must never be overwritten
		// by directory data.
		if userID != nil {
			if refreshErr := refreshWorkdayUser(tx, *userID, item); refreshErr != nil {
				return refreshErr
			}
		}

			projection.Provider = provider
			projection.ExternalWorkerID = item.ExternalID
			projection.UserID = userID
			projection.PrimaryOrgExternalID = optionalString(item.PrimaryOrgExternalID)
			projection.ManagerExternalWorkerID = optionalString(item.ManagerExternalWorkerID)
			projection.CorporateEmail = item.CorporateEmail
			projection.WorkerStatus = item.Status
			projection.Attributes = attributes
			projection.Checksum = checksum
			projection.EffectiveFrom = item.EffectiveFrom
			projection.EffectiveTo = item.EffectiveTo
			projection.LastSeenAt = now
			projection.UpdatedAt = now
			if isNew {
				projection.CreatedAt = now
				if err := tx.Create(&projection).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&types.ExternalWorker{}).
				Where("id = ?", projection.ID).
				Updates(projection).Error; err != nil {
				return err
			}

			if userID == nil {
				counters.UnmatchedWorkers++
				continue
			}
			counters.WorkersLinked++
			changed, err := applyWorkdayMembership(tx, provider, *userID, item)
			if err != nil {
				return err
			}
			if changed {
				counters.MembershipsChanged++
			}
		}
		return updateRunCursor(tx, runID, "workers", nextCursor)
	})
	return counters, err
}

func normalizeWorkerRecord(item types.WorkdayWorkerRecord) types.WorkdayWorkerRecord {
	item.ExternalID = strings.TrimSpace(item.ExternalID)
	item.PrimaryOrgExternalID = strings.TrimSpace(item.PrimaryOrgExternalID)
	item.ManagerExternalWorkerID = strings.TrimSpace(item.ManagerExternalWorkerID)
	item.CorporateEmail = strings.ToLower(strings.TrimSpace(item.CorporateEmail))
	switch item.Status {
	case types.ExternalWorkerActive, types.ExternalWorkerInactive, types.ExternalWorkerLeave:
	default:
		item.Status = types.ExternalWorkerActive
	}
	if item.Attributes == nil {
		item.Attributes = map[string]any{}
	}
	return item
}

// upsertWorkdayUser auto-provisions a platform user for a Workday/Roche worker
// that is not yet linked to an existing user (directory data arrives before
// the employee's first SSO login). Active workers get status 0 (normal) and
// terminated workers status 2 (resigned); the account is created with a
// random, unusable password so authentication only happens through SSO. The
// directory attributes (employee id, account, names, department) are copied
// onto the user profile. A nil user is returned without error when no usable
// email is present.
func upsertWorkdayUser(tx *gorm.DB, item types.WorkdayWorkerRecord) (*types.User, error) {
	email := strings.ToLower(strings.TrimSpace(item.CorporateEmail))
	if email == "" {
		return nil, nil
	}
	attr := item.Attributes
	account := attrString(attr, "user_id")
	username := sanitizeWorkdayUsername(account)
	if username == "" {
		username = sanitizeWorkdayUsername(strings.SplitN(email, "@", 2)[0])
	}
	if username == "" {
		username = "workday-user"
	}
	username = truncateWorkdayUsername(username)
	if err := ensureUniqueWorkdayUsername(tx, &username); err != nil {
		return nil, err
	}

	randomPassword, err := generateRandomPassword(32)
	if err != nil {
		return nil, fmt.Errorf("generate Workday user password: %w", err)
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash Workday user password: %w", err)
	}

	now := time.Now()
	userStatus := types.UserStatusNormal
	if !item.Status.IsActive() {
		userStatus = types.UserStatusResigned
	}
	user := &types.User{
		ID:             uuid.NewString(),
		Username:       username,
		Email:          email,
		PasswordHash:   string(hashedPassword),
		EmployeeID:     attrString(attr, "employee_id"),
		Account:        account,
		EnglishName:    attrString(attr, "display_name"),
		DepartmentCode: attrString(attr, "supervisory_code"),
		DepartmentName: attrString(attr, "supervisory_name"),
		Provider:       types.UserProviderWorkday,
		Status:         userStatus,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := tx.Create(user).Error; err != nil {
		return nil, fmt.Errorf("create Workday user: %w", err)
	}
	return user, nil
}

// refreshWorkdayUser aligns the linked user's directory-driven fields and
// employment status with the worker record on every sync: active workers map
// to status 0 (normal) and terminated workers to status 2 (resigned).
// Platform-banned users (status=1) are returned untouched — a local ban must
// never be overridden by directory data.
func refreshWorkdayUser(tx *gorm.DB, userID string, item types.WorkdayWorkerRecord) error {
	var user types.User
	err := tx.Where("id = ? AND deleted_at IS NULL", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if user.Status == types.UserStatusBanned {
		return nil
	}
	attr := item.Attributes
	status := types.UserStatusNormal
	if !item.Status.IsActive() {
		status = types.UserStatusResigned
	}
	return tx.Model(&types.User{}).
		Where("id = ? AND deleted_at IS NULL", userID).
		Updates(map[string]any{
			"status":          status,
			"employee_id":     attrString(attr, "employee_id"),
			"account":         attrString(attr, "user_id"),
			"english_name":    attrString(attr, "display_name"),
			"department_code": attrString(attr, "supervisory_code"),
			"department_name": attrString(attr, "supervisory_name"),
			"provider":        types.UserProviderWorkday,
			"updated_at":      time.Now(),
		}).Error
}

func attrString(attributes map[string]any, key string) string {
	if attributes == nil {
		return ""
	}
	raw, ok := attributes[key]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

// sanitizeWorkdayUsername keeps the characters users.username accepts and
// strips everything else; the result may be empty.
func sanitizeWorkdayUsername(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.', r == '-', r == '_':
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), ".-_")
}

func truncateWorkdayUsername(value string) string {
	value = strings.TrimSpace(value)
	const maxLen = 100
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

// ensureUniqueWorkdayUsername mutates username to a value that is not already
// taken, appending a short random suffix when the base name is occupied.
func ensureUniqueWorkdayUsername(tx *gorm.DB, username *string) error {
	for i := 0; i < 10; i++ {
		var count int64
		if err := tx.Model(&types.User{}).
			Where("username = ?", *username).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		suffix, err := randomSuffix(4)
		if err != nil {
			return err
		}
		candidate := truncateWorkdayUsername(*username + "-" + suffix)
		if len(candidate) < len(*username) {
			return fmt.Errorf("workday username %q cannot be uniquified", *username)
		}
		*username = candidate
	}
	return fmt.Errorf("workday username %q collided too many times", *username)
}

// generateRandomPassword returns a URL-safe random string of the given length.
func generateRandomPassword(length int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	buffer := make([]byte, length)
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	for i, b := range randomBytes {
		buffer[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buffer), nil
}

func randomSuffix(length int) (string, error) {
	const hexChars = "0123456789abcdef"
	buffer := make([]byte, length)
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	for i, b := range randomBytes {
		buffer[i] = hexChars[int(b)%len(hexChars)]
	}
	return string(buffer), nil
}

func applyWorkdayMembership(
	tx *gorm.DB,
	provider, userID string,
	item types.WorkdayWorkerRecord,
) (bool, error) {
	if !item.Status.IsActive() || item.PrimaryOrgExternalID == "" {
		result := tx.Model(&types.UserOrgMembership{}).
			Where("user_id = ? AND source = ? AND status = ?", userID, types.OrgUnitSourceWorkday, types.OrgUnitStatusActive).
			Updates(map[string]any{
				"status":     types.OrgUnitStatusInactive,
				"is_primary": false,
				"updated_at": time.Now(),
			})
		return result.RowsAffected > 0, result.Error
	}

	var orgProjection types.ExternalOrgUnit
	err := tx.Where(
		"provider = ? AND external_org_id = ? AND status = ?",
		provider,
		item.PrimaryOrgExternalID,
		types.OrgUnitStatusActive,
	).First(&orgProjection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || orgProjection.OrgUnitID == nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	now := time.Now()
	if err := tx.Model(&types.UserOrgMembership{}).
		Where("user_id = ? AND is_primary = ?", userID, true).
		Updates(map[string]any{"is_primary": false, "updated_at": now}).Error; err != nil {
		return false, err
	}
	if err := tx.Model(&types.UserOrgMembership{}).
		Where("user_id = ? AND source = ? AND org_unit_id <> ?", userID, types.OrgUnitSourceWorkday, *orgProjection.OrgUnitID).
		Updates(map[string]any{
			"status":     types.OrgUnitStatusInactive,
			"is_primary": false,
			"updated_at": now,
		}).Error; err != nil {
		return false, err
	}

	membership := &types.UserOrgMembership{
		UserID:    userID,
		OrgUnitID: *orgProjection.OrgUnitID,
		IsPrimary: true,
		Status:    types.OrgUnitStatusActive,
		Source:    types.OrgUnitSourceWorkday,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err = tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "org_unit_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"is_primary", "status", "source", "updated_at",
		}),
	}).Create(membership).Error
	return true, err
}

func (r *enterpriseIntegrationRepository) FinishSyncRun(
	ctx context.Context,
	runID string,
	status types.IntegrationSyncStatus,
	counters types.WorkdaySyncCounters,
	errorCode, errorSummary string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run types.IntegrationSyncRun
		if err := tx.Where("id = ?", runID).First(&run).Error; err != nil {
			return err
		}
		if status == types.IntegrationSyncSucceeded &&
			run.Mode == types.IntegrationSyncModeFull &&
			run.StartedAt != nil {
			// Organizations are only reconciled when this run actually synced
			// them. A workers-only run (WORKDAY_SYNC_ORG_UNITS=false) must not
			// deactivate previously imported organizations.
			if err := deactivateMissingWorkdayRecords(
				tx,
				run.Provider,
				*run.StartedAt,
				counters.OrgUnitsSeen > 0,
			); err != nil {
				return err
			}
		}
		now := time.Now()
		return tx.Model(&types.IntegrationSyncRun{}).
			Where("id = ?", runID).
			Updates(map[string]any{
				"status":        status,
				"counters":      jsonValue(counters),
				"error_code":    errorCode,
				"error_summary": truncateIntegrationError(errorSummary),
				"finished_at":   now,
			}).Error
	})
}

func deactivateMissingWorkdayRecords(tx *gorm.DB, provider string, startedAt time.Time, deactivateOrgs bool) error {
	var workers []types.ExternalWorker
	if err := tx.Where(
		"provider = ? AND last_seen_at < ? AND worker_status <> ?",
		provider,
		startedAt,
		types.ExternalWorkerInactive,
	).Find(&workers).Error; err != nil {
		return err
	}
	now := time.Now()
	for i := range workers {
		if workers[i].UserID != nil {
			if err := tx.Model(&types.UserOrgMembership{}).
				Where("user_id = ? AND source = ?", *workers[i].UserID, types.OrgUnitSourceWorkday).
				Updates(map[string]any{
					"status":     types.OrgUnitStatusInactive,
					"is_primary": false,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Model(&types.ExternalWorker{}).
		Where("provider = ? AND last_seen_at < ?", provider, startedAt).
		Updates(map[string]any{
			"worker_status": types.ExternalWorkerInactive,
			"updated_at":    now,
		}).Error; err != nil {
		return err
	}

	var orgs []types.ExternalOrgUnit
	if err := tx.Where(
		"provider = ? AND last_seen_at < ? AND status <> ?",
		provider,
		startedAt,
		types.OrgUnitStatusInactive,
	).Find(&orgs).Error; err != nil {
		return err
	}
	for i := range orgs {
		if orgs[i].OrgUnitID != nil {
			if err := tx.Model(&types.UserOrgMembership{}).
				Where(
					"org_unit_id = ? AND status = ?",
					*orgs[i].OrgUnitID,
					types.OrgUnitStatusActive,
				).
				Updates(map[string]any{
					"status":     types.OrgUnitStatusInactive,
					"is_primary": false,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
			if err := tx.Model(&types.OrgUnit{}).
				Where("id = ?", *orgs[i].OrgUnitID).
				Updates(map[string]any{
					"status":     types.OrgUnitStatusInactive,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
	}
	return tx.Model(&types.ExternalOrgUnit{}).
		Where("provider = ? AND last_seen_at < ?", provider, startedAt).
		Updates(map[string]any{
			"status":     types.OrgUnitStatusInactive,
			"updated_at": now,
		}).Error
}

func (r *enterpriseIntegrationRepository) CreateEventIfNew(
	ctx context.Context,
	event *types.IntegrationEvent,
) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "external_event_id"}},
		DoNothing: true,
	}).Create(event)
	return result.RowsAffected > 0, result.Error
}

func (r *enterpriseIntegrationRepository) MarkEvent(
	ctx context.Context,
	id uint64,
	status types.IntegrationEventStatus,
	errorSummary string,
) error {
	updates := map[string]any{
		"status":        status,
		"error_summary": truncateIntegrationError(errorSummary),
	}
	if status == types.IntegrationEventProcessed || status == types.IntegrationEventFailed {
		updates["processed_at"] = time.Now()
	}
	if status == types.IntegrationEventProcessing {
		updates["attempt_count"] = gorm.Expr("attempt_count + 1")
	}
	return r.db.WithContext(ctx).Model(&types.IntegrationEvent{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func integrationRecordChecksum(value any) (string, types.JSON, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(data)
	var attributes map[string]any
	switch typed := value.(type) {
	case types.WorkdayOrgUnitRecord:
		attributes = typed.Attributes
	case types.WorkdayWorkerRecord:
		attributes = typed.Attributes
	}
	return hex.EncodeToString(sum[:]), jsonValue(attributes), nil
}

func updateRunCursor(tx *gorm.DB, runID, key, cursor string) error {
	var run types.IntegrationSyncRun
	if err := tx.Where("id = ?", runID).First(&run).Error; err != nil {
		return err
	}
	cursorMap := map[string]string{}
	if len(run.CursorAfter) > 0 {
		_ = json.Unmarshal(run.CursorAfter, &cursorMap)
	}
	cursorMap[key] = cursor
	return tx.Model(&types.IntegrationSyncRun{}).
		Where("id = ?", runID).
		Update("cursor_after", jsonValue(cursorMap)).Error
}

func jsonValue(value any) types.JSON {
	data, err := json.Marshal(value)
	if err != nil {
		return types.JSON([]byte(`{}`))
	}
	return types.JSON(data)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func truncateIntegrationError(value string) string {
	value = strings.TrimSpace(value)
	const maxLength = 2048
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}
