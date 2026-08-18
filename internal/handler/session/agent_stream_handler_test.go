package session

import (
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestAppendKnowledgeReferencesAcceptsNamedReferences(t *testing.T) {
	first := &types.SearchResult{ID: "existing"}
	second := &types.SearchResult{ID: "named-reference"}

	got := appendKnowledgeReferences(
		[]*types.SearchResult{first},
		types.References{second},
	)

	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("references = %#v", got)
	}
}

func TestAppendKnowledgeReferencesAcceptsLegacyRepresentations(t *testing.T) {
	direct := &types.SearchResult{ID: "direct"}
	fromMap := map[string]interface{}{
		"id":                "mapped",
		"knowledge_id":      "knowledge",
		"knowledge_base_id": "kb",
		"chunk_index":       float64(3),
		"metadata":          map[string]interface{}{"source": "test"},
	}

	got := appendKnowledgeReferences(nil, []interface{}{direct, fromMap})

	if len(got) != 2 || got[0] != direct || got[1].ID != "mapped" ||
		got[1].KnowledgeID != "knowledge" || got[1].ChunkIndex != 3 ||
		got[1].Metadata["source"] != "test" {
		t.Fatalf("references = %#v", got)
	}
}
