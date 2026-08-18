package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

type KnowledgeDomainAdminRepository interface {
	Upsert(ctx context.Context, admin *types.KnowledgeDomainAdmin) error
	Get(ctx context.Context, userID string, knowledgeDomainID uint64) (*types.KnowledgeDomainAdmin, error)
	ListDomainIDsByUser(ctx context.Context, userID string) ([]uint64, error)
	ListByDomain(ctx context.Context, knowledgeDomainID uint64, search string, offset, limit int) ([]*types.KnowledgeDomainAdmin, int64, error)
	Delete(ctx context.Context, userID string, knowledgeDomainID uint64) error
}

type KnowledgeDomainAdminService interface {
	Grant(ctx context.Context, userID string, knowledgeDomainID uint64, grantedBy string) (*types.KnowledgeDomainAdmin, error)
	Revoke(ctx context.Context, userID string, knowledgeDomainID uint64) error
	IsAdmin(ctx context.Context, userID string, knowledgeDomainID uint64) (bool, error)
	ListDomainIDs(ctx context.Context, userID string) ([]uint64, error)
	ListPage(ctx context.Context, knowledgeDomainID uint64, search string, page, pageSize int) ([]*types.KnowledgeDomainAdmin, int64, error)
}
