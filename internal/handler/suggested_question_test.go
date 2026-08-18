package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type suggestedQuestionHandlerService struct {
	interfaces.SuggestedQuestionService
	configured []types.SuggestedQuestionConfigItem
}

func (s *suggestedQuestionHandlerService) ListSuggestedQuestions(context.Context) ([]types.HomepageSuggestedQuestion, error) {
	return []types.HomepageSuggestedQuestion{{
		ID: "id-1", Question: "费用怎么报销？",
		AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 1,
	}}, nil
}

func (s *suggestedQuestionHandlerService) ConfigureSuggestedQuestions(
	_ context.Context, items []types.SuggestedQuestionConfigItem,
) ([]types.HomepageSuggestedQuestion, error) {
	s.configured = items
	result := make([]types.HomepageSuggestedQuestion, 0, len(items))
	for _, item := range items {
		result = append(result, types.HomepageSuggestedQuestion{
			ID: item.ID, Question: item.Question, AnswerMode: item.AnswerMode,
			CustomAnswer: item.CustomAnswer, SortOrder: item.SortOrder,
		})
	}
	return result, nil
}

func TestSuggestedQuestionHandlerList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewSuggestedQuestionHandler(&suggestedQuestionHandlerService{})
	r.GET("/suggested-questions", h.List)

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/suggested-questions", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"answer_mode":"generated"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSuggestedQuestionHandlerConfigureAcceptsThreeQuestions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &suggestedQuestionHandlerService{}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.PUT("/suggested-questions/config", NewSuggestedQuestionHandler(service).Configure)

	body := `{"items":[` +
		`{"question":"Q1","answer_mode":"generated","sort_order":1},` +
		`{"question":"Q2","answer_mode":"custom","custom_answer":"A2","sort_order":2},` +
		`{"question":"Q3","answer_mode":"generated","sort_order":3}` +
		`]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/suggested-questions/config", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || len(service.configured) != 3 || service.configured[1].CustomAnswer != "A2" ||
		!strings.Contains(recorder.Body.String(), `"message":"推荐问题配置保存成功"`) {
		t.Fatalf("status=%d body=%s configured=%+v", recorder.Code, recorder.Body.String(), service.configured)
	}
}

func TestSuggestedQuestionHandlerRejectsNonThreeQuestionPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.PUT("/suggested-questions/config", NewSuggestedQuestionHandler(&suggestedQuestionHandlerService{}).Configure)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/suggested-questions/config", strings.NewReader(
		`{"items":[{"question":"Q1","answer_mode":"generated","sort_order":1}]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
