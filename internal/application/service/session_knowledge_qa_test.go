package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/models/asr"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/models/embedding"
	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/models/vlm"
	"roche.local/knowledge-agent-platform/internal/types"
)

type captureChatModel struct {
	lastMessages []chat.Message
}

func (m *captureChatModel) Chat(
	context.Context,
	[]chat.Message,
	*chat.ChatOptions,
) (*types.ChatResponse, error) {
	return nil, nil
}

func (m *captureChatModel) ChatStream(
	_ context.Context,
	messages []chat.Message,
	_ *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	m.lastMessages = append([]chat.Message(nil), messages...)

	ch := make(chan types.StreamResponse, 1)
	ch <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      "ok",
		Done:         true,
	}
	close(ch)
	return ch, nil
}

func (m *captureChatModel) GetModelName() string { return "capture" }
func (m *captureChatModel) GetModelID() string   { return "capture" }

type stubModelService struct {
	chatModel  chat.Chat
	modelsByID map[string]*types.Model
}

func (s *stubModelService) CreateModel(context.Context, *types.Model) error {
	return nil
}

func (s *stubModelService) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	return s.modelsByID[id], nil
}

func (s *stubModelService) ListModels(context.Context) ([]*types.Model, error) {
	return nil, nil
}

func (s *stubModelService) UpdateModel(context.Context, *types.Model) error {
	return nil
}

func (s *stubModelService) DeleteModel(context.Context, string) error {
	return nil
}

func (s *stubModelService) UpdateModelCredentials(
	context.Context, string, *string, *string,
) (*types.Model, error) {
	return nil, nil
}

func (s *stubModelService) ClearModelCredential(context.Context, string, string) error {
	return nil
}

func (s *stubModelService) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return nil, nil
}

func (s *stubModelService) GetEmbeddingModelForKnowledgeDomain(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}

func (s *stubModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}

func (s *stubModelService) GetChatModel(context.Context, string) (chat.Chat, error) {
	return s.chatModel, nil
}

func (s *stubModelService) GetVLMModel(context.Context, string) (vlm.VLM, error) {
	return nil, nil
}

func (s *stubModelService) GetASRModel(context.Context, string) (asr.ASR, error) {
	return nil, nil
}

func TestHandleModelFallback_IncludesHistoryMessages(t *testing.T) {
	chatModel := &captureChatModel{}
	svc := &sessionService{
		modelService: &stubModelService{chatModel: chatModel},
	}

	bus := event.NewEventBus()
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			SessionID:      "session-1",
			Query:          "现在还能继续讲吗？",
			ChatModelID:    "chat-model",
			FallbackPrompt: "Answer the latest user question: {{query}}",
			SummaryConfig: types.SummaryConfig{
				Temperature: 0.2,
			},
			Language: "zh-CN",
		},
		PipelineState: types.PipelineState{
			History: []*types.History{
				{
					Query:  "先介绍一下 RocheKAP",
					Answer: "RocheKAP 是一个知识库问答系统。",
				},
			},
		},
		PipelineContext: types.PipelineContext{
			EventBus: bus.AsEventBusInterface(),
		},
	}

	svc.handleModelFallback(context.Background(), cm)

	require.Len(t, chatModel.lastMessages, 3)
	assert.Equal(t, "user", chatModel.lastMessages[0].Role)
	assert.Equal(t, "先介绍一下 RocheKAP", chatModel.lastMessages[0].Content)
	assert.Equal(t, "assistant", chatModel.lastMessages[1].Role)
	assert.Equal(t, "RocheKAP 是一个知识库问答系统。", chatModel.lastMessages[1].Content)
	assert.Equal(t, "user", chatModel.lastMessages[2].Role)
	assert.Contains(t, chatModel.lastMessages[2].Content, "现在还能继续讲吗？")
}
