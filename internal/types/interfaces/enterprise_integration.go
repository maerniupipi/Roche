package interfaces

import (
	"context"

	"github.com/hibiken/asynq"
	"roche.local/knowledge-agent-platform/internal/types"
)

type WorkdayProvider interface {
	Name() string
	FetchOrgUnits(ctx context.Context, cursor string, pageSize int) (*types.WorkdayOrgUnitPage, error)
	FetchWorkers(ctx context.Context, cursor string, pageSize int) (*types.WorkdayWorkerPage, error)
}

type EnterpriseIntegrationRepository interface {
	CreateSyncRun(ctx context.Context, run *types.IntegrationSyncRun) error
	GetSyncRun(ctx context.Context, runID string) (*types.IntegrationSyncRun, error)
	ListSyncRuns(ctx context.Context, provider string, offset, limit int) ([]*types.IntegrationSyncRun, int64, error)
	ListExternalOrgUnits(ctx context.Context, provider string) ([]*types.ExternalOrgUnit, error)
	ListExternalWorkers(
		ctx context.Context,
		provider, orgExternalID, search string,
		offset, limit int,
	) ([]*types.ExternalWorker, int64, error)
	LatestSuccessfulCursor(ctx context.Context, provider, connectionKey string) (types.JSON, error)
	MarkSyncRunRunning(ctx context.Context, runID string) error
	ApplyOrgUnitPage(
		ctx context.Context,
		runID, provider string,
		items []types.WorkdayOrgUnitRecord,
		nextCursor string,
	) (*types.WorkdaySyncCounters, error)
	ApplyWorkerPage(
		ctx context.Context,
		runID, provider string,
		items []types.WorkdayWorkerRecord,
		nextCursor string,
	) (*types.WorkdaySyncCounters, error)
	FinishSyncRun(
		ctx context.Context,
		runID string,
		status types.IntegrationSyncStatus,
		counters types.WorkdaySyncCounters,
		errorCode, errorSummary string,
	) error
	CreateEventIfNew(ctx context.Context, event *types.IntegrationEvent) (bool, error)
	MarkEvent(ctx context.Context, id uint64, status types.IntegrationEventStatus, errorSummary string) error
}

type EnterpriseIntegrationService interface {
	TriggerWorkdaySync(
		ctx context.Context,
		mode types.IntegrationSyncMode,
		traceID string,
	) (*types.IntegrationSyncRun, error)
	ProcessWorkdaySync(ctx context.Context, task *asynq.Task) error
	GetWorkdaySyncRun(ctx context.Context, runID string) (*types.IntegrationSyncRun, error)
	ListWorkdaySyncRuns(ctx context.Context, offset, limit int) ([]*types.IntegrationSyncRun, int64, error)
	ListWorkdayOrgUnits(ctx context.Context) ([]*types.ExternalOrgUnit, error)
	ListWorkdayWorkers(
		ctx context.Context,
		orgExternalID, search string,
		offset, limit int,
	) ([]*types.ExternalWorker, int64, error)
	AcceptWorkdayEvent(
		ctx context.Context,
		externalEventID, eventType string,
		payload []byte,
		traceID string,
	) (bool, *types.IntegrationSyncRun, error)
}
