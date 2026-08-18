package repository

import (
	"context"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// messageRepository implements the message repository interface
type messageRepository struct {
	db *gorm.DB
}

// NewMessageRepository creates a new message repository
func NewMessageRepository(db *gorm.DB) interfaces.MessageRepository {
	return &messageRepository{
		db: db,
	}
}

// CreateMessage creates a new message
func (r *messageRepository) CreateMessage(
	ctx context.Context, message *types.Message,
) (*types.Message, error) {
	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return nil, err
	}
	return message, nil
}

// GetMessage retrieves a message
func (r *messageRepository) GetMessage(
	ctx context.Context, sessionID string, messageID string,
) (*types.Message, error) {
	var message types.Message
	if err := r.db.WithContext(ctx).Preload("Feedback").Where(
		"id = ? AND session_id = ?", messageID, sessionID,
	).First(&message).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

// GetMessagesBySession retrieves all messages for a session with pagination
func (r *messageRepository) GetMessagesBySession(
	ctx context.Context, sessionID string, page int, pageSize int,
) ([]*types.Message, error) {
	var messages []*types.Message
	if err := r.db.WithContext(ctx).Preload("Feedback").Where("session_id = ?", sessionID).Order("created_at ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// GetRecentMessagesBySession retrieves recent messages for a session
func (r *messageRepository) GetRecentMessagesBySession(
	ctx context.Context, sessionID string, limit int,
) ([]*types.Message, error) {
	var messages []*types.Message
	if err := r.db.WithContext(ctx).Preload("Feedback").Where(
		"session_id = ?", sessionID,
	).Order("created_at DESC").Limit(limit).Find(&messages).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	slices.SortFunc(messages, func(a, b *types.Message) int {
		cmp := a.CreatedAt.Compare(b.CreatedAt)
		if cmp == 0 {
			if a.Role == "user" { // User messages come first
				return -1
			}
			return 1 // Assistant messages come last
		}
		return cmp
	})
	return messages, nil
}

// GetMessagesBySessionBeforeTime retrieves messages from a session created before a specific time
func (r *messageRepository) GetMessagesBySessionBeforeTime(
	ctx context.Context, sessionID string, beforeTime time.Time, limit int,
) ([]*types.Message, error) {
	var messages []*types.Message
	if err := r.db.WithContext(ctx).Preload("Feedback").Where(
		"session_id = ? AND created_at < ?", sessionID, beforeTime,
	).Order("created_at DESC").Limit(limit).Find(&messages).Error; err != nil {
		return nil, err
	}
	slices.SortFunc(messages, func(a, b *types.Message) int {
		cmp := a.CreatedAt.Compare(b.CreatedAt)
		if cmp == 0 {
			if a.Role == "user" { // User messages come first
				return -1
			}
			return 1 // Assistant messages come last
		}
		return cmp
	})
	return messages, nil
}

// UpdateMessage updates an existing message
func (r *messageRepository) UpdateMessage(ctx context.Context, message *types.Message) error {
	return r.db.WithContext(ctx).Model(&types.Message{}).Where(
		"id = ? AND session_id = ?", message.ID, message.SessionID,
	).Updates(message).Error
}

// DeleteMessage deletes a message
func (r *messageRepository) DeleteMessage(ctx context.Context, sessionID string, messageID string) error {
	return r.db.WithContext(ctx).Where(
		"id = ? AND session_id = ?", messageID, sessionID,
	).Delete(&types.Message{}).Error
}

// GetFirstMessageOfUser retrieves the first message from a user in a session
func (r *messageRepository) GetFirstMessageOfUser(ctx context.Context, sessionID string) (*types.Message, error) {
	var message types.Message
	if err := r.db.WithContext(ctx).Where(
		"session_id = ? and role = ?", sessionID, "user",
	).Order("created_at ASC").First(&message).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

// GetMessageByRequestID retrieves a message by request ID
func (r *messageRepository) GetMessageByRequestID(
	ctx context.Context, sessionID string, requestID string,
) (*types.Message, error) {
	var message types.Message

	result := r.db.WithContext(ctx).
		Where("session_id = ? AND request_id = ?", sessionID, requestID).
		First(&message)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}

	return &message, nil
}

// UpdateMessageImages updates only the images JSONB column for a message.
// Uses Select to force GORM to include the column even when struct-based
// Updates would otherwise skip custom Valuer types.
func (r *messageRepository) UpdateMessageImages(ctx context.Context, sessionID, messageID string, images types.MessageImages) error {
	return r.db.WithContext(ctx).
		Model(&types.Message{}).
		Where("id = ? AND session_id = ?", messageID, sessionID).
		Update("images", images).Error
}

// UpdateMessageRenderedContent updates only the rendered_content column for a message.
func (r *messageRepository) UpdateMessageRenderedContent(ctx context.Context, sessionID, messageID string, renderedContent string) error {
	return r.db.WithContext(ctx).
		Model(&types.Message{}).
		Where("id = ? AND session_id = ?", messageID, sessionID).
		Update("rendered_content", renderedContent).Error
}

// UpsertMessageFeedback creates the feedback row or replaces its mutable
// fields when the same message is rated again.
func (r *messageRepository) UpsertMessageFeedback(
	ctx context.Context, feedback *types.MessageFeedback,
) (*types.MessageFeedback, error) {
	now := time.Now()
	feedback.UpdatedAt = now
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = now
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"session_id": feedback.SessionID,
			"user_id":    feedback.UserID,
			"rating":     feedback.Rating,
			"reason":     feedback.Reason,
			"comment":    feedback.Comment,
			"updated_at": now,
		}),
	}).Create(feedback).Error
	if err != nil {
		return nil, err
	}

	var stored types.MessageFeedback
	if err := r.db.WithContext(ctx).
		Where("message_id = ? AND session_id = ?", feedback.MessageID, feedback.SessionID).
		First(&stored).Error; err != nil {
		return nil, err
	}
	return &stored, nil
}

// DeleteMessageFeedback removes feedback only within the owning session.
func (r *messageRepository) DeleteMessageFeedback(ctx context.Context, sessionID, messageID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ? AND message_id = ?", sessionID, messageID).
		Delete(&types.MessageFeedback{}).Error
}

// DeleteMessagesBySessionID deletes all messages belonging to a session (soft delete)
func (r *messageRepository) DeleteMessagesBySessionID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&types.Message{}).Error
}
