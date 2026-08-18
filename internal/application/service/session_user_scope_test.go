package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/application/repository"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
)

func testSessionScopeContext(knowledgeDomainID uint64, userID string) context.Context {
	ctx := context.Background()
	if userID != "" {
		ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
	}
	return ctx
}

func newTestSessionService(t *testing.T) (*sessionService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Session{}))

	return &sessionService{
		sessionRepo: repository.NewSessionRepository(db),
	}, db
}

func TestGetSessionIsScopedToCurrentUser(t *testing.T) {
	svc, db := newTestSessionService(t)
	aliceSession := &types.Session{
		UserID: "alice",
		Title:  "alice private session",
	}
	require.NoError(t, db.Create(aliceSession).Error)
	bobSession := &types.Session{
		UserID: "bob",
		Title:  "bob private session",
	}
	require.NoError(t, db.Create(bobSession).Error)

	_, err := svc.GetSession(testSessionScopeContext(1, "bob"), aliceSession.ID)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)

	got, err := svc.GetSession(testSessionScopeContext(1, "bob"), bobSession.ID)
	require.NoError(t, err)
	require.Equal(t, bobSession.ID, got.ID)

}

func TestUpdateSessionIsScopedToCurrentUserAndAllowsNoOp(t *testing.T) {
	svc, db := newTestSessionService(t)
	aliceSession := &types.Session{
		UserID:      "alice",
		Title:       "alice private session",
		Description: "original description",
	}
	require.NoError(t, db.Create(aliceSession).Error)

	err := svc.UpdateSession(testSessionScopeContext(1, "bob"), &types.Session{
		ID:          aliceSession.ID,
		Title:       "bob update attempt",
		Description: "should not be saved",
	})
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)

	var unchanged types.Session
	require.NoError(t, db.First(&unchanged, "id = ?", aliceSession.ID).Error)
	require.Equal(t, aliceSession.Title, unchanged.Title)
	require.Equal(t, aliceSession.Description, unchanged.Description)

	err = svc.UpdateSession(testSessionScopeContext(1, "alice"), &types.Session{
		ID:          aliceSession.ID,
		Title:       aliceSession.Title,
		Description: aliceSession.Description,
	})
	require.NoError(t, err)
}
