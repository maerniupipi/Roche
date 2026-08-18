package service

import (
	"context"
	"errors"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/types"
)

// KnowledgeQA has a single execution path. Legacy quick-answer and pipeline
// code remains available internally but is not selected by request fields.
func (s *sessionService) KnowledgeQA(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error {
	if s.unifiedQAService == nil {
		return errors.New("unified QA service is not configured")
	}
	return s.unifiedQAService.Execute(ctx, req, eventBus)
}
