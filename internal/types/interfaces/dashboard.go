package interfaces

import (
	"context"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
)

// DashboardRepository aggregates raw analytics from the persistence layer.
type DashboardRepository interface {
	// CountKnowledgesByStatus returns document counters for the given knowledge base (or all when kbID is empty).
	CountKnowledgesByStatus(ctx context.Context, kbID string) (*types.DashboardKnowledgeBaseStats, error)
	// ListDailyChatStats returns day-by-day question count / unique users for a date range.
	ListDailyChatStats(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) ([]types.DashboardDailyChatStats, error)
	// GetAverageResponseDurations returns average agent durations in seconds over the date range.
	GetAverageResponseDurations(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (avgFirstResponseSec, avgCompleteSec float64, err error)
	// GetFallbackQuestions returns recent user questions that triggered fallback answers.
	GetFallbackQuestions(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardFallbackQuestionItem, error)
	// CountValidAndFallbackAnswers returns totals of valid vs fallback assistant answers.
	CountValidAndFallbackAnswers(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (valid, fallback int64, err error)
	// ListTopAskers returns users ordered by question count.
	ListTopAskers(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardTopUserItem, error)
	// ListHotDocuments returns documents ordered by citation hit count.
	ListHotDocuments(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardTopDocumentItem, error)
	// ListProductFeedback returns aggregated feedback categories for the date range.
	ListProductFeedback(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardFeedbackItem, error)
	// ListDomainDistribution returns question distribution by knowledge domain.
	ListDomainDistribution(ctx context.Context, start, end time.Time) ([]types.DashboardDomainDistributionItem, error)
}

// DashboardService exposes analytics for the dashboard UI.
type DashboardService interface {
	// GetKnowledgeBaseStats returns document lifecycle counters.
	GetKnowledgeBaseStats(ctx context.Context, kbID string) (*types.DashboardKnowledgeBaseStats, error)
	// GetChatStats returns conversation metrics over a date range.
	GetChatStats(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (*types.DashboardChatStats, error)
	// GetOverview returns cross-domain aggregated analytics.
	GetOverview(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (*types.DashboardOverview, error)
}
