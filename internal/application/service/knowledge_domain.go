package service

import (
	"context"
	"errors"
	"time"

	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// ListKnowledgeDomainsParams defines parameters for listing knowledgeDomains with filtering and pagination
type ListKnowledgeDomainsParams struct {
	Page     int    // Page number for pagination
	PageSize int    // Number of items per page
	Status   string // Filter by knowledgeDomain status
	Name     string // Filter by knowledgeDomain name
}

// knowledgeDomainService implements the KnowledgeDomainService interface
type knowledgeDomainService struct {
	repo interfaces.KnowledgeDomainRepository // Repository for knowledgeDomain data operations
}

// NewKnowledgeDomainService creates a new knowledgeDomain service instance
func NewKnowledgeDomainService(repo interfaces.KnowledgeDomainRepository) interfaces.KnowledgeDomainService {
	return &knowledgeDomainService{repo: repo}
}

// CreateKnowledgeDomain creates a new knowledgeDomain
func (s *knowledgeDomainService) CreateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) (*types.KnowledgeDomain, error) {
	logger.Info(ctx, "Start creating knowledgeDomain")

	if knowledgeDomain.Name == "" {
		logger.Error(ctx, "KnowledgeDomain name cannot be empty")
		return nil, errors.New("knowledgeDomain name cannot be empty")
	}

	logger.Infof(ctx, "Creating knowledgeDomain, name: %s", knowledgeDomain.Name)

	knowledgeDomain.Status = "active"
	knowledgeDomain.CreatedAt = time.Now()
	knowledgeDomain.UpdatedAt = time.Now()

	logger.Info(ctx, "Saving knowledgeDomain information to database")
	if err := s.repo.CreateKnowledgeDomain(ctx, knowledgeDomain); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_name": knowledgeDomain.Name,
		})
		return nil, err
	}

	logger.Infof(ctx, "KnowledgeDomain created successfully, ID: %d, name: %s", knowledgeDomain.ID, knowledgeDomain.Name)
	return knowledgeDomain, nil
}

// GetKnowledgeDomainByID retrieves a knowledgeDomain by their ID
func (s *knowledgeDomainService) GetKnowledgeDomainByID(ctx context.Context, id uint64) (*types.KnowledgeDomain, error) {
	if id == 0 {
		logger.Error(ctx, "KnowledgeDomain ID cannot be 0")
		return nil, errors.New("knowledgeDomain ID cannot be 0")
	}

	knowledgeDomain, err := s.repo.GetKnowledgeDomainByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": id,
		})
		return nil, err
	}

	return knowledgeDomain, nil
}

// GetKnowledgeDomainsByIDs batches GetKnowledgeDomainByID; returns a map keyed by knowledgeDomain ID.
func (s *knowledgeDomainService) GetKnowledgeDomainsByIDs(ctx context.Context, ids []uint64) (map[uint64]*types.KnowledgeDomain, error) {
	return s.repo.GetKnowledgeDomainsByIDs(ctx, ids)
}

// ListKnowledgeDomains retrieves a list of all knowledgeDomains
func (s *knowledgeDomainService) ListKnowledgeDomains(ctx context.Context) ([]*types.KnowledgeDomain, error) {
	knowledgeDomains, err := s.repo.ListKnowledgeDomains(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}

	logger.Infof(ctx, "KnowledgeDomain list retrieved successfully, total: %d", len(knowledgeDomains))
	return knowledgeDomains, nil
}

// UpdateKnowledgeDomain updates an existing knowledgeDomain's information
func (s *knowledgeDomainService) UpdateKnowledgeDomain(ctx context.Context, knowledgeDomain *types.KnowledgeDomain) (*types.KnowledgeDomain, error) {
	if knowledgeDomain.ID == 0 {
		logger.Error(ctx, "KnowledgeDomain ID cannot be 0")
		return nil, errors.New("knowledgeDomain ID cannot be 0")
	}

	logger.Infof(ctx, "Updating knowledgeDomain, ID: %d, name: %s", knowledgeDomain.ID, knowledgeDomain.Name)

	knowledgeDomain.UpdatedAt = time.Now()
	logger.Info(ctx, "Saving knowledgeDomain information to database")

	if err := s.repo.UpdateKnowledgeDomain(ctx, knowledgeDomain); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomain.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "KnowledgeDomain updated successfully, ID: %d", knowledgeDomain.ID)
	return knowledgeDomain, nil
}

// GetPlatformRuntimeConfig returns the singleton configuration shared by every
// knowledge domain.
func (s *knowledgeDomainService) GetPlatformRuntimeConfig(
	ctx context.Context,
) (*types.PlatformRuntimeConfig, error) {
	runtimeConfig, err := s.repo.GetPlatformRuntimeConfig(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}
	return runtimeConfig, nil
}

// UpdatePlatformRuntimeConfig updates the singleton configuration shared by
// every knowledge domain. The route layer restricts this operation to the
// system administrator.
func (s *knowledgeDomainService) UpdatePlatformRuntimeConfig(
	ctx context.Context,
	config *types.PlatformRuntimeConfig,
) (*types.PlatformRuntimeConfig, error) {
	if config == nil {
		return nil, errors.New("runtime configuration is required")
	}
	if err := s.repo.UpdatePlatformRuntimeConfig(ctx, config); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}
	return config, nil
}

// DeleteKnowledgeDomain removes a knowledgeDomain by their ID
func (s *knowledgeDomainService) DeleteKnowledgeDomain(ctx context.Context, id uint64) error {
	logger.Info(ctx, "Start deleting knowledgeDomain")

	if id == 0 {
		logger.Error(ctx, "KnowledgeDomain ID cannot be 0")
		return errors.New("knowledgeDomain ID cannot be 0")
	}

	logger.Infof(ctx, "Deleting knowledgeDomain, ID: %d", id)

	// Get knowledgeDomain information for logging
	knowledgeDomain, err := s.repo.GetKnowledgeDomainByID(ctx, id)
	if err != nil {
		if err.Error() == "record not found" {
			logger.Warnf(ctx, "KnowledgeDomain to be deleted does not exist, ID: %d", id)
		} else {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"knowledge_domain_id": id,
			})
			return err
		}
	} else {
		logger.Infof(ctx, "Deleting knowledgeDomain, ID: %d, name: %s", id, knowledgeDomain.Name)
	}

	err = s.repo.DeleteKnowledgeDomain(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": id,
		})
		return err
	}

	logger.Infof(ctx, "KnowledgeDomain deleted successfully, ID: %d", id)
	return nil
}

// ListAllKnowledgeDomains lists all knowledgeDomains (for users with cross-knowledgeDomain access permission)
// This method returns all knowledgeDomains without filtering, intended for admin users
func (s *knowledgeDomainService) ListAllKnowledgeDomains(ctx context.Context) ([]*types.KnowledgeDomain, error) {
	knowledgeDomains, err := s.repo.ListKnowledgeDomains(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}

	logger.Infof(ctx, "All knowledgeDomains list retrieved successfully, total: %d", len(knowledgeDomains))
	return knowledgeDomains, nil
}

// BulkSetStorageQuota delegates to the repository. Validation is
// minimal — quotaBytes <= 0 is rejected because the storage-quota
// enforcement in knowledge_create.go treats <=0 as "unlimited", which
// is never what a SystemAdmin pressing "apply default" intends.
func (s *knowledgeDomainService) BulkSetStorageQuota(ctx context.Context, quotaBytes int64) (int64, error) {
	if quotaBytes <= 0 {
		return 0, errors.New("quota must be positive")
	}
	affected, err := s.repo.BulkSetStorageQuota(ctx, quotaBytes)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"quota_bytes": quotaBytes})
		return 0, err
	}
	logger.Infof(ctx, "Bulk set storage_quota=%d on %d knowledgeDomains", quotaBytes, affected)
	return affected, nil
}

// SearchKnowledgeDomains searches knowledgeDomains with pagination and filters
func (s *knowledgeDomainService) SearchKnowledgeDomains(ctx context.Context, keyword string, knowledgeDomainID uint64, page, pageSize int) ([]*types.KnowledgeDomain, int64, error) {
	knowledgeDomains, total, err := s.repo.SearchKnowledgeDomains(ctx, keyword, knowledgeDomainID, page, pageSize)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"keyword":           keyword,
			"knowledgeDomainID": knowledgeDomainID,
			"page":              page,
			"pageSize":          pageSize,
		})
		return nil, 0, err
	}

	logger.Infof(ctx, "KnowledgeDomains search completed, keyword: %s, knowledgeDomainID: %d, page: %d, pageSize: %d, total: %d, found: %d",
		keyword, knowledgeDomainID, page, pageSize, total, len(knowledgeDomains))
	return knowledgeDomains, total, nil
}

// GetKnowledgeDomainByIDForUser gets a knowledgeDomain by ID with permission check
// This method verifies that the user has permission to access the knowledgeDomain
func (s *knowledgeDomainService) GetKnowledgeDomainByIDForUser(ctx context.Context, knowledgeDomainID uint64, userID string) (*types.KnowledgeDomain, error) {
	knowledgeDomain, err := s.repo.GetKnowledgeDomainByID(ctx, knowledgeDomainID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomainID,
			"user_id":             userID,
		})
		return nil, err
	}

	return knowledgeDomain, nil
}
