package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type scopedKnowledgeServiceStub struct {
	interfaces.KnowledgeService
	knowledge *types.Knowledge
}

func (s *scopedKnowledgeServiceStub) GetKnowledgeByIDOnly(context.Context, string) (*types.Knowledge, error) {
	return s.knowledge, nil
}

func TestDataSchemaRejectsKnowledgeOutsideSessionScope(t *testing.T) {
	knowledgeService := &scopedKnowledgeServiceStub{
		knowledge: &types.Knowledge{
			ID:              "doc-2",
			KnowledgeBaseID: "kb-1",
		},
	}
	tool := NewScopedDataSchemaTool(
		knowledgeService,
		nil,
		types.SearchTargets{
			{
				Type:            types.SearchTargetTypeKnowledge,
				KnowledgeBaseID: "kb-1",
				KnowledgeIDs:    []string{"doc-1"},
			},
		},
	)
	args, err := json.Marshal(DataSchemaInput{KnowledgeID: "doc-2"})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not accessible")
}

func TestDataAnalysisRejectsKnowledgeOutsideSessionScope(t *testing.T) {
	tool := NewDataAnalysisTool(
		nil,
		nil,
		nil,
		nil,
		nil,
		"session-1",
		types.SearchTargets{
			{
				Type:            types.SearchTargetTypeKnowledge,
				KnowledgeBaseID: "kb-1",
				KnowledgeIDs:    []string{"doc-1"},
			},
		},
	)

	_, err := tool.LoadFromKnowledge(context.Background(), &types.Knowledge{
		ID:              "doc-2",
		KnowledgeBaseID: "kb-1",
		FileType:        "xlsx",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not accessible")
}
