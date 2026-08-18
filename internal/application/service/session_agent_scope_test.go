package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type authorizedAgentKBService struct {
	interfaces.KnowledgeBaseService
	kbs []*types.KnowledgeBase
}

func (s *authorizedAgentKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

func (s *authorizedAgentKBService) GetKnowledgeBasesByIDsOnly(
	_ context.Context,
	ids []string,
) ([]*types.KnowledgeBase, error) {
	requested := make(map[string]bool, len(ids))
	for _, id := range ids {
		requested[id] = true
	}
	result := make([]*types.KnowledgeBase, 0, len(ids))
	for _, kb := range s.kbs {
		if kb != nil && requested[kb.ID] {
			result = append(result, kb)
		}
	}
	return result, nil
}

type authorizedAgentAccessService struct {
	interfaces.EnterpriseAccessService
	scopes map[string]*types.KnowledgeBaseAccessScope
}

func (s *authorizedAgentAccessService) ResolveKnowledgeBaseAccess(
	_ context.Context,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBaseAccessScope, error) {
	return s.scopes[kb.ID], nil
}

func TestBuildAuthorizedAgentSearchTargets_UsesOnlyEffectiveUserGrants(t *testing.T) {
	svc := &sessionService{
		knowledgeBaseService: &authorizedAgentKBService{
			kbs: []*types.KnowledgeBase{
				{ID: "kb-full", KnowledgeDomainID: 10},
				{ID: "kb-docs", KnowledgeDomainID: 20},
			},
		},
		accessService: &authorizedAgentAccessService{
			scopes: map[string]*types.KnowledgeBaseAccessScope{
				"kb-full": {
					Allowed:    true,
					FullAccess: true,
				},
				"kb-docs": {
					Allowed:      true,
					KnowledgeIDs: []string{"doc-1", "doc-2"},
				},
			},
		},
	}

	targets, err := svc.buildAuthorizedAgentSearchTargets(context.Background())

	require.NoError(t, err)
	require.Len(t, targets, 2)

	byKB := make(map[string]*types.SearchTarget, len(targets))
	for _, target := range targets {
		byKB[target.KnowledgeBaseID] = target
	}
	assert.Equal(t, types.SearchTargetTypeKnowledgeBase, byKB["kb-full"].Type)
	assert.Equal(t, uint64(10), byKB["kb-full"].KnowledgeDomainID)
	assert.Equal(t, types.SearchTargetTypeKnowledge, byKB["kb-docs"].Type)
	assert.Equal(t, uint64(20), byKB["kb-docs"].KnowledgeDomainID)
	assert.ElementsMatch(t, []string{"doc-1", "doc-2"}, byKB["kb-docs"].KnowledgeIDs)
	assert.ElementsMatch(t, []string{"doc-1", "doc-2"}, knowledgeIDsFromSearchTargets(targets))
}
