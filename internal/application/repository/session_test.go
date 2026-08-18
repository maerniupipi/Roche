package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

func newSessionRepositoryForTest(t *testing.T) (interfaces.SessionRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Session{}))
	return NewSessionRepository(db), db
}

func createSessionForTest(t *testing.T, db *gorm.DB, userID string) *types.Session {
	t.Helper()
	session := &types.Session{UserID: userID, Title: userID + " session"}
	require.NoError(t, db.Create(session).Error)
	return session
}

func countActiveSessionsForTest(t *testing.T, db *gorm.DB, id string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&types.Session{}).Where("id = ?", id).Count(&count).Error)
	return count
}

func sessionIDsForTest(sessions []*types.Session) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}

func TestSessionRepositoryGetAndListHonorUserScope(t *testing.T) {
	repo, db := newSessionRepositoryForTest(t)
	ctx := context.Background()
	aliceSession := createSessionForTest(t, db, "alice")
	bobSession := createSessionForTest(t, db, "bob")

	_, err := repo.Get(ctx, "bob", aliceSession.ID)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)

	got, err := repo.Get(ctx, "bob", bobSession.ID)
	require.NoError(t, err)
	require.Equal(t, bobSession.ID, got.ID)

	sessions, err := repo.GetByUserID(ctx, "bob")
	require.NoError(t, err)
	require.Equal(t, []string{bobSession.ID}, sessionIDsForTest(sessions))

	paged, total, err := repo.QueryPaged(ctx, &types.SessionListQuery{
		UserID: "bob", Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, paged, 1)
	require.Equal(t, bobSession.ID, paged[0].ID)
}

func TestSessionRepositoryUpdateHonorsUserScope(t *testing.T) {
	repo, db := newSessionRepositoryForTest(t)
	ctx := context.Background()
	aliceSession := createSessionForTest(t, db, "alice")

	rows, err := repo.Update(ctx, &types.Session{
		ID: aliceSession.ID, Title: "bob update attempt",
	}, "bob")
	require.NoError(t, err)
	require.Zero(t, rows)

	rows, err = repo.Update(ctx, &types.Session{
		ID: aliceSession.ID, Title: "alice updated session",
	}, "alice")
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var changed types.Session
	require.NoError(t, db.First(&changed, "id = ?", aliceSession.ID).Error)
	require.Equal(t, "alice updated session", changed.Title)
}

func TestSessionRepositoryDeleteOperationsHonorUserScope(t *testing.T) {
	repo, db := newSessionRepositoryForTest(t)
	ctx := context.Background()
	aliceSession := createSessionForTest(t, db, "alice")
	bobSession1 := createSessionForTest(t, db, "bob")
	bobSession2 := createSessionForTest(t, db, "bob")

	rows, err := repo.Delete(ctx, "bob", aliceSession.ID)
	require.NoError(t, err)
	require.Zero(t, rows)

	rows, err = repo.BatchDelete(ctx, "bob", []string{aliceSession.ID, bobSession1.ID})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	require.EqualValues(t, 1, countActiveSessionsForTest(t, db, aliceSession.ID))
	require.Zero(t, countActiveSessionsForTest(t, db, bobSession1.ID))

	rows, err = repo.DeleteAllByUserID(ctx, "bob")
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	require.Zero(t, countActiveSessionsForTest(t, db, bobSession2.ID))
	require.EqualValues(t, 1, countActiveSessionsForTest(t, db, aliceSession.ID))
}
