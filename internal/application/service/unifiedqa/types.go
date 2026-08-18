package unifiedqa

import (
	"encoding/json"
	"fmt"
	"slices"

	"roche.local/knowledge-agent-platform/internal/types"
)

type ConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AuthorizedKnowledgeBase struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	KnowledgeDomainID   uint64   `json:"knowledge_domain_id"`
	EmbeddingModelID    string   `json:"embedding_model_id,omitempty"`
	Type                string   `json:"type,omitempty"`
	KnowledgeDomainName string   `json:"knowledge_domain_name,omitempty"`
	FullAccess          bool     `json:"full_access"`
	KnowledgeIDs        []string `json:"knowledge_ids,omitempty"`
}

// AuthorizedScope is resolved once at request entry. Each domain agent receives
// an intersection of this ACL-derived scope and its configured knowledge department.
type AuthorizedScope struct {
	KnowledgeBases   []AuthorizedKnowledgeBase `json:"knowledge_bases"`
	KnowledgeBaseIDs []string                  `json:"knowledge_base_ids"`
	SearchTargets    types.SearchTargets       `json:"search_targets"`
}

type AgentTask struct {
	AgentID       string   `json:"agent_id"`
	TopicIDs      []string `json:"topic_ids,omitempty"`
	Goal          string   `json:"goal"`
	SearchQueries []string `json:"search_queries"`
	ExactTerms    []string `json:"exact_terms,omitempty"`
	DocumentTypes []string `json:"document_types,omitempty"`
	ToolIntent    string   `json:"tool_intent,omitempty"`
}

type MasterRoutePlan struct {
	StandaloneQuery string            `json:"standalone_query"`
	Intent          string            `json:"intent"`
	Outcome         string            `json:"outcome,omitempty"`
	Entities        map[string]string `json:"entities,omitempty"`
	Tasks           []AgentTask       `json:"tasks"`
	Degraded        bool              `json:"degraded,omitempty"`
}

type EvidenceCandidate struct {
	OpaqueID          string            `json:"opaque_id"`
	KnowledgeBaseID   string            `json:"knowledge_base_id"`
	KnowledgeID       string            `json:"knowledge_id"`
	ChunkID           string            `json:"chunk_id"`
	ChunkIndex        int               `json:"chunk_index,omitempty"`
	StartAt           int               `json:"start_at,omitempty"`
	EndAt             int               `json:"end_at,omitempty"`
	Title             string            `json:"title"`
	KnowledgeFilename string            `json:"knowledge_filename,omitempty"`
	KnowledgeSource   string            `json:"knowledge_source,omitempty"`
	KnowledgeChannel  string            `json:"knowledge_channel,omitempty"`
	Description       string            `json:"knowledge_description,omitempty"`
	Content           string            `json:"content"`
	ImageInfo         string            `json:"image_info,omitempty"`
	Score             float64           `json:"score"`
	RetrievalScore    float64           `json:"retrieval_score"`
	RerankScore       float64           `json:"rerank_score,omitempty"`
	MatchedQueries    []string          `json:"matched_queries,omitempty"`
	RetrievalChannels []string          `json:"retrieval_channels,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	ChunkType         string            `json:"chunk_type,omitempty"`
	FAQ               *FAQEvidence      `json:"faq,omitempty"`
	FAQDirectMatch    bool              `json:"faq_direct_match,omitempty"`
}

type FAQEvidence struct {
	StandardQuestion string               `json:"standard_question"`
	Answers          []string             `json:"answers"`
	AnswerStrategy   types.AnswerStrategy `json:"answer_strategy,omitempty"`
}

type EvidenceCitation struct {
	OpaqueID string `json:"opaque_id"`
	Quote    string `json:"quote,omitempty"`
}

type ObservedFact struct {
	Statement          string             `json:"statement"`
	Quote              string             `json:"quote,omitempty"`
	IsAmbiguous        bool               `json:"is_ambiguous,omitempty"`
	Scenario           string             `json:"scenario,omitempty"`
	DocumentLevel      string             `json:"document_level,omitempty"`
	Currency           string             `json:"currency,omitempty"`
	Citations          []EvidenceCitation `json:"citations"`
	ContributingAgents []string           `json:"contributing_agents,omitempty"`
}

type RecoveryRequest struct {
	Tool    string   `json:"tool"`
	Query   string   `json:"query"`
	Queries []string `json:"queries,omitempty"`
	Terms   []string `json:"terms,omitempty"`
}

type AgentObservation struct {
	AgentID                   string           `json:"agent_id"`
	TopicID                   string           `json:"topic_id,omitempty"`
	Status                    string           `json:"status"`
	Facts                     []ObservedFact   `json:"facts"`
	RequiresScenarioSelection bool             `json:"requires_scenario_selection"`
	MissingRequirements       []string         `json:"missing_requirements,omitempty"`
	Conflicts                 []string         `json:"conflicts,omitempty"`
	RecoveryRequest           *RecoveryRequest `json:"recovery_request,omitempty"`
	Metrics                   map[string]any   `json:"metrics,omitempty"`
}

type RunContextInput struct {
	RunID           string
	SessionID       string
	RequestID       string
	UserID          string
	OriginalQuery   string
	History         []ConversationTurn
	AuthorizedScope AuthorizedScope
	ConfigSnapshot  types.JSONMap
}

// RunContext is a per-request immutable-by-convention snapshot. Construct it
// with NewRunContext; callers should not share mutable input slices or maps.
type RunContext struct {
	RunID           string
	SessionID       string
	RequestID       string
	UserID          string
	OriginalQuery   string
	History         []ConversationTurn
	AuthorizedScope AuthorizedScope
	ConfigSnapshot  types.JSONMap
}

func NewRunContext(input RunContextInput) (*RunContext, error) {
	snapshot, err := cloneJSONMap(input.ConfigSnapshot)
	if err != nil {
		return nil, fmt.Errorf("clone unified QA config snapshot: %w", err)
	}
	return &RunContext{
		RunID:           input.RunID,
		SessionID:       input.SessionID,
		RequestID:       input.RequestID,
		UserID:          input.UserID,
		OriginalQuery:   input.OriginalQuery,
		History:         slices.Clone(input.History),
		AuthorizedScope: cloneAuthorizedScope(input.AuthorizedScope),
		ConfigSnapshot:  snapshot,
	}, nil
}

func cloneAuthorizedScope(scope AuthorizedScope) AuthorizedScope {
	cloned := AuthorizedScope{
		KnowledgeBases:   slices.Clone(scope.KnowledgeBases),
		KnowledgeBaseIDs: slices.Clone(scope.KnowledgeBaseIDs),
		SearchTargets:    make(types.SearchTargets, 0, len(scope.SearchTargets)),
	}
	for i := range cloned.KnowledgeBases {
		cloned.KnowledgeBases[i].KnowledgeIDs = slices.Clone(scope.KnowledgeBases[i].KnowledgeIDs)
	}
	for _, target := range scope.SearchTargets {
		if target == nil {
			cloned.SearchTargets = append(cloned.SearchTargets, nil)
			continue
		}
		copy := *target
		copy.KnowledgeIDs = slices.Clone(target.KnowledgeIDs)
		copy.TagIDs = slices.Clone(target.TagIDs)
		cloned.SearchTargets = append(cloned.SearchTargets, &copy)
	}
	return cloned
}

func cloneJSONMap(value types.JSONMap) (types.JSONMap, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned types.JSONMap
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}
