package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

const requiredSuggestedQuestionCount = 3

type suggestedQuestionService struct {
	repo interfaces.SuggestedQuestionRepository
}

func NewSuggestedQuestionService(repo interfaces.SuggestedQuestionRepository) interfaces.SuggestedQuestionService {
	return &suggestedQuestionService{repo: repo}
}

func (s *suggestedQuestionService) ListSuggestedQuestions(ctx context.Context) ([]types.HomepageSuggestedQuestion, error) {
	questions, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if questions == nil {
		questions = []types.HomepageSuggestedQuestion{}
	}
	return questions, nil
}

func (s *suggestedQuestionService) GetSuggestedQuestion(ctx context.Context, id string) (*types.HomepageSuggestedQuestion, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.NewBadRequestError("suggested_question_id cannot be empty")
	}
	question, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if question == nil {
		return nil, apperrors.NewNotFoundError("suggested question not found")
	}
	return question, nil
}

func (s *suggestedQuestionService) ConfigureSuggestedQuestions(
	ctx context.Context,
	items []types.SuggestedQuestionConfigItem,
) ([]types.HomepageSuggestedQuestion, error) {
	if len(items) != requiredSuggestedQuestionCount {
		return nil, apperrors.NewBadRequestError("exactly 3 suggested questions are required")
	}

	questions := make([]types.HomepageSuggestedQuestion, 0, requiredSuggestedQuestionCount)
	seenIDs := make(map[string]struct{}, requiredSuggestedQuestionCount)
	seenQuestions := make(map[string]struct{}, requiredSuggestedQuestionCount)
	seenOrder := make(map[int]struct{}, requiredSuggestedQuestionCount)
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = uuid.NewString()
		} else if _, err := uuid.Parse(id); err != nil {
			return nil, apperrors.NewBadRequestError("id must be a UUID")
		}
		if _, exists := seenIDs[id]; exists {
			return nil, apperrors.NewBadRequestError("suggested question IDs must be unique")
		}
		seenIDs[id] = struct{}{}

		question := strings.TrimSpace(item.Question)
		if question == "" {
			return nil, apperrors.NewBadRequestError("question cannot be empty")
		}
		questionKey := strings.ToLower(question)
		if _, exists := seenQuestions[questionKey]; exists {
			return nil, apperrors.NewBadRequestError("suggested questions must be unique")
		}
		seenQuestions[questionKey] = struct{}{}

		if item.SortOrder < 1 || item.SortOrder > requiredSuggestedQuestionCount {
			return nil, apperrors.NewBadRequestError("sort_order must be between 1 and 3")
		}
		if _, exists := seenOrder[item.SortOrder]; exists {
			return nil, apperrors.NewBadRequestError("sort_order must be unique")
		}
		seenOrder[item.SortOrder] = struct{}{}

		answerMode := item.AnswerMode
		customAnswer := strings.TrimSpace(item.CustomAnswer)
		switch answerMode {
		case types.SuggestedQuestionAnswerGenerated:
			customAnswer = ""
		case types.SuggestedQuestionAnswerCustom:
			if customAnswer == "" {
				return nil, apperrors.NewBadRequestError("custom_answer is required when answer_mode is custom")
			}
		default:
			return nil, apperrors.NewBadRequestError("answer_mode must be generated or custom")
		}

		questions = append(questions, types.HomepageSuggestedQuestion{
			ID: id, Question: question, AnswerMode: answerMode,
			CustomAnswer: customAnswer, SortOrder: item.SortOrder,
		})
	}

	if err := s.repo.ReplaceAll(ctx, questions); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}
