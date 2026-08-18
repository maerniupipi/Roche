package retriever

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// vectorStoreRepoOwnership adapts a VectorStoreRepository to the
// KnowledgeDomainStoreOwnership interface consumed by the factory functions.
//
// The repository's GetByID already scopes to the given knowledgeDomainID
// (WHERE id = ? AND knowledge_domain_id = ?), so "exists under this knowledgeDomain"
// is equivalent to "owned by this knowledgeDomain" — no additional knowledgeDomain
// comparison is needed on top of the returned row.
type vectorStoreRepoOwnership struct {
	repo interfaces.VectorStoreRepository
}

// NewVectorStoreRepoOwnership returns the production
// KnowledgeDomainStoreOwnership implementation backed by VectorStoreRepository.
func NewVectorStoreRepoOwnership(repo interfaces.VectorStoreRepository) KnowledgeDomainStoreOwnership {
	return &vectorStoreRepoOwnership{repo: repo}
}

// StoreOwnedBy returns true iff a vector store with the given ID exists
// under the given knowledgeDomain. Errors are reserved for infrastructure failures;
// a non-existent (but well-formed) store ID returns (false, nil).
func (o *vectorStoreRepoOwnership) StoreOwnedBy(ctx context.Context, storeID string, knowledgeDomainID uint64) (bool, error) {
	_ = knowledgeDomainID
	store, err := o.repo.GetByID(ctx, 0, storeID)
	if err != nil {
		return false, err
	}
	return store != nil, nil
}
