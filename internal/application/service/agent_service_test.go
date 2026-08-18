package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type fakeAgentKnowledgeBaseService struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *fakeAgentKnowledgeBaseService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	if s.kb == nil {
		return nil, errors.New("knowledge base not found")
	}
	return s.kb, nil
}

type fakeAgentKnowledgeService struct {
	interfaces.KnowledgeService
	knowledges []*types.Knowledge
	lastFilter types.KnowledgeListFilter
}

func (s *fakeAgentKnowledgeService) ListPagedKnowledgeByKnowledgeBaseID(
	_ context.Context,
	_ string,
	page *types.Pagination,
	filter types.KnowledgeListFilter,
) (*types.PageResult, error) {
	s.lastFilter = filter

	filtered := make([]*types.Knowledge, 0, len(s.knowledges))
	for _, knowledge := range s.knowledges {
		if filter.ParseStatus != "" && knowledge.ParseStatus != filter.ParseStatus {
			continue
		}
		if len(filter.AllowedKnowledgeIDs) > 0 {
			allowed := false
			for _, id := range filter.AllowedKnowledgeIDs {
				if knowledge.ID == id {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		filtered = append(filtered, knowledge)
	}
	return types.NewPageResult(int64(len(filtered)), page, filtered), nil
}

func TestGetKnowledgeBaseInfos_ExcludesUnprocessedDocuments(t *testing.T) {
	now := time.Now()
	knowledgeService := &fakeAgentKnowledgeService{
		knowledges: []*types.Knowledge{
			{
				ID:              "doc-processing",
				KnowledgeBaseID: "kb-1",
				Title:           "still parsing",
				FileName:        "processing.pdf",
				FileType:        "pdf",
				ParseStatus:     types.ParseStatusProcessing,
				CreatedAt:       now,
			},
			{
				ID:              "doc-completed",
				KnowledgeBaseID: "kb-1",
				Title:           "ready document",
				FileName:        "ready.pdf",
				FileType:        "pdf",
				ParseStatus:     types.ParseStatusCompleted,
				CreatedAt:       now.Add(-time.Minute),
			},
		},
	}
	service := &agentService{
		knowledgeBaseService: &fakeAgentKnowledgeBaseService{
			kb: &types.KnowledgeBase{
				ID:                "kb-1",
				Name:              "KB",
				Type:              types.KnowledgeBaseTypeDocument,
				KnowledgeDomainID: 1,
			},
		},
		knowledgeService: knowledgeService,
	}

	infos, err := service.getKnowledgeBaseInfos(context.Background(), []string{"kb-1"}, nil)

	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, types.ParseStatusCompleted, knowledgeService.lastFilter.ParseStatus)
	assert.Equal(t, 1, infos[0].DocCount)
	require.Len(t, infos[0].RecentDocs, 1)
	assert.Equal(t, "doc-completed", infos[0].RecentDocs[0].KnowledgeID)
}

func TestGetKnowledgeBaseInfos_FiltersDocumentOnlyScope(t *testing.T) {
	now := time.Now()
	knowledgeService := &fakeAgentKnowledgeService{
		knowledges: []*types.Knowledge{
			{
				ID:              "doc-allowed",
				KnowledgeBaseID: "kb-1",
				Title:           "allowed",
				ParseStatus:     types.ParseStatusCompleted,
				CreatedAt:       now,
			},
			{
				ID:              "doc-private",
				KnowledgeBaseID: "kb-1",
				Title:           "private",
				ParseStatus:     types.ParseStatusCompleted,
				CreatedAt:       now,
			},
		},
	}
	service := &agentService{
		knowledgeBaseService: &fakeAgentKnowledgeBaseService{
			kb: &types.KnowledgeBase{
				ID:                "kb-1",
				Name:              "KB",
				Type:              types.KnowledgeBaseTypeDocument,
				KnowledgeDomainID: 1,
			},
		},
		knowledgeService: knowledgeService,
	}

	infos, err := service.getKnowledgeBaseInfos(
		context.Background(),
		[]string{"kb-1"},
		types.SearchTargets{{
			Type:            types.SearchTargetTypeKnowledge,
			KnowledgeBaseID: "kb-1",
			KnowledgeIDs:    []string{"doc-allowed"},
		}},
	)

	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, []string{"doc-allowed"}, knowledgeService.lastFilter.AllowedKnowledgeIDs)
	require.Len(t, infos[0].RecentDocs, 1)
	assert.Equal(t, "doc-allowed", infos[0].RecentDocs[0].KnowledgeID)
}
