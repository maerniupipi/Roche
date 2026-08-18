package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

// setupTestDB creates an in-memory SQLite database with knowledge-domain tables.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.KnowledgeDomain{},
		&types.KnowledgeDomainStorage{},
		&types.KnowledgeDomainAdmin{},
		&types.PlatformRuntimeConfig{},
	))
	return db
}

func TestDeleteKnowledgeDomainSoftDeletesDomain(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	repo := NewKnowledgeDomainRepository(db)

	knowledgeDomain := &types.KnowledgeDomain{Name: "gone", Status: "active"}
	require.NoError(t, db.Create(knowledgeDomain).Error)

	require.NoError(t, repo.DeleteKnowledgeDomain(ctx, knowledgeDomain.ID))

	var knowledgeDomainCount int64
	require.NoError(t, db.Model(&types.KnowledgeDomain{}).Count(&knowledgeDomainCount).Error)
	assert.Equal(t, int64(0), knowledgeDomainCount)

	// Unscoped: the domain row still exists but is soft-deleted.
	var rawKnowledgeDomainCount int64
	require.NoError(t, db.Unscoped().Model(&types.KnowledgeDomain{}).Count(&rawKnowledgeDomainCount).Error)
	assert.Equal(t, int64(1), rawKnowledgeDomainCount)

}

func TestKnowledgeDomainRepository_SplitsDomainStorageAndRuntimeConfig(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	repo := NewKnowledgeDomainRepository(db)

	require.NoError(t, db.Create(&types.PlatformRuntimeConfig{
		ID: 1,
		RetrieverEngines: types.RetrieverEngines{Engines: []types.RetrieverEngineParams{
			{
				RetrieverType:       types.VectorRetrieverType,
				RetrieverEngineType: types.MilvusRetrieverEngineType,
			},
		}},
		RetrievalConfig: &types.RetrievalConfig{EmbeddingTopK: 25},
	}).Error)

	domain := &types.KnowledgeDomain{
		Name:         "Legal Knowledge",
		Status:       "active",
		StorageQuota: 2048,
	}
	require.NoError(t, repo.CreateKnowledgeDomain(ctx, domain))
	assert.NotEmpty(t, domain.Code)
	assert.Equal(t, int64(2048), domain.StorageQuota)
	require.NotNil(t, domain.RetrievalConfig)
	assert.Equal(t, 25, domain.RetrievalConfig.EmbeddingTopK)

	var persistedDomain types.KnowledgeDomain
	require.NoError(t, db.First(&persistedDomain, domain.ID).Error)
	assert.Equal(t, "Legal Knowledge", persistedDomain.Name)

	var accounting types.KnowledgeDomainStorage
	require.NoError(t, db.First(&accounting, "knowledge_domain_id = ?", domain.ID).Error)
	assert.Equal(t, int64(2048), accounting.StorageQuota)

	domain.Name = "Legal and Compliance"
	domain.StorageQuota = 4096
	domain.RetrievalConfig = &types.RetrievalConfig{EmbeddingTopK: 99}
	require.NoError(t, repo.UpdateKnowledgeDomain(ctx, domain))

	var runtimeConfig types.PlatformRuntimeConfig
	require.NoError(t, db.First(&runtimeConfig, "id = ?", 1).Error)
	require.NotNil(t, runtimeConfig.RetrievalConfig)
	assert.Equal(t, 25, runtimeConfig.RetrievalConfig.EmbeddingTopK,
		"updating domain identity must not change platform runtime configuration")

	runtimeConfig.RetrievalConfig = &types.RetrievalConfig{EmbeddingTopK: 40}
	require.NoError(t, repo.UpdatePlatformRuntimeConfig(ctx, &runtimeConfig))
	storedRuntimeConfig, err := repo.GetPlatformRuntimeConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, storedRuntimeConfig.RetrievalConfig)
	assert.Equal(t, 40, storedRuntimeConfig.RetrievalConfig.EmbeddingTopK)

	loaded, err := repo.GetKnowledgeDomainByID(ctx, domain.ID)
	require.NoError(t, err)
	assert.Equal(t, "Legal and Compliance", loaded.Name)
	assert.Equal(t, int64(4096), loaded.StorageQuota)
	require.NotNil(t, loaded.RetrievalConfig)
	assert.Equal(t, 40, loaded.RetrievalConfig.EmbeddingTopK)
}
