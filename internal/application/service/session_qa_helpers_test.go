package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestResolveKnowledgeBasesPreservesOrdinaryRAGSelection(t *testing.T) {
	svc := &sessionService{}
	req := &types.QARequest{
		KnowledgeBaseIDs: []string{"kb-a", "", "kb-a", "kb-b"},
		KnowledgeIDs:     []string{"doc-1", "doc-1", "", "doc-2"},
	}

	kbIDs, knowledgeIDs, err := svc.resolveKnowledgeBases(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []string{"kb-a", "kb-b"}, kbIDs)
	assert.Equal(t, []string{"doc-1", "doc-2"}, knowledgeIDs)
}

func TestResolveKnowledgeBasesAllowsOrdinaryPureChat(t *testing.T) {
	svc := &sessionService{}

	kbIDs, knowledgeIDs, err := svc.resolveKnowledgeBases(context.Background(), &types.QARequest{})

	require.NoError(t, err)
	assert.Empty(t, kbIDs)
	assert.Empty(t, knowledgeIDs)
}
