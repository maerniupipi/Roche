package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestWorkdayMoveChangesOrgMembershipButKeepsDirectGrant(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&types.User{},
		&types.OrgUnit{},
		&types.UserOrgMembership{},
		&types.ExternalOrgUnit{},
		&types.ExternalWorker{},
		&types.IntegrationSyncRun{},
		&types.IntegrationEvent{},
		&types.KnowledgeResourceGrant{},
	); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	user := &types.User{
		ID:           uuid.NewString(),
		Username:     "employee",
		Email:        "employee@example.com",
		PasswordHash: "not-used",
		Status:       types.UserStatusNormal,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	run := &types.IntegrationSyncRun{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ConnectionKey: "test",
		Mode:          types.IntegrationSyncModeIncremental,
		Status:        types.IntegrationSyncRunning,
		CursorBefore:  types.JSON([]byte(`{}`)),
		CursorAfter:   types.JSON([]byte(`{}`)),
		Counters:      types.JSON([]byte(`{}`)),
		CreatedAt:     time.Now(),
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	repo := &enterpriseIntegrationRepository{db: db}
	_, err = repo.ApplyOrgUnitPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayOrgUnitRecord{
		{ExternalID: "org-a", Code: "A", Name: "Department A", Status: types.OrgUnitStatusActive},
		{ExternalID: "org-b", Code: "B", Name: "Department B", Status: types.OrgUnitStatusActive},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	directGrant := &types.KnowledgeResourceGrant{
		KnowledgeDomainID: 1,
		KnowledgeBaseID:   "kb-a",
		ResourceType:      types.KnowledgeResourceKnowledgeBase,
		ResourceID:        "kb-a",
		SubjectType:       types.GrantSubjectUser,
		SubjectID:         user.ID,
		Permission:        types.KnowledgeBasePermissionRead,
		Effect:            types.GrantEffectAllow,
		InheritToChildren: true,
	}
	if err := db.Create(directGrant).Error; err != nil {
		t.Fatal(err)
	}

	_, err = repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:           "worker-1",
		CorporateEmail:       user.Email,
		PrimaryOrgExternalID: "org-a",
		Status:               types.ExternalWorkerActive,
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	assertActiveWorkdayMembership(t, db, user.ID, "org-a")

	_, err = repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:           "worker-1",
		CorporateEmail:       user.Email,
		PrimaryOrgExternalID: "org-b",
		Status:               types.ExternalWorkerActive,
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	assertActiveWorkdayMembership(t, db, user.ID, "org-b")

	var grants int64
	if err := db.Model(&types.KnowledgeResourceGrant{}).
		Where("subject_type = ? AND subject_id = ?", types.GrantSubjectUser, user.ID).
		Count(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if grants != 1 {
		t.Fatalf("direct user grants = %d, want 1", grants)
	}

	_, err = repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:           "worker-1",
		CorporateEmail:       user.Email,
		PrimaryOrgExternalID: "org-b",
		Status:               types.ExternalWorkerInactive,
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	var active int64
	if err := db.Model(&types.UserOrgMembership{}).
		Where(
			"user_id = ? AND source = ? AND status = ?",
			user.ID,
			types.OrgUnitSourceWorkday,
			types.OrgUnitStatusActive,
		).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active Workday memberships after deactivation = %d, want 0", active)
	}
}

func TestWorkdayOrgParentResolvesWhenParentArrivesOnLaterPage(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&types.OrgUnit{},
		&types.ExternalOrgUnit{},
		&types.IntegrationSyncRun{},
	); err != nil {
		t.Fatal(err)
	}

	run := &types.IntegrationSyncRun{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ConnectionKey: "test",
		Mode:          types.IntegrationSyncModeIncremental,
		Status:        types.IntegrationSyncRunning,
		CursorBefore:  types.JSON([]byte(`{}`)),
		CursorAfter:   types.JSON([]byte(`{}`)),
		Counters:      types.JSON([]byte(`{}`)),
		CreatedAt:     time.Now(),
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	repo := &enterpriseIntegrationRepository{db: db}
	if _, err := repo.ApplyOrgUnitPage(
		ctx,
		run.ID,
		types.EnterpriseProviderWorkday,
		[]types.WorkdayOrgUnitRecord{{
			ExternalID:       "child",
			ParentExternalID: "parent",
			Name:             "Child Department",
			Status:           types.OrgUnitStatusActive,
		}},
		"page-2",
	); err != nil {
		t.Fatal(err)
	}

	var childProjection types.ExternalOrgUnit
	if err := db.Where(
		"provider = ? AND external_org_id = ?",
		types.EnterpriseProviderWorkday,
		"child",
	).First(&childProjection).Error; err != nil {
		t.Fatal(err)
	}
	var child types.OrgUnit
	if err := db.First(&child, "id = ?", *childProjection.OrgUnitID).Error; err != nil {
		t.Fatal(err)
	}
	if child.ParentID != nil {
		t.Fatalf("child parent before parent page = %v, want nil", *child.ParentID)
	}

	if _, err := repo.ApplyOrgUnitPage(
		ctx,
		run.ID,
		types.EnterpriseProviderWorkday,
		[]types.WorkdayOrgUnitRecord{{
			ExternalID: "parent",
			Name:       "Parent Department",
			Status:     types.OrgUnitStatusActive,
		}},
		"",
	); err != nil {
		t.Fatal(err)
	}

	var parentProjection types.ExternalOrgUnit
	if err := db.Where(
		"provider = ? AND external_org_id = ?",
		types.EnterpriseProviderWorkday,
		"parent",
	).First(&parentProjection).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&child, "id = ?", *childProjection.OrgUnitID).Error; err != nil {
		t.Fatal(err)
	}
	if child.ParentID == nil || *child.ParentID != *parentProjection.OrgUnitID {
		t.Fatalf(
			"child parent after parent page = %v, want %s",
			child.ParentID,
			*parentProjection.OrgUnitID,
		)
	}
}

func TestListExternalDirectoryIncludesUnlinkedWorkers(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&types.ExternalOrgUnit{}, &types.ExternalWorker{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	orgID := "canonical-finance"
	org := &types.ExternalOrgUnit{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ExternalOrgID: "WD-FIN",
		OrgUnitID:     &orgID,
		Name:          "Finance",
		Status:        types.OrgUnitStatusActive,
		Attributes:    types.JSON([]byte(`{"cost_center":"CC-1001"}`)),
		Checksum:      "org-checksum",
		LastSeenAt:    now,
	}
	if err := db.Create(org).Error; err != nil {
		t.Fatal(err)
	}
	workers := []*types.ExternalWorker{
		{
			ID:                   uuid.NewString(),
			Provider:             types.EnterpriseProviderWorkday,
			ExternalWorkerID:     "WD-WORKER-1",
			PrimaryOrgExternalID: stringPointer("WD-FIN"),
			CorporateEmail:       "linked@example.com",
			WorkerStatus:         types.ExternalWorkerActive,
			Attributes:           types.JSON([]byte(`{"display_name":"Linked Worker"}`)),
			Checksum:             "worker-checksum-1",
			LastSeenAt:           now,
		},
		{
			ID:                   uuid.NewString(),
			Provider:             types.EnterpriseProviderWorkday,
			ExternalWorkerID:     "WD-WORKER-2",
			PrimaryOrgExternalID: stringPointer("WD-FIN"),
			CorporateEmail:       "unlinked@example.com",
			WorkerStatus:         types.ExternalWorkerActive,
			Attributes:           types.JSON([]byte(`{"display_name":"Unlinked Worker"}`)),
			Checksum:             "worker-checksum-2",
			LastSeenAt:           now,
		},
	}
	linkedUserID := uuid.NewString()
	workers[0].UserID = &linkedUserID
	if err := db.Create(workers).Error; err != nil {
		t.Fatal(err)
	}

	repo := &enterpriseIntegrationRepository{db: db}
	units, err := repo.ListExternalOrgUnits(context.Background(), types.EnterpriseProviderWorkday)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].ExternalOrgID != "WD-FIN" {
		t.Fatalf("units = %+v, want Workday Finance", units)
	}

	got, total, err := repo.ListExternalWorkers(
		context.Background(),
		types.EnterpriseProviderWorkday,
		"WD-FIN",
		"",
		0,
		200,
	)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(got) != 2 {
		t.Fatalf("workers total=%d len=%d, want 2", total, len(got))
	}
	if got[0].UserID == nil || got[1].UserID != nil {
		t.Fatalf("linked states = %v/%v, want linked/unlinked", got[0].UserID, got[1].UserID)
	}
}

func TestApplyWorkerPageAutoProvisionsUser(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&types.User{},
		&types.OrgUnit{},
		&types.UserOrgMembership{},
		&types.ExternalOrgUnit{},
		&types.ExternalWorker{},
		&types.IntegrationSyncRun{},
		&types.IntegrationEvent{},
	); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	run := &types.IntegrationSyncRun{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ConnectionKey: "test",
		Mode:          types.IntegrationSyncModeIncremental,
		Status:        types.IntegrationSyncRunning,
		CursorBefore:  types.JSON([]byte(`{}`)),
		CursorAfter:   types.JSON([]byte(`{}`)),
		Counters:      types.JSON([]byte(`{}`)),
		CreatedAt:     time.Now(),
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	repo := &enterpriseIntegrationRepository{db: db}
	if _, err := repo.ApplyOrgUnitPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayOrgUnitRecord{
		{ExternalID: "WD-IT", Code: "IT", Name: "Informatics", Status: types.OrgUnitStatusActive},
	}, ""); err != nil {
		t.Fatal(err)
	}

	// An active worker with no matching user must be auto-provisioned and linked.
	_, err = repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:           "employee-wid-1",
		CorporateEmail:       "alice@roche.example.com",
		PrimaryOrgExternalID: "WD-IT",
		Status:               types.ExternalWorkerActive,
		Attributes: map[string]any{
			"user_id":          "ALICE1",
			"employee_id":      "100001",
			"display_name":     "Alice Zhang",
			"supervisory_code": "IT",
			"supervisory_name": "Informatics",
		},
	}}, "")
	if err != nil {
		t.Fatal(err)
	}

	var user types.User
	if err := db.Where("LOWER(email) = ?", "alice@roche.example.com").First(&user).Error; err != nil {
		t.Fatalf("auto-provisioned user not found: %v", err)
	}
	if user.Username != "ALICE1" {
		t.Fatalf("username = %q, want ALICE1", user.Username)
	}
	if user.EmployeeID != "100001" {
		t.Fatalf("employee_id = %q, want 100001", user.EmployeeID)
	}
	if user.Account != "ALICE1" {
		t.Fatalf("account = %q, want ALICE1", user.Account)
	}
	if user.EnglishName != "Alice Zhang" {
		t.Fatalf("english_name = %q, want Alice Zhang", user.EnglishName)
	}
	if user.DepartmentCode != "IT" || user.DepartmentName != "Informatics" {
		t.Fatalf("department = %q/%q", user.DepartmentCode, user.DepartmentName)
	}
	if user.Status != types.UserStatusNormal {
		t.Fatalf("active worker user status = %d, want %d (normal)", user.Status, types.UserStatusNormal)
	}
	var worker types.ExternalWorker
	if err := db.Where(
		"provider = ? AND external_worker_id = ?",
		types.EnterpriseProviderWorkday,
		"employee-wid-1",
	).First(&worker).Error; err != nil {
		t.Fatal(err)
	}
	if worker.UserID == nil || *worker.UserID != user.ID {
		t.Fatalf("worker user_id = %v, want %s", worker.UserID, user.ID)
	}
	assertActiveWorkdayMembership(t, db, user.ID, "WD-IT")

	// A terminated worker must still be imported into users with status 2
	// (resigned) and be linked from the external_workers row.
	_, err = repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:     "ex-employee-wid-1",
		CorporateEmail: "bob@roche.example.com",
		Status:         types.ExternalWorkerInactive,
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	var bob types.User
	if err := db.Where("LOWER(email) = ?", "bob@roche.example.com").First(&bob).Error; err != nil {
		t.Fatalf("terminated worker user not imported: %v", err)
	}
	if bob.Status != types.UserStatusResigned {
		t.Fatalf("terminated worker user status = %d, want %d (resigned)", bob.Status, types.UserStatusResigned)
	}
	var inactiveWorker types.ExternalWorker
	if err := db.Where(
		"provider = ? AND external_worker_id = ?",
		types.EnterpriseProviderWorkday,
		"ex-employee-wid-1",
	).First(&inactiveWorker).Error; err != nil {
		t.Fatal(err)
	}
	if inactiveWorker.UserID == nil || *inactiveWorker.UserID != bob.ID {
		t.Fatalf("inactive worker user_id = %v, want %s", inactiveWorker.UserID, bob.ID)
	}
}

func TestApplyWorkerPageUsesExistingUser(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&types.User{},
		&types.OrgUnit{},
		&types.UserOrgMembership{},
		&types.ExternalOrgUnit{},
		&types.ExternalWorker{},
		&types.IntegrationSyncRun{},
		&types.IntegrationEvent{},
	); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	existing := &types.User{
		ID:           uuid.NewString(),
		Username:     "existing-user",
		Email:        "carol@roche.example.com",
		PasswordHash: "not-used",
	}
	if err := db.Create(existing).Error; err != nil {
		t.Fatal(err)
	}
	run := &types.IntegrationSyncRun{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ConnectionKey: "test",
		Mode:          types.IntegrationSyncModeIncremental,
		Status:        types.IntegrationSyncRunning,
		CursorBefore:  types.JSON([]byte(`{}`)),
		CursorAfter:   types.JSON([]byte(`{}`)),
		Counters:      types.JSON([]byte(`{}`)),
		CreatedAt:     time.Now(),
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	repo := &enterpriseIntegrationRepository{db: db}
	if _, err := repo.ApplyOrgUnitPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayOrgUnitRecord{
		{ExternalID: "WD-IT", Code: "IT", Name: "Informatics", Status: types.OrgUnitStatusActive},
	}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:           "employee-wid-2",
		CorporateEmail:       existing.Email,
		PrimaryOrgExternalID: "WD-IT",
		Status:               types.ExternalWorkerActive,
	}}, ""); err != nil {
		t.Fatal(err)
	}

	var userCount int64
	if err := db.Model(&types.User{}).Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Fatalf("user count = %d, want 1 (no duplicate)", userCount)
	}
	var worker types.ExternalWorker
	if err := db.Where(
		"provider = ? AND external_worker_id = ?",
		types.EnterpriseProviderWorkday,
		"employee-wid-2",
	).First(&worker).Error; err != nil {
		t.Fatal(err)
	}
	if worker.UserID == nil || *worker.UserID != existing.ID {
		t.Fatalf("worker user_id = %v, want %s", worker.UserID, existing.ID)
	}

	// Full-sync semantics: when the same worker turns terminated, the linked
	// (non-banned) user must be moved to status 2 (resigned).
	if _, err := repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:     "employee-wid-2",
		CorporateEmail: existing.Email,
		Status:         types.ExternalWorkerInactive,
	}}, ""); err != nil {
		t.Fatal(err)
	}
	var synced types.User
	if err := db.Where("id = ?", existing.ID).First(&synced).Error; err != nil {
		t.Fatal(err)
	}
	if synced.Status != types.UserStatusResigned {
		t.Fatalf("existing user status after termination = %d, want %d (resigned)", synced.Status, types.UserStatusResigned)
	}
}

func TestApplyWorkerPageBannedUserNotOverwritten(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&types.User{},
		&types.OrgUnit{},
		&types.UserOrgMembership{},
		&types.ExternalOrgUnit{},
		&types.ExternalWorker{},
		&types.IntegrationSyncRun{},
		&types.IntegrationEvent{},
	); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	banned := &types.User{
		ID:             uuid.NewString(),
		Username:       "banned-user",
		Email:          "dave@roche.example.com",
		PasswordHash:   "not-used",
		Status:         types.UserStatusBanned,
		EnglishName:    "Dave Original",
		DepartmentCode: "OLD",
	}
	if err := db.Create(banned).Error; err != nil {
		t.Fatal(err)
	}
	run := &types.IntegrationSyncRun{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ConnectionKey: "test",
		Mode:          types.IntegrationSyncModeIncremental,
		Status:        types.IntegrationSyncRunning,
		CursorBefore:  types.JSON([]byte(`{}`)),
		CursorAfter:   types.JSON([]byte(`{}`)),
		Counters:      types.JSON([]byte(`{}`)),
		CreatedAt:     time.Now(),
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	repo := &enterpriseIntegrationRepository{db: db}
	if _, err := repo.ApplyOrgUnitPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayOrgUnitRecord{
		{ExternalID: "WD-IT", Code: "IT", Name: "Informatics", Status: types.OrgUnitStatusActive},
	}, ""); err != nil {
		t.Fatal(err)
	}

	// external_workers must always be updated, but a banned user's row in
	// users must never be overwritten by directory data.
	if _, err := repo.ApplyWorkerPage(ctx, run.ID, types.EnterpriseProviderWorkday, []types.WorkdayWorkerRecord{{
		ExternalID:           "employee-wid-3",
		CorporateEmail:       banned.Email,
		PrimaryOrgExternalID: "WD-IT",
		Status:               types.ExternalWorkerActive,
		Attributes: map[string]any{
			"user_id":          "DAVE1",
			"employee_id":      "100003",
			"display_name":     "Dave New",
			"supervisory_code": "IT",
			"supervisory_name": "Informatics",
		},
	}}, ""); err != nil {
		t.Fatal(err)
	}

	var after types.User
	if err := db.Where("id = ?", banned.ID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != types.UserStatusBanned {
		t.Fatalf("banned user status overwritten to %d", after.Status)
	}
	if after.EnglishName != "Dave Original" {
		t.Fatalf("banned user english_name overwritten to %q", after.EnglishName)
	}
	if after.DepartmentCode != "OLD" {
		t.Fatalf("banned user department_code overwritten to %q", after.DepartmentCode)
	}

	var worker types.ExternalWorker
	if err := db.Where(
		"provider = ? AND external_worker_id = ?",
		types.EnterpriseProviderWorkday,
		"employee-wid-3",
	).First(&worker).Error; err != nil {
		t.Fatal(err)
	}
	if worker.UserID == nil || *worker.UserID != banned.ID {
		t.Fatalf("banned user worker not linked: %v", worker.UserID)
	}
	if worker.WorkerStatus != types.ExternalWorkerActive {
		t.Fatalf("external_worker status = %s, want active (always updated)", worker.WorkerStatus)
	}
}

func stringPointer(value string) *string { return &value }

func assertActiveWorkdayMembership(
	t *testing.T,
	db *gorm.DB,
	userID, externalOrgID string,
) {
	t.Helper()
	var projection types.ExternalOrgUnit
	if err := db.Where(
		"provider = ? AND external_org_id = ?",
		types.EnterpriseProviderWorkday,
		externalOrgID,
	).First(&projection).Error; err != nil {
		t.Fatal(err)
	}
	var membership types.UserOrgMembership
	if err := db.Where(
		"user_id = ? AND org_unit_id = ?",
		userID,
		*projection.OrgUnitID,
	).First(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if membership.Status != types.OrgUnitStatusActive ||
		membership.Source != types.OrgUnitSourceWorkday ||
		!membership.IsPrimary {
		t.Fatalf("unexpected membership: %+v", membership)
	}
}
