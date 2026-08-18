package unifiedqa

import (
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestNewRunContextDeepCopiesMutableInput(t *testing.T) {
	input := RunContextInput{
		RunID:         "run-1",
		OriginalQuery: "Can this expense be reimbursed?",
		History:       []ConversationTurn{{Role: "user", Content: "Earlier question"}},
		AuthorizedScope: AuthorizedScope{
			KnowledgeBaseIDs: []string{"kb-1"},
			SearchTargets: types.SearchTargets{
				&types.SearchTarget{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-1", TagIDs: []string{"tag-1"}},
			},
		},
		ConfigSnapshot: types.JSONMap{"agents": []any{map[string]any{"id": "finance"}}},
	}
	runCtx, err := NewRunContext(input)
	if err != nil {
		t.Fatalf("NewRunContext() error = %v", err)
	}

	input.History[0].Content = "mutated"
	input.AuthorizedScope.KnowledgeBaseIDs[0] = "mutated"
	input.AuthorizedScope.SearchTargets[0].TagIDs[0] = "mutated"
	input.ConfigSnapshot["agents"].([]any)[0].(map[string]any)["id"] = "mutated"

	if runCtx.History[0].Content != "Earlier question" {
		t.Fatalf("History was mutated: %+v", runCtx.History)
	}
	if runCtx.AuthorizedScope.KnowledgeBaseIDs[0] != "kb-1" || runCtx.AuthorizedScope.SearchTargets[0].TagIDs[0] != "tag-1" {
		t.Fatalf("AuthorizedScope was mutated: %+v", runCtx.AuthorizedScope)
	}
	agents := runCtx.ConfigSnapshot["agents"].([]any)
	if agents[0].(map[string]any)["id"] != "finance" {
		t.Fatalf("ConfigSnapshot was mutated: %+v", runCtx.ConfigSnapshot)
	}
}
