package milvus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestUpdateChunkEnabledStatusInCollectionSkipsEmptyChunkIDs(t *testing.T) {
	repo := &milvusRepository{}

	require.NoError(t, repo.updateChunkEnabledStatusInCollection(
		context.Background(),
		"roche_kap_embeddings_1024",
		nil,
		false,
	))
	require.NoError(t, repo.updateChunkEnabledStatusInCollection(
		context.Background(),
		"roche_kap_embeddings_1024",
		[]string{},
		true,
	))
}

func TestBaseFilterCombinesKnowledgeBaseAndDocumentScope(t *testing.T) {
	repo := &milvusRepository{filter: filter{}}

	expr, params, err := repo.getBaseFilterForQuery(types.RetrieveParams{
		KnowledgeBaseIDs: []string{"kb-a"},
		KnowledgeIDs:     []string{"doc-1", "doc-2"},
	})

	require.NoError(t, err)
	require.Contains(t, expr, "knowledge_base_id in {knowledge_base_id_1}")
	require.Contains(t, expr, "knowledge_id in {knowledge_id_2}")
	require.Contains(t, expr, ") and (")
	require.Equal(t, []string{"kb-a"}, params["knowledge_base_id_1"])
	require.Equal(t, []string{"doc-1", "doc-2"}, params["knowledge_id_2"])
	require.Equal(t, true, params["is_enabled_3"])
}
