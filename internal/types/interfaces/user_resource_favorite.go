package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// UserResourceFavoriteRepository stores global per-user favorites.
type UserResourceFavoriteRepository interface {
	List(ctx context.Context, userID string, resourceType string) ([]*types.UserResourceFavorite, error)
	Add(ctx context.Context, userID string, resourceType, resourceID string) (created bool, err error)
	Remove(ctx context.Context, userID string, resourceType, resourceID string) (removed bool, err error)
	IsFavorite(ctx context.Context, userID string, resourceType, resourceID string) (bool, error)
}

type UserResourceFavoriteService interface {
	List(ctx context.Context, userID string, resourceType string) ([]*types.UserResourceFavorite, error)
	Add(ctx context.Context, userID string, resourceType, resourceID string) error
	Remove(ctx context.Context, userID string, resourceType, resourceID string) error
}
