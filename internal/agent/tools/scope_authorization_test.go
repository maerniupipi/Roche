package tools

import (
	"context"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func testKnowledgeTag(id string) *types.KnowledgeTag {
	return &types.KnowledgeTag{ID: id}
}

func TestKnowledgeIDsMatchingAnyTag(t *testing.T) {
	matches, err := knowledgeIDsMatchingAnyTag(
		context.Background(),
		[]string{"doc-1", "doc-2"},
		[]string{"tag-a"},
		func(_ context.Context, ids []string) (map[string][]*types.KnowledgeTag, error) {
			return map[string][]*types.KnowledgeTag{
				"doc-1": []*types.KnowledgeTag{testKnowledgeTag("tag-z")},
				"doc-2": []*types.KnowledgeTag{testKnowledgeTag("tag-a")},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("knowledgeIDsMatchingAnyTag() error = %v", err)
	}
	if matches["doc-1"] {
		t.Fatalf("doc-1 should not match tag-a")
	}
	if !matches["doc-2"] {
		t.Fatalf("doc-2 should match tag-a")
	}
}

func TestSearchTargetsAllowFullKnowledgeBase(t *testing.T) {
	tests := []struct {
		name    string
		targets types.SearchTargets
		want    bool
	}{
		{
			name: "whole knowledge base",
			targets: types.SearchTargets{
				{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-1"},
			},
			want: true,
		},
		{
			name: "document grant is not whole knowledge base access",
			targets: types.SearchTargets{
				{
					Type:            types.SearchTargetTypeKnowledge,
					KnowledgeBaseID: "kb-1",
					KnowledgeIDs:    []string{"doc-1"},
				},
			},
			want: false,
		},
		{
			name: "tag scope is not whole knowledge base access",
			targets: types.SearchTargets{
				{
					Type:            types.SearchTargetTypeKnowledgeBase,
					KnowledgeBaseID: "kb-1",
					TagIDs:          []string{"tag-1"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchTargetsAllowFullKnowledgeBase(tt.targets, "kb-1"); got != tt.want {
				t.Fatalf("searchTargetsAllowFullKnowledgeBase() = %v, want %v", got, tt.want)
			}
		})
	}
}
