package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type feedbackMessageService struct {
	interfaces.MessageService
	set    func(context.Context, string, string, *types.MessageFeedback) (*types.MessageFeedback, error)
	delete func(context.Context, string, string) error
}

func (s *feedbackMessageService) SetMessageFeedback(
	ctx context.Context, sessionID, messageID string, feedback *types.MessageFeedback,
) (*types.MessageFeedback, error) {
	return s.set(ctx, sessionID, messageID, feedback)
}

func (s *feedbackMessageService) DeleteMessageFeedback(ctx context.Context, sessionID, messageID string) error {
	return s.delete(ctx, sessionID, messageID)
}

func newFeedbackMessageRouter(svc interfaces.MessageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	h := NewMessageHandler(svc)
	r.PUT("/messages/:session_id/:id/feedback", h.SetMessageFeedback)
	r.DELETE("/messages/:session_id/:id/feedback", h.DeleteMessageFeedback)
	return r
}

func TestSetMessageFeedbackEndpoint(t *testing.T) {
	var got *types.MessageFeedback
	svc := &feedbackMessageService{
		set: func(_ context.Context, sessionID, messageID string, feedback *types.MessageFeedback) (*types.MessageFeedback, error) {
			require.Equal(t, "session-1", sessionID)
			require.Equal(t, "message-1", messageID)
			got = feedback
			return &types.MessageFeedback{
				ID: "feedback-1", SessionID: sessionID, MessageID: messageID,
				Rating: feedback.Rating, Reason: feedback.Reason,
			}, nil
		},
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut, "/messages/session-1/message-1/feedback",
		strings.NewReader(`{"rating":"dislike","reason":"factual_error"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	newFeedbackMessageRouter(svc).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, got)
	require.Equal(t, types.FeedbackRatingDislike, got.Rating)
	require.Equal(t, "factual_error", got.Reason)
	require.Contains(t, w.Body.String(), `"feedback-1"`)
	require.Contains(t, w.Body.String(), `"reason_zh":"事实性错误"`)
	require.Contains(t, w.Body.String(), `"reason_en":"Factual error"`)
}

func TestSetMessageFeedbackEndpointReturnsValidationError(t *testing.T) {
	svc := &feedbackMessageService{
		set: func(_ context.Context, _, _ string, _ *types.MessageFeedback) (*types.MessageFeedback, error) {
			return nil, apperrors.NewValidationError("invalid reason")
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut, "/messages/session-1/message-1/feedback",
		strings.NewReader(`{"rating":"dislike","reason":"invalid"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	newFeedbackMessageRouter(svc).ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestDeleteMessageFeedbackEndpoint(t *testing.T) {
	called := false
	svc := &feedbackMessageService{
		delete: func(_ context.Context, sessionID, messageID string) error {
			called = true
			require.Equal(t, "session-1", sessionID)
			require.Equal(t, "message-1", messageID)
			return nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/messages/session-1/message-1/feedback", nil)
	newFeedbackMessageRouter(svc).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, called)
}
