package unifiedqa

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/types"
)

type HybridSearcher interface {
	HybridSearch(ctx context.Context, primaryKBID string, params types.SearchParams) ([]*types.SearchResult, error)
}

type KnowledgeDomainLookup interface {
	GetKnowledgeDomainsByIDs(ctx context.Context, ids []uint64) (map[uint64]*types.KnowledgeDomain, error)
}

type RetrievalSettings struct {
	MatchCount       int
	VectorThreshold  float64
	KeywordThreshold float64
	RerankTopK       int
	RerankThreshold  float64
}

type RetrievalResult struct {
	Candidates     []EvidenceCandidate
	ToolCalls      int
	RerankDegraded bool
}

type RetrievalPolicy struct {
	FAQPriorityEnabled       bool    `json:"faq_priority_enabled"`
	FAQDirectAnswerThreshold float64 `json:"faq_direct_answer_threshold"`
	FAQScoreBoost            float64 `json:"faq_score_boost"`
}

func DefaultRetrievalPolicy() RetrievalPolicy {
	return RetrievalPolicy{
		FAQPriorityEnabled:       true,
		FAQDirectAnswerThreshold: 0.9,
		FAQScoreBoost:            1.2,
	}
}

func (p RetrievalPolicy) normalized() RetrievalPolicy {
	if p.FAQDirectAnswerThreshold <= 0 || p.FAQDirectAnswerThreshold > 1 {
		p.FAQDirectAnswerThreshold = 0.9
	}
	if p.FAQScoreBoost <= 0 {
		p.FAQScoreBoost = 1
	}
	return p
}

type RetrievalAdapter struct {
	searcher HybridSearcher
	domains  KnowledgeDomainLookup
	settings RetrievalSettings
}

func NewRetrievalAdapter(
	searcher HybridSearcher,
	settings RetrievalSettings,
	domainLookups ...KnowledgeDomainLookup,
) *RetrievalAdapter {
	if settings.MatchCount <= 0 {
		settings.MatchCount = 20
	}
	if settings.RerankTopK <= 0 {
		settings.RerankTopK = 10
	}
	var domains KnowledgeDomainLookup
	if len(domainLookups) > 0 {
		domains = domainLookups[0]
	}
	return &RetrievalAdapter{searcher: searcher, domains: domains, settings: settings}
}

// Retrieve reuses HybridSearch for every approved query, merges cross-query
// results without a second RRF, then makes at most one reranker call.
func (a *RetrievalAdapter) Retrieve(
	ctx context.Context,
	task AgentTask,
	scope AuthorizedScope,
	rerankerModel rerank.Reranker,
	policy RetrievalPolicy,
) (RetrievalResult, error) {
	return a.retrieve(ctx, task, scope, rerankerModel, nil, policy)
}

func (a *RetrievalAdapter) RetrieveWithExisting(
	ctx context.Context,
	task AgentTask,
	scope AuthorizedScope,
	rerankerModel rerank.Reranker,
	existing []EvidenceCandidate,
	policy RetrievalPolicy,
) (RetrievalResult, error) {
	return a.retrieve(ctx, task, scope, rerankerModel, existing, policy)
}

func (a *RetrievalAdapter) retrieve(
	ctx context.Context,
	task AgentTask,
	scope AuthorizedScope,
	rerankerModel rerank.Reranker,
	existing []EvidenceCandidate,
	policy RetrievalPolicy,
) (RetrievalResult, error) {
	if len(scope.KnowledgeBaseIDs) == 0 {
		return RetrievalResult{}, ErrNoAccessibleKnowledgeBase
	}
	if a == nil || a.searcher == nil {
		return RetrievalResult{}, fmt.Errorf("hybrid searcher is required")
	}
	if len(task.SearchQueries) == 0 || len(task.SearchQueries) > 3 {
		return RetrievalResult{}, fmt.Errorf("task must contain one to three search queries")
	}
	policy = policy.normalized()

	kbIDs := slices.Clone(scope.KnowledgeBaseIDs)
	sort.Strings(kbIDs)
	allowedKBs := make(map[string]struct{}, len(kbIDs))
	for _, id := range kbIDs {
		allowedKBs[id] = struct{}{}
	}
	merged := make(map[string]*EvidenceCandidate)
	for _, candidate := range existing {
		copy := cloneEvidenceCandidate(candidate)
		merged[evidenceKey(candidate.KnowledgeBaseID, candidate.KnowledgeID, candidate.ChunkID)] = &copy
	}
	groups, err := buildRetrievalScopeGroups(scope)
	if err != nil {
		return RetrievalResult{}, err
	}
	domainInfos, err := a.loadKnowledgeDomains(ctx, groups)
	if err != nil {
		return RetrievalResult{}, err
	}
	toolCalls := 0
	for _, query := range task.SearchQueries {
		query = strings.TrimSpace(query)
		if query == "" {
			return RetrievalResult{}, fmt.Errorf("search query is empty")
		}
		for _, group := range groups {
			params := types.SearchParams{
				QueryText:             query,
				VectorThreshold:       a.settings.VectorThreshold,
				KeywordThreshold:      a.settings.KeywordThreshold,
				MatchCount:            a.settings.MatchCount,
				KnowledgeBaseIDs:      slices.Clone(group.KnowledgeBaseIDs),
				KnowledgeIDs:          slices.Clone(group.KnowledgeIDs),
				SkipContextEnrichment: true,
			}
			if task.ToolIntent == "grep_chunks" {
				params.DisableVectorMatch = true
			}
			searchCtx := ctx
			if group.KnowledgeDomainID != 0 {
				searchCtx = context.WithValue(searchCtx, types.KnowledgeDomainIDContextKey, group.KnowledgeDomainID)
				searchCtx = context.WithValue(searchCtx, types.KnowledgeDomainInfoContextKey, domainInfos[group.KnowledgeDomainID])
			}
			results, searchErr := a.searcher.HybridSearch(searchCtx, group.KnowledgeBaseIDs[0], params)
			toolCalls++
			if searchErr != nil {
				return RetrievalResult{ToolCalls: toolCalls}, fmt.Errorf("hybrid search %q: %w", query, searchErr)
			}
			for _, result := range results {
				if result == nil || strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.KnowledgeID) == "" {
					continue
				}
				if _, allowed := allowedKBs[result.KnowledgeBaseID]; !allowed {
					continue
				}
				mergeSearchResult(merged, result, query)
			}
		}
	}

	candidates := make([]EvidenceCandidate, 0, len(merged))
	for _, candidate := range merged {
		candidates = append(candidates, *candidate)
	}
	sortEvidenceByRetrieval(candidates)
	result := RetrievalResult{Candidates: candidates, ToolCalls: toolCalls}
	if len(candidates) == 0 || rerankerModel == nil {
		applyFAQPolicy(candidates, policy, false)
		result.Candidates = limitEvidence(candidates, a.settings.RerankTopK)
		return result, nil
	}

	rerankCandidates := make([]EvidenceCandidate, 0, len(candidates))
	passages := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Content) == "" {
			continue
		}
		rerankCandidates = append(rerankCandidates, candidate)
		passages = append(passages, candidate.Content)
	}
	if len(passages) == 0 {
		applyFAQPolicy(candidates, policy, false)
		result.Candidates = limitEvidence(candidates, a.settings.RerankTopK)
		return result, nil
	}
	ranks, err := rerankerModel.Rerank(ctx, strings.Join(task.SearchQueries, "\n"), passages)
	if err != nil {
		result.RerankDegraded = true
		applyFAQPolicy(candidates, policy, false)
		result.Candidates = limitEvidence(candidates, a.settings.RerankTopK)
		return result, nil
	}
	reranked := make([]EvidenceCandidate, 0, min(len(ranks), a.settings.RerankTopK))
	seenIndices := make(map[int]struct{}, len(ranks))
	for _, rank := range ranks {
		if rank.Index < 0 || rank.Index >= len(rerankCandidates) || rank.RelevanceScore < a.settings.RerankThreshold {
			continue
		}
		if _, duplicate := seenIndices[rank.Index]; duplicate {
			continue
		}
		seenIndices[rank.Index] = struct{}{}
		candidate := rerankCandidates[rank.Index]
		candidate.RerankScore = rank.RelevanceScore
		candidate.Score = rank.RelevanceScore
		reranked = append(reranked, candidate)
	}
	applyFAQPolicy(reranked, policy, true)
	result.Candidates = limitEvidence(reranked, a.settings.RerankTopK)
	return result, nil
}

type retrievalScopeGroup struct {
	KnowledgeDomainID uint64
	KnowledgeBaseIDs  []string
	KnowledgeIDs      []string
}

type retrievalScopeGroupKey struct {
	KnowledgeDomainID uint64
	EmbeddingModelID  string
	KnowledgeBaseType string
	PartialAccess     bool
}

func buildRetrievalScopeGroups(scope AuthorizedScope) ([]retrievalScopeGroup, error) {
	// Backward compatibility for internal callers/tests that predate SearchTargets.
	// Production unified QA always supplies SearchTargets through AuthorizedKBResolver.
	if len(scope.SearchTargets) == 0 {
		kbIDs := uniqueAuthorizedIDs(scope.KnowledgeBaseIDs)
		if len(kbIDs) == 0 {
			return nil, ErrNoAccessibleKnowledgeBase
		}
		return []retrievalScopeGroup{{KnowledgeBaseIDs: kbIDs}}, nil
	}

	metadata := make(map[string]AuthorizedKnowledgeBase, len(scope.KnowledgeBases))
	for _, kb := range scope.KnowledgeBases {
		metadata[kb.ID] = kb
	}
	grouped := make(map[retrievalScopeGroupKey]*retrievalScopeGroup)
	for _, target := range scope.SearchTargets {
		if target == nil || strings.TrimSpace(target.KnowledgeBaseID) == "" {
			continue
		}
		partial := target.Type != types.SearchTargetTypeKnowledgeBase
		knowledgeIDs := uniqueAuthorizedIDs(target.KnowledgeIDs)
		if partial && len(knowledgeIDs) == 0 {
			continue
		}
		meta := metadata[target.KnowledgeBaseID]
		key := retrievalScopeGroupKey{
			KnowledgeDomainID: target.KnowledgeDomainID,
			EmbeddingModelID:  meta.EmbeddingModelID,
			KnowledgeBaseType: meta.Type,
			PartialAccess:     partial,
		}
		group := grouped[key]
		if group == nil {
			group = &retrievalScopeGroup{KnowledgeDomainID: target.KnowledgeDomainID}
			grouped[key] = group
		}
		group.KnowledgeBaseIDs = append(group.KnowledgeBaseIDs, target.KnowledgeBaseID)
		group.KnowledgeIDs = append(group.KnowledgeIDs, knowledgeIDs...)
	}

	groups := make([]retrievalScopeGroup, 0, len(grouped))
	for _, group := range grouped {
		group.KnowledgeBaseIDs = uniqueAuthorizedIDs(group.KnowledgeBaseIDs)
		group.KnowledgeIDs = uniqueAuthorizedIDs(group.KnowledgeIDs)
		if len(group.KnowledgeBaseIDs) > 0 {
			groups = append(groups, *group)
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].KnowledgeDomainID != groups[j].KnowledgeDomainID {
			return groups[i].KnowledgeDomainID < groups[j].KnowledgeDomainID
		}
		if len(groups[i].KnowledgeIDs) != len(groups[j].KnowledgeIDs) {
			return len(groups[i].KnowledgeIDs) < len(groups[j].KnowledgeIDs)
		}
		return strings.Join(groups[i].KnowledgeBaseIDs, "\x00") < strings.Join(groups[j].KnowledgeBaseIDs, "\x00")
	})
	if len(groups) == 0 {
		return nil, ErrNoAccessibleKnowledgeBase
	}
	return groups, nil
}

func (a *RetrievalAdapter) loadKnowledgeDomains(
	ctx context.Context,
	groups []retrievalScopeGroup,
) (map[uint64]*types.KnowledgeDomain, error) {
	ids := make([]uint64, 0, len(groups))
	seen := make(map[uint64]struct{}, len(groups))
	for _, group := range groups {
		if group.KnowledgeDomainID == 0 {
			continue
		}
		if _, duplicate := seen[group.KnowledgeDomainID]; duplicate {
			continue
		}
		seen[group.KnowledgeDomainID] = struct{}{}
		ids = append(ids, group.KnowledgeDomainID)
	}
	if len(ids) == 0 {
		return map[uint64]*types.KnowledgeDomain{}, nil
	}
	if a.domains == nil {
		return nil, fmt.Errorf("knowledge domain lookup is required for scoped retrieval")
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	domains, err := a.domains.GetKnowledgeDomainsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load knowledge domains: %w", err)
	}
	for _, id := range ids {
		if domains[id] == nil {
			return nil, fmt.Errorf("knowledge domain %d is unavailable", id)
		}
	}
	return domains, nil
}

func mergeSearchResult(merged map[string]*EvidenceCandidate, result *types.SearchResult, query string) {
	key := evidenceKey(result.KnowledgeBaseID, result.KnowledgeID, result.ID)
	channel := retrievalChannel(result.MatchType)
	candidate, exists := merged[key]
	if !exists {
		hash := sha256.Sum256([]byte(key))
		metadata := make(map[string]string, len(result.Metadata))
		for key, value := range result.Metadata {
			metadata[key] = value
		}
		content, faq := faqEvidenceFromSearchResult(result)
		candidate = &EvidenceCandidate{
			OpaqueID:          fmt.Sprintf("e_%x", hash[:12]),
			KnowledgeBaseID:   result.KnowledgeBaseID,
			KnowledgeID:       result.KnowledgeID,
			ChunkID:           result.ID,
			ChunkIndex:        result.ChunkIndex,
			StartAt:           result.StartAt,
			EndAt:             result.EndAt,
			Title:             result.KnowledgeTitle,
			KnowledgeFilename: result.KnowledgeFilename,
			KnowledgeSource:   result.KnowledgeSource,
			KnowledgeChannel:  result.KnowledgeChannel,
			Description:       result.KnowledgeDescription,
			Content:           content,
			ImageInfo:         result.ImageInfo,
			Score:             result.Score,
			RetrievalScore:    result.Score,
			MatchedQueries:    []string{query},
			RetrievalChannels: []string{channel},
			Metadata:          metadata,
			ChunkType:         result.ChunkType,
			FAQ:               faq,
		}
		merged[key] = candidate
		return
	}
	if !slices.Contains(candidate.MatchedQueries, query) {
		candidate.MatchedQueries = append(candidate.MatchedQueries, query)
	}
	if !slices.Contains(candidate.RetrievalChannels, channel) {
		candidate.RetrievalChannels = append(candidate.RetrievalChannels, channel)
	}
	if result.Score > candidate.RetrievalScore {
		candidate.RetrievalScore = result.Score
		candidate.Score = result.Score
		candidate.Content, candidate.FAQ = faqEvidenceFromSearchResult(result)
		if result.KnowledgeTitle != "" {
			candidate.Title = result.KnowledgeTitle
		}
		if result.ChunkIndex != 0 || candidate.ChunkIndex == 0 {
			candidate.ChunkIndex = result.ChunkIndex
		}
		if result.StartAt != 0 || candidate.StartAt == 0 {
			candidate.StartAt = result.StartAt
		}
		if result.EndAt != 0 || candidate.EndAt == 0 {
			candidate.EndAt = result.EndAt
		}
		if result.KnowledgeFilename != "" {
			candidate.KnowledgeFilename = result.KnowledgeFilename
		}
		if result.KnowledgeSource != "" {
			candidate.KnowledgeSource = result.KnowledgeSource
		}
		if result.KnowledgeChannel != "" {
			candidate.KnowledgeChannel = result.KnowledgeChannel
		}
		if result.KnowledgeDescription != "" {
			candidate.Description = result.KnowledgeDescription
		}
		if result.ImageInfo != "" {
			candidate.ImageInfo = result.ImageInfo
		}
		if result.ChunkType != "" {
			candidate.ChunkType = result.ChunkType
		}
	}
}

func faqEvidenceFromSearchResult(result *types.SearchResult) (string, *FAQEvidence) {
	if result == nil || result.ChunkType != string(types.ChunkTypeFAQ) {
		if result == nil {
			return "", nil
		}
		return result.Content, nil
	}
	var metadata types.FAQChunkMetadata
	if len(result.ChunkMetadata) == 0 || json.Unmarshal(result.ChunkMetadata, &metadata) != nil {
		return result.Content, nil
	}
	metadata.Sanitize()
	if metadata.StandardQuestion == "" && len(metadata.Answers) == 0 {
		return result.Content, nil
	}
	faq := &FAQEvidence{
		StandardQuestion: metadata.StandardQuestion,
		Answers:          slices.Clone(metadata.Answers),
		AnswerStrategy:   metadata.AnswerStrategy,
	}
	var content strings.Builder
	if faq.StandardQuestion != "" {
		content.WriteString("Q: ")
		content.WriteString(faq.StandardQuestion)
	}
	if len(faq.Answers) > 0 {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString("Answer:\n")
		for _, answer := range faq.Answers {
			content.WriteString("- ")
			content.WriteString(answer)
			content.WriteString("\n")
		}
	}
	enriched := strings.TrimSpace(content.String())
	if enriched == "" {
		enriched = result.Content
	}
	return enriched, faq
}

func applyFAQPolicy(candidates []EvidenceCandidate, policy RetrievalPolicy, reranked bool) {
	for i := range candidates {
		candidate := &candidates[i]
		baseScore := candidate.RetrievalScore
		if reranked {
			baseScore = candidate.RerankScore
		}
		candidate.Score = baseScore
		candidate.FAQDirectMatch = false
		if !policy.FAQPriorityEnabled || candidate.ChunkType != string(types.ChunkTypeFAQ) {
			continue
		}
		if candidate.Metadata == nil {
			candidate.Metadata = make(map[string]string)
		}
		if policy.FAQScoreBoost > 1 {
			candidate.Score = math.Min(baseScore*policy.FAQScoreBoost, 1)
			candidate.Metadata["faq_boosted"] = "true"
			candidate.Metadata["faq_original_score"] = fmt.Sprintf("%.4f", baseScore)
		}
		// The boost only affects ranking. A direct match must be backed by the
		// rerank model's unmodified confidence so a priority boost cannot turn a
		// merely relevant FAQ into a high-confidence direct answer.
		candidate.FAQDirectMatch = reranked && baseScore >= policy.FAQDirectAnswerThreshold
		if candidate.FAQDirectMatch {
			candidate.Metadata["faq_direct_match"] = "true"
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].OpaqueID < candidates[j].OpaqueID
	})
}

func cloneEvidenceCandidate(candidate EvidenceCandidate) EvidenceCandidate {
	copy := candidate
	copy.MatchedQueries = slices.Clone(candidate.MatchedQueries)
	copy.RetrievalChannels = slices.Clone(candidate.RetrievalChannels)
	copy.Metadata = make(map[string]string, len(candidate.Metadata))
	for key, value := range candidate.Metadata {
		copy.Metadata[key] = value
	}
	if candidate.FAQ != nil {
		faq := *candidate.FAQ
		faq.Answers = slices.Clone(candidate.FAQ.Answers)
		copy.FAQ = &faq
	}
	return copy
}

func evidenceKey(knowledgeBaseID, knowledgeID, chunkID string) string {
	return knowledgeBaseID + "\x00" + knowledgeID + "\x00" + chunkID
}

func retrievalChannel(matchType types.MatchType) string {
	switch matchType {
	case types.MatchTypeEmbedding:
		return "embedding"
	case types.MatchTypeKeywords:
		return "keywords"
	case types.MatchTypeGraph:
		return "graph"
	case types.MatchTypeDirectLoad:
		return "direct_load"
	default:
		return fmt.Sprintf("match_type_%d", matchType)
	}
}

func sortEvidenceByRetrieval(candidates []EvidenceCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].RetrievalScore != candidates[j].RetrievalScore {
			return candidates[i].RetrievalScore > candidates[j].RetrievalScore
		}
		return candidates[i].OpaqueID < candidates[j].OpaqueID
	})
}

func limitEvidence(candidates []EvidenceCandidate, topK int) []EvidenceCandidate {
	if topK > 0 && len(candidates) > topK {
		return candidates[:topK]
	}
	return candidates
}
