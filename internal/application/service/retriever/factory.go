package retriever

import (
	"context"
	"errors"
	"slices"

	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// Sentinel errors returned by factory functions. Callers may use errors.Is to
// classify. User-facing responses MUST wrap or replace these with generic
// messages — the sentinels intentionally omit store UUIDs to avoid enumeration
// leaks. Structured logs inside the factory record the knowledgeDomain/store IDs.
var (
	// ErrKnowledgeDomainInfoMissing is returned when the factory needs a knowledgeDomain from
	// context (synchronous, unbound KB path) and none is present.
	ErrKnowledgeDomainInfoMissing = errors.New("knowledgeDomain info not found in context")

	// ErrVectorStoreNotFound is returned when the store ID is not registered
	// (or an internal lookup for ownership failed). Async workers should treat
	// this as non-retryable.
	ErrVectorStoreNotFound = errors.New("vector store not available")

	// ErrVectorStoreForbidden is returned when the resolved store is not
	// owned by the given knowledgeDomain. This guards against cross-knowledgeDomain access
	// in case the upstream validation layer has a gap. Async workers should
	// treat this as non-retryable.
	ErrVectorStoreForbidden = errors.New("vector store access denied")
)

// KnowledgeDomainStoreOwnership abstracts the lookup used by factory functions to
// verify that a given vector store ID is owned by the given knowledgeDomain ID.
//
// Production implementations wrap the VectorStoreRepository; tests inject
// in-memory fakes so they can cover the ownership branches without touching
// a database.
type KnowledgeDomainStoreOwnership interface {
	// StoreOwnedBy reports whether the store with the given ID is owned
	// by the given knowledgeDomain. When the store does not exist, it returns
	// (false, nil). Errors are reserved for infrastructure failures such as
	// database connectivity issues.
	StoreOwnedBy(ctx context.Context, storeID string, knowledgeDomainID uint64) (bool, error)
}

// VerifyBinding asserts that a non-empty storeID is owned by knowledgeDomainID and
// registered in the in-memory engine registry. It encapsulates the two
// checks that gate every store-bound resolution so that callers outside
// the retriever package (notably the KB create-validation path) can reuse
// the same sentinel hierarchy instead of duplicating the logic.
//
// Resolution rules:
//
//   - ownership.StoreOwnedBy returns an infrastructure error → that error
//     is returned verbatim so callers can decide retry/abort.
//   - ownership returns (false, nil) → ErrVectorStoreForbidden.
//   - ownership returns (true, nil) + registry.GetByStoreID fails →
//     ErrVectorStoreNotFound.
//   - all checks succeed → nil.
//
// VerifyBinding itself never echoes the store UUID; callers MUST wrap the
// sentinels into user-facing errors at the boundary (and log the
// knowledgeDomain/store pair via structured fields when appropriate).
//
// resolveBoundEngine (below) intentionally does NOT delegate to VerifyBinding
// because it also needs the resolved engine service; sharing would require
// either a second registry lookup or returning the service from VerifyBinding,
// both of which dilute the helper's single purpose. The two paths are kept
// in lockstep by the factory_test.go matrix.
func VerifyBinding(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	ownership KnowledgeDomainStoreOwnership,
	knowledgeDomainID uint64,
	storeID string,
) error {
	owned, err := ownership.StoreOwnedBy(ctx, storeID, knowledgeDomainID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrVectorStoreForbidden
	}
	if _, err := registry.GetByStoreID(storeID); err != nil {
		return ErrVectorStoreNotFound
	}
	return nil
}

// CreateRetrieveEngineForKB returns a CompositeRetrieveEngine resolved from
// a KB's VectorStore binding.
//
// Resolution rules:
//
//   - vectorStoreID == nil || *vectorStoreID == "" →
//     falls back to the knowledgeDomain's effective engines (env-store flow driven
//     by RETRIEVE_DRIVER). KnowledgeDomainInfo is read from ctx.
//   - otherwise →
//     1) ownership.StoreOwnedBy(*storeID, knowledgeDomainID) must return true;
//     cross-knowledgeDomain attempts yield ErrVectorStoreForbidden.
//     2) registry.GetByStoreID(*storeID) must succeed;
//     unregistered stores yield ErrVectorStoreNotFound.
//     3) the single engine is wrapped by NewCompositeRetrieveEngine so
//     that its Support()-based retriever-type matching is preserved.
//
// Use this for 23 synchronous call sites across the application services.
// Async task handlers that cannot rely on ctx-based KnowledgeDomainInfo (currently:
// ProcessKBDeleteTask, ProcessIndexDelete) must use
// CreateRetrieveEngineFromPayload instead.
func CreateRetrieveEngineForKB(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	ownership KnowledgeDomainStoreOwnership,
	knowledgeDomainID uint64,
	vectorStoreID *string,
) (*CompositeRetrieveEngine, error) {
	// Normalize nil and empty-string pointer to "unbound" so that callers
	// cannot accidentally route an empty UUID into GetByStoreID.
	if vectorStoreID == nil || *vectorStoreID == "" {
		knowledgeDomainInfo, ok := types.KnowledgeDomainInfoFromContext(ctx)
		if !ok {
			return nil, ErrKnowledgeDomainInfoMissing
		}
		return NewCompositeRetrieveEngine(registry, knowledgeDomainInfo.GetEffectiveEngines())
	}

	return resolveBoundEngine(ctx, registry, ownership, knowledgeDomainID, *vectorStoreID)
}

// CreateRetrieveEngineFromPayload is the async-task variant. It does not
// read KnowledgeDomainInfo from ctx because async handlers do not populate it.
// Instead, knowledgeDomainID is passed explicitly from the deserialized payload and
// is verified against the store's knowledgeDomain when vectorStoreID is non-empty.
//
// Tasks enqueued before vectorStoreID was added to the payload decode it as
// nil and transparently fall back to the pre-serialized effectiveEngines
// path — no in-flight task is lost across upgrades.
func CreateRetrieveEngineFromPayload(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	ownership KnowledgeDomainStoreOwnership,
	knowledgeDomainID uint64,
	effectiveEngines []types.RetrieverEngineParams,
	vectorStoreID *string,
) (*CompositeRetrieveEngine, error) {
	if vectorStoreID == nil || *vectorStoreID == "" {
		return NewCompositeRetrieveEngine(registry, effectiveEngines)
	}

	return resolveBoundEngine(ctx, registry, ownership, knowledgeDomainID, *vectorStoreID)
}

// resolveBoundEngine is the shared ownership-verified lookup path used by
// both CreateRetrieveEngineForKB and CreateRetrieveEngineFromPayload. It
// returns sentinel errors so that handlers can classify them (for example,
// async workers convert Forbidden/NotFound into asynq.SkipRetry).
func resolveBoundEngine(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	ownership KnowledgeDomainStoreOwnership,
	knowledgeDomainID uint64,
	storeID string,
) (*CompositeRetrieveEngine, error) {
	owned, err := ownership.StoreOwnedBy(ctx, storeID, knowledgeDomainID)
	if err != nil {
		// Infrastructure failure — record the raw error for operators but
		// do not leak internals to the caller.
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomainID,
			"store_id":            storeID,
			"reason":              "ownership lookup failed",
		})
		return nil, ErrVectorStoreNotFound
	}
	if !owned {
		// Cross-knowledgeDomain attempt (or the store has been deleted in the
		// meantime). Log with WARN so that audits can surface probing.
		logger.Warnf(ctx,
			"[retriever.factory] cross-knowledgeDomain store access attempted: knowledgeDomain=%d store=%s",
			knowledgeDomainID, storeID)
		return nil, ErrVectorStoreForbidden
	}

	svc, err := registry.GetByStoreID(storeID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomainID,
			"store_id":            storeID,
			"reason":              "store not registered",
		})
		return nil, ErrVectorStoreNotFound
	}

	// Build the composite directly from the resolved service.
	//
	// We cannot delegate to NewCompositeRetrieveEngine here because that
	// function resolves engines through registry.GetRetrieveEngineService,
	// which reads from the byEngineType map (env stores). DB stores live
	// in the byStoreID map and are not reachable via engine type alone —
	// multiple stores can share the same engine type.
	//
	// Semantics: a KB bound to a DB store uses every retriever type that
	// store supports. This intentionally overrides the knowledgeDomain-level
	// effective-engines filter, because binding a KB to a specific store
	// is an explicit opt-out of knowledgeDomain-default routing.
	return &CompositeRetrieveEngine{
		engineInfos: []*engineInfo{{
			retrieveEngine: svc,
			retrieverType:  slices.Clone(svc.Support()),
		}},
	}, nil
}
