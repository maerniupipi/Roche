package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var (
	ErrBlacklistEntryNotFound = errors.New("blacklist entry not found")
)

// blacklistEntryRepository implements BlacklistEntryRepository interface.
type blacklistEntryRepository struct {
	db *gorm.DB
}

// NewBlacklistEntryRepository creates a new blacklist entry repository.
func NewBlacklistEntryRepository(db *gorm.DB) interfaces.BlacklistEntryRepository {
	return &blacklistEntryRepository{db: db}
}

// Add adds a user to the blacklist. Uses ON CONFLICT DO NOTHING for idempotency.
func (r *blacklistEntryRepository) Add(ctx context.Context, entry *types.BlacklistEntry) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(entry).Error
}

// Remove removes a user from the blacklist. Idempotent — no error if not found.
func (r *blacklistEntryRepository) Remove(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&types.BlacklistEntry{}).Error
}

// IsBlacklisted checks whether a user is in the blacklist.
func (r *blacklistEntryRepository) IsBlacklisted(ctx context.Context, userID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.BlacklistEntry{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByUserID returns the blacklist entry for a user, or nil if not found.
func (r *blacklistEntryRepository) GetByUserID(ctx context.Context, userID string) (*types.BlacklistEntry, error) {
	var entry types.BlacklistEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entry, nil
}
