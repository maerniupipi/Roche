package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	UnifiedQAMasterAgentID     = "unified-master"
	UnifiedQAFinanceAgentID    = "finance"
	UnifiedQAComplianceAgentID = "compliance"
)

var unifiedQAResearchToolWhitelist = map[string]struct{}{
	"knowledge_search":      {},
	"grep_chunks":           {},
	"get_document_info":     {},
	"list_knowledge_chunks": {},
	"query_knowledge_graph": {},
}

const (
	defaultUnifiedQAMaxSelectedAgents = 3
	maxUnifiedQACatalogAgents         = 16
	maxUnifiedQASelectedAgents        = 5
)

var unifiedQAAgentIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// UnifiedQAAgentsConfig is the merged runtime catalog used by unified knowledge
// Q&A. Authorization scope is resolved from the requesting user's permissions;
// department name keywords only narrow that already-authorized scope.
type UnifiedQAAgentsConfig struct {
	Version           string                 `json:"version"`
	MasterAgentID     string                 `json:"master_agent_id"`
	MaxSelectedAgents int                    `json:"max_selected_agents"`
	FallbackAgentIDs  []string               `json:"fallback_agent_ids"`
	Agents            []UnifiedQAAgentConfig `json:"agents"`
}

// UnifiedQAAgentConfig combines catalog identity with the referenced research
// profile, never authorization or knowledge-base identifiers.
type UnifiedQAAgentConfig struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Enabled              bool     `json:"enabled"`
	SystemPromptVersion  string   `json:"system_prompt_version"`
	SearchHints          []string `json:"search_hints"`
	ResearchRules        []string `json:"research_rules"`
	EvidenceRequirements []string `json:"evidence_requirements"`
	AllowedResearchTools []string `json:"allowed_research_tools"`
	KnowledgeDomainNames []string `json:"knowledge_domain_names"`
	RouteKeywords        []string `json:"route_keywords"`
}

type unifiedQAAgentsCatalogFile struct {
	Version           string                       `yaml:"version"`
	MasterAgentID     string                       `yaml:"master_agent_id"`
	MaxSelectedAgents int                          `yaml:"max_selected_agents"`
	FallbackAgentIDs  []string                     `yaml:"fallback_agent_ids"`
	Agents            []unifiedQAAgentCatalogEntry `yaml:"agents"`
}

type unifiedQAAgentCatalogEntry struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
	Profile string `yaml:"profile"`
}

type unifiedQAAgentProfileFile struct {
	Description          string   `yaml:"description"`
	SystemPromptVersion  string   `yaml:"system_prompt_version"`
	SearchHints          []string `yaml:"search_hints"`
	ResearchRules        []string `yaml:"research_rules"`
	EvidenceRequirements []string `yaml:"evidence_requirements"`
	AllowedResearchTools []string `yaml:"allowed_research_tools"`
	KnowledgeDomainNames []string `yaml:"knowledge_domain_names"`
	RouteKeywords        []string `yaml:"route_keywords"`
}

// LoadUnifiedQAAgentsFile loads and strictly validates a Unified QA
// domain-agent catalog.
func LoadUnifiedQAAgentsFile(path string) (*UnifiedQAAgentsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read unified QA agents config: %w", err)
	}
	var catalogFile unifiedQAAgentsCatalogFile
	if err := decodeStrictUnifiedQAYAML(data, &catalogFile); err != nil {
		return nil, fmt.Errorf("decode unified QA agents catalog: %w", err)
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("resolve unified QA agents catalog directory: %w", err)
	}
	cfg := UnifiedQAAgentsConfig{
		Version:           catalogFile.Version,
		MasterAgentID:     catalogFile.MasterAgentID,
		MaxSelectedAgents: catalogFile.MaxSelectedAgents,
		FallbackAgentIDs:  append([]string(nil), catalogFile.FallbackAgentIDs...),
		Agents:            make([]UnifiedQAAgentConfig, 0, len(catalogFile.Agents)),
	}
	usedProfiles := make(map[string]string, len(catalogFile.Agents))
	for i, entry := range catalogFile.Agents {
		profilePath, err := resolveUnifiedQAAgentProfilePath(baseDir, entry.Profile)
		if err != nil {
			return nil, fmt.Errorf("resolve profile for agent %q at agents[%d]: %w", entry.ID, i, err)
		}
		if previousID, duplicate := usedProfiles[profilePath]; duplicate {
			return nil, fmt.Errorf("agents %q and %q reference the same profile", previousID, entry.ID)
		}
		usedProfiles[profilePath] = entry.ID

		profileData, err := os.ReadFile(profilePath)
		if err != nil {
			return nil, fmt.Errorf("read profile for agent %q: %w", entry.ID, err)
		}
		var profile unifiedQAAgentProfileFile
		if err := decodeStrictUnifiedQAYAML(profileData, &profile); err != nil {
			return nil, fmt.Errorf("decode profile for agent %q: %w", entry.ID, err)
		}
		cfg.Agents = append(cfg.Agents, UnifiedQAAgentConfig{
			ID:                   entry.ID,
			Name:                 entry.Name,
			Enabled:              entry.Enabled,
			Description:          profile.Description,
			SystemPromptVersion:  profile.SystemPromptVersion,
			SearchHints:          append([]string(nil), profile.SearchHints...),
			ResearchRules:        append([]string(nil), profile.ResearchRules...),
			EvidenceRequirements: append([]string(nil), profile.EvidenceRequirements...),
			AllowedResearchTools: append([]string(nil), profile.AllowedResearchTools...),
			KnowledgeDomainNames: append([]string(nil), profile.KnowledgeDomainNames...),
			RouteKeywords:        append([]string(nil), profile.RouteKeywords...),
		})
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate unified QA agents config: %w", err)
	}
	return &cfg, nil
}

func decodeStrictUnifiedQAYAML(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple YAML documents are not allowed")
		}
		return err
	}
	return nil
}

func resolveUnifiedQAAgentProfilePath(baseDir, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", fmt.Errorf("profile is required")
	}
	if filepath.IsAbs(reference) {
		return "", fmt.Errorf("profile path must be relative to the catalog directory")
	}
	cleaned := filepath.Clean(reference)
	if extension := strings.ToLower(filepath.Ext(cleaned)); extension != ".yaml" && extension != ".yml" {
		return "", fmt.Errorf("profile path must use a .yaml or .yml extension")
	}
	target := filepath.Join(baseDir, cleaned)
	if err := ensurePathWithinDirectory(baseDir, target); err != nil {
		return "", err
	}

	resolvedBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve catalog directory: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve profile file: %w", err)
	}
	if err := ensurePathWithinDirectory(resolvedBase, resolvedTarget); err != nil {
		return "", fmt.Errorf("resolved profile path: %w", err)
	}
	return resolvedTarget, nil
}

func ensurePathWithinDirectory(baseDir, target string) error {
	relative, err := filepath.Rel(baseDir, target)
	if err != nil {
		return fmt.Errorf("compare profile path with catalog directory: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("profile path must stay within the catalog directory")
	}
	return nil
}

// Validate enforces a bounded, configuration-driven agent topology and the
// research-tool boundary. It also applies safe defaults for older catalogs.
func (c *UnifiedQAAgentsConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if c.MasterAgentID != UnifiedQAMasterAgentID {
		return fmt.Errorf("master_agent_id must be %q", UnifiedQAMasterAgentID)
	}
	if len(c.Agents) == 0 || len(c.Agents) > maxUnifiedQACatalogAgents {
		return fmt.Errorf("agents must contain between 1 and %d entries", maxUnifiedQACatalogAgents)
	}

	seen := make(map[string]struct{}, len(c.Agents))
	enabledIDs := make([]string, 0, len(c.Agents))
	enabledSet := make(map[string]struct{}, len(c.Agents))
	for i := range c.Agents {
		agent := &c.Agents[i]
		if !unifiedQAAgentIDPattern.MatchString(agent.ID) {
			return fmt.Errorf("agents[%d].id %q must match %s", i, agent.ID, unifiedQAAgentIDPattern.String())
		}
		if _, exists := seen[agent.ID]; exists {
			return fmt.Errorf("duplicate agent id %q", agent.ID)
		}
		seen[agent.ID] = struct{}{}
		if agent.Enabled {
			enabledIDs = append(enabledIDs, agent.ID)
			enabledSet[agent.ID] = struct{}{}
		}
		if strings.TrimSpace(agent.Name) == "" || strings.TrimSpace(agent.Description) == "" {
			return fmt.Errorf("agent %q name and description are required", agent.ID)
		}
		if strings.TrimSpace(agent.SystemPromptVersion) == "" {
			return fmt.Errorf("agent %q system_prompt_version is required", agent.ID)
		}
		if err := validateNonEmptyUniqueStrings(agent.ID, "research_rules", agent.ResearchRules, true); err != nil {
			return err
		}
		if err := validateNonEmptyUniqueStrings(agent.ID, "evidence_requirements", agent.EvidenceRequirements, true); err != nil {
			return err
		}
		if err := validateNonEmptyUniqueStrings(agent.ID, "search_hints", agent.SearchHints, false); err != nil {
			return err
		}
		if err := validateNonEmptyUniqueStrings(agent.ID, "allowed_research_tools", agent.AllowedResearchTools, true); err != nil {
			return err
		}
		if err := validateNonEmptyUniqueStrings(agent.ID, "knowledge_domain_names", agent.KnowledgeDomainNames, true); err != nil {
			return err
		}
		if err := validateNonEmptyUniqueStrings(agent.ID, "route_keywords", agent.RouteKeywords, false); err != nil {
			return err
		}
		for _, tool := range agent.AllowedResearchTools {
			if _, ok := unifiedQAResearchToolWhitelist[tool]; !ok {
				return fmt.Errorf("agent %q research tool %q is not allowed", agent.ID, tool)
			}
		}
	}
	if len(enabledIDs) == 0 {
		return fmt.Errorf("at least one agent must be enabled")
	}
	if c.MaxSelectedAgents == 0 {
		c.MaxSelectedAgents = min(defaultUnifiedQAMaxSelectedAgents, len(enabledIDs))
	}
	if c.MaxSelectedAgents < 1 || c.MaxSelectedAgents > len(enabledIDs) || c.MaxSelectedAgents > maxUnifiedQASelectedAgents {
		return fmt.Errorf("max_selected_agents must be between 1 and %d and not exceed the enabled agent count", maxUnifiedQASelectedAgents)
	}
	if len(c.FallbackAgentIDs) == 0 {
		c.FallbackAgentIDs = append([]string(nil), enabledIDs[:min(c.MaxSelectedAgents, len(enabledIDs))]...)
	}
	if len(c.FallbackAgentIDs) > c.MaxSelectedAgents {
		return fmt.Errorf("fallback_agent_ids must not exceed max_selected_agents")
	}
	seenFallback := make(map[string]struct{}, len(c.FallbackAgentIDs))
	for _, id := range c.FallbackAgentIDs {
		if _, ok := enabledSet[id]; !ok {
			return fmt.Errorf("fallback agent %q is not enabled or does not exist", id)
		}
		if _, duplicate := seenFallback[id]; duplicate {
			return fmt.Errorf("fallback_agent_ids contains duplicate %q", id)
		}
		seenFallback[id] = struct{}{}
	}
	return nil
}

func validateNonEmptyUniqueStrings(agentID, field string, values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("agent %q %s must not be empty", agentID, field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("agent %q %s contains an empty value", agentID, field)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("agent %q %s contains duplicate value %q", agentID, field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
