package unifiedqa

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"roche.local/knowledge-agent-platform/internal/types"
)

type KnowledgeBaseLister interface {
	ListKnowledgeBases(ctx context.Context) ([]*types.KnowledgeBase, error)
}

type KnowledgeBaseAccessResolver interface {
	ResolveKnowledgeBaseAccess(ctx context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBaseAccessScope, error)
}

type AuthorizedKBResolver struct {
	lister         KnowledgeBaseLister
	accessResolver KnowledgeBaseAccessResolver
	domains        KnowledgeDomainBatchResolver
}

type KnowledgeDomainBatchResolver interface {
	GetKnowledgeDomainsByIDs(context.Context, []uint64) (map[uint64]*types.KnowledgeDomain, error)
}

func NewAuthorizedKBResolver(lister KnowledgeBaseLister, domainResolvers ...KnowledgeDomainBatchResolver) *AuthorizedKBResolver {
	resolver, _ := lister.(KnowledgeBaseAccessResolver)
	var domains KnowledgeDomainBatchResolver
	if len(domainResolvers) > 0 {
		domains = domainResolvers[0]
	}
	return &AuthorizedKBResolver{lister: lister, accessResolver: resolver, domains: domains}
}

// Resolve takes the effective ACL-filtered list from KnowledgeBaseService and
// converts every visible, non-temporary KB into one stable retrieval scope.
func (r *AuthorizedKBResolver) Resolve(ctx context.Context) (AuthorizedScope, error) {
	if r == nil || r.lister == nil {
		return AuthorizedScope{}, fmt.Errorf("resolve authorized knowledge bases: lister is required")
	}
	kbs, err := r.lister.ListKnowledgeBases(ctx)
	if err != nil {
		return AuthorizedScope{}, fmt.Errorf("resolve authorized knowledge bases: %w", err)
	}

	valid := make([]*types.KnowledgeBase, 0, len(kbs))
	seen := make(map[string]struct{}, len(kbs))
	for _, kb := range kbs {
		if kb == nil || strings.TrimSpace(kb.ID) == "" || kb.IsTemporary || kb.DeletedAt.Valid {
			continue
		}
		if _, duplicate := seen[kb.ID]; duplicate {
			continue
		}
		seen[kb.ID] = struct{}{}
		valid = append(valid, kb)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].ID < valid[j].ID })
	if len(valid) == 0 {
		return AuthorizedScope{}, ErrNoAccessibleKnowledgeBase
	}
	domainIDs := make([]uint64, 0, len(valid))
	seenDomainIDs := make(map[uint64]struct{}, len(valid))
	for _, kb := range valid {
		if _, seen := seenDomainIDs[kb.KnowledgeDomainID]; !seen {
			seenDomainIDs[kb.KnowledgeDomainID] = struct{}{}
			domainIDs = append(domainIDs, kb.KnowledgeDomainID)
		}
	}
	domainMap := map[uint64]*types.KnowledgeDomain{}
	if r.domains != nil {
		domainMap, err = r.domains.GetKnowledgeDomainsByIDs(ctx, domainIDs)
		if err != nil {
			return AuthorizedScope{}, fmt.Errorf("resolve knowledge departments: %w", err)
		}
	}

	scope := AuthorizedScope{
		KnowledgeBases:   make([]AuthorizedKnowledgeBase, 0, len(valid)),
		KnowledgeBaseIDs: make([]string, 0, len(valid)),
		SearchTargets:    make(types.SearchTargets, 0, len(valid)),
	}
	for _, kb := range valid {
		access := &types.KnowledgeBaseAccessScope{Allowed: true, FullAccess: true}
		if r.accessResolver != nil {
			access, err = r.accessResolver.ResolveKnowledgeBaseAccess(ctx, kb)
			if err != nil {
				return AuthorizedScope{}, fmt.Errorf("resolve knowledge base access %s: %w", kb.ID, err)
			}
			if access == nil || !access.Allowed {
				continue
			}
		}

		knowledgeIDs := uniqueAuthorizedIDs(access.KnowledgeIDs)
		if !access.FullAccess && len(knowledgeIDs) == 0 {
			// A visible empty folder may make the KB appear in the management
			// list, but it does not create a retrievable document scope.
			continue
		}
		scope.KnowledgeBases = append(scope.KnowledgeBases, AuthorizedKnowledgeBase{
			ID:                  kb.ID,
			Name:                kb.Name,
			KnowledgeDomainID:   kb.KnowledgeDomainID,
			EmbeddingModelID:    kb.EmbeddingModelID,
			Type:                kb.Type,
			KnowledgeDomainName: knowledgeDomainName(domainMap[kb.KnowledgeDomainID]),
			FullAccess:          access.FullAccess,
			KnowledgeIDs:        knowledgeIDs,
		})
		scope.KnowledgeBaseIDs = append(scope.KnowledgeBaseIDs, kb.ID)
		if access.FullAccess {
			scope.SearchTargets = append(scope.SearchTargets, &types.SearchTarget{
				Type:              types.SearchTargetTypeKnowledgeBase,
				KnowledgeBaseID:   kb.ID,
				KnowledgeDomainID: kb.KnowledgeDomainID,
			})
		} else {
			scope.SearchTargets = append(scope.SearchTargets, &types.SearchTarget{
				Type:              types.SearchTargetTypeKnowledge,
				KnowledgeBaseID:   kb.ID,
				KnowledgeDomainID: kb.KnowledgeDomainID,
				KnowledgeIDs:      knowledgeIDs,
			})
		}
	}
	if len(scope.KnowledgeBaseIDs) == 0 {
		return AuthorizedScope{}, ErrNoAccessibleKnowledgeBase
	}
	return scope, nil
}

func knowledgeDomainName(domain *types.KnowledgeDomain) string {
	if domain == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(domain.Name))
}

func uniqueAuthorizedIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
