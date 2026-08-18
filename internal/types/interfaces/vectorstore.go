package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// StoreRegistry provides VectorStore-based engine registration/lookup.
// Separated from RetrieveEngineRegistry to avoid changing the existing interface
// used by 6 services (17 call sites). Phase 2 may merge into RetrieveEngineRegistry.
type StoreRegistry interface {
	// RegisterWithStoreID registers an engine service by VectorStore ID.
	// Upsert semantics: existing entry is overwritten silently.
	RegisterWithStoreID(storeID string, svc RetrieveEngineService)
	// GetByStoreID retrieves an engine service by VectorStore ID.
	GetByStoreID(storeID string) (RetrieveEngineService, error)
	// UnregisterByStoreID removes an engine service by VectorStore ID (idempotent).
	UnregisterByStoreID(storeID string)
}

// EngineFactory creates a RetrieveEngineService from a VectorStore's config.
// Defined as a function type to avoid circular imports between container and service packages.
type EngineFactory func(ctx context.Context, store types.VectorStore) (RetrieveEngineService, error)

// VectorStoreService defines platform-wide vector store management.
type VectorStoreService interface {
	// CreateStore validates and creates a new vector store.
	CreateStore(ctx context.Context, store *types.VectorStore) error
	// UpdateStore updates an existing vector store (name only).
	UpdateStore(ctx context.Context, store *types.VectorStore) error
	// DeleteStore deletes a platform-wide vector store by ID.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	// Rejects deletion when any active knowledge base is bound to the store
	// (binding guard); the caller must unbind or delete those KBs first.
	DeleteStore(ctx context.Context, knowledgeDomainID uint64, id string) error
	// TestConnection tests connectivity to a vector database.
	// Returns the detected server version on success (e.g., "7.10.1"), empty string if unknown.
	//
	// Validation-free: intended for trusted configs (env stores, stored
	// configs already validated at create time). Handlers receiving raw
	// user input MUST use TestRawConnection instead.
	TestConnection(ctx context.Context, engineType types.RetrieverEngineType, config types.ConnectionConfig) (string, error)
	// TestRawConnection validates raw user-supplied connection config
	// (engine-type allowlist, required fields, SSRF policy) and then delegates
	// to TestConnection. This is the entry point for unpersisted user input.
	TestRawConnection(ctx context.Context, engineType types.RetrieverEngineType, config types.ConnectionConfig) (string, error)
	// SaveDetectedVersion updates the connection_config.version for a stored vector store.
	SaveDetectedVersion(ctx context.Context, store *types.VectorStore, version string) error

	// ResolveStoreView returns the API-safe display projection of a single
	// store ID. Tries DB stores first, then the
	// cached env-store set. Returns types.UnavailableStoreDisplay() when the
	// store cannot be resolved — callers should still succeed in such cases
	// (the response carries a "unavailable" status signal for the UI).
	//
	// Never returns connection credentials in any form: the StoreDisplay
	// payload carries only Name / Source / EngineType / Status.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	ResolveStoreView(ctx context.Context, knowledgeDomainID uint64, storeID string) (types.StoreDisplay, error)

	// BatchResolveStoreView resolves multiple store IDs in a single DB read
	// plus the cached env-store match. Returned map keys are the storeIDs
	// originally requested; missing IDs map to types.UnavailableStoreDisplay().
	//
	// Intended for list endpoints that need store metadata for many KBs at
	// once without incurring N+1 ResolveStoreView calls.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	BatchResolveStoreView(ctx context.Context, knowledgeDomainID uint64, storeIDs []string) (map[string]types.StoreDisplay, error)

	// EnvDefaultStoreView returns the display payload for KBs that fall
	// back to the env-configured store. Unlike DefaultStoreDisplay() in
	// the types package, this method also fills the engine type
	// (e.g. "milvus") so UIs can render the same badge shape for
	// env-bound and user-bound KBs. Independent of ResolveStoreView so
	// list paths can use it without violating the "no per-KB
	// ResolveStoreView" invariant.
	EnvDefaultStoreView(ctx context.Context) types.StoreDisplay
}

// VectorStoreRepository defines the repository interface for VectorStore CRUD.
type VectorStoreRepository interface {
	// Create creates a new vector store
	Create(ctx context.Context, store *types.VectorStore) error
	// GetByID retrieves a platform-wide vector store by ID.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	GetByID(ctx context.Context, knowledgeDomainID uint64, id string) (*types.VectorStore, error)
	// List lists all platform-wide vector stores.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	List(ctx context.Context, knowledgeDomainID uint64) ([]*types.VectorStore, error)
	// Update updates a vector store (only mutable fields: name)
	Update(ctx context.Context, store *types.VectorStore) error
	// UpdateConnectionConfig updates only the connection_config column
	UpdateConnectionConfig(ctx context.Context, store *types.VectorStore) error
	// Delete soft-deletes a platform-wide vector store.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	Delete(ctx context.Context, knowledgeDomainID uint64, id string) error
	// ExistsByEndpointAndIndex checks globally for the same endpoint and index.
	// knowledgeDomainID is retained temporarily for interface compatibility and is ignored.
	ExistsByEndpointAndIndex(ctx context.Context, knowledgeDomainID uint64, engineType types.RetrieverEngineType, endpoint string, indexName string) (bool, error)
}
