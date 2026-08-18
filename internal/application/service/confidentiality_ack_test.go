package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/types"
)

// newConfidentialityAckTestService wires up a userService backed by an
// in-memory SQLite database so we can exercise the confidentiality-ack
// flow end-to-end (repo + service + DB) without spinning up Postgres or
// the HTTP server.
func newConfidentialityAckTestService(t *testing.T) (*userService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	// AutoMigrate builds the users table from the User struct, which now
	// includes ConfidentialityAcknowledgedAt. This implicitly verifies
	// the new field maps to a real column the same way the SQL migration
	// would after deploy.
	require.NoError(t, db.AutoMigrate(&types.User{}))

	userRepo := repository.NewUserRepository(db)
	svc := &userService{
		userRepo: userRepo,
	}
	return svc, db
}

// TestConfidentialityAck_FullLifecycle walks the complete happy path:
// fresh user → query (not acknowledged) → acknowledge → query (acknowledged)
// → acknowledge again (idempotent, timestamp refreshes but never clears).
func TestConfidentialityAck_FullLifecycle(t *testing.T) {
	svc, db := newConfidentialityAckTestService(t)

	// Seed a user that has never acknowledged.
	user := &types.User{
		ID:           "u-ack-1",
		Username:     "ack-tester",
		Email:        "ack@test.local",
		PasswordHash: "x",
	}
	require.NoError(t, db.Create(user).Error)

	ctx := context.Background()

	// 1. Fresh user → not acknowledged, timestamp is nil.
	acknowledged, at, err := svc.GetConfidentialityAck(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, acknowledged)
	assert.Nil(t, at)

	// 2. Acknowledge → returns a timestamp.
	firstAt, err := svc.AcknowledgeConfidentiality(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, firstAt)
	assert.WithinDuration(t, time.Now(), *firstAt, 5*time.Second)

	// 3. Re-query → now acknowledged, timestamp matches.
	acknowledged, at, err = svc.GetConfidentialityAck(ctx, user.ID)
	require.NoError(t, err)
	assert.True(t, acknowledged)
	require.NotNil(t, at)
	assert.Equal(t, firstAt.UTC(), at.UTC())

	// 4. Acknowledge again → idempotent: timestamp refreshes, never clears.
	time.Sleep(20 * time.Millisecond)
	secondAt, err := svc.AcknowledgeConfidentiality(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, secondAt)
	assert.True(t, secondAt.After(*firstAt), "second ack should refresh timestamp")

	// 5. Final DB-level check: the column is non-null and matches.
	var fresh types.User
	require.NoError(t, db.First(&fresh, "id = ?", user.ID).Error)
	require.NotNil(t, fresh.ConfidentialityAcknowledgedAt)
	assert.Equal(t, secondAt.UTC(), fresh.ConfidentialityAcknowledgedAt.UTC())
}

// TestConfidentialityAck_PerUserIsolation confirms one user's acknowledgement
// does not leak into another user's state.
func TestConfidentialityAck_PerUserIsolation(t *testing.T) {
	svc, db := newConfidentialityAckTestService(t)

	alice := &types.User{ID: "u-alice", Username: "alice", Email: "alice@test.local", PasswordHash: "x"}
	bob := &types.User{ID: "u-bob", Username: "bob", Email: "bob@test.local", PasswordHash: "x"}
	require.NoError(t, db.Create(alice).Error)
	require.NoError(t, db.Create(bob).Error)

	ctx := context.Background()

	_, err := svc.AcknowledgeConfidentiality(ctx, alice.ID)
	require.NoError(t, err)

	bobAck, bobAt, err := svc.GetConfidentialityAck(ctx, bob.ID)
	require.NoError(t, err)
	assert.False(t, bobAck, "bob must remain unacknowledged")
	assert.Nil(t, bobAt)

	aliceAck, aliceAt, err := svc.GetConfidentialityAck(ctx, alice.ID)
	require.NoError(t, err)
	assert.True(t, aliceAck, "alice must be acknowledged")
	require.NotNil(t, aliceAt)
}

// TestConfidentialityAck_UnknownUserErrors makes sure a non-existent userID
// surfaces an error rather than silently reporting "not acknowledged".
func TestConfidentialityAck_UnknownUserErrors(t *testing.T) {
	svc, _ := newConfidentialityAckTestService(t)
	ctx := context.Background()

	_, _, err := svc.GetConfidentialityAck(ctx, "does-not-exist")
	require.Error(t, err)

	_, err = svc.AcknowledgeConfidentiality(ctx, "does-not-exist")
	require.Error(t, err)
}
