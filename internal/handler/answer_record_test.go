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
)

type adminAnswerRecordHandlerService struct {
	query *types.AdminAnswerRecordQuery
}

func (s *adminAnswerRecordHandlerService) List(
	_ context.Context, query *types.AdminAnswerRecordQuery,
) (*types.PageResult, error) {
	s.query = query
	feedback := &types.MessageFeedback{
		Rating: types.FeedbackRatingDislike, Reason: "other",
		ReasonZh: "其他", ReasonEn: "Other", Comment: "详细反馈",
	}
	return &types.PageResult{Total: 1, Page: 1, PageSize: 20, Data: []types.AdminAnswerRecord{{
		ID: "a1", KnowledgeBases: []string{"DoA"}, Feedback: feedback,
	}}}, nil
}

func (s *adminAnswerRecordHandlerService) Export(
	context.Context, *types.AdminAnswerRecordQuery,
) ([]byte, error) {
	return []byte("csv"), nil
}

func TestAdminAnswerRecordHandlerReturnsKnowledgeBaseNamesAndFeedback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &adminAnswerRecordHandlerService{}
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.GET("/system/admin/answer-records", NewAdminAnswerRecordHandler(service).List)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/system/admin/answer-records?channel=app&feedback=dislike&is_fallback=true&username=Helen&start_time=2026-08-01&end_time=2026-08-31", nil)
	router.ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"knowledge_bases":["DoA"]`) ||
		!strings.Contains(body, `"reason_zh":"其他"`) || !strings.Contains(body, `"comment":"详细反馈"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
	if strings.Contains(body, `"is_fallback"`) {
		t.Fatalf("is_fallback must remain filter-only: %s", body)
	}
	if service.query == nil || service.query.Channel != "app" || service.query.Feedback != "dislike" ||
		service.query.IsFallback == nil || !*service.query.IsFallback ||
		service.query.StartTime == nil || service.query.EndTime == nil {
		t.Fatalf("query=%+v", service.query)
	}
}

func TestAdminAnswerRecordHandlerRejectsInvalidFallbackFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.GET("/system/admin/answer-records", NewAdminAnswerRecordHandler(&adminAnswerRecordHandlerService{}).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/system/admin/answer-records?is_fallback=unknown", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminAnswerRecordHandlerRejectsInvalidTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.GET("/system/admin/answer-records", NewAdminAnswerRecordHandler(&adminAnswerRecordHandlerService{}).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/system/admin/answer-records?start_time=invalid", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
