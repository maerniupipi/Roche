package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"golang.org/x/sync/errgroup"
	"roche.local/knowledge-agent-platform/internal/application/service/retriever"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// collectImageURLs extracts unique provider:// image URLs from image_info JSON strings.
func collectImageURLs(ctx context.Context, imageInfos []string) []string {
	seen := make(map[string]struct{})
	var urls []string
	for _, info := range imageInfos {
		if info == "" {
			continue
		}
		var images []*types.ImageInfo
		if err := json.Unmarshal([]byte(info), &images); err != nil {
			logger.Warnf(ctx, "Failed to parse image_info JSON: %v", err)
			continue
		}
		for _, img := range images {
			if img.URL != "" {
				if _, exists := seen[img.URL]; !exists {
					seen[img.URL] = struct{}{}
					urls = append(urls, img.URL)
				}
			}
		}
	}
	return urls
}

// deleteExtractedImages deletes all extracted image files from storage.
// Standalone function — callable from both knowledgeService and knowledgeBaseService.
// Errors are logged but do not fail the overall deletion.
func deleteExtractedImages(ctx context.Context, fileSvc interfaces.FileService, imageURLs []string) {
	if len(imageURLs) == 0 {
		return
	}
	logger.Infof(ctx, "Deleting %d extracted images", len(imageURLs))
	for _, url := range imageURLs {
		if err := fileSvc.DeleteFile(ctx, url); err != nil {
			logger.Errorf(ctx, "Failed to delete extracted image %s: %v", url, err)
		}
	}
}

// DeleteKnowledge deletes a knowledge entry and all related resources
func (s *knowledgeService) DeleteKnowledge(ctx context.Context, id string) error {
	// Get the knowledge entry
	knowledge, err := s.repo.GetKnowledgeByID(ctx, ctx.Value(types.KnowledgeDomainIDContextKey).(uint64), id)
	if err != nil {
		return err
	}

	// Mark as deleting first to prevent async task conflicts
	// This ensures that any running async tasks will detect the deletion and abort
	originalStatus := knowledge.ParseStatus
	knowledge.ParseStatus = types.ParseStatusDeleting
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge failed to mark as deleting")
		// Continue with deletion even if marking fails
	} else {
		logger.Infof(ctx, "Marked knowledge %s as deleting (previous status: %s)", id, originalStatus)
	}

	// Best-effort: purge any queued downstream tasks for this knowledge
	// (multimodal / post-process / question / summary / graph extract).
	// Worker checkpoints already drop them on the floor, but dequeuing
	// here avoids waking workers just to no-op when the parse was still
	// in flight at delete time. No-op in Lite mode and on completed rows
	// (no queued descendants anyway).
	if originalStatus == types.ParseStatusPending ||
		originalStatus == types.ParseStatusProcessing {
		s.dequeueKnowledgeTasks(ctx, id)
	}

	// Resolve file service for this KB before spawning goroutines
	kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	kbFileSvc := s.resolveFileService(ctx, kb)

	// Collect image URLs before chunks are deleted (ImageInfo references are lost after deletion)
	knowledgeDomainID := ctx.Value(types.KnowledgeDomainIDContextKey).(uint64)
	chunkImageInfos, err := s.chunkService.GetRepository().ListImageInfoByKnowledgeIDs(ctx, knowledgeDomainID, []string{id})
	if err != nil {
		logger.Errorf(ctx, "Failed to collect image URLs for cleanup: %v", err)
	}
	var imageInfoStrs []string
	for _, ci := range chunkImageInfos {
		imageInfoStrs = append(imageInfoStrs, ci.ImageInfo)
	}
	imageURLs := collectImageURLs(ctx, imageInfoStrs)

	wg := errgroup.Group{}
	// Delete knowledge embeddings from vector store.
	// Skip entirely when the knowledge has no embedding model (e.g. graph-only KB):
	// nothing was ever written to the vector store, so there is nothing to delete,
	// and GetEmbeddingModel would fail with "model ID cannot be empty".
	if strings.TrimSpace(knowledge.EmbeddingModelID) != "" {
		wg.Go(func() error {
			// kb was already loaded above for resolveFileService — reuse its
			// VectorStoreID for engine routing.
			var boundStoreID *string
			if kb != nil {
				boundStoreID = kb.VectorStoreID
			}
			retrieveEngine, err := retriever.CreateRetrieveEngineForKB(
				ctx,
				s.retrieveEngine,
				s.ownership,
				knowledgeDomainID,
				boundStoreID,
			)
			if err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
				return err
			}
			embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)
			if err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
				return err
			}
			if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), knowledge.Type); err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
				return err
			}
			return nil
		})
	} else {
		logger.Infof(ctx, "Knowledge %s has no embedding model, skipping vector store cleanup", knowledge.ID)
	}

	// Delete all chunks associated with this knowledge
	wg.Go(func() error {
		if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete chunks failed")
			return err
		}
		return nil
	})

	// Delete the physical file and extracted images if they exist
	wg.Go(func() error {
		if knowledge.FilePath != "" {
			if err := kbFileSvc.DeleteFile(ctx, knowledge.FilePath); err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete file failed")
			}
		}
		deleteExtractedImages(ctx, kbFileSvc, imageURLs)
		knowledgeDomainInfo := ctx.Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain)
		knowledgeDomainInfo.StorageUsed -= knowledge.StorageSize
		if err := s.knowledgeDomainRepo.AdjustStorageUsed(ctx, knowledgeDomainInfo.ID, -knowledge.StorageSize); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge update knowledgeDomain storage used failed")
		}
		return nil
	})

	// Delete the knowledge graph
	wg.Go(func() error {
		namespace := types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID}
		if err := s.graphEngine.DelGraph(ctx, []types.NameSpace{namespace}); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge graph failed")
			return err
		}
		return nil
	})

	if err = wg.Wait(); err != nil {
		return err
	}
	if err := s.repo.DeleteKnowledgeTagRelations(ctx, id); err != nil {
		logger.Warnf(ctx, "Failed to delete tag relations for knowledge %s: %v", id, err)
	}
	// Delete the knowledge entry itself from the database
	if err := s.repo.DeleteKnowledge(ctx, ctx.Value(types.KnowledgeDomainIDContextKey).(uint64), id); err != nil {
		return err
	}

	// Audit: knowledge document deletion
	s.businessAudit.RecordKnowledgeDeleted(ctx,
		knowledge.ID, knowledge.Title, knowledge.KnowledgeBaseID, false)

	return nil
}

func (s *knowledgeService) DeleteKnowledgeList(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	// 1. Get the knowledge entry
	knowledgeDomainInfo := ctx.Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain)
	knowledgeList, err := s.repo.GetKnowledgeBatch(ctx, knowledgeDomainInfo.ID, ids)
	if err != nil {
		return err
	}

	// Mark all as deleting first to prevent async task conflicts.
	// Remember which entries still had queued / in-flight downstream tasks
	// so we can dequeue them in one pass after marking.
	var inFlightIDs []string
	for _, knowledge := range knowledgeList {
		prev := knowledge.ParseStatus
		knowledge.ParseStatus = types.ParseStatusDeleting
		knowledge.UpdatedAt = time.Now()
		if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
			logger.GetLogger(ctx).WithField("error", err).WithField("knowledge_id", knowledge.ID).
				Errorf("DeleteKnowledgeList failed to mark as deleting")
			// Continue with deletion even if marking fails
		}
		if prev == types.ParseStatusPending || prev == types.ParseStatusProcessing {
			inFlightIDs = append(inFlightIDs, knowledge.ID)
		}
	}
	logger.Infof(ctx, "Marked %d knowledge entries as deleting", len(knowledgeList))

	// Best-effort dequeue of downstream tasks for in-flight entries.
	// See DeleteKnowledge for the rationale; loop is per-knowledge because
	// the inspector only filters by knowledge_id, not by ID set.
	for _, kid := range inFlightIDs {
		s.dequeueKnowledgeTasks(ctx, kid)
	}

	// Pre-resolve file services per KB so goroutines don't need DB access
	kbFileServices := make(map[string]interfaces.FileService)
	for _, knowledge := range knowledgeList {
		if _, ok := kbFileServices[knowledge.KnowledgeBaseID]; !ok {
			kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
			kbFileServices[knowledge.KnowledgeBaseID] = s.resolveFileService(ctx, kb)
		}
	}

	// Collect image URLs before chunks are deleted
	chunkImageInfos, err := s.chunkService.GetRepository().ListImageInfoByKnowledgeIDs(ctx, knowledgeDomainInfo.ID, ids)
	if err != nil {
		logger.Errorf(ctx, "Failed to collect image URLs for batch cleanup: %v", err)
	}
	knowledgeToKB := make(map[string]string)
	for _, k := range knowledgeList {
		knowledgeToKB[k.ID] = k.KnowledgeBaseID
	}
	kbImageInfos := make(map[string][]string) // kbID → []imageInfo JSON
	for _, ci := range chunkImageInfos {
		kbID := knowledgeToKB[ci.KnowledgeID]
		kbImageInfos[kbID] = append(kbImageInfos[kbID], ci.ImageInfo)
	}
	kbImageURLs := make(map[string][]string) // kbID → []imageURL (deduplicated)
	for kbID, infos := range kbImageInfos {
		kbImageURLs[kbID] = collectImageURLs(ctx, infos)
	}

	wg := errgroup.Group{}
	// 2. Delete knowledge embeddings from vector store
	wg.Go(func() error {
		knowledgeDomainID := types.MustKnowledgeDomainIDFromContext(ctx)
		// Batch cleanup spans multiple KBs that may be bound to different
		// VectorStores; routing this batch through knowledgeDomain effective engines
		// keeps the legacy behavior intact.
		// TODO: fan out the batch per-store using each KB's own
		// VectorStoreID so cleanup hits the right backend for bound KBs.
		retrieveEngine, err := retriever.CreateRetrieveEngineForKB(
			ctx, s.retrieveEngine, s.ownership, knowledgeDomainID, nil)
		if err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
			return err
		}
		// Group by EmbeddingModelID and Type
		type groupKey struct {
			EmbeddingModelID string
			Type             string
		}
		group := map[groupKey][]string{}
		for _, knowledge := range knowledgeList {
			key := groupKey{EmbeddingModelID: knowledge.EmbeddingModelID, Type: knowledge.Type}
			group[key] = append(group[key], knowledge.ID)
		}
		for key, knowledgeIDs := range group {
			// Knowledge without an embedding model never had embeddings written to the vector store,
			// and its EmbeddingModelID is intentionally empty. Skip the whole group
			// to avoid the spurious "model ID cannot be empty" failure.
			if strings.TrimSpace(key.EmbeddingModelID) == "" {
				logger.Infof(ctx, "Skipping vector store cleanup for %d knowledge entries without embedding model", len(knowledgeIDs))
				continue
			}
			embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, key.EmbeddingModelID)
			if err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge get embedding model failed")
				return err
			}
			if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, knowledgeIDs, embeddingModel.GetDimensions(), key.Type); err != nil {
				logger.GetLogger(ctx).
					WithField("error", err).
					Errorf("DeleteKnowledge delete knowledge embedding failed")
				return err
			}
		}
		return nil
	})

	// 3. Delete all chunks associated with this knowledge
	wg.Go(func() error {
		if err := s.chunkService.DeleteByKnowledgeList(ctx, ids); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete chunks failed")
			return err
		}
		return nil
	})

	// 5. Delete the physical file and extracted images if they exist
	wg.Go(func() error {
		storageAdjust := int64(0)
		for _, knowledge := range knowledgeList {
			if knowledge.FilePath != "" {
				fSvc := kbFileServices[knowledge.KnowledgeBaseID]
				if err := fSvc.DeleteFile(ctx, knowledge.FilePath); err != nil {
					logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete file failed")
				}
			}
			storageAdjust -= knowledge.StorageSize
		}
		// Delete extracted images per KB
		for kbID, urls := range kbImageURLs {
			fSvc := kbFileServices[kbID]
			if fSvc == nil {
				logger.Warnf(ctx, "No file service for KB %s, skipping %d image deletions", kbID, len(urls))
				continue
			}
			deleteExtractedImages(ctx, fSvc, urls)
		}
		knowledgeDomainInfo.StorageUsed += storageAdjust
		if err := s.knowledgeDomainRepo.AdjustStorageUsed(ctx, knowledgeDomainInfo.ID, storageAdjust); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge update knowledgeDomain storage used failed")
		}
		return nil
	})

	// Delete the knowledge graph
	wg.Go(func() error {
		namespaces := []types.NameSpace{}
		for _, knowledge := range knowledgeList {
			namespaces = append(
				namespaces,
				types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID},
			)
		}
		if err := s.graphEngine.DelGraph(ctx, namespaces); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge graph failed")
			return err
		}
		return nil
	})

	if err = wg.Wait(); err != nil {
		return err
	}
	for _, knowledgeID := range ids {
		if err := s.repo.DeleteKnowledgeTagRelations(ctx, knowledgeID); err != nil {
			logger.Warnf(ctx, "Failed to delete tag relations for knowledge %s: %v", knowledgeID, err)
		}
	}
	// 6. Delete the knowledge entry itself from the database
	return s.repo.DeleteKnowledgeList(ctx, knowledgeDomainInfo.ID, ids)
}

func (s *knowledgeService) cleanupKnowledgeResources(ctx context.Context, knowledge *types.Knowledge) error {
	logger.GetLogger(ctx).Infof("Cleaning knowledge resources before manual update, knowledge ID: %s", knowledge.ID)

	var cleanupErr error

	if knowledge.ParseStatus == types.ManualKnowledgeStatusDraft && knowledge.StorageSize == 0 {
		// Draft without indexed data, skip cleanup.
		return nil
	}

	knowledgeDomainInfo := ctx.Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain)
	if knowledge.EmbeddingModelID != "" {
		// Load KB to discover its VectorStoreID binding. Falls back to knowledgeDomain
		// effective engines if the KB has no binding or the load fails.
		//
		// Silent fallback risk: if a bound KB fails to load here due to a
		// transient DB error, the cleanup will delete from env engines and
		// leave orphan vectors in the bound store. Warn so operators can spot it.
		var boundStoreID *string
		if kb, loadErr := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID); loadErr == nil && kb != nil {
			boundStoreID = kb.VectorStoreID
		} else if loadErr != nil {
			logger.GetLogger(ctx).WithField("error", loadErr).WithField("knowledge_base_id", knowledge.KnowledgeBaseID).
				Warnf("cleanupKnowledgeResources: failed to load KB for vector store resolution; falling back to knowledgeDomain effective engines")
		}
		retrieveEngine, err := retriever.CreateRetrieveEngineForKB(
			ctx, s.retrieveEngine, s.ownership, knowledgeDomainInfo.ID, boundStoreID)
		if err != nil {
			logger.GetLogger(ctx).WithField("error", err).Error("Failed to init retrieve engine during cleanup")
			cleanupErr = errors.Join(cleanupErr, err)
		} else {
			embeddingModel, modelErr := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)
			if modelErr != nil {
				logger.GetLogger(ctx).WithField("error", modelErr).Error("Failed to get embedding model during cleanup")
				cleanupErr = errors.Join(cleanupErr, modelErr)
			} else {
				if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), knowledge.Type); err != nil {
					logger.GetLogger(ctx).WithField("error", err).Error("Failed to delete manual knowledge index")
					cleanupErr = errors.Join(cleanupErr, err)
				}
			}
		}
	}

	// Collect image URLs before chunks are deleted
	kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	fileSvc := s.resolveFileService(ctx, kb)
	chunkImageInfos, imgErr := s.chunkService.GetRepository().ListImageInfoByKnowledgeIDs(ctx, knowledgeDomainInfo.ID, []string{knowledge.ID})
	if imgErr != nil {
		logger.GetLogger(ctx).WithField("error", imgErr).Error("Failed to collect image URLs for cleanup")
		cleanupErr = errors.Join(cleanupErr, imgErr)
	}
	var imageInfoStrs []string
	for _, ci := range chunkImageInfos {
		imageInfoStrs = append(imageInfoStrs, ci.ImageInfo)
	}
	imageURLs := collectImageURLs(ctx, imageInfoStrs)

	if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Error("Failed to delete manual knowledge chunks")
		cleanupErr = errors.Join(cleanupErr, err)
	}

	// Delete extracted images after chunks are deleted
	deleteExtractedImages(ctx, fileSvc, imageURLs)

	namespace := types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID}
	if err := s.graphEngine.DelGraph(ctx, []types.NameSpace{namespace}); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Error("Failed to delete manual knowledge graph data")
		cleanupErr = errors.Join(cleanupErr, err)
	}

	if knowledge.StorageSize > 0 {
		knowledgeDomainInfo.StorageUsed -= knowledge.StorageSize
		if knowledgeDomainInfo.StorageUsed < 0 {
			knowledgeDomainInfo.StorageUsed = 0
		}
		if err := s.knowledgeDomainRepo.AdjustStorageUsed(ctx, knowledgeDomainInfo.ID, -knowledge.StorageSize); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Error("Failed to adjust storage usage during manual cleanup")
			cleanupErr = errors.Join(cleanupErr, err)
		}
		knowledge.StorageSize = 0
	}

	return cleanupErr
}

// ProcessKnowledgeListDelete handles Asynq knowledge list delete tasks
func (s *knowledgeService) ProcessKnowledgeListDelete(ctx context.Context, t *asynq.Task) error {
	var payload types.KnowledgeListDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal knowledge list delete payload: %v", err)
		return err
	}

	logger.Infof(ctx, "Processing knowledge list delete task for %d knowledge items", len(payload.KnowledgeIDs))

	// Get knowledgeDomain info
	knowledgeDomain, err := s.knowledgeDomainRepo.GetKnowledgeDomainByID(ctx, payload.KnowledgeDomainID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledgeDomain %d: %v", payload.KnowledgeDomainID, err)
		return err
	}

	// Set context values
	ctx = context.WithValue(ctx, types.KnowledgeDomainIDContextKey, payload.KnowledgeDomainID)
	ctx = context.WithValue(ctx, types.KnowledgeDomainInfoContextKey, knowledgeDomain)

	// Delete knowledge list
	if err := s.DeleteKnowledgeList(ctx, payload.KnowledgeIDs); err != nil {
		logger.Errorf(ctx, "Failed to delete knowledge list: %v", err)
		return err
	}

	logger.Infof(ctx, "Successfully deleted %d knowledge items", len(payload.KnowledgeIDs))
	return nil
}
