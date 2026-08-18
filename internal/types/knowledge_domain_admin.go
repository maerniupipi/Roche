package types

import "time"

const KnowledgeDomainAdminStatusActive = "active"

// KnowledgeDomainAdmin assigns management responsibility for one knowledge
// domain. It is independent from enterprise organization membership.
type KnowledgeDomainAdmin struct {
	ID                uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	KnowledgeDomainID uint64    `json:"knowledge_domain_id" gorm:"not null;uniqueIndex:uq_knowledge_domain_admin"`
	UserID            string    `json:"user_id" gorm:"type:varchar(36);not null;uniqueIndex:uq_knowledge_domain_admin"`
	GrantedBy         *string   `json:"granted_by,omitempty" gorm:"type:varchar(36)"`
	Status            string    `json:"status" gorm:"type:varchar(16);not null;default:'active'"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (KnowledgeDomainAdmin) TableName() string {
	return "knowledge_domain_admins"
}

type KnowledgeDomainAdminResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar,omitempty"`
	Status    string    `json:"status"`
	GrantedBy *string   `json:"granted_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
