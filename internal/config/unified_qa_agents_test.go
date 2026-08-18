package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const validUnifiedQAAgentsCatalogYAML = `
version: catalog-v1
master_agent_id: unified-master
agents:
  - id: finance
    name: Finance
    enabled: true
    profile: agents/finance.yaml
  - id: compliance
    name: Compliance
    enabled: true
    profile: agents/compliance.yaml
`

const validFinanceProfileYAML = `
description: Financial policy research
system_prompt_version: finance-v1
search_hints: [expense, reimbursement]
research_rules: [Verify monetary limits]
evidence_requirements: [Cite the amount and effective date]
allowed_research_tools: [knowledge_search, grep_chunks]
knowledge_domain_names: [财务]
`

const validComplianceProfileYAML = `
description: Compliance policy research
system_prompt_version: compliance-v1
search_hints: [HCP, HCO]
research_rules: [Check prohibited conduct]
evidence_requirements: [Cite the applicable policy clause]
allowed_research_tools: [knowledge_search, get_document_info]
knowledge_domain_names: [合规]
`

func TestUnifiedQAAgentsLoadsSplitCatalog(t *testing.T) {
	path := writeUnifiedQATestCatalog(t, validUnifiedQAAgentsCatalogYAML, map[string]string{
		"agents/finance.yaml":    validFinanceProfileYAML,
		"agents/compliance.yaml": validComplianceProfileYAML,
	})
	cfg, err := LoadUnifiedQAAgentsFile(path)
	if err != nil {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v", err)
	}
	if got, want := cfg.MasterAgentID, UnifiedQAMasterAgentID; got != want {
		t.Fatalf("MasterAgentID = %q, want %q", got, want)
	}
	if got, want := len(cfg.Agents), 2; got != want {
		t.Fatalf("len(Agents) = %d, want %d", got, want)
	}
	if cfg.Agents[0].ID != UnifiedQAFinanceAgentID || cfg.Agents[0].SystemPromptVersion != "finance-v1" || cfg.Agents[1].ID != UnifiedQAComplianceAgentID {
		t.Fatalf("merged agents = %+v", cfg.Agents)
	}
	if cfg.MaxSelectedAgents != 2 || !reflect.DeepEqual(cfg.FallbackAgentIDs, []string{"finance", "compliance"}) {
		t.Fatalf("routing defaults = max %d, fallback %v", cfg.MaxSelectedAgents, cfg.FallbackAgentIDs)
	}
}

func TestUnifiedQAAgentsAllowsSingleEnabledAgent(t *testing.T) {
	catalog := `
version: catalog-v1
master_agent_id: unified-master
agents:
  - id: finance
    name: Finance
    enabled: true
    profile: agents/finance.yaml
`
	path := writeUnifiedQATestCatalog(t, catalog, map[string]string{"agents/finance.yaml": validFinanceProfileYAML})
	cfg, err := LoadUnifiedQAAgentsFile(path)
	if err != nil {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v", err)
	}
	if cfg.MaxSelectedAgents != 1 || !reflect.DeepEqual(cfg.FallbackAgentIDs, []string{"finance"}) {
		t.Fatalf("single-agent defaults = max %d, fallback %v", cfg.MaxSelectedAgents, cfg.FallbackAgentIDs)
	}
}

func TestUnifiedQAAgentsAcceptsThirdAgent(t *testing.T) {
	catalog := validUnifiedQAAgentsCatalogYAML + `  - id: hr
    name: Human Resources
    enabled: true
    profile: agents/hr.yaml
`
	path := writeUnifiedQATestCatalog(t, catalog, map[string]string{
		"agents/finance.yaml":    validFinanceProfileYAML,
		"agents/compliance.yaml": validComplianceProfileYAML,
		"agents/hr.yaml": strings.NewReplacer(
			"Financial policy research", "Human resources policy research",
			"finance-v1", "hr-v1",
		).Replace(validFinanceProfileYAML),
	})
	cfg, err := LoadUnifiedQAAgentsFile(path)
	if err != nil {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v", err)
	}
	if got := len(cfg.Agents); got != 3 || cfg.Agents[2].ID != "hr" || cfg.MaxSelectedAgents != 3 {
		t.Fatalf("third-agent catalog = %+v", cfg)
	}
}

func TestUnifiedQAAgentsRejectsInvalidFallback(t *testing.T) {
	catalog := strings.Replace(validUnifiedQAAgentsCatalogYAML, "master_agent_id: unified-master", "master_agent_id: unified-master\nmax_selected_agents: 1\nfallback_agent_ids: [finance, compliance]", 1)
	path := writeUnifiedQATestCatalog(t, catalog, map[string]string{
		"agents/finance.yaml":    validFinanceProfileYAML,
		"agents/compliance.yaml": validComplianceProfileYAML,
	})
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil {
		t.Fatal("LoadUnifiedQAAgentsFile() error = nil, want fallback limit error")
	}
}

func TestUnifiedQAAgentsRejectsBehaviorFieldsInCatalog(t *testing.T) {
	catalog := strings.Replace(validUnifiedQAAgentsCatalogYAML, "    name: Finance", "    name: Finance\n    description: must live in profile", 1)
	path := writeUnifiedQATestCatalog(t, catalog, nil)
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil || !strings.Contains(err.Error(), "field description not found") {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v, want catalog field rejection", err)
	}
}

func TestUnifiedQAAgentsRejectsIdentityFieldsInProfile(t *testing.T) {
	profile := "id: finance\n" + validFinanceProfileYAML
	path := writeUnifiedQATestCatalog(t, validUnifiedQAAgentsCatalogYAML, map[string]string{
		"agents/finance.yaml":    profile,
		"agents/compliance.yaml": validComplianceProfileYAML,
	})
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil || !strings.Contains(err.Error(), "field id not found") {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v, want profile identity rejection", err)
	}
}

func TestUnifiedQAAgentsRejectsKnowledgeBaseFieldInProfile(t *testing.T) {
	profile := strings.Replace(validFinanceProfileYAML, "search_hints:", "knowledge_base_ids: [kb-finance]\nsearch_hints:", 1)
	path := writeUnifiedQATestCatalog(t, validUnifiedQAAgentsCatalogYAML, map[string]string{
		"agents/finance.yaml":    profile,
		"agents/compliance.yaml": validComplianceProfileYAML,
	})
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil {
		t.Fatal("LoadUnifiedQAAgentsFile() error = nil, want knowledge_base_ids rejection")
	}
}

func TestUnifiedQAAgentsRejectsUnknownResearchTool(t *testing.T) {
	profile := strings.Replace(validFinanceProfileYAML, "allowed_research_tools: [knowledge_search, grep_chunks]", "allowed_research_tools: [knowledge_search, web_search]", 1)
	path := writeUnifiedQATestCatalog(t, validUnifiedQAAgentsCatalogYAML, map[string]string{
		"agents/finance.yaml":    profile,
		"agents/compliance.yaml": validComplianceProfileYAML,
	})
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil {
		t.Fatal("LoadUnifiedQAAgentsFile() error = nil, want unknown tool error")
	}
}

func TestUnifiedQAAgentsRejectsProfileOutsideCatalogDirectory(t *testing.T) {
	catalog := strings.Replace(validUnifiedQAAgentsCatalogYAML, "agents/finance.yaml", "../finance.yaml", 1)
	path := writeUnifiedQATestCatalog(t, catalog, nil)
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil || !strings.Contains(err.Error(), "must stay within") {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v, want path boundary error", err)
	}
}

func TestUnifiedQAAgentsRejectsSharedProfileFile(t *testing.T) {
	catalog := strings.Replace(validUnifiedQAAgentsCatalogYAML, "agents/compliance.yaml", "agents/finance.yaml", 1)
	path := writeUnifiedQATestCatalog(t, catalog, map[string]string{"agents/finance.yaml": validFinanceProfileYAML})
	if _, err := LoadUnifiedQAAgentsFile(path); err == nil || !strings.Contains(err.Error(), "reference the same profile") {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v, want shared profile rejection", err)
	}
}

func TestUnifiedQAAgentsRepositoryConfigResolvesPrompts(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	configDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "config")
	agents, err := LoadUnifiedQAAgentsFile(filepath.Join(configDir, "unified_qa_agents.yaml"))
	if err != nil {
		t.Fatalf("LoadUnifiedQAAgentsFile() error = %v", err)
	}
	if got, want := len(agents.Agents), 2; got != want {
		t.Fatalf("len(Agents) = %d, want %d", got, want)
	}
	prompts, err := loadPromptTemplates(configDir)
	if err != nil {
		t.Fatalf("load unified QA prompts: %v", err)
	}
	if got, want := len(prompts.UnifiedQA), 7; got != want {
		t.Fatalf("len(UnifiedQA) = %d, want %d", got, want)
	}
	promptIDs := make(map[string]struct{}, len(prompts.UnifiedQA))
	for _, prompt := range prompts.UnifiedQA {
		if prompt.ID == "" || strings.TrimSpace(prompt.Content) == "" {
			t.Fatalf("invalid prompt template: %+v", prompt)
		}
		if _, duplicate := promptIDs[prompt.ID]; duplicate {
			t.Fatalf("duplicate prompt ID %q", prompt.ID)
		}
		promptIDs[prompt.ID] = struct{}{}
	}
	for _, agent := range agents.Agents {
		if _, exists := promptIDs[agent.SystemPromptVersion]; !exists {
			t.Fatalf("agent %q prompt version %q cannot be resolved", agent.ID, agent.SystemPromptVersion)
		}
	}
}

func TestLoadPromptTemplatesRejectsDuplicateUnifiedQAPromptIDs(t *testing.T) {
	configDir := t.TempDir()
	templatesDir := filepath.Join(configDir, "prompt_templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("create prompt templates directory: %v", err)
	}
	for _, filename := range []string{"unified_qa_finance.yaml", "unified_qa_master.yaml"} {
		content := []byte("templates:\n  - id: duplicate-v1\n    content: test\n")
		if err := os.WriteFile(filepath.Join(templatesDir, filename), content, 0o600); err != nil {
			t.Fatalf("write %s: %v", filename, err)
		}
	}
	if _, err := loadPromptTemplates(configDir); err == nil || !strings.Contains(err.Error(), "duplicate unified QA prompt ID") {
		t.Fatalf("loadPromptTemplates() error = %v, want duplicate prompt ID error", err)
	}
}

func writeUnifiedQATestCatalog(t *testing.T, catalog string, profiles map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range profiles {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create profile directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write profile %s: %v", name, err)
		}
	}
	catalogPath := filepath.Join(root, "unified_qa_agents.yaml")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	return catalogPath
}
