package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/application/service/unifiedqa"
	"roche.local/knowledge-agent-platform/internal/config"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"

	chatpipeline "roche.local/knowledge-agent-platform/internal/application/service/chat_pipeline"
)

func sessionUserIDFromContext(ctx context.Context) string {
	return types.SessionOwnerIDFromContext(ctx)
}

// generateEventID generates a unique event ID with type suffix for better traceability
func generateEventID(suffix string) string {
	return fmt.Sprintf("%s-%s", uuid.New().String()[:8], suffix)
}

// sessionService implements the SessionService interface for managing conversation sessions.
// History for multi-turn conversations is rebuilt from the messages table on demand
// (see service.LoadAgentHistory and chat_pipeline history loading) — there is no
// separate cross-turn cache layer.
type sessionService struct {
	cfg                    *config.Config                         // Application configuration
	sessionRepo            interfaces.SessionRepository           // Repository for session data
	messageRepo            interfaces.MessageRepository           // Repository for message data
	knowledgeBaseService   interfaces.KnowledgeBaseService        // Service for knowledge base operations
	modelService           interfaces.ModelService                // Service for model operations
	knowledgeDomainService interfaces.KnowledgeDomainService      // Knowledge-domain resource settings
	eventManager           *chatpipeline.EventManager             // Event manager for chat pipeline
	agentService           interfaces.AgentService                // Service for agent operations
	knowledgeService       interfaces.KnowledgeService            // Service for knowledge operations
	chunkService           interfaces.ChunkService                // Service for chunk operations
	webSearchStateRepo     interfaces.WebSearchStateService       // Service for web search state
	webSearchProviderRepo  interfaces.WebSearchProviderRepository // Repository for web search provider entities
	memoryService          interfaces.MemoryService               // Service for memory operations
	accessService          interfaces.EnterpriseAccessService
	unifiedQAService       unifiedqa.Executor
}

// NewSessionService creates a new session service instance with all required dependencies
func NewSessionService(cfg *config.Config,
	sessionRepo interfaces.SessionRepository,
	messageRepo interfaces.MessageRepository,
	knowledgeBaseService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
	chunkService interfaces.ChunkService,
	modelService interfaces.ModelService,
	knowledgeDomainService interfaces.KnowledgeDomainService,
	eventManager *chatpipeline.EventManager,
	agentService interfaces.AgentService,
	webSearchStateRepo interfaces.WebSearchStateService,
	webSearchProviderRepo interfaces.WebSearchProviderRepository,
	memoryService interfaces.MemoryService,
	accessService interfaces.EnterpriseAccessService,
	unifiedQAService unifiedqa.Executor,
) interfaces.SessionService {
	return &sessionService{
		cfg:                    cfg,
		sessionRepo:            sessionRepo,
		messageRepo:            messageRepo,
		knowledgeBaseService:   knowledgeBaseService,
		knowledgeService:       knowledgeService,
		chunkService:           chunkService,
		modelService:           modelService,
		knowledgeDomainService: knowledgeDomainService,
		eventManager:           eventManager,
		agentService:           agentService,
		webSearchStateRepo:     webSearchStateRepo,
		webSearchProviderRepo:  webSearchProviderRepo,
		memoryService:          memoryService,
		accessService:          accessService,
		unifiedQAService:       unifiedQAService,
	}
}

// CreateSession creates a new conversation session
func (s *sessionService) CreateSession(ctx context.Context, session *types.Session) (*types.Session, error) {
	logger.Info(ctx, "Start creating session")

	if session.UserID == "" {
		session.UserID = sessionUserIDFromContext(ctx)
	}
	if session.UserID == "" {
		return nil, stderrors.New("session owner is required")
	}

	// Create session in repository
	createdSession, err := s.sessionRepo.Create(ctx, session)
	if err != nil {
		return nil, err
	}

	logger.Infof(ctx, "Session created successfully, ID: %s, owner: %s", createdSession.ID, createdSession.UserID)
	return createdSession, nil
}

// GetSession retrieves a session by its ID
func (s *sessionService) GetSession(ctx context.Context, id string) (*types.Session, error) {
	logger.Info(ctx, "Start retrieving session")

	// Validate session ID
	if id == "" {
		logger.Error(ctx, "Failed to get session: session ID cannot be empty")
		return nil, stderrors.New("session id is required")
	}

	userID := sessionUserIDFromContext(ctx)
	logger.Infof(ctx, "Retrieving session, ID: %s, owner: %s", id, userID)

	// Get session from repository
	session, err := s.sessionRepo.Get(ctx, userID, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": id})
		return nil, err
	}

	logger.Infof(ctx, "Session retrieved successfully, ID: %s", session.ID)
	return session, nil
}

// ListSessions returns a page of sessions with search/source filters, scoped to
// the current user.
func (s *sessionService) ListSessions(
	ctx context.Context, query *types.SessionListQuery,
) (*types.PageResult, error) {
	if query == nil {
		query = &types.SessionListQuery{}
	}
	if uid := types.SessionOwnerIDFromContext(ctx); uid != "" {
		query.UserID = uid
	}

	items, total, err := s.sessionRepo.QueryPaged(ctx, query)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"user_id": query.UserID, "keyword": query.Keyword})
		return nil, err
	}

	pagination := &types.Pagination{Page: query.Page, PageSize: query.PageSize}
	return types.NewPageResult(total, pagination, items), nil
}

// SetSessionPinned pins or unpins a session for the current user scope.
// Returns the number of rows affected; 0 means the session doesn't exist
// or is not owned by the caller so the handler can respond 404.
func (s *sessionService) SetSessionPinned(
	ctx context.Context, sessionID string, pinned bool,
) (int64, error) {
	if sessionID == "" {
		return 0, stderrors.New("session id is required")
	}
	userID := sessionUserIDFromContext(ctx)
	return s.sessionRepo.SetPinned(ctx, userID, sessionID, pinned)
}

// UpdateSession updates an existing session's properties
func (s *sessionService) UpdateSession(ctx context.Context, session *types.Session) error {
	// Validate session ID
	if session.ID == "" {
		logger.Error(ctx, "Failed to update session: session ID cannot be empty")
		return stderrors.New("session id is required")
	}

	// Update session in repository
	userID := sessionUserIDFromContext(ctx)
	if _, err := s.sessionRepo.Get(ctx, userID, session.ID); err != nil {
		return err
	}

	_, err := s.sessionRepo.Update(ctx, session, userID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": session.ID})
		return err
	}

	logger.Infof(ctx, "Session updated successfully, ID: %s", session.ID)
	return nil
}

// UpdateSessionLastRequestState persists the input-bar state used by the most
// recent QA request on this session. Called from the QA handler after a
// request is accepted so the UI can rehydrate the same settings on reopen.
// Best-effort: scope mismatches are logged and swallowed — failing to record
// the UI memo should never fail the user's chat request.
func (s *sessionService) UpdateSessionLastRequestState(
	ctx context.Context, sessionID string, state *types.SessionLastRequestState,
) error {
	if sessionID == "" {
		return stderrors.New("session id is required")
	}
	userID := sessionUserIDFromContext(ctx)
	affected, err := s.sessionRepo.UpdateLastRequestState(ctx, userID, sessionID, state)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": sessionID})
		return err
	}
	if affected == 0 {
		logger.Warnf(ctx, "UpdateSessionLastRequestState: no rows affected for session %s", sessionID)
	}
	return nil
}

// DeleteSession removes a session by its ID
func (s *sessionService) DeleteSession(ctx context.Context, id string) error {
	// Validate session ID
	if id == "" {
		logger.Error(ctx, "Failed to delete session: session ID cannot be empty")
		return stderrors.New("session id is required")
	}

	userID := sessionUserIDFromContext(ctx)

	if _, err := s.sessionRepo.Get(ctx, userID, id); err != nil {
		return err
	}

	// Cleanup temporary KB stored in Redis for this session
	if err := s.webSearchStateRepo.DeleteWebSearchTempKBState(ctx, id); err != nil {
		logger.Warnf(ctx, "Failed to cleanup temporary KB for session %s: %v", id, err)
	}

	// Delete session from repository
	rows, err := s.sessionRepo.Delete(ctx, userID, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": id})
		return err
	}
	if rows == 0 {
		return apperrors.ErrSessionNotFound
	}

	return nil
}

// BatchDeleteSessions deletes multiple sessions by IDs
func (s *sessionService) BatchDeleteSessions(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		logger.Error(ctx, "Failed to batch delete sessions: IDs list is empty")
		return stderrors.New("session ids are required")
	}

	userID := sessionUserIDFromContext(ctx)

	visibleIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := s.sessionRepo.Get(ctx, userID, id); err == nil {
			visibleIDs = append(visibleIDs, id)
		} else if !stderrors.Is(err, apperrors.ErrSessionNotFound) {
			return err
		}
	}
	if len(visibleIDs) == 0 {
		return apperrors.ErrSessionNotFound
	}

	// Cleanup associated resources for each session
	for _, id := range visibleIDs {
		if err := s.webSearchStateRepo.DeleteWebSearchTempKBState(ctx, id); err != nil {
			logger.Warnf(ctx, "Failed to cleanup temporary KB for session %s: %v", id, err)
		}
	}

	// Batch delete sessions from repository
	if _, err := s.sessionRepo.BatchDelete(ctx, userID, visibleIDs); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_ids": visibleIDs})
		return err
	}

	return nil
}

// DeleteAllSessions deletes all sessions owned by the current user.
func (s *sessionService) DeleteAllSessions(ctx context.Context) error {
	userID := sessionUserIDFromContext(ctx)
	logger.Infof(ctx, "Deleting all sessions for user %s", userID)

	sessions, err := s.sessionRepo.GetByUserID(ctx, userID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list sessions for cleanup: %v", err)
	} else {
		for _, session := range sessions {
			if err := s.webSearchStateRepo.DeleteWebSearchTempKBState(ctx, session.ID); err != nil {
				logger.Warnf(ctx, "Failed to cleanup temporary KB for session %s: %v", session.ID, err)
			}
		}
	}

	if _, err := s.sessionRepo.DeleteAllByUserID(ctx, userID); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"user_id": userID})
		return err
	}

	logger.Infof(ctx, "All sessions deleted for user %s", userID)
	return nil
}

// GenerateTitle generates a title for the current conversation content
// modelID: optional model ID to use for title generation (if empty, uses first available KnowledgeQA model)
func (s *sessionService) GenerateTitle(ctx context.Context,
	session *types.Session, messages []types.Message, modelID string,
) (string, error) {
	if session == nil {
		logger.Error(ctx, "Failed to generate title: session cannot be empty")
		return "", stderrors.New("session cannot be empty")
	}

	// Skip if title already exists
	if session.Title != "" {
		return session.Title, nil
	}
	var err error
	// Get the first user message, either from provided messages or repository
	var message *types.Message
	if len(messages) == 0 {
		message, err = s.messageRepo.GetFirstMessageOfUser(ctx, session.ID)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"session_id": session.ID,
			})
			return "", err
		}
	} else {
		for _, m := range messages {
			if m.Role == "user" {
				message = &m
				break
			}
		}
	}

	// Ensure a user message was found
	if message == nil {
		logger.Error(ctx, "No user message found, cannot generate title")
		return "", stderrors.New("no user message found")
	}

	// Use provided modelID, or fallback to first available KnowledgeQA model
	if modelID == "" {
		models, err := s.modelService.ListModels(ctx)
		if err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			return "", fmt.Errorf("failed to list models: %w", err)
		}
		for _, model := range models {
			if model == nil {
				continue
			}
			if model.Type == types.ModelTypeKnowledgeQA {
				modelID = model.ID
				logger.Infof(ctx, "Using first available KnowledgeQA model for title: %s", modelID)
				break
			}
		}
		if modelID == "" {
			logger.Error(ctx, "No KnowledgeQA model found")
			return "", stderrors.New("no KnowledgeQA model available for title generation")
		}
	} else {
		logger.Infof(ctx, "Using specified model for title generation: %s", modelID)
	}

	chatModel, err := s.modelService.GetChatModel(ctx, modelID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": modelID,
		})
		return "", err
	}

	// Prepare messages for title generation
	titlePrompt := types.RenderPromptPlaceholders(s.cfg.Conversation.GenerateSessionTitlePrompt, types.PlaceholderValues{
		"language": types.LanguageNameFromContext(ctx),
	})
	var chatMessages []chat.Message
	chatMessages = append(chatMessages,
		chat.Message{Role: "system", Content: titlePrompt},
	)
	chatMessages = append(chatMessages,
		chat.Message{Role: "user", Content: message.Content},
	)

	// Call model to generate title
	thinking := false
	response, err := chatModel.Chat(ctx, chatMessages, &chat.ChatOptions{
		Temperature: 0.3,
		Thinking:    &thinking,
	})
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return "", err
	}

	// Process and store the generated title
	session.Title = strings.TrimPrefix(response.Content, "<think>\n\n</think>")

	// Update session with new title
	_, err = s.sessionRepo.Update(ctx, session, session.UserID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return "", err
	}

	return session.Title, nil
}

// GenerateTitleAsync generates a title for the session asynchronously
// This method clones the session and generates the title in a goroutine
// It emits an event when the title is generated
// modelID: optional model ID to use for title generation (if empty, uses first available KnowledgeQA model)
func (s *sessionService) GenerateTitleAsync(
	ctx context.Context,
	session *types.Session,
	userQuery string,
	modelID string,
	eventBus *event.EventBus,
) {
	requestID := ctx.Value(types.RequestIDContextKey)
	language := ctx.Value(types.LanguageContextKey)
	// Keep the Langfuse trace handle so the async title generation shows up
	// as a child of the same trace as the originating chat request.
	langfuseTrace := ctx.Value(types.LangfuseTraceContextKey)
	go func() {
		bgCtx := context.Background()
		if requestID != nil {
			bgCtx = context.WithValue(bgCtx, types.RequestIDContextKey, requestID)
		}
		if language != nil {
			bgCtx = context.WithValue(bgCtx, types.LanguageContextKey, language)
		}
		if langfuseTrace != nil {
			bgCtx = context.WithValue(bgCtx, types.LangfuseTraceContextKey, langfuseTrace)
		}

		// Skip if title already exists
		if session.Title != "" {
			return
		}

		// Generate title using the first user message
		messages := []types.Message{
			{
				Role:    "user",
				Content: userQuery,
			},
		}

		title, err := s.GenerateTitle(bgCtx, session, messages, modelID)
		if err != nil {
			logger.ErrorWithFields(bgCtx, err, map[string]interface{}{
				"session_id": session.ID,
			})
			return
		}

		// Emit title update event - BUG FIX: use bgCtx instead of ctx
		// The original ctx is from the HTTP request and may be cancelled by the time we get here
		if eventBus != nil {
			if err := eventBus.Emit(bgCtx, event.Event{
				Type:      event.EventSessionTitle,
				SessionID: session.ID,
				Data: event.SessionTitleData{
					SessionID: session.ID,
					Title:     title,
				},
			}); err != nil {
				logger.ErrorWithFields(bgCtx, err, map[string]interface{}{
					"session_id": session.ID,
				})
			} else {
				logger.Infof(bgCtx, "Title update event emitted successfully, session ID: %s, title: %s", session.ID, title)
			}
		}
	}()
}
