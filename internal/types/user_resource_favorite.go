package types

import "time"

const (
	ResourceTypeKB    = "kb"
	ResourceTypeAgent = "agent"
)

// UserResourceFavorite is a user's global star on a single resource.
// Resource IDs are globally unique, so favorites do not depend on a selected
// knowledgeDomain.
type UserResourceFavorite struct {
	UserID       string    `json:"user_id"       gorm:"type:varchar(36);primaryKey"`
	ResourceType string    `json:"resource_type" gorm:"type:varchar(16);primaryKey"`
	ResourceID   string    `json:"resource_id"   gorm:"type:varchar(64);primaryKey"`
	CreatedAt    time.Time `json:"created_at"    gorm:"autoCreateTime"`
}

func (UserResourceFavorite) TableName() string {
	return "user_resource_favorites"
}

func IsValidFavoriteResourceType(t string) bool {
	switch t {
	case ResourceTypeKB, ResourceTypeAgent:
		return true
	default:
		return false
	}
}
