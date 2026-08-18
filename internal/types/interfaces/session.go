package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/types"
)

// SessionService defines the session service interface
type SessionService interface {
	// CreateSession creates a session
	CreateSession(ctx context.Context, session *types.Session) (*types.Session, error)
	// GetSession gets a session
	GetSession(ctx context.Context, id string) (*types.Session, error)
	// UpdateSession updates a session
	UpdateSession(ctx context.Context, session *types.Session) error
	// UpdateSessionLastRequestState records the input-bar state used for the
	// most recent QA request on this session. Best-effort: callers should log
	// but not surface failures to the user.
	UpdateSessionLastRequestState(ctx context.Context, sessionID string, state *types.SessionLastRequestState) error
	// DeleteSession deletes a session
	DeleteSession(ctx context.Context, id string) error
	// BatchDeleteSessions deletes multiple sessions by IDs
	BatchDeleteSessions(ctx context.Context, ids []string) error
	// DeleteAllSessions deletes all sessions owned by the current user.
	DeleteAllSessions(ctx context.Context) error
	// ListSessions returns a page of sessions for the current user with
	// search/source filters and pin-aware ordering. User scope is pulled from ctx.
	ListSessions(ctx context.Context, query *types.SessionListQuery) (*types.PageResult, error)
	// SetSessionPinned pins or unpins the session for the current user scope.
	// Returns the number of rows affected; 0 signals "not found" to the handler.
	SetSessionPinned(ctx context.Context, sessionID string, pinned bool) (int64, error)
	// GenerateTitle generates a title for the current conversation
	// modelID: optional model ID to use for title generation (if empty, uses first available KnowledgeQA model)
	GenerateTitle(ctx context.Context, session *types.Session, messages []types.Message, modelID string) (string, error)
	// GenerateTitleAsync generates a title for the session asynchronously
	// It emits an event when the title is generated
	// modelID: optional model ID to use for title generation (if empty, uses first available KnowledgeQA model)
	GenerateTitleAsync(ctx context.Context, session *types.Session, userQuery string, modelID string, eventBus *event.EventBus)
	// KnowledgeQA performs knowledge-based question answering.
	// Events are emitted through eventBus (references, answer chunks, completion).
	KnowledgeQA(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error
	// KnowledgeQAByEvent performs knowledge-based question answering by event
	KnowledgeQAByEvent(ctx context.Context, chatManage *types.ChatManage, eventList []types.EventType) error
	// SearchKnowledge performs knowledge-based search, without summarization
	// knowledgeBaseIDs: list of knowledge base IDs to search (supports multi-KB)
	// knowledgeIDs: list of specific knowledge (file) IDs to search
	SearchKnowledge(ctx context.Context, knowledgeBaseIDs []string, knowledgeIDs []string, tagScopes []types.TagScope, query string) ([]*types.SearchResult, error)
	// AgentQA performs agent-based question answering with conversation history and streaming support.
	AgentQA(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error
}

// SessionRepository defines the session repository interface
type SessionRepository interface {
	// Create creates a session
	Create(ctx context.Context, session *types.Session) (*types.Session, error)
	// Get gets a session visible to the user scope.
	Get(ctx context.Context, userID string, id string) (*types.Session, error)
	// GetByID loads a session by id without user scoping. Callers
	// must enforce access (e.g. embed channel + session signature).
	GetByID(ctx context.Context, id string) (*types.Session, error)
	// GetByUserID gets all sessions owned by a user.
	GetByUserID(ctx context.Context, userID string) ([]*types.Session, error)
	// QueryPaged lists sessions with filters, user-scoped ownership and pin-aware ordering.
	QueryPaged(ctx context.Context, q *types.SessionListQuery) ([]*types.SessionListItem, int64, error)
	// Update updates a session visible to the knowledgeDomain/user scope.
	Update(ctx context.Context, session *types.Session, userID string) (int64, error)
	// SetOwnerID assigns sessions.user_id for a row.
	SetOwnerID(ctx context.Context, id, ownerID string) (int64, error)
	// UpdateLastRequestState persists the most recent input-bar state for a
	// session (agent, model, request scope, etc.) so the chat UI can restore it
	// when the session is reopened. Scope rules match Update.
	UpdateLastRequestState(ctx context.Context, userID string, sessionID string, state *types.SessionLastRequestState) (int64, error)
	// SetPinned pins or unpins a user-owned session row.
	// userID, when non-empty, is enforced so users cannot pin sessions they don't own.
	// Returns the number of rows affected; 0 means the session doesn't exist or is
	// not visible to this caller.
	SetPinned(ctx context.Context, userID string, id string, pinned bool) (int64, error)
	// Delete deletes a user-owned session.
	Delete(ctx context.Context, userID string, id string) (int64, error)
	// BatchDelete deletes multiple user-owned sessions.
	BatchDelete(ctx context.Context, userID string, ids []string) (int64, error)
	// DeleteAllByUserID deletes all sessions owned by a user.
	DeleteAllByUserID(ctx context.Context, userID string) (int64, error)
}
