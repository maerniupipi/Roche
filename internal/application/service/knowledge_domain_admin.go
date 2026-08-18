package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var ErrKnowledgeDomainAdminNotFound = errors.New("knowledgeDomain administrator not found")

type knowledgeDomainAdminService struct {
	repo          interfaces.KnowledgeDomainAdminRepository
	businessAudit *BusinessAuditRecorder
}

func NewKnowledgeDomainAdminService(
	repo interfaces.KnowledgeDomainAdminRepository,
	businessAudit *BusinessAuditRecorder,
) interfaces.KnowledgeDomainAdminService {
	return &knowledgeDomainAdminService{repo: repo, businessAudit: businessAudit}
}

func (s *knowledgeDomainAdminService) Grant(
	ctx context.Context,
	userID string,
	knowledgeDomainID uint64,
	grantedBy string,
) (*types.KnowledgeDomainAdmin, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || knowledgeDomainID == 0 {
		return nil, errors.New("user and knowledgeDomain are required")
	}
	var grantedByPtr *string
	if value := strings.TrimSpace(grantedBy); value != "" {
		grantedByPtr = &value
	}
	admin := &types.KnowledgeDomainAdmin{
		KnowledgeDomainID: knowledgeDomainID,
		UserID:            userID,
		GrantedBy:         grantedByPtr,
		Status:            types.KnowledgeDomainAdminStatusActive,
	}
	if err := s.repo.Upsert(ctx, admin); err != nil {
		return nil, err
	}

	// Audit: domain admin granted
	s.businessAudit.RecordDomainAdminGranted(ctx, knowledgeDomainID, "",
		userID, "", "", false)

	return s.repo.Get(ctx, userID, knowledgeDomainID)
}

func (s *knowledgeDomainAdminService) Revoke(ctx context.Context, userID string, knowledgeDomainID uint64) error {
	if err := s.repo.Delete(ctx, userID, knowledgeDomainID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrKnowledgeDomainAdminNotFound
		}
		return err
	}

	// Audit: domain admin revoked
	s.businessAudit.RecordDomainAdminRevoked(ctx, knowledgeDomainID, "",
		userID, "", "", true)

	return nil
}

func (s *knowledgeDomainAdminService) IsAdmin(ctx context.Context, userID string, knowledgeDomainID uint64) (bool, error) {
	admin, err := s.repo.Get(ctx, userID, knowledgeDomainID)
	return admin != nil, err
}

func (s *knowledgeDomainAdminService) ListDomainIDs(ctx context.Context, userID string) ([]uint64, error) {
	return s.repo.ListDomainIDsByUser(ctx, userID)
}

func (s *knowledgeDomainAdminService) ListPage(
	ctx context.Context,
	knowledgeDomainID uint64,
	search string,
	page, pageSize int,
) ([]*types.KnowledgeDomainAdmin, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListByDomain(ctx, knowledgeDomainID, search, (page-1)*pageSize, pageSize)
}
