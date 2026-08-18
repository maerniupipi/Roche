package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type accessRepoStub struct {
	interfaces.EnterpriseAccessRepository
	orgUnitsByUser map[string][]string
	grants         map[string][]*types.KnowledgeResourceGrant
	folders        map[string][]*types.KnowledgeFolder
	resources      map[string][]*types.KnowledgeAccessResource
	orgUnits       []*types.OrgUnit
	users          []*types.GrantUser
}

type domainAdminServiceStub struct {
	interfaces.KnowledgeDomainAdminService
	admins map[string]bool
}

func (s *domainAdminServiceStub) IsAdmin(
	_ context.Context,
	userID string,
	knowledgeDomainID uint64,
) (bool, error) {
	return s.admins[userID+":"+uintToDecimal(knowledgeDomainID)], nil
}

func uintToDecimal(value uint64) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[index:])
}

func (r *accessRepoStub) ListEffectiveOrgUnitIDs(
	_ context.Context,
	userID string,
) ([]string, error) {
	return r.orgUnitsByUser[userID], nil
}

func (r *accessRepoStub) ListSubjectResourceGrants(
	_ context.Context,
	kbID string,
	userID string,
	orgUnitIDs []string,
) ([]*types.KnowledgeResourceGrant, error) {
	orgs := make(map[string]struct{}, len(orgUnitIDs))
	for _, id := range orgUnitIDs {
		orgs[id] = struct{}{}
	}
	var result []*types.KnowledgeResourceGrant
	for _, grant := range r.grants[kbID] {
		if grant.SubjectType == types.GrantSubjectUser && grant.SubjectID == userID {
			result = append(result, grant)
		}
		if grant.SubjectType == types.GrantSubjectOrgUnit {
			if _, ok := orgs[grant.SubjectID]; ok {
				result = append(result, grant)
			}
		}
	}
	return result, nil
}

func (r *accessRepoStub) ListKnowledgeFoldersForAccess(
	_ context.Context,
	kbID string,
) ([]*types.KnowledgeFolder, error) {
	return r.folders[kbID], nil
}

func (r *accessRepoStub) ListKnowledgeResourcesForAccess(
	_ context.Context,
	kbID string,
) ([]*types.KnowledgeAccessResource, error) {
	return r.resources[kbID], nil
}

func (r *accessRepoStub) ListOrgUnits(context.Context) ([]*types.OrgUnit, error) {
	return r.orgUnits, nil
}

func (r *accessRepoStub) SearchActiveUsers(
	context.Context,
	string,
	int,
) ([]*types.GrantUser, error) {
	return r.users, nil
}

func enterpriseTestContext(
	knowledgeDomainID uint64,
	userID string,
	root bool,
) context.Context {
	ctx := context.WithValue(context.Background(), types.KnowledgeDomainIDContextKey, knowledgeDomainID)
	ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
	ctx = context.WithValue(ctx, types.SystemAdminContextKey, root)
	ctx = context.WithValue(ctx, types.UserContextKey, &types.User{
		ID: userID, IsSystemAdmin: root,
	})
	return ctx
}

func TestEnterpriseKnowledgeBaseAccessMatrix(t *testing.T) {
	repo := &accessRepoStub{
		orgUnitsByUser: map[string][]string{},
		grants:         map[string][]*types.KnowledgeResourceGrant{"kb-1": {}},
		folders:        map[string][]*types.KnowledgeFolder{"kb-1": {}},
		resources: map[string][]*types.KnowledgeAccessResource{
			"kb-1": {{ID: "doc-1"}},
		},
	}
	svc := &enterpriseAccessService{
		repo: repo,
		domainAdmins: &domainAdminServiceStub{admins: map[string]bool{
			"admin:1": true,
		}},
	}
	kb := &types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1, CreatorID: "creator"}

	viewer := enterpriseTestContext(1, "viewer", false)
	scope, err := svc.ResolveKnowledgeBaseAccess(viewer, kb)
	require.NoError(t, err)
	require.False(t, scope.Allowed)

	repo.grants[kb.ID] = append(repo.grants[kb.ID], &types.KnowledgeResourceGrant{
		KnowledgeBaseID:   kb.ID,
		ResourceType:      types.KnowledgeResourceKnowledgeBase,
		ResourceID:        kb.ID,
		SubjectType:       types.GrantSubjectUser,
		SubjectID:         "viewer",
		Permission:        types.KnowledgeBasePermissionRead,
		Effect:            types.GrantEffectAllow,
		InheritToChildren: true,
	})
	scope, err = svc.ResolveKnowledgeBaseAccess(viewer, kb)
	require.NoError(t, err)
	require.True(t, scope.Allowed)
	require.True(t, scope.FullAccess)
	require.False(t, scope.CanManage)

	viewerCreator := enterpriseTestContext(1, "creator", false)
	allowed, err := svc.CanManageKnowledgeBase(viewerCreator, kb)
	require.NoError(t, err)
	require.False(t, allowed, "resource creation must not imply management rights")

	admin := enterpriseTestContext(1, "admin", false)
	scope, err = svc.ResolveKnowledgeBaseAccess(admin, kb)
	require.NoError(t, err)
	require.True(t, scope.FullAccess)
	require.True(t, scope.CanManage)

	root := enterpriseTestContext(99, "root", true)
	scope, err = svc.ResolveKnowledgeBaseAccess(root, kb)
	require.NoError(t, err)
	require.True(t, scope.FullAccess)
	require.True(t, scope.CanManage)
}

func TestEnterpriseAccessCombinesOrgAndDocumentGrantsWithoutWidening(t *testing.T) {
	repo := &accessRepoStub{
		orgUnitsByUser: map[string][]string{
			"employee": {"team-a", "division-a", enterpriseRootOrgUnitID},
		},
		grants: map[string][]*types.KnowledgeResourceGrant{
			"kb-full": {{
				KnowledgeBaseID:   "kb-full",
				ResourceType:      types.KnowledgeResourceKnowledgeBase,
				ResourceID:        "kb-full",
				SubjectType:       types.GrantSubjectOrgUnit,
				SubjectID:         "division-a",
				Permission:        types.KnowledgeBasePermissionRead,
				Effect:            types.GrantEffectAllow,
				InheritToChildren: true,
			}},
			"kb-doc": {
				{
					KnowledgeBaseID: "kb-doc",
					ResourceType:    types.KnowledgeResourceKnowledge,
					ResourceID:      "doc-1",
					SubjectType:     types.GrantSubjectOrgUnit,
					SubjectID:       "team-a",
					Permission:      types.KnowledgeBasePermissionRead,
					Effect:          types.GrantEffectAllow,
				},
				{
					KnowledgeBaseID: "kb-doc",
					ResourceType:    types.KnowledgeResourceKnowledge,
					ResourceID:      "doc-2",
					SubjectType:     types.GrantSubjectUser,
					SubjectID:       "employee",
					Permission:      types.KnowledgeBasePermissionRead,
					Effect:          types.GrantEffectAllow,
				},
			},
		},
		folders: map[string][]*types.KnowledgeFolder{},
		resources: map[string][]*types.KnowledgeAccessResource{
			"kb-full": {{ID: "full-doc"}},
			"kb-doc":  {{ID: "doc-1"}, {ID: "doc-2"}, {ID: "doc-3"}},
		},
	}
	svc := &enterpriseAccessService{repo: repo}
	ctx := enterpriseTestContext(2, "employee", false)

	fullScope, err := svc.ResolveKnowledgeBaseAccess(
		ctx,
		&types.KnowledgeBase{ID: "kb-full", KnowledgeDomainID: 1},
	)
	require.NoError(t, err)
	require.True(t, fullScope.FullAccess, "an ancestor organization grant must cover descendants")

	documentScope, err := svc.ResolveKnowledgeBaseAccess(
		ctx,
		&types.KnowledgeBase{ID: "kb-doc", KnowledgeDomainID: 1},
	)
	require.NoError(t, err)
	require.True(t, documentScope.Allowed)
	require.False(t, documentScope.FullAccess)
	require.ElementsMatch(t, []string{"doc-1", "doc-2"}, documentScope.KnowledgeIDs)
	require.True(t, documentScope.AllowsKnowledge("doc-1"))
	require.False(t, documentScope.AllowsKnowledge("doc-3"))
}

func TestEnterpriseAccessFolderDenyOverridesKnowledgeBaseAllow(t *testing.T) {
	privateFolderID := "folder-private"
	childFolderID := "folder-private-child"
	publicFolderID := "folder-public"
	repo := &accessRepoStub{
		orgUnitsByUser: map[string][]string{},
		grants: map[string][]*types.KnowledgeResourceGrant{
			"kb-1": {
				{
					KnowledgeBaseID:   "kb-1",
					ResourceType:      types.KnowledgeResourceKnowledgeBase,
					ResourceID:        "kb-1",
					SubjectType:       types.GrantSubjectUser,
					SubjectID:         "viewer",
					Permission:        types.KnowledgeBasePermissionRead,
					Effect:            types.GrantEffectAllow,
					InheritToChildren: true,
				},
				{
					KnowledgeBaseID:   "kb-1",
					ResourceType:      types.KnowledgeResourceFolder,
					ResourceID:        privateFolderID,
					SubjectType:       types.GrantSubjectUser,
					SubjectID:         "viewer",
					Permission:        types.KnowledgeBasePermissionRead,
					Effect:            types.GrantEffectDeny,
					InheritToChildren: true,
				},
			},
		},
		folders: map[string][]*types.KnowledgeFolder{
			"kb-1": {
				{ID: privateFolderID, KnowledgeBaseID: "kb-1", Name: "Private"},
				{ID: childFolderID, KnowledgeBaseID: "kb-1", ParentID: &privateFolderID, Name: "Child"},
				{ID: publicFolderID, KnowledgeBaseID: "kb-1", Name: "Public"},
			},
		},
		resources: map[string][]*types.KnowledgeAccessResource{
			"kb-1": {
				{ID: "doc-private", FolderID: &privateFolderID},
				{ID: "doc-private-child", FolderID: &childFolderID},
				{ID: "doc-public", FolderID: &publicFolderID},
			},
		},
	}
	svc := &enterpriseAccessService{repo: repo}
	scope, err := svc.ResolveKnowledgeBaseAccess(
		enterpriseTestContext(1, "viewer", false),
		&types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1},
	)
	require.NoError(t, err)
	require.True(t, scope.Allowed)
	require.False(t, scope.FullAccess)
	require.Equal(t, []string{"doc-public"}, scope.KnowledgeIDs)
	require.ElementsMatch(t, []string{publicFolderID}, scope.FolderIDs)
}

func TestEnterpriseAccessFolderManageInheritsToDocuments(t *testing.T) {
	folderID := "folder-team"
	repo := &accessRepoStub{
		orgUnitsByUser: map[string][]string{},
		grants: map[string][]*types.KnowledgeResourceGrant{
			"kb-1": {{
				KnowledgeBaseID:   "kb-1",
				ResourceType:      types.KnowledgeResourceFolder,
				ResourceID:        folderID,
				SubjectType:       types.GrantSubjectUser,
				SubjectID:         "manager",
				Permission:        types.KnowledgeBasePermissionManage,
				Effect:            types.GrantEffectAllow,
				InheritToChildren: true,
			}},
		},
		folders: map[string][]*types.KnowledgeFolder{
			"kb-1": {{ID: folderID, KnowledgeBaseID: "kb-1", Name: "Team"}},
		},
		resources: map[string][]*types.KnowledgeAccessResource{
			"kb-1": {{ID: "doc-1", FolderID: &folderID}},
		},
	}
	svc := &enterpriseAccessService{repo: repo}
	scope, err := svc.ResolveKnowledgeBaseAccess(
		enterpriseTestContext(1, "manager", false),
		&types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1},
	)
	require.NoError(t, err)
	require.False(t, scope.CanManage, "folder management must not grant KB settings access")
	require.True(t, scope.ManagesFolder(folderID))
	require.True(t, scope.ManagesKnowledge("doc-1"))
	require.True(t, scope.AllowsKnowledge("doc-1"))
}

func TestEnterpriseAccessDirectEmptyFolderGrantRemainsVisible(t *testing.T) {
	folderID := "folder-empty"
	repo := &accessRepoStub{
		orgUnitsByUser: map[string][]string{},
		grants: map[string][]*types.KnowledgeResourceGrant{
			"kb-1": {{
				KnowledgeBaseID:   "kb-1",
				ResourceType:      types.KnowledgeResourceFolder,
				ResourceID:        folderID,
				SubjectType:       types.GrantSubjectUser,
				SubjectID:         "manager",
				Permission:        types.KnowledgeBasePermissionManage,
				Effect:            types.GrantEffectAllow,
				InheritToChildren: false,
			}},
		},
		folders: map[string][]*types.KnowledgeFolder{
			"kb-1": {{ID: folderID, KnowledgeBaseID: "kb-1", Name: "Empty"}},
		},
		resources: map[string][]*types.KnowledgeAccessResource{"kb-1": {}},
	}
	svc := &enterpriseAccessService{repo: repo}

	scope, err := svc.ResolveKnowledgeBaseAccess(
		enterpriseTestContext(1, "manager", false),
		&types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1},
	)

	require.NoError(t, err)
	require.True(t, scope.Allowed)
	require.False(t, scope.FullAccess)
	require.True(t, scope.ManagesFolder(folderID))
	require.Equal(t, []string{folderID}, scope.FolderIDs)
	require.Empty(t, scope.KnowledgeIDs)
}

func TestEnterpriseAccessFolderManagerCannotAdministerGrantSubjects(t *testing.T) {
	folderID := "folder-managed"
	repo := &accessRepoStub{
		orgUnitsByUser: map[string][]string{},
		grants: map[string][]*types.KnowledgeResourceGrant{
			"kb-1": {
				{
					KnowledgeBaseID:   "kb-1",
					ResourceType:      types.KnowledgeResourceFolder,
					ResourceID:        folderID,
					SubjectType:       types.GrantSubjectUser,
					SubjectID:         "manager",
					Permission:        types.KnowledgeBasePermissionManage,
					Effect:            types.GrantEffectAllow,
					InheritToChildren: true,
				},
				{
					KnowledgeBaseID:   "kb-1",
					ResourceType:      types.KnowledgeResourceFolder,
					ResourceID:        folderID,
					SubjectType:       types.GrantSubjectUser,
					SubjectID:         "reader",
					Permission:        types.KnowledgeBasePermissionRead,
					Effect:            types.GrantEffectAllow,
					InheritToChildren: true,
				},
			},
		},
		folders: map[string][]*types.KnowledgeFolder{
			"kb-1": {{ID: folderID, KnowledgeBaseID: "kb-1", Name: "Managed"}},
		},
		resources: map[string][]*types.KnowledgeAccessResource{
			"kb-1": {{ID: "doc-1", FolderID: &folderID}},
		},
		orgUnits: []*types.OrgUnit{{ID: "org-1", Name: "Finance"}},
		users:    []*types.GrantUser{{ID: "user-1", Email: "user@example.com"}},
	}
	svc := &enterpriseAccessService{repo: repo}
	kb := &types.KnowledgeBase{ID: "kb-1", KnowledgeDomainID: 1}

	_, err := svc.ListResourceGrantSubjects(
		enterpriseTestContext(1, "manager", false),
		kb,
		types.KnowledgeResourceFolder,
		folderID,
		"",
		100,
	)
	require.ErrorIs(t, err, ErrEnterpriseAccessDenied)

	_, err = svc.ListResourceGrantSubjects(
		enterpriseTestContext(1, "reader", false),
		kb,
		types.KnowledgeResourceFolder,
		folderID,
		"",
		100,
	)
	require.ErrorIs(t, err, ErrEnterpriseAccessDenied)

	subjects, err := svc.ListResourceGrantSubjects(
		enterpriseTestContext(1, "system-admin", true),
		kb,
		types.KnowledgeResourceFolder,
		folderID,
		"",
		100,
	)
	require.NoError(t, err)
	require.Equal(t, repo.orgUnits, subjects.OrgUnits)
	require.Equal(t, repo.users, subjects.Users)
}
