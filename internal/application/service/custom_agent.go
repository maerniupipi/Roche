package service

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/agent/tools"
	"roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// Custom agent related errors
var (
	ErrAgentNotFound       = errors.New("agent not found")
	ErrCannotModifyBuiltin = errors.New("cannot modify built-in agent basic info")
	ErrCannotDeleteBuiltin = errors.New("cannot delete built-in agent")
	ErrAgentNameRequired   = errors.New("agent name is required")
)

// customAgentService implements the CustomAgentService interface
type customAgentService struct {
	repo          interfaces.CustomAgentRepository
	chunkRepo     interfaces.ChunkRepository
	kbService     interfaces.KnowledgeBaseService
	tagRepo       interfaces.KnowledgeTagRepository
	knowledgeRepo interfaces.KnowledgeRepository
	accessService interfaces.EnterpriseAccessService
}

// NewCustomAgentService creates a new custom agent service
func NewCustomAgentService(
	repo interfaces.CustomAgentRepository,
	chunkRepo interfaces.ChunkRepository,
	kbService interfaces.KnowledgeBaseService,
	tagRepo interfaces.KnowledgeTagRepository,
	knowledgeRepo interfaces.KnowledgeRepository,
	accessService interfaces.EnterpriseAccessService,
) interfaces.CustomAgentService {
	return &customAgentService{
		repo:          repo,
		chunkRepo:     chunkRepo,
		kbService:     kbService,
		tagRepo:       tagRepo,
		knowledgeRepo: knowledgeRepo,
		accessService: accessService,
	}
}

// CreateAgent creates a new custom agent
func (s *customAgentService) CreateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	// Validate required fields
	if strings.TrimSpace(agent.Name) == "" {
		return nil, ErrAgentNameRequired
	}

	// Generate UUID and set creation timestamps
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}

	// Record the creator for ownership checks.
	if uid, ok := types.UserIDFromContext(ctx); ok {
		agent.CreatedBy = uid
	}

	// Set timestamps
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()

	// Ensure agent mode is set for user-created agents
	if agent.Config.AgentMode == "" {
		agent.Config.AgentMode = types.AgentModeQuickAnswer
	}

	// Cannot create built-in agents
	agent.IsBuiltin = false

	// Set defaults
	agent.EnsureDefaults()

	logger.Infof(ctx, "Creating custom agent, ID: %s, name: %s, agent_mode: %s",
		agent.ID, agent.Name, agent.Config.AgentMode)

	if err := s.repo.CreateAgent(ctx, agent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": agent.ID})
		return nil, err
	}

	logger.Infof(ctx, "Custom agent created successfully, ID: %s, name: %s", agent.ID, agent.Name)
	return agent, nil
}

// GetAgentByID retrieves an agent by its ID (including built-in agents)
func (s *customAgentService) GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error) {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}
	// Check if it's a built-in agent using the registry
	if types.IsBuiltinAgentID(id) {
		// Try to get from database first (for customized config)
		agent, err := s.repo.GetAgentByID(ctx, id)
		if err == nil {
			// Found in database, return with customized config
			return agent, nil
		}
		// Not in database, return default built-in agent from registry (i18n-aware)
		if builtinAgent := types.GetBuiltinAgentWithContext(ctx, id); builtinAgent != nil {
			return builtinAgent, nil
		}
	}

	// Query from database
	agent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return nil, err
	}

	return agent, nil
}

// ListAgents lists all platform agents (including built-in agents).
func (s *customAgentService) ListAgents(ctx context.Context) ([]*types.CustomAgent, error) {
	// Get all agents from database (including built-in agents with customized config)
	allAgents, err := s.repo.ListAgents(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}

	// Track which built-in agents exist in database
	builtinInDB := make(map[string]bool)
	for _, agent := range allAgents {
		if types.IsBuiltinAgentID(agent.ID) {
			builtinInDB[agent.ID] = true
		}
	}

	// Build result: built-in agents first, then custom agents
	builtinIDs := types.GetBuiltinAgentIDs()
	result := make([]*types.CustomAgent, 0, len(allAgents)+len(builtinIDs))

	// Add built-in agents in order
	for _, builtinID := range builtinIDs {
		if builtinInDB[builtinID] {
			// Use customized config from database
			for _, agent := range allAgents {
				if agent.ID == builtinID {
					result = append(result, agent)
					break
				}
			}
		} else {
			// Use default built-in agent (i18n-aware)
			if agent := types.GetBuiltinAgentWithContext(ctx, builtinID); agent != nil {
				result = append(result, agent)
			}
		}
	}

	// Add custom agents
	for _, agent := range allAgents {
		if !types.IsBuiltinAgentID(agent.ID) {
			result = append(result, agent)
		}
	}
	return result, nil
}

// UpdateAgent updates an agent's information
func (s *customAgentService) UpdateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	if agent.ID == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Handle built-in agents specially using registry
	if types.IsBuiltinAgentID(agent.ID) {
		if err := requireAgentAdministrator(ctx, nil); err != nil {
			return nil, err
		}
		return s.updateBuiltinAgent(ctx, agent)
	}

	// Get existing agent
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	if err := requireAgentAdministrator(ctx, existingAgent); err != nil {
		return nil, err
	}

	// Cannot modify built-in status
	if existingAgent.IsBuiltin {
		return nil, ErrCannotModifyBuiltin
	}

	// Validate name
	if strings.TrimSpace(agent.Name) == "" {
		return nil, ErrAgentNameRequired
	}
	// Update fields
	existingAgent.Name = agent.Name
	existingAgent.Description = agent.Description
	existingAgent.Avatar = agent.Avatar
	existingAgent.Config = agent.Config
	existingAgent.UpdatedAt = time.Now()

	// Ensure defaults
	existingAgent.EnsureDefaults()

	logger.Infof(ctx, "Updating custom agent, ID: %s, name: %s", agent.ID, agent.Name)

	if err := s.repo.UpdateAgent(ctx, existingAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": agent.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Custom agent updated successfully, ID: %s", agent.ID)
	return existingAgent, nil
}

func requireAgentAdministrator(ctx context.Context, agent *types.CustomAgent) error {
	userID, root, ok := enterpriseContext(ctx)
	if !ok || (!root && (agent == nil || agent.CreatedBy != userID)) {
		return ErrEnterpriseAccessDenied
	}
	return nil
}

// updateBuiltinAgent updates a built-in agent's configuration (but not basic info)
func (s *customAgentService) updateBuiltinAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	// Get the default built-in agent from registry (i18n-aware)
	defaultAgent := types.GetBuiltinAgentWithContext(ctx, agent.ID)
	if defaultAgent == nil {
		return nil, ErrAgentNotFound
	}

	// Try to get existing customized config from database
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID)
	if err != nil && !errors.Is(err, repository.ErrCustomAgentNotFound) {
		return nil, err
	}

	if existingAgent != nil {
		// Update existing record - only update config, keep basic info unchanged
		existingAgent.Config = agent.Config
		existingAgent.UpdatedAt = time.Now()
		existingAgent.EnsureDefaults()

		logger.Infof(ctx, "Updating built-in agent config, ID: %s", agent.ID)

		if err := s.repo.UpdateAgent(ctx, existingAgent); err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id": agent.ID,
			})
			return nil, err
		}

		logger.Infof(ctx, "Built-in agent config updated successfully, ID: %s", agent.ID)
		return existingAgent, nil
	}

	// Create new record for built-in agent with customized config
	newAgent := &types.CustomAgent{
		ID:          defaultAgent.ID,
		Name:        defaultAgent.Name,
		Description: defaultAgent.Description,
		Avatar:      defaultAgent.Avatar,
		IsBuiltin:   true,
		Config:      agent.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	newAgent.EnsureDefaults()

	logger.Infof(ctx, "Creating built-in agent config record, ID: %s", agent.ID)

	if err := s.repo.CreateAgent(ctx, newAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": agent.ID})
		return nil, err
	}

	logger.Infof(ctx, "Built-in agent config record created successfully, ID: %s", agent.ID)
	return newAgent, nil
}

// DeleteAgent deletes an agent
func (s *customAgentService) DeleteAgent(ctx context.Context, id string) error {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return errors.New("agent ID cannot be empty")
	}

	// Cannot delete built-in agents using registry check
	if types.IsBuiltinAgentID(id) {
		return ErrCannotDeleteBuiltin
	}

	// Get existing agent to verify ownership
	existingAgent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return ErrAgentNotFound
		}
		return err
	}

	// Cannot delete built-in agents
	if existingAgent.IsBuiltin {
		return ErrCannotDeleteBuiltin
	}
	if err := requireAgentAdministrator(ctx, existingAgent); err != nil {
		return err
	}

	logger.Infof(ctx, "Deleting custom agent, ID: %s", id)

	if err := s.repo.DeleteAgent(ctx, id); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return err
	}

	logger.Infof(ctx, "Custom agent deleted successfully, ID: %s", id)
	return nil
}

// CopyAgent creates a copy of an existing agent
func (s *customAgentService) CopyAgent(ctx context.Context, id string) (*types.CustomAgent, error) {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Get the source agent
	sourceAgent, err := s.GetAgentByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Create a new agent with copied data
	newAgent := &types.CustomAgent{
		ID:          uuid.New().String(),
		Name:        sourceAgent.Name + " (副本)",
		Description: sourceAgent.Description,
		Avatar:      sourceAgent.Avatar,
		IsBuiltin:   false, // Copied agents are never built-in
		Config:      sourceAgent.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	// The clone is owned by whoever ran the copy, not the original creator.
	if uid, ok := types.UserIDFromContext(ctx); ok {
		newAgent.CreatedBy = uid
	}

	// Ensure defaults
	newAgent.EnsureDefaults()

	logger.Infof(ctx, "Copying agent, source ID: %s, new ID: %s", id, newAgent.ID)

	if err := s.repo.CreateAgent(ctx, newAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"source_agent_id": id,
			"new_agent_id":    newAgent.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Agent copied successfully, source ID: %s, new ID: %s", id, newAgent.ID)
	return newAgent, nil
}

func normalizedAgentKBIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	result := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// GetSuggestedQuestions returns suggestions from the knowledge resources the
// current user is authorized to read.
func (s *customAgentService) GetSuggestedQuestions(
	ctx context.Context,
	agentID string,
	kbIDs []string,
	knowledgeIDs []string,
	tagIDs []string,
	limit int,
) ([]types.SuggestedQuestion, error) {
	if limit <= 0 {
		limit = 6
	}

	// Get agent configuration
	agent, err := s.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	var result []types.SuggestedQuestion

	// 1. Add agent config suggested_prompts first (highest priority)
	if len(agent.Config.SuggestedPrompts) > 0 {
		for _, prompt := range agent.Config.SuggestedPrompts {
			if strings.TrimSpace(prompt) == "" {
				continue
			}
			result = append(result, types.SuggestedQuestion{
				Question: prompt,
				Source:   "agent_config",
			})
		}
	}

	if len(tagIDs) > 0 {
		resolved, err := s.resolveKnowledgeIDsFromTags(ctx, tagIDs)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id": agentID,
				"tag_ids":  tagIDs,
			})
			return s.truncateQuestions(result, limit), nil
		}
		knowledgeIDs = mergeUniqueStrings(knowledgeIDs, resolved)
		if len(knowledgeIDs) == 0 {
			return s.truncateQuestions(result, limit), nil
		}
	}
	if len(kbIDs) > 0 || len(knowledgeIDs) > 0 {
		if err := s.authorizeSuggestedQuestionTargets(ctx, kbIDs, knowledgeIDs); err != nil {
			return nil, err
		}
	}

	// 2. Determine knowledge scope. Without an explicit request scope, use every
	// knowledge base the current user can read. Agent configuration never grants
	// or narrows knowledge access.
	effectiveKBIDs := kbIDs
	if len(effectiveKBIDs) == 0 && len(knowledgeIDs) == 0 {
		kbs, listErr := s.kbService.ListKnowledgeBases(ctx)
		if listErr != nil {
			logger.ErrorWithFields(ctx, listErr, map[string]interface{}{"agent_id": agentID})
			return s.truncateQuestions(result, limit), nil
		}
		capFilter := tools.DeriveKBFilterForAgent(agent.Config.AgentMode, agent.Config.AllowedTools)
		for _, kb := range kbs {
			if !capFilter.IsEmpty() &&
				!tools.KBSatisfiesAgentRequirements(kb.Capabilities(), agent.Config.AgentMode, agent.Config.AllowedTools) {
				continue
			}
			effectiveKBIDs = append(effectiveKBIDs, kb.ID)
		}
	}

	if len(effectiveKBIDs) == 0 && len(knowledgeIDs) == 0 {
		return s.truncateQuestions(result, limit), nil
	}

	// Deduplicate questions we've already collected
	seen := make(map[string]bool)
	for _, q := range result {
		seen[q.Question] = true
	}

	remaining := limit - len(result)
	if remaining <= 0 {
		return s.truncateQuestions(result, limit), nil
	}

	// 3. Collect candidate chunks from both FAQ and Document KBs,
	//    grouped by knowledge_id for diversity.
	//    knowledgeID -> list of questions
	buckets := make(map[string][]types.SuggestedQuestion)

	// Determine query scope
	queryKBIDs := effectiveKBIDs
	queryKnowledgeIDs := knowledgeIDs

	// Fetch a large pool so DB-level random sampling covers multiple documents.
	fetchLimit := remaining * 5
	if fetchLimit < 20 {
		fetchLimit = 20
	}

	kbGroups, err := s.groupSuggestedQuestionTargetsByDomain(ctx, queryKBIDs, queryKnowledgeIDs)
	if err != nil {
		return nil, err
	}

	// Collect FAQ recommended chunks
	for groupKnowledgeDomainID, groupKBIDs := range kbGroups {
		faqChunks, err := s.chunkRepo.ListRecommendedFAQChunks(ctx, groupKnowledgeDomainID, groupKBIDs, queryKnowledgeIDs, fetchLimit)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id":            agentID,
				"knowledge_domain_id": groupKnowledgeDomainID,
			})
			continue
		}
		for _, chunk := range faqChunks {
			meta, err := chunk.FAQMetadata()
			if err != nil || meta == nil || meta.StandardQuestion == "" {
				continue
			}
			if seen[meta.StandardQuestion] {
				continue
			}
			seen[meta.StandardQuestion] = true
			buckets[chunk.KnowledgeID] = append(buckets[chunk.KnowledgeID], types.SuggestedQuestion{
				Question:        meta.StandardQuestion,
				Source:          "faq",
				KnowledgeBaseID: chunk.KnowledgeBaseID,
			})
		}
	}

	// Collect Document chunks with generated questions
	for groupKnowledgeDomainID, groupKBIDs := range kbGroups {
		docChunks, err := s.chunkRepo.ListRecentDocumentChunksWithQuestions(ctx, groupKnowledgeDomainID, groupKBIDs, queryKnowledgeIDs, fetchLimit)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id":            agentID,
				"knowledge_domain_id": groupKnowledgeDomainID,
			})
			continue
		}
		for _, chunk := range docChunks {
			meta, err := chunk.DocumentMetadata()
			if err != nil || meta == nil || len(meta.GeneratedQuestions) == 0 {
				continue
			}
			q := meta.GeneratedQuestions[0].Question
			if q == "" || seen[q] {
				continue
			}
			seen[q] = true
			buckets[chunk.KnowledgeID] = append(buckets[chunk.KnowledgeID], types.SuggestedQuestion{
				Question:        q,
				Source:          "document",
				KnowledgeBaseID: chunk.KnowledgeBaseID,
			})
		}
	}

	// 4. Shuffle within each bucket, then round-robin across buckets
	//    to ensure diversity across different documents.
	bucketKeys := make([]string, 0, len(buckets))
	for k, qs := range buckets {
		bucketKeys = append(bucketKeys, k)
		rand.Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
		buckets[k] = qs
	}
	rand.Shuffle(len(bucketKeys), func(i, j int) {
		bucketKeys[i], bucketKeys[j] = bucketKeys[j], bucketKeys[i]
	})

	// Round-robin pick one question from each document in turn.
	offsets := make(map[string]int, len(bucketKeys))
	for len(result) < limit {
		picked := false
		for _, key := range bucketKeys {
			if len(result) >= limit {
				break
			}
			qs := buckets[key]
			idx := offsets[key]
			if idx < len(qs) {
				result = append(result, qs[idx])
				offsets[key] = idx + 1
				picked = true
			}
		}
		if !picked {
			break
		}
	}

	return s.truncateQuestions(result, limit), nil
}

func (s *customAgentService) authorizeSuggestedQuestionTargets(
	ctx context.Context,
	kbIDs []string,
	knowledgeIDs []string,
) error {
	targetIDs := normalizedAgentKBIDs(kbIDs)
	for _, knowledgeID := range normalizedAgentKBIDs(knowledgeIDs) {
		if s.knowledgeRepo == nil {
			return ErrEnterpriseAccessDenied
		}
		knowledge, err := s.knowledgeRepo.GetKnowledgeByIDOnly(ctx, knowledgeID)
		if err != nil || knowledge == nil {
			return ErrEnterpriseAccessDenied
		}
		targetIDs = append(targetIDs, knowledge.KnowledgeBaseID)
	}
	targetIDs = normalizedAgentKBIDs(targetIDs)
	if len(targetIDs) == 0 {
		return nil
	}
	kbs, err := s.kbService.GetKnowledgeBasesByIDsOnly(ctx, targetIDs)
	if err != nil {
		return err
	}
	byID := make(map[string]*types.KnowledgeBase, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			byID[kb.ID] = kb
		}
	}
	if len(byID) != len(targetIDs) {
		return ErrEnterpriseAccessDenied
	}

	for _, id := range targetIDs {
		kb := byID[id]
		if s.accessService != nil {
			allowed, accessErr := s.accessService.CanReadKnowledgeBase(ctx, kb)
			if accessErr != nil {
				return accessErr
			}
			if !allowed {
				return ErrEnterpriseAccessDenied
			}
		}
	}
	return nil
}

func (s *customAgentService) resolveKnowledgeIDsFromTags(
	ctx context.Context,
	tagIDs []string,
) ([]string, error) {
	if len(tagIDs) == 0 || s.tagRepo == nil || s.knowledgeRepo == nil {
		return nil, nil
	}
	tags, err := s.tagRepo.GetByIDsOnly(ctx, tagIDs)
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, nil
	}
	byDomainAndKB := make(map[uint64]map[string][]string)
	for _, tag := range tags {
		if byDomainAndKB[tag.KnowledgeDomainID] == nil {
			byDomainAndKB[tag.KnowledgeDomainID] = make(map[string][]string)
		}
		byDomainAndKB[tag.KnowledgeDomainID][tag.KnowledgeBaseID] = append(
			byDomainAndKB[tag.KnowledgeDomainID][tag.KnowledgeBaseID],
			tag.ID,
		)
	}
	var out []string
	for knowledgeDomainID, byKB := range byDomainAndKB {
		ids, err := mergeKnowledgeIDsFromTagGroups(ctx, s.knowledgeRepo, knowledgeDomainID, byKB)
		if err != nil {
			return nil, err
		}
		out = mergeUniqueStrings(out, ids)
	}
	return out, nil
}

func (s *customAgentService) groupSuggestedQuestionTargetsByDomain(
	ctx context.Context,
	knowledgeBaseIDs []string,
	knowledgeIDs []string,
) (map[uint64][]string, error) {
	groups := make(map[uint64][]string)
	seenKBs := make(map[string]bool)

	if len(knowledgeBaseIDs) > 0 {
		kbs, err := s.kbService.GetKnowledgeBasesByIDsOnly(ctx, normalizedAgentKBIDs(knowledgeBaseIDs))
		if err != nil {
			return nil, err
		}
		for _, kb := range kbs {
			if kb == nil || seenKBs[kb.ID] {
				continue
			}
			seenKBs[kb.ID] = true
			groups[kb.KnowledgeDomainID] = append(groups[kb.KnowledgeDomainID], kb.ID)
		}
	}

	for _, knowledgeID := range normalizedAgentKBIDs(knowledgeIDs) {
		knowledge, err := s.knowledgeRepo.GetKnowledgeByIDOnly(ctx, knowledgeID)
		if err != nil {
			return nil, err
		}
		if knowledge == nil {
			continue
		}
		if !seenKBs[knowledge.KnowledgeBaseID] {
			seenKBs[knowledge.KnowledgeBaseID] = true
			groups[knowledge.KnowledgeDomainID] = append(
				groups[knowledge.KnowledgeDomainID],
				knowledge.KnowledgeBaseID,
			)
		}
	}
	return groups, nil
}

func mergeKnowledgeIDsFromTagGroups(
	ctx context.Context,
	knowledgeRepo interfaces.KnowledgeRepository,
	knowledgeDomainID uint64,
	byKB map[string][]string,
) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for kbID, ids := range byKB {
		kids, err := knowledgeRepo.ListIDsByTagIDs(ctx, knowledgeDomainID, kbID, ids)
		if err != nil {
			return nil, err
		}
		for _, kid := range kids {
			if !seen[kid] {
				seen[kid] = true
				out = append(out, kid)
			}
		}
	}
	return out, nil
}

func mergeUniqueStrings(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, s := range base {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range extra {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// truncateQuestions truncates the question list to the specified limit
func (s *customAgentService) truncateQuestions(questions []types.SuggestedQuestion, limit int) []types.SuggestedQuestion {
	if len(questions) > limit {
		return questions[:limit]
	}
	return questions
}
