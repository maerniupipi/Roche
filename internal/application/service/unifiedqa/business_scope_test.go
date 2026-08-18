package unifiedqa

import (
	"reflect"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestScopeForAgentIntersectsACLAndKnowledgeDepartment(t *testing.T) {
	cfg := testAgentCatalogConfig()
	cfg.Agents[0].KnowledgeDomainNames = []string{"财务", "finance"}
	cfg.Agents[1].KnowledgeDomainNames = []string{"合规", "compliance"}
	catalog, err := NewAgentCatalog(cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	scope := AuthorizedScope{
		KnowledgeBases: []AuthorizedKnowledgeBase{
			{ID: "kb-finance", KnowledgeDomainName: "财务部门的知识库", FullAccess: true},
			{ID: "kb-finance-en", KnowledgeDomainName: "APAC FINANCE Department", FullAccess: true},
			{ID: "kb-compliance", KnowledgeDomainName: "中国合规部门", FullAccess: true},
			{ID: "kb-compliance-en", KnowledgeDomainName: "Global COMPLIANCE Department", FullAccess: true},
			{ID: "kb-other", KnowledgeDomainName: "人力资源部门", FullAccess: true},
		},
		KnowledgeBaseIDs: []string{"kb-finance", "kb-finance-en", "kb-compliance", "kb-compliance-en", "kb-other"},
		SearchTargets: types.SearchTargets{
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-finance"},
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-finance-en"},
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-compliance"},
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-compliance-en"},
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-other"},
		},
	}

	finance, err := scopeForAgent(scope, catalog, FinanceAgentID)
	if err != nil {
		t.Fatalf("scopeForAgent(finance) error = %v", err)
	}
	if got, want := finance.KnowledgeBaseIDs, []string{"kb-finance", "kb-finance-en"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finance KBs = %v, want %v", got, want)
	}
	if len(finance.SearchTargets) != 2 || finance.SearchTargets[0].KnowledgeBaseID != "kb-finance" || finance.SearchTargets[1].KnowledgeBaseID != "kb-finance-en" {
		t.Fatalf("finance targets = %+v", finance.SearchTargets)
	}

	compliance, err := scopeForAgent(scope, catalog, ComplianceAgentID)
	if err != nil {
		t.Fatalf("scopeForAgent(compliance) error = %v", err)
	}
	if got, want := compliance.KnowledgeBaseIDs, []string{"kb-compliance", "kb-compliance-en"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compliance KBs = %v, want %v", got, want)
	}
}

func TestScopeForAgentDoesNotFallThroughWhenDomainIsUnavailable(t *testing.T) {
	cfg := testAgentCatalogConfig()
	cfg.Agents[0].KnowledgeDomainNames = []string{"财务"}
	catalog, err := NewAgentCatalog(cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	scope := AuthorizedScope{
		KnowledgeBases:   []AuthorizedKnowledgeBase{{ID: "kb-compliance", KnowledgeDomainName: "合规部门"}},
		KnowledgeBaseIDs: []string{"kb-compliance"},
	}
	if _, err := scopeForAgent(scope, catalog, FinanceAgentID); err == nil {
		t.Fatal("scopeForAgent(finance) error = nil, want no accessible finance KB")
	}
}

func TestScopeForTopicMatchesKnowledgeBaseNameCaseInsensitively(t *testing.T) {
	catalog := mustTestTopicCatalog(t)
	scope := AuthorizedScope{
		KnowledgeBases: []AuthorizedKnowledgeBase{
			{ID: "kb-doa", Name: "RDSL_DOA Policies", KnowledgeDomainName: "财务", FullAccess: true},
			{ID: "kb-te", Name: "China T&E", KnowledgeDomainName: "财务", FullAccess: true},
		},
		KnowledgeBaseIDs: []string{"kb-doa", "kb-te"},
		SearchTargets: types.SearchTargets{
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-doa"},
			{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-te"},
		},
	}
	doa, err := scopeForTopic(scope, catalog, FinanceAgentID, "doa")
	if err != nil {
		t.Fatalf("scopeForTopic() error = %v", err)
	}
	if got, want := doa.KnowledgeBaseIDs, []string{"kb-doa"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DoA KBs = %v, want %v", got, want)
	}
}

func TestScopeForComplianceTopicMatchesEnglishAndChineseKnowledgeBaseNames(t *testing.T) {
	catalog := mustTestTopicCatalog(t)
	for _, tc := range []struct {
		name string
		kbID string
		kb   string
	}{
		{name: "uppercase English", kbID: "kb-compliance-en", kb: "GLOBAL COMPLIANCE POLICIES"},
		{name: "Chinese", kbID: "kb-compliance-zh", kb: "中国合规知识库"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := AuthorizedScope{
				KnowledgeBases:   []AuthorizedKnowledgeBase{{ID: tc.kbID, Name: tc.kb, KnowledgeDomainName: "合规", FullAccess: true}},
				KnowledgeBaseIDs: []string{tc.kbID},
				SearchTargets: types.SearchTargets{
					{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: tc.kbID},
				},
			}
			got, err := scopeForTopic(scope, catalog, ComplianceAgentID, "compliance")
			if err != nil {
				t.Fatalf("scopeForTopic() error = %v", err)
			}
			if !reflect.DeepEqual(got.KnowledgeBaseIDs, []string{tc.kbID}) {
				t.Fatalf("Compliance KBs = %v, want [%s]", got.KnowledgeBaseIDs, tc.kbID)
			}
		})
	}
}
