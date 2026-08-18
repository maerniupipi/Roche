package session

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// qaRequestContext holds all the common data needed for QA requests
type qaRequestContext struct {
	ctx              context.Context
	c                *gin.Context
	sessionID        string
	requestID        string
	receivedAt       time.Time // Wall-clock time the handler started processing the request
	query            string
	session          *types.Session
	customAgent      *types.CustomAgent
	assistantMessage *types.Message
	knowledgeBaseIDs []string
	knowledgeIDs     []string
	tagScopes        []types.TagScope
	tagIDs           []string
	mcpServiceIDs    []string
	skillNames       []string
	summaryModelID   string
	webSearchEnabled bool
	enableMemory     bool // Whether memory feature is enabled
	mentionedItems   types.MentionedItems
	images           []ImageAttachment        // Uploaded images with analysis text
	userMessageID    string                   // Created user message ID (populated after createUserMessage)
	channel          string                   // Client channel: "web" or "app".
	attachments      types.MessageAttachments // Processed file attachments

	// Snapshot of the request fields needed to persist the input-bar state
	// for session restoration. Kept verbatim from the request so we record
	// what the user had selected on the UI (not server-side resolutions).
	reqAgentEnabled      bool
	reqAgentID           string
	fixedSuggestedAnswer string
}

// chatStorageNamespaceID creates a stable object-storage namespace for one
// authenticated user. Chat uploads are session resources, not knowledge-domain
// resources, so they must never depend on an active knowledgeDomain.
func chatStorageNamespaceID(ctx context.Context) uint64 {
	userID, _ := types.UserIDFromContext(ctx)
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte("chat:" + userID))
	return hasher.Sum64() | (uint64(1) << 63)
}

// buildQARequest converts the qaRequestContext into a types.QARequest for service invocation.
func (rc *qaRequestContext) buildQARequest() *types.QARequest {
	imageURLs, imageDescription := extractImageURLsAndOCRText(rc.images)
	return &types.QARequest{
		Session:            rc.session,
		Query:              rc.query,
		AssistantMessageID: rc.assistantMessage.ID,
		SummaryModelID:     rc.summaryModelID,
		CustomAgent:        rc.customAgent,
		KnowledgeBaseIDs:   rc.knowledgeBaseIDs,
		KnowledgeIDs:       rc.knowledgeIDs,
		TagScopes:          rc.tagScopes,
		MCPServiceIDs:      rc.mcpServiceIDs,
		SkillNames:         rc.skillNames,
		ImageURLs:          imageURLs,
		ImageDescription:   imageDescription,
		UserMessageID:      rc.userMessageID,
		WebSearchEnabled:   rc.webSearchEnabled,
		EnableMemory:       rc.enableMemory,
		Attachments:        rc.attachments,
	}
}

// parseQARequest parses and validates a QA request, returns the request context
func (h *Handler) parseQARequest(c *gin.Context, logPrefix string) (*qaRequestContext, *CreateKnowledgeQARequest, error) {
	receivedAt := time.Now()
	ctx := logger.CloneContext(c.Request.Context())
	requestID := secutils.SanitizeForLog(c.GetString(types.RequestIDContextKey.String()))
	logger.Infof(ctx, "[%s] TTFB:start request_id=%s received_at=%d",
		logPrefix, requestID, receivedAt.UnixMilli())

	// Get session ID from URL parameter
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	if sessionID == "" {
		logger.Error(ctx, "Session ID is empty")
		return nil, nil, errors.NewBadRequestError(errors.ErrInvalidSessionID.Error())
	}

	// Parse request body
	var request CreateKnowledgeQARequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request data", err)
		return nil, nil, errors.NewBadRequestError(err.Error())
	}

	// Validate query content
	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		return nil, nil, errors.NewBadRequestError("Query content cannot be empty")
	}

	// SSRF protection: strip client-supplied URL/Caption fields from image attachments.
	// The URL field must only be populated server-side by saveImageAttachments; an
	// attacker could inject internal network URLs to trigger SSRF via the LLM provider.
	for i := range request.Images {
		request.Images[i].URL = ""
		request.Images[i].Caption = ""
	}

	// Log request details
	if requestJSON, err := json.Marshal(request); err == nil {
		logger.Infof(ctx, "[%s] Request: session_id=%s, request=%s",
			logPrefix, sessionID, secutils.SanitizeForLog(secutils.CompactImageDataURLForLog(string(requestJSON))))
	}

	// Get session
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session, session ID: %s, error: %v", sessionID, err)
		return nil, nil, errors.NewNotFoundError("Session not found")
	}

	customAgent := h.resolveAgent(ctx, request.AgentID)

	// Merge @mentioned items into knowledge_base_ids and knowledge_ids
	kbIDs, knowledgeIDs := mergeKnowledgeTargets(request.KnowledgeBaseIDs, request.KnowledgeIds, request.MentionedItems)

	// Log merge results for debugging
	logger.Infof(ctx, "[%s] @mention merge: request.KnowledgeBaseIDs=%v, request.MentionedItems=%d, merged kbIDs=%v, merged knowledgeIDs=%v",
		logPrefix, request.KnowledgeBaseIDs, len(request.MentionedItems), kbIDs, knowledgeIDs)

	// Process inline base64 images: decode and save to storage.
	// VLM analysis for RAG paths is deferred to the pipeline rewrite step.
	// For pure chat paths with non-vision models, VLM analysis runs here as fallback.
	if len(request.Images) > 0 {
		if customAgent == nil || !customAgent.Config.ImageUploadEnabled {
			logger.Warnf(ctx, "[%s] Image upload is not enabled for this agent, rejecting %d images", logPrefix, len(request.Images))
			return nil, nil, errors.NewBadRequestError("Image upload is not enabled for this agent")
		}
		storageNamespaceID := chatStorageNamespaceID(ctx)
		if err := h.saveImageAttachments(ctx, request.Images, storageNamespaceID); err != nil {
			logger.Errorf(ctx, "[%s] Failed to save images: %v", logPrefix, err)
			return nil, nil, errors.NewBadRequestError(fmt.Sprintf("Image save failed: %v", err))
		}

		// VLM analysis is always deferred to after SSE stream is up:
		// - Agent mode: runs in async execution flow with tool_call/tool_result events
		// - Normal RAG mode: runs in the pipeline rewrite step with progress events
		// - Normal pure-chat mode: runs in the async goroutine with progress events
	}

	// Process file attachments: decode and save to storage, extract content
	var processedAttachments types.MessageAttachments
	if len(request.AttachmentUploads) > 0 {
		logger.Infof(ctx, "[%s] processing %d attachment(s)", logPrefix, len(request.AttachmentUploads))

		// MAX_FILE_SIZE_MB env (50MB default). See utils/filesize.go for
		// why this is deploy-time-only rather than a runtime setting.
		maxSizeMB := secutils.GetMaxFileSizeMB()
		maxSize := maxSizeMB * 1024 * 1024
		for i, upload := range request.AttachmentUploads {
			if upload.FileSize > maxSize {
				return nil, nil, errors.NewBadRequestError(
					fmt.Sprintf("attachment %d exceeds size limit of %dMB", i+1, maxSizeMB))
			}
		}

		storageNamespaceID := chatStorageNamespaceID(ctx)

		// Use ASR only when the agent has audio upload enabled.
		asrModelID := ""
		if customAgent != nil && customAgent.Config.AudioUploadEnabled && customAgent.Config.ASRModelID != "" {
			asrModelID = customAgent.Config.ASRModelID
		}

		// Process all attachments concurrently.
		processedAttachments = make(types.MessageAttachments, len(request.AttachmentUploads))
		var wg sync.WaitGroup
		errChan := make(chan error, len(request.AttachmentUploads))

		for i, upload := range request.AttachmentUploads {
			wg.Add(1)
			go func(idx int, att AttachmentUpload) {
				defer wg.Done()

				data, err := DecodeBase64Attachment(att.Data)
				if err != nil {
					errChan <- fmt.Errorf("attachment %d decode failed: %w", idx+1, err)
					return
				}

				processed, err := h.attachmentProcessor.ProcessAttachment(
					ctx, data, att.FileName, att.FileSize, storageNamespaceID, asrModelID,
				)
				if err != nil {
					errChan <- fmt.Errorf("attachment %d processing failed: %w", idx+1, err)
					return
				}

				processedAttachments[idx] = *processed
			}(i, upload)
		}

		wg.Wait()
		close(errChan)

		if len(errChan) > 0 {
			err := <-errChan
			logger.Errorf(ctx, "[%s] attachment processing failed: %v", logPrefix, err)
			return nil, nil, errors.NewBadRequestError(fmt.Sprintf("attachment processing failed: %v", err))
		}

		logger.Infof(ctx, "[%s] all attachments processed", logPrefix)
	}

	// Resolve enable_memory:
	//   1. Explicit value in request → honour it. Used by embedded mode
	//      (force false) and by older clients still sending the literal bool.
	//   2. Not set → fall back to the calling user's stored preference.
	//      The toggle is persisted server-side per user (see PUT
	//      /auth/me/preferences); this is the canonical path for the
	//      normal logged-in web UI now that it no longer sends the field.
	//   3. No user / no preference → false. API-key-only callers never
	//      had memory enabled in practice, keep that behaviour.
	enableMemory := h.resolveEnableMemory(ctx, request.EnableMemory)

	mentionScopes := tagScopesFromMentionedItems(request.MentionedItems)
	requestTagIDs := dedupRequestStrings(request.TagIDs)
	if err := validateUnscopedTagIDs(orphanTagIDsForScope(requestTagIDs, mentionScopes), secutils.SanitizeForLogArray(kbIDs)); err != nil {
		return nil, nil, errors.NewBadRequestError(err.Error())
	}
	tagScopes := mergeTagScopesFromRequestIDs(mentionScopes, requestTagIDs, secutils.SanitizeForLogArray(kbIDs))
	tagIDs := dedupRequestStrings(append(request.TagIDs, mentionedIDsByType(request.MentionedItems, "tag")...))
	mcpServiceIDs := dedupRequestStrings(append(request.MCPServiceIDs, mentionedIDsByType(request.MentionedItems, "mcp")...))
	skillNames := dedupRequestStrings(append(request.SkillNames, mentionedIDsByType(request.MentionedItems, "skill")...))

	// Build request context
	reqCtx := &qaRequestContext{
		ctx:         ctx,
		c:           c,
		sessionID:   sessionID,
		requestID:   requestID,
		receivedAt:  receivedAt,
		query:       request.Query,
		session:     session,
		customAgent: customAgent,
		assistantMessage: &types.Message{
			SessionID:   sessionID,
			Role:        "assistant",
			RequestID:   c.GetString(types.RequestIDContextKey.String()),
			IsCompleted: false,
			Channel:     request.Channel,
		},
		knowledgeBaseIDs: secutils.SanitizeForLogArray(kbIDs),
		knowledgeIDs:     secutils.SanitizeForLogArray(knowledgeIDs),
		tagScopes:        tagScopes,
		tagIDs:           secutils.SanitizeForLogArray(tagIDs),
		mcpServiceIDs:    secutils.SanitizeForLogArray(mcpServiceIDs),
		skillNames:       secutils.SanitizeForLogArray(skillNames),
		summaryModelID:   secutils.SanitizeForLog(request.SummaryModelID),
		webSearchEnabled: request.WebSearchEnabled,
		enableMemory:     enableMemory,
		mentionedItems:   convertMentionedItems(request.MentionedItems),
		images:           request.Images,
		channel:          request.Channel,
		attachments:      processedAttachments,
		reqAgentEnabled:  request.AgentEnabled,
		reqAgentID:       request.AgentID,
	}

	return reqCtx, &request, nil
}

// resolveEnableMemory decides whether the memory pipeline runs for this
// request. See the call-site comment in parseQARequest for the resolution
// order. Lookup errors are logged but never propagate — a failure to read
// the user's preference shouldn't break the chat request itself, we just
// fall back to false (the safe default).
func (h *Handler) resolveEnableMemory(ctx context.Context, override *bool) bool {
	if override != nil {
		return *override
	}
	if h.userService == nil {
		return false
	}
	user, err := h.userService.GetCurrentUser(ctx)
	if err != nil {
		// API-key-only callers or revoked sessions land here; the chat
		// request itself stays authorised via the middleware that already
		// ran, we just have nobody to look preferences up for.
		logger.Debugf(ctx, "enable_memory: no user in context, defaulting to false: %v", err)
		return false
	}
	if user.Preferences.EnableMemory != nil {
		return *user.Preferences.EnableMemory
	}
	return false
}

// resolveAgent resolves a platform agent for the authenticated caller.
func (h *Handler) resolveAgent(ctx context.Context, agentID string) *types.CustomAgent {
	if agentID == "" {
		return nil
	}

	logger.Infof(ctx, "Resolving agent, agent ID: %s", secutils.SanitizeForLog(agentID))

	customAgent, err := h.customAgentService.GetAgentByID(ctx, agentID)
	if err != nil {
		logger.Warnf(ctx, "Failed to get custom agent, agent ID: %s, error: %v, using default config",
			secutils.SanitizeForLog(agentID), err)
		return nil
	}
	logger.Infof(ctx, "Using agent: ID=%s, Name=%s, AgentMode=%s",
		customAgent.ID, customAgent.Name, customAgent.Config.AgentMode)
	return customAgent
}

// mergeKnowledgeTargets merges request KB/knowledge IDs with @mentioned items into deduplicated slices.
func mergeKnowledgeTargets(requestKBIDs []string, requestKnowledgeIDs []string, mentionedItems []MentionedItemRequest) (kbIDs []string, knowledgeIDs []string) {
	kbIDSet := make(map[string]bool)
	kbIDs = make([]string, 0, len(requestKBIDs)+len(mentionedItems))
	for _, id := range requestKBIDs {
		if id != "" && !kbIDSet[id] {
			kbIDs = append(kbIDs, id)
			kbIDSet[id] = true
		}
	}

	knowledgeIDSet := make(map[string]bool)
	knowledgeIDs = make([]string, 0, len(requestKnowledgeIDs)+len(mentionedItems))
	for _, id := range requestKnowledgeIDs {
		if id != "" && !knowledgeIDSet[id] {
			knowledgeIDs = append(knowledgeIDs, id)
			knowledgeIDSet[id] = true
		}
	}

	for _, item := range mentionedItems {
		if item.ID == "" {
			continue
		}
		switch item.Type {
		case "kb":
			if !kbIDSet[item.ID] {
				kbIDs = append(kbIDs, item.ID)
				kbIDSet[item.ID] = true
			}
		case "file":
			if !knowledgeIDSet[item.ID] {
				knowledgeIDs = append(knowledgeIDs, item.ID)
				knowledgeIDSet[item.ID] = true
			}
		}
	}
	return kbIDs, knowledgeIDs
}

// sseStreamContext holds the context for SSE streaming
type sseStreamContext struct {
	eventBus         *event.EventBus
	asyncCtx         context.Context
	cancel           context.CancelFunc
	assistantMessage *types.Message
}

// setupSSEStream sets up the SSE streaming context
func (h *Handler) setupSSEStream(reqCtx *qaRequestContext, generateTitle bool) *sseStreamContext {
	// Set SSE headers
	setSSEHeaders(reqCtx.c)

	// Write initial agent_query event
	h.writeAgentQueryEvent(reqCtx.ctx, reqCtx.sessionID, reqCtx.assistantMessage.ID)

	// Create EventBus and cancellable context
	eventBus := event.NewEventBus()
	asyncCtx, cancel := context.WithCancel(logger.CloneContext(reqCtx.ctx))

	streamCtx := &sseStreamContext{
		eventBus:         eventBus,
		asyncCtx:         asyncCtx,
		cancel:           cancel,
		assistantMessage: reqCtx.assistantMessage,
	}

	// Setup stop event handler
	h.setupStopEventHandler(eventBus, reqCtx.sessionID, reqCtx.assistantMessage, cancel)

	// Watch for stop events independently of the client SSE connection so a
	// user-requested stop reliably cancels generation even when the client
	// has already disconnected (e.g. API-Key callers that close the stream
	// before POSTing /stop). The watcher self-terminates on a terminal stream
	// event, so its lifetime is decoupled from when the QA service call
	// returns (KnowledgeQA returns immediately while streaming continues in a
	// background goroutine, whereas AgentQA blocks until done). Use a
	// connection-independent context derived from baseCtx so it survives the
	// client disconnect.
	h.startStopWatcher(logger.CloneContext(reqCtx.ctx), reqCtx.sessionID, reqCtx.assistantMessage.ID, eventBus)

	// Setup stream handler
	h.setupStreamHandler(asyncCtx, reqCtx.sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, reqCtx.receivedAt, reqCtx.assistantMessage, eventBus)

	// Generate title if needed
	if generateTitle && reqCtx.session.Title == "" {
		// Use the same model as the conversation for title generation
		modelID := ""
		if reqCtx.customAgent != nil && reqCtx.customAgent.Config.ModelID != "" {
			modelID = reqCtx.customAgent.Config.ModelID
		}
		logger.Infof(reqCtx.ctx, "Session has no title, starting async title generation, session ID: %s, model: %s", reqCtx.sessionID, modelID)
		h.sessionService.GenerateTitleAsync(asyncCtx, reqCtx.session, reqCtx.query, modelID, eventBus)
	}

	return streamCtx
}

// SearchKnowledge godoc
// @Summary      知识搜索
// @Description  在知识库中搜索（不使用LLM总结）
// @Tags         问答
// @Accept       json
// @Produce      json
// @Param        request  body      SearchKnowledgeRequest  true  "搜索请求"
// @Success      200      {object}  map[string]interface{}  "搜索结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-search [post]
func (h *Handler) SearchKnowledge(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	logger.Info(ctx, "Start processing knowledge search request")

	// Parse request body
	var request SearchKnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request data", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	// Validate request parameters
	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		c.Error(errors.NewBadRequestError("Query content cannot be empty"))
		return
	}

	// Merge single knowledge_base_id into knowledge_base_ids for backward compatibility
	knowledgeBaseIDs := request.KnowledgeBaseIDs
	if request.KnowledgeBaseID != "" {
		// Check if it's already in the list to avoid duplicates
		found := false
		for _, id := range knowledgeBaseIDs {
			if id == request.KnowledgeBaseID {
				found = true
				break
			}
		}
		if !found {
			knowledgeBaseIDs = append(knowledgeBaseIDs, request.KnowledgeBaseID)
		}
	}

	mentionScopes := tagScopesFromMentionedItems(request.MentionedItems)
	requestTagIDs := dedupRequestStrings(request.TagIDs)
	if err := validateUnscopedTagIDs(orphanTagIDsForScope(requestTagIDs, mentionScopes), secutils.SanitizeForLogArray(knowledgeBaseIDs)); err != nil {
		logger.Error(ctx, err.Error())
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	tagScopes := mergeTagScopesFromRequestIDs(mentionScopes, requestTagIDs, secutils.SanitizeForLogArray(knowledgeBaseIDs))

	if len(knowledgeBaseIDs) == 0 && len(request.KnowledgeIDs) == 0 && len(tagScopes) == 0 {
		logger.Error(ctx, "No knowledge base IDs, knowledge IDs, or tag scopes provided")
		c.Error(errors.NewBadRequestError("At least one knowledge_base_id, knowledge_base_ids, knowledge_ids, or scoped tag must be provided"))
		return
	}
	logger.Infof(
		ctx,
		"Knowledge search request, knowledge base IDs: %v, knowledge IDs: %v, tag scopes: %d, query: %s",
		secutils.SanitizeForLogArray(knowledgeBaseIDs),
		secutils.SanitizeForLogArray(request.KnowledgeIDs),
		len(tagScopes),
		secutils.SanitizeForLog(request.Query),
	)

	// Directly call knowledge retrieval service without LLM summarization
	searchResults, err := h.sessionService.SearchKnowledge(ctx, knowledgeBaseIDs, request.KnowledgeIDs, tagScopes, request.Query)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge search completed, found %d results", len(searchResults))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    searchResults,
	})
}

// KnowledgeQA godoc
// @Summary      知识问答
// @Description  基于知识库的问答（使用LLM总结），支持SSE流式响应
// @Tags         问答
// @Accept       json
// @Produce      text/event-stream
// @Param        session_id  path      string                   true  "会话ID"
// @Param        request     body      CreateKnowledgeQARequest true  "问答请求"
// @Success      200         {object}  map[string]interface{}   "问答结果（SSE流）"
// @Failure      400         {object}  errors.AppError          "请求参数错误"
// @Security     Bearer
// @Router       /knowledge-chat/{session_id} [post]
func (h *Handler) KnowledgeQA(c *gin.Context) {
	// Parse and validate request
	reqCtx, request, err := h.parseQARequest(c, "KnowledgeQA")
	if err != nil {
		c.Error(err)
		return
	}

	if request.SuggestedQuestionID != "" {
		fixedAnswer, resolveErr := h.resolveSuggestedQuestionAnswer(
			reqCtx.ctx, request.SuggestedQuestionID, request.Query,
		)
		if resolveErr != nil {
			c.Error(resolveErr)
			return
		}
		reqCtx.fixedSuggestedAnswer = fixedAnswer
	}

	// A fixed answer must not invoke any model, including the async title
	// generator. Generated suggestions continue through the normal QA path.
	generateTitle := !request.DisableTitle && reqCtx.fixedSuggestedAnswer == ""
	h.executeQA(reqCtx, qaModeNormal, generateTitle)
}

// resolveSuggestedQuestionAnswer validates that the selected global question
// still exists and that the client did not pair its ID with different text.
// An empty answer means the configured question should use normal generation.
func (h *Handler) resolveSuggestedQuestionAnswer(
	ctx context.Context,
	suggestedQuestionID string,
	query string,
) (string, error) {
	if h.suggestedQuestions == nil {
		return "", errors.NewInternalServerError("suggested question service is unavailable")
	}
	suggestion, err := h.suggestedQuestions.GetSuggestedQuestion(ctx, suggestedQuestionID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(query) != strings.TrimSpace(suggestion.Question) {
		return "", errors.NewBadRequestError("query does not match suggested question")
	}
	if suggestion.AnswerMode == types.SuggestedQuestionAnswerCustom {
		return suggestion.CustomAnswer, nil
	}
	return "", nil
}

// AgentQA godoc
// @Summary      Agent问答
// @Description  基于Agent的智能问答，支持多轮对话和SSE流式响应
// @Tags         问答
// @Accept       json
// @Produce      text/event-stream
// @Param        session_id  path      string                   true  "会话ID"
// @Param        request     body      CreateKnowledgeQARequest true  "问答请求"
// @Success      200         {object}  map[string]interface{}   "问答结果（SSE流）"
// @Failure      400         {object}  errors.AppError          "请求参数错误"
// @Security     Bearer
// @Router       /agent-chat/{session_id} [post]
func (h *Handler) AgentQA(c *gin.Context) {
	// Parse and validate request
	reqCtx, request, err := h.parseQARequest(c, "AgentQA")
	if err != nil {
		c.Error(err)
		return
	}

	// Determine if agent mode should be enabled
	// Priority: customAgent.IsAgentMode() > request.AgentEnabled
	agentModeEnabled := request.AgentEnabled
	if reqCtx.customAgent != nil {
		agentModeEnabled = reqCtx.customAgent.IsAgentMode()
		logger.Infof(reqCtx.ctx, "Agent mode determined by custom agent: %v (config.agent_mode=%s)",
			agentModeEnabled, reqCtx.customAgent.Config.AgentMode)
	}

	// Sanity gate: agent mode requires a resolved CustomAgent. If we got
	// here with agent_enabled=true but agent_id missing/unresolvable, the
	// AgentQA service will fail deep inside the async goroutine with a
	// generic "custom agent configuration is required" error and the user
	// just sees a broken stream. Reject early with a clear 400 so the
	// frontend can recover (e.g. fall back to quick-answer). Most likely
	// cause is a stale localStorage settings blob where selectedAgentId
	// got blanked but isAgentEnabled stayed true — usually after a
	// cross-knowledgeDomain switch where the previously selected agent is no
	// longer visible.
	if agentModeEnabled && reqCtx.customAgent == nil {
		logger.Warnf(reqCtx.ctx,
			"Agent mode requested without a resolvable agent_id, rejecting; session=%s, request.AgentID=%q",
			reqCtx.sessionID, secutils.SanitizeForLog(request.AgentID))
		c.Error(errors.NewBadRequestError(
			"agent_id is required when agent mode is enabled"))
		return
	}

	// Route to appropriate handler based on agent mode
	if agentModeEnabled {
		h.executeQA(reqCtx, qaModeAgent, true)
	} else {
		logger.Infof(reqCtx.ctx, "Agent mode disabled, delegating to normal mode for session: %s", reqCtx.sessionID)
		h.executeQA(reqCtx, qaModeNormal, !request.DisableTitle)
	}
}

// qaMode determines which QA execution path to use.
type qaMode int

const (
	qaModeNormal qaMode = iota // KnowledgeQA pipeline (RAG / pure chat)
	qaModeAgent                // Agent engine with tool calling
)

// executeQA is the unified execution flow for both KnowledgeQA and AgentQA modes.
// It handles message creation, SSE setup, VLM analysis, service invocation, and error handling.
func (h *Handler) executeQA(reqCtx *qaRequestContext, mode qaMode, generateTitle bool) {
	ctx := reqCtx.ctx
	sessionID := reqCtx.sessionID

	// Persist the input-bar state used for this request so reopening the
	// session can rehydrate agent / model / KB / web-search / MCP selections.
	// This is a pure UI memo (no behavioural effect) and runs in a goroutine
	// to avoid adding a DB round-trip to TTFB. Use WithoutCancel so a fast
	// client disconnect doesn't drop the write.
	go h.persistLastRequestState(ctx, reqCtx, mode)

	// Agent mode: emit agent query event before message creation
	if mode == qaModeAgent {
		if err := event.Emit(ctx, event.Event{
			Type:      event.EventAgentQuery,
			SessionID: sessionID,
			RequestID: reqCtx.requestID,
			Data: event.AgentQueryData{
				SessionID: sessionID,
				Query:     reqCtx.query,
				RequestID: reqCtx.requestID,
			},
		}); err != nil {
			logger.Errorf(ctx, "Failed to emit agent query event: %v", err)
			return
		}
	}

	// Create user message
	userMsg, err := h.createUserMessage(ctx, sessionID, reqCtx.query, reqCtx.requestID, reqCtx.mentionedItems, convertImageAttachments(reqCtx.images), reqCtx.attachments, reqCtx.channel)
	if err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	reqCtx.userMessageID = userMsg.ID

	// Create assistant message
	assistantMessagePtr, err := h.createAssistantMessage(ctx, reqCtx.assistantMessage)
	if err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	reqCtx.assistantMessage = assistantMessagePtr

	if mode == qaModeNormal {
		logger.Infof(ctx, "Using knowledge bases: %v", reqCtx.knowledgeBaseIDs)
	} else {
		logger.Infof(ctx, "Calling agent QA service, session ID: %s", sessionID)
	}

	// Setup SSE stream
	streamCtx := h.setupSSEStream(reqCtx, generateTitle)

	// Normal mode: register completion handler on EventAgentFinalAnswer
	// (Agent mode handles completion in the defer block instead). The final
	// stream-level complete event is emitted only after KnowledgeQA returns so
	// unified-QA can close its thinking stream first.
	var normalAnswerDone bool
	var progressPersistMu sync.Mutex
	if mode == qaModeNormal {
		// Persist reasoning_content into agent_steps so historical reload can
		// reconstruct the thinking card (same shape as Agent-mode steps).
		// Accumulate on assistantMessage directly so user-initiated stop also
		// keeps whatever reasoning had streamed before the cancel.
		streamCtx.eventBus.On(event.EventAgentThought, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.AgentThoughtData)
			if !ok || (data.Content == "" && data.StepID == "") {
				return nil
			}
			progressPersistMu.Lock()
			defer progressPersistMu.Unlock()
			appendQuickAnswerProgressWithType(streamCtx.assistantMessage, types.ResponseTypeThinking, data)
			return nil
		})
		streamCtx.eventBus.On(event.EventQuestionUnderstood, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.AgentThoughtData)
			if !ok {
				return nil
			}
			progressPersistMu.Lock()
			defer progressPersistMu.Unlock()
			appendQuickAnswerProgressWithType(streamCtx.assistantMessage, types.ResponseTypeQuestionUnderstood, data)
			return nil
		})
		streamCtx.eventBus.On(event.EventKnowledgeSearch, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.AgentThoughtData)
			if !ok {
				return nil
			}
			progressPersistMu.Lock()
			defer progressPersistMu.Unlock()
			appendQuickAnswerProgressWithType(streamCtx.assistantMessage, types.ResponseTypeKnowledgeRetrieved, data)
			return nil
		})

		streamCtx.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.AgentFinalAnswerData)
			if !ok {
				return nil
			}
			streamCtx.assistantMessage.Content += data.Content
			if data.IsFallback {
				streamCtx.assistantMessage.IsFallback = true
			}
			if data.Done {
				normalAnswerDone = true
			}
			return nil
		})
	}

	// Execute QA asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 10240)
				runtime.Stack(buf, true)
				stageName := "Knowledge QA"
				if mode == qaModeAgent {
					stageName = "Agent QA"
				}
				logger.ErrorWithFields(streamCtx.asyncCtx,
					errors.NewInternalServerError(fmt.Sprintf("%s service panicked: %v\n%s", stageName, r, string(buf))),
					map[string]interface{}{"session_id": sessionID})
			}
			// Agent mode completes in defer; normal mode completes after KnowledgeQA returns.
			if mode == qaModeAgent {
				// Use WithoutCancel so a user-triggered stop (which cancels
				// asyncCtx) doesn't also cancel the GORM UPDATE that persists
				// AgentSteps/Content. Without this, cancelled-ctx makes
				// GORM skip the write and the agent's intermediate steps
				// (thinking / tool_call history) are lost on page refresh.
				updateCtx := context.WithoutCancel(streamCtx.asyncCtx)
				h.completeAssistantMessage(updateCtx, streamCtx.assistantMessage)
				logger.Infof(streamCtx.asyncCtx, "Agent QA service completed for session: %s", sessionID)
			}
		}()

		// Run VLM image analysis if applicable
		h.runVLMAnalysisIfNeeded(streamCtx, reqCtx, mode)

		// Build QA request and invoke the appropriate service
		qaReq := reqCtx.buildQARequest()

		var serviceErr error
		var stageName string
		if mode == qaModeNormal && reqCtx.fixedSuggestedAnswer != "" {
			stageName = "suggested_question_fixed_answer"
			serviceErr = streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
				Type: event.EventAgentFinalAnswer, SessionID: sessionID, RequestID: reqCtx.requestID,
				Data: event.AgentFinalAnswerData{Content: reqCtx.fixedSuggestedAnswer, Done: true},
			})
		} else if mode == qaModeNormal {
			stageName = "knowledge_qa_execution"
			serviceErr = h.sessionService.KnowledgeQA(streamCtx.asyncCtx, qaReq, streamCtx.eventBus)
		} else {
			stageName = "agent_execution"
			serviceErr = h.sessionService.AgentQA(streamCtx.asyncCtx, qaReq, streamCtx.eventBus)
		}

		if serviceErr != nil {
			// A user-requested stop cancels asyncCtx, which surfaces here as a
			// context cancellation. That is an expected outcome, not a failure:
			// the stop event already notifies the client, so don't emit a
			// spurious error event (which would otherwise show an error toast).
			if streamCtx.asyncCtx.Err() != nil {
				logger.Infof(streamCtx.asyncCtx, "QA cancelled by user stop for session: %s", sessionID)
			} else {
				logger.ErrorWithFields(streamCtx.asyncCtx, serviceErr, nil)
				streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
					Type:      event.EventError,
					SessionID: sessionID,
					Data: event.ErrorData{
						Error:     serviceErr.Error(),
						Stage:     stageName,
						SessionID: sessionID,
					},
				})
			}
			return
		}

		if mode == qaModeNormal && normalAnswerDone {
			durationMS := time.Since(reqCtx.receivedAt).Milliseconds()
			progressPersistMu.Lock()
			streamCtx.assistantMessage.AgentDurationMs = durationMS
			stepCount := len(streamCtx.assistantMessage.AgentSteps)
			progressPersistMu.Unlock()
			logger.Infof(streamCtx.asyncCtx, "Knowledge QA service completed for session: %s", sessionID)
			h.completeAssistantMessage(streamCtx.asyncCtx, streamCtx.assistantMessage)
			streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
				Type:      event.EventAgentComplete,
				SessionID: sessionID,
				Data: event.AgentCompleteData{
					MessageID: reqCtx.assistantMessage.ID, TotalSteps: stepCount,
					TotalDurationMs: durationMS,
				},
			})
		}
	}()

	// Handle SSE events (blocking)
	shouldWaitForTitle := generateTitle && reqCtx.session.Title == ""
	h.handleAgentEventsForSSE(ctx, reqCtx.c, sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, streamCtx.eventBus, shouldWaitForTitle)
}

// runVLMAnalysisIfNeeded runs VLM image analysis within the async goroutine,
// emitting tool_call/tool_result events so the user can see progress.
// For normal mode, VLM only runs on the pure-chat path (no KB, no web search);
// RAG paths defer VLM to the pipeline rewrite step.
// For agent mode, VLM always runs when images and a VLM model are present.
func (h *Handler) runVLMAnalysisIfNeeded(streamCtx *sseStreamContext, reqCtx *qaRequestContext, mode qaMode) {
	if len(reqCtx.images) == 0 || reqCtx.customAgent == nil || reqCtx.customAgent.Config.VLMModelID == "" {
		return
	}

	sessionID := reqCtx.sessionID

	// In normal mode, only run VLM for pure-chat path
	if mode == qaModeNormal {
		hasRequestKBs := len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0
		if hasRequestKBs || reqCtx.webSearchEnabled {
			return // VLM will be handled by the pipeline rewrite step
		}
	}

	// Emit VLM tool call/result events
	toolCallID := uuid.New().String()
	iteration := 0 // agent mode uses iteration field

	streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
		Type:      event.EventAgentToolCall,
		SessionID: sessionID,
		Data: event.AgentToolCallData{
			ToolCallID: toolCallID,
			ToolName:   "image_analysis",
			Iteration:  iteration,
		},
	})

	vlmStart := time.Now()
	h.analyzeImageAttachments(streamCtx.asyncCtx, reqCtx.images,
		reqCtx.customAgent.Config.VLMModelID, reqCtx.query)

	outputMsg := "已分析图片内容"
	if mode == qaModeAgent {
		outputMsg = "已查看图片内容"
	}
	streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
		Type:      event.EventAgentToolResult,
		SessionID: sessionID,
		Data: event.AgentToolResultData{
			ToolCallID: toolCallID,
			ToolName:   "image_analysis",
			Output:     outputMsg,
			Success:    true,
			Duration:   time.Since(vlmStart).Milliseconds(),
			Iteration:  iteration,
		},
	})
}

// persistLastRequestState records the input-bar state the user just sent so
// that reopening this session restores agent/model/KB/web-search/MCP picks.
// Pure UI memo — failures are logged but never bubble up; the caller runs
// this in a goroutine and is safe to discard the returned context.
func (h *Handler) persistLastRequestState(parentCtx context.Context, reqCtx *qaRequestContext, mode qaMode) {
	// Detach from the HTTP request lifetime: this write must survive both
	// SSE disconnects and the parent gin context being released after the
	// handler returns.
	ctx := logger.CloneContext(context.WithoutCancel(parentCtx))

	agentEnabled := reqCtx.reqAgentEnabled
	// Mirror the resolution rule used in AgentQA: a resolved custom agent's
	// agent_mode wins over the request flag. For KnowledgeQA the request
	// itself carries agent_enabled=false, so this collapses correctly.
	if mode == qaModeAgent && reqCtx.customAgent != nil {
		agentEnabled = reqCtx.customAgent.IsAgentMode()
	}

	state := &types.SessionLastRequestState{
		AgentID:          reqCtx.reqAgentID,
		AgentEnabled:     agentEnabled,
		ModelID:          reqCtx.summaryModelID,
		KnowledgeBaseIDs: reqCtx.knowledgeBaseIDs,
		KnowledgeIDs:     reqCtx.knowledgeIDs,
		TagIDs:           reqCtx.tagIDs,
		MCPServiceIDs:    reqCtx.mcpServiceIDs,
		SkillNames:       reqCtx.skillNames,
		MentionedItems:   reqCtx.mentionedItems,
		WebSearchEnabled: reqCtx.webSearchEnabled,
	}

	if err := h.sessionService.UpdateSessionLastRequestState(ctx, reqCtx.sessionID, state); err != nil {
		logger.Warnf(ctx, "persist last_request_state failed for session %s: %v", reqCtx.sessionID, err)
	}
}

// appendQuickAnswerProgress persists sanitized unified-QA progress in the
// existing agent_steps JSON column. Legacy unstructured reasoning continues
// to accumulate in one step for backward compatibility.
func appendQuickAnswerProgress(msg *types.Message, data event.AgentThoughtData) {
	appendQuickAnswerProgressWithType(msg, types.ResponseTypeThinking, data)
}

func appendQuickAnswerProgressWithType(
	msg *types.Message,
	responseType types.ResponseType,
	data event.AgentThoughtData,
) {
	if data.Content == "" && data.StepID == "" {
		return
	}
	if data.StepID != "" {
		for i := range msg.AgentSteps {
			if msg.AgentSteps[i].ProgressStepID != data.StepID {
				continue
			}
			step := &msg.AgentSteps[i]
			if data.Content != "" && !strings.Contains(step.ReasoningContent, data.Content) {
				step.ReasoningContent += data.Content
			}
			applyQuickAnswerProgressFields(step, responseType, data)
			return
		}
		step := types.AgentStep{
			Iteration: len(msg.AgentSteps), Timestamp: time.Now(), ToolCalls: make([]types.ToolCall, 0),
			ReasoningContent: data.Content,
		}
		applyQuickAnswerProgressFields(&step, responseType, data)
		msg.AgentSteps = append(msg.AgentSteps, step)
		return
	}
	if len(msg.AgentSteps) == 0 {
		msg.AgentSteps = types.AgentSteps{{
			Iteration: 0,
			Timestamp: time.Now(),
			ToolCalls: make([]types.ToolCall, 0),
		}}
	}
	msg.AgentSteps[0].ReasoningContent += data.Content
}

func applyQuickAnswerProgressFields(step *types.AgentStep, responseType types.ResponseType, data event.AgentThoughtData) {
	step.RunID = data.RunID
	step.ProgressStepID = data.StepID
	step.ProgressResponseType = responseType
	step.ProgressStage = data.Stage
	step.ProgressAgentID = data.AgentID
	step.ProgressStatus = data.Status
	step.ProgressResultCount = data.ResultCount
	step.ProgressToolCalls = data.ToolCalls
	step.ProgressModelCalls = data.ModelCalls
	if data.DurationMS > 0 {
		step.Duration = data.DurationMS
	}
}

// completeAssistantMessage marks an assistant message as complete and updates it.
func (h *Handler) completeAssistantMessage(ctx context.Context, assistantMessage *types.Message) {
	assistantMessage.UpdatedAt = time.Now()
	assistantMessage.IsCompleted = true
	_ = h.messageService.UpdateMessage(ctx, assistantMessage)
}
