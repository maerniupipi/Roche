package dto

import (
	"context"
	"time"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

// KnowledgeDomainResponse exposes only knowledge-domain identity and storage
// accounting. Platform runtime configuration is managed separately.
type KnowledgeDomainResponse struct {
	ID           uint64         `json:"id"`
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	NameEn       string         `json:"name_en"`
	Description  string         `json:"description"`
	Status       string         `json:"status"`
	StorageQuota int64          `json:"storage_quota"`
	StorageUsed  int64          `json:"storage_used"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at"`
}

func NewKnowledgeDomainResponse(_ context.Context, domain *types.KnowledgeDomain) *KnowledgeDomainResponse {
	if domain == nil {
		return nil
	}
	return &KnowledgeDomainResponse{
		ID:           domain.ID,
		Code:         domain.Code,
		Name:         domain.Name,
		NameEn:       domain.NameEn,
		Description:  domain.Description,
		Status:       domain.Status,
		StorageQuota: domain.StorageQuota,
		StorageUsed:  domain.StorageUsed,
		CreatedAt:    domain.CreatedAt,
		UpdatedAt:    domain.UpdatedAt,
		DeletedAt:    domain.DeletedAt,
	}
}

func NewKnowledgeDomainResponses(ctx context.Context, domains []*types.KnowledgeDomain) []*KnowledgeDomainResponse {
	out := make([]*KnowledgeDomainResponse, 0, len(domains))
	for _, domain := range domains {
		out = append(out, NewKnowledgeDomainResponse(ctx, domain))
	}
	return out
}
