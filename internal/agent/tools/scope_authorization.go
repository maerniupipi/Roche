package tools

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type knowledgeTagsFetcher func(context.Context, []string) (map[string][]*types.KnowledgeTag, error)

func searchTargetsAllowFullKnowledgeBase(searchTargets types.SearchTargets, kbID string) bool {
	if kbID == "" {
		return false
	}
	for _, target := range searchTargets {
		if target == nil || target.KnowledgeBaseID != kbID {
			continue
		}
		if target.Type == types.SearchTargetTypeKnowledgeBase && len(target.TagIDs) == 0 {
			return true
		}
	}
	return false
}

// searchTargetsKnowledgeScope resolves the authorized scope for one knowledge
// base. A full KB target wins; otherwise the returned IDs are the exact
// document-level authorization boundary.
func searchTargetsKnowledgeScope(
	searchTargets types.SearchTargets,
	kbID string,
) (allowed bool, fullAccess bool, knowledgeIDs []string) {
	for _, target := range searchTargets {
		if target == nil || target.KnowledgeBaseID != kbID {
			continue
		}
		allowed = true
		if target.Type == types.SearchTargetTypeKnowledgeBase && len(target.TagIDs) == 0 {
			return true, true, nil
		}
		knowledgeIDs = append(knowledgeIDs, target.KnowledgeIDs...)
	}
	return allowed, false, dedupNonEmptyStrings(knowledgeIDs)
}

func searchTargetsAllowKnowledgeID(
	ctx context.Context,
	searchTargets types.SearchTargets,
	knowledgeID string,
	kbID string,
	knowledgeService interfaces.KnowledgeService,
) (bool, error) {
	if knowledgeID == "" || kbID == "" {
		return false, nil
	}

	var tagIDs []string
	matchedKB := false
	for _, target := range searchTargets {
		if target == nil || target.KnowledgeBaseID != kbID {
			continue
		}
		matchedKB = true
		if target.Type == types.SearchTargetTypeKnowledgeBase && len(target.TagIDs) == 0 {
			return true, nil
		}
		for _, allowedID := range target.KnowledgeIDs {
			if allowedID == knowledgeID {
				return true, nil
			}
		}
		tagIDs = append(tagIDs, target.TagIDs...)
	}
	if !matchedKB || len(tagIDs) == 0 || knowledgeService == nil {
		return false, nil
	}

	matches, err := knowledgeIDsMatchingAnyTag(ctx, []string{knowledgeID}, tagIDs, knowledgeService.GetKnowledgeTags)
	if err != nil {
		return false, err
	}
	return matches[knowledgeID], nil
}

func knowledgeIDsMatchingAnyTag(
	ctx context.Context,
	knowledgeIDs []string,
	tagIDs []string,
	fetchTags knowledgeTagsFetcher,
) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(knowledgeIDs) == 0 || len(tagIDs) == 0 || fetchTags == nil {
		return result, nil
	}

	uniqueKnowledgeIDs := dedupNonEmptyStrings(knowledgeIDs)
	uniqueTagIDs := dedupNonEmptyStrings(tagIDs)
	if len(uniqueKnowledgeIDs) == 0 || len(uniqueTagIDs) == 0 {
		return result, nil
	}

	tagSet := make(map[string]bool, len(uniqueTagIDs))
	for _, tagID := range uniqueTagIDs {
		tagSet[tagID] = true
	}

	tagMap, err := fetchTags(ctx, uniqueKnowledgeIDs)
	if err != nil {
		return nil, err
	}
	for _, knowledgeID := range uniqueKnowledgeIDs {
		for _, tag := range tagMap[knowledgeID] {
			if tag != nil && tagSet[tag.ID] {
				result[knowledgeID] = true
				break
			}
		}
	}
	return result, nil
}
