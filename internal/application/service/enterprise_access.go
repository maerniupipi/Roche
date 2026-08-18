package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

const enterpriseRootOrgUnitID = "00000000-0000-0000-0000-000000000001"

var (
	ErrEnterpriseAccessDenied   = errors.New("enterprise access denied")
	ErrEnterpriseMemberMissing  = errors.New("grant subject is missing or inactive")
	ErrInvalidPermission        = errors.New("invalid resource permission")
	ErrInvalidGrantSubject      = errors.New("invalid grant subject")
	ErrKnowledgeResourceMissing = errors.New("knowledge resource not found")
	ErrOrgUnitNotFound          = errors.New("organization unit not found")
	ErrOrgUnitNotEmpty          = errors.New("organization unit still has child units or members")
	ErrOrgUnitCycle             = errors.New("organization hierarchy cannot contain a cycle")
	ErrProtectedOrgUnit         = errors.New("the enterprise root organization cannot be changed or deleted")
	ErrWorkdayMembershipManaged = errors.New("Workday-managed organization memberships cannot be changed manually")
)

type enterpriseAccessService struct {
	repo          interfaces.EnterpriseAccessRepository
	domainAdmins  interfaces.KnowledgeDomainAdminService
	businessAudit *BusinessAuditRecorder
}

func NewEnterpriseAccessService(
	repo interfaces.EnterpriseAccessRepository,
	domainAdmins interfaces.KnowledgeDomainAdminService,
	businessAudit *BusinessAuditRecorder,
) interfaces.EnterpriseAccessService {
	return &enterpriseAccessService{
		repo:          repo,
		domainAdmins:  domainAdmins,
		businessAudit: businessAudit,
	}
}

func enterpriseContext(ctx context.Context) (userID string, root bool, ok bool) {
	userID, userOK := types.UserIDFromContext(ctx)
	if !userOK || userID == "" {
		return "", false, false
	}
	user, _ := ctx.Value(types.UserContextKey).(*types.User)
	root = types.IsSystemAdminFromContext(ctx) && user != nil && user.IsSystemAdmin
	return userID, root, true
}

func (s *enterpriseAccessService) ResolveKnowledgeBaseAccess(
	ctx context.Context,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBaseAccessScope, error) {
	scope := &types.KnowledgeBaseAccessScope{}
	if kb == nil {
		return scope, nil
	}
	userID, _, ok := enterpriseContext(ctx)
	if !ok {
		return scope, nil
	}

	// 1. System administrator → full access (unchanged).
	user, _ := ctx.Value(types.UserContextKey).(*types.User)
	if user != nil && user.IsSystemAdmin {
		scope.Allowed = true
		scope.CanManage = true
		scope.FullAccess = true
		scope.Permission = types.KnowledgeBasePermissionManage
		return scope, nil
	}

	// 2. Knowledge officer → check KB-officer binding (new).
	if user != nil && user.RoleKnowledgeOfficer == types.RoleFlagTrue {
		isOfficer, err := s.repo.IsKnowledgeBaseOfficer(ctx, userID, kb.ID)
		if err != nil {
			return nil, err
		}
		if isOfficer {
			scope.Allowed = true
			scope.CanManage = true
			scope.FullAccess = true
			scope.Permission = types.KnowledgeBasePermissionManage
			return scope, nil
		}
		// Knowledge officer but NOT assigned to this KB → fall through to ACL.
	}

	// 3. Knowledge-domain administrator (unchanged).
	if s.domainAdmins != nil {
		isDomainAdmin, err := s.domainAdmins.IsAdmin(ctx, userID, kb.KnowledgeDomainID)
		if err != nil {
			return nil, err
		}
		if isDomainAdmin {
			scope.Allowed = true
			scope.CanManage = true
			scope.FullAccess = true
			scope.Permission = types.KnowledgeBasePermissionManage
			return scope, nil
		}
	}

	// 4. ACL grants — inject a virtual company-wide allow grant when the KB is public.
	orgUnitIDs, err := s.repo.ListEffectiveOrgUnitIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	grants, err := s.repo.ListSubjectResourceGrants(ctx, kb.ID, userID, orgUnitIDs)
	if err != nil {
		return nil, err
	}

	// When the KB is public, prepend a virtual allow-read grant for the
	// whole company. A per-user or per-org-unit deny grant still takes
	// precedence because effectiveResourcePermission evaluates deny before
	// allow, so administrators can explicitly block specific users even
	// when the knowledge base is otherwise open to everyone.
	if kb.IsPublic {
		grants = append([]*types.KnowledgeResourceGrant{
			{
				ResourceType:      types.KnowledgeResourceKnowledgeBase,
				ResourceID:        kb.ID,
				SubjectType:       types.GrantSubjectUser,
				Effect:            types.GrantEffectAllow,
				Permission:        types.KnowledgeBasePermissionRead,
				InheritToChildren: true,
			},
		}, grants...)
	}
	folders, err := s.repo.ListKnowledgeFoldersForAccess(ctx, kb.ID)
	if err != nil {
		return nil, err
	}
	resources, err := s.repo.ListKnowledgeResourcesForAccess(ctx, kb.ID)
	if err != nil {
		return nil, err
	}

	folderByID := make(map[string]*types.KnowledgeFolder, len(folders))
	for _, folder := range folders {
		if folder != nil {
			folderByID[folder.ID] = folder
		}
	}

	kbRead, kbManage := effectiveResourcePermission(
		grants,
		types.KnowledgeResourceKnowledgeBase,
		kb.ID,
		nil,
		folderByID,
	)
	scope.CanManage = kbManage

	readFolders := make(map[string]struct{}, len(folders))
	manageFolders := make(map[string]struct{}, len(folders))
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		read, manage := effectiveResourcePermission(
			grants,
			types.KnowledgeResourceFolder,
			folder.ID,
			&folder.ID,
			folderByID,
		)
		if read {
			readFolders[folder.ID] = struct{}{}
		}
		if manage {
			manageFolders[folder.ID] = struct{}{}
		}
	}

	allDocumentsReadable := true
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		read, manage := effectiveResourcePermission(
			grants,
			types.KnowledgeResourceKnowledge,
			resource.ID,
			resource.FolderID,
			folderByID,
		)
		if read {
			scope.KnowledgeIDs = append(scope.KnowledgeIDs, resource.ID)
			addFolderAncestors(readFolders, resource.FolderID, folderByID)
		} else {
			allDocumentsReadable = false
		}
		if manage {
			scope.ManageKnowledgeIDs = append(scope.ManageKnowledgeIDs, resource.ID)
		}
	}

	scope.FullAccess = kbRead && allDocumentsReadable
	if scope.FullAccess {
		scope.KnowledgeIDs = nil
	}
	scope.FolderIDs = orderedSetValues(readFolders, folders)
	scope.ManageFolderIDs = orderedSetValues(manageFolders, folders)
	scope.Allowed = kbRead || len(scope.KnowledgeIDs) > 0 || len(scope.FolderIDs) > 0
	if scope.Allowed {
		if scope.CanManage {
			scope.Permission = types.KnowledgeBasePermissionManage
		} else {
			scope.Permission = types.KnowledgeBasePermissionRead
		}
	}
	return scope, nil
}

func effectiveResourcePermission(
	grants []*types.KnowledgeResourceGrant,
	resourceType types.KnowledgeResourceType,
	resourceID string,
	folderID *string,
	folderByID map[string]*types.KnowledgeFolder,
) (read bool, manage bool) {
	var allowRead, allowManage, denyRead, denyManage bool
	for _, grant := range grants {
		if !grantAppliesToResource(grant, resourceType, resourceID, folderID, folderByID) {
			continue
		}
		switch grant.Effect {
		case types.GrantEffectDeny:
			if grant.Permission == types.KnowledgeBasePermissionRead {
				denyRead = true
				denyManage = true
			} else if grant.Permission == types.KnowledgeBasePermissionManage {
				denyManage = true
			}
		case types.GrantEffectAllow:
			if grant.Permission == types.KnowledgeBasePermissionRead {
				allowRead = true
			} else if grant.Permission == types.KnowledgeBasePermissionManage {
				allowRead = true
				allowManage = true
			}
		}
	}
	return allowRead && !denyRead, allowManage && !denyRead && !denyManage
}

func grantAppliesToResource(
	grant *types.KnowledgeResourceGrant,
	resourceType types.KnowledgeResourceType,
	resourceID string,
	folderID *string,
	folderByID map[string]*types.KnowledgeFolder,
) bool {
	if grant == nil {
		return false
	}
	switch grant.ResourceType {
	case types.KnowledgeResourceKnowledgeBase:
		if resourceType == types.KnowledgeResourceKnowledgeBase {
			return grant.ResourceID == resourceID
		}
		return grant.InheritToChildren
	case types.KnowledgeResourceKnowledge:
		return resourceType == types.KnowledgeResourceKnowledge &&
			grant.ResourceID == resourceID
	case types.KnowledgeResourceFolder:
		if resourceType == types.KnowledgeResourceFolder && grant.ResourceID == resourceID {
			return true
		}
		if !grant.InheritToChildren || folderID == nil {
			return false
		}
		return folderIsOrDescendsFrom(*folderID, grant.ResourceID, folderByID)
	default:
		return false
	}
}

func folderIsOrDescendsFrom(
	folderID string,
	ancestorID string,
	folderByID map[string]*types.KnowledgeFolder,
) bool {
	seen := make(map[string]struct{})
	for folderID != "" {
		if folderID == ancestorID {
			return true
		}
		if _, exists := seen[folderID]; exists {
			return false
		}
		seen[folderID] = struct{}{}
		folder := folderByID[folderID]
		if folder == nil || folder.ParentID == nil {
			return false
		}
		folderID = *folder.ParentID
	}
	return false
}

func addFolderAncestors(
	target map[string]struct{},
	folderID *string,
	folderByID map[string]*types.KnowledgeFolder,
) {
	if folderID == nil {
		return
	}
	current := *folderID
	seen := make(map[string]struct{})
	for current != "" {
		if _, exists := seen[current]; exists {
			return
		}
		seen[current] = struct{}{}
		target[current] = struct{}{}
		folder := folderByID[current]
		if folder == nil || folder.ParentID == nil {
			return
		}
		current = *folder.ParentID
	}
}

func orderedSetValues(
	values map[string]struct{},
	folders []*types.KnowledgeFolder,
) []string {
	result := make([]string, 0, len(values))
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		if _, exists := values[folder.ID]; exists {
			result = append(result, folder.ID)
		}
	}
	return result
}

func (s *enterpriseAccessService) CanManageKnowledge(
	ctx context.Context,
	knowledge *types.Knowledge,
) (bool, error) {
	if knowledge == nil {
		return false, nil
	}
	scope, err := s.ResolveKnowledgeBaseAccess(ctx, &types.KnowledgeBase{
		ID:                knowledge.KnowledgeBaseID,
		KnowledgeDomainID: knowledge.KnowledgeDomainID,
	})
	if err != nil {
		return false, err
	}
	return scope.ManagesKnowledge(knowledge.ID), nil
}

func (s *enterpriseAccessService) CanManageFolder(
	ctx context.Context,
	kb *types.KnowledgeBase,
	folderID string,
) (bool, error) {
	scope, err := s.ResolveKnowledgeBaseAccess(ctx, kb)
	if err != nil {
		return false, err
	}
	return scope.ManagesFolder(folderID), nil
}

func (s *enterpriseAccessService) ListResourceGrants(
	ctx context.Context,
	kb *types.KnowledgeBase,
	resourceType types.KnowledgeResourceType,
	resourceID string,
) ([]*types.KnowledgeResourceGrant, error) {
	if err := s.requireKnowledgeBaseGrantManager(ctx, kb, resourceType, resourceID); err != nil {
		return nil, err
	}
	return s.repo.ListResourceGrants(
		ctx,
		kb.KnowledgeDomainID,
		kb.ID,
		resourceType,
		resourceID,
	)
}

func (s *enterpriseAccessService) ListResourceGrantSubjects(
	ctx context.Context,
	kb *types.KnowledgeBase,
	resourceType types.KnowledgeResourceType,
	resourceID string,
	search string,
	limit int,
) (*types.KnowledgeGrantSubjects, error) {
	if err := s.requireKnowledgeBaseGrantManager(ctx, kb, resourceType, resourceID); err != nil {
		return nil, err
	}
	orgUnits, err := s.repo.ListOrgUnits(ctx)
	if err != nil {
		return nil, err
	}
	users, err := s.repo.SearchActiveUsers(ctx, search, limit)
	if err != nil {
		return nil, err
	}
	return &types.KnowledgeGrantSubjects{
		OrgUnits: orgUnits,
		Users:    users,
	}, nil
}

func (s *enterpriseAccessService) GrantResource(
	ctx context.Context,
	kb *types.KnowledgeBase,
	grant *types.KnowledgeResourceGrant,
) error {
	if kb == nil || grant == nil {
		return ErrKnowledgeResourceMissing
	}
	grant.ResourceID = strings.TrimSpace(grant.ResourceID)
	grant.SubjectID = strings.TrimSpace(grant.SubjectID)
	grant.KnowledgeDomainID = kb.KnowledgeDomainID
	grant.KnowledgeBaseID = kb.ID
	if !grant.ValidResourceShape() {
		return ErrKnowledgeResourceMissing
	}
	if !grant.Permission.IsValid() {
		return ErrInvalidPermission
	}
	if grant.Effect == "" {
		grant.Effect = types.GrantEffectAllow
	}
	if !grant.Effect.IsValid() {
		return ErrInvalidPermission
	}
	if grant.ResourceType == types.KnowledgeResourceKnowledgeBase {
		grant.InheritToChildren = true
	}
	if grant.ResourceType == types.KnowledgeResourceKnowledge {
		grant.InheritToChildren = false
	}
	if err := s.requireKnowledgeBaseGrantManager(
		ctx,
		kb,
		grant.ResourceType,
		grant.ResourceID,
	); err != nil {
		return err
	}
	exists, err := s.repo.ResourceExists(
		ctx,
		kb.KnowledgeDomainID,
		kb.ID,
		grant.ResourceType,
		grant.ResourceID,
	)
	if err != nil {
		return err
	}
	if !exists {
		return ErrKnowledgeResourceMissing
	}
	if err := s.requireGrantSubject(ctx, grant.SubjectType, grant.SubjectID); err != nil {
		return err
	}
	actor, _ := types.UserIDFromContext(ctx)
	now := time.Now()
	grant.GrantedBy = &actor
	grant.CreatedAt = now
	grant.UpdatedAt = now
	if err := s.repo.UpsertResourceGrant(ctx, grant); err != nil {
		return err
	}

	// Audit: resource permission granted
	s.businessAudit.RecordPermissionGranted(ctx,
		string(grant.ResourceType), grant.ResourceID, kb.Name,
		grant.SubjectID, "", string(grant.Permission))

	return nil
}

// GrantResourceBatch writes one knowledge-base grant for the same subject
// across multiple knowledge bases. The subject and grant attributes are
// validated once; each knowledge base is then checked for management
// permission and existence independently. Knowledge bases the caller cannot
// manage (or that do not exist) are skipped and reported in the result
// instead of aborting the whole batch.
func (s *enterpriseAccessService) GrantResourceBatch(
	ctx context.Context,
	kbs []*types.KnowledgeBase,
	grant *types.KnowledgeResourceGrant,
) ([]*types.KnowledgeResourceGrantResult, error) {
	if grant == nil {
		return nil, ErrInvalidGrantSubject
	}
	grant.SubjectID = strings.TrimSpace(grant.SubjectID)
	if !grant.SubjectType.IsValid() || grant.SubjectID == "" {
		return nil, ErrInvalidGrantSubject
	}
	if !grant.Permission.IsValid() {
		return nil, ErrInvalidPermission
	}
	if grant.Effect == "" {
		grant.Effect = types.GrantEffectAllow
	}
	if !grant.Effect.IsValid() {
		return nil, ErrInvalidPermission
	}
	if err := s.requireGrantSubject(ctx, grant.SubjectType, grant.SubjectID); err != nil {
		return nil, err
	}
	actor, _ := types.UserIDFromContext(ctx)
	now := time.Now()

	results := make([]*types.KnowledgeResourceGrantResult, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		result := &types.KnowledgeResourceGrantResult{KnowledgeBaseID: kb.ID}
		g := *grant
		g.ResourceType = types.KnowledgeResourceKnowledgeBase
		g.ResourceID = kb.ID
		g.KnowledgeDomainID = kb.KnowledgeDomainID
		g.KnowledgeBaseID = kb.ID
		g.InheritToChildren = true
		if !g.ValidResourceShape() {
			result.Reason = "invalid resource"
			results = append(results, result)
			continue
		}
		if err := s.requireKnowledgeBaseGrantManager(ctx, kb, g.ResourceType, g.ResourceID); err != nil {
			if errors.Is(err, ErrEnterpriseAccessDenied) {
				result.Reason = "permission denied"
				results = append(results, result)
				continue
			}
			return nil, err
		}
		exists, err := s.repo.ResourceExists(
			ctx,
			kb.KnowledgeDomainID,
			kb.ID,
			g.ResourceType,
			g.ResourceID,
		)
		if err != nil {
			return nil, err
		}
		if !exists {
			result.Reason = "knowledge base not found"
			results = append(results, result)
			continue
		}
		g.GrantedBy = &actor
		g.CreatedAt = now
		g.UpdatedAt = now
		if err := s.repo.UpsertResourceGrant(ctx, &g); err != nil {
			return nil, err
		}
		result.Granted = true
		result.GrantID = g.ID

		// Audit: resource permission granted
		s.businessAudit.RecordPermissionGranted(ctx,
			string(g.ResourceType), g.ResourceID, kb.Name,
			g.SubjectID, "", string(g.Permission))

		results = append(results, result)
	}
	return results, nil
}

func (s *enterpriseAccessService) RevokeResource(
	ctx context.Context,
	kb *types.KnowledgeBase,
	grantID uint64,
) error {
	if kb == nil {
		return ErrKnowledgeResourceMissing
	}
	grant, err := s.repo.GetResourceGrant(
		ctx,
		kb.KnowledgeDomainID,
		kb.ID,
		grantID,
	)
	if err != nil {
		return err
	}
	if grant == nil {
		return ErrKnowledgeResourceMissing
	}
	if err := s.requireKnowledgeBaseGrantManager(
		ctx,
		kb,
		grant.ResourceType,
		grant.ResourceID,
	); err != nil {
		return err
	}
	if err := s.repo.DeleteResourceGrant(ctx, kb.KnowledgeDomainID, kb.ID, grantID); err != nil {
		return err
	}

	// Audit: resource permission revoked
	s.businessAudit.RecordPermissionRevoked(ctx,
		string(grant.ResourceType), grant.ResourceID, kb.Name,
		grant.SubjectID, "", string(grant.Permission))

	return nil
}

// requireKnowledgeBaseGrantManager centralizes ACL administration at the
// knowledge-base boundary. A folder/document manage grant controls content
// use; it does not let its recipient rewrite authorization policy.
func (s *enterpriseAccessService) requireKnowledgeBaseGrantManager(
	ctx context.Context,
	kb *types.KnowledgeBase,
	resourceType types.KnowledgeResourceType,
	resourceID string,
) error {
	if kb == nil || !resourceType.IsValid() || strings.TrimSpace(resourceID) == "" {
		return ErrKnowledgeResourceMissing
	}
	if resourceType == types.KnowledgeResourceKnowledgeBase && resourceID != kb.ID {
		return ErrKnowledgeResourceMissing
	}
	allowed, err := s.CanManageKnowledgeBase(ctx, kb)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrEnterpriseAccessDenied
	}
	return nil
}

func (s *enterpriseAccessService) CanReadKnowledgeBase(
	ctx context.Context,
	kb *types.KnowledgeBase,
) (bool, error) {
	scope, err := s.ResolveKnowledgeBaseAccess(ctx, kb)
	return err == nil && scope != nil && scope.Allowed, err
}

func (s *enterpriseAccessService) CanReadKnowledge(
	ctx context.Context,
	knowledge *types.Knowledge,
) (bool, error) {
	if knowledge == nil {
		return false, nil
	}
	scope, err := s.ResolveKnowledgeBaseAccess(ctx, &types.KnowledgeBase{
		ID:                knowledge.KnowledgeBaseID,
		KnowledgeDomainID: knowledge.KnowledgeDomainID,
	})
	if err != nil {
		return false, err
	}
	return scope.AllowsKnowledge(knowledge.ID), nil
}

func (s *enterpriseAccessService) CanManageKnowledgeBase(
	ctx context.Context,
	kb *types.KnowledgeBase,
) (bool, error) {
	scope, err := s.ResolveKnowledgeBaseAccess(ctx, kb)
	return err == nil && scope != nil && scope.CanManage, err
}

func (s *enterpriseAccessService) ListGrantedKnowledgeBaseIDs(ctx context.Context) ([]string, error) {
	userID, root, ok := enterpriseContext(ctx)
	if !ok || root {
		return nil, nil
	}

	orgUnitIDs, err := s.repo.ListEffectiveOrgUnitIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	grantIDs, err := s.repo.ListKnowledgeBaseIDsForSubjects(ctx, userID, orgUnitIDs)
	if err != nil {
		return nil, err
	}
	// Also include knowledge-base-officer bindings for knowledge officers.
	user, _ := ctx.Value(types.UserContextKey).(*types.User)
	if user != nil && user.RoleKnowledgeOfficer == types.RoleFlagTrue {
		officerIDs, officerErr := s.repo.ListOfficerKnowledgeBaseIDs(ctx, userID)
		if officerErr != nil {
			return nil, officerErr
		}
		seen := make(map[string]struct{}, len(grantIDs)+len(officerIDs))
		for _, id := range grantIDs {
			seen[id] = struct{}{}
		}
		for _, id := range officerIDs {
			if _, exists := seen[id]; !exists {
				grantIDs = append(grantIDs, id)
				seen[id] = struct{}{}
			}
		}
	}
	// Merge public knowledge bases so that search and listing cover them.
	publicIDs, err := s.repo.ListPublicKnowledgeBaseIDs(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(grantIDs)+len(publicIDs))
	for _, id := range grantIDs {
		seen[id] = struct{}{}
	}
	for _, id := range publicIDs {
		if _, exists := seen[id]; !exists {
			grantIDs = append(grantIDs, id)
			seen[id] = struct{}{}
		}
	}
	return grantIDs, nil
}

func (s *enterpriseAccessService) FilterKnowledgeBases(
	ctx context.Context,
	knowledgeBases []*types.KnowledgeBase,
) ([]*types.KnowledgeBase, error) {
	result := make([]*types.KnowledgeBase, 0, len(knowledgeBases))
	for _, kb := range knowledgeBases {
		scope, err := s.ResolveKnowledgeBaseAccess(ctx, kb)
		if err != nil {
			return nil, err
		}
		if scope == nil || !scope.Allowed {
			continue
		}
		kb.MyPermission = scope.Permission
		result = append(result, kb)
	}
	return result, nil
}

// ListAllKnowledgeBaseIDs returns every non-deleted knowledge-base ID across
// all knowledge domains. The menu endpoint combines this with
// CanManageKnowledgeBase to build the caller's navigation tree.
func (s *enterpriseAccessService) ListAllKnowledgeBaseIDs(ctx context.Context) ([]string, error) {
	return s.repo.ListAllKnowledgeBaseIDs(ctx)
}

func (s *enterpriseAccessService) ListOrgUnits(ctx context.Context) ([]*types.OrgUnit, error) {
	if !s.canAdministerEnterpriseDirectory(ctx) {
		return nil, ErrEnterpriseAccessDenied
	}
	return s.repo.ListOrgUnits(ctx)
}

func (s *enterpriseAccessService) CreateOrgUnit(ctx context.Context, unit *types.OrgUnit) error {
	if !isSystemAdministrator(ctx) {
		return ErrEnterpriseAccessDenied
	}
	if unit == nil || strings.TrimSpace(unit.Code) == "" || strings.TrimSpace(unit.Name) == "" {
		return ErrInvalidGrantSubject
	}
	if unit.ParentID != nil {
		parent, err := s.repo.GetOrgUnit(ctx, *unit.ParentID)
		if err != nil {
			return err
		}
		if parent == nil || parent.Status != types.OrgUnitStatusActive {
			return ErrOrgUnitNotFound
		}
	}
	actor, _ := types.UserIDFromContext(ctx)
	unit.Code = strings.TrimSpace(unit.Code)
	unit.Name = strings.TrimSpace(unit.Name)
	unit.Source = types.OrgUnitSourceManual
	unit.CreatedBy = &actor
	if unit.Status == "" {
		unit.Status = types.OrgUnitStatusActive
	}
	if !unit.Status.IsValid() {
		return ErrInvalidGrantSubject
	}
	return s.repo.CreateOrgUnit(ctx, unit)
}

func (s *enterpriseAccessService) UpdateOrgUnit(ctx context.Context, unit *types.OrgUnit) error {
	if !isSystemAdministrator(ctx) {
		return ErrEnterpriseAccessDenied
	}
	if unit == nil || strings.TrimSpace(unit.ID) == "" {
		return ErrOrgUnitNotFound
	}
	if unit.ID == enterpriseRootOrgUnitID {
		return ErrProtectedOrgUnit
	}
	existing, err := s.repo.GetOrgUnit(ctx, unit.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrOrgUnitNotFound
	}
	if strings.TrimSpace(unit.Code) == "" || strings.TrimSpace(unit.Name) == "" || !unit.Status.IsValid() {
		return ErrInvalidGrantSubject
	}
	if err := s.validateOrgParent(ctx, unit.ID, unit.ParentID); err != nil {
		return err
	}
	unit.Code = strings.TrimSpace(unit.Code)
	unit.Name = strings.TrimSpace(unit.Name)
	return s.repo.UpdateOrgUnit(ctx, unit)
}

func (s *enterpriseAccessService) DeleteOrgUnit(ctx context.Context, id string) error {
	if !isSystemAdministrator(ctx) {
		return ErrEnterpriseAccessDenied
	}
	if id == enterpriseRootOrgUnitID {
		return ErrProtectedOrgUnit
	}
	unit, err := s.repo.GetOrgUnit(ctx, id)
	if err != nil {
		return err
	}
	if unit == nil {
		return ErrOrgUnitNotFound
	}
	units, err := s.repo.ListOrgUnits(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range units {
		if candidate.ParentID != nil && *candidate.ParentID == id {
			return ErrOrgUnitNotEmpty
		}
	}
	members, err := s.repo.ListOrgUnitMembers(ctx, id)
	if err != nil {
		return err
	}
	if len(members) > 0 {
		return ErrOrgUnitNotEmpty
	}
	return s.repo.DeleteOrgUnit(ctx, id)
}

func (s *enterpriseAccessService) ListOrgUnitMembers(
	ctx context.Context,
	orgUnitID string,
) ([]*types.OrgUnitMember, error) {
	if !s.canAdministerEnterpriseDirectory(ctx) {
		return nil, ErrEnterpriseAccessDenied
	}
	unit, err := s.repo.GetOrgUnit(ctx, orgUnitID)
	if err != nil {
		return nil, err
	}
	if unit == nil {
		return nil, ErrOrgUnitNotFound
	}
	return s.repo.ListOrgUnitMembers(ctx, orgUnitID)
}

func (s *enterpriseAccessService) ListUserOrgMemberships(
	ctx context.Context,
	userID string,
) ([]*types.UserOrgMembership, error) {
	if !isSystemAdministrator(ctx) {
		return nil, ErrEnterpriseAccessDenied
	}
	active, err := s.repo.IsActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrEnterpriseMemberMissing
	}
	return s.repo.ListUserOrgMemberships(ctx, userID)
}

func (s *enterpriseAccessService) ReplaceUserOrgMemberships(
	ctx context.Context,
	userID string,
	memberships []*types.UserOrgMembership,
) error {
	if !isSystemAdministrator(ctx) {
		return ErrEnterpriseAccessDenied
	}
	active, err := s.repo.IsActiveUser(ctx, userID)
	if err != nil {
		return err
	}
	if !active {
		return ErrEnterpriseMemberMissing
	}
	if len(memberships) == 0 {
		return ErrEnterpriseMemberMissing
	}

	existingMemberships, err := s.repo.ListUserOrgMemberships(ctx, userID)
	if err != nil {
		return err
	}
	existingByOrgUnit := make(map[string]*types.UserOrgMembership, len(existingMemberships))
	requestedOrgUnits := make(map[string]struct{}, len(memberships))
	for _, existing := range existingMemberships {
		if existing != nil {
			existingByOrgUnit[existing.OrgUnitID] = existing
		}
	}

	seen := make(map[string]struct{}, len(memberships))
	primaryCount := 0
	for _, membership := range memberships {
		if membership == nil || strings.TrimSpace(membership.OrgUnitID) == "" {
			return ErrOrgUnitNotFound
		}
		if _, duplicate := seen[membership.OrgUnitID]; duplicate {
			return ErrInvalidGrantSubject
		}
		seen[membership.OrgUnitID] = struct{}{}
		requestedOrgUnits[membership.OrgUnitID] = struct{}{}
		unit, lookupErr := s.repo.GetOrgUnit(ctx, membership.OrgUnitID)
		if lookupErr != nil {
			return lookupErr
		}
		if unit == nil || unit.Status != types.OrgUnitStatusActive {
			return ErrOrgUnitNotFound
		}
		if membership.IsPrimary {
			primaryCount++
		}
		membership.UserID = userID
		membership.Status = types.OrgUnitStatusActive
		membership.Source = types.OrgUnitSourceManual
		if existing := existingByOrgUnit[membership.OrgUnitID]; existing != nil {
			membership.Source = existing.Source
		}
	}
	for _, existing := range existingMemberships {
		if existing == nil || existing.Source != types.OrgUnitSourceWorkday {
			continue
		}
		if _, retained := requestedOrgUnits[existing.OrgUnitID]; !retained {
			return ErrWorkdayMembershipManaged
		}
	}
	if primaryCount == 0 {
		memberships[0].IsPrimary = true
	} else if primaryCount > 1 {
		return ErrInvalidGrantSubject
	}
	return s.repo.ReplaceUserOrgMemberships(ctx, userID, memberships)
}

func (s *enterpriseAccessService) SearchGrantUsers(
	ctx context.Context,
	search string,
	limit int,
) ([]*types.GrantUser, error) {
	if !s.canAdministerEnterpriseDirectory(ctx) {
		return nil, ErrEnterpriseAccessDenied
	}
	return s.repo.SearchActiveUsers(ctx, search, limit)
}

func (s *enterpriseAccessService) requireKnowledgeBaseManager(
	ctx context.Context,
	kb *types.KnowledgeBase,
) error {
	allowed, err := s.CanManageKnowledgeBase(ctx, kb)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrEnterpriseAccessDenied
	}
	return nil
}

func (s *enterpriseAccessService) requireGrantSubject(
	ctx context.Context,
	subjectType types.GrantSubjectType,
	subjectID string,
) error {
	subjectID = strings.TrimSpace(subjectID)
	if !subjectType.IsValid() || subjectID == "" {
		return ErrInvalidGrantSubject
	}
	switch subjectType {
	case types.GrantSubjectUser:
		active, err := s.repo.IsActiveUser(ctx, subjectID)
		if err != nil {
			return err
		}
		if !active {
			return ErrEnterpriseMemberMissing
		}
	case types.GrantSubjectOrgUnit:
		unit, err := s.repo.GetOrgUnit(ctx, subjectID)
		if err != nil {
			return err
		}
		if unit == nil || unit.Status != types.OrgUnitStatusActive {
			return ErrOrgUnitNotFound
		}
	}
	return nil
}

func (s *enterpriseAccessService) validateOrgParent(
	ctx context.Context,
	unitID string,
	parentID *string,
) error {
	if parentID == nil {
		return nil
	}
	if *parentID == unitID {
		return ErrOrgUnitCycle
	}
	units, err := s.repo.ListOrgUnits(ctx)
	if err != nil {
		return err
	}
	parentByID := make(map[string]*string, len(units))
	foundParent := false
	for _, unit := range units {
		parentByID[unit.ID] = unit.ParentID
		if unit.ID == *parentID && unit.Status == types.OrgUnitStatusActive {
			foundParent = true
		}
	}
	if !foundParent {
		return ErrOrgUnitNotFound
	}
	for current := parentID; current != nil; current = parentByID[*current] {
		if *current == unitID {
			return ErrOrgUnitCycle
		}
	}
	return nil
}

func knowledgeBelongsToKnowledgeBase(knowledge *types.Knowledge, kb *types.KnowledgeBase) bool {
	return knowledge != nil &&
		kb != nil &&
		knowledge.KnowledgeBaseID == kb.ID &&
		knowledge.KnowledgeDomainID == kb.KnowledgeDomainID
}

func (s *enterpriseAccessService) canAdministerEnterpriseDirectory(ctx context.Context) bool {
	userID, root, ok := enterpriseContext(ctx)
	if !ok || root {
		return ok && root
	}
	if s.domainAdmins == nil {
		return false
	}
	domainIDs, err := s.domainAdmins.ListDomainIDs(ctx, userID)
	return err == nil && len(domainIDs) > 0
}

func isSystemAdministrator(ctx context.Context) bool {
	_, root, ok := enterpriseContext(ctx)
	return ok && root
}

// -- Knowledge-base-officer bindings --

func (s *enterpriseAccessService) ListKnowledgeBaseOfficers(
	ctx context.Context,
	kbID string,
) ([]*types.KnowledgeBaseOfficer, error) {
	return s.repo.ListKnowledgeBaseOfficers(ctx, kbID)
}

func (s *enterpriseAccessService) AddKnowledgeBaseOfficer(
	ctx context.Context,
	kbID, userID string,
) error {
	if !isSystemAdministrator(ctx) {
		return ErrEnterpriseAccessDenied
	}
	actor, _ := types.UserIDFromContext(ctx)
	return s.repo.AddKnowledgeBaseOfficer(ctx, kbID, userID, actor)
}

func (s *enterpriseAccessService) RemoveKnowledgeBaseOfficer(
	ctx context.Context,
	kbID, userID string,
) error {
	if !isSystemAdministrator(ctx) {
		return ErrEnterpriseAccessDenied
	}
	return s.repo.RemoveKnowledgeBaseOfficer(ctx, kbID, userID)
}
