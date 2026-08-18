package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const knowledgeFolderTestDDL = `
CREATE TABLE IF NOT EXISTS knowledge_folders (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_domain_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    parent_id VARCHAR(36),
    name VARCHAR(255) NOT NULL,
    relative_path VARCHAR(2048) NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_base_id, relative_path)
);

CREATE TABLE IF NOT EXISTS knowledge_resource_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_domain_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    resource_type VARCHAR(24) NOT NULL,
    resource_id VARCHAR(36) NOT NULL,
    subject_type VARCHAR(16) NOT NULL,
    subject_id VARCHAR(36) NOT NULL,
    permission VARCHAR(16) NOT NULL,
    effect VARCHAR(8) NOT NULL,
    inherit_to_children BOOLEAN NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func TestResolveKnowledgeFolderPathCreatesHierarchyIdempotently(t *testing.T) {
	db := setupKnowledgeTestDB(t)
	require.NoError(t, db.Exec(knowledgeFolderTestDDL).Error)
	repo := &knowledgeRepository{db: db}
	ctx := context.Background()
	kbID := uuid.New().String()

	folder, err := repo.ResolveKnowledgeFolderPath(ctx, 1, kbID, "Policies/Finance")
	require.NoError(t, err)
	require.NotNil(t, folder)
	assert.Equal(t, "Finance", folder.Name)
	assert.Equal(t, "Policies/Finance", folder.RelativePath)
	require.NotNil(t, folder.ParentID)

	parent, err := repo.GetKnowledgeFolderByID(ctx, 1, *folder.ParentID)
	require.NoError(t, err)
	assert.Equal(t, "Policies", parent.RelativePath)
	assert.Nil(t, parent.ParentID)

	sameFolder, err := repo.ResolveKnowledgeFolderPath(ctx, 1, kbID, "Policies/Finance")
	require.NoError(t, err)
	assert.Equal(t, folder.ID, sameFolder.ID)

	var count int64
	require.NoError(t, db.Table("knowledge_folders").
		Where("knowledge_base_id = ?", kbID).
		Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestListKnowledgeFoldersRestrictsDocumentLevelReaders(t *testing.T) {
	db := setupKnowledgeTestDB(t)
	require.NoError(t, db.Exec(knowledgeFolderTestDDL).Error)
	repo := &knowledgeRepository{db: db}
	ctx := context.Background()
	kbID := uuid.New().String()

	allowedFolder, err := repo.ResolveKnowledgeFolderPath(ctx, 1, kbID, "Policies/Finance")
	require.NoError(t, err)
	_, err = repo.ResolveKnowledgeFolderPath(ctx, 1, kbID, "Private/HR")
	require.NoError(t, err)

	knowledgeID := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO knowledges (
			id, knowledge_domain_id, knowledge_base_id, folder_id, type, title, source, parse_status
		) VALUES (?, 1, ?, ?, 'file', 'finance.pdf', 'upload', 'completed')
	`, knowledgeID, kbID, allowedFolder.ID).Error)

	full, err := repo.ListKnowledgeFolders(ctx, 1, kbID, nil)
	require.NoError(t, err)
	assert.Len(t, full, 4)

	restricted, err := repo.ListKnowledgeFolders(ctx, 1, kbID, []string{knowledgeID})
	require.NoError(t, err)
	require.Len(t, restricted, 2)
	assert.Equal(t, "Policies", restricted[0].RelativePath)
	assert.Equal(t, "Policies/Finance", restricted[1].RelativePath)

	none, err := repo.ListKnowledgeFolders(ctx, 1, kbID, []string{})
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestDeleteKnowledgeFolderRemovesSubtreeACLsAfterDocumentsAreCleaned(t *testing.T) {
	db := setupKnowledgeTestDB(t)
	require.NoError(t, db.Exec(knowledgeFolderTestDDL).Error)
	repo := &knowledgeRepository{db: db}
	ctx := context.Background()
	kbID := uuid.NewString()

	parent, err := repo.ResolveKnowledgeFolderPath(ctx, 1, kbID, "Policies")
	require.NoError(t, err)
	child, err := repo.ResolveKnowledgeFolderPath(ctx, 1, kbID, "Policies/Finance")
	require.NoError(t, err)

	knowledgeID := uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO knowledges (
			id, knowledge_domain_id, knowledge_base_id, folder_id, type, title, source, parse_status
		) VALUES (?, 1, ?, ?, 'file', 'finance.pdf', 'upload', 'completed')
	`, knowledgeID, kbID, child.ID).Error)
	require.ErrorIs(
		t,
		repo.DeleteKnowledgeFolder(ctx, 1, kbID, parent.ID),
		ErrKnowledgeFolderNotEmpty,
	)

	require.NoError(t, db.Exec(`
		INSERT INTO knowledge_resource_grants (
			knowledge_domain_id, knowledge_base_id, resource_type, resource_id,
			subject_type, subject_id, permission, effect, inherit_to_children
		) VALUES
			(1, ?, 'folder', ?, 'user', ?, 'manage', 'allow', 1),
			(1, ?, 'knowledge', ?, 'user', ?, 'read', 'deny', 0)
	`, kbID, child.ID, uuid.NewString(), kbID, knowledgeID, uuid.NewString()).Error)

	require.NoError(t, db.Model(&struct {
		DeletedAt *time.Time `gorm:"column:deleted_at"`
	}{}).Table("knowledges").Where("id = ?", knowledgeID).
		Update("deleted_at", time.Now()).Error)

	require.NoError(t, repo.DeleteKnowledgeFolder(ctx, 1, kbID, parent.ID))

	var folderCount int64
	require.NoError(t, db.Table("knowledge_folders").
		Where("id IN ?", []string{parent.ID, child.ID}).
		Count(&folderCount).Error)
	require.Zero(t, folderCount)

	var grantCount int64
	require.NoError(t, db.Table("knowledge_resource_grants").
		Where(
			"(resource_type = 'folder' AND resource_id IN ?) OR (resource_type = 'knowledge' AND resource_id = ?)",
			[]string{parent.ID, child.ID},
			knowledgeID,
		).
		Count(&grantCount).Error)
	require.Zero(t, grantCount)
}
