package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var (
	ErrKnowledgeDomainNotFound         = errors.New("knowledgeDomain not found")
	ErrKnowledgeDomainHasKnowledgeBase = errors.New("knowledgeDomain has associated knowledge bases")
)

// knowledgeDomainRepository persists knowledge domains and their platform
// configuration projections.
type knowledgeDomainRepository struct {
	db *gorm.DB
}

func NewKnowledgeDomainRepository(db *gorm.DB) interfaces.KnowledgeDomainRepository {
	return &knowledgeDomainRepository{db: db}
}

func (r *knowledgeDomainRepository) CreateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) error {
	if knowledgeDomain.StorageQuota <= 0 {
		knowledgeDomain.StorageQuota = 10 * 1024 * 1024 * 1024
	}
	if knowledgeDomain.StorageUsed < 0 {
		knowledgeDomain.StorageUsed = 0
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(knowledgeDomain).Error; err != nil {
			return err
		}
		accounting := &types.KnowledgeDomainStorage{
			KnowledgeDomainID: knowledgeDomain.ID,
			StorageQuota:      knowledgeDomain.StorageQuota,
			StorageUsed:       knowledgeDomain.StorageUsed,
		}
		return tx.Create(accounting).Error
	})
	if err != nil {
		return err
	}
	return r.hydrateKnowledgeDomains(ctx, []*types.KnowledgeDomain{knowledgeDomain})
}

func (r *knowledgeDomainRepository) GetKnowledgeDomainByID(ctx context.Context, id uint64) (*types.KnowledgeDomain, error) {
	var knowledgeDomain types.KnowledgeDomain
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&knowledgeDomain).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKnowledgeDomainNotFound
		}
		return nil, err
	}
	if err := r.hydrateKnowledgeDomains(ctx, []*types.KnowledgeDomain{&knowledgeDomain}); err != nil {
		return nil, err
	}
	return &knowledgeDomain, nil
}

func (r *knowledgeDomainRepository) GetKnowledgeDomainsByIDs(
	ctx context.Context,
	ids []uint64,
) (map[uint64]*types.KnowledgeDomain, error) {
	if len(ids) == 0 {
		return map[uint64]*types.KnowledgeDomain{}, nil
	}
	var knowledgeDomains []*types.KnowledgeDomain
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&knowledgeDomains).Error; err != nil {
		return nil, err
	}
	if err := r.hydrateKnowledgeDomains(ctx, knowledgeDomains); err != nil {
		return nil, err
	}

	out := make(map[uint64]*types.KnowledgeDomain, len(knowledgeDomains))
	for _, knowledgeDomain := range knowledgeDomains {
		if knowledgeDomain != nil {
			out[knowledgeDomain.ID] = knowledgeDomain
		}
	}
	return out, nil
}

func (r *knowledgeDomainRepository) ListKnowledgeDomains(ctx context.Context) ([]*types.KnowledgeDomain, error) {
	var knowledgeDomains []*types.KnowledgeDomain
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&knowledgeDomains).Error; err != nil {
		return nil, err
	}
	if err := r.hydrateKnowledgeDomains(ctx, knowledgeDomains); err != nil {
		return nil, err
	}
	return knowledgeDomains, nil
}

func (r *knowledgeDomainRepository) SearchKnowledgeDomains(
	ctx context.Context,
	keyword string,
	knowledgeDomainID uint64,
	page,
	pageSize int,
) ([]*types.KnowledgeDomain, int64, error) {
	var knowledgeDomains []*types.KnowledgeDomain
	var total int64

	query := r.db.WithContext(ctx).Model(&types.KnowledgeDomain{})
	if knowledgeDomainID > 0 && keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		query = query.Where(
			"id = ? OR code LIKE ? OR name LIKE ? OR name_en LIKE ? OR description LIKE ?",
			knowledgeDomainID,
			"%"+escaped+"%",
			"%"+escaped+"%",
			"%"+escaped+"%",
			"%"+escaped+"%",
		)
	} else if knowledgeDomainID > 0 {
		query = query.Where("id = ?", knowledgeDomainID)
	} else if keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		query = query.Where(
			"code LIKE ? OR name LIKE ? OR name_en LIKE ? OR description LIKE ?",
			"%"+escaped+"%",
			"%"+escaped+"%",
			"%"+escaped+"%",
			"%"+escaped+"%",
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	if err := query.Order("created_at DESC").Find(&knowledgeDomains).Error; err != nil {
		return nil, 0, err
	}
	if err := r.hydrateKnowledgeDomains(ctx, knowledgeDomains); err != nil {
		return nil, 0, err
	}
	return knowledgeDomains, total, nil
}

// UpdateKnowledgeDomain updates knowledge-domain identity and storage accounting only.
func (r *knowledgeDomainRepository) UpdateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
	result := tx.Model(&types.KnowledgeDomain{}).
		Where("id = ?", knowledgeDomain.ID).
		Select("code", "name", "name_en", "description", "status", "updated_at").
		Updates(knowledgeDomain)
		if result.Error != nil {
			return result.Error
		}

		accounting := &types.KnowledgeDomainStorage{
			KnowledgeDomainID: knowledgeDomain.ID,
			StorageQuota:      knowledgeDomain.StorageQuota,
			StorageUsed:       knowledgeDomain.StorageUsed,
			UpdatedAt:         time.Now(),
		}
		if accounting.StorageUsed < 0 {
			accounting.StorageUsed = 0
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "knowledge_domain_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"storage_quota",
				"storage_used",
				"updated_at",
			}),
		}).Create(accounting).Error
	})
}

// UpdatePlatformRuntimeConfig persists the singleton platform configuration.
func (r *knowledgeDomainRepository) UpdatePlatformRuntimeConfig(
	ctx context.Context,
	runtimeConfig *types.PlatformRuntimeConfig,
) error {
	if runtimeConfig.ID == 0 {
		runtimeConfig.ID = 1
	}
	runtimeConfig.UpdatedAt = time.Now()
	if runtimeConfig.RetrieverEngines.Engines == nil {
		runtimeConfig.RetrieverEngines.Engines = []types.RetrieverEngineParams{}
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"retriever_engines",
			"context_config",
			"web_search_config",
			"parser_engine_config",
			"storage_engine_config",
			"retrieval_config",
			"updated_at",
		}),
	}).Create(runtimeConfig).Error
}

// GetPlatformRuntimeConfig returns the singleton platform configuration. An
// unsaved default is returned while a fresh database is being initialized.
func (r *knowledgeDomainRepository) GetPlatformRuntimeConfig(
	ctx context.Context,
) (*types.PlatformRuntimeConfig, error) {
	return r.getPlatformRuntimeConfig(ctx)
}

// DeleteKnowledgeDomain soft-deletes the domain and all active membership rows.
func (r *knowledgeDomainRepository) DeleteKnowledgeDomain(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("knowledge_domain_id = ?", id).Delete(&types.KnowledgeDomainAdmin{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&types.KnowledgeDomain{}).Error
	})
}

func (r *knowledgeDomainRepository) AdjustStorageUsed(
	ctx context.Context,
	knowledgeDomainID uint64,
	delta int64,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var accounting types.KnowledgeDomainStorage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("knowledge_domain_id = ?", knowledgeDomainID).
			First(&accounting).Error; err != nil {
			return err
		}

		accounting.StorageUsed += delta
		if accounting.StorageUsed < 0 {
			logger.Errorf(
				ctx,
				"knowledgeDomain storage used is negative %d: %d",
				accounting.KnowledgeDomainID,
				accounting.StorageUsed,
			)
			accounting.StorageUsed = 0
		}
		return tx.Save(&accounting).Error
	})
}

func (r *knowledgeDomainRepository) BulkSetStorageQuota(
	ctx context.Context,
	quotaBytes int64,
) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&types.KnowledgeDomainStorage{}).
		Where("1 = 1").
		Update("storage_quota", quotaBytes)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *knowledgeDomainRepository) hydrateKnowledgeDomains(ctx context.Context, knowledgeDomains []*types.KnowledgeDomain) error {
	if len(knowledgeDomains) == 0 {
		return nil
	}

	ids := make([]uint64, 0, len(knowledgeDomains))
	for _, knowledgeDomain := range knowledgeDomains {
		if knowledgeDomain != nil {
			ids = append(ids, knowledgeDomain.ID)
		}
	}

	var accountingRows []types.KnowledgeDomainStorage
	if err := r.db.WithContext(ctx).
		Where("knowledge_domain_id IN ?", ids).
		Find(&accountingRows).Error; err != nil {
		return err
	}
	accountingByDomain := make(map[uint64]types.KnowledgeDomainStorage, len(accountingRows))
	for _, row := range accountingRows {
		accountingByDomain[row.KnowledgeDomainID] = row
	}

	runtimeConfig, err := r.getPlatformRuntimeConfig(ctx)
	if err != nil {
		return err
	}

	for _, knowledgeDomain := range knowledgeDomains {
		if knowledgeDomain == nil {
			continue
		}
		if accounting, ok := accountingByDomain[knowledgeDomain.ID]; ok {
			knowledgeDomain.StorageQuota = accounting.StorageQuota
			knowledgeDomain.StorageUsed = accounting.StorageUsed
		}
		knowledgeDomain.RetrieverEngines = runtimeConfig.RetrieverEngines
		knowledgeDomain.ContextConfig = runtimeConfig.ContextConfig
		knowledgeDomain.WebSearchConfig = runtimeConfig.WebSearchConfig
		knowledgeDomain.ParserEngineConfig = runtimeConfig.ParserEngineConfig
		knowledgeDomain.StorageEngineConfig = runtimeConfig.StorageEngineConfig
		knowledgeDomain.RetrievalConfig = runtimeConfig.RetrievalConfig
	}
	return nil
}

func (r *knowledgeDomainRepository) getPlatformRuntimeConfig(
	ctx context.Context,
) (*types.PlatformRuntimeConfig, error) {
	var runtimeConfig types.PlatformRuntimeConfig
	err := r.db.WithContext(ctx).Where("id = ?", 1).First(&runtimeConfig).Error
	if err == nil {
		return &runtimeConfig, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &types.PlatformRuntimeConfig{
		ID: 1,
		RetrieverEngines: types.RetrieverEngines{
			Engines: []types.RetrieverEngineParams{},
		},
	}, nil
}
