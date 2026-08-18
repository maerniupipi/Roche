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

type ssoIdentityRepository struct{ db *gorm.DB }

func NewSSOIdentityRepository(db *gorm.DB) interfaces.SSOIdentityRepository {
	return &ssoIdentityRepository{db: db}
}

func (r *ssoIdentityRepository) GetBySubject(ctx context.Context, provider, issuer, subject string) (*types.SSOIdentity, error) {
	var identity types.SSOIdentity
	err := r.db.WithContext(ctx).Where("provider = ? AND issuer = ? AND subject = ?", provider, issuer, subject).First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &identity, err
}

func (r *ssoIdentityRepository) Upsert(ctx context.Context, identity *types.SSOIdentity) error {
	identity.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "issuer"}, {Name: "subject"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "updated_at", "last_login_at"}),
	}).Create(identity).Error
}

func (r *ssoIdentityRepository) TouchLogin(ctx context.Context, id uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&types.SSOIdentity{}).Where("id = ?", id).Updates(map[string]any{"last_login_at": now, "updated_at": now}).Error
}

func (r *ssoIdentityRepository) CreateEnterpriseUser(ctx context.Context, user *types.User, identity *types.SSOIdentity) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(identity).Error; err != nil {
			return err
		}
		return linkWorkdayWorkerForNewUser(tx, user)
	})
}

// linkWorkdayWorkerForNewUser handles the common ordering where Workday data
// arrives before the employee's first PingIdentity login. Email is used only
// for this initial link; subsequent synchronization uses external_worker_id.
func linkWorkdayWorkerForNewUser(tx *gorm.DB, user *types.User) error {
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return nil
	}
	var worker types.ExternalWorker
	err := tx.Where(
		"provider = ? AND LOWER(corporate_email) = ?",
		types.EnterpriseProviderWorkday,
		strings.ToLower(strings.TrimSpace(user.Email)),
	).Order("last_seen_at DESC").First(&worker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := tx.Model(&types.ExternalWorker{}).
		Where("id = ?", worker.ID).
		Update("user_id", user.ID).Error; err != nil {
		return err
	}
	_, err = applyWorkdayMembership(
		tx,
		types.EnterpriseProviderWorkday,
		user.ID,
		types.WorkdayWorkerRecord{
			ExternalID:           worker.ExternalWorkerID,
			PrimaryOrgExternalID: dereferenceString(worker.PrimaryOrgExternalID),
			Status:               worker.WorkerStatus,
		},
	)
	return err
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
