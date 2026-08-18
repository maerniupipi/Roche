package service

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
)

func (s *knowledgeBaseService) fetchKnowledgeData(
	ctx context.Context,
	knowledgeDomainID uint64,
	knowledgeIDs []string,
) (map[string]*types.Knowledge, error) {
	knowledges, err := s.kgRepo.GetKnowledgeBatch(ctx, knowledgeDomainID, knowledgeIDs)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_domain_id": knowledgeDomainID,
			"knowledge_ids":       knowledgeIDs,
		})
		return nil, err
	}

	knowledgeMap := make(map[string]*types.Knowledge, len(knowledges))
	for _, knowledge := range knowledges {
		knowledgeMap[knowledge.ID] = knowledge
	}
	return knowledgeMap, nil
}

func (s *knowledgeBaseService) listChunksByID(
	ctx context.Context,
	knowledgeDomainID uint64,
	chunkIDs []string,
) ([]*types.Chunk, error) {
	return s.chunkRepo.ListChunksByID(ctx, knowledgeDomainID, chunkIDs)
}
