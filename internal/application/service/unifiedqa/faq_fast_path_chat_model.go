package unifiedqa

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"roche.local/knowledge-agent-platform/internal/models/chat"
)

type ChatFAQFastPathModel struct{ models RouteChatModelProvider }

func NewChatFAQFastPathModel(models RouteChatModelProvider) *ChatFAQFastPathModel {
	return &ChatFAQFastPathModel{models: models}
}

func (m *ChatFAQFastPathModel) GenerateFAQFastPathReview(ctx context.Context, request FAQFastPathModelRequest) (FAQFastPathModelResponse, error) {
	callID := uuid.NewString()
	modelID, err := NewChatRouteModel(m.models).resolveModelID(ctx, request.ModelID)
	if err != nil {
		return FAQFastPathModelResponse{ModelCallID: callID}, err
	}
	chatModel, err := m.models.GetChatModel(ctx, modelID)
	if err != nil {
		return FAQFastPathModelResponse{ModelCallID: callID}, fmt.Errorf("get FAQ fast-path model: %w", err)
	}
	input, err := json.Marshal(struct {
		Question        string              `json:"question"`
		StandaloneQuery string              `json:"standalone_query"`
		FAQ             EvidenceCandidate   `json:"faq_candidate"`
		Alternatives    []EvidenceCandidate `json:"alternative_evidence"`
	}{request.Question, request.StandaloneQuery, request.Candidate, request.Alternatives})
	if err != nil {
		return FAQFastPathModelResponse{ModelCallID: callID}, fmt.Errorf("marshal FAQ fast-path input: %w", err)
	}
	thinking := false
	response, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: request.SystemPrompt},
		{Role: "user", Content: string(input)},
	}, &chat.ChatOptions{
		Temperature: 0, MaxCompletionTokens: 500, Thinking: &thinking, Format: faqFastPathJSONSchema(),
	})
	if err != nil {
		return FAQFastPathModelResponse{ModelCallID: callID}, fmt.Errorf("generate FAQ fast-path review: %w", err)
	}
	if response == nil {
		return FAQFastPathModelResponse{ModelCallID: callID}, fmt.Errorf("generate FAQ fast-path review: empty model response")
	}
	return FAQFastPathModelResponse{Content: response.Content, ModelCallID: callID}, nil
}

func faqFastPathJSONSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object","additionalProperties":false,
  "required":["eligible","risks","reason"],
  "properties":{
    "eligible":{"type":"boolean"},
    "risks":{"type":"array","uniqueItems":true,"items":{"type":"string","enum":[
      "time_sensitive","amount_or_currency","regulatory_or_compliance","conflicting_evidence",
      "scope_or_condition","ambiguous_match","incomplete_answer"
    ]}},
    "reason":{"type":"string"}
  }
}`)
}
