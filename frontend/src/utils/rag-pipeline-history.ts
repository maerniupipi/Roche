export const RAG_PIPELINE_TOOL_NAMES = new Set(['query_understand', 'knowledge_search'])

function readProgressResultCount(
  agentSteps: Array<Record<string, unknown>> | undefined,
): number {
  if (!Array.isArray(agentSteps)) return 0
  for (const step of agentSteps) {
    if (!step || typeof step !== 'object') continue
    if ((step as Record<string, unknown>).progress_response_type !== 'knowledge_retrieved') continue
    const cnt = (step as Record<string, unknown>).progress_result_count
    if (typeof cnt === 'number' && cnt > 0) return cnt
  }
  return 0
}

type RagHistoryReference = {
  chunk_type?: string
  knowledge_id?: string
  knowledge_title?: string
}

type RagHistoryMessage = {
  knowledge_references?: RagHistoryReference[]
  agentEventStream?: Array<Record<string, unknown>>
  agent_steps?: Array<Record<string, unknown>>
  agent_duration_ms?: number
}

export function hasRagPipelineToolEvents(stream: Array<Record<string, unknown>> | undefined): boolean {
  if (!stream?.length) return false
  return stream.some((event) => {
    return (
      event.type === 'tool_call' &&
      typeof event.tool_name === 'string' &&
      RAG_PIPELINE_TOOL_NAMES.has(event.tool_name)
    )
  })
}

export function synthesizeRagPipelineToolEvents(
  item: RagHistoryMessage,
): Array<Record<string, unknown>> {
  const refs = item.knowledge_references ?? []
  const kbCounts: Record<string, number> = {}
  let docCount = 0

  for (const ref of refs) {
    docCount++
    const key = ref.knowledge_id || ref.knowledge_title || 'document'
    kbCounts[key] = (kbCounts[key] || 0) + 1
  }
  const progressCount = readProgressResultCount(item.agent_steps)

  const events: Array<Record<string, unknown>> = [
    {
      type: 'tool_call',
      tool_call_id: 'rag-history-query-understand',
      tool_name: 'query_understand',
      pending: false,
      success: true,
    },
    {
      type: 'tool_call',
      tool_call_id: 'rag-history-knowledge-search',
      tool_name: 'knowledge_search',
      pending: false,
      success: true,
      arguments: { search_source: 'knowledge' },
      tool_data: {
        count: progressCount > 0 ? progressCount : refs.length,
        doc_count: docCount,
        search_source: 'knowledge',
        kb_counts: kbCounts,
        results: refs,
      },
    },
  ]

  return events
}

export function ensureRagPipelineHistoryStream(item: RagHistoryMessage & {
  content?: string
  is_completed?: boolean
  isAgentMode?: boolean
  hideContent?: boolean
}): void {
  if (!item.is_completed) return

  const stream = Array.isArray(item.agentEventStream)
    ? [...item.agentEventStream]
    : []

  if (hasRagPipelineToolEvents(stream)) return

  const hasRestorablePayload =
  Boolean(item.content?.trim()) ||
  Boolean(item.knowledge_references?.length)
  if (!hasRestorablePayload) return

  const synthesized = synthesizeRagPipelineToolEvents(item)
  const preserved = stream.filter((event) => {
    return !(
      event.type === 'tool_call' &&
      typeof event.tool_name === 'string' &&
      RAG_PIPELINE_TOOL_NAMES.has(event.tool_name)
    )
  })

  item.agentEventStream = [...synthesized, ...preserved]

  const hasAnswer = preserved.some((event) => {
    if (event.type !== 'answer' || event.superseded) return false
    const content = event.content
    return typeof content === 'string' && content.trim().length > 0
  })

  if (!hasAnswer && item.content?.trim()) {
    item.agentEventStream.push({
      type: 'answer',
      content: item.content,
      done: true,
    })
  }

  const hasAgentComplete = preserved.some((event) => event.type === 'agent_complete')
  if (!hasAgentComplete) {
    item.agentEventStream.push({
      type: 'agent_complete',
      total_duration_ms: Number(item.agent_duration_ms) || 0,
      total_steps: 1,
    })
  }

  item.isAgentMode = true
  item.hideContent = true
}
