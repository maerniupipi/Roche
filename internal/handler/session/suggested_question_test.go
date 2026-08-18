package session

import (
	"context"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type fixedAnswerSuggestedQuestionService struct {
	interfaces.SuggestedQuestionService
	question *types.HomepageSuggestedQuestion
	gotID    string
}

func (s *fixedAnswerSuggestedQuestionService) GetSuggestedQuestion(
	_ context.Context, id string,
) (*types.HomepageSuggestedQuestion, error) {
	s.gotID = id
	return s.question, nil
}

func TestResolveSuggestedQuestionAnswerReturnsFixedAnswer(t *testing.T) {
	service := &fixedAnswerSuggestedQuestionService{question: &types.HomepageSuggestedQuestion{
		ID: "fixed-id", Question: "How do I use this?",
		AnswerMode: types.SuggestedQuestionAnswerCustom, CustomAnswer: "Use the search box.",
	}}
	h := &Handler{suggestedQuestions: service}
	answer, err := h.resolveSuggestedQuestionAnswer(context.Background(), "fixed-id", "How do I use this?")
	if err != nil || answer != "Use the search box." || service.gotID != "fixed-id" {
		t.Fatalf("answer=%q gotID=%q err=%v", answer, service.gotID, err)
	}
}

func TestResolveSuggestedQuestionAnswerReturnsEmptyForGeneratedMode(t *testing.T) {
	h := &Handler{suggestedQuestions: &fixedAnswerSuggestedQuestionService{question: &types.HomepageSuggestedQuestion{
		ID: "generated-id", Question: "What is DoA?", AnswerMode: types.SuggestedQuestionAnswerGenerated,
	}}}
	answer, err := h.resolveSuggestedQuestionAnswer(context.Background(), "generated-id", "What is DoA?")
	if err != nil || answer != "" {
		t.Fatalf("answer=%q err=%v", answer, err)
	}
}

func TestResolveSuggestedQuestionAnswerRejectsMismatchedQuery(t *testing.T) {
	h := &Handler{suggestedQuestions: &fixedAnswerSuggestedQuestionService{question: &types.HomepageSuggestedQuestion{
		ID: "fixed-id", Question: "Configured question", AnswerMode: types.SuggestedQuestionAnswerCustom, CustomAnswer: "A",
	}}}
	if _, err := h.resolveSuggestedQuestionAnswer(context.Background(), "fixed-id", "Forged question"); err == nil {
		t.Fatal("mismatched query error = nil")
	}
}
