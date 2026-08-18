package unifiedqa

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/models/chat"
)

type ChatReviewModel struct{ models RouteChatModelProvider }

const evidenceReviewMaxCompletionTokens = 4096

type reviewEvidenceCandidate struct {
	OpaqueID          string            `json:"opaque_id"`
	Title             string            `json:"title,omitempty"`
	KnowledgeFilename string            `json:"knowledge_filename,omitempty"`
	KnowledgeSource   string            `json:"knowledge_source,omitempty"`
	KnowledgeChannel  string            `json:"knowledge_channel,omitempty"`
	Content           string            `json:"content"`
	Score             float64           `json:"score,omitempty"`
	MatchedQueries    []string          `json:"matched_queries,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	ChunkType         string            `json:"chunk_type,omitempty"`
	FAQ               *FAQEvidence      `json:"faq,omitempty"`
	FAQDirectMatch    bool              `json:"faq_direct_match,omitempty"`
}

func NewChatReviewModel(models RouteChatModelProvider) *ChatReviewModel {
	return &ChatReviewModel{models: models}
}

func (m *ChatReviewModel) GenerateReview(ctx context.Context, request ReviewModelRequest) (ReviewModelResponse, error) {
	callID := uuid.NewString()
	format, err := buildEvidenceReviewJSONSchema(request.Profile, request.Attempt)
	if err != nil {
		return ReviewModelResponse{ModelCallID: callID}, err
	}
	modelID, err := NewChatRouteModel(m.models).resolveModelID(ctx, request.ModelID)
	if err != nil {
		return ReviewModelResponse{ModelCallID: callID}, err
	}
	chatModel, err := m.models.GetChatModel(ctx, modelID)
	if err != nil {
		return ReviewModelResponse{ModelCallID: callID}, fmt.Errorf("get evidence review model: %w", err)
	}
	input, err := json.Marshal(struct {
		Question             string                    `json:"question"`
		ResponseLanguage     string                    `json:"response_language"`
		Task                 AgentTask                 `json:"task"`
		CoverageChecklist    []string                  `json:"coverage_checklist"`
		ResearchRules        []string                  `json:"research_rules"`
		EvidenceRequirements []string                  `json:"evidence_requirements"`
		AllowedTools         []string                  `json:"allowed_research_tools"`
		Attempt              int                       `json:"attempt"`
		Candidates           []reviewEvidenceCandidate `json:"candidates"`
	}{
		Question:             request.Question,
		ResponseLanguage:     detectAnswerLanguage(ctx, request.Question).promptName(),
		Task:                 request.Task,
		CoverageChecklist:    buildEvidenceCoverageChecklist(request.Question, request.Task, request.Profile),
		ResearchRules:        request.Profile.ResearchRules,
		EvidenceRequirements: request.Profile.EvidenceRequirements,
		AllowedTools:         request.Profile.AllowedResearchTools,
		Attempt:              request.Attempt,
		Candidates:           compactReviewEvidenceCandidates(request.Candidates),
	})
	if err != nil {
		return ReviewModelResponse{ModelCallID: callID}, fmt.Errorf("marshal evidence review input: %w", err)
	}
	thinking := false
	response, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: request.SystemPrompt},
		{Role: "user", Content: string(input)},
	}, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: evidenceReviewMaxCompletionTokens,
		Thinking:            &thinking,
		Format:              format,
	})
	if err != nil {
		return ReviewModelResponse{ModelCallID: callID}, fmt.Errorf("generate evidence review: %w", err)
	}
	if response == nil {
		return ReviewModelResponse{ModelCallID: callID}, fmt.Errorf("generate evidence review: empty model response")
	}
	return ReviewModelResponse{Content: response.Content, ModelCallID: callID}, nil
}

func buildEvidenceCoverageChecklist(question string, task AgentTask, profile DomainAgentProfile) []string {
	checklist := make([]string, 0, 2+len(task.SearchQueries)+len(profile.EvidenceRequirements))
	seen := make(map[string]struct{}, cap(checklist))
	appendItem := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, duplicate := seen[key]; duplicate {
			return
		}
		seen[key] = struct{}{}
		checklist = append(checklist, value)
	}
	appendItem(question)
	appendItem(task.Goal)
	for _, query := range task.SearchQueries {
		appendItem(query)
	}
	for _, requirement := range profile.EvidenceRequirements {
		appendItem(requirement)
	}
	return checklist
}

func compactReviewEvidenceCandidates(candidates []EvidenceCandidate) []reviewEvidenceCandidate {
	compacted := make([]reviewEvidenceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		compacted = append(compacted, reviewEvidenceCandidate{
			OpaqueID:          candidate.OpaqueID,
			Title:             candidate.Title,
			KnowledgeFilename: candidate.KnowledgeFilename,
			KnowledgeSource:   candidate.KnowledgeSource,
			KnowledgeChannel:  candidate.KnowledgeChannel,
			Content:           candidate.Content,
			Score:             candidate.Score,
			MatchedQueries:    candidate.MatchedQueries,
			Metadata:          candidate.Metadata,
			ChunkType:         candidate.ChunkType,
			FAQ:               candidate.FAQ,
			FAQDirectMatch:    candidate.FAQDirectMatch,
		})
	}
	return compacted
}

func buildEvidenceReviewJSONSchema(profile DomainAgentProfile, attempt int) (json.RawMessage, error) {
	if profile.ID == "" {
		return nil, fmt.Errorf("evidence review schema requires an agent ID")
	}
	agentID, err := json.Marshal([]string{profile.ID})
	if err != nil {
		return nil, fmt.Errorf("marshal evidence review agent ID: %w", err)
	}
	allowedTools, err := json.Marshal(profile.AllowedResearchTools)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence review tools: %w", err)
	}
	recoverySchema := fmt.Sprintf(`{"type":["object","null"],"additionalProperties":false,
	  "required":["tool","query"],"properties":{"tool":{"type":"string","enum":%s},"query":{"type":"string","maxLength":500},"queries":{"type":"array","maxItems":3,"items":{"type":"string","maxLength":500}},"terms":{"type":"array","maxItems":10,"items":{"type":"string","maxLength":100}}}}`, allowedTools)
	if attempt > 0 {
		recoverySchema = `{"type":"null"}`
	}
	return json.RawMessage(fmt.Sprintf(`{
  "type":"object","additionalProperties":false,
  "required":["agent_id","status","facts","requires_scenario_selection","missing_requirements","conflicts"],
  "properties":{
    "agent_id":{"type":"string","enum":%s},
    "status":{"type":"string","enum":["sufficient","insufficient"]},
	"facts":{"type":"array","maxItems":8,"items":{"type":"object","additionalProperties":false,
		"required":["statement","quote","is_ambiguous","scenario","document_level","currency","citations"],"properties":{
		"statement":{"type":"string","maxLength":400},
		"quote":{"type":"string","maxLength":800},
		"is_ambiguous":{"type":"boolean"},
		"scenario":{"type":"string","maxLength":200},
		"document_level":{"type":"string","enum":["internal_sop","formal_policy","industry_guideline","other","unspecified"]},
		"currency":{"type":"string","enum":["RMB","CHF","mixed","unspecified"]},
		"citations":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"object","additionalProperties":false,
		  "required":["opaque_id"],"properties":{"opaque_id":{"type":"string"}}}}
      }}},
	"requires_scenario_selection":{"type":"boolean"},
	"missing_requirements":{"type":"array","maxItems":10,"items":{"type":"string","maxLength":500}},
	"conflicts":{"type":"array","maxItems":10,"items":{"type":"string","maxLength":500}},
    "recovery_request":%s
  }
}`, agentID, recoverySchema)), nil
}
