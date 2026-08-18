package tools

import (
	"strings"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestDatabaseQueryToolScopesSQLToExplicitGrants(t *testing.T) {
	tool := NewDatabaseQueryTool(nil, types.SearchTargets{
		{
			Type:              types.SearchTargetTypeKnowledgeBase,
			KnowledgeBaseID:   "kb-full",
			KnowledgeDomainID: 10,
		},
		{
			Type:              types.SearchTargetTypeKnowledge,
			KnowledgeBaseID:   "kb-docs",
			KnowledgeDomainID: 20,
			KnowledgeIDs:      []string{"doc-1", "doc-2"},
		},
	})

	secured, err := tool.validateAndSecureSQL(
		"SELECT id, content FROM chunks ORDER BY chunk_index LIMIT 10",
	)
	if err != nil {
		t.Fatalf("validateAndSecureSQL() error = %v", err)
	}

	for _, expected := range []string{"kb-full", "kb-docs", "doc-1", "doc-2"} {
		if !strings.Contains(secured, expected) {
			t.Fatalf("secured SQL does not contain %q: %s", expected, secured)
		}
	}
	if strings.Contains(secured, "knowledge_domain_id =") {
		t.Fatalf("secured SQL must not inject a single-domain filter: %s", secured)
	}
}

func TestDatabaseQueryToolRejectsEmptyGrantScope(t *testing.T) {
	tool := NewDatabaseQueryTool(nil, nil)

	_, err := tool.validateAndSecureSQL("SELECT id FROM knowledge_bases")
	if err == nil || !strings.Contains(err.Error(), "no authorized knowledge scope") {
		t.Fatalf("validateAndSecureSQL() error = %v, want no authorized knowledge scope", err)
	}
}
