package service

import (
	"context"
	"fmt"
	"strings"

	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
)

// ---------------------------------------------------------------------------
// Shared QA helpers: KB resolution, model resolution, retrieval knowledgeDomain
// ---------------------------------------------------------------------------

// resolveKnowledgeBases preserves the scope selected for ordinary RAG.
// buildSearchTargets subsequently intersects these IDs with the current user's
// effective enterprise grants. AgentQA does not call this helper: agents build
// their complete scope directly from the current user's grants.
func (s *sessionService) resolveKnowledgeBases(
	ctx context.Context,
	req *types.QARequest,
) (kbIDs []string, knowledgeIDs []string, err error) {
	if req == nil {
		return nil, nil, nil
	}
	return uniqueNonEmptyStrings(req.KnowledgeBaseIDs),
		uniqueNonEmptyStrings(req.KnowledgeIDs),
		nil
}

// resolveChatModelID resolves the effective chat model ID for a QA request.
//
// When an agent is selected, its model configuration must be complete and
// valid. A request-level override may choose another valid model for this
// request, but it must not make an unconfigured or stale agent appear usable.
//
// Without an agent, the legacy KB / session / system fallback remains
// available for non-agent callers.
func (s *sessionService) resolveChatModelID(
	ctx context.Context,
	req *types.QARequest,
	knowledgeBaseIDs []string,
	knowledgeIDs []string,
) (string, error) {
	summaryModelID := req.SummaryModelID
	customAgent := req.CustomAgent
	session := req.Session

	if customAgent != nil {
		configuredModelID := strings.TrimSpace(customAgent.Config.ModelID)
		if configuredModelID == "" {
			return "", fmt.Errorf("chat model is not configured: please set model_id on agent %s", customAgent.ID)
		}
		model, err := s.modelService.GetModelByID(ctx, configuredModelID)
		if err != nil || model == nil || model.Type != types.ModelTypeKnowledgeQA {
			return "", fmt.Errorf("configured chat model %s is unavailable for agent %s", configuredModelID, customAgent.ID)
		}
	}

	summaryModelID = strings.TrimSpace(summaryModelID)
	if summaryModelID != "" {
		if model, err := s.modelService.GetModelByID(ctx, summaryModelID); err == nil && model != nil &&
			model.Type == types.ModelTypeKnowledgeQA {
			logger.Infof(ctx, "Using request's summary model override: %s", summaryModelID)
			return summaryModelID, nil
		}
		logger.Warnf(ctx, "Request provided invalid summary model ID %s, falling back", summaryModelID)
	}
	if customAgent != nil && strings.TrimSpace(customAgent.Config.ModelID) != "" {
		logger.Infof(ctx, "Using custom agent's model_id: %s", strings.TrimSpace(customAgent.Config.ModelID))
		return strings.TrimSpace(customAgent.Config.ModelID), nil
	}
	return s.selectChatModelID(ctx, session, knowledgeBaseIDs, knowledgeIDs)
}

// applyAgentOverridesToChatManage applies custom agent configuration overrides
// to a ChatManage object that was initialized with system defaults.
// This covers: system prompt, context template, temperature, max tokens, thinking,
// retrieval thresholds, rewrite settings, fallback settings, FAQ strategy, and history turns.
func (s *sessionService) applyAgentOverridesToChatManage(
	ctx context.Context,
	customAgent *types.CustomAgent,
	cm *types.ChatManage,
) {
	if customAgent == nil {
		return
	}

	// Ensure defaults are set
	customAgent.EnsureDefaults()

	// Override summary config fields
	if customAgent.Config.SystemPrompt != "" {
		cm.SummaryConfig.Prompt = customAgent.Config.SystemPrompt
		logger.Infof(ctx, "Using custom agent's system_prompt")
	}
	if customAgent.Config.ContextTemplate != "" {
		cm.SummaryConfig.ContextTemplate = customAgent.Config.ContextTemplate
		logger.Infof(ctx, "Using custom agent's context_template")
	}
	if customAgent.Config.Temperature >= 0 {
		cm.SummaryConfig.Temperature = customAgent.Config.Temperature
		logger.Infof(ctx, "Using custom agent's temperature: %f", customAgent.Config.Temperature)
	}
	if customAgent.Config.MaxCompletionTokens > 0 {
		cm.SummaryConfig.MaxCompletionTokens = customAgent.Config.MaxCompletionTokens
		logger.Infof(ctx, "Using custom agent's max_completion_tokens: %d", customAgent.Config.MaxCompletionTokens)
	}
	// Agent-level thinking setting takes full control (no global fallback).
	// EnsureDefaults pins nil to explicit false so thinking_control wire formats
	// always receive a value.
	cm.SummaryConfig.Thinking = customAgent.Config.Thinking
	if customAgent.Config.Thinking != nil {
		logger.Infof(ctx, "Using custom agent's thinking: %v", *customAgent.Config.Thinking)
	} else {
		logger.Warnf(ctx, "Custom agent thinking is unset after EnsureDefaults; model thinking param will be omitted")
	}

	// Override retrieval strategy settings
	if customAgent.Config.EmbeddingTopK > 0 {
		cm.EmbeddingTopK = customAgent.Config.EmbeddingTopK
	}
	if customAgent.Config.KeywordThreshold > 0 {
		cm.KeywordThreshold = customAgent.Config.KeywordThreshold
	}
	if customAgent.Config.VectorThreshold > 0 {
		cm.VectorThreshold = customAgent.Config.VectorThreshold
	}
	if customAgent.Config.RerankTopK > 0 {
		cm.RerankTopK = customAgent.Config.RerankTopK
	}
	cm.RerankThreshold = customAgent.Config.RerankThreshold
	if customAgent.Config.RerankModelID != "" {
		cm.RerankModelID = customAgent.Config.RerankModelID
	}

	// Override rewrite settings
	cm.EnableRewrite = customAgent.Config.EnableRewrite
	cm.EnableQueryExpansion = customAgent.Config.EnableQueryExpansion
	if customAgent.Config.RewritePromptSystem != "" {
		cm.RewritePromptSystem = customAgent.Config.RewritePromptSystem
	}
	if customAgent.Config.RewritePromptUser != "" {
		cm.RewritePromptUser = customAgent.Config.RewritePromptUser
	}
	if customAgent.Config.QueryUnderstandModelID != "" {
		cm.QueryUnderstandModelID = customAgent.Config.QueryUnderstandModelID
		logger.Infof(ctx, "Using custom agent's query_understand_model_id: %s",
			customAgent.Config.QueryUnderstandModelID)
	}

	// Override fallback settings
	if customAgent.Config.FallbackStrategy != "" {
		cm.FallbackStrategy = types.FallbackStrategy(customAgent.Config.FallbackStrategy)
	}
	if customAgent.Config.FallbackResponse != "" {
		cm.FallbackResponse = customAgent.Config.FallbackResponse
	}
	if customAgent.Config.FallbackPrompt != "" {
		cm.FallbackPrompt = customAgent.Config.FallbackPrompt
	}

	// Override web search settings
	if customAgent.Config.WebSearchMaxResults > 0 {
		cm.WebSearchMaxResults = customAgent.Config.WebSearchMaxResults
	}

	// Override history turns
	if customAgent.Config.HistoryTurns > 0 {
		cm.MaxRounds = customAgent.Config.HistoryTurns
		logger.Infof(ctx, "Using custom agent's history_turns: %d", cm.MaxRounds)
	}
	if !customAgent.Config.MultiTurnEnabled {
		cm.MaxRounds = 0
		logger.Infof(ctx, "Multi-turn disabled by custom agent, clearing history")
	}

	// FAQ strategy settings
	cm.FAQPriorityEnabled = customAgent.Config.FAQPriorityEnabled
	cm.FAQDirectAnswerThreshold = customAgent.Config.FAQDirectAnswerThreshold
	cm.FAQScoreBoost = customAgent.Config.FAQScoreBoost
	if cm.FAQPriorityEnabled {
		logger.Infof(ctx, "FAQ priority enabled: threshold=%.2f, boost=%.2f",
			cm.FAQDirectAnswerThreshold, cm.FAQScoreBoost)
	}

	// Data-analysis pipeline stage (opt-in, default off).
	cm.DataAnalysisEnabled = customAgent.Config.DataAnalysisEnabled
	if cm.DataAnalysisEnabled {
		logger.Infof(ctx, "Data analysis pipeline stage enabled by custom agent")
	}

	if len(customAgent.Config.IntentPrompts) > 0 {
		cm.IntentPromptOverrides = customAgent.Config.IntentPrompts
		logger.Infof(ctx, "Using custom agent's intent_prompts (%d overrides)", len(cm.IntentPromptOverrides))
	}
}
