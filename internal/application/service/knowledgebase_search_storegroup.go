package service

import (
	"context"
	"errors"
	"fmt"

	"roche.local/knowledge-agent-platform/internal/application/service/retriever"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// storeGroup is one fan-out unit of HybridSearch: a set of KB IDs that share
// the same (VectorStore, owning knowledgeDomain) pair.
//
// Partition key is (VectorStoreID, OwnerKnowledgeDomainID) so ownership checks remain
// explicit even when this service is called from system-level jobs.
//
// BaseParams are immutable across iterations and goroutines. TopK is the
// only mutable per-iteration value; paramsWithTopK builds a fresh
// []RetrieveParams per call so no goroutine sees a slice being mutated by
// another. Callers MUST NOT mutate BaseParams after resolveStoreGroups
// returns; doing so would race with the fan-out goroutines.
type storeGroup struct {
	// StoreID is the bound VectorStore UUID, or "" for the env-store group
	// (KBs with VectorStoreID = NULL). Never echo this in user-facing
	// errors; use secutils.SanitizeForLog when emitting in structured logs.
	StoreID string

	// OwnerKnowledgeDomainID is the knowledgeDomain that owns the KBs and the store for this group.
	OwnerKnowledgeDomainID uint64

	// KBIDs are the knowledge base IDs in this group. The caller MUST have
	// authorized the request to access every ID here (the trust boundary
	// is the HTTP handler / session layer, matching the pre-existing
	// chat_pipeline pattern).
	KBIDs []string

	// Engine is the resolved CompositeRetrieveEngine for this group.
	// Reused across iterative FAQ retries — never re-resolved.
	Engine *retriever.CompositeRetrieveEngine

	// BaseParams is the immutable per-group retrieval parameter list.
	// TopK is filled in at retrieve time via paramsWithTopK.
	BaseParams []types.RetrieveParams

	// TopK is the requested over-retrieval count. The iterative FAQ path
	// (knowledgebase_search_faq.go) updates this between calls to
	// retrieveFromStores. Single-shot HybridSearch sets it once.
	TopK int
}

// resolveStoreGroups partitions kbs by (VectorStoreID, KB.KnowledgeDomainID),
// resolves the engine per group via the PR2 factory using the OWNING
// knowledgeDomain for ownership lookup, and builds the per-store base RetrieveParams
// once. Returns groups in non-deterministic order (caller must not rely on
// iteration order).
//
// The primary KB supplies the embedding model and FAQ type for params; the
// caller MUST invoke validateSameEmbeddingModel first to guarantee a
// single embedding model identity across kbs.
//
// Errors are translated from sentinel to typed AppError so that the
// upstream handler reports a stable error code without leaking storeIDs:
//
//   - retriever.ErrVectorStoreForbidden →
//     apperrors.NewVectorStoreBindingInvalidError (2200)
//   - retriever.ErrVectorStoreNotFound →
//     apperrors.NewVectorStoreUnavailableError (2201)
//   - retriever.ErrKnowledgeDomainInfoMissing →
//     apperrors.NewVectorStoreBindingInvalidError (2200)
//   - any other error → returned with %w wrap (no UUID embedded).
func (s *knowledgeBaseService) resolveStoreGroups(
	ctx context.Context,
	primary *types.KnowledgeBase,
	kbs []*types.KnowledgeBase,
	params types.SearchParams,
	matchCount int,
) ([]*storeGroup, error) {
	type partitionKey struct {
		storeID           string
		knowledgeDomainID uint64
	}
	buckets := make(map[partitionKey][]*types.KnowledgeBase)
	for _, kb := range kbs {
		sid := ""
		if kb.HasVectorStore() {
			sid = *kb.VectorStoreID
		}
		key := partitionKey{storeID: sid, knowledgeDomainID: kb.KnowledgeDomainID}
		buckets[key] = append(buckets[key], kb)
	}

	groups := make([]*storeGroup, 0, len(buckets))
	for key, groupKBs := range buckets {
		var storeIDPtr *string
		if key.storeID != "" {
			sid := key.storeID
			storeIDPtr = &sid
		}
		engine, err := retriever.CreateRetrieveEngineForKB(
			ctx, s.retrieveEngine, s.ownership, key.knowledgeDomainID, storeIDPtr)
		if err != nil {
			return nil, classifyFactoryError(ctx, err, key.knowledgeDomainID, key.storeID)
		}
		baseParams, err := s.buildRetrievalParams(
			ctx, engine, primary, groupKBs, params, matchCount)
		if err != nil {
			return nil, fmt.Errorf("build store-group params: %w", err)
		}
		ids := make([]string, len(groupKBs))
		for i, kb := range groupKBs {
			ids[i] = kb.ID
		}
		groups = append(groups, &storeGroup{
			StoreID:                key.storeID,
			OwnerKnowledgeDomainID: key.knowledgeDomainID,
			KBIDs:                  ids,
			Engine:                 engine,
			BaseParams:             baseParams,
			TopK:                   matchCount,
		})
	}
	return groups, nil
}

// classifyFactoryError translates retriever sentinels into typed AppErrors
// without leaking the store UUID into the user-facing message. The UUID is
// recorded in the structured log only, sanitized via SanitizeForLog to
// defeat log-injection through CR/LF in store IDs.
func classifyFactoryError(
	ctx context.Context, err error, knowledgeDomainID uint64, storeID string,
) error {
	logger.WarnWithFields(ctx, logger.Fields{
		"knowledge_domain_id": knowledgeDomainID,
		"store_id":            secutils.SanitizeForLog(storeID),
		"reason":              "resolve store engine",
	}, err.Error())
	switch {
	case errors.Is(err, retriever.ErrVectorStoreForbidden):
		return apperrors.NewVectorStoreBindingInvalidError(
			"vector store bound to the knowledge base is not available")
	case errors.Is(err, retriever.ErrVectorStoreNotFound):
		return apperrors.NewVectorStoreUnavailableError(
			"vector store is currently unavailable")
	case errors.Is(err, retriever.ErrKnowledgeDomainInfoMissing):
		return apperrors.NewVectorStoreBindingInvalidError(
			"knowledgeDomain information missing in context")
	default:
		return err
	}
}

// authorizeKBAccess validates every requested KB against the caller's direct
// enterprise grants. Explicit read grants may cross department boundaries.
//
// Returning NotFound rather than Forbidden avoids leaking the existence
// of unauthorized KB IDs that the caller could not otherwise observe.
// Structured logs record the rejection with the offending kb_id (always
// safe — KB IDs are UUIDs without sensitive content) and the requesting
// knowledgeDomain for audit.
func (s *knowledgeBaseService) authorizeKBAccess(
	ctx context.Context,
	kbs []*types.KnowledgeBase,
	requestKnowledgeDomainID uint64,
	params types.SearchParams,
) error {
	if len(kbs) == 0 {
		return nil
	}

	knowledgeByID := make(map[string]*types.Knowledge, len(params.KnowledgeIDs))
	for _, knowledgeID := range uniqueNonEmptyStrings(params.KnowledgeIDs) {
		knowledge, err := s.kgRepo.GetKnowledgeByIDOnly(ctx, knowledgeID)
		if err != nil || knowledge == nil {
			return apperrors.NewNotFoundError("knowledge document not found")
		}
		knowledgeByID[knowledgeID] = knowledge
	}

	for _, kb := range kbs {
		scope := &types.KnowledgeBaseAccessScope{
			Allowed:    kb.KnowledgeDomainID == requestKnowledgeDomainID,
			FullAccess: kb.KnowledgeDomainID == requestKnowledgeDomainID,
		}
		if s.accessService != nil {
			var err error
			scope, err = s.accessService.ResolveKnowledgeBaseAccess(ctx, kb)
			if err != nil {
				return err
			}
		}
		if scope != nil && scope.Allowed && scope.FullAccess {
			continue
		}
		if scope != nil && scope.Allowed {
			requestedForKB := 0
			for knowledgeID, knowledge := range knowledgeByID {
				if knowledge.KnowledgeBaseID != kb.ID {
					continue
				}
				requestedForKB++
				if !scope.AllowsKnowledge(knowledgeID) {
					return apperrors.NewNotFoundError("knowledge document not found")
				}
			}
			if requestedForKB > 0 {
				continue
			}
		}
		logger.WarnWithFields(ctx, logger.Fields{
			"caller_knowledge_domain_id": requestKnowledgeDomainID,
			"kb_knowledge_domain_id":     kb.KnowledgeDomainID,
			"kb_id":                      kb.ID,
			"reason":                     "knowledge base was not granted to caller",
		}, "search scope rejected")
		return apperrors.NewNotFoundError("knowledge base not found")
	}
	return nil
}

// validateSameEmbeddingModel rejects multi-KB searches that span more than
// one resolved embedding-model identity key. Single-KB calls no-op.
//
// Graph-only KBs (empty resolved key) are tolerated: if every
// KB lacks an embedding model, validation passes and HybridSearch returns
// an empty result set via the allBaseParamsEmpty fast path.
//
// Log fields are sanitized via secutils.SanitizeForLog because resolved
// keys are derived from model.Parameters.BaseURL, which is knowledgeDomain-
// configured and can contain CR/LF or other control characters.
func (s *knowledgeBaseService) validateSameEmbeddingModel(
	ctx context.Context,
	kbs []*types.KnowledgeBase,
) error {
	if len(kbs) <= 1 {
		return nil
	}
	keys := s.ResolveEmbeddingModelKeys(ctx, kbs)
	var seen string
	for _, kb := range kbs {
		k, ok := keys[kb.ID]
		if !ok || k == "" {
			// Graph-only carve-out: KB has no embedding model.
			continue
		}
		if seen == "" {
			seen = k
			continue
		}
		if k != seen {
			logger.WarnWithFields(ctx, logger.Fields{
				"primary_key": secutils.SanitizeForLog(seen),
				"diverging":   secutils.SanitizeForLog(k),
				"kb_id":       kb.ID,
				"kb_count":    len(kbs),
			}, "multi-KB search rejected: embedding models differ")
			return apperrors.NewBadRequestError(
				"selected knowledge bases use different embedding models; " +
					"multi-KB search requires every knowledge base to share a single embedding model")
		}
	}
	return nil
}
