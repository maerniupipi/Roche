package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// KnowledgeDomainService defines knowledge-domain operations.
type KnowledgeDomainService interface {
	// CreateKnowledgeDomain creates a knowledgeDomain.
	CreateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) (*types.KnowledgeDomain, error)
	// GetKnowledgeDomainByID gets a knowledgeDomain by ID.
	GetKnowledgeDomainByID(ctx context.Context, id uint64) (*types.KnowledgeDomain, error)
	// GetKnowledgeDomainsByIDs batches GetKnowledgeDomainByID for multiple IDs in a single
	// query. Missing domains are absent from the returned map.
	GetKnowledgeDomainsByIDs(ctx context.Context, ids []uint64) (map[uint64]*types.KnowledgeDomain, error)
	// ListKnowledgeDomains lists knowledgeDomains.
	ListKnowledgeDomains(ctx context.Context) ([]*types.KnowledgeDomain, error)
	// UpdateKnowledgeDomain updates a knowledgeDomain.
	UpdateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) (*types.KnowledgeDomain, error)
	// GetPlatformRuntimeConfig returns the singleton parser, storage and
	// retrieval configuration shared by all knowledge domains.
	GetPlatformRuntimeConfig(ctx context.Context) (*types.PlatformRuntimeConfig, error)
	// UpdatePlatformRuntimeConfig updates the singleton parser, storage and
	// retrieval configuration shared by all knowledge domains.
	UpdatePlatformRuntimeConfig(ctx context.Context, config *types.PlatformRuntimeConfig) (*types.PlatformRuntimeConfig, error)
	// DeleteKnowledgeDomain deletes a knowledgeDomain.
	DeleteKnowledgeDomain(ctx context.Context, id uint64) error
	// ListAllKnowledgeDomains lists all knowledgeDomains for system administration.
	ListAllKnowledgeDomains(ctx context.Context) ([]*types.KnowledgeDomain, error)
	// BulkSetStorageQuota overwrites every knowledgeDomain's storage quota with
	// quotaBytes. Returns how many rows were affected. Used by the
	// SystemAdmin "apply default to all knowledgeDomains" action; bypasses the
	// domain identity update endpoint. quotaBytes must be > 0;
	// callers are responsible for resolving GB→bytes.
	BulkSetStorageQuota(ctx context.Context, quotaBytes int64) (int64, error)
	// SearchKnowledgeDomains searches knowledgeDomains.
	SearchKnowledgeDomains(ctx context.Context, keyword string, knowledgeDomainID uint64, page, pageSize int) ([]*types.KnowledgeDomain, int64, error)
	// GetKnowledgeDomainByIDForUser gets a domain by ID for a user-facing view.
	GetKnowledgeDomainByIDForUser(ctx context.Context, knowledgeDomainID uint64, userID string) (*types.KnowledgeDomain, error)
}

// KnowledgeDomainRepository defines knowledge-domain persistence.
type KnowledgeDomainRepository interface {
	// CreateKnowledgeDomain creates a knowledgeDomain
	CreateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) error
	// GetKnowledgeDomainByID gets a knowledgeDomain by ID
	GetKnowledgeDomainByID(ctx context.Context, id uint64) (*types.KnowledgeDomain, error)
	// GetKnowledgeDomainsByIDs batches GetKnowledgeDomainByID; see KnowledgeDomainService.GetKnowledgeDomainsByIDs.
	GetKnowledgeDomainsByIDs(ctx context.Context, ids []uint64) (map[uint64]*types.KnowledgeDomain, error)
	// ListKnowledgeDomains lists all knowledgeDomains
	ListKnowledgeDomains(ctx context.Context) ([]*types.KnowledgeDomain, error)
	// SearchKnowledgeDomains searches knowledgeDomains with pagination and filters
	SearchKnowledgeDomains(ctx context.Context, keyword string, knowledgeDomainID uint64, page, pageSize int) ([]*types.KnowledgeDomain, int64, error)
	// UpdateKnowledgeDomain updates a knowledgeDomain
	UpdateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) error
	// GetPlatformRuntimeConfig returns the singleton runtime configuration.
	GetPlatformRuntimeConfig(ctx context.Context) (*types.PlatformRuntimeConfig, error)
	// UpdatePlatformRuntimeConfig updates the singleton runtime configuration.
	UpdatePlatformRuntimeConfig(ctx context.Context, config *types.PlatformRuntimeConfig) error
	// DeleteKnowledgeDomain deletes a knowledgeDomain
	DeleteKnowledgeDomain(ctx context.Context, id uint64) error
	// AdjustStorageUsed adjusts the storage used for a knowledgeDomain
	AdjustStorageUsed(ctx context.Context, knowledgeDomainID uint64, delta int64) error
	// BulkSetStorageQuota — see KnowledgeDomainService.BulkSetStorageQuota.
	BulkSetStorageQuota(ctx context.Context, quotaBytes int64) (int64, error)
}
