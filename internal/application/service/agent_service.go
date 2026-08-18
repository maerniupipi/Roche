package service

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/agent"
	"roche.local/knowledge-agent-platform/internal/agent/approval"
	"roche.local/knowledge-agent-platform/internal/agent/skills"
	"roche.local/knowledge-agent-platform/internal/agent/tools"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/mcp"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

const MAX_ITERATIONS = 100 // Max iterations for agent execution

// dedupStrings removes duplicate strings while preserving the first occurrence order.
func dedupStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// agentHasKnowledgeScope reports whether the current user has any effective
// knowledge scope available to this agent execution.
func agentHasKnowledgeScope(config *types.AgentConfig) bool {
	if config == nil {
		return false
	}
	if len(config.KnowledgeBases) > 0 || len(config.KnowledgeIDs) > 0 {
		return true
	}
	return len(config.SearchTargets) > 0
}

// knowledgeBaseIDsForPrompt returns KB IDs to show in runtime_context metadata.
// Prefer explicit KnowledgeBases; fall back to deduped IDs from SearchTargets.
func knowledgeBaseIDsForPrompt(config *types.AgentConfig) []string {
	if config == nil {
		return nil
	}
	if len(config.KnowledgeBases) > 0 {
		return config.KnowledgeBases
	}
	if len(config.SearchTargets) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(config.SearchTargets))
	out := make([]string, 0, len(config.SearchTargets))
	for _, target := range config.SearchTargets {
		if target == nil || target.KnowledgeBaseID == "" {
			continue
		}
		if _, ok := seen[target.KnowledgeBaseID]; ok {
			continue
		}
		seen[target.KnowledgeBaseID] = struct{}{}
		out = append(out, target.KnowledgeBaseID)
	}
	return out
}

// agentService implements agent-related business logic
type agentService struct {
	cfg                    *config.Config
	modelService           interfaces.ModelService
	mcpServiceService      interfaces.MCPServiceService
	mcpManager             *mcp.MCPManager
	eventBus               *event.EventBus
	db                     *gorm.DB
	webSearchService       interfaces.WebSearchService
	knowledgeBaseService   interfaces.KnowledgeBaseService
	knowledgeService       interfaces.KnowledgeService
	graphRepository        interfaces.RetrieveGraphRepository
	fileService            interfaces.FileService
	chunkService           interfaces.ChunkService
	duckdb                 *sql.DB
	webSearchStateService  interfaces.WebSearchStateService
	knowledgeDomainService interfaces.KnowledgeDomainService
	toolApprovalGate       approval.MCPApproval
}

// NewAgentService creates a new agent service
func NewAgentService(
	cfg *config.Config,
	modelService interfaces.ModelService,
	knowledgeBaseService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
	graphRepository interfaces.RetrieveGraphRepository,
	fileService interfaces.FileService,
	chunkService interfaces.ChunkService,
	mcpServiceService interfaces.MCPServiceService,
	mcpManager *mcp.MCPManager,
	eventBus *event.EventBus,
	db *gorm.DB,
	webSearchService interfaces.WebSearchService,
	duckdb *sql.DB,
	webSearchStateService interfaces.WebSearchStateService,
	knowledgeDomainService interfaces.KnowledgeDomainService,
	toolApprovalGate approval.MCPApproval,
) interfaces.AgentService {
	return &agentService{
		cfg:                    cfg,
		modelService:           modelService,
		knowledgeBaseService:   knowledgeBaseService,
		knowledgeService:       knowledgeService,
		graphRepository:        graphRepository,
		fileService:            fileService,
		chunkService:           chunkService,
		mcpServiceService:      mcpServiceService,
		mcpManager:             mcpManager,
		eventBus:               eventBus,
		db:                     db,
		webSearchService:       webSearchService,
		duckdb:                 duckdb,
		webSearchStateService:  webSearchStateService,
		knowledgeDomainService: knowledgeDomainService,
		toolApprovalGate:       toolApprovalGate,
	}
}

// CreateAgentEngine creates an agent engine with the given configuration and EventBus.
// History is loaded once per turn by the caller (see service.LoadAgentHistory)
// and handed to AgentEngine.Execute as llmContext; the engine is stateless across turns.
func (s *agentService) CreateAgentEngine(
	ctx context.Context,
	config *types.AgentConfig,
	chatModel chat.Chat,
	rerankModel rerank.Reranker,
	eventBus *event.EventBus,
	sessionID, assistantMessageID string,
) (interfaces.AgentEngine, error) {
	logger.Infof(ctx, "Creating agent engine with custom EventBus")

	// 1. Validate config
	if err := s.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid agent config: %w", err)
	}
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil after initialization")
	}

	// 2. Build tool registry
	toolRegistry := tools.NewToolRegistry()
	if config.MaxToolOutputChars > 0 {
		toolRegistry.SetMaxToolOutputSize(config.MaxToolOutputChars)
	}
	if err := s.registerTools(ctx, toolRegistry, config, rerankModel, chatModel, sessionID); err != nil {
		return nil, fmt.Errorf("failed to register tools: %w", err)
	}
	if s.cfg.IsMCPEnabled() {
		s.registerMCPTools(ctx, toolRegistry, config, eventBus, sessionID, assistantMessageID)
	}

	// 3. Resolve knowledge base and selected document metadata
	kbInfos, selectedDocs := s.resolveKBAndDocInfos(ctx, config)

	// 4. Resolve system prompt template
	systemPromptTemplate := ""
	if config.UseCustomSystemPrompt || config.SystemPrompt != "" {
		systemPromptTemplate = config.SystemPrompt
	}

	// 5. Create engine
	engine := agent.NewAgentEngine(
		config, chatModel, toolRegistry, eventBus,
		kbInfos, selectedDocs, sessionID,
		systemPromptTemplate,
	)
	engine.SetAppConfig(s.cfg)
	var pinnedMCP []*agent.PinnedMCPServiceInfo
	if s.cfg.IsMCPEnabled() {
		pinnedMCP = s.resolvePinnedMCPServiceInfos(ctx, config)
		s.attachPinnedMCPToolNames(toolRegistry, pinnedMCP)
	}
	var pinnedSkills []*agent.PinnedSkillInfo
	if s.cfg.AreSkillsEnabled() {
		pinnedSkills = s.resolvePinnedSkillInfos(config)
	}
	engine.SetPinnedMentions(pinnedMCP, pinnedSkills)

	// Set VLM image describer for MCP tool result image analysis.
	// When an MCP tool returns images, the engine uses VLM to generate text descriptions
	// and appends them to the tool result content (since Chat Completions API does not
	// reliably support images in tool role messages across providers).
	if config.VLMModelID != "" {
		if vlmModel, err := s.modelService.GetVLMModel(ctx, config.VLMModelID); err == nil {
			engine.SetImageDescriber(func(ctx context.Context, imgBytes []byte, prompt string) (string, error) {
				return vlmModel.Predict(ctx, [][]byte{imgBytes}, prompt)
			})
			logger.Infof(ctx, "VLM image describer set for MCP tool result analysis (model: %s)", config.VLMModelID)
		} else {
			logger.Warnf(ctx, "Failed to load VLM model %s for MCP image fallback: %v", config.VLMModelID, err)
		}
	}

	// Initialize skills manager if skills are enabled
	if s.cfg.AreSkillsEnabled() && config.SkillsEnabled && len(config.SkillDirs) > 0 {
		skillsManager, err := s.initializeSkillsManager(ctx, config, toolRegistry)
		if err != nil {
			logger.Warnf(ctx, "Failed to initialize skills manager: %v", err)
		} else if skillsManager != nil {
			engine.SetSkillsManager(skillsManager)
			logger.Infof(ctx, "Skills manager initialized with %d skills",
				len(skillsManager.GetAllMetadata()))
		}
	}

	return engine, nil
}

// registerMCPTools registers MCP tools from enabled services for this knowledgeDomain.
func (s *agentService) registerMCPTools(
	ctx context.Context,
	toolRegistry *tools.ToolRegistry,
	config *types.AgentConfig,
	eventBus *event.EventBus,
	sessionID, assistantMessageID string,
) {
	knowledgeDomainID := uint64(0)
	if tid, ok := types.KnowledgeDomainIDFromContext(ctx); ok {
		knowledgeDomainID = tid
	}
	if knowledgeDomainID == 0 || s.mcpServiceService == nil || s.mcpManager == nil {
		return
	}

	mcpMode := config.MCPSelectionMode
	if mcpMode == "" {
		mcpMode = "all"
	}
	if mcpMode == "none" {
		logger.Infof(ctx, "MCP services disabled by agent config (mode: none)")
		return
	}

	var mcpServices []*types.MCPService
	var err error

	if mcpMode == "selected" {
		if len(config.MCPServices) == 0 {
			logger.Infof(ctx, "MCP services disabled by agent config (mode: selected, no services)")
			return
		}
		mcpServices, err = s.mcpServiceService.ListMCPServicesByIDs(ctx, knowledgeDomainID, config.MCPServices)
		if err != nil {
			logger.Warnf(ctx, "Failed to list selected MCP services: %v", err)
			return
		}
		logger.Infof(ctx, "Using %d selected MCP services from agent config", len(mcpServices))
	} else {
		mcpServices, err = s.mcpServiceService.ListMCPServices(ctx, knowledgeDomainID)
		if err != nil {
			logger.Warnf(ctx, "Failed to list MCP services: %v", err)
			return
		}
	}

	enabledServices := make([]*types.MCPService, 0)
	for _, svc := range mcpServices {
		if svc != nil && svc.Enabled {
			enabledServices = append(enabledServices, svc)
		}
	}
	if len(enabledServices) > 0 {
		var regCtx *tools.MCPOAuthSession
		if eventBus != nil && sessionID != "" && assistantMessageID != "" {
			regCtx = &tools.MCPOAuthSession{
				EventBus:               eventBus,
				SessionID:              sessionID,
				AssistantMessageID:     assistantMessageID,
				ApprovalCtx:            ctx,
				AuthWaitTimeoutSeconds: config.MCPAuthWaitTimeout,
			}
		}
		registered, err := tools.RegisterMCPTools(
			ctx, toolRegistry, enabledServices, s.mcpManager, s.toolApprovalGate, regCtx,
		)
		if err != nil {
			logger.Warnf(ctx, "Failed to register MCP tools: %v", err)
		} else if registered == 0 {
			logger.Warnf(ctx, "No MCP tools registered from %d enabled service(s)", len(enabledServices))
		} else {
			logger.Infof(ctx, "Registered %d MCP tool(s) from %d enabled service(s)", registered, len(enabledServices))
		}
	}
}

// resolveKBAndDocInfos loads knowledge base metadata and selected document info for prompt.
func (s *agentService) resolveKBAndDocInfos(
	ctx context.Context,
	config *types.AgentConfig,
) ([]*agent.KnowledgeBaseInfo, []*agent.SelectedDocumentInfo) {
	kbIDs := knowledgeBaseIDsForPrompt(config)
	kbInfos, err := s.getKnowledgeBaseInfos(ctx, kbIDs, config.SearchTargets)
	if err != nil {
		logger.Warnf(ctx, "Failed to get knowledge base details, using IDs only: %v", err)
		kbInfos = make([]*agent.KnowledgeBaseInfo, 0, len(kbIDs))
		for _, kbID := range kbIDs {
			kbInfos = append(kbInfos, &agent.KnowledgeBaseInfo{
				ID:          kbID,
				Name:        kbID,
				Description: "",
				DocCount:    0,
			})
		}
	}

	selectedDocs, err := s.getSelectedDocumentInfos(ctx, config.KnowledgeIDs, config.SearchTargets)
	if err != nil {
		logger.Warnf(ctx, "Failed to get selected document details: %v", err)
		selectedDocs = []*agent.SelectedDocumentInfo{}
	}

	return kbInfos, selectedDocs
}

// initializeSkillsManager creates and initializes the skills manager
func (s *agentService) initializeSkillsManager(
	ctx context.Context,
	config *types.AgentConfig,
	toolRegistry *tools.ToolRegistry,
) (*skills.Manager, error) {
	// Enterprise skills are instruction-only. Script execution and sandbox
	// managers are deliberately not registered.
	skillsConfig := &skills.ManagerConfig{
		SkillDirs:     config.SkillDirs,
		AllowedSkills: config.AllowedSkills,
		Enabled:       config.SkillsEnabled,
	}

	skillsManager := skills.NewManager(skillsConfig, nil)

	// Initialize (discover skills)
	if err := skillsManager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skills: %w", err)
	}

	bindings, err := skillsManager.GetAllSkillToolBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load skill tool bindings: %w", err)
	}
	for skillName, toolNames := range bindings {
		if len(toolNames) == 0 {
			continue
		}
		if err := toolRegistry.BindSkillTools(skillName, toolNames); err != nil {
			return nil, fmt.Errorf("failed to bind tools for skill %s: %w", skillName, err)
		}
	}

	// Register skills tools
	readSkillTool := tools.NewReadSkillTool(skillsManager, toolRegistry)
	toolRegistry.RegisterTool(readSkillTool)
	logger.Infof(ctx, "Registered read_skill tool")

	return skillsManager, nil
}

// registerTools registers tools based on the agent configuration
func (s *agentService) registerTools(
	ctx context.Context,
	registry *tools.ToolRegistry,
	config *types.AgentConfig,
	rerankModel rerank.Reranker,
	chatModel chat.Chat,
	sessionID string,
) error {
	// Source of truth policy:
	//   - `config.AllowedTools` is the explicit, user-editable whitelist —
	//     populated by the agent-type preset on create and freely editable
	//     afterwards.
	//   - We never silently *inject* tools the user didn't pick.
	//   - We still *filter out* tools whose capability prerequisites are missing
	//     (for example, no KB in scope) so the LLM can't call tools
	//     that would error at runtime.
	//   - Legacy agents without AllowedTools fall back to DefaultAllowedTools().
	var allowedTools []string
	if len(config.AllowedTools) > 0 {
		allowedTools = make([]string, len(config.AllowedTools))
		copy(allowedTools, config.AllowedTools)
		logger.Infof(ctx, "Using custom allowed tools from config: %v", allowedTools)
	} else {
		allowedTools = tools.DefaultAllowedTools()
		logger.Infof(ctx, "Using default allowed tools: %v", allowedTools)
	}

	// ---- Capability detection from SearchTargets ----
	var hasVectorKB bool
	var hasGraphKB bool
	for _, target := range config.SearchTargets {
		kb, err := s.knowledgeBaseService.GetKnowledgeBaseByIDOnly(ctx, target.KnowledgeBaseID)
		if err != nil {
			continue
		}
		if kb.IsVectorEnabled() || kb.IsKeywordEnabled() {
			hasVectorKB = true
		}
		if kb.ExtractConfig != nil &&
			(kb.ExtractConfig.Enabled ||
				len(kb.ExtractConfig.Nodes) > 0 ||
				len(kb.ExtractConfig.Relations) > 0) {
			hasGraphKB = true
		}
	}

	// Filter out knowledge base tools if no knowledge scope is configured for this turn.
	hasKnowledge := agentHasKnowledgeScope(config)
	if !hasKnowledge {
		filteredTools := make([]string, 0)
		kbTools := map[string]bool{
			tools.ToolKnowledgeSearch:     true,
			tools.ToolGrepChunks:          true,
			tools.ToolListKnowledgeChunks: true,
			tools.ToolQueryKnowledgeGraph: true,
			tools.ToolGetDocumentInfo:     true,
			tools.ToolDatabaseQuery:       true,
			tools.ToolDataAnalysis:        true,
			tools.ToolDataSchema:          true,
		}

		// If no knowledge and no web search, also disable todo_write (not useful for simple chat)
		if !config.WebSearchEnabled {
			kbTools[tools.ToolTodoWrite] = true
		}

		for _, toolName := range allowedTools {
			if !kbTools[toolName] {
				filteredTools = append(filteredTools, toolName)
			}
		}
		allowedTools = filteredTools
		logger.Infof(ctx, "Pure Agent Mode: Knowledge base tools filtered out, remaining: %v", allowedTools)
	}

	// If web search is enabled, add web_search to allowedTools
	if config.WebSearchEnabled {
		allowedTools = append(allowedTools, tools.ToolWebSearch)
		allowedTools = append(allowedTools, tools.ToolWebFetch)
	}

	// Tool capability sets — used by the hard safety nets below to drop tools
	// whose runtime prerequisite (a matching KB surface) is missing.
	//
	// NOTE: ragToolSet must stay in sync with frontend `knowledgeBaseTools`
	// in AgentEditorModal.vue. These are *all* tools that retrieve/inspect
	// content from RAG-style knowledge bases.
	ragToolSet := map[string]bool{
		tools.ToolKnowledgeSearch:     true,
		tools.ToolGrepChunks:          true,
		tools.ToolListKnowledgeChunks: true,
		tools.ToolQueryKnowledgeGraph: true,
		tools.ToolGetDocumentInfo:     true,
		tools.ToolDatabaseQuery:       true,
	}
	// Hard safety nets: drop tools whose runtime prerequisite is missing.
	// This guards against stale configs whose selected KBs no longer expose
	// chunk retrieval.
	if !hasVectorKB {
		filtered := make([]string, 0, len(allowedTools))
		dropped := make([]string, 0)
		for _, t := range allowedTools {
			if t == tools.ToolQueryKnowledgeGraph && hasGraphKB {
				filtered = append(filtered, t)
				continue
			}
			if ragToolSet[t] {
				dropped = append(dropped, t)
				continue
			}
			filtered = append(filtered, t)
		}
		allowedTools = filtered
		if len(dropped) > 0 {
			logger.Warnf(ctx, "Dropped RAG tools %v because no RAG-capable KB is in scope", dropped)
		}
	}

	// Deduplicate while preserving original order.
	allowedTools = dedupStrings(allowedTools)

	// logger.Infof(ctx, "Registering tools: %v, webSearchEnabled: %v", allowedTools, config.WebSearchEnabled)
	// Register each allowed tool
	for _, toolName := range allowedTools {
		var toolToRegister types.Tool

		switch toolName {
		case tools.ToolThinking:
			toolToRegister = tools.NewSequentialThinkingTool()
		case tools.ToolTodoWrite:
			toolToRegister = tools.NewTodoWriteTool()
		case tools.ToolKnowledgeSearch:
			toolToRegister = tools.NewKnowledgeSearchTool(
				s.knowledgeBaseService,
				s.knowledgeService,
				s.chunkService,
				config.SearchTargets,
				rerankModel,
				chatModel,
				s.cfg,
			)
		case tools.ToolGrepChunks:
			toolToRegister = tools.NewGrepChunksTool(s.db, config.SearchTargets)
			logger.Infof(ctx, "Registered grep_chunks tool with searchTargets: %d targets", len(config.SearchTargets))
		case tools.ToolListKnowledgeChunks:
			toolToRegister = tools.NewListKnowledgeChunksTool(s.knowledgeService, s.chunkService, config.SearchTargets)
		case tools.ToolQueryKnowledgeGraph:
			toolToRegister = tools.NewScopedQueryKnowledgeGraphTool(
				s.knowledgeBaseService,
				s.graphRepository,
				config.SearchTargets,
			)
		case tools.ToolGetDocumentInfo:
			toolToRegister = tools.NewGetDocumentInfoTool(s.knowledgeService, s.chunkService, config.SearchTargets)
		case tools.ToolDatabaseQuery:
			toolToRegister = tools.NewDatabaseQueryTool(s.db, config.SearchTargets)
		case tools.ToolWebSearch:
			toolToRegister = tools.NewWebSearchTool(
				s.webSearchService,
				s.knowledgeBaseService,
				s.knowledgeService,
				s.webSearchStateService,
				sessionID,
				config.WebSearchMaxResults,
				config.WebSearchProviderID,
			)
			logger.Infof(ctx, "Registered web_search tool for session: %s, maxResults: %d, providerID: %s", sessionID, config.WebSearchMaxResults, config.WebSearchProviderID)

		case tools.ToolWebFetch:
			toolToRegister = tools.NewWebFetchTool(chatModel)
			logger.Infof(ctx, "Registered web_fetch tool for session: %s", sessionID)

		case tools.ToolDataAnalysis:
			toolToRegister = tools.NewDataAnalysisTool(s.knowledgeBaseService, s.knowledgeService, s.knowledgeDomainService, s.fileService, s.duckdb, sessionID, config.SearchTargets)
			logger.Infof(ctx, "Registered data_analysis tool for session: %s", sessionID)

		case tools.ToolDataSchema:
			toolToRegister = tools.NewScopedDataSchemaTool(s.knowledgeService, s.chunkService.GetRepository(), config.SearchTargets)
			logger.Infof(ctx, "Registered data_schema tool")

		case tools.ToolTextCounter:
			toolToRegister = tools.NewTextCounterTool()
			logger.Infof(ctx, "Registered text_counter tool")

		default:
			logger.Warnf(ctx, "Unknown tool: %s", toolName)
		}

		if toolToRegister != nil {
			if toolToRegister.Name() != toolName {
				logger.Warnf(ctx, "Tool name mismatch: expected %s, got %s", toolName, toolToRegister.Name())
			}
			registry.RegisterTool(toolToRegister)
		}
	}

	logger.Infof(ctx, "Registered %d tools", len(registry.ListTools()))
	return nil
}

// ValidateConfig validates the agent configuration
func (s *agentService) ValidateConfig(config *types.AgentConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if config.MaxIterations <= 0 {
		config.MaxIterations = 5 // Default
	}

	if config.MaxIterations > MAX_ITERATIONS {
		return fmt.Errorf("max iterations too high: %d (max %d)", config.MaxIterations, MAX_ITERATIONS)
	}

	return nil
}

// getKnowledgeBaseInfos retrieves detailed information for knowledge bases
func (s *agentService) getKnowledgeBaseInfos(
	ctx context.Context,
	kbIDs []string,
	searchTargets types.SearchTargets,
) ([]*agent.KnowledgeBaseInfo, error) {
	if len(kbIDs) == 0 {
		return []*agent.KnowledgeBaseInfo{}, nil
	}

	kbInfos := make([]*agent.KnowledgeBaseInfo, 0, len(kbIDs))

	for _, kbID := range kbIDs {
		// Get knowledge base details
		kb, err := s.knowledgeBaseService.GetKnowledgeBaseByID(ctx, kbID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get knowledge base %s: %v", secutils.SanitizeForLog(kbID), err)
			kbInfos = append(kbInfos, &agent.KnowledgeBaseInfo{
				ID:          kbID,
				Name:        kbID,
				Type:        "document",
				Description: "",
				DocCount:    0,
				RecentDocs:  []agent.RecentDocInfo{},
			})
			continue
		}

		// Skip temporary knowledge bases.
		if kb.IsTemporary {
			logger.Debugf(ctx, "Skipping temporary knowledge base %s (%s) from prompt", kb.ID, kb.Name)
			continue
		}

		// Get document count and recent documents
		docCount := 0
		recentDocs := []agent.RecentDocInfo{}

		fullAccess, allowedKnowledgeIDs, scoped := knowledgeScopeForPrompt(searchTargets, kbID)
		kbCtx := context.WithValue(ctx, types.KnowledgeDomainIDContextKey, kb.KnowledgeDomainID)

		if kb.Type == types.KnowledgeBaseTypeFAQ && (!scoped || fullAccess) {
			pageResult, err := s.knowledgeService.ListFAQEntries(kbCtx, kbID, &types.Pagination{
				Page:     1,
				PageSize: 10,
			}, 0, "", "", "")
			if err == nil && pageResult != nil {
				docCount = int(pageResult.Total)
				if entries, ok := pageResult.Data.([]*types.FAQEntry); ok {
					for _, entry := range entries {
						if len(recentDocs) >= 10 {
							break
						}
						recentDocs = append(recentDocs, agent.RecentDocInfo{
							ChunkID:             entry.ChunkID,
							KnowledgeID:         entry.KnowledgeID,
							KnowledgeBaseID:     entry.KnowledgeBaseID,
							Title:               entry.StandardQuestion,
							Type:                string(types.ChunkTypeFAQ),
							CreatedAt:           entry.CreatedAt.Format("2006-01-02"),
							FAQStandardQuestion: entry.StandardQuestion,
							FAQSimilarQuestions: entry.SimilarQuestions,
							FAQAnswers:          entry.Answers,
						})
					}
				}
			} else if err != nil {
				logger.Warnf(ctx, "Failed to list FAQ entries for %s: %v", kbID, err)
			}
		}

		// Fallback to generic knowledge listing when not FAQ or FAQ retrieval failed
		if kb.Type != types.KnowledgeBaseTypeFAQ || len(recentDocs) == 0 {
			filter := types.KnowledgeListFilter{
				ParseStatus: types.ParseStatusCompleted,
			}
			if scoped && !fullAccess {
				filter.AllowedKnowledgeIDs = allowedKnowledgeIDs
			}
			pageResult, err := s.knowledgeService.ListPagedKnowledgeByKnowledgeBaseID(kbCtx, kbID, &types.Pagination{
				Page:     1,
				PageSize: 10,
			}, filter)

			if err == nil && pageResult != nil {
				docCount = int(pageResult.Total)

				// Convert to Knowledge slice
				if knowledges, ok := pageResult.Data.([]*types.Knowledge); ok {
					for _, k := range knowledges {
						if len(recentDocs) >= 10 {
							break
						}
						recentDocs = append(recentDocs, agent.RecentDocInfo{
							KnowledgeID: k.ID,
							Title:       k.Title,
							Description: k.Description,
							FileName:    k.FileName,
							Type:        k.FileType,
							CreatedAt:   k.CreatedAt.Format("2006-01-02"),
							FileSize:    k.FileSize,
						})
					}
				}
			}
		}

		kbType := kb.Type
		if kbType == "" {
			kbType = "document" // Default type
		}
		kbInfos = append(kbInfos, &agent.KnowledgeBaseInfo{
			ID:           kb.ID,
			Name:         kb.Name,
			Type:         kbType,
			Description:  kb.Description,
			DocCount:     docCount,
			Capabilities: kbRetrievalCapabilities(kb),
			RecentDocs:   recentDocs,
		})
	}

	return kbInfos, nil
}

func knowledgeScopeForPrompt(
	searchTargets types.SearchTargets,
	kbID string,
) (fullAccess bool, knowledgeIDs []string, scoped bool) {
	if len(searchTargets) == 0 {
		return true, nil, false
	}
	for _, target := range searchTargets {
		if target == nil || target.KnowledgeBaseID != kbID {
			continue
		}
		scoped = true
		if target.Type == types.SearchTargetTypeKnowledgeBase && len(target.TagIDs) == 0 {
			return true, nil, true
		}
		knowledgeIDs = append(knowledgeIDs, target.KnowledgeIDs...)
	}
	return false, dedupStrings(knowledgeIDs), scoped
}

// kbRetrievalCapabilities reports which retrieval surfaces a KB exposes.
// Returned values currently contain "chunks" when the KB has vector or
// keyword indexing enabled.
func kbRetrievalCapabilities(kb *types.KnowledgeBase) []string {
	if kb == nil {
		return nil
	}
	caps := make([]string, 0, 1)
	if kb.IsVectorEnabled() || kb.IsKeywordEnabled() {
		caps = append(caps, "chunks")
	}
	return caps
}

// getSelectedDocumentInfos retrieves metadata for document-scoped grants.
func (s *agentService) getSelectedDocumentInfos(
	ctx context.Context,
	knowledgeIDs []string,
	searchTargets types.SearchTargets,
) ([]*agent.SelectedDocumentInfo, error) {
	if len(knowledgeIDs) == 0 {
		return []*agent.SelectedDocumentInfo{}, nil
	}

	requested := make(map[string]bool, len(knowledgeIDs))
	for _, id := range knowledgeIDs {
		requested[id] = true
	}

	idsByKnowledgeDomain := make(map[uint64][]string)
	for _, target := range searchTargets {
		if target == nil || target.Type != types.SearchTargetTypeKnowledge {
			continue
		}
		for _, id := range target.KnowledgeIDs {
			if requested[id] {
				idsByKnowledgeDomain[target.KnowledgeDomainID] = append(idsByKnowledgeDomain[target.KnowledgeDomainID], id)
			}
		}
	}
	if len(idsByKnowledgeDomain) == 0 {
		knowledgeDomainID, _ := types.KnowledgeDomainIDFromContext(ctx)
		idsByKnowledgeDomain[knowledgeDomainID] = knowledgeIDs
	}

	var knowledges []*types.Knowledge
	for knowledgeDomainID, ids := range idsByKnowledgeDomain {
		batch, err := s.knowledgeService.GetKnowledgeBatch(ctx, knowledgeDomainID, dedupStrings(ids))
		if err != nil {
			return nil, fmt.Errorf("get authorized knowledge batch for knowledgeDomain %d: %w", knowledgeDomainID, err)
		}
		knowledges = append(knowledges, batch...)
	}

	// Build map for quick lookup
	knowledgeMap := make(map[string]*types.Knowledge)
	for _, k := range knowledges {
		if k != nil {
			knowledgeMap[k.ID] = k
		}
	}

	selectedDocs := make([]*agent.SelectedDocumentInfo, 0, len(knowledgeIDs))

	for _, kid := range knowledgeIDs {
		k, ok := knowledgeMap[kid]
		if !ok {
			logger.Warnf(ctx, "Selected knowledge %s not found", kid)
			continue
		}

		docInfo := &agent.SelectedDocumentInfo{
			KnowledgeID:     k.ID,
			KnowledgeBaseID: k.KnowledgeBaseID,
			Title:           k.Title,
			FileName:        k.FileName,
			FileType:        k.FileType,
		}

		selectedDocs = append(selectedDocs, docInfo)
	}

	logger.Infof(ctx, "Loaded %d selected documents metadata for prompt", len(selectedDocs))
	return selectedDocs, nil
}

func (s *agentService) resolvePinnedMCPServiceInfos(
	ctx context.Context,
	config *types.AgentConfig,
) []*agent.PinnedMCPServiceInfo {
	if len(config.PinnedMCPServiceIDs) == 0 || s.mcpServiceService == nil {
		return nil
	}
	knowledgeDomainID := uint64(0)
	if tid, ok := types.KnowledgeDomainIDFromContext(ctx); ok {
		knowledgeDomainID = tid
	}
	if knowledgeDomainID == 0 {
		return fallbackPinnedMCPInfos(config.PinnedMCPServiceIDs)
	}

	services, err := s.mcpServiceService.ListMCPServicesByIDs(ctx, knowledgeDomainID, config.PinnedMCPServiceIDs)
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve pinned MCP services: %v", err)
		return fallbackPinnedMCPInfos(config.PinnedMCPServiceIDs)
	}
	byID := make(map[string]*types.MCPService, len(services))
	for _, svc := range services {
		if svc != nil {
			byID[svc.ID] = svc
		}
	}
	result := make([]*agent.PinnedMCPServiceInfo, 0, len(config.PinnedMCPServiceIDs))
	for _, id := range config.PinnedMCPServiceIDs {
		if id == "" {
			continue
		}
		if svc, ok := byID[id]; ok {
			result = append(result, &agent.PinnedMCPServiceInfo{
				ID:          svc.ID,
				Name:        svc.Name,
				Description: svc.Description,
			})
			continue
		}
		result = append(result, &agent.PinnedMCPServiceInfo{ID: id, Name: id})
	}
	return result
}

func (s *agentService) attachPinnedMCPToolNames(
	registry *tools.ToolRegistry,
	pinned []*agent.PinnedMCPServiceInfo,
) {
	if registry == nil || len(pinned) == 0 {
		return
	}
	byService := tools.MCPToolNamesByServiceID(registry)
	for _, info := range pinned {
		if info == nil || info.ID == "" {
			continue
		}
		info.ToolNames = append([]string(nil), byService[info.ID]...)
	}
}

func fallbackPinnedMCPInfos(ids []string) []*agent.PinnedMCPServiceInfo {
	result := make([]*agent.PinnedMCPServiceInfo, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		result = append(result, &agent.PinnedMCPServiceInfo{ID: id, Name: id})
	}
	return result
}

func (s *agentService) resolvePinnedSkillInfos(config *types.AgentConfig) []*agent.PinnedSkillInfo {
	if len(config.PinnedSkillNames) == 0 {
		return nil
	}

	descByName := make(map[string]string)
	if len(config.SkillDirs) > 0 {
		loader := skills.NewLoader(config.SkillDirs)
		if metadata, err := loader.DiscoverSkills(); err == nil {
			for _, meta := range metadata {
				if meta != nil {
					descByName[meta.Name] = meta.Description
				}
			}
		}
	}

	result := make([]*agent.PinnedSkillInfo, 0, len(config.PinnedSkillNames))
	for _, name := range config.PinnedSkillNames {
		if name == "" {
			continue
		}
		result = append(result, &agent.PinnedSkillInfo{
			Name:        name,
			Description: descByName[name],
		})
	}
	return result
}
