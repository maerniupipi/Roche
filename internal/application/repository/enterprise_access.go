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

type enterpriseAccessRepository struct{ db *gorm.DB }

func NewEnterpriseAccessRepository(db *gorm.DB) interfaces.EnterpriseAccessRepository {
	return &enterpriseAccessRepository{db: db}
}

func (r *enterpriseAccessRepository) ListEffectiveOrgUnitIDs(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Raw(`
		WITH RECURSIVE effective_org_units(id, parent_id) AS (
			SELECT ou.id, ou.parent_id
			FROM user_org_memberships uom
			JOIN org_units ou ON ou.id = uom.org_unit_id
			WHERE uom.user_id = ?
			  AND uom.status = 'active'
			  AND ou.status = 'active'
			  AND ou.deleted_at IS NULL
			UNION
			SELECT parent.id, parent.parent_id
			FROM org_units parent
			JOIN effective_org_units child ON child.parent_id = parent.id
			WHERE parent.status = 'active'
			  AND parent.deleted_at IS NULL
		)
		SELECT DISTINCT id FROM effective_org_units
	`, userID).Scan(&ids).Error
	return ids, err
}

func subjectGrantQuery(
	db *gorm.DB,
	userID string,
	orgUnitIDs []string,
) *gorm.DB {
	if len(orgUnitIDs) == 0 {
		return db.Where("subject_type = ? AND subject_id = ?", types.GrantSubjectUser, userID)
	}
	return db.Where(
		"(subject_type = ? AND subject_id = ?) OR (subject_type = ? AND subject_id IN ?)",
		types.GrantSubjectUser,
		userID,
		types.GrantSubjectOrgUnit,
		orgUnitIDs,
	)
}

func (r *enterpriseAccessRepository) ListSubjectResourceGrants(
	ctx context.Context,
	knowledgeBaseID string,
	userID string,
	orgUnitIDs []string,
) ([]*types.KnowledgeResourceGrant, error) {
	var grants []*types.KnowledgeResourceGrant
	q := r.db.WithContext(ctx).
		Model(&types.KnowledgeResourceGrant{}).
		Where("knowledge_base_id = ?", knowledgeBaseID)
	err := subjectGrantQuery(q, userID, orgUnitIDs).
		Order("resource_type ASC, resource_id ASC, permission ASC").
		Find(&grants).Error
	return grants, err
}

func (r *enterpriseAccessRepository) ListKnowledgeBaseIDsForSubjects(
	ctx context.Context,
	userID string,
	orgUnitIDs []string,
) ([]string, error) {
	var ids []string
	query := r.db.WithContext(ctx).
		Model(&types.KnowledgeResourceGrant{}).
		Where("effect = ?", types.GrantEffectAllow)
	if err := subjectGrantQuery(query, userID, orgUnitIDs).
		Distinct("knowledge_base_id").
		Order("knowledge_base_id ASC").
		Pluck("knowledge_base_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *enterpriseAccessRepository) ListKnowledgeFoldersForAccess(
	ctx context.Context,
	knowledgeBaseID string,
) ([]*types.KnowledgeFolder, error) {
	var folders []*types.KnowledgeFolder
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Order("relative_path ASC").
		Find(&folders).Error
	return folders, err
}

func (r *enterpriseAccessRepository) ListKnowledgeResourcesForAccess(
	ctx context.Context,
	knowledgeBaseID string,
) ([]*types.KnowledgeAccessResource, error) {
	var resources []*types.KnowledgeAccessResource
	err := r.db.WithContext(ctx).
		Table("knowledges").
		Select("id, folder_id").
		Where("knowledge_base_id = ? AND deleted_at IS NULL", knowledgeBaseID).
		Order("id ASC").
		Scan(&resources).Error
	return resources, err
}

func (r *enterpriseAccessRepository) ListResourceGrants(
	ctx context.Context,
	knowledgeDomainID uint64,
	knowledgeBaseID string,
	resourceType types.KnowledgeResourceType,
	resourceID string,
) ([]*types.KnowledgeResourceGrant, error) {
	var grants []*types.KnowledgeResourceGrant
	err := r.db.WithContext(ctx).
		Table("knowledge_resource_grants AS grants").
		Select(`
			grants.*,
			CASE
				WHEN grants.subject_type = 'user'
					THEN COALESCE(NULLIF(users.username, ''), users.email, grants.subject_id)
				ELSE COALESCE(org_units.name, grants.subject_id)
			END AS subject_name,
			CASE
				WHEN grants.subject_type = 'user'
					THEN COALESCE(users.email, '')
				ELSE COALESCE(org_units.code, '')
			END AS subject_detail,
			CASE grants.resource_type
				WHEN 'knowledge_base' THEN COALESCE(knowledge_bases.name, grants.resource_id)
				WHEN 'folder' THEN COALESCE(knowledge_folders.name, grants.resource_id)
				WHEN 'knowledge' THEN COALESCE(knowledges.title, grants.resource_id)
				ELSE grants.resource_id
			END AS resource_name,
			CASE grants.resource_type
				WHEN 'folder' THEN COALESCE(knowledge_folders.relative_path, '')
				WHEN 'knowledge' THEN COALESCE(knowledge_folders_for_document.relative_path, '')
				ELSE ''
			END AS resource_path
		`).
		Joins("LEFT JOIN users ON grants.subject_type = 'user' AND users.id = grants.subject_id AND users.deleted_at IS NULL").
		Joins("LEFT JOIN org_units ON grants.subject_type = 'org_unit' AND org_units.id = grants.subject_id AND org_units.deleted_at IS NULL").
		Joins("LEFT JOIN knowledge_bases ON grants.resource_type = 'knowledge_base' AND knowledge_bases.id = grants.resource_id AND knowledge_bases.deleted_at IS NULL").
		Joins("LEFT JOIN knowledge_folders ON grants.resource_type = 'folder' AND knowledge_folders.id = grants.resource_id").
		Joins("LEFT JOIN knowledges ON grants.resource_type = 'knowledge' AND knowledges.id = grants.resource_id AND knowledges.deleted_at IS NULL").
		Joins("LEFT JOIN knowledge_folders AS knowledge_folders_for_document ON knowledges.folder_id = knowledge_folders_for_document.id").
		Where(
			"grants.knowledge_domain_id = ? AND grants.knowledge_base_id = ? AND grants.resource_type = ? AND grants.resource_id = ?",
			knowledgeDomainID,
			knowledgeBaseID,
			resourceType,
			resourceID,
		).
		Order("grants.effect DESC, grants.permission DESC, grants.subject_type ASC, grants.subject_id ASC").
		Scan(&grants).Error
	return grants, err
}

func (r *enterpriseAccessRepository) GetResourceGrant(
	ctx context.Context,
	knowledgeDomainID uint64,
	knowledgeBaseID string,
	grantID uint64,
) (*types.KnowledgeResourceGrant, error) {
	var grant types.KnowledgeResourceGrant
	err := r.db.WithContext(ctx).
		Where(
			"id = ? AND knowledge_domain_id = ? AND knowledge_base_id = ?",
			grantID,
			knowledgeDomainID,
			knowledgeBaseID,
		).
		First(&grant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &grant, err
}

func (r *enterpriseAccessRepository) UpsertResourceGrant(
	ctx context.Context,
	grant *types.KnowledgeResourceGrant,
) error {
	grant.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "knowledge_base_id"},
			{Name: "resource_type"},
			{Name: "resource_id"},
			{Name: "subject_type"},
			{Name: "subject_id"},
			{Name: "permission"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"knowledge_domain_id", "effect", "inherit_to_children", "granted_by", "updated_at",
		}),
	}).Create(grant).Error
}

func (r *enterpriseAccessRepository) DeleteResourceGrant(
	ctx context.Context,
	knowledgeDomainID uint64,
	knowledgeBaseID string,
	grantID uint64,
) error {
	result := r.db.WithContext(ctx).
		Where(
			"id = ? AND knowledge_domain_id = ? AND knowledge_base_id = ?",
			grantID,
			knowledgeDomainID,
			knowledgeBaseID,
		).
		Delete(&types.KnowledgeResourceGrant{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *enterpriseAccessRepository) ResourceExists(
	ctx context.Context,
	knowledgeDomainID uint64,
	knowledgeBaseID string,
	resourceType types.KnowledgeResourceType,
	resourceID string,
) (bool, error) {
	var count int64
	var query *gorm.DB
	switch resourceType {
	case types.KnowledgeResourceKnowledgeBase:
		query = r.db.WithContext(ctx).
			Model(&types.KnowledgeBase{}).
			Where(
				"id = ? AND knowledge_domain_id = ? AND deleted_at IS NULL",
				resourceID,
				knowledgeDomainID,
			)
	case types.KnowledgeResourceFolder:
		query = r.db.WithContext(ctx).
			Model(&types.KnowledgeFolder{}).
			Where(
				"id = ? AND knowledge_domain_id = ? AND knowledge_base_id = ?",
				resourceID,
				knowledgeDomainID,
				knowledgeBaseID,
			)
	case types.KnowledgeResourceKnowledge:
		query = r.db.WithContext(ctx).
			Model(&types.Knowledge{}).
			Where(
				"id = ? AND knowledge_domain_id = ? AND knowledge_base_id = ? AND deleted_at IS NULL",
				resourceID,
				knowledgeDomainID,
				knowledgeBaseID,
			)
	default:
		return false, nil
	}
	if err := query.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *enterpriseAccessRepository) ListOrgUnits(ctx context.Context) ([]*types.OrgUnit, error) {
	var units []*types.OrgUnit
	err := r.db.WithContext(ctx).
		Order("sort_order ASC, name ASC, id ASC").
		Find(&units).Error
	return units, err
}

func (r *enterpriseAccessRepository) GetOrgUnit(ctx context.Context, id string) (*types.OrgUnit, error) {
	var unit types.OrgUnit
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&unit).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &unit, err
}

func (r *enterpriseAccessRepository) CreateOrgUnit(ctx context.Context, unit *types.OrgUnit) error {
	return r.db.WithContext(ctx).Create(unit).Error
}

func (r *enterpriseAccessRepository) UpdateOrgUnit(ctx context.Context, unit *types.OrgUnit) error {
	updates := map[string]any{
		"parent_id":  unit.ParentID,
		"code":       unit.Code,
		"name":       unit.Name,
		"status":     unit.Status,
		"sort_order": unit.SortOrder,
		"attributes": unit.Attributes,
		"updated_at": time.Now(),
	}
	result := r.db.WithContext(ctx).Model(&types.OrgUnit{}).
		Where("id = ?", unit.ID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *enterpriseAccessRepository) DeleteOrgUnit(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.OrgUnit{}).Error
}

func (r *enterpriseAccessRepository) ListOrgUnitMembers(
	ctx context.Context,
	orgUnitID string,
) ([]*types.OrgUnitMember, error) {
	var members []*types.OrgUnitMember
	err := r.db.WithContext(ctx).
		Table("user_org_memberships AS uom").
		Select(`
			uom.id AS membership_id,
			uom.user_id,
			users.email,
			users.username,
			users.avatar,
			uom.org_unit_id,
			uom.is_primary,
			uom.status,
			uom.source
		`).
		Joins("JOIN users ON users.id = uom.user_id AND users.deleted_at IS NULL").
		Where("uom.org_unit_id = ?", orgUnitID).
		Order("users.username ASC, users.email ASC").
		Scan(&members).Error
	return members, err
}

func (r *enterpriseAccessRepository) ListUserOrgMemberships(
	ctx context.Context,
	userID string,
) ([]*types.UserOrgMembership, error) {
	var memberships []*types.UserOrgMembership
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("is_primary DESC, id ASC").
		Find(&memberships).Error
	return memberships, err
}

func (r *enterpriseAccessRepository) ReplaceUserOrgMemberships(
	ctx context.Context,
	userID string,
	memberships []*types.UserOrgMembership,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&types.UserOrgMembership{}).Error; err != nil {
			return err
		}
		if len(memberships) == 0 {
			return nil
		}
		return tx.Create(memberships).Error
	})
}

func (r *enterpriseAccessRepository) SearchActiveUsers(
	ctx context.Context,
	search string,
	limit int,
) ([]*types.GrantUser, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	search = strings.TrimSpace(search)
	var users []*types.GrantUser
	q := r.db.WithContext(ctx).Table("users").
		Select("id, email, username, avatar, (status = ?) AS is_active", types.UserStatusNormal).
		Where("deleted_at IS NULL AND status = ?", types.UserStatusNormal).
		Order("username ASC, email ASC").
		Limit(limit)
	if search != "" {
		like := "%" + escapeLikePattern(search) + "%"
		q = q.Where(
			"LOWER(email) LIKE LOWER(?) OR LOWER(username) LIKE LOWER(?)",
			like,
			like,
		)
	}
	err := q.Scan(&users).Error
	return users, err
}

func (r *enterpriseAccessRepository) IsActiveUser(ctx context.Context, userID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.User{}).
		Where("id = ? AND status = ?", userID, types.UserStatusNormal).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

// -- Knowledge-base-officer bindings --

func (r *enterpriseAccessRepository) ListKnowledgeBaseOfficers(
	ctx context.Context,
	kbID string,
) ([]*types.KnowledgeBaseOfficer, error) {
	var officers []*types.KnowledgeBaseOfficer
	err := r.db.WithContext(ctx).
		Table("knowledge_base_officers AS kbo").
		Select("kbo.*, COALESCE(NULLIF(u.username, ''), u.email, kbo.user_id) AS username, COALESCE(u.email, '') AS email").
		Joins("LEFT JOIN users AS u ON u.id = kbo.user_id AND u.deleted_at IS NULL").
		Where("kbo.knowledge_base_id = ?", kbID).
		Order("kbo.created_at ASC").
		Scan(&officers).Error
	return officers, err
}

func (r *enterpriseAccessRepository) AddKnowledgeBaseOfficer(
	ctx context.Context,
	kbID, userID, grantedBy string,
) error {
	return r.db.WithContext(ctx).Create(&types.KnowledgeBaseOfficer{
		KnowledgeBaseID: kbID,
		UserID:          userID,
		GrantedBy:       &grantedBy,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}).Error
}

func (r *enterpriseAccessRepository) RemoveKnowledgeBaseOfficer(
	ctx context.Context,
	kbID, userID string,
) error {
	return r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND user_id = ?", kbID, userID).
		Delete(&types.KnowledgeBaseOfficer{}).Error
}

func (r *enterpriseAccessRepository) IsKnowledgeBaseOfficer(
	ctx context.Context,
	userID, kbID string,
) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.KnowledgeBaseOfficer{}).
		Where("knowledge_base_id = ? AND user_id = ?", kbID, userID).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

func (r *enterpriseAccessRepository) ListOfficerKnowledgeBaseIDs(
	ctx context.Context,
	userID string,
) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&types.KnowledgeBaseOfficer{}).
		Where("user_id = ?", userID).
		Distinct("knowledge_base_id").
		Order("knowledge_base_id ASC").
		Pluck("knowledge_base_id", &ids).Error
	return ids, err
}

func (r *enterpriseAccessRepository) ListPublicKnowledgeBaseIDs(
	ctx context.Context,
) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&types.KnowledgeBase{}).
		Where("is_public = ? AND deleted_at IS NULL", true).
		Distinct("id").
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}

// ListAllKnowledgeBaseIDs returns IDs of all non-deleted knowledge bases.
// Used to grant whitelisted users read-only access to every KB.
func (r *enterpriseAccessRepository) ListAllKnowledgeBaseIDs(
	ctx context.Context,
) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&types.KnowledgeBase{}).
		Where("deleted_at IS NULL").
		Distinct("id").
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}
