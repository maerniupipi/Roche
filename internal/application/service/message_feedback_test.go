package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"roche.local/knowledge-agent-platform/internal/application/repository"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
)

func newMessageFeedbackTestService(t *testing.T) (interface {
	SetMessageFeedback(ctx context.Context, sessionID, messageID string, feedback *types.MessageFeedback) (*types.MessageFeedback, error)
	DeleteMessageFeedback(ctx context.Context, sessionID, messageID string) error
	GetRecentMessagesBySession(ctx context.Context, sessionID string, limit int) ([]*types.Message, error)
}, *gorm.DB, *types.Session, *types.Message) {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE messages (
		id TEXT PRIMARY KEY, session_id TEXT NOT NULL, request_id TEXT,
		role TEXT NOT NULL, content TEXT NOT NULL, created_at DATETIME,
		updated_at DATETIME, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE message_feedbacks (
		id TEXT PRIMARY KEY, message_id TEXT NOT NULL UNIQUE, session_id TEXT NOT NULL,
		user_id TEXT NOT NULL, rating TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
		comment TEXT NOT NULL DEFAULT '', created_at DATETIME, updated_at DATETIME
	)`).Error)

	session := &types.Session{ID: uuid.NewString(), UserID: "alice", Title: "feedback test"}
	require.NoError(t, db.Exec(
		"INSERT INTO sessions (id, user_id, title) VALUES (?, ?, ?)",
		session.ID, session.UserID, session.Title,
	).Error)
	message := &types.Message{
		ID:          uuid.NewString(),
		SessionID:   session.ID,
		RequestID:   uuid.NewString(),
		Role:        "assistant",
		Content:     "answer",
		IsCompleted: true,
	}
	require.NoError(t, db.Exec(
		"INSERT INTO messages (id, session_id, request_id, role, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		message.ID, message.SessionID, message.RequestID, message.Role, message.Content,
	).Error)

	return NewMessageService(
		repository.NewMessageRepository(db), repository.NewSessionRepository(db),
	), db, session, message
}

func TestMessageFeedbackCanBeCreatedChangedLoadedAndDeleted(t *testing.T) {
	svc, _, session, message := newMessageFeedbackTestService(t)
	ctx := testSessionScopeContext(1, "alice")

	liked, err := svc.SetMessageFeedback(ctx, session.ID, message.ID, &types.MessageFeedback{
		Rating:  types.FeedbackRatingLike,
		Reason:  "factual_error",
		Comment: "stale dislike detail",
	})
	require.NoError(t, err)
	require.Equal(t, types.FeedbackRatingLike, liked.Rating)
	require.Empty(t, liked.Reason)
	require.Empty(t, liked.Comment)

	disliked, err := svc.SetMessageFeedback(ctx, session.ID, message.ID, &types.MessageFeedback{
		Rating:  types.FeedbackRatingDislike,
		Reason:  "other",
		Comment: "  missing a key exception  ",
	})
	require.NoError(t, err)
	require.Equal(t, liked.ID, disliked.ID, "upsert must keep one row per message")
	require.Equal(t, "missing a key exception", disliked.Comment)
	require.Equal(t, "其他", disliked.ReasonZh)
	require.Equal(t, "Other", disliked.ReasonEn)

	messages, err := svc.GetRecentMessagesBySession(ctx, session.ID, 20)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.NotNil(t, messages[0].Feedback)
	require.Equal(t, types.FeedbackRatingDislike, messages[0].Feedback.Rating)
	require.Equal(t, "其他", messages[0].Feedback.ReasonZh)
	require.Equal(t, "Other", messages[0].Feedback.ReasonEn)

	require.NoError(t, svc.DeleteMessageFeedback(ctx, session.ID, message.ID))
	messages, err = svc.GetRecentMessagesBySession(ctx, session.ID, 20)
	require.NoError(t, err)
	require.Nil(t, messages[0].Feedback)
	// Cancellation is idempotent.
	require.NoError(t, svc.DeleteMessageFeedback(ctx, session.ID, message.ID))
}

func TestMessageFeedbackValidatesOwnershipTargetAndReason(t *testing.T) {
	svc, db, session, message := newMessageFeedbackTestService(t)

	_, err := svc.SetMessageFeedback(testSessionScopeContext(1, "bob"), session.ID, message.ID, &types.MessageFeedback{
		Rating: types.FeedbackRatingLike,
	})
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)

	_, err = svc.SetMessageFeedback(testSessionScopeContext(1, "alice"), session.ID, message.ID, &types.MessageFeedback{
		Rating: types.FeedbackRatingDislike,
		Reason: "unknown_reason",
	})
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.ErrValidation, appErr.Code)

	userMessage := &types.Message{
		ID:        uuid.NewString(),
		SessionID: session.ID,
		RequestID: uuid.NewString(),
		Role:      "user",
		Content:   "question",
	}
	require.NoError(t, db.Exec(
		"INSERT INTO messages (id, session_id, request_id, role, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		userMessage.ID, userMessage.SessionID, userMessage.RequestID, userMessage.Role, userMessage.Content,
	).Error)
	_, err = svc.SetMessageFeedback(testSessionScopeContext(1, "alice"), session.ID, userMessage.ID, &types.MessageFeedback{
		Rating: types.FeedbackRatingLike,
	})
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.ErrValidation, appErr.Code)
}
