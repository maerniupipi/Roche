package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// SuggestedQuestionRepository persists the three global homepage questions.
type SuggestedQuestionRepository interface {
	List(ctx context.Context) ([]types.HomepageSuggestedQuestion, error)
	Get(ctx context.Context, id string) (*types.HomepageSuggestedQuestion, error)
	ReplaceAll(ctx context.Context, questions []types.HomepageSuggestedQuestion) error
}

// SuggestedQuestionService manages the global, Agent-independent homepage
// questions required by URS Func012-013 and Func066-068.
type SuggestedQuestionService interface {
	ListSuggestedQuestions(ctx context.Context) ([]types.HomepageSuggestedQuestion, error)
	GetSuggestedQuestion(ctx context.Context, id string) (*types.HomepageSuggestedQuestion, error)
	ConfigureSuggestedQuestions(ctx context.Context, items []types.SuggestedQuestionConfigItem) ([]types.HomepageSuggestedQuestion, error)
}
