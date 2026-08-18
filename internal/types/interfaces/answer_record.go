package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

type AdminAnswerRecordRepository interface {
	Query(ctx context.Context, query *types.AdminAnswerRecordQuery) ([]types.AdminAnswerRecord, int64, error)
}

type AdminAnswerRecordService interface {
	List(ctx context.Context, query *types.AdminAnswerRecordQuery) (*types.PageResult, error)
	Export(ctx context.Context, query *types.AdminAnswerRecordQuery) ([]byte, error)
}
