import { listModels, type ModelConfig } from '@/api/model'

export interface PlatformModelReadiness {
  chatCount: number
  embeddingCount: number
  hasChat: boolean
  hasEmbedding: boolean
  isReadyForDocumentKb: boolean
  isReadyForAgent: boolean
}

export function evaluatePlatformModelReadiness(
  models: ModelConfig[],
): PlatformModelReadiness {
  const chatCount = models.filter((model) => model.type === 'KnowledgeQA').length
  const embeddingCount = models.filter((model) => model.type === 'Embedding').length
  const hasChat = chatCount > 0
  const hasEmbedding = embeddingCount > 0
  return {
    chatCount,
    embeddingCount,
    hasChat,
    hasEmbedding,
    isReadyForDocumentKb: hasChat && hasEmbedding,
    isReadyForAgent: hasChat,
  }
}

export async function fetchPlatformModelReadiness(): Promise<PlatformModelReadiness> {
  try {
    return evaluatePlatformModelReadiness((await listModels()) || [])
  } catch {
    return evaluatePlatformModelReadiness([])
  }
}
