package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type KnowledgeBasePermissionLevel string

const (
	KnowledgeBasePermissionRead   KnowledgeBasePermissionLevel = "read"
	KnowledgeBasePermissionManage KnowledgeBasePermissionLevel = "manage"
)

func (p KnowledgeBasePermissionLevel) IsValid() bool {
	return p == KnowledgeBasePermissionRead || p == KnowledgeBasePermissionManage
}

func (p KnowledgeBasePermissionLevel) Allows(required KnowledgeBasePermissionLevel) bool {
	if p == KnowledgeBasePermissionManage {
		return true
	}
	return p == required
}

type GrantSubjectType string

const (
	GrantSubjectUser    GrantSubjectType = "user"
	GrantSubjectOrgUnit GrantSubjectType = "org_unit"
)

func (s GrantSubjectType) IsValid() bool {
	return s == GrantSubjectUser || s == GrantSubjectOrgUnit
}

type KnowledgeResourceType string

const (
	KnowledgeResourceKnowledgeBase KnowledgeResourceType = "knowledge_base"
	KnowledgeResourceFolder        KnowledgeResourceType = "folder"
	KnowledgeResourceKnowledge     KnowledgeResourceType = "knowledge"
)

func (r KnowledgeResourceType) IsValid() bool {
	return r == KnowledgeResourceKnowledgeBase ||
		r == KnowledgeResourceFolder ||
		r == KnowledgeResourceKnowledge
}

type GrantEffect string

const (
	GrantEffectAllow GrantEffect = "allow"
	GrantEffectDeny  GrantEffect = "deny"
)

func (e GrantEffect) IsValid() bool {
	return e == GrantEffectAllow || e == GrantEffectDeny
}

type OrgUnitStatus string

const (
	OrgUnitStatusActive   OrgUnitStatus = "active"
	OrgUnitStatusInactive OrgUnitStatus = "inactive"
)

func (s OrgUnitStatus) IsValid() bool {
	return s == OrgUnitStatusActive || s == OrgUnitStatusInactive
}

type OrgUnitSource string

const (
	OrgUnitSourceManual    OrgUnitSource = "manual"
	OrgUnitSourceWorkday   OrgUnitSource = "workday"
	OrgUnitSourceBootstrap OrgUnitSource = "bootstrap"
)

type OrgUnit struct {
	ID         string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	ParentID   *string        `json:"parent_id,omitempty" gorm:"type:varchar(36);index"`
	Code       string         `json:"code" gorm:"type:varchar(128);not null"`
	Name       string         `json:"name" gorm:"type:varchar(255);not null"`
	Status     OrgUnitStatus  `json:"status" gorm:"type:varchar(20);not null;default:'active'"`
	Source     OrgUnitSource  `json:"source" gorm:"type:varchar(32);not null;default:'manual'"`
	ExternalID *string        `json:"external_id,omitempty" gorm:"type:varchar(255)"`
	SortOrder  int            `json:"sort_order" gorm:"not null;default:0"`
	Attributes JSON           `json:"attributes" gorm:"type:jsonb;not null;default:'{}'"`
	CreatedBy  *string        `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (OrgUnit) TableName() string { return "org_units" }

func (o *OrgUnit) BeforeCreate(*gorm.DB) error {
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	if o.Status == "" {
		o.Status = OrgUnitStatusActive
	}
	if o.Source == "" {
		o.Source = OrgUnitSourceManual
	}
	if o.Attributes == nil {
		o.Attributes = JSON([]byte(`{}`))
	}
	return nil
}

type UserOrgMembership struct {
	ID        uint64        `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    string        `json:"user_id" gorm:"type:varchar(36);not null;index;uniqueIndex:uq_user_org_membership"`
	OrgUnitID string        `json:"org_unit_id" gorm:"type:varchar(36);not null;index;uniqueIndex:uq_user_org_membership"`
	IsPrimary bool          `json:"is_primary" gorm:"not null;default:false"`
	Status    OrgUnitStatus `json:"status" gorm:"type:varchar(20);not null;default:'active'"`
	Source    OrgUnitSource `json:"source" gorm:"type:varchar(32);not null;default:'manual'"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func (UserOrgMembership) TableName() string { return "user_org_memberships" }

// KnowledgeResourceGrant is the single ACL model for a physical knowledge
// base and every logical folder/document beneath it. ResourceID contains the
// knowledge-base, folder, or knowledge UUID selected by ResourceType.
//
// Manage implies read. Deny rules override matching allow rules. KB and folder
// rules may be inherited by descendants; document rules are always exact.
type KnowledgeResourceGrant struct {
	ID                uint64                       `json:"id" gorm:"primaryKey;autoIncrement"`
	KnowledgeDomainID uint64                       `json:"knowledge_domain_id" gorm:"not null;index"`
	KnowledgeBaseID   string                       `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index"`
	ResourceType      KnowledgeResourceType        `json:"resource_type" gorm:"type:varchar(24);not null;index"`
	ResourceID        string                       `json:"resource_id" gorm:"type:varchar(36);not null;index"`
	SubjectType       GrantSubjectType             `json:"subject_type" gorm:"type:varchar(16);not null"`
	SubjectID         string                       `json:"subject_id" gorm:"type:varchar(36);not null"`
	Permission        KnowledgeBasePermissionLevel `json:"permission" gorm:"type:varchar(16);not null;default:'read'"`
	Effect            GrantEffect                  `json:"effect" gorm:"type:varchar(8);not null;default:'allow'"`
	InheritToChildren bool                         `json:"inherit_to_children" gorm:"not null;default:true"`
	GrantedBy         *string                      `json:"granted_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
	SubjectName       string                       `json:"subject_name" gorm:"->"`
	SubjectDetail     string                       `json:"subject_detail,omitempty" gorm:"->"`
	ResourceName      string                       `json:"resource_name" gorm:"->"`
	ResourcePath      string                       `json:"resource_path,omitempty" gorm:"->"`
}

func (KnowledgeResourceGrant) TableName() string { return "knowledge_resource_grants" }

func (g *KnowledgeResourceGrant) ValidResourceShape() bool {
	if g == nil || !g.ResourceType.IsValid() || g.ResourceID == "" {
		return false
	}
	return g.ResourceType != KnowledgeResourceKnowledgeBase ||
		g.ResourceID == g.KnowledgeBaseID
}

// KnowledgeResourceGrantResult reports the outcome of a batch grant for a
// single knowledge base. Granted=false carries a machine-readable Reason so
// callers can tell which knowledge bases were skipped and why.
type KnowledgeResourceGrantResult struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Granted         bool   `json:"granted"`
	GrantID         uint64 `json:"grant_id,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

type KnowledgeAccessResource struct {
	ID       string
	FolderID *string
}

// KnowledgeBaseAccessScope is computed for the current caller. FullAccess
// permits every document in the knowledge base. Otherwise only KnowledgeIDs
// are readable. Organization membership alone never produces a scope.
type KnowledgeBaseAccessScope struct {
	Allowed            bool                         `json:"allowed"`
	CanManage          bool                         `json:"can_manage"`
	FullAccess         bool                         `json:"full_access"`
	KnowledgeIDs       []string                     `json:"knowledge_ids,omitempty"`
	ManageKnowledgeIDs []string                     `json:"manage_knowledge_ids,omitempty"`
	FolderIDs          []string                     `json:"folder_ids,omitempty"`
	ManageFolderIDs    []string                     `json:"manage_folder_ids,omitempty"`
	Permission         KnowledgeBasePermissionLevel `json:"permission,omitempty"`
}

func (s *KnowledgeBaseAccessScope) AllowsKnowledge(knowledgeID string) bool {
	if s == nil || !s.Allowed {
		return false
	}
	if s.FullAccess {
		return true
	}
	for _, allowedID := range s.KnowledgeIDs {
		if allowedID == knowledgeID {
			return true
		}
	}
	return false
}

func (s *KnowledgeBaseAccessScope) AllowsFolder(folderID string) bool {
	if s == nil || !s.Allowed {
		return false
	}
	if s.FullAccess {
		return true
	}
	return containsString(s.FolderIDs, folderID)
}

func (s *KnowledgeBaseAccessScope) ManagesKnowledge(knowledgeID string) bool {
	if s == nil || !s.Allowed {
		return false
	}
	if s.CanManage {
		return true
	}
	return containsString(s.ManageKnowledgeIDs, knowledgeID)
}

func (s *KnowledgeBaseAccessScope) ManagesFolder(folderID string) bool {
	if s == nil || !s.Allowed {
		return false
	}
	if s.CanManage {
		return true
	}
	return containsString(s.ManageFolderIDs, folderID)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type OrgUnitMember struct {
	UserID       string        `json:"user_id"`
	Email        string        `json:"email"`
	Username     string        `json:"username"`
	Avatar       string        `json:"avatar,omitempty"`
	OrgUnitID    string        `json:"org_unit_id"`
	IsPrimary    bool          `json:"is_primary"`
	Status       OrgUnitStatus `json:"status"`
	Source       OrgUnitSource `json:"source"`
	MembershipID uint64        `json:"membership_id"`
}

type GrantUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
	IsActive bool   `json:"is_active"`
}

// KnowledgeGrantSubjects contains the safe directory projection exposed to a
// manager while editing one resource's ACL.
type KnowledgeGrantSubjects struct {
	OrgUnits []*OrgUnit   `json:"org_units"`
	Users    []*GrantUser `json:"users"`
}

type SSOIdentity struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      string     `json:"user_id" gorm:"type:varchar(36);not null;index"`
	Provider    string     `json:"provider" gorm:"type:varchar(64);not null"`
	Issuer      string     `json:"issuer" gorm:"type:varchar(255);not null"`
	Subject     string     `json:"subject" gorm:"type:varchar(255);not null"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

func (SSOIdentity) TableName() string { return "sso_identities" }

// KnowledgeBaseOfficer records the binding between a knowledge officer and a knowledge base.
type KnowledgeBaseOfficer struct {
	ID              uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);not null;uniqueIndex:uq_kb_officer"`
	UserID          string    `json:"user_id" gorm:"type:varchar(36);not null;uniqueIndex:uq_kb_officer"`
	GrantedBy       *string   `json:"granted_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// Joined fields
	Username string `json:"username,omitempty" gorm:"->"`
	Email    string `json:"email,omitempty" gorm:"->"`
}

func (KnowledgeBaseOfficer) TableName() string { return "knowledge_base_officers" }
