package unifiedqa

import (
	"fmt"
	"slices"
	"strings"
)

// scopeForAgent intersects the caller's ACL-derived knowledge-base scope with
// the agent's configured knowledge-department name keywords. It can only remove
// authorized knowledge bases; it can never grant access to another one.
func scopeForAgent(scope AuthorizedScope, catalog *AgentCatalog, agentID string) (AuthorizedScope, error) {
	if catalog == nil {
		return AuthorizedScope{}, fmt.Errorf("resolve knowledge department scope: agent catalog is required")
	}
	profile, ok := catalog.Get(agentID)
	if !ok {
		return AuthorizedScope{}, fmt.Errorf("resolve knowledge department scope: unknown agent %q", agentID)
	}
	allowedKBs := make(map[string]struct{})
	result := AuthorizedScope{}
	for _, kb := range scope.KnowledgeBases {
		name := strings.ToLower(strings.TrimSpace(kb.KnowledgeDomainName))
		allowed := false
		for _, keyword := range profile.KnowledgeDomainNames {
			if strings.Contains(name, strings.ToLower(strings.TrimSpace(keyword))) {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		copy := kb
		copy.KnowledgeIDs = slices.Clone(kb.KnowledgeIDs)
		result.KnowledgeBases = append(result.KnowledgeBases, copy)
		result.KnowledgeBaseIDs = append(result.KnowledgeBaseIDs, kb.ID)
		allowedKBs[kb.ID] = struct{}{}
	}
	for _, target := range scope.SearchTargets {
		if target == nil {
			continue
		}
		if _, allowed := allowedKBs[target.KnowledgeBaseID]; !allowed {
			continue
		}
		copy := *target
		copy.KnowledgeIDs = slices.Clone(target.KnowledgeIDs)
		copy.TagIDs = slices.Clone(target.TagIDs)
		result.SearchTargets = append(result.SearchTargets, &copy)
	}
	if len(result.KnowledgeBaseIDs) == 0 {
		return AuthorizedScope{}, fmt.Errorf("%w for knowledge department agent %q", ErrNoAccessibleKnowledgeBase, agentID)
	}
	return result, nil
}

// scopeForTopic narrows an already department- and ACL-filtered scope by
// configured knowledge-base name fragments. Matching is case-insensitive and
// can only remove knowledge bases; it never expands authorization.
func scopeForTopic(scope AuthorizedScope, catalog *AgentCatalog, agentID, topicID string) (AuthorizedScope, error) {
	topic, ok := catalog.Topic(topicID)
	if !ok || topic.AgentID != agentID {
		return AuthorizedScope{}, fmt.Errorf("resolve topic scope: topic %q does not belong to agent %q", topicID, agentID)
	}
	allowedKBs := make(map[string]struct{})
	result := AuthorizedScope{}
	for _, kb := range scope.KnowledgeBases {
		name := strings.ToLower(strings.TrimSpace(kb.Name))
		matched := false
		for _, fragment := range topic.KnowledgeBaseNameContains {
			if strings.Contains(name, strings.ToLower(strings.TrimSpace(fragment))) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		copy := kb
		copy.KnowledgeIDs = slices.Clone(kb.KnowledgeIDs)
		result.KnowledgeBases = append(result.KnowledgeBases, copy)
		result.KnowledgeBaseIDs = append(result.KnowledgeBaseIDs, kb.ID)
		allowedKBs[kb.ID] = struct{}{}
	}
	for _, target := range scope.SearchTargets {
		if target == nil {
			continue
		}
		if _, allowed := allowedKBs[target.KnowledgeBaseID]; !allowed {
			continue
		}
		copy := *target
		copy.KnowledgeIDs = slices.Clone(target.KnowledgeIDs)
		copy.TagIDs = slices.Clone(target.TagIDs)
		result.SearchTargets = append(result.SearchTargets, &copy)
	}
	if len(result.KnowledgeBaseIDs) == 0 {
		return AuthorizedScope{}, fmt.Errorf("%w for topic %q", ErrNoAccessibleKnowledgeBase, topicID)
	}
	return result, nil
}
