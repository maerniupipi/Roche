package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

func newUnifiedQATestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.QAExecutionRun{},
	))
	return db
}

func TestUnifiedQARunRepositoryRoundTripsRunAndFinish(t *testing.T) {
	t.Parallel()

	db := newUnifiedQATestDB(t)
	repo := NewUnifiedQARunRepository(db)
	ctx := context.Background()
	startedAt := time.Now().UTC().Truncate(time.Millisecond)

	run := &types.QAExecutionRun{
		ID:               uuid.NewString(),
		SessionID:        uuid.NewString(),
		UserID:           uuid.NewString(),
		RouteType:        types.QARouteTypeMultiAgent,
		SelectedAgentIDs: types.JSONStringArray{"finance", "compliance"},
		Status:           types.QARunStatusRunning,
		OriginalQuery:    "差旅期间招待客户如何报销？",
		ConfigSnapshot: types.JSONMap{
			"catalog_version": "catalog-v1",
		},
		Metrics:         types.JSONMap{},
		LangfuseTraceID: "trace-123",
		StartedAt:       startedAt,
	}
	require.NoError(t, repo.CreateRun(ctx, run))

	stored, err := repo.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"finance", "compliance"}, []string(stored.SelectedAgentIDs))
	require.Equal(t, "catalog-v1", stored.ConfigSnapshot["catalog_version"])
	require.Equal(t, "trace-123", stored.LangfuseTraceID)

	completedAt := startedAt.Add(1250 * time.Millisecond)
	require.NoError(t, repo.FinishRun(ctx, run.ID, types.QARunFinishUpdate{
		Status:           types.QARunStatusCompleted,
		RewrittenQuery:   "差旅客户招待费用的报销与合规要求",
		RouteType:        types.QARouteTypeMultiAgent,
		SelectedAgentIDs: types.JSONStringArray{"finance", "compliance"},
		Metrics:          types.JSONMap{"generative_calls": float64(4)},
		CompletedAt:      completedAt,
		DurationMS:       1250,
	}))

	finished, err := repo.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, types.QARunStatusCompleted, finished.Status)
	require.Equal(t, int64(1250), finished.DurationMS)
	require.Equal(t, "catalog-v1", finished.ConfigSnapshot["catalog_version"], "finish must not overwrite the immutable snapshot")
}
