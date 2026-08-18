package unifiedqa

import (
	"reflect"
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
)

func TestAgentCatalogExposesOrderedDomainAgents(t *testing.T) {
	cfg := testAgentCatalogConfig()
	catalog, err := NewAgentCatalog(cfg, func(id string) bool {
		return id == "finance-v1" || id == "compliance-v1"
	})
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}

	agents := catalog.Agents()
	if got, want := []string{agents[0].ID, agents[1].ID}, []string{FinanceAgentID, ComplianceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("agent IDs = %v, want %v", got, want)
	}
	if _, ok := catalog.Get("generic"); ok {
		t.Fatal("Get(generic) returned an unconfigured agent")
	}
	finance, ok := catalog.Get(FinanceAgentID)
	if !ok || finance.SystemPromptVersion != "finance-v1" {
		t.Fatalf("Get(finance) = (%+v, %v)", finance, ok)
	}
}

func TestAgentCatalogSupportsConfiguredThirdAgentAndSkipsDisabledAgents(t *testing.T) {
	cfg := testThreeAgentCatalogConfig()
	cfg.Agents = append(cfg.Agents, config.UnifiedQAAgentConfig{
		ID:                   "disabled-agent",
		Name:                 "Disabled",
		Description:          "Disabled domain",
		Enabled:              false,
		SystemPromptVersion:  "disabled-v1",
		ResearchRules:        []string{"Do not run"},
		EvidenceRequirements: []string{"Cite evidence"},
		AllowedResearchTools: []string{"knowledge_search"},
		KnowledgeDomainNames: []string{"人力资源"},
	})
	catalog, err := NewAgentCatalog(cfg, func(id string) bool { return id != "disabled-v1" })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}

	agents := catalog.Agents()
	if got := []string{agents[0].ID, agents[1].ID, agents[2].ID}; !reflect.DeepEqual(got, []string{"finance", "compliance", "hr"}) {
		t.Fatalf("agent IDs = %v", got)
	}
	if _, ok := catalog.Get("hr"); !ok {
		t.Fatal("Get(hr) did not return the configured third agent")
	}
	if _, ok := catalog.Get("disabled-agent"); ok {
		t.Fatal("Get(disabled-agent) returned a disabled agent")
	}
}

func TestAgentCatalogReturnsDefensiveCopies(t *testing.T) {
	catalog, err := NewAgentCatalog(testAgentCatalogConfig(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}

	agents := catalog.Agents()
	agents[0].ResearchRules[0] = "mutated"
	agents[0].AllowedResearchTools[0] = "mutated"

	finance, _ := catalog.Get(FinanceAgentID)
	if finance.ResearchRules[0] == "mutated" || finance.AllowedResearchTools[0] == "mutated" {
		t.Fatal("catalog state was mutated through Agents() result")
	}
}

func TestAgentCatalogRejectsUnresolvablePromptVersion(t *testing.T) {
	_, err := NewAgentCatalog(testAgentCatalogConfig(), func(id string) bool {
		return id != "compliance-v1"
	})
	if err == nil {
		t.Fatal("NewAgentCatalog() error = nil, want unresolved prompt error")
	}
}

func TestAgentCatalogSnapshotContainsBehaviorButNoKnowledgeBaseScope(t *testing.T) {
	catalog, err := NewAgentCatalog(testAgentCatalogConfig(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}

	snapshot := catalog.ConfigSnapshot()
	if snapshot["catalog_version"] != "catalog-v1" || snapshot["master_agent_id"] != "unified-master" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	agents := snapshot["agents"].([]any)
	if len(agents) != 2 {
		t.Fatalf("snapshot agents = %+v", agents)
	}
	if snapshot["max_selected_agents"] != 2 || !reflect.DeepEqual(snapshot["fallback_agent_ids"], []string{"finance", "compliance"}) {
		t.Fatalf("snapshot routing policy = %+v", snapshot)
	}
	for _, item := range agents {
		agent := item.(map[string]any)
		if _, exists := agent["knowledge_base_ids"]; exists {
			t.Fatalf("snapshot agent contains knowledge-base scope: %+v", agent)
		}
	}
}

func testThreeAgentCatalogConfig() *config.UnifiedQAAgentsConfig {
	cfg := testAgentCatalogConfig()
	cfg.MaxSelectedAgents = 3
	cfg.FallbackAgentIDs = []string{"finance", "compliance", "hr"}
	cfg.Agents = append(cfg.Agents, config.UnifiedQAAgentConfig{
		ID:                   "hr",
		Name:                 "人力资源",
		Description:          "人力资源政策研究",
		Enabled:              true,
		SystemPromptVersion:  "hr-v1",
		SearchHints:          []string{"休假", "福利"},
		ResearchRules:        []string{"核对适用员工范围"},
		EvidenceRequirements: []string{"引用政策条款"},
		AllowedResearchTools: []string{"knowledge_search"},
		KnowledgeDomainNames: []string{"人力资源"},
	})
	return cfg
}

func testAgentCatalogConfig() *config.UnifiedQAAgentsConfig {
	return &config.UnifiedQAAgentsConfig{
		Version:       "catalog-v1",
		MasterAgentID: config.UnifiedQAMasterAgentID,
		Agents: []config.UnifiedQAAgentConfig{
			{
				ID:                   config.UnifiedQAFinanceAgentID,
				Name:                 "财务子智能体",
				Description:          "Financial policy research",
				Enabled:              true,
				SystemPromptVersion:  "finance-v1",
				ResearchRules:        []string{"Verify limits"},
				EvidenceRequirements: []string{"Cite amounts"},
				AllowedResearchTools: []string{"knowledge_search"},
				KnowledgeDomainNames: []string{"财务"},
			},
			{
				ID:                   config.UnifiedQAComplianceAgentID,
				Name:                 "合规子智能体",
				Description:          "Compliance policy research",
				Enabled:              true,
				SystemPromptVersion:  "compliance-v1",
				ResearchRules:        []string{"Check prohibited conduct"},
				EvidenceRequirements: []string{"Cite clauses"},
				AllowedResearchTools: []string{"knowledge_search"},
				KnowledgeDomainNames: []string{"合规"},
			},
		},
	}
}
