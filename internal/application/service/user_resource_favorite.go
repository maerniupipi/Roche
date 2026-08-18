package service

import (
	"context"
	"errors"
	"strings"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var (
	ErrFavoriteInvalidType = errors.New("invalid favorite resource type")
	ErrFavoriteEmptyID     = errors.New("favorite resource id is required")
)

type userResourceFavoriteService struct {
	repo interfaces.UserResourceFavoriteRepository
}

func NewUserResourceFavoriteService(repo interfaces.UserResourceFavoriteRepository) interfaces.UserResourceFavoriteService {
	return &userResourceFavoriteService{repo: repo}
}

func (s *userResourceFavoriteService) List(
	ctx context.Context, userID string, resourceType string,
) ([]*types.UserResourceFavorite, error) {
	if !types.IsValidFavoriteResourceType(resourceType) {
		return nil, ErrFavoriteInvalidType
	}
	return s.repo.List(ctx, userID, resourceType)
}

func (s *userResourceFavoriteService) Add(
	ctx context.Context, userID string, resourceType, resourceID string,
) error {
	if !types.IsValidFavoriteResourceType(resourceType) {
		return ErrFavoriteInvalidType
	}
	if strings.TrimSpace(resourceID) == "" {
		return ErrFavoriteEmptyID
	}
	_, err := s.repo.Add(ctx, userID, resourceType, resourceID)
	return err
}

func (s *userResourceFavoriteService) Remove(
	ctx context.Context, userID string, resourceType, resourceID string,
) error {
	if !types.IsValidFavoriteResourceType(resourceType) {
		return ErrFavoriteInvalidType
	}
	if strings.TrimSpace(resourceID) == "" {
		return ErrFavoriteEmptyID
	}
	_, err := s.repo.Remove(ctx, userID, resourceType, resourceID)
	return err
}
