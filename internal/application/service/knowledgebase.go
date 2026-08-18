package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"roche.local/knowledge-agent-platform/internal/application/service/retriever"
	"roche.local/knowledge-agent-platform/internal/datasource"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/tracing/langfuse"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// knowledgeBaseService implements the knowledge base service interface
type knowledgeBaseService struct {
	repo                interfaces.KnowledgeBaseRepository
	kgRepo              interfaces.KnowledgeRepository
	chunkRepo           interfaces.ChunkRepository
	modelService        interfaces.ModelService
	retrieveEngine      interfaces.RetrieveEngineRegistry
	ownership           retriever.KnowledgeDomainStoreOwnership
	knowledgeDomainRepo interfaces.KnowledgeDomainRepository
	fileSvc             interfaces.FileService
	graphEngine         interfaces.RetrieveGraphRepository
	asynqClient         interfaces.TaskEnqueuer
	dsRepo              interfaces.DataSourceRepository
	syncLogRepo         interfaces.SyncLogRepository
	dsScheduler         *datasource.Scheduler
	accessService       interfaces.EnterpriseAccessService
}

// NewKnowledgeBaseService creates a new knowledge base service
func NewKnowledgeBaseService(repo interfaces.KnowledgeBaseRepository,
	kgRepo interfaces.KnowledgeRepository,
	chunkRepo interfaces.ChunkRepository,
	modelService interfaces.ModelService,
	retrieveEngine interfaces.RetrieveEngineRegistry,
	ownership retriever.KnowledgeDomainStoreOwnership,
	knowledgeDomainRepo interfaces.KnowledgeDomainRepository,
	fileSvc interfaces.FileService,
	graphEngine interfaces.RetrieveGraphRepository,
	asynqClient interfaces.TaskEnqueuer,
	dsRepo interfaces.DataSourceRepository,
	syncLogRepo interfaces.SyncLogRepository,
	dsScheduler *datasource.Scheduler,
	accessService interfaces.EnterpriseAccessService,
) interfaces.KnowledgeBaseService {
	return &knowledgeBaseService{
		repo:                repo,
		kgRepo:              kgRepo,
		chunkRepo:           chunkRepo,
		modelService:        modelService,
		retrieveEngine:      retrieveEngine,
		ownership:           ownership,
		knowledgeDomainRepo: knowledgeDomainRepo,
		fileSvc:             fileSvc,
		graphEngine:         graphEngine,
		asynqClient:         asynqClient,
		dsRepo:              dsRepo,
		syncLogRepo:         syncLogRepo,
		dsScheduler:         dsScheduler,
		accessService:       accessService,
	}
}

// GetRepository gets the knowledge base repository
// Parameters:
//   - ctx: Context with authentication and request information
//
// Returns:
//   - interfaces.KnowledgeBaseRepository: Knowledge base repository
func (s *knowledgeBaseService) GetRepository() interfaces.KnowledgeBaseRepository {
	return s.repo
}

// CreateKnowledgeBase creates a new knowledge base.
//
// When VectorStoreID is set, the binding is validated against the caller's
// knowledgeDomain scope and the engine registry before persisting. A nil or
// empty-string VectorStoreID is normalized to nil ("use the knowledgeDomain's
// effective engines") to match the retrieve-engine factory's pre-condition.
func (s *knowledgeBaseService) CreateKnowledgeBase(ctx context.Context,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBase, error) {
	if kb == nil || kb.KnowledgeDomainID == 0 {
		return nil, apperrors.NewBadRequestError("knowledge_domain_id is required")
	}
	if s.accessService != nil {
		allowed, err := s.accessService.CanManageKnowledgeBase(ctx, &types.KnowledgeBase{
			KnowledgeDomainID: kb.KnowledgeDomainID,
		})
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, apperrors.NewForbiddenError("you cannot create knowledge bases in this knowledgeDomain")
		}
	}
	if s.knowledgeDomainRepo != nil {
		if _, err := s.knowledgeDomainRepo.GetKnowledgeDomainByID(ctx, kb.KnowledgeDomainID); err != nil {
			return nil, apperrors.NewBadRequestError("knowledgeDomain does not exist")
		}
	}

	// Generate UUID and set creation timestamps
	if kb.ID == "" {
		kb.ID = uuid.New().String()
	}
	kb.CreatedAt = time.Now()
	kb.UpdatedAt = time.Now()
	// Record the creator so ownership checks can distinguish user-created KBs.
	if uid, ok := types.UserIDFromContext(ctx); ok {
		kb.CreatorID = uid
	}
	kb.EnsureDefaults()
	s.applyPlatformDefaultStorageProvider(ctx, kb)

	// Fold empty-string vector_store_id into nil so this path and the
	// retrieve-engine factory's pre-condition share a single representation.
	wasEmpty := kb.VectorStoreID != nil && *kb.VectorStoreID == ""
	kb.Normalize()
	if wasEmpty {
		logger.Debugf(ctx,
			"[kb.create] empty vector_store_id normalized to nil for knowledgeDomain=%d",
			kb.KnowledgeDomainID)
	}

	if kb.HasVectorStore() {
		if err := s.validateVectorStoreBinding(ctx, kb.KnowledgeDomainID, *kb.VectorStoreID); err != nil {
			return nil, err
		}
	}

	logger.Infof(ctx, "Creating knowledge base, ID: %s, knowledgeDomain ID: %d, name: %s", kb.ID, kb.KnowledgeDomainID, kb.Name)

	if err := s.repo.CreateKnowledgeBase(ctx, kb); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id":   kb.ID,
			"knowledge_domain_id": kb.KnowledgeDomainID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Knowledge base created successfully, ID: %s, name: %s", kb.ID, kb.Name)
	return kb, nil
}

// applyPlatformDefaultStorageProvider fills an empty KB storage provider from
// the platform runtime configuration projected by the knowledge-domain
// repository. Frontend clients normally send the same value explicitly.
func (s *knowledgeBaseService) applyPlatformDefaultStorageProvider(ctx context.Context, kb *types.KnowledgeBase) {
	if kb == nil || strings.TrimSpace(kb.GetStorageProvider()) != "" {
		return
	}
	provider := "local"
	if s.knowledgeDomainRepo != nil {
		if domain, err := s.knowledgeDomainRepo.GetKnowledgeDomainByID(ctx, kb.KnowledgeDomainID); err == nil && domain != nil {
			if domain.StorageEngineConfig != nil {
				if p := strings.ToLower(strings.TrimSpace(domain.StorageEngineConfig.DefaultProvider)); p != "" {
					provider = p
				}
			}
		}
	}
	kb.SetStorageProvider(provider)
}

// validateVectorStoreBinding routes through retriever.VerifyBinding so the
// ownership + registry sentinel hierarchy stays the single source of truth.
// The service layer's responsibility is to:
//
//  1. fast-reject malformed UUIDs (cheap pre-flight that also avoids a DB
//     round trip for type-confusion inputs like "' OR 1=1 --"),
//  2. translate retriever sentinels into user-facing AppErrors with
//     generic messages and the typed error codes.
//
// UUID parse failures map to the same "vector store not found" message as
// cross-knowledgeDomain attempts to avoid an enumeration oracle that distinguishes
// "malformed input" from "non-existent UUID".
func (s *knowledgeBaseService) validateVectorStoreBinding(
	ctx context.Context, knowledgeDomainID uint64, storeID string,
) error {
	sanitized := secutils.SanitizeForLog(storeID)

	if _, err := uuid.Parse(storeID); err != nil {
		logger.WarnWithFields(ctx, logger.Fields{
			"knowledge_domain_id": knowledgeDomainID,
			"store_id":            sanitized,
			"reason":              "malformed vector_store_id",
		}, "[kb.create] vector store id is not a valid UUID")
		return apperrors.NewVectorStoreBindingInvalidError("vector store not found")
	}

	switch err := retriever.VerifyBinding(
		ctx, s.retrieveEngine, s.ownership, knowledgeDomainID, storeID,
	); {
	case err == nil:
		return nil
	case errors.Is(err, retriever.ErrVectorStoreForbidden):
		logger.WarnWithFields(ctx, logger.Fields{
			"knowledge_domain_id": knowledgeDomainID,
			"store_id":            sanitized,
			"reason":              "cross-knowledgeDomain or unknown store",
		}, "[kb.create] vector store not owned by knowledgeDomain")
		return apperrors.NewVectorStoreBindingInvalidError("vector store not found")
	case errors.Is(err, retriever.ErrVectorStoreNotFound):
		logger.WarnWithFields(ctx, logger.Fields{
			"knowledge_domain_id": knowledgeDomainID,
			"store_id":            sanitized,
			"reason":              "store registered in DB but missing in registry",
		}, "[kb.create] vector store currently unavailable")
		return apperrors.NewVectorStoreUnavailableError(
			"vector store is currently unavailable; check its connection configuration")
	default:
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomainID,
			"store_id":            sanitized,
			"reason":              "binding verification failed",
		})
		return apperrors.NewInternalServerError("failed to verify vector store binding")
	}
}

// GetKnowledgeBaseByID retrieves a knowledge base by its ID
func (s *knowledgeBaseService) GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return nil, errors.New("knowledge base ID cannot be empty")
	}

	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	kb.EnsureDefaults()
	return kb, nil
}

// GetKnowledgeBaseByIDOnly retrieves a knowledge base by ID before authorization.
func (s *knowledgeBaseService) GetKnowledgeBaseByIDOnly(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return nil, errors.New("knowledge base ID cannot be empty")
	}

	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	kb.EnsureDefaults()
	return kb, nil
}

// GetKnowledgeBasesByIDsOnly retrieves knowledge bases by IDs without knowledgeDomain filter (batch).
func (s *knowledgeBaseService) GetKnowledgeBasesByIDsOnly(ctx context.Context, ids []string) ([]*types.KnowledgeBase, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	kbs, err := s.repo.GetKnowledgeBaseByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, kb := range kbs {
		if kb != nil {
			kb.EnsureDefaults()
		}
	}
	return kbs, nil
}

// ListKnowledgeBases returns every knowledge base the current user can access.
// Knowledge domains are administrative containers and do not establish an
// implicit user scope.
func (s *knowledgeBaseService) ListKnowledgeBases(ctx context.Context) ([]*types.KnowledgeBase, error) {
	kbs, err := s.repo.ListKnowledgeBases(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}
	filtered := kbs[:0]
	for _, kb := range kbs {
		if kb == nil || kb.IsTemporary {
			continue
		}
		kb.EnsureDefaults()
		filtered = append(filtered, kb)
	}
	kbs = filtered
	if s.accessService != nil {
		kbs, err = s.accessService.FilterKnowledgeBases(ctx, kbs)
		if err != nil {
			return nil, err
		}
	}

	// Counts are scoped to the owning department of each KB, not the caller's
	// home department. This matters for cross-department grants.
	for _, kb := range kbs {
		_ = s.FillKnowledgeBaseCounts(ctx, kb)
	}

	// Per-user pin stamping + ordering. The "main" list view is the
	// only path that needs to honour the caller's personal pin set;
	// agent/share/IM callers go through ListKnowledgeBasesByKnowledgeDomainID
	// which also enriches but keys off the user in their own context.
	if userID, ok := types.UserIDFromContext(ctx); ok && userID != "" {
		byKnowledgeDomain := make(map[uint64][]*types.KnowledgeBase)
		for _, kb := range kbs {
			byKnowledgeDomain[kb.KnowledgeDomainID] = append(byKnowledgeDomain[kb.KnowledgeDomainID], kb)
		}
		for ownerKnowledgeDomainID, knowledgeDomainKBs := range byKnowledgeDomain {
			s.applyUserKBPins(ctx, ownerKnowledgeDomainID, userID, knowledgeDomainKBs)
		}
	}
	return kbs, nil
}

// ResolveKnowledgeBaseAccess, CanReadKnowledgeBase and
// CanManageKnowledgeBase are consumed by route guards and search-scope
// construction.
func (s *knowledgeBaseService) ResolveKnowledgeBaseAccess(
	ctx context.Context,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBaseAccessScope, error) {
	if s.accessService == nil {
		return &types.KnowledgeBaseAccessScope{
			Allowed:    kb != nil,
			CanManage:  kb != nil,
			FullAccess: kb != nil,
			Permission: types.KnowledgeBasePermissionManage,
		}, nil
	}
	return s.accessService.ResolveKnowledgeBaseAccess(ctx, kb)
}

func (s *knowledgeBaseService) CanReadKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error) {
	if s.accessService == nil {
		return kb != nil, nil
	}
	return s.accessService.CanReadKnowledgeBase(ctx, kb)
}

func (s *knowledgeBaseService) CanManageKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error) {
	if s.accessService == nil {
		return kb != nil, nil
	}
	return s.accessService.CanManageKnowledgeBase(ctx, kb)
}

// ListKnowledgeBasesByKnowledgeDomainID returns all knowledge bases for the given knowledgeDomain.
func (s *knowledgeBaseService) ListKnowledgeBasesByKnowledgeDomainID(ctx context.Context, knowledgeDomainID uint64) ([]*types.KnowledgeBase, error) {
	kbs, err := s.repo.ListKnowledgeBasesByKnowledgeDomainID(ctx, knowledgeDomainID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomainID,
		})
		return nil, err
	}
	for _, kb := range kbs {
		kb.EnsureDefaults()
		switch kb.Type {
		case types.KnowledgeBaseTypeDocument:
			if cnt, err := s.kgRepo.CountKnowledgeByKnowledgeBaseID(ctx, knowledgeDomainID, kb.ID); err == nil {
				kb.KnowledgeCount = cnt
			}
		case types.KnowledgeBaseTypeFAQ:
			if cnt, err := s.chunkRepo.CountChunksByKnowledgeBaseID(ctx, knowledgeDomainID, kb.ID); err == nil {
				kb.ChunkCount = cnt
			}
		}
		if processingCount, err := s.kgRepo.CountKnowledgeByStatus(ctx, knowledgeDomainID, kb.ID, []string{"pending", "processing"}); err == nil {
			kb.IsProcessing = processingCount > 0
			kb.ProcessingCount = processingCount
		}
	}

	// Stamp pin state from the caller's perspective, scoped by knowledgeDomainID.
	if userID, ok := types.UserIDFromContext(ctx); ok && userID != "" {
		s.applyUserKBPins(ctx, knowledgeDomainID, userID, kbs)
	}
	return kbs, nil
}

// FillKnowledgeBaseCounts fills KnowledgeCount, ChunkCount, IsProcessing, ProcessingCount for the given KB using kb.KnowledgeDomainID.
func (s *knowledgeBaseService) FillKnowledgeBaseCounts(ctx context.Context, kb *types.KnowledgeBase) error {
	if kb == nil {
		return nil
	}
	knowledgeDomainID := kb.KnowledgeDomainID
	kb.EnsureDefaults()
	switch kb.Type {
	case types.KnowledgeBaseTypeDocument:
		if cnt, err := s.kgRepo.CountKnowledgeByKnowledgeBaseID(ctx, knowledgeDomainID, kb.ID); err == nil {
			kb.KnowledgeCount = cnt
		}
	case types.KnowledgeBaseTypeFAQ:
		if cnt, err := s.chunkRepo.CountChunksByKnowledgeBaseID(ctx, knowledgeDomainID, kb.ID); err == nil {
			kb.ChunkCount = cnt
		}
	}
	if processingCount, err := s.kgRepo.CountKnowledgeByStatus(ctx, knowledgeDomainID, kb.ID, []string{"pending", "processing"}); err == nil {
		kb.IsProcessing = processingCount > 0
		kb.ProcessingCount = processingCount
	}
	return nil
}

// UpdateKnowledgeBase updates a knowledge base's mutable properties.
//
// IMPORTANT — vector_store_id immutability contract:
// The vector_store_id binding is deliberately not accepted by this method.
// Two layers enforce immutability:
//
//  1. ORM layer: the GORM tag `<-:create` on KnowledgeBase.VectorStoreID
//     makes every UPDATE path (Save / Updates / Select-Updates) a no-op for
//     that column. Verified by repository/knowledgebase_sqlite_test.go.
//  2. Service layer: this method intentionally omits VectorStoreID from its
//     parameter list, and the matching handler DTO UpdateKnowledgeBaseRequest
//     omits the field as well. A reflection-based regression test
//     (handler/knowledgebase_request_test.go) fails if either DTO field
//     is added back, alerting future maintainers.
//
// Any future cross-store rebind workflow must use raw SQL through a
// dedicated repository method — the only sanctioned write path post-creation.
func (s *knowledgeBaseService) UpdateKnowledgeBase(ctx context.Context,
	id string,
	name string,
	description string,
	config *types.KnowledgeBaseConfig,
) (*types.KnowledgeBase, error) {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return nil, errors.New("knowledge base ID cannot be empty")
	}

	logger.Infof(ctx, "Updating knowledge base, ID: %s, name: %s", id, name)

	// Get existing knowledge base
	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	// Update the knowledge base properties
	kb.Name = name
	kb.Description = description
	if config != nil {
		kb.ChunkingConfig = config.ChunkingConfig
		kb.ImageProcessingConfig = config.ImageProcessingConfig
		if config.FAQConfig != nil {
			kb.FAQConfig = config.FAQConfig
		}
		// Update indexing strategy — syncs to ExtractConfig for backward compat
		if config.IndexingStrategy != nil {
			if !config.IndexingStrategy.HasAnyIndexing() {
				return nil, errors.New("at least one indexing strategy must be enabled")
			}
			kb.IndexingStrategy = *config.IndexingStrategy
			// Sync GraphEnabled → ExtractConfig
			if kb.ExtractConfig != nil {
				kb.ExtractConfig.Enabled = config.IndexingStrategy.GraphEnabled
			} else if config.IndexingStrategy.GraphEnabled {
				kb.ExtractConfig = &types.ExtractConfig{Enabled: true}
			}
		}
	}
	kb.UpdatedAt = time.Now()
	kb.EnsureDefaults()

	logger.Info(ctx, "Saving knowledge base update")
	if err := s.repo.UpdateKnowledgeBase(ctx, kb); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	logger.Infof(ctx, "Knowledge base updated successfully, ID: %s, name: %s", kb.ID, kb.Name)
	return kb, nil
}

// TogglePinKnowledgeBase toggles whether the calling user has pinned
// this knowledge base. Pin state is per-(user, kb) as of migration
// 000050; previously this method flipped a knowledgeDomain-wide column on the
// KB row which broke down under RBAC (only Admin/creator could pin,
// and the pin reordered the list for everyone in the knowledgeDomain). The
// public signature is unchanged so the HTTP handler / CLI / SDK don't
// move.
//
// The KB still has to belong to the caller's knowledgeDomain — the route is
// already gated behind KBAccessRead, but we re-check via
// GetKnowledgeBaseByIDAndKnowledgeDomain so a stale param survives a knowledgeDomain
// switch cleanly.
func (s *knowledgeBaseService) TogglePinKnowledgeBase(
	ctx context.Context, id string,
) (*types.KnowledgeBase, error) {
	if id == "" {
		return nil, errors.New("knowledge base ID cannot be empty")
	}
	knowledgeDomainID := types.MustKnowledgeDomainIDFromContext(ctx)
	userID, ok := types.UserIDFromContext(ctx)
	if !ok || userID == "" {
		// API-key callers without a user identity can't have a personal
		// pin set, so surface an explicit error.
		return nil, errors.New("pin requires an authenticated user")
	}

	// The route's KBAccessRead guard already validated this resource.
	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id":   id,
			"knowledge_domain_id": knowledgeDomainID,
		})
		return nil, err
	}

	// Read current pin state to decide direction. ListUserKBPinIDs is
	// already optimised for the "many KBs at once" path; for a single-id
	// check the round-trip is acceptable and avoids leaking a second
	// repository method just for this.
	pins, err := s.repo.ListUserKBPinIDs(ctx, knowledgeDomainID, userID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id":   id,
			"knowledge_domain_id": knowledgeDomainID,
			"user_id":             userID,
		})
		return nil, err
	}
	_, currentlyPinned := pins[id]

	pinnedAt, err := s.repo.SetUserKBPin(ctx, knowledgeDomainID, userID, id, !currentlyPinned)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id":   id,
			"knowledge_domain_id": knowledgeDomainID,
			"user_id":             userID,
			"target_pinned":       !currentlyPinned,
		})
		return nil, err
	}

	kb.EnsureDefaults()
	kb.IsPinned = !currentlyPinned
	kb.PinnedAt = pinnedAt
	logger.Infof(ctx, "Knowledge base pin toggled, ID: %s, user: %s, is_pinned: %v",
		id, userID, kb.IsPinned)
	return kb, nil
}

// TogglePublicKnowledgeBase sets the public visibility flag on a knowledge base.
// Public knowledge bases are readable by every authenticated user unless a
// per-user or per-org-unit deny grant overrides it.
func (s *knowledgeBaseService) TogglePublicKnowledgeBase(
	ctx context.Context, id string, isPublic bool,
) (*types.KnowledgeBase, error) {
	if id == "" {
		return nil, errors.New("knowledge base ID cannot be empty")
	}
	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}
	kb.IsPublic = isPublic
	kb.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledgeBase(ctx, kb); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
			"is_public":         isPublic,
		})
		return nil, err
	}
	kb.EnsureDefaults()
	logger.Infof(ctx, "Knowledge base public toggled, ID: %s, is_public: %v", id, isPublic)
	return kb, nil
}
// from the caller's perspective and sorts the slice so pinned rows
// float to the top (newest pin first, ties broken by created_at desc).
// Safe to call with an empty userID (no-op stamp; default sort by
// created_at preserved).
func (s *knowledgeBaseService) applyUserKBPins(
	ctx context.Context, knowledgeDomainID uint64, userID string, kbs []*types.KnowledgeBase,
) {
	if len(kbs) == 0 || userID == "" {
		return
	}
	pins, err := s.repo.ListUserKBPinIDs(ctx, knowledgeDomainID, userID)
	if err != nil {
		// Pin enrichment is best-effort: a transient DB blip here
		// should not break listing KBs. Log and bail without altering
		// the slice — caller still gets a valid list, just unsorted by
		// pin.
		logger.Warnf(ctx, "applyUserKBPins: failed to load pins for knowledgeDomain=%d user=%s: %v",
			knowledgeDomainID, userID, err)
		return
	}
	if len(pins) == 0 {
		return
	}
	for _, kb := range kbs {
		if ts, ok := pins[kb.ID]; ok {
			kb.IsPinned = true
			t := ts
			kb.PinnedAt = &t
		}
	}
	sort.SliceStable(kbs, func(i, j int) bool {
		a, b := kbs[i], kbs[j]
		if a.IsPinned != b.IsPinned {
			return a.IsPinned
		}
		if a.IsPinned && b.IsPinned {
			at, bt := a.PinnedAt, b.PinnedAt
			if at != nil && bt != nil && !at.Equal(*bt) {
				return at.After(*bt)
			}
		}
		return a.CreatedAt.After(b.CreatedAt)
	})
}

// DeleteKnowledgeBase deletes a knowledge base by its ID
// This method marks the knowledge base as deleted and enqueues an async task
// to handle the heavy cleanup operations (embeddings, chunks, files, graph data)
func (s *knowledgeBaseService) DeleteKnowledgeBase(ctx context.Context, id string) error {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return errors.New("knowledge base ID cannot be empty")
	}

	logger.Infof(ctx, "Deleting knowledge base, ID: %s", id)

	// Get knowledgeDomain ID from context
	knowledgeDomainID := types.MustKnowledgeDomainIDFromContext(ctx)
	knowledgeDomainInfo, _ := types.KnowledgeDomainInfoFromContext(ctx)

	// Load the KB before soft-delete so we can snapshot its VectorStoreID
	// into the async cleanup payload. GORM's soft-delete filter hides the
	// row from subsequent reads, so this read must happen first.
	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return err
	}
	var vectorStoreIDSnapshot *string
	if kb != nil {
		vectorStoreIDSnapshot = kb.VectorStoreID
	}

	// Step 1: Delete the knowledge base record first (mark as deleted)
	logger.Infof(ctx, "Deleting knowledge base from database")
	err = s.repo.DeleteKnowledgeBase(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return err
	}

	// Step 1b: Stop and soft-delete all data sources bound to this KB so cron
	// schedules and in-flight sync logs do not keep running against a deleted KB.
	s.deleteDataSourcesForKnowledgeBase(ctx, id)

	// Step 2: Enqueue async task for heavy cleanup operations
	payload := types.KBDeletePayload{
		KnowledgeDomainID: knowledgeDomainID,
		KnowledgeBaseID:   id,
		EffectiveEngines:  knowledgeDomainInfo.GetEffectiveEngines(),
		VectorStoreID:     vectorStoreIDSnapshot, // snapshot taken before soft-delete
	}
	langfuse.InjectTracing(ctx, &payload)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Warnf(ctx, "Failed to marshal KB delete payload: %v", err)
		// Don't fail the request, the KB record is already deleted
		return nil
	}

	task := asynq.NewTask(types.TypeKBDelete, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
	info, err := s.asynqClient.Enqueue(task)
	if err != nil {
		logger.Warnf(ctx, "Failed to enqueue KB delete task: %v", err)
		// Don't fail the request, the KB record is already deleted
		return nil
	}

	logger.Infof(ctx, "KB delete task enqueued: %s, knowledge base ID: %s", info.ID, id)
	logger.Infof(ctx, "Knowledge base deleted successfully, ID: %s", id)
	return nil
}

// ProcessKBDelete handles async knowledge base deletion task
// This method performs heavy cleanup operations: deleting embeddings, chunks, files, and graph data
func (s *knowledgeBaseService) ProcessKBDelete(ctx context.Context, t *asynq.Task) error {
	var payload types.KBDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal KB delete payload: %v", err)
		return err
	}

	knowledgeDomainID := payload.KnowledgeDomainID
	kbID := payload.KnowledgeBaseID

	// Set knowledgeDomain context for downstream services
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, knowledgeDomainID)

	logger.Infof(ctx, "Processing KB delete task for knowledge base: %s", kbID)

	// Step 1: Get all knowledge entries in this knowledge base
	logger.Infof(ctx, "Fetching all knowledge entries in knowledge base, ID: %s", kbID)
	knowledgeList, err := s.kgRepo.ListKnowledgeByKnowledgeBaseID(ctx, knowledgeDomainID, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": kbID,
		})
		return err
	}
	logger.Infof(ctx, "Found %d knowledge entries to delete", len(knowledgeList))

	// Step 2: Delete all knowledge entries and their resources
	if len(knowledgeList) > 0 {
		knowledgeIDs := make([]string, 0, len(knowledgeList))
		for _, knowledge := range knowledgeList {
			knowledgeIDs = append(knowledgeIDs, knowledge.ID)
		}

		logger.Infof(ctx, "Deleting all knowledge entries and their resources")

		// Delete embeddings from vector store.
		// Resolve the engine via the factory, using the VectorStoreID captured
		// at enqueue time (may be nil → falls back to payload.EffectiveEngines).
		// If the payload references a store no longer owned/registered
		// (e.g. tampered queue entry or a store that was deleted while the
		// task sat in the queue), the factory returns a sentinel and we
		// SkipRetry to avoid burning retries on an unrecoverable situation.
		logger.Infof(ctx, "Deleting embeddings from vector store")
		retrieveEngine, err := retriever.CreateRetrieveEngineFromPayload(
			ctx,
			s.retrieveEngine,
			s.ownership,
			payload.KnowledgeDomainID,
			payload.EffectiveEngines,
			payload.VectorStoreID,
		)
		if errors.Is(err, retriever.ErrVectorStoreForbidden) ||
			errors.Is(err, retriever.ErrVectorStoreNotFound) {
			logger.Errorf(ctx, "KB delete task aborted: %v (knowledgeDomain=%d, kb=%s)", err, payload.KnowledgeDomainID, payload.KnowledgeBaseID)
			return asynq.SkipRetry
		}
		if err != nil {
			logger.Warnf(ctx, "Failed to create retrieve engine: %v", err)
		} else {
			// Group knowledge by embedding model and type
			type groupKey struct {
				EmbeddingModelID string
				Type             string
			}
			embeddingGroups := make(map[groupKey][]string)
			for _, knowledge := range knowledgeList {
				key := groupKey{EmbeddingModelID: knowledge.EmbeddingModelID, Type: knowledge.Type}
				embeddingGroups[key] = append(embeddingGroups[key], knowledge.ID)
			}

			for key, knowledgeGroup := range embeddingGroups {
				embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, key.EmbeddingModelID)
				if err != nil {
					logger.Warnf(ctx, "Failed to get embedding model %s: %v", key.EmbeddingModelID, err)
					continue
				}
				if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, knowledgeGroup, embeddingModel.GetDimensions(), key.Type); err != nil {
					logger.Warnf(ctx, "Failed to delete embeddings for model %s: %v", key.EmbeddingModelID, err)
				}
			}
		}

		// Collect image URLs before chunks are deleted
		chunkImageInfos, imgErr := s.chunkRepo.ListImageInfoByKnowledgeIDs(ctx, knowledgeDomainID, knowledgeIDs)
		if imgErr != nil {
			logger.Warnf(ctx, "Failed to collect image URLs for KB delete: %v", imgErr)
		}
		var imageInfoStrs []string
		for _, ci := range chunkImageInfos {
			imageInfoStrs = append(imageInfoStrs, ci.ImageInfo)
		}
		imageURLs := collectImageURLs(ctx, imageInfoStrs)

		// Delete all chunks
		logger.Infof(ctx, "Deleting all chunks in knowledge base")
		for _, knowledgeID := range knowledgeIDs {
			if err := s.chunkRepo.DeleteChunksByKnowledgeID(ctx, knowledgeDomainID, knowledgeID); err != nil {
				logger.Warnf(ctx, "Failed to delete chunks for knowledge %s: %v", knowledgeID, err)
			}
		}

		// Delete physical files, extracted images, and adjust storage
		logger.Infof(ctx, "Deleting physical files and extracted images")
		storageAdjust := int64(0)
		for _, knowledge := range knowledgeList {
			if knowledge.FilePath != "" {
				if err := s.fileSvc.DeleteFile(ctx, knowledge.FilePath); err != nil {
					logger.Warnf(ctx, "Failed to delete file %s: %v", knowledge.FilePath, err)
				}
			}
			storageAdjust -= knowledge.StorageSize
		}
		deleteExtractedImages(ctx, s.fileSvc, imageURLs)
		if storageAdjust != 0 {
			if err := s.knowledgeDomainRepo.AdjustStorageUsed(ctx, knowledgeDomainID, storageAdjust); err != nil {
				logger.Warnf(ctx, "Failed to adjust knowledgeDomain storage: %v", err)
			}
		}

		// Delete knowledge graph data
		logger.Infof(ctx, "Deleting knowledge graph data")
		namespaces := make([]types.NameSpace, 0, len(knowledgeList))
		for _, knowledge := range knowledgeList {
			namespaces = append(namespaces, types.NameSpace{
				KnowledgeBase: knowledge.KnowledgeBaseID,
				Knowledge:     knowledge.ID,
			})
		}
		if s.graphEngine != nil && len(namespaces) > 0 {
			if err := s.graphEngine.DelGraph(ctx, namespaces); err != nil {
				logger.Warnf(ctx, "Failed to delete knowledge graph: %v", err)
			}
		}

		// Delete all knowledge entries from database
		logger.Infof(ctx, "Deleting knowledge entries from database")
		if err := s.kgRepo.DeleteKnowledgeList(ctx, knowledgeDomainID, knowledgeIDs); err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"knowledge_base_id": kbID,
			})
			return err
		}
	}

	logger.Infof(ctx, "KB delete task completed successfully, knowledge base ID: %s", kbID)
	return nil
}

// deleteDataSourcesForKnowledgeBase mirrors DataSourceService.DeleteDataSource for
// every data source attached to the KB. Errors on individual sources are logged
// but do not fail KB deletion — the KB record is already soft-deleted.
func (s *knowledgeBaseService) deleteDataSourcesForKnowledgeBase(ctx context.Context, kbID string) {
	if s.dsRepo == nil {
		return
	}

	dataSources, err := s.dsRepo.FindByKnowledgeBase(ctx, kbID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list data sources for deleted KB %s: %v", kbID, err)
		return
	}
	for _, ds := range dataSources {
		if ds == nil || ds.ID == "" {
			continue
		}
		if err := s.dsRepo.Delete(ctx, ds.ID); err != nil {
			logger.Warnf(ctx, "Failed to delete data source %s for KB %s: %v", ds.ID, kbID, err)
			continue
		}
		if s.dsScheduler != nil {
			s.dsScheduler.Remove(ds.ID)
		}
		if s.syncLogRepo != nil {
			if err := s.syncLogRepo.CancelPendingByDataSource(ctx, ds.ID); err != nil {
				logger.Warnf(ctx, "Failed to cancel pending sync logs for ds=%s (kb=%s): %v", ds.ID, kbID, err)
			}
		}
		logger.Infof(ctx, "Data source deleted with knowledge base: ds=%s kb=%s", ds.ID, kbID)
	}
}

// SetEmbeddingModel sets the embedding model for a knowledge base
func (s *knowledgeBaseService) SetEmbeddingModel(ctx context.Context, id string, modelID string) error {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return errors.New("knowledge base ID cannot be empty")
	}

	if modelID == "" {
		logger.Error(ctx, "Model ID is empty")
		return errors.New("model ID cannot be empty")
	}

	logger.Infof(ctx, "Setting embedding model for knowledge base, knowledge base ID: %s, model ID: %s", id, modelID)

	// Get the knowledge base
	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return err
	}

	// Update the knowledge base's embedding model
	kb.EmbeddingModelID = modelID
	kb.UpdatedAt = time.Now()

	logger.Info(ctx, "Saving knowledge base embedding model update")
	err = s.repo.UpdateKnowledgeBase(ctx, kb)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id":  id,
			"embedding_model_id": modelID,
		})
		return err
	}

	logger.Infof(
		ctx,
		"Knowledge base embedding model set successfully, knowledge base ID: %s, model ID: %s",
		id,
		modelID,
	)
	return nil
}

// CopyKnowledgeBase copies a knowledge base to a new knowledge base (shallow copy).
// Source and target must belong to the knowledgeDomain in context; cross-knowledgeDomain access is rejected.
//
// Defensive checks:
//
//   - When dstKB != "" (clone into an existing target), the source's
//     EmbeddingModelID and VectorStoreID must match the target's. Mismatched
//     embedding models would silently mix incompatible vector spaces;
//     mismatched vector stores would require copying physical vector data
//     between stores, which is not yet supported.
//   - When dstKB == "" (create a new target), VectorStoreID is copied from
//     the source so the new KB shares the same physical vector index. GORM
//     `<-:create` allows INSERT, so the new row is well-formed.
//
// The handler's CopyKnowledgeBase endpoint runs the same checks synchronously
// before enqueueing the async clone task, so the 400 errors here are
// defense-in-depth for the worker entry point.
func (s *knowledgeBaseService) CopyKnowledgeBase(ctx context.Context,
	srcKB string, dstKB string,
) (*types.KnowledgeBase, *types.KnowledgeBase, error) {
	knowledgeDomainID := types.MustKnowledgeDomainIDFromContext(ctx)
	// Load source KB with knowledgeDomain scope to prevent cross-knowledgeDomain cloning
	sourceKB, err := s.repo.GetKnowledgeBaseByIDAndKnowledgeDomain(ctx, srcKB, knowledgeDomainID)
	if err != nil {
		logger.Errorf(ctx, "Get source knowledge base failed: %v", err)
		return nil, nil, err
	}
	sourceKB.EnsureDefaults()
	var targetKB *types.KnowledgeBase
	if dstKB != "" {
		// Load target KB with knowledgeDomain scope so we only clone into the caller's knowledgeDomain
		targetKB, err = s.repo.GetKnowledgeBaseByIDAndKnowledgeDomain(ctx, dstKB, knowledgeDomainID)
		if err != nil {
			return nil, nil, err
		}

		// Defense 1: embedding model must match. Mixing incompatible
		// vector spaces would produce semantically broken search results.
		if sourceKB.EmbeddingModelID != targetKB.EmbeddingModelID {
			return nil, nil, apperrors.NewBadRequestError(
				"source and target knowledge bases use different embedding models; " +
					"clone into a target with the same embedding model")
		}

		// Defense 2: vector store binding must match. Cross-store cloning
		// would require copying physical vector data between stores.
		// (both nil → equal; both same UUID → equal; otherwise → rejected)
		if !sourceKB.SharesStoreWith(targetKB) {
			return nil, nil, apperrors.NewBadRequestError(
				"source and target knowledge bases are bound to different vector stores; " +
					"cross-store cloning is not yet supported")
		}

		// Defense 3: storage backend must match — only meaningful when the
		// knowledgeDomain has a StorageEngineConfig. Without it, resolveFileService
		// ignores per-KB provider pins and routes ALL KBs to the global
		// storage service, so a clone can never span two real backends and
		// the pins must NOT be used to reject (that would be a false positive).
		// When a knowledgeDomain config exists, pins are honored, so compare effective
		// providers and reject a genuine cross-backend clone up front (it would
		// otherwise fail mid-clone with ErrCrossBackendCopy).
		if knowledgeDomain, _ := ctx.Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain); knowledgeDomain != nil && knowledgeDomain.StorageEngineConfig != nil {
			knowledgeDomainDefault := knowledgeDomain.StorageEngineConfig.DefaultProvider
			srcProvider := sourceKB.EffectiveStorageProvider(knowledgeDomainDefault)
			dstProvider := targetKB.EffectiveStorageProvider(knowledgeDomainDefault)
			if srcProvider != "" && dstProvider != "" && srcProvider != dstProvider {
				return nil, nil, apperrors.NewBadRequestError(fmt.Sprintf(
					"source and target knowledge bases use different storage backends (%s vs %s); "+
						"cross-storage-backend cloning is not supported", srcProvider, dstProvider))
			}
		}
	} else {
		var faqConfig *types.FAQConfig
		if sourceKB.FAQConfig != nil {
			cfg := *sourceKB.FAQConfig
			faqConfig = &cfg
		}
		// Preserve VectorStoreID so the cloned KB lands on the same
		// physical index. GORM `<-:create` permits the value at INSERT.
		targetKB = &types.KnowledgeBase{
			ID:                    uuid.New().String(),
			Name:                  sourceKB.Name,
			Type:                  sourceKB.Type,
			Description:           sourceKB.Description,
			KnowledgeDomainID:     knowledgeDomainID,
			ChunkingConfig:        sourceKB.ChunkingConfig,
			ImageProcessingConfig: sourceKB.ImageProcessingConfig,
			EmbeddingModelID:      sourceKB.EmbeddingModelID,
			SummaryModelID:        sourceKB.SummaryModelID,
			VLMConfig:             sourceKB.VLMConfig,
			StorageProviderConfig: sourceKB.StorageProviderConfig,
			StorageConfig:         sourceKB.StorageConfig,
			FAQConfig:             faqConfig,
			VectorStoreID:         sourceKB.VectorStoreID,
		}
		// The clone is owned by the caller, not the original creator.
		if uid, ok := types.UserIDFromContext(ctx); ok {
			targetKB.CreatorID = uid
		}
		targetKB.EnsureDefaults()
		if err := s.repo.CreateKnowledgeBase(ctx, targetKB); err != nil {
			return nil, nil, err
		}
	}
	return sourceKB, targetKB, nil
}
