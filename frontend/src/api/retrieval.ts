import { get, put } from '@/utils/request'

// RetrievalConfig represents the global retrieval/search configuration for a knowledgeDomain.
// Shared by knowledge search and message search.
export interface RetrievalConfig {
  embedding_top_k: number
  vector_threshold: number
  keyword_threshold: number
  rerank_top_k: number
  rerank_threshold: number
  rerank_model_id: string
}

// Get knowledgeDomain retrieval config via KV API
export function getKnowledgeDomainRetrievalConfig() {
  return get('/api/v1/system/runtime-config/retrieval-config')
}

// Update knowledgeDomain retrieval config via KV API
export function updateKnowledgeDomainRetrievalConfig(config: RetrievalConfig) {
  return put('/api/v1/system/runtime-config/retrieval-config', config)
}
