package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// messageService implements the MessageService interface for managing messaging operations
// It handles creating, retrieving, updating, and deleting messages within sessions.
type messageService struct {
	messageRepo interfaces.MessageRepository // Repository for message storage operations
	sessionRepo interfaces.SessionRepository // Repository for session validation
}

// NewMessageService creates a new message service instance with the required repositories
func NewMessageService(messageRepo interfaces.MessageRepository,
	sessionRepo interfaces.SessionRepository,
) interfaces.MessageService {
	return &messageService{
		messageRepo: messageRepo,
		sessionRepo: sessionRepo,
	}
}

func sessionUserIDForLookup(ctx context.Context) string {
	return types.SessionOwnerIDFromContext(ctx)
}

// CreateMessage creates a new message within an existing session
func (s *messageService) CreateMessage(ctx context.Context, message *types.Message) (*types.Message, error) {
	logger.Info(ctx, "Start creating message")
	logger.Infof(ctx, "Creating message for session ID: %s", message.SessionID)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), message.SessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return nil, err
	}

	logger.Info(ctx, "Session exists, creating message")
	createdMessage, err := s.messageRepo.CreateMessage(ctx, message)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id": message.SessionID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Message created successfully, ID: %s", createdMessage.ID)
	return createdMessage, nil
}

// GetMessage retrieves a specific message by its ID within a session
func (s *messageService) GetMessage(ctx context.Context, sessionID string, messageID string) (*types.Message, error) {
	logger.Info(ctx, "Start getting message")
	logger.Infof(ctx, "Getting message, session ID: %s, message ID: %s", sessionID, messageID)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return nil, err
	}

	logger.Info(ctx, "Session exists, getting message")
	message, err := s.messageRepo.GetMessage(ctx, sessionID, messageID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id": sessionID,
			"message_id": messageID,
		})
		return nil, err
	}

	logger.Info(ctx, "Message retrieved successfully")
	return message, nil
}

// GetMessagesBySession retrieves paginated messages for a specific session
func (s *messageService) GetMessagesBySession(ctx context.Context,
	sessionID string, page int, pageSize int,
) ([]*types.Message, error) {
	logger.Info(ctx, "Start getting messages by session")
	logger.Infof(ctx, "Getting messages for session ID: %s, page: %d, pageSize: %d", sessionID, page, pageSize)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return nil, err
	}

	logger.Info(ctx, "Session exists, getting messages")
	messages, err := s.messageRepo.GetMessagesBySession(ctx, sessionID, page, pageSize)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id": sessionID,
			"page":       page,
			"page_size":  pageSize,
		})
		return nil, err
	}

	logger.Infof(ctx, "Retrieved %d messages successfully", len(messages))
	return messages, nil
}

// GetRecentMessagesBySession retrieves the most recent messages from a session
func (s *messageService) GetRecentMessagesBySession(ctx context.Context,
	sessionID string, limit int,
) ([]*types.Message, error) {
	logger.Info(ctx, "Start getting recent messages by session")
	logger.Infof(ctx, "Getting recent messages for session ID: %s, limit: %d", sessionID, limit)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return nil, err
	}

	logger.Info(ctx, "Session exists, getting recent messages")
	messages, err := s.messageRepo.GetRecentMessagesBySession(ctx, sessionID, limit)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id": sessionID,
			"limit":      limit,
		})
		return nil, err
	}

	logger.Infof(ctx, "Retrieved %d recent messages successfully", len(messages))
	return messages, nil
}

// GetMessagesBySessionBeforeTime retrieves messages sent before a specific time
func (s *messageService) GetMessagesBySessionBeforeTime(ctx context.Context,
	sessionID string, beforeTime time.Time, limit int,
) ([]*types.Message, error) {
	logger.Info(ctx, "Start getting messages before time")
	logger.Infof(ctx, "Getting messages before %v for session ID: %s, limit: %d", beforeTime, sessionID, limit)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return nil, err
	}

	logger.Info(ctx, "Session exists, getting messages before time")
	messages, err := s.messageRepo.GetMessagesBySessionBeforeTime(ctx, sessionID, beforeTime, limit)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id":  sessionID,
			"before_time": beforeTime,
			"limit":       limit,
		})
		return nil, err
	}

	logger.Infof(ctx, "Retrieved %d messages before time successfully", len(messages))
	return messages, nil
}

// UpdateMessage updates an existing message's content or metadata
func (s *messageService) UpdateMessage(ctx context.Context, message *types.Message) error {
	logger.Info(ctx, "Start updating message")
	logger.Infof(ctx, "Updating message, ID: %s, session ID: %s", message.ID, message.SessionID)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), message.SessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return err
	}

	logger.Info(ctx, "Session exists, updating message")
	err = s.messageRepo.UpdateMessage(ctx, message)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id": message.SessionID,
			"message_id": message.ID,
		})
		return err
	}

	logger.Info(ctx, "Message updated successfully")
	return nil
}

// UpdateMessageImages updates only the images JSONB column for a message.
func (s *messageService) UpdateMessageImages(ctx context.Context, sessionID, messageID string, images types.MessageImages) error {
	return s.messageRepo.UpdateMessageImages(ctx, sessionID, messageID, images)
}

// UpdateMessageRenderedContent updates the rendered_content column for a user message.
func (s *messageService) UpdateMessageRenderedContent(ctx context.Context, sessionID, messageID string, renderedContent string) error {
	return s.messageRepo.UpdateMessageRenderedContent(ctx, sessionID, messageID, renderedContent)
}

// SetMessageFeedback creates or replaces feedback for an assistant message.
// Session ownership is checked before the message is read or updated.
func (s *messageService) SetMessageFeedback(
	ctx context.Context, sessionID, messageID string, feedback *types.MessageFeedback,
) (*types.MessageFeedback, error) {
	if feedback == nil {
		return nil, apperrors.NewValidationError("feedback is required")
	}
	if err := s.validateFeedbackTarget(ctx, sessionID, messageID); err != nil {
		return nil, err
	}

	rating := types.FeedbackRating(strings.TrimSpace(string(feedback.Rating)))
	reason := strings.TrimSpace(feedback.Reason)
	comment := strings.TrimSpace(feedback.Comment)
	switch rating {
	case types.FeedbackRatingLike:
		// A like carries no negative-reason payload. Clearing it here also
		// prevents stale dislike details when the user switches their choice.
		reason = ""
		comment = ""
	case types.FeedbackRatingDislike:
		if _, ok := types.LookupFeedbackReason(reason); !ok {
			return nil, apperrors.NewValidationError("a valid dislike reason is required")
		}
		if reason == string(types.FeedbackReasonOther) && comment == "" {
			return nil, apperrors.NewValidationError("comment is required when reason is other")
		}
	default:
		return nil, apperrors.NewValidationError("rating must be like or dislike")
	}
	if utf8.RuneCountInString(comment) > 500 {
		return nil, apperrors.NewValidationError("comment must not exceed 500 characters")
	}

	userID := types.SessionOwnerIDFromContext(ctx)
	if userID == "" {
		return nil, apperrors.NewUnauthorizedError("user context is required")
	}
	return s.messageRepo.UpsertMessageFeedback(ctx, &types.MessageFeedback{
		MessageID: messageID,
		SessionID: sessionID,
		UserID:    userID,
		Rating:    rating,
		Reason:    reason,
		Comment:   comment,
	})
}

// DeleteMessageFeedback removes the user's current rating. Deleting an
// already-absent feedback is intentionally idempotent.
func (s *messageService) DeleteMessageFeedback(ctx context.Context, sessionID, messageID string) error {
	if err := s.validateFeedbackTarget(ctx, sessionID, messageID); err != nil {
		return err
	}
	return s.messageRepo.DeleteMessageFeedback(ctx, sessionID, messageID)
}

func (s *messageService) validateFeedbackTarget(ctx context.Context, sessionID, messageID string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(messageID) == "" {
		return apperrors.NewValidationError("session_id and message_id are required")
	}
	if types.SessionOwnerIDFromContext(ctx) == "" {
		return apperrors.NewUnauthorizedError("user context is required")
	}
	if _, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID); err != nil {
		return err
	}
	message, err := s.messageRepo.GetMessage(ctx, sessionID, messageID)
	if err != nil {
		return err
	}
	if message.Role != "assistant" {
		return apperrors.NewValidationError("only assistant messages can be rated")
	}
	return nil
}

// DeleteMessage removes a message from a session.
func (s *messageService) DeleteMessage(ctx context.Context, sessionID string, messageID string) error {
	logger.Info(ctx, "Start deleting message")
	logger.Infof(ctx, "Deleting message, session ID: %s, message ID: %s", sessionID, messageID)

	_, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return err
	}

	// Delete the message from the repository
	logger.Info(ctx, "Session exists, deleting message")
	err = s.messageRepo.DeleteMessage(ctx, sessionID, messageID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"session_id": sessionID,
			"message_id": messageID,
		})
		return err
	}

	logger.Info(ctx, "Message deleted successfully")
	return nil
}

// ClearSessionMessages deletes all messages in a session.
func (s *messageService) ClearSessionMessages(ctx context.Context, sessionID string) error {
	logger.Infof(ctx, "Start clearing all messages for session: %s", sessionID)

	if _, err := s.sessionRepo.Get(ctx, sessionUserIDForLookup(ctx), sessionID); err != nil {
		logger.Errorf(ctx, "Failed to get session: %v", err)
		return err
	}

	if err := s.messageRepo.DeleteMessagesBySessionID(ctx, sessionID); err != nil {
		logger.Errorf(ctx, "Failed to delete messages for session %s: %v", sessionID, err)
		return err
	}

	logger.Infof(ctx, "All messages cleared for session: %s", sessionID)
	return nil
}
