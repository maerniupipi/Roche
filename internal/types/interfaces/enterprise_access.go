package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

type EnterpriseAccessRepository interface {
	ListEffectiveOrgUnitIDs(ctx context.Context, userID string) ([]string, error)
	ListSubjectResourceGrants(ctx context.Context, knowledgeBaseID, userID string, orgUnitIDs []string) ([]*types.KnowledgeResourceGrant, error)
	ListKnowledgeBaseIDsForSubjects(ctx context.Context, userID string, orgUnitIDs []string) ([]string, error)
	ListKnowledgeFoldersForAccess(ctx context.Context, knowledgeBaseID string) ([]*types.KnowledgeFolder, error)
	ListKnowledgeResourcesForAccess(ctx context.Context, knowledgeBaseID string) ([]*types.KnowledgeAccessResource, error)

	ListResourceGrants(ctx context.Context, knowledgeDomainID uint64, knowledgeBaseID string, resourceType types.KnowledgeResourceType, resourceID string) ([]*types.KnowledgeResourceGrant, error)
	GetResourceGrant(ctx context.Context, knowledgeDomainID uint64, knowledgeBaseID string, grantID uint64) (*types.KnowledgeResourceGrant, error)
	UpsertResourceGrant(ctx context.Context, grant *types.KnowledgeResourceGrant) error
	DeleteResourceGrant(ctx context.Context, knowledgeDomainID uint64, knowledgeBaseID string, grantID uint64) error
	ResourceExists(ctx context.Context, knowledgeDomainID uint64, knowledgeBaseID string, resourceType types.KnowledgeResourceType, resourceID string) (bool, error)

	ListOrgUnits(ctx context.Context) ([]*types.OrgUnit, error)
	GetOrgUnit(ctx context.Context, id string) (*types.OrgUnit, error)
	CreateOrgUnit(ctx context.Context, unit *types.OrgUnit) error
	UpdateOrgUnit(ctx context.Context, unit *types.OrgUnit) error
	DeleteOrgUnit(ctx context.Context, id string) error
	ListOrgUnitMembers(ctx context.Context, orgUnitID string) ([]*types.OrgUnitMember, error)
	ListUserOrgMemberships(ctx context.Context, userID string) ([]*types.UserOrgMembership, error)
	ReplaceUserOrgMemberships(ctx context.Context, userID string, memberships []*types.UserOrgMembership) error
	SearchActiveUsers(ctx context.Context, search string, limit int) ([]*types.GrantUser, error)
	IsActiveUser(ctx context.Context, userID string) (bool, error)

	// Knowledge-base-officer bindings
	ListKnowledgeBaseOfficers(ctx context.Context, kbID string) ([]*types.KnowledgeBaseOfficer, error)
	AddKnowledgeBaseOfficer(ctx context.Context, kbID, userID, grantedBy string) error
	RemoveKnowledgeBaseOfficer(ctx context.Context, kbID, userID string) error
	IsKnowledgeBaseOfficer(ctx context.Context, userID, kbID string) (bool, error)
	ListOfficerKnowledgeBaseIDs(ctx context.Context, userID string) ([]string, error)

	// Public knowledge bases
	ListPublicKnowledgeBaseIDs(ctx context.Context) ([]string, error)
	// All knowledge bases (used by whitelist access)
	ListAllKnowledgeBaseIDs(ctx context.Context) ([]string, error)
}

type SSOIdentityRepository interface {
	GetBySubject(ctx context.Context, provider, issuer, subject string) (*types.SSOIdentity, error)
	Upsert(ctx context.Context, identity *types.SSOIdentity) error
	TouchLogin(ctx context.Context, id uint64) error
	CreateEnterpriseUser(ctx context.Context, user *types.User, identity *types.SSOIdentity) error
}

type EnterpriseAccessService interface {
	ResolveKnowledgeBaseAccess(ctx context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBaseAccessScope, error)
	CanReadKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error)
	CanReadKnowledge(ctx context.Context, knowledge *types.Knowledge) (bool, error)
	CanManageKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error)
	CanManageKnowledge(ctx context.Context, knowledge *types.Knowledge) (bool, error)
	CanManageFolder(ctx context.Context, kb *types.KnowledgeBase, folderID string) (bool, error)
	ListGrantedKnowledgeBaseIDs(ctx context.Context) ([]string, error)
	// ListAllKnowledgeBaseIDs returns every non-deleted knowledge-base ID
	// regardless of knowledge domain or permission. Used by the menu endpoint
	// to build the manageable-knowledge-base navigation tree.
	ListAllKnowledgeBaseIDs(ctx context.Context) ([]string, error)
	FilterKnowledgeBases(ctx context.Context, knowledgeBases []*types.KnowledgeBase) ([]*types.KnowledgeBase, error)

	ListResourceGrants(ctx context.Context, kb *types.KnowledgeBase, resourceType types.KnowledgeResourceType, resourceID string) ([]*types.KnowledgeResourceGrant, error)
	ListResourceGrantSubjects(ctx context.Context, kb *types.KnowledgeBase, resourceType types.KnowledgeResourceType, resourceID, search string, limit int) (*types.KnowledgeGrantSubjects, error)
	GrantResource(ctx context.Context, kb *types.KnowledgeBase, grant *types.KnowledgeResourceGrant) error
	// GrantResourceBatch writes one knowledge-base grant for the same subject
	// across multiple knowledge bases. It validates the caller's management
	// permission per knowledge base and skips knowledge bases the caller
	// cannot manage; the per-knowledge-base outcome is reported in the result.
	GrantResourceBatch(ctx context.Context, kbs []*types.KnowledgeBase, grant *types.KnowledgeResourceGrant) ([]*types.KnowledgeResourceGrantResult, error)
	RevokeResource(ctx context.Context, kb *types.KnowledgeBase, grantID uint64) error

	ListOrgUnits(ctx context.Context) ([]*types.OrgUnit, error)
	CreateOrgUnit(ctx context.Context, unit *types.OrgUnit) error
	UpdateOrgUnit(ctx context.Context, unit *types.OrgUnit) error
	DeleteOrgUnit(ctx context.Context, id string) error
	ListOrgUnitMembers(ctx context.Context, orgUnitID string) ([]*types.OrgUnitMember, error)
	ListUserOrgMemberships(ctx context.Context, userID string) ([]*types.UserOrgMembership, error)
	ReplaceUserOrgMemberships(ctx context.Context, userID string, memberships []*types.UserOrgMembership) error
	SearchGrantUsers(ctx context.Context, search string, limit int) ([]*types.GrantUser, error)

	// Knowledge-base-officer bindings
	ListKnowledgeBaseOfficers(ctx context.Context, kbID string) ([]*types.KnowledgeBaseOfficer, error)
	AddKnowledgeBaseOfficer(ctx context.Context, kbID, userID string) error
	RemoveKnowledgeBaseOfficer(ctx context.Context, kbID, userID string) error
}
