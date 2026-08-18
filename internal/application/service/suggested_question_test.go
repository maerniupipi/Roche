package service

import (
	"context"
	"testing"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
)

type suggestedQuestionRepo struct {
	questions []types.HomepageSuggestedQuestion
	replaced  []types.HomepageSuggestedQuestion
}

func (r *suggestedQuestionRepo) List(context.Context) ([]types.HomepageSuggestedQuestion, error) {
	return append([]types.HomepageSuggestedQuestion(nil), r.questions...), nil
}

func (r *suggestedQuestionRepo) Get(_ context.Context, id string) (*types.HomepageSuggestedQuestion, error) {
	for i := range r.questions {
		if r.questions[i].ID == id {
			question := r.questions[i]
			return &question, nil
		}
	}
	return nil, nil
}

func (r *suggestedQuestionRepo) ReplaceAll(_ context.Context, questions []types.HomepageSuggestedQuestion) error {
	r.replaced = append([]types.HomepageSuggestedQuestion(nil), questions...)
	r.questions = append([]types.HomepageSuggestedQuestion(nil), questions...)
	return nil
}

func validSuggestedQuestionItems() []types.SuggestedQuestionConfigItem {
	return []types.SuggestedQuestionConfigItem{
		{Question: "费用如何报销？", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 1},
		{Question: "什么是 DoA？", AnswerMode: types.SuggestedQuestionAnswerCustom, CustomAnswer: "DoA 是授权委托规则。", SortOrder: 2},
		{Question: "HCP 互动有哪些要求？", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 3},
	}
}

func TestSuggestedQuestionServiceReplacesExactlyThreeGlobalQuestions(t *testing.T) {
	repo := &suggestedQuestionRepo{}
	service := NewSuggestedQuestionService(repo)

	questions, err := service.ConfigureSuggestedQuestions(context.Background(), validSuggestedQuestionItems())
	if err != nil {
		t.Fatalf("ConfigureSuggestedQuestions() error = %v", err)
	}
	if len(questions) != 3 || len(repo.replaced) != 3 {
		t.Fatalf("questions=%+v replaced=%+v", questions, repo.replaced)
	}
	if questions[0].ID == "" || questions[1].CustomAnswer != "DoA 是授权委托规则。" {
		t.Fatalf("questions = %+v", questions)
	}
}

func TestSuggestedQuestionServiceRequiresExactlyThreeQuestions(t *testing.T) {
	service := NewSuggestedQuestionService(&suggestedQuestionRepo{})
	_, err := service.ConfigureSuggestedQuestions(context.Background(), validSuggestedQuestionItems()[:2])
	appErr, ok := err.(*apperrors.AppError)
	if !ok || appErr.HTTPCode != 400 {
		t.Fatalf("error = %#v, want bad request", err)
	}
}

func TestSuggestedQuestionServiceValidatesCustomAnswerAndOrder(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]types.SuggestedQuestionConfigItem)
	}{
		{name: "missing custom answer", mutate: func(items []types.SuggestedQuestionConfigItem) { items[1].CustomAnswer = "" }},
		{name: "duplicate order", mutate: func(items []types.SuggestedQuestionConfigItem) { items[2].SortOrder = 2 }},
		{name: "duplicate question", mutate: func(items []types.SuggestedQuestionConfigItem) { items[2].Question = items[0].Question }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := validSuggestedQuestionItems()
			tt.mutate(items)
			_, err := NewSuggestedQuestionService(&suggestedQuestionRepo{}).ConfigureSuggestedQuestions(context.Background(), items)
			if err == nil {
				t.Fatal("ConfigureSuggestedQuestions() error = nil")
			}
		})
	}
}

func TestSuggestedQuestionServiceClearsCustomAnswerInGeneratedMode(t *testing.T) {
	items := validSuggestedQuestionItems()
	items[0].CustomAnswer = "must not leak"
	repo := &suggestedQuestionRepo{}
	_, err := NewSuggestedQuestionService(repo).ConfigureSuggestedQuestions(context.Background(), items)
	if err != nil {
		t.Fatalf("ConfigureSuggestedQuestions() error = %v", err)
	}
	if repo.replaced[0].CustomAnswer != "" {
		t.Fatalf("generated custom answer = %q", repo.replaced[0].CustomAnswer)
	}
}

func TestSuggestedQuestionServiceGetsConfiguredQuestionByID(t *testing.T) {
	repo := &suggestedQuestionRepo{questions: []types.HomepageSuggestedQuestion{{
		ID: "question-id", Question: "Q", AnswerMode: types.SuggestedQuestionAnswerCustom, CustomAnswer: "A", SortOrder: 1,
	}}}
	question, err := NewSuggestedQuestionService(repo).GetSuggestedQuestion(context.Background(), "question-id")
	if err != nil || question == nil || question.CustomAnswer != "A" {
		t.Fatalf("question=%+v err=%v", question, err)
	}
}
