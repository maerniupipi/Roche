package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// TaskDeadLetterRepository persists rows for the generic task dead-letter
// archive (`task_dead_letters`). The asynq dead-letter middleware writes
// one row per archived task.
//
// Reads are operator-driven: list by scope or task type. No TTL.
type TaskDeadLetterRepository interface {
	// Insert records one dead letter. Best-effort caller: the asynq
	// middleware ignores the error so a failed insert never masks the
	// underlying task error.
	Insert(ctx context.Context, dl *types.TaskDeadLetter) error

	// ListByScope returns dead letters for the given scope tuple,
	// newest-first, paginated by failed-id cursor. `cursor` is the
	// stringified id of the oldest entry from the previous page; "" =
	// from the newest. Empty nextCursor = end of stream. `limit` is
	// clamped to [1, 200].
	ListByScope(ctx context.Context, scope, scopeID, cursor string, limit int) ([]*types.TaskDeadLetter, string, error)

	// ListByTaskType returns dead letters for the given task_type,
	// newest-first, with the same cursor semantics as ListByScope.
	ListByTaskType(ctx context.Context, taskType, cursor string, limit int) ([]*types.TaskDeadLetter, string, error)

	// DeleteByID drops a single dead letter (e.g. after operators have
	// requeued the task manually).
	DeleteByID(ctx context.Context, id int64) error
}
