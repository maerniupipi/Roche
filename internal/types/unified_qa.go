package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSONStringArray is a JSON-backed string slice that works with PostgreSQL
// JSONB and SQLite-backed repository tests.
type JSONStringArray []string

func (a JSONStringArray) Value() (driver.Value, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(a)
}

func (a *JSONStringArray) Scan(src any) error {
	if src == nil {
		*a = JSONStringArray{}
		return nil
	}

	var raw []byte
	switch value := src.(type) {
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return errors.New("JSONStringArray.Scan: unsupported source type")
	}
	if len(raw) == 0 {
		*a = JSONStringArray{}
		return nil
	}
	return json.Unmarshal(raw, a)
}

type QARunStatus string

const (
	QARunStatusRunning      QARunStatus = "running"
	QARunStatusCompleted    QARunStatus = "completed"
	QARunStatusPartial      QARunStatus = "partial"
	QARunStatusInsufficient QARunStatus = "insufficient"
	QARunStatusFailed       QARunStatus = "failed"
	QARunStatusCancelled    QARunStatus = "cancelled"
)

type QANodeStatus string

const (
	QANodeStatusRunning   QANodeStatus = "running"
	QANodeStatusCompleted QANodeStatus = "completed"
	QANodeStatusFailed    QANodeStatus = "failed"
	QANodeStatusDegraded  QANodeStatus = "degraded"
	QANodeStatusCancelled QANodeStatus = "cancelled"
)

type QARouteType string

const (
	QARouteTypeSingleAgent QARouteType = "single_agent"
	QARouteTypeMultiAgent  QARouteType = "multi_agent"
)

type QAExecutionRun struct {
	ID                 string          `json:"id" gorm:"type:varchar(36);primaryKey"`
	SessionID          string          `json:"session_id" gorm:"type:varchar(36);not null;index"`
	RequestID          string          `json:"request_id" gorm:"type:varchar(64);not null;default:'';index"`
	UserMessageID      string          `json:"user_message_id" gorm:"type:varchar(36);not null;default:''"`
	AssistantMessageID string          `json:"assistant_message_id" gorm:"type:varchar(36);not null;default:''"`
	UserID             string          `json:"user_id" gorm:"type:varchar(36);not null;index:idx_qa_execution_runs_user_started,priority:1"`
	EntryAgentID       string          `json:"entry_agent_id" gorm:"type:varchar(36);not null;default:''"`
	RouteType          QARouteType     `json:"route_type" gorm:"type:varchar(24);not null;default:''"`
	SelectedAgentIDs   JSONStringArray `json:"selected_agent_ids" gorm:"type:jsonb;not null;default:'[]'"`
	Status             QARunStatus     `json:"status" gorm:"type:varchar(24);not null;index"`
	OriginalQuery      string          `json:"original_query" gorm:"type:text;not null"`
	RewrittenQuery     string          `json:"rewritten_query" gorm:"type:text;not null;default:''"`
	ConfigSnapshot     JSONMap         `json:"config_snapshot" gorm:"type:jsonb;not null;default:'{}'"`
	Metrics            JSONMap         `json:"metrics" gorm:"type:jsonb;not null;default:'{}'"`
	LangfuseTraceID    string          `json:"langfuse_trace_id" gorm:"type:varchar(64);not null;default:'';index"`
	ErrorCode          string          `json:"error_code" gorm:"type:varchar(64);not null;default:''"`
	StartedAt          time.Time       `json:"started_at" gorm:"not null;index:idx_qa_execution_runs_user_started,priority:2,sort:desc"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	DurationMS         int64           `json:"duration_ms" gorm:"not null;default:0"`
}

func (QAExecutionRun) TableName() string { return "qa_execution_runs" }

type QARunFinishUpdate struct {
	Status           QARunStatus
	RewrittenQuery   string
	RouteType        QARouteType
	SelectedAgentIDs JSONStringArray
	Metrics          JSONMap
	ErrorCode        string
	CompletedAt      time.Time
	DurationMS       int64
}

type QARunObservation struct {
	Run *QAExecutionRun `json:"run"`
}
