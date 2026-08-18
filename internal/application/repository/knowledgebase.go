package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var ErrKnowledgeBaseNotFound = errors.New("knowledge base not found")

// knowledgeBaseRepository implements the KnowledgeBaseRepository interface
type knowledgeBaseRepository struct {
	db *gorm.DB
}

// NewKnowledgeBaseRepository creates a new knowledge base repository
func NewKnowledgeBaseRepository(db *gorm.DB) interfaces.KnowledgeBaseRepository {
	return &knowledgeBaseRepository{db: db}
}

// CreateKnowledgeBase creates a new knowledge base
func (r *knowledgeBaseRepository) CreateKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) error {
	return r.db.WithContext(ctx).Create(kb).Error
}

// GetKnowledgeBaseByID gets a knowledge base by id (no knowledgeDomain scope; caller must enforce isolation where needed)
func (r *knowledgeBaseRepository) GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	var kb types.KnowledgeBase
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&kb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKnowledgeBaseNotFound
		}
		return nil, err
	}
	return &kb, nil
}

// GetKnowledgeBaseByIDAndKnowledgeDomain gets a knowledge base by id only if it belongs to the given knowledgeDomain (enforces knowledgeDomain isolation)
func (r *knowledgeBaseRepository) GetKnowledgeBaseByIDAndKnowledgeDomain(ctx context.Context, id string, knowledgeDomainID uint64) (*types.KnowledgeBase, error) {
	var kb types.KnowledgeBase
	if err := r.db.WithContext(ctx).Where("id = ? AND knowledge_domain_id = ?", id, knowledgeDomainID).First(&kb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKnowledgeBaseNotFound
		}
		return nil, err
	}
	return &kb, nil
}

// GetKnowledgeBaseByIDs gets knowledge bases by multiple ids
func (r *knowledgeBaseRepository) GetKnowledgeBaseByIDs(ctx context.Context, ids []string) ([]*types.KnowledgeBase, error) {
	if len(ids) == 0 {
		return []*types.KnowledgeBase{}, nil
	}
	var kbs []*types.KnowledgeBase
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&kbs).Error; err != nil {
		return nil, err
	}
	return kbs, nil
}

// ListKnowledgeBases lists all knowledge bases
func (r *knowledgeBaseRepository) ListKnowledgeBases(ctx context.Context) ([]*types.KnowledgeBase, error) {
	var kbs []*types.KnowledgeBase
	if err := r.db.WithContext(ctx).Find(&kbs).Error; err != nil {
		return nil, err
	}
	return kbs, nil
}

// ListKnowledgeBasesByKnowledgeDomainID lists all knowledge bases by knowledgeDomain id.
//
// Ordering used to also include `is_pinned DESC, pinned_at DESC` so the
// repository would return knowledgeDomain-wide pinned rows first. That column is
// no longer the source of truth (see migration 000050) — pin state is
// now per (user, kb) and applied by the service layer after enrichment.
// We keep `created_at DESC` here so callers that don't enrich (chat
// pipeline, agent editor, IM commands) still get a stable ordering.
func (r *knowledgeBaseRepository) ListKnowledgeBasesByKnowledgeDomainID(
	ctx context.Context, knowledgeDomainID uint64,
) ([]*types.KnowledgeBase, error) {
	var kbs []*types.KnowledgeBase
	if err := r.db.WithContext(ctx).Where("knowledge_domain_id = ? AND is_temporary = ?", knowledgeDomainID, false).
		Order("created_at DESC").Find(&kbs).Error; err != nil {
		return nil, err
	}
	return kbs, nil
}

// userKBPinRow mirrors the user_kb_pins table. Kept local to the
// repository because it never escapes the package; callers see the
// higher-level map[kb_id]pinned_at returned by ListUserKBPinIDs.
type userKBPinRow struct {
	KnowledgeDomainID uint64    `gorm:"column:knowledge_domain_id"`
	UserID            string    `gorm:"column:user_id"`
	KBID              string    `gorm:"column:kb_id"`
	PinnedAt          time.Time `gorm:"column:pinned_at"`
}

func (userKBPinRow) TableName() string { return "user_kb_pins" }

// SetUserKBPin upserts (pinned=true) or deletes (pinned=false) the row
// for the given (knowledgeDomain, user, kb) triple. The returned pinned_at is
// nil when pinned=false; otherwise it carries the timestamp written
// to the row (either the existing one if the row already existed, or
// the current time on insert) so the caller can stamp the response
// without a follow-up SELECT.
func (r *knowledgeBaseRepository) SetUserKBPin(
	ctx context.Context, knowledgeDomainID uint64, userID string, kbID string, pinned bool,
) (*time.Time, error) {
	if userID == "" {
		return nil, errors.New("user_kb_pins: empty user_id")
	}
	if !pinned {
		err := r.db.WithContext(ctx).
			Where("knowledge_domain_id = ? AND user_id = ? AND kb_id = ?", knowledgeDomainID, userID, kbID).
			Delete(&userKBPinRow{}).Error
		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Upsert with idempotent INSERT … ON CONFLICT DO NOTHING. We then
	// SELECT to learn whether an existing row's pinned_at survived (so
	// repeated calls return a stable timestamp instead of bumping it).
	row := userKBPinRow{
		KnowledgeDomainID: knowledgeDomainID,
		UserID:            userID,
		KBID:              kbID,
		PinnedAt:          time.Now(),
	}
	if err := r.db.WithContext(ctx).
		Where("knowledge_domain_id = ? AND user_id = ? AND kb_id = ?", knowledgeDomainID, userID, kbID).
		Attrs(userKBPinRow{PinnedAt: row.PinnedAt}).
		FirstOrCreate(&row).Error; err != nil {
		return nil, err
	}
	pa := row.PinnedAt
	return &pa, nil
}

// ListUserKBPinIDs returns every KB id this user has personally pinned
// in this knowledgeDomain, mapped to its pinned_at. Returns an empty map (not
// nil) when there are no pins, so callers can do `len(m) == 0` checks
// without a nil guard.
func (r *knowledgeBaseRepository) ListUserKBPinIDs(
	ctx context.Context, knowledgeDomainID uint64, userID string,
) (map[string]time.Time, error) {
	out := make(map[string]time.Time)
	if userID == "" {
		return out, nil
	}
	var rows []userKBPinRow
	if err := r.db.WithContext(ctx).
		Where("knowledge_domain_id = ? AND user_id = ?", knowledgeDomainID, userID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.KBID] = row.PinnedAt
	}
	return out, nil
}

// UpdateKnowledgeBase updates a knowledge base
func (r *knowledgeBaseRepository) UpdateKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) error {
	return r.db.WithContext(ctx).Save(kb).Error
}

// DeleteKnowledgeBase deletes a knowledge base
func (r *knowledgeBaseRepository) DeleteKnowledgeBase(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.KnowledgeBase{}).Error
}

// CountByVectorStoreID counts active knowledge bases that are bound to the
// given platform vector store.
//
// Soft-delete filter is applied automatically by GORM because KnowledgeBase
// has a gorm.DeletedAt column — we deliberately do not add an explicit
// `deleted_at IS NULL` predicate to keep the single source of truth on the
// auto-scope.
//
// Pass db == nil to use the repository's default db handle; pass a *gorm.DB
// bound to a transaction (e.g., from db.Transaction) to share the same
// write-lock context as the caller. Query column order matches the
// composite index idx_knowledge_bases_knowledge_domain_vector_store(knowledge_domain_id,
// vector_store_id).
func (r *knowledgeBaseRepository) CountByVectorStoreID(
	ctx context.Context, db *gorm.DB, _ uint64, storeID string,
) (int64, error) {
	if db == nil {
		db = r.db
	}
	var count int64
	err := db.WithContext(ctx).
		Model(&types.KnowledgeBase{}).
		Where("vector_store_id = ?", storeID).
		Count(&count).Error
	return count, err
}

// CountByModelID counts active knowledge bases that reference modelID in any
// model-binding column (scalar fields or JSON config blobs).
func (r *knowledgeBaseRepository) CountByModelID(
	ctx context.Context, modelID string,
) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&types.KnowledgeBase{})
	query = scopeKnowledgeBasesByModelID(query, modelID)
	err := query.Count(&count).Error
	return count, err
}
