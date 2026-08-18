package service

import (
	"context"
	"fmt"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// dashboardService implements the DashboardService interface.
type dashboardService struct {
	repo interfaces.DashboardRepository
}

// NewDashboardService creates a new dashboard service.
func NewDashboardService(repo interfaces.DashboardRepository) interfaces.DashboardService {
	return &dashboardService{repo: repo}
}

func (s *dashboardService) GetKnowledgeBaseStats(ctx context.Context, kbID string) (*types.DashboardKnowledgeBaseStats, error) {
	return s.repo.CountKnowledgesByStatus(ctx, kbID)
}

func (s *dashboardService) GetChatStats(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (*types.DashboardChatStats, error) {
	avgFirst, avgComplete, err := s.repo.GetAverageResponseDurations(ctx, knowledgeDomainID, start, end)
	if err != nil {
		return nil, fmt.Errorf("avg durations: %w", err)
	}

	daily, err := s.repo.ListDailyChatStats(ctx, knowledgeDomainID, start, end)
	if err != nil {
		return nil, fmt.Errorf("daily stats: %w", err)
	}

	return &types.DashboardChatStats{
		AvgFirstResponseSec: avgFirst,
		AvgCompleteSec:      avgComplete,
		Daily:               fillMissingDates(daily, start, end),
	}, nil
}

func (s *dashboardService) GetOverview(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (*types.DashboardOverview, error) {
	domainDist, err := s.repo.ListDomainDistribution(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("domain distribution: %w", err)
	}

	hotDocs, err := s.repo.ListHotDocuments(ctx, knowledgeDomainID, start, end, 10)
	if err != nil {
		return nil, fmt.Errorf("hot documents: %w", err)
	}

	feedback, err := s.repo.ListProductFeedback(ctx, knowledgeDomainID, start, end, 10)
	if err != nil {
		return nil, fmt.Errorf("product feedback: %w", err)
	}

	topUsers, err := s.repo.ListTopAskers(ctx, knowledgeDomainID, start, end, 10)
	if err != nil {
		return nil, fmt.Errorf("top askers: %w", err)
	}

	valid, fallback, err := s.repo.CountValidAndFallbackAnswers(ctx, knowledgeDomainID, start, end)
	if err != nil {
		return nil, fmt.Errorf("valid/fallback count: %w", err)
	}

	fallbackQuestions, err := s.repo.GetFallbackQuestions(ctx, knowledgeDomainID, start, end, 10)
	if err != nil {
		return nil, fmt.Errorf("fallback questions: %w", err)
	}

	single, multi := int64(0), int64(0)
	for _, item := range domainDist {
		if item.Name != "" && item.Name != "default" {
			multi += item.Value
		} else {
			single += item.Value
		}
	}
	if multi > 0 {
		single, multi = multi, single
	}

	return &types.DashboardOverview{
		DomainDistribution:  domainDist,
		CrossDomainSingle:   single,
		CrossDomainMulti:    multi,
		TopDocuments:        hotDocs,
		ProductFeedback:     feedback,
		TopUsers:            topUsers,
		ValidAnswerCount:    valid,
		FallbackAnswerCount: fallback,
		FallbackQuestions:   fallbackQuestions,
	}, nil
}

func fillMissingDates(daily []types.DashboardDailyChatStats, start, end time.Time) []types.DashboardDailyChatStats {
	if len(daily) == 0 {
		return daily
	}

	idx := make(map[string]types.DashboardDailyChatStats)
	for _, d := range daily {
		idx[d.Date] = d
	}

	result := make([]types.DashboardDailyChatStats, 0)
	for d := start.Truncate(24 * time.Hour); !d.After(end); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		if v, ok := idx[key]; ok {
			result = append(result, v)
		} else {
			result = append(result, types.DashboardDailyChatStats{Date: key})
		}
	}
	return result
}
