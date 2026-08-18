package service

import (
	"context"
	"errors"
	"fmt"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/models/asr"
	"roche.local/knowledge-agent-platform/internal/models/chat"
	"roche.local/knowledge-agent-platform/internal/models/embedding"
	"roche.local/knowledge-agent-platform/internal/models/rerank"
	"roche.local/knowledge-agent-platform/internal/models/vlm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	"roche.local/knowledge-agent-platform/internal/utils"
)

// ErrModelNotFound is returned when a model cannot be found in the repository
var ErrModelNotFound = errors.New("model not found")

// modelService implements the model service interface
type modelService struct {
	repo      interfaces.ModelRepository
	kbRepo    interfaces.KnowledgeBaseRepository
	agentRepo interfaces.CustomAgentRepository
	pooler    embedding.EmbedderPooler
}

// NewModelService creates a new model service instance
func NewModelService(repo interfaces.ModelRepository,
	kbRepo interfaces.KnowledgeBaseRepository,
	agentRepo interfaces.CustomAgentRepository,
	pooler embedding.EmbedderPooler,
) interfaces.ModelService {
	return &modelService{
		repo:      repo,
		kbRepo:    kbRepo,
		agentRepo: agentRepo,
		pooler:    pooler,
	}
}

// decryptAppSecret 解密 AppSecret（如果为空或 cryptoSvc 为空则原样返回）
func (s *modelService) decryptAppSecret(encrypted string) string {
	if encrypted == "" {
		return encrypted
	}
	if key := utils.GetAESKey(); key != nil {
		if encrypted, err := utils.DecryptAESGCM(encrypted, key); err == nil {
			return encrypted
		}
	}
	return encrypted
}

func (s *modelService) resolveModelAppSecret(params *types.ModelParameters) string {
	return s.decryptAppSecret(params.AppSecret)
}

// CreateModel creates a new model in the repository
func (s *modelService) CreateModel(ctx context.Context, model *types.Model) error {
	logger.Infof(ctx, "Creating model: %s, type: %s, source: %s", model.Name, model.Type, model.Source)
	if model.Source != types.ModelSourceRemote {
		return apperrors.NewBadRequestError("only remote model sources are supported")
	}
	model.Status = types.ModelStatusActive
	err := s.repo.Create(ctx, model)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_name": model.Name,
			"model_type": model.Type,
		})
		return err
	}
	logger.Infof(ctx, "Remote model created successfully: %s", model.ID)
	return nil
}

// GetModelByID retrieves a model by its ID
// Returns an error if the model is not found or is in a non-active state
func (s *modelService) GetModelByID(ctx context.Context, id string) (*types.Model, error) {
	// Check if ID is empty
	if id == "" {
		logger.Error(ctx, "Model ID is empty")
		return nil, errors.New("model ID cannot be empty")
	}

	// Fetch model from repository
	model, err := s.repo.GetByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id})
		return nil, err
	}

	// Check if model exists
	if model == nil {
		logger.Error(ctx, "Model not found")
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Model found, name: %s, status: %s", model.Name, model.Status)

	// Check model status
	if model.Status == types.ModelStatusActive {
		return model, nil
	}

	logger.Error(ctx, "Model status is abnormal")
	return nil, errors.New("abnormal model status")
}

// ListModels returns the platform-wide model configuration.
func (s *modelService) ListModels(ctx context.Context) ([]*types.Model, error) {
	logger.Info(ctx, "Start listing models")

	// List models from repository with no additional filters
	models, err := s.repo.List(ctx, "", "")
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}

	logger.Infof(ctx, "Retrieved %d models successfully", len(models))
	return models, nil
}

// UpdateModel updates an existing model in the repository
func (s *modelService) UpdateModel(ctx context.Context, model *types.Model) error {
	logger.Info(ctx, "Start updating model")
	logger.Infof(ctx, "Updating model ID: %s, name: %s", model.ID, model.Name)

	// Check if the model is builtin - builtin models cannot be updated
	existingModel, err := s.repo.GetByID(ctx, model.ID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": model.ID,
		})
		return err
	}
	if existingModel != nil && existingModel.IsBuiltin {
		logger.Warnf(ctx, "Attempted to update builtin model: %s", model.ID)
		return errors.New("builtin models cannot be updated")
	}

	// Update model in repository
	err = s.repo.Update(ctx, model)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return err
	}

	logger.Infof(ctx, "Model updated successfully: %s", model.ID)
	return nil
}

// UpdateModelCredentials writes one or more credential fields on the model's
// Parameters jsonb. Models are not pooled per-instance the way MCP clients
// are (each call to GetEmbeddingModel/GetChatModel rebuilds the client from
// the current Parameters), so no explicit cache invalidation is required —
// the next call will pick up the new credential automatically.
func (s *modelService) UpdateModelCredentials(
	ctx context.Context, id string, apiKey, appSecret *string,
) (*types.Model, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrModelNotFound
	}
	if existing.IsBuiltin {
		return nil, errors.New("builtin models cannot have credentials modified")
	}

	changed := false
	if apiKey != nil && *apiKey != "" && *apiKey != existing.Parameters.APIKey {
		existing.Parameters.APIKey = *apiKey
		changed = true
	}
	if appSecret != nil && *appSecret != "" && *appSecret != existing.Parameters.AppSecret {
		existing.Parameters.AppSecret = *appSecret
		changed = true
	}
	if !changed {
		return existing, nil
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "Model credentials updated: id=%s", id)
	return existing, nil
}

// ClearModelCredential removes a single credential field. Idempotent.
func (s *modelService) ClearModelCredential(ctx context.Context, id, field string) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrModelNotFound
	}
	if existing.IsBuiltin {
		return errors.New("builtin models cannot have credentials modified")
	}

	changed := false
	switch field {
	case "api_key":
		if existing.Parameters.APIKey != "" {
			existing.Parameters.APIKey = ""
			changed = true
		}
	case "app_secret":
		if existing.Parameters.AppSecret != "" {
			existing.Parameters.AppSecret = ""
			changed = true
		}
	default:
		return errors.New("unknown credential field: " + field)
	}
	if !changed {
		return nil
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return err
	}
	logger.Infof(ctx, "Model credential cleared by user: id=%s field=%s", id, field)
	return nil
}

// DeleteModel removes a model from the repository
func (s *modelService) DeleteModel(ctx context.Context, id string) error {
	logger.Info(ctx, "Start deleting model")
	logger.Infof(ctx, "Deleting model ID: %s", id)

	// Check if the model is builtin - builtin models cannot be deleted
	existingModel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": id,
		})
		return err
	}
	if existingModel == nil {
		return ErrModelNotFound
	}
	if existingModel.IsBuiltin {
		logger.Warnf(ctx, "Attempted to delete builtin model: %s", id)
		return apperrors.NewBadRequestError("builtin models cannot be deleted")
	}

	kbCount, err := s.kbRepo.CountByModelID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": id,
		})
		return err
	}
	agentCount, err := s.agentRepo.CountByModelID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": id,
		})
		return err
	}
	if kbCount > 0 || agentCount > 0 {
		logger.Warnf(ctx, "Model %s is in use: kb=%d agent=%d", id, kbCount, agentCount)
		return apperrors.NewBadRequestError(formatModelInUseMessage(kbCount, agentCount))
	}

	// Delete model from repository
	err = s.repo.Delete(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id})
		return err
	}

	logger.Infof(ctx, "Model deleted successfully: %s", id)
	return nil
}

// GetEmbeddingModel retrieves and initializes an embedding model instance
// Takes a model ID and returns an Embedder interface implementation
func (s *modelService) GetEmbeddingModel(ctx context.Context, modelId string) (embedding.Embedder, error) {
	// Get the model details
	model, err := s.GetModelByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": modelId,
		})
		return nil, err
	}

	logger.Infof(ctx, "Getting embedding model: %s, source: %s", model.Name, model.Source)

	embedder, err := embedding.NewEmbedder(embedding.ConfigFromModel(model), s.pooler)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	logger.Info(ctx, "Embedding model initialized successfully")
	return embedder, nil
}

// GetRerankModel retrieves and initializes a reranking model instance
// Takes a model ID and returns a Reranker interface implementation
func (s *modelService) GetRerankModel(ctx context.Context, modelId string) (rerank.Reranker, error) {
	// Get the model details
	model, err := s.GetModelByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": modelId,
		})
		return nil, err
	}

	logger.Infof(ctx, "Getting rerank model: %s, source: %s", model.Name, model.Source)

	appSecret := s.resolveModelAppSecret(&model.Parameters)
	reranker, err := rerank.NewReranker(rerank.ConfigFromModel(model, appSecret))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	logger.Info(ctx, "Rerank model initialized successfully")
	return reranker, nil
}

// GetChatModel retrieves and initializes a chat model instance
// Takes a model ID and returns a Chat interface implementation
func (s *modelService) GetChatModel(ctx context.Context, modelId string) (chat.Chat, error) {
	// Check if model ID is empty
	if modelId == "" {
		logger.Error(ctx, "Model ID is empty")
		return nil, errors.New("model ID cannot be empty")
	}

	// Get the model directly from repository to avoid status checks
	model, err := s.repo.GetByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": modelId})
		return nil, err
	}

	if model == nil {
		logger.Error(ctx, "Chat model not found")
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Getting chat model: %s, source: %s", model.Name, model.Source)

	chatModel, err := chat.NewChat(chat.ConfigFromModel(model))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	return chatModel, nil
}

// GetVLMModel retrieves and initializes a vision language model instance.
func (s *modelService) GetVLMModel(ctx context.Context, modelId string) (vlm.VLM, error) {
	if modelId == "" {
		return nil, errors.New("model ID cannot be empty")
	}

	model, err := s.repo.GetByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": modelId})
		return nil, err
	}

	if model == nil {
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Getting VLM model: %s, source: %s", model.Name, model.Source)

	vlmModel, err := vlm.NewVLM(vlm.ConfigFromModel(model))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	return vlmModel, nil
}

// Note: default model selection logic has been removed; models no longer
// maintain a per-type default flag at the service layer.

// GetASRModel retrieves and initializes an automatic speech recognition model instance.
func (s *modelService) GetASRModel(ctx context.Context, modelId string) (asr.ASR, error) {
	if modelId == "" {
		return nil, errors.New("model ID cannot be empty")
	}

	model, err := s.repo.GetByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": modelId})
		return nil, err
	}

	if model == nil {
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Getting ASR model: %s, source: %s", model.Name, model.Source)

	sttModel, err := asr.NewASR(asr.ConfigFromModel(model))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	return sttModel, nil
}

func formatModelInUseMessage(kbCount, agentCount int64) string {
	switch {
	case kbCount > 0 && agentCount > 0:
		return fmt.Sprintf(
			"model is used by %d knowledge base(s) and %d agent(s); "+
				"reconfigure or remove those references before deleting",
			kbCount, agentCount,
		)
	case kbCount > 0:
		return fmt.Sprintf(
			"model is used by %d knowledge base(s); "+
				"reconfigure or remove those references before deleting",
			kbCount,
		)
	default:
		return fmt.Sprintf(
			"model is used by %d agent(s); "+
				"reconfigure or remove those references before deleting",
			agentCount,
		)
	}
}
