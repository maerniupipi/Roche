package types

import (
	"time"

	"github.com/google/uuid"
)

const EnterpriseProviderWorkday = "workday"

type IntegrationSyncMode string

const (
	IntegrationSyncModeFull        IntegrationSyncMode = "full"
	IntegrationSyncModeIncremental IntegrationSyncMode = "incremental"
)

func (m IntegrationSyncMode) IsValid() bool {
	return m == IntegrationSyncModeFull || m == IntegrationSyncModeIncremental
}

type IntegrationSyncStatus string

const (
	IntegrationSyncPending   IntegrationSyncStatus = "pending"
	IntegrationSyncRunning   IntegrationSyncStatus = "running"
	IntegrationSyncSucceeded IntegrationSyncStatus = "succeeded"
	IntegrationSyncFailed    IntegrationSyncStatus = "failed"
)

type ExternalWorkerStatus string

const (
	ExternalWorkerActive   ExternalWorkerStatus = "active"
	ExternalWorkerInactive ExternalWorkerStatus = "inactive"
	ExternalWorkerLeave    ExternalWorkerStatus = "leave"
)

func (s ExternalWorkerStatus) IsActive() bool { return s == ExternalWorkerActive }

type ExternalOrgUnit struct {
	ID                  string        `json:"id" gorm:"type:varchar(36);primaryKey"`
	Provider            string        `json:"provider" gorm:"type:varchar(32);not null"`
	ExternalOrgID       string        `json:"external_org_id" gorm:"type:varchar(255);not null"`
	ParentExternalOrgID *string       `json:"parent_external_org_id,omitempty" gorm:"type:varchar(255)"`
	OrgUnitID           *string       `json:"org_unit_id,omitempty" gorm:"type:varchar(36)"`
	Name                string        `json:"name" gorm:"type:varchar(255);not null"`
	OrgType             string        `json:"org_type,omitempty" gorm:"type:varchar(64)"`
	Status              OrgUnitStatus `json:"status" gorm:"type:varchar(20);not null"`
	Attributes          JSON          `json:"attributes" gorm:"type:jsonb;not null;default:'{}'"`
	Checksum            string        `json:"checksum" gorm:"type:varchar(64);not null"`
	EffectiveFrom       *time.Time    `json:"effective_from,omitempty"`
	EffectiveTo         *time.Time    `json:"effective_to,omitempty"`
	LastSeenAt          time.Time     `json:"last_seen_at"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

func (ExternalOrgUnit) TableName() string { return "external_org_units" }

func (o *ExternalOrgUnit) EnsureID() {
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
}

type ExternalWorker struct {
	ID                      string               `json:"id" gorm:"type:varchar(36);primaryKey"`
	Provider                string               `json:"provider" gorm:"type:varchar(32);not null"`
	ExternalWorkerID        string               `json:"external_worker_id" gorm:"type:varchar(255);not null"`
	UserID                  *string              `json:"user_id,omitempty" gorm:"type:varchar(36)"`
	PrimaryOrgExternalID    *string              `json:"primary_org_external_id,omitempty" gorm:"type:varchar(255)"`
	ManagerExternalWorkerID *string              `json:"manager_external_worker_id,omitempty" gorm:"type:varchar(255)"`
	CorporateEmail          string               `json:"corporate_email,omitempty" gorm:"type:varchar(255)"`
	WorkerStatus            ExternalWorkerStatus `json:"worker_status" gorm:"type:varchar(20);not null"`
	Attributes              JSON                 `json:"attributes" gorm:"type:jsonb;not null;default:'{}'"`
	Checksum                string               `json:"checksum" gorm:"type:varchar(64);not null"`
	EffectiveFrom           *time.Time           `json:"effective_from,omitempty"`
	EffectiveTo             *time.Time           `json:"effective_to,omitempty"`
	LastSeenAt              time.Time            `json:"last_seen_at"`
	CreatedAt               time.Time            `json:"created_at"`
	UpdatedAt               time.Time            `json:"updated_at"`
}

func (ExternalWorker) TableName() string { return "external_workers" }

func (w *ExternalWorker) EnsureID() {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
}

type IntegrationSyncRun struct {
	ID            string                `json:"id" gorm:"type:varchar(36);primaryKey"`
	Provider      string                `json:"provider" gorm:"type:varchar(32);not null"`
	ConnectionKey string                `json:"connection_key" gorm:"type:varchar(128);not null"`
	Mode          IntegrationSyncMode   `json:"mode" gorm:"type:varchar(20);not null"`
	CursorBefore  JSON                  `json:"cursor_before" gorm:"type:jsonb;not null;default:'{}'"`
	CursorAfter   JSON                  `json:"cursor_after" gorm:"type:jsonb;not null;default:'{}'"`
	Status        IntegrationSyncStatus `json:"status" gorm:"type:varchar(20);not null"`
	Counters      JSON                  `json:"counters" gorm:"type:jsonb;not null;default:'{}'"`
	TraceID       string                `json:"trace_id,omitempty" gorm:"type:varchar(128)"`
	ErrorCode     string                `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorSummary  string                `json:"error_summary,omitempty" gorm:"type:text"`
	StartedAt     *time.Time            `json:"started_at,omitempty"`
	FinishedAt    *time.Time            `json:"finished_at,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
}

func (IntegrationSyncRun) TableName() string { return "integration_sync_runs" }

type IntegrationEventStatus string

const (
	IntegrationEventReceived   IntegrationEventStatus = "received"
	IntegrationEventProcessing IntegrationEventStatus = "processing"
	IntegrationEventProcessed  IntegrationEventStatus = "processed"
	IntegrationEventFailed     IntegrationEventStatus = "failed"
)

type IntegrationEvent struct {
	ID              uint64                 `json:"id" gorm:"primaryKey;autoIncrement"`
	Provider        string                 `json:"provider" gorm:"type:varchar(32);not null"`
	ExternalEventID string                 `json:"external_event_id" gorm:"type:varchar(255);not null"`
	EventType       string                 `json:"event_type" gorm:"type:varchar(128);not null"`
	PayloadHash     string                 `json:"payload_hash" gorm:"type:varchar(64);not null"`
	Status          IntegrationEventStatus `json:"status" gorm:"type:varchar(20);not null"`
	AttemptCount    int                    `json:"attempt_count"`
	TraceID         string                 `json:"trace_id,omitempty" gorm:"type:varchar(128)"`
	ReceivedAt      time.Time              `json:"received_at"`
	ProcessedAt     *time.Time             `json:"processed_at,omitempty"`
	ErrorSummary    string                 `json:"error_summary,omitempty" gorm:"type:text"`
}

func (IntegrationEvent) TableName() string { return "integration_events" }

// WorkdayOrgUnitRecord is the stable provider-neutral contract consumed by
// synchronization. An adapter may obtain it from Workday directly, MuleSoft,
// a fixture file, or another enterprise integration layer.
type WorkdayOrgUnitRecord struct {
	ExternalID       string         `json:"external_id"`
	ParentExternalID string         `json:"parent_external_id,omitempty"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	OrgType          string         `json:"org_type,omitempty"`
	Status           OrgUnitStatus  `json:"status"`
	Attributes       map[string]any `json:"attributes,omitempty"`
	EffectiveFrom    *time.Time     `json:"effective_from,omitempty"`
	EffectiveTo      *time.Time     `json:"effective_to,omitempty"`
}

type WorkdayWorkerRecord struct {
	ExternalID              string               `json:"external_id"`
	PrimaryOrgExternalID    string               `json:"primary_org_external_id,omitempty"`
	ManagerExternalWorkerID string               `json:"manager_external_worker_id,omitempty"`
	CorporateEmail          string               `json:"corporate_email,omitempty"`
	Status                  ExternalWorkerStatus `json:"status"`
	Attributes              map[string]any       `json:"attributes,omitempty"`
	EffectiveFrom           *time.Time           `json:"effective_from,omitempty"`
	EffectiveTo             *time.Time           `json:"effective_to,omitempty"`
}

type WorkdayOrgUnitPage struct {
	Items      []WorkdayOrgUnitRecord `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	Cursor     string                 `json:"cursor,omitempty"`
}

type WorkdayWorkerPage struct {
	Items      []WorkdayWorkerRecord `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
	Cursor     string                `json:"cursor,omitempty"`
}

type WorkdaySyncCounters struct {
	OrgUnitsSeen       int `json:"org_units_seen"`
	OrgUnitsChanged    int `json:"org_units_changed"`
	WorkersSeen        int `json:"workers_seen"`
	WorkersChanged     int `json:"workers_changed"`
	WorkersLinked      int `json:"workers_linked"`
	MembershipsChanged int `json:"memberships_changed"`
	UnmatchedWorkers   int `json:"unmatched_workers"`
}

type WorkdaySyncPayload struct {
	TracingContext
	RunID string `json:"run_id"`
}
