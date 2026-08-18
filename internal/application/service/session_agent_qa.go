package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"roche.local/knowledge-agent-platform/internal/agent/tools"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

// AgentQA performs agent-based question answering with conversation history and streaming support
// customAgent provides the platform agent configuration for this request.
// summaryModelID optionally overrides the model from customAgent config.
func (s *sessionService) AgentQA(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
) error {
	sessionID := req.Session.ID
	sessionJSON, err := json.Marshal(req.Session)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal session, session ID: %s, error: %v", sessionID, err)
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// customAgent is required for AgentQA; knowledge access is checked when
	// the effective retrieval scope is resolved.
	if req.CustomAgent == nil {
		logger.Warnf(ctx, "Custom agent not provided for session: %s", sessionID)
		return errors.New("custom agent configuration is required for agent QA")
	}

	logger.Infof(ctx, "Start agent-based question answering, session ID: %s, query: %s, session: %s",
		sessionID, req.Query, string(sessionJSON))

	// Ensure defaults are set
	req.CustomAgent.EnsureDefaults()

	// Build the runtime AgentConfig.
	agentConfig, err := s.buildAgentConfig(ctx, req)
	if err != nil {
		return err
	}

	// Set VLM model ID for tool result image analysis (runtime-only field)
	if req.CustomAgent != nil && req.CustomAgent.Config.VLMModelID != "" {
		agentConfig.VLMModelID = req.CustomAgent.Config.VLMModelID
	}

	// Resolve model ID using shared helper (AgentQA requires a model, so error if not found)
	effectiveModelID, err := s.resolveChatModelID(ctx, req, agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs)
	if err != nil {
		return err
	}
	if effectiveModelID == "" {
		logger.Warnf(ctx, "No summary model configured for custom agent %s", req.CustomAgent.ID)
		return errors.New("summary model (model_id) is not configured in custom agent settings")
	}

	summaryModel, err := s.modelService.GetChatModel(ctx, effectiveModelID)
	if err != nil {
		logger.Warnf(ctx, "Failed to get chat model: %v", err)
		return fmt.Errorf("failed to get chat model: %w", err)
	}

	// Get rerank model from custom agent config only when knowledge_search can
	// actually run. A disabled KB scope makes all KB tools ineffective, so it
	// must not force users to configure an otherwise-unused rerank model.
	var rerankModel rerank.Reranker
	if agentHasKnowledgeScope(agentConfig) && agentRequiresRerankModel(req.CustomAgent) {
		// Rerank model is resolved purely from the agent config now.
		// The agent must declare its own rerank model. If an agent doesn't
		// need reranking,
		// agentRequiresRerankModel() below already lets it pass.
		rerankModelID := req.CustomAgent.Config.RerankModelID
		if rerankModelID == "" {
			logger.Warnf(ctx, "No rerank model configured for custom agent %s, but knowledge_search tool is enabled", req.CustomAgent.ID)
			return errors.New("rerank model is not configured: please set rerank_model_id on the agent")
		}

		rerankModel, err = s.modelService.GetRerankModel(ctx, rerankModelID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get rerank model: %v", err)
			return fmt.Errorf("failed to get rerank model: %w", err)
		}
	} else {
		logger.Infof(ctx, "knowledge_search is unavailable for the effective agent scope, skipping rerank model initialization")
	}

	// Load multi-turn history directly from DB (the single source of truth).
	// AgentSteps on each historical assistant message are expanded into proper
	// assistant_with_tool_calls + tool messages so the model can see what was
	// tried last turn — except final_answer, which is replayed as the trailing
	// canonical assistant message.
	var llmContext []chat.Message
	if agentConfig.MultiTurnEnabled {
		historyTurns := agentConfig.HistoryTurns
		if historyTurns <= 0 {
			historyTurns = 5
		}
		llmContext, err = LoadAgentHistory(ctx, s.messageRepo, sessionID, historyTurns)
		if err != nil {
			logger.Warnf(ctx, "Failed to load agent history from DB: %v, continuing without history", err)
			llmContext = []chat.Message{}
		}
		logger.Infof(ctx, "Loaded %d history messages from DB (turns=%d)", len(llmContext), historyTurns)
	} else {
		logger.Infof(ctx, "Multi-turn disabled for this agent, running without history")
		llmContext = []chat.Message{}
	}

	// Create agent engine with EventBus
	logger.Info(ctx, "Creating agent engine")
	engine, err := s.agentService.CreateAgentEngine(
		ctx,
		agentConfig,
		summaryModel,
		rerankModel,
		eventBus,
		sessionID,
		req.AssistantMessageID,
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to create agent engine: %v", err)
		return err
	}

	// Route image data based on agent model's vision capability
	var agentModelSupportsVision bool
	if effectiveModelID != "" {
		if modelInfo, err := s.modelService.GetModelByID(ctx, effectiveModelID); err == nil && modelInfo != nil {
			agentModelSupportsVision = modelInfo.Parameters.SupportsVision
		}
	}

	agentQuery := req.Query
	var agentImageURLs []string
	if agentModelSupportsVision && len(req.ImageURLs) > 0 {
		agentImageURLs = req.ImageURLs
		logger.Infof(ctx, "Agent model supports vision, passing %d image(s) directly", len(agentImageURLs))
	} else if req.ImageDescription != "" {
		agentQuery = req.Query + "\n\n[用户上传图片内容]\n" + req.ImageDescription
		logger.Infof(ctx, "Agent model does not support vision, appending image description (%d chars)", len(req.ImageDescription))
	}
	if req.QuotedContext != "" {
		agentQuery += "\n\n" + req.QuotedContext
	}
	// Inject attachment content (documents, audio transcripts, etc.) so the agent
	// can see uploaded files. Mirrors the behavior of the KnowledgeQA pipeline
	// (see chat_pipeline/into_chat_message.go).
	if len(req.Attachments) > 0 {
		agentQuery += req.Attachments.BuildPrompt()
		logger.Infof(ctx, "Appended %d attachment(s) to agent query", len(req.Attachments))
	}

	// Scope envelopes (runtime_context / must_use) are injected per LLM call inside
	// the agent engine only; we intentionally do not persist them on user messages
	// so multi-turn history stays clean and is not skewed by stale @mention scope.

	// Execute agent with streaming (asynchronously)
	// Events will be emitted to EventBus and handled by the Handler layer
	logger.Info(ctx, "Executing agent with streaming")
	if _, err := engine.Execute(ctx, sessionID, req.AssistantMessageID, agentQuery, llmContext, agentImageURLs); err != nil {
		logger.Errorf(ctx, "Agent execution failed: %v", err)
		// Emit error event to the EventBus used by this agent
		eventBus.Emit(ctx, event.Event{
			Type:      event.EventError,
			SessionID: sessionID,
			Data: event.ErrorData{
				Error:     err.Error(),
				Stage:     "agent_execution",
				SessionID: sessionID,
			},
		})
	}
	// Return empty - events will be handled by Handler via EventBus subscription
	return nil
}

// buildAgentConfig creates a runtime AgentConfig from the QARequest's custom
// agent configuration and the current user's authorized search targets.
func (s *sessionService) buildAgentConfig(
	ctx context.Context,
	req *types.QARequest,
) (*types.AgentConfig, error) {
	customAgent := req.CustomAgent
	agentConfig := &types.AgentConfig{
		MaxIterations:       customAgent.Config.MaxIterations,
		Temperature:         customAgent.Config.Temperature,
		WebSearchEnabled:    customAgent.Config.WebSearchEnabled && req.WebSearchEnabled,
		WebSearchMaxResults: customAgent.Config.WebSearchMaxResults,
		WebSearchProviderID: customAgent.Config.WebSearchProviderID,
		MultiTurnEnabled:    customAgent.Config.MultiTurnEnabled,
		HistoryTurns:        customAgent.Config.HistoryTurns,
		MCPSelectionMode:    customAgent.Config.MCPSelectionMode,
		MCPServices:         customAgent.Config.MCPServices,
		MCPAuthWaitTimeout:  customAgent.Config.MCPAuthWaitTimeout,
		Thinking:            customAgent.Config.Thinking,
		// Agent retrieval is always derived from the current user's grants.
		// Agent KB selection and per-turn @mentions are not authorization inputs.
		LLMCallTimeout:         customAgent.Config.LLMCallTimeout,
		RetainRetrievalHistory: customAgent.Config.RetainRetrievalHistory,
	}

	// Falls back to global configuration if no specific timeout is set for the agent.
	if agentConfig.LLMCallTimeout == 0 && s.cfg.Agent != nil && s.cfg.Agent.LLMCallTimeout > 0 {
		agentConfig.LLMCallTimeout = s.cfg.Agent.LLMCallTimeout
	}

	// Configure skills based on CustomAgentConfig
	s.configureSkillsFromAgent(ctx, agentConfig, customAgent)

	// Agent retrieval has one source of truth: the current user's effective
	// knowledge grants. Request @mentions and agent KB configuration do not
	// narrow or expand this authorization scope.
	searchTargets, err := s.buildAuthorizedAgentSearchTargets(ctx)
	if err != nil {
		return nil, err
	}
	agentConfig.SearchTargets = searchTargets
	agentConfig.KnowledgeBases = searchTargets.GetAllKnowledgeBaseIDs()
	agentConfig.KnowledgeIDs = knowledgeIDsFromSearchTargets(searchTargets)

	// Use custom agent's allowed tools if specified, otherwise use defaults
	if len(customAgent.Config.AllowedTools) > 0 {
		agentConfig.AllowedTools = customAgent.Config.AllowedTools
	} else {
		agentConfig.AllowedTools = tools.DefaultAllowedTools()
	}
	// Apply per-turn @Skill / @MCP scope. Each helper narrows the agent's
	// whitelist to the mentioned items and records the pinned set used for the
	// <must_use> hint, keeping all scope logic in one place per resource type.
	applyPerRequestSkillScope(ctx, agentConfig, customAgent.Config.SkillsSelectionMode, req.SkillNames)
	applyPerRequestMCPScope(ctx, agentConfig, customAgent.Config.MCPServices, req.MCPServiceIDs)

	// Use custom agent's system prompt if specified
	if customAgent.Config.SystemPrompt != "" {
		agentConfig.UseCustomSystemPrompt = true
		agentConfig.SystemPrompt = customAgent.Config.SystemPrompt
	}

	logger.Infof(ctx, "Custom agent config applied: MaxIterations=%d, Temperature=%.2f, AllowedTools=%v, WebSearchEnabled=%v",
		agentConfig.MaxIterations, agentConfig.Temperature, agentConfig.AllowedTools, agentConfig.WebSearchEnabled)

	// Apply the platform fallback for web search result count.
	if agentConfig.WebSearchMaxResults == 0 {
		agentConfig.WebSearchMaxResults = 5
	}

	logger.Infof(ctx, "Built platform agent config for session %s", req.Session.ID)

	if len(agentConfig.KnowledgeBases) > 0 {
		logger.Infof(ctx, "Agent authorized for %d knowledge base(s): %v",
			len(agentConfig.KnowledgeBases), agentConfig.KnowledgeBases)
	} else {
		logger.Infof(ctx, "User has no authorized knowledge scope; agent is running without knowledge tools")
	}
	logger.Infof(ctx, "Agent authorization produced %d search target(s)", len(searchTargets))

	if agentConfig.MaxContextTokens <= 0 {
		agentConfig.MaxContextTokens = types.DefaultMaxContextTokens
	}

	return agentConfig, nil
}

// buildAuthorizedAgentSearchTargets constructs the agent's entire retrieval
// boundary from the current user's effective grants. It deliberately ignores
// agent KB configuration, request knowledge IDs, tags, and @mentions.
//
// A whole-KB grant becomes a KnowledgeBase target. Document-only grants become
// a Knowledge target containing exactly the granted knowledge IDs.
func (s *sessionService) buildAuthorizedAgentSearchTargets(
	ctx context.Context,
) (types.SearchTargets, error) {
	accessibleKBs, err := s.knowledgeBaseService.ListKnowledgeBases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list user-authorized knowledge bases: %w", err)
	}

	kbIDs := make([]string, 0, len(accessibleKBs))
	for _, kb := range accessibleKBs {
		if kb == nil || kb.ID == "" || kb.IsTemporary {
			continue
		}
		kbIDs = append(kbIDs, kb.ID)
	}
	kbIDs = uniqueNonEmptyStrings(kbIDs)
	if len(kbIDs) == 0 {
		return nil, nil
	}

	targets, err := s.buildSearchTargets(ctx, kbIDs, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("build user-authorized agent search targets: %w", err)
	}
	return targets, nil
}

func knowledgeIDsFromSearchTargets(targets types.SearchTargets) []string {
	var ids []string
	for _, target := range targets {
		if target == nil || target.Type != types.SearchTargetTypeKnowledge {
			continue
		}
		ids = append(ids, target.KnowledgeIDs...)
	}
	return uniqueNonEmptyStrings(ids)
}

// applyPerRequestSkillScope narrows the agent's skill whitelist to the @Skill
// mentions for this turn and records the pinned set for the <must_use> hint.
// It is a no-op when no skills were mentioned or skills are disabled.
func applyPerRequestSkillScope(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	skillsMode string,
	requested []string,
) {
	if len(requested) == 0 {
		return
	}
	if skillsMode == "none" || skillsMode == "" {
		logger.Warnf(ctx, "Ignoring @skill mention: agent skills selection is disabled (mode=%s)", skillsMode)
		return
	}
	if !agentConfig.SkillsEnabled {
		return
	}
	switch skillsMode {
	case "selected":
		agentConfig.AllowedSkills = intersectPreservingRequestOrder(requested, agentConfig.AllowedSkills)
		if len(agentConfig.AllowedSkills) == 0 {
			agentConfig.SkillsEnabled = false
		}
	case "all":
		agentConfig.AllowedSkills = dedupPreservingOrder(requested)
	}
	if agentConfig.SkillsEnabled && len(agentConfig.AllowedSkills) > 0 {
		agentConfig.PinnedSkillNames = intersectPreservingRequestOrder(requested, agentConfig.AllowedSkills)
	}
	logger.Infof(ctx, "Applied per-request @skill scope: requested=%v effective=%v pinned=%v",
		requested, agentConfig.AllowedSkills, agentConfig.PinnedSkillNames)
}

// applyPerRequestMCPScope narrows the agent's MCP services to the @MCP mentions
// for this turn and records the pinned set for the <must_use> hint. It is a
// no-op when no services were mentioned or MCP selection is disabled.
func applyPerRequestMCPScope(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	agentPresetMCPs []string,
	requested []string,
) {
	if len(requested) == 0 {
		return
	}
	if agentConfig.MCPSelectionMode == "none" {
		logger.Warnf(ctx, "Ignoring @MCP mention: agent MCP selection is disabled (mode=none)")
		return
	}
	mentioned := dedupPreservingOrder(requested)
	effective, mode := resolvePerRequestMCPScope(mentioned, agentPresetMCPs, agentConfig.MCPSelectionMode)
	if len(effective) == 0 {
		logger.Warnf(ctx, "Ignoring @MCP scope outside agent preset: requested=%v agent=%v",
			requested, agentPresetMCPs)
		return
	}
	agentConfig.MCPSelectionMode = mode
	agentConfig.MCPServices = effective
	agentConfig.PinnedMCPServiceIDs = intersectPreservingRequestOrder(requested, agentConfig.MCPServices)
	logger.Infof(ctx, "Applied per-request @MCP scope: requested=%v mode=%s effective=%v",
		requested, agentConfig.MCPSelectionMode, agentConfig.MCPServices)
}

// resolvePerRequestMCPScope narrows MCP registration for a per-turn @mention.
// selectionMode "none" rejects all mentions.
func resolvePerRequestMCPScope(
	mentioned, agentMCPs []string,
	selectionMode string,
) (effective []string, mode string) {
	if len(mentioned) == 0 {
		return nil, selectionMode
	}
	switch selectionMode {
	case "none":
		return nil, selectionMode
	case "selected":
		effective = intersectPreservingRequestOrder(mentioned, agentMCPs)
	case "all", "":
		effective = mentioned
	default:
		effective = mentioned
	}
	if len(effective) == 0 {
		return nil, selectionMode
	}
	return effective, "selected"
}

func intersectPreservingRequestOrder(requested []string, allowed []string) []string {
	allowedSet := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		if value != "" {
			allowedSet[value] = true
		}
	}
	result := make([]string, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for _, value := range requested {
		if value == "" || seen[value] || !allowedSet[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func dedupPreservingOrder(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// configureSkillsFromAgent configures skills settings in AgentConfig based on CustomAgentConfig
// Returns the skill directories and allowed skills based on the selection mode:
//   - "all": uses all preloaded skills
//   - "selected": uses the explicitly selected skills
//   - "none" or "": skills are disabled
func (s *sessionService) configureSkillsFromAgent(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	customAgent *types.CustomAgent,
) {
	if customAgent == nil {
		return
	}
	if !s.cfg.AreSkillsEnabled() {
		agentConfig.SkillsEnabled = false
		agentConfig.SkillDirs = nil
		agentConfig.AllowedSkills = nil
		logger.Infof(ctx, "Skills feature is disabled")
		return
	}
	dir := getPreloadedSkillsDir()
	switch customAgent.Config.SkillsSelectionMode {
	case "all":
		// Enable all preloaded skills
		agentConfig.SkillsEnabled = true
		agentConfig.SkillDirs = []string{dir}
		agentConfig.AllowedSkills = nil // Empty means all skills allowed
		logger.Infof(ctx, "SkillsSelectionMode=all: enabled all preloaded skills")
	case "selected":
		// Enable only selected skills
		if len(customAgent.Config.SelectedSkills) > 0 {
			agentConfig.SkillsEnabled = true
			agentConfig.SkillDirs = []string{dir}
			agentConfig.AllowedSkills = customAgent.Config.SelectedSkills
			logger.Infof(ctx, "SkillsSelectionMode=selected: enabled %d selected skills: %v",
				len(customAgent.Config.SelectedSkills), customAgent.Config.SelectedSkills)
		} else {
			agentConfig.SkillsEnabled = false
			logger.Infof(ctx, "SkillsSelectionMode=selected but no skills selected: skills disabled")
		}
	case "none", "":
		// Skills disabled
		agentConfig.SkillsEnabled = false
		logger.Infof(ctx, "SkillsSelectionMode=%s: skills disabled", customAgent.Config.SkillsSelectionMode)
	default:
		// Unknown mode, disable skills
		agentConfig.SkillsEnabled = false
		logger.Warnf(ctx, "Unknown SkillsSelectionMode=%s: skills disabled", customAgent.Config.SkillsSelectionMode)
	}

}
