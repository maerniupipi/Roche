package interfaces

import (
	"context"
	"time"

	"roche.local/knowledge-agent-platform/internal/types"
)

// MessageService defines the message service interface
type MessageService interface {
	// CreateMessage creates a message
	CreateMessage(ctx context.Context, message *types.Message) (*types.Message, error)

	// GetMessage gets a message
	GetMessage(ctx context.Context, sessionID string, id string) (*types.Message, error)

	// GetMessagesBySession gets all messages of a session
	GetMessagesBySession(ctx context.Context, sessionID string, page int, pageSize int) ([]*types.Message, error)

	// GetRecentMessagesBySession gets recent messages of a session
	GetRecentMessagesBySession(ctx context.Context, sessionID string, limit int) ([]*types.Message, error)

	// GetMessagesBySessionBeforeTime gets messages before a specific time of a session
	GetMessagesBySessionBeforeTime(
		ctx context.Context, sessionID string, beforeTime time.Time, limit int,
	) ([]*types.Message, error)

	// UpdateMessage updates a message
	UpdateMessage(ctx context.Context, message *types.Message) error

	// UpdateMessageImages updates only the images JSONB column for a message.
	UpdateMessageImages(ctx context.Context, sessionID, messageID string, images types.MessageImages) error

	// UpdateMessageRenderedContent updates the rendered_content column for a user message.
	UpdateMessageRenderedContent(ctx context.Context, sessionID, messageID string, renderedContent string) error

	// SetMessageFeedback creates or replaces the current user's feedback for an assistant message.
	SetMessageFeedback(ctx context.Context, sessionID, messageID string, feedback *types.MessageFeedback) (*types.MessageFeedback, error)

	// DeleteMessageFeedback removes the current feedback for an assistant message.
	DeleteMessageFeedback(ctx context.Context, sessionID, messageID string) error

	// DeleteMessage deletes a message
	DeleteMessage(ctx context.Context, sessionID string, id string) error

	// ClearSessionMessages deletes all messages in a session
	ClearSessionMessages(ctx context.Context, sessionID string) error
}

// MessageRepository defines the message repository interface
type MessageRepository interface {
	// CreateMessage creates a message
	CreateMessage(ctx context.Context, message *types.Message) (*types.Message, error)
	// GetMessage gets a message
	GetMessage(ctx context.Context, sessionID string, id string) (*types.Message, error)
	// GetMessagesBySession gets all messages of a session
	GetMessagesBySession(ctx context.Context, sessionID string, page int, pageSize int) ([]*types.Message, error)
	// GetRecentMessagesBySession gets recent messages of a session
	GetRecentMessagesBySession(ctx context.Context, sessionID string, limit int) ([]*types.Message, error)
	// GetMessagesBySessionBeforeTime gets messages before a specific time of a session
	GetMessagesBySessionBeforeTime(
		ctx context.Context, sessionID string, beforeTime time.Time, limit int,
	) ([]*types.Message, error)
	// UpdateMessage updates a message
	UpdateMessage(ctx context.Context, message *types.Message) error
	// UpdateMessageImages updates only the images JSONB column for a message
	UpdateMessageImages(ctx context.Context, sessionID, messageID string, images types.MessageImages) error
	// UpdateMessageRenderedContent updates the rendered_content column for a user message
	UpdateMessageRenderedContent(ctx context.Context, sessionID, messageID string, renderedContent string) error
	// UpsertMessageFeedback creates or replaces feedback for a message.
	UpsertMessageFeedback(ctx context.Context, feedback *types.MessageFeedback) (*types.MessageFeedback, error)
	// DeleteMessageFeedback deletes feedback scoped to its message and session.
	DeleteMessageFeedback(ctx context.Context, sessionID, messageID string) error
	// DeleteMessage deletes a message
	DeleteMessage(ctx context.Context, sessionID string, id string) error
	// DeleteMessagesBySessionID deletes all messages belonging to a session
	DeleteMessagesBySessionID(ctx context.Context, sessionID string) error
	// GetFirstMessageOfUser gets the first message of a user
	GetFirstMessageOfUser(ctx context.Context, sessionID string) (*types.Message, error)
}
