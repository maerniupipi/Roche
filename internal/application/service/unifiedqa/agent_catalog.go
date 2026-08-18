package unifiedqa

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
)

const (
	FinanceAgentID    = config.UnifiedQAFinanceAgentID
	ComplianceAgentID = config.UnifiedQAComplianceAgentID
)

// PromptVersionResolver reports whether a configured prompt version exists.
type PromptVersionResolver func(id string) bool

// DomainAgentProfile is the immutable runtime view of one fixed domain agent.
type DomainAgentProfile struct {
	ID                   string
	Name                 string
	Description          string
	SystemPromptVersion  string
	SearchHints          []string
	ResearchRules        []string
	EvidenceRequirements []string
	AllowedResearchTools []string
	KnowledgeDomainNames []string
	RouteKeywords        []string
	Topics               []TopicProfile
}

type TopicProfile struct {
	ID                        string
	AgentID                   string
	KnowledgeBaseNameContains []string
	RouteKeywords             []string
	NoMatchResponse           map[string]string
	AnswerDisclaimer          map[string]string
	Addenda                   []TopicAddendumProfile
}

type TopicAddendumProfile struct {
	ID              string
	TriggerKeywords []string
	Response        map[string]string
}

// AgentCatalog exposes enabled domain agents in deterministic configuration
// order and provides O(1) lookup for routing and validation.
type AgentCatalog struct {
	version         string
	masterAgentID   string
	agents          []DomainAgentProfile
	byID            map[string]int
	maxSelected     int
	fallbackIDs     []string
	fallbackVersion string
	byTopic         map[string]TopicProfile
	globalFallbacks map[string]map[string]string
}

func NewAgentCatalog(cfg *config.UnifiedQAAgentsConfig, promptExists PromptVersionResolver, fallbackConfigs ...*config.UnifiedQAFallbacksConfig) (*AgentCatalog, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate agent catalog: %w", err)
	}
	if promptExists == nil {
		return nil, fmt.Errorf("validate agent catalog: prompt resolver is required")
	}

	catalog := &AgentCatalog{
		version:         cfg.Version,
		masterAgentID:   cfg.MasterAgentID,
		agents:          make([]DomainAgentProfile, 0, len(cfg.Agents)),
		byID:            make(map[string]int, len(cfg.Agents)),
		maxSelected:     cfg.MaxSelectedAgents,
		fallbackIDs:     slices.Clone(cfg.FallbackAgentIDs),
		byTopic:         make(map[string]TopicProfile),
		globalFallbacks: make(map[string]map[string]string),
	}
	for _, agent := range cfg.Agents {
		if !agent.Enabled {
			continue
		}
		if !promptExists(agent.SystemPromptVersion) {
			return nil, fmt.Errorf("validate agent catalog: prompt version %q for agent %q cannot be resolved", agent.SystemPromptVersion, agent.ID)
		}
		catalog.byID[agent.ID] = len(catalog.agents)
		catalog.agents = append(catalog.agents, profileFromConfig(agent))
	}
	if len(fallbackConfigs) > 0 && fallbackConfigs[0] != nil {
		fallbacks := fallbackConfigs[0]
		catalog.fallbackVersion = fallbacks.Version
		if err := fallbacks.Validate(); err != nil {
			return nil, fmt.Errorf("validate fallback catalog: %w", err)
		}
		for key, localized := range fallbacks.Global {
			catalog.globalFallbacks[key] = cloneStringMap(localized)
		}
		for _, configured := range fallbacks.Topics {
			index, ok := catalog.byID[configured.AgentID]
			if !ok {
				return nil, fmt.Errorf("validate fallback catalog: topic %q references disabled or unknown agent %q", configured.ID, configured.AgentID)
			}
			topic := topicFromConfig(configured)
			if _, duplicate := catalog.byTopic[topic.ID]; duplicate {
				return nil, fmt.Errorf("validate fallback catalog: duplicate topic %q", topic.ID)
			}
			catalog.byTopic[topic.ID] = topic
			catalog.agents[index].Topics = append(catalog.agents[index].Topics, cloneTopic(topic))
		}
	}
	return catalog, nil
}

// LoadAgentCatalog loads the deployment catalog next to config.yaml and
// resolves prompt versions from the already loaded application templates.
func LoadAgentCatalog(appConfig *config.Config) (*AgentCatalog, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("load agent catalog: application config is required")
	}
	cfg, err := config.LoadUnifiedQAAgentsFile(filepath.Join(config.ConfigDir(), "unified_qa_agents.yaml"))
	if err != nil {
		return nil, err
	}
	fallbacks, err := config.LoadUnifiedQAFallbacksFile(filepath.Join(config.ConfigDir(), "unified_qa_fallbacks.yaml"))
	if err != nil {
		return nil, err
	}
	return NewAgentCatalog(cfg, func(id string) bool {
		return config.FindTemplateByID(appConfig.PromptTemplates, id) != nil
	}, fallbacks)
}

func (c *AgentCatalog) Version() string        { return c.version }
func (c *AgentCatalog) MasterAgentID() string  { return c.masterAgentID }
func (c *AgentCatalog) MaxSelectedAgents() int { return c.maxSelected }
func (c *AgentCatalog) FallbackAgentIDs() []string {
	return slices.Clone(c.fallbackIDs)
}

func (c *AgentCatalog) Agents() []DomainAgentProfile {
	agents := make([]DomainAgentProfile, len(c.agents))
	for i := range c.agents {
		agents[i] = cloneProfile(c.agents[i])
	}
	return agents
}

func (c *AgentCatalog) Get(id string) (DomainAgentProfile, bool) {
	index, ok := c.byID[id]
	if !ok {
		return DomainAgentProfile{}, false
	}
	return cloneProfile(c.agents[index]), true
}

func (c *AgentCatalog) OrderOf(id string) int {
	if index, ok := c.byID[id]; ok {
		return index
	}
	return len(c.agents)
}

func (c *AgentCatalog) NameOf(id string) string {
	if profile, ok := c.Get(id); ok && profile.Name != "" {
		return profile.Name
	}
	return id
}

func (c *AgentCatalog) Topic(id string) (TopicProfile, bool) {
	topic, ok := c.byTopic[strings.TrimSpace(id)]
	if !ok {
		return TopicProfile{}, false
	}
	return cloneTopic(topic), true
}

func (c *AgentCatalog) TopicsForAgent(agentID string) []TopicProfile {
	profile, ok := c.Get(agentID)
	if !ok {
		return nil
	}
	return profile.Topics
}

func (c *AgentCatalog) GlobalFallback(kind, locale string) string {
	localized := c.globalFallbacks[kind]
	if localized == nil {
		return ""
	}
	if value := strings.TrimSpace(localized[locale]); value != "" {
		return value
	}
	return strings.TrimSpace(localized["zh-CN"])
}

// ConfigSnapshot returns the behavior-affecting catalog state persisted with a
// run. Authorization scope is deliberately absent and is captured separately.
func (c *AgentCatalog) ConfigSnapshot() types.JSONMap {
	agents := make([]any, 0, len(c.agents))
	for _, agent := range c.Agents() {
		agents = append(agents, map[string]any{
			"id":                     agent.ID,
			"system_prompt_version":  agent.SystemPromptVersion,
			"search_hints":           slices.Clone(agent.SearchHints),
			"research_rules":         slices.Clone(agent.ResearchRules),
			"evidence_requirements":  slices.Clone(agent.EvidenceRequirements),
			"allowed_research_tools": slices.Clone(agent.AllowedResearchTools),
			"knowledge_domain_names": slices.Clone(agent.KnowledgeDomainNames),
			"route_keywords":         slices.Clone(agent.RouteKeywords),
			"topics":                 cloneTopics(agent.Topics),
		})
	}
	return types.JSONMap{
		"catalog_version":     c.version,
		"fallback_version":    c.fallbackVersion,
		"master_agent_id":     c.masterAgentID,
		"max_selected_agents": c.maxSelected,
		"fallback_agent_ids":  slices.Clone(c.fallbackIDs),
		"agents":              agents,
	}
}

func profileFromConfig(agent config.UnifiedQAAgentConfig) DomainAgentProfile {
	return DomainAgentProfile{
		ID:                   agent.ID,
		Name:                 agent.Name,
		Description:          agent.Description,
		SystemPromptVersion:  agent.SystemPromptVersion,
		SearchHints:          slices.Clone(agent.SearchHints),
		ResearchRules:        slices.Clone(agent.ResearchRules),
		EvidenceRequirements: slices.Clone(agent.EvidenceRequirements),
		AllowedResearchTools: slices.Clone(agent.AllowedResearchTools),
		KnowledgeDomainNames: slices.Clone(agent.KnowledgeDomainNames),
		RouteKeywords:        slices.Clone(agent.RouteKeywords),
	}
}

func cloneProfile(agent DomainAgentProfile) DomainAgentProfile {
	agent.SearchHints = slices.Clone(agent.SearchHints)
	agent.ResearchRules = slices.Clone(agent.ResearchRules)
	agent.EvidenceRequirements = slices.Clone(agent.EvidenceRequirements)
	agent.AllowedResearchTools = slices.Clone(agent.AllowedResearchTools)
	agent.KnowledgeDomainNames = slices.Clone(agent.KnowledgeDomainNames)
	agent.RouteKeywords = slices.Clone(agent.RouteKeywords)
	agent.Topics = cloneTopics(agent.Topics)
	return agent
}

func topicFromConfig(configured config.UnifiedQATopicConfig) TopicProfile {
	topic := TopicProfile{
		ID: configured.ID, AgentID: configured.AgentID,
		KnowledgeBaseNameContains: slices.Clone(configured.KnowledgeBaseNameContains),
		RouteKeywords:             slices.Clone(configured.RouteKeywords),
		NoMatchResponse:           cloneStringMap(configured.NoMatchResponse),
		AnswerDisclaimer:          cloneStringMap(configured.AnswerDisclaimer),
		Addenda:                   make([]TopicAddendumProfile, 0, len(configured.Addenda)),
	}
	for _, addendum := range configured.Addenda {
		topic.Addenda = append(topic.Addenda, TopicAddendumProfile{
			ID: addendum.ID, TriggerKeywords: slices.Clone(addendum.TriggerKeywords), Response: cloneStringMap(addendum.Response),
		})
	}
	return topic
}

func cloneTopics(topics []TopicProfile) []TopicProfile {
	cloned := make([]TopicProfile, len(topics))
	for i := range topics {
		cloned[i] = cloneTopic(topics[i])
	}
	return cloned
}

func cloneTopic(topic TopicProfile) TopicProfile {
	topic.KnowledgeBaseNameContains = slices.Clone(topic.KnowledgeBaseNameContains)
	topic.RouteKeywords = slices.Clone(topic.RouteKeywords)
	topic.NoMatchResponse = cloneStringMap(topic.NoMatchResponse)
	topic.AnswerDisclaimer = cloneStringMap(topic.AnswerDisclaimer)
	topic.Addenda = slices.Clone(topic.Addenda)
	for i := range topic.Addenda {
		topic.Addenda[i].TriggerKeywords = slices.Clone(topic.Addenda[i].TriggerKeywords)
		topic.Addenda[i].Response = cloneStringMap(topic.Addenda[i].Response)
	}
	return topic
}

func cloneStringMap[V ~string](source map[string]V) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = string(value)
	}
	return cloned
}
