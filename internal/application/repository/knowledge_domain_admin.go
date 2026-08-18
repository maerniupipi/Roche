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

type knowledgeDomainAdminRepository struct{ db *gorm.DB }

func escapeLikePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func NewKnowledgeDomainAdminRepository(db *gorm.DB) interfaces.KnowledgeDomainAdminRepository {
	return &knowledgeDomainAdminRepository{db: db}
}

func (r *knowledgeDomainAdminRepository) Upsert(ctx context.Context, admin *types.KnowledgeDomainAdmin) error {
	if admin.Status == "" {
		admin.Status = types.KnowledgeDomainAdminStatusActive
	}
	now := time.Now()
	if admin.CreatedAt.IsZero() {
		admin.CreatedAt = now
	}
	admin.UpdatedAt = now
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "knowledge_domain_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"granted_by": admin.GrantedBy,
			"status":     types.KnowledgeDomainAdminStatusActive,
			"updated_at": now,
		}),
	}).Create(admin).Error
}

func (r *knowledgeDomainAdminRepository) Get(ctx context.Context, userID string, knowledgeDomainID uint64) (*types.KnowledgeDomainAdmin, error) {
	var admin types.KnowledgeDomainAdmin
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND knowledge_domain_id = ? AND status = ?", userID, knowledgeDomainID, types.KnowledgeDomainAdminStatusActive).
		First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &admin, err
}

func (r *knowledgeDomainAdminRepository) ListDomainIDsByUser(ctx context.Context, userID string) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).Model(&types.KnowledgeDomainAdmin{}).
		Where("user_id = ? AND status = ?", userID, types.KnowledgeDomainAdminStatusActive).
		Order("knowledge_domain_id ASC").
		Pluck("knowledge_domain_id", &ids).Error
	return ids, err
}

func (r *knowledgeDomainAdminRepository) ListByDomain(
	ctx context.Context,
	knowledgeDomainID uint64,
	search string,
	offset, limit int,
) ([]*types.KnowledgeDomainAdmin, int64, error) {
	q := r.db.WithContext(ctx).Model(&types.KnowledgeDomainAdmin{}).
		Where("knowledge_domain_admins.knowledge_domain_id = ? AND knowledge_domain_admins.status = ?", knowledgeDomainID, types.KnowledgeDomainAdminStatusActive)
	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + escapeLikePattern(search) + "%"
		q = q.Joins("JOIN users ON users.id = knowledge_domain_admins.user_id AND users.deleted_at IS NULL").
			Where("LOWER(users.email) LIKE LOWER(?) OR LOWER(users.username) LIKE LOWER(?)", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var admins []*types.KnowledgeDomainAdmin
	err := q.Order("knowledge_domain_admins.created_at ASC, knowledge_domain_admins.id ASC").
		Offset(offset).Limit(limit).Find(&admins).Error
	return admins, total, err
}

func (r *knowledgeDomainAdminRepository) Delete(ctx context.Context, userID string, knowledgeDomainID uint64) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND knowledge_domain_id = ?", userID, knowledgeDomainID).
		Delete(&types.KnowledgeDomainAdmin{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
