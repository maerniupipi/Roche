package service

import (
	"context"
	"strings"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

type adminAnswerRecordRepoStub struct {
	query *types.AdminAnswerRecordQuery
}

func TestAdminAnswerRecordServiceExportsFeedbackAndKnowledgeBaseNames(t *testing.T) {
	repo := &exportAdminAnswerRecordRepo{}
	content, err := NewAdminAnswerRecordService(repo).Export(context.Background(), &types.AdminAnswerRecordQuery{})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	csv := string(content)
	for _, want := range []string{"DoA;T&E", "dislike", "other", "其他", "Other", "具体反馈"} {
		if !strings.Contains(csv, want) {
			t.Fatalf("CSV missing %q: %s", want, csv)
		}
	}
}

type exportAdminAnswerRecordRepo struct{}

func (r *exportAdminAnswerRecordRepo) Query(
	_ context.Context, query *types.AdminAnswerRecordQuery,
) ([]types.AdminAnswerRecord, int64, error) {
	if query.PageSize == 1 {
		return []types.AdminAnswerRecord{}, 1, nil
	}
	return []types.AdminAnswerRecord{{
		Channel: "web", Username: "Helen", KnowledgeBases: []string{"DoA", "T&E"},
		Feedback: &types.MessageFeedback{
			Rating: types.FeedbackRatingDislike, Reason: "other",
			ReasonZh: "其他", ReasonEn: "Other", Comment: "具体反馈",
		},
	}}, 1, nil
}

func (r *adminAnswerRecordRepoStub) Query(
	_ context.Context, query *types.AdminAnswerRecordQuery,
) ([]types.AdminAnswerRecord, int64, error) {
	r.query = query
	return []types.AdminAnswerRecord{}, 0, nil
}

func TestAdminAnswerRecordServiceNormalizesAndDefaultsQuery(t *testing.T) {
	repo := &adminAnswerRecordRepoStub{}
	isFallback := false
	result, err := NewAdminAnswerRecordService(repo).List(context.Background(), &types.AdminAnswerRecordQuery{
		Channel: " APP ", Feedback: " DISLIKE ", IsFallback: &isFallback,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repo.query.Channel != "app" || repo.query.Feedback != "dislike" ||
		repo.query.IsFallback == nil || *repo.query.IsFallback ||
		result.Page != 1 || result.PageSize != 20 {
		t.Fatalf("query=%+v result=%+v", repo.query, result)
	}
}

func TestAdminAnswerRecordServiceRejectsInvalidFilters(t *testing.T) {
	service := NewAdminAnswerRecordService(&adminAnswerRecordRepoStub{})
	for _, query := range []*types.AdminAnswerRecordQuery{
		{Channel: "api"}, {Channel: "mobile"}, {Feedback: "bad"},
	} {
		if _, err := service.List(context.Background(), query); err == nil {
			t.Fatalf("List(%+v) error = nil", query)
		}
	}
}
