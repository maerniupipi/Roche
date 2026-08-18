package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

var (
	ErrWorkdayDisabled        = errors.New("Workday integration is disabled")
	ErrInvalidSyncMode        = errors.New("invalid integration sync mode")
	ErrIntegrationRunNotFound = errors.New("integration sync run not found")
)

type enterpriseIntegrationService struct {
	cfg      *config.Config
	repo     interfaces.EnterpriseIntegrationRepository
	provider interfaces.WorkdayProvider
	enqueuer interfaces.TaskEnqueuer
}

func NewEnterpriseIntegrationService(
	cfg *config.Config,
	repo interfaces.EnterpriseIntegrationRepository,
	provider interfaces.WorkdayProvider,
	enqueuer interfaces.TaskEnqueuer,
) interfaces.EnterpriseIntegrationService {
	return &enterpriseIntegrationService{
		cfg:      cfg,
		repo:     repo,
		provider: provider,
		enqueuer: enqueuer,
	}
}

func (s *enterpriseIntegrationService) TriggerWorkdaySync(
	ctx context.Context,
	mode types.IntegrationSyncMode,
	traceID string,
) (*types.IntegrationSyncRun, error) {
	if !s.enabled() {
		return nil, ErrWorkdayDisabled
	}
	if !mode.IsValid() {
		return nil, ErrInvalidSyncMode
	}

	cursor := types.JSON([]byte(`{}`))
	if mode == types.IntegrationSyncModeIncremental {
		latest, err := s.repo.LatestSuccessfulCursor(
			ctx,
			types.EnterpriseProviderWorkday,
			s.cfg.Workday.ConnectionKey,
		)
		if err != nil {
			return nil, fmt.Errorf("load Workday sync cursor: %w", err)
		}
		if len(latest) > 0 {
			cursor = latest
		}
	}
	run := &types.IntegrationSyncRun{
		ID:            uuid.NewString(),
		Provider:      types.EnterpriseProviderWorkday,
		ConnectionKey: s.cfg.Workday.ConnectionKey,
		Mode:          mode,
		CursorBefore:  cursor,
		CursorAfter:   cursor,
		Status:        types.IntegrationSyncPending,
		Counters:      types.JSON([]byte(`{}`)),
		TraceID:       strings.TrimSpace(traceID),
		CreatedAt:     time.Now(),
	}
	if err := s.repo.CreateSyncRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create Workday sync run: %w", err)
	}
	payload, err := json.Marshal(types.WorkdaySyncPayload{
		TracingContext: types.TracingContext{LangfuseTraceID: run.TraceID},
		RunID:          run.ID,
	})
	if err != nil {
		return nil, err
	}
	task := asynq.NewTask(types.TypeWorkdaySync, payload)
	if _, err := s.enqueuer.Enqueue(
		task,
		asynq.Queue(types.QueueLow),
		asynq.MaxRetry(3),
	); err != nil {
		_ = s.repo.FinishSyncRun(
			ctx,
			run.ID,
			types.IntegrationSyncFailed,
			types.WorkdaySyncCounters{},
			"enqueue_failed",
			err.Error(),
		)
		return nil, fmt.Errorf("enqueue Workday sync: %w", err)
	}
	return run, nil
}

func (s *enterpriseIntegrationService) ProcessWorkdaySync(
	ctx context.Context,
	task *asynq.Task,
) (processErr error) {
	var payload types.WorkdaySyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode Workday sync payload: %w", err)
	}
	run, err := s.repo.GetSyncRun(ctx, payload.RunID)
	if err != nil {
		return err
	}
	if run == nil {
		return ErrIntegrationRunNotFound
	}
	if run.Status == types.IntegrationSyncSucceeded {
		return nil
	}
	if err := s.repo.MarkSyncRunRunning(ctx, run.ID); err != nil {
		return err
	}

	counters := types.WorkdaySyncCounters{}
	defer func() {
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if processErr == nil {
			processErr = s.repo.FinishSyncRun(
				finishCtx,
				run.ID,
				types.IntegrationSyncSucceeded,
				counters,
				"",
				"",
			)
			return
		}
		_ = s.repo.FinishSyncRun(
			finishCtx,
			run.ID,
			types.IntegrationSyncFailed,
			counters,
			"sync_failed",
			processErr.Error(),
		)
	}()

	cursorBefore := decodeIntegrationCursor(run.CursorBefore)
	if err := s.syncOrgUnits(ctx, run.ID, cursorBefore["org_units"], &counters); err != nil {
		return fmt.Errorf("sync Workday organizations: %w", err)
	}
	if err := s.syncWorkers(ctx, run.ID, cursorBefore["workers"], &counters); err != nil {
		return fmt.Errorf("sync Workday workers: %w", err)
	}
	return nil
}

func (s *enterpriseIntegrationService) syncOrgUnits(
	ctx context.Context,
	runID, initialCursor string,
	counters *types.WorkdaySyncCounters,
) error {
	requestCursor := initialCursor
	checkpoint := initialCursor
	for {
		page, err := s.provider.FetchOrgUnits(ctx, requestCursor, s.cfg.Workday.PageSize)
		if err != nil {
			return err
		}
		if page.Cursor != "" {
			checkpoint = page.Cursor
		}
		delta, err := s.repo.ApplyOrgUnitPage(
			ctx,
			runID,
			types.EnterpriseProviderWorkday,
			page.Items,
			checkpoint,
		)
		if err != nil {
			return err
		}
		addSyncCounters(counters, delta)
		if page.NextCursor == "" {
			return nil
		}
		if page.NextCursor == requestCursor {
			return errors.New("Workday organization cursor did not advance")
		}
		requestCursor = page.NextCursor
	}
}

func (s *enterpriseIntegrationService) syncWorkers(
	ctx context.Context,
	runID, initialCursor string,
	counters *types.WorkdaySyncCounters,
) error {
	requestCursor := initialCursor
	checkpoint := initialCursor
	for {
		page, err := s.provider.FetchWorkers(ctx, requestCursor, s.cfg.Workday.PageSize)
		if err != nil {
			return err
		}
		if page.Cursor != "" {
			checkpoint = page.Cursor
		}
		delta, err := s.repo.ApplyWorkerPage(
			ctx,
			runID,
			types.EnterpriseProviderWorkday,
			page.Items,
			checkpoint,
		)
		if err != nil {
			return err
		}
		addSyncCounters(counters, delta)
		if page.NextCursor == "" {
			return nil
		}
		if page.NextCursor == requestCursor {
			return errors.New("Workday worker cursor did not advance")
		}
		requestCursor = page.NextCursor
	}
}

func (s *enterpriseIntegrationService) ListWorkdaySyncRuns(
	ctx context.Context,
	offset, limit int,
) ([]*types.IntegrationSyncRun, int64, error) {
	return s.repo.ListSyncRuns(
		ctx,
		types.EnterpriseProviderWorkday,
		offset,
		limit,
	)
}

func (s *enterpriseIntegrationService) GetWorkdaySyncRun(
	ctx context.Context,
	runID string,
) (*types.IntegrationSyncRun, error) {
	run, err := s.repo.GetSyncRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	if run == nil || run.Provider != types.EnterpriseProviderWorkday {
		return nil, ErrIntegrationRunNotFound
	}
	return run, nil
}

func (s *enterpriseIntegrationService) ListWorkdayOrgUnits(
	ctx context.Context,
) ([]*types.ExternalOrgUnit, error) {
	return s.repo.ListExternalOrgUnits(ctx, types.EnterpriseProviderWorkday)
}

func (s *enterpriseIntegrationService) ListWorkdayWorkers(
	ctx context.Context,
	orgExternalID, search string,
	offset, limit int,
) ([]*types.ExternalWorker, int64, error) {
	return s.repo.ListExternalWorkers(
		ctx,
		types.EnterpriseProviderWorkday,
		orgExternalID,
		search,
		offset,
		limit,
	)
}

func (s *enterpriseIntegrationService) AcceptWorkdayEvent(
	ctx context.Context,
	externalEventID, eventType string,
	payload []byte,
	traceID string,
) (bool, *types.IntegrationSyncRun, error) {
	if !s.enabled() {
		return false, nil, ErrWorkdayDisabled
	}
	externalEventID = strings.TrimSpace(externalEventID)
	eventType = strings.TrimSpace(eventType)
	if externalEventID == "" || eventType == "" {
		return false, nil, errors.New("external_event_id and event_type are required")
	}
	hash := sha256.Sum256(payload)
	event := &types.IntegrationEvent{
		Provider:        types.EnterpriseProviderWorkday,
		ExternalEventID: externalEventID,
		EventType:       eventType,
		PayloadHash:     hex.EncodeToString(hash[:]),
		Status:          types.IntegrationEventReceived,
		TraceID:         strings.TrimSpace(traceID),
		ReceivedAt:      time.Now(),
	}
	created, err := s.repo.CreateEventIfNew(ctx, event)
	if err != nil || !created {
		return created, nil, err
	}
	if err := s.repo.MarkEvent(ctx, event.ID, types.IntegrationEventProcessing, ""); err != nil {
		return true, nil, err
	}
	run, err := s.TriggerWorkdaySync(ctx, types.IntegrationSyncModeIncremental, traceID)
	if err != nil {
		_ = s.repo.MarkEvent(ctx, event.ID, types.IntegrationEventFailed, err.Error())
		return true, nil, err
	}
	if err := s.repo.MarkEvent(ctx, event.ID, types.IntegrationEventProcessed, ""); err != nil {
		return true, run, err
	}
	return true, run, nil
}

func (s *enterpriseIntegrationService) enabled() bool {
	return s != nil &&
		s.cfg != nil &&
		s.cfg.Workday != nil &&
		s.cfg.Workday.Enable &&
		s.provider != nil &&
		s.provider.Name() == types.EnterpriseProviderWorkday
}

func decodeIntegrationCursor(raw types.JSON) map[string]string {
	result := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &result)
	}
	return result
}

func addSyncCounters(total, delta *types.WorkdaySyncCounters) {
	if total == nil || delta == nil {
		return
	}
	total.OrgUnitsSeen += delta.OrgUnitsSeen
	total.OrgUnitsChanged += delta.OrgUnitsChanged
	total.WorkersSeen += delta.WorkersSeen
	total.WorkersChanged += delta.WorkersChanged
	total.WorkersLinked += delta.WorkersLinked
	total.MembershipsChanged += delta.MembershipsChanged
	total.UnmatchedWorkers += delta.UnmatchedWorkers
}
