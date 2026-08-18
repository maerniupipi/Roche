<template>
  <div v-if="visible" ref="rootElement" class="rag-pipeline-progress">
    <div v-if="!showCollapsedRoot" class="tree-children">
      <div v-for="(step, index) in steps" :key="step.id" class="tree-child"
        :class="{ 'tree-child-last': !showDoneRow && !showThinkingStep && index === steps.length - 1, }">
        <div class="tree-branch" />
        <div class="tree-child-content">
          <div class="tool-event">
            <div class="action-card">
              <div class="action-header no-results">
                <div class="action-title">
                  <t-icon class="action-title-icon" :name="step.iconName" />
                  <span class="action-name" :class="{ 'is-running': step.pending }">{{ step.title }}</span>
                </div>
              </div>
              <div v-if="step.summaryHtml" class="search-results-summary-fixed">
                <div class="results-summary-text" v-html="step.summaryHtml" />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="showThinkingStep" class="tree-child rag-thinking-step" :class="{ 'tree-child-last': !showDoneRow }">
        <div class="tree-branch" />
        <div class="tree-child-content">
          <div class="tool-event">
            <div class="action-card" :class="{ 'action-pending': thinkingPending }">
              <div class="action-header" :class="{ 'no-results': !thinkingContent }" @click="toggleThinking">
                <div class="action-title">
                  <t-icon class="action-title-icon" name="lightbulb" />
                  <span class="action-name">{{ t('agent.think') }}</span>
                </div>
              </div>
              <div v-if="thinkingPending && !thinkingContent" class="thinking-loading">
                <div class="loading-typing">
                  <span />
                  <span />
                  <span />
                </div>
              </div>
              <div v-else-if="thinkingContent && thinkingExpanded" class="thinking-detail-content">
                {{ thinkingContent }}
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="showDoneRow" class="tree-child agent-step-done tree-child-last">
        <div class="tree-branch" />
        <div class="tree-child-content">
          <div class="tool-event">
            <div class="action-card">
              <div class="action-header no-results">
                <div class="action-title">
                  <t-icon class="action-title-icon" name="check-circle" />
                  <span class="action-name">{{ t('common.finish') }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="tree-container" :class="{ 'streaming-loading-node': showPrePipelineWait }">
      <div class="tool-event">
        <div class="action-card tree-root">
          <div class="tree-root-toolbar">
            <button type="button" class="tree-root-expand" :aria-expanded="showExpandedTimeline"
              :aria-label="toolbarAriaLabel" @click="toggleExpanded">
              <span class="tree-root-status">{{ toolbarStatusText }}</span>
              <span v-if="referenceSummaryText" class="tree-root-reference"> {{ referenceSummaryText }}</span>
              <span v-if="toolbarDurationText" class="tree-root-duration">{{ toolbarDurationText }}</span>
              <t-icon class="tree-root-expand__icon" :name="showExpandedTimeline ? 'chevron-down' : 'chevron-right'" />
            </button>
          </div>
        </div>
      </div>

      <div v-if="showExpandedTimeline" class="tree-children tree-children-expanded">
        <div v-for="(step, index) in steps" :key="step.id" class="tree-child"
          :class="{ 'tree-child-last': index === steps.length - 1 && !showDoneRow && !showThinkingStep }">
          <div class="tree-branch" />
          <div class="tree-child-content">
            <div class="tool-event">
              <div class="action-card">
                <div class="action-header no-results">
                  <div class="action-title">
                    <t-icon class="action-title-icon" :name="step.iconName" />
                    <span class="action-name" :class="{ 'is-running': step.pending }">{{ step.title }}</span>
                  </div>
                </div>
                <div v-if="step.summaryHtml" class="search-results-summary-fixed">
                  <div class="results-summary-text" v-html="step.summaryHtml" />
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="showThinkingStep" class="tree-child rag-thinking-step" :class="{ 'tree-child-last': !showDoneRow }">
          <div class="tree-branch" />
          <div class="tree-child-content">
            <div class="tool-event">
              <div class="action-card" :class="{ 'action-pending': thinkingPending }">
                <div class="action-header" :class="{ 'no-results': !thinkingContent }" @click="toggleThinking">
                  <div class="action-title">
                    <t-icon class="action-title-icon" name="lightbulb" />
                    <span class="action-name">{{ t('agent.think') }}</span>
                  </div>
                </div>
                <div v-if="thinkingPending && !thinkingContent" class="thinking-loading">
                  <div class="loading-typing">
                    <span />
                    <span />
                    <span />
                  </div>
                </div>
                <div v-else-if="thinkingContent && thinkingExpanded" class="thinking-detail-content">
                  {{ thinkingContent }}
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="showDoneRow" class="tree-child agent-step-done tree-child-last">
          <div class="tree-branch" />
          <div class="tree-child-content">
            <div class="tool-event">
              <div class="action-card">
                <div class="action-header no-results">
                  <div class="action-title">
                    <t-icon class="action-title-icon" name="check-circle" />
                    <span class="action-name">{{ t('common.finish') }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAgentToolIconName } from '@/utils/agent-tool-icons'
import {
  getKnowledgeSearchSummaryHtml,
  getRagPipelineStepTitle,
} from '@/utils/agent-tool-display'
import { RAG_PIPELINE_TOOL_NAMES } from '@/utils/rag-pipeline-history'
import { useChatReferencesDrawer } from '@/composables/useChatReferencesDrawer'
import { buildReferenceSections } from '@/utils/referenceSources'

const props = defineProps<{
  session?: {
    id?: string | number
    agentEventStream?: Array<Record<string, unknown>>
    content?: string
    knowledge_references?: Array<{ chunk_type?: string; knowledge_id?: string; knowledge_title?: string }>
    is_completed?: boolean
    agent_duration_ms?: number
  }
}>()

const { t } = useI18n()
const referencesDrawer = useChatReferencesDrawer()
const userExpanded = ref(false)
const thinkingExpanded = ref(true)
const rootElement = ref<HTMLElement | null>(null)

const thinkingContent = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return ''
  return stream
    .filter((event) => event.type === 'thinking' && !event.stage)
    .map((event) => String(event.content || ''))
    .join('')
})

const hasThinking = computed(() => thinkingContent.value.trim().length > 0)

const hasThinkingEvent = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return false
  return stream.some((event) => event.type === 'thinking' && !event.stage)
})

const hasAnswer = computed(() => {
  const sessionContent = props.session?.content
  if (typeof sessionContent === 'string' && sessionContent.trim().length > 0) return true

  const stream = props.session?.agentEventStream
  if (!stream?.length) return false
  return stream.some((event) => {
    if (event.type !== 'answer' || event.superseded) return false
    const content = event.content
    return typeof content === 'string' && content.trim().length > 0
  })
})

const hasReferences = computed(
  () => (props.session?.knowledge_references?.length ?? 0) > 0,
)

const referenceSections = computed(() => buildReferenceSections(props.session?.knowledge_references))

const steps = computed(() => {
  const stream = props.session?.agentEventStream
  if (!stream?.length) return []

  type Step = {
    id: string
    pending: boolean
    iconName: string
    title: string
    summaryHtml: string
    canOpenReferences: boolean
  }

  function buildStepFromEvent(event: Record<string, unknown>): Step | null {
    // 新接口（2026-08-15 起）：question_understood / knowledge_retrieved 独立节点。
    // 这两个事件均为"一次性 done"节点，没有 pending 状态。
    if (event.type === 'question_understood') {
      return {
        id: `question-understood-${(event.timestamp as number) || 0}`,
        pending: false,
        iconName: getAgentToolIconName('query_understand'),
        title: t('agentStream.toolStatus.queryUnderstandDone'),
        summaryHtml: '',
        canOpenReferences: false,
      }
    }
    if (event.type === 'knowledge_retrieved') {
      const resultCount =
        typeof event.result_count === 'number' ? event.result_count : 0
      const toolData: Record<string, unknown> | null =
        resultCount > 0
          ? { count: resultCount, results: Array(resultCount).fill({}) }
          : null
      return {
        id: `knowledge-retrieved-${(event.timestamp as number) || 0}`,
        pending: false,
        iconName: getAgentToolIconName('knowledge_search'),
        title: t('agentStream.toolStatus.searchKb'),
        summaryHtml: toolData ? getKnowledgeSearchSummaryHtml(t, toolData) : '',
        canOpenReferences: hasReferences.value,
      }
    }

    // 历史 / 旧接口：tool_call + tool_name。
    if (event.type !== 'tool_call') return null
    const toolNameRaw = event.tool_name
    if (typeof toolNameRaw !== 'string') return null
    if (!RAG_PIPELINE_TOOL_NAMES.has(toolNameRaw)) return null

    const toolName = toolNameRaw
    const pending = event.pending === true
    const toolData =
      event.tool_data && typeof event.tool_data === 'object'
        ? (event.tool_data as Record<string, unknown>)
        : null

    const isSearchTool = toolName === 'knowledge_search' || toolName === 'search_knowledge'
    const summaryHtml =
      !pending && isSearchTool && toolData
        ? getKnowledgeSearchSummaryHtml(t, toolData)
        : ''
    const canOpenReferences = !pending && isSearchTool && hasReferences.value

    return {
      id: String(event.tool_call_id || `${toolName}-${event.timestamp || 0}`),
      pending,
      iconName: getAgentToolIconName(toolName),
      title: getRagPipelineStepTitle(t, {
        tool_name: toolName,
        pending,
        success: event.success as boolean | undefined,
        arguments: event.arguments,
        tool_data: toolData,
      }),
      summaryHtml,
      canOpenReferences,
    }
  }

  return stream
    .map(buildStepFromEvent)
    .filter((step): step is Step => step !== null)
})

const allStepsDone = computed(
  () => steps.value.length > 0 && steps.value.every((step) => !step.pending),
)

const showPrePipelineWait = computed(() => {
  // history resume 场景：步骤还在反序列化重建阶段，不要先闪烁"正在理解..."等待文案，
  // 否则刷新页面会先看到一段错误的占位文字，再被真实步骤覆盖。
  // __historyWasInFlight 是 useChatStreamHandler 内部局部标记，未在 props 类型中声明，
  // 这里用 as any 兜住读取，运行时由 handleMsgList 写入。
  if ((props.session as any)?.__historyWasInFlight) return false
  if (hasAnswer.value || props.session?.is_completed || steps.value.length > 0 || hasThinking.value) {
    return false
  }
  return true
})

// Show the collapsed toolbar whenever there is anything to summarize — including in-progress
// streaming — so the user can see the live pipeline status (understanding → searching →
// thinking → answering → completed) instead of only after the turn finishes. The
// `showPrePipelineWait` branch is also folded in so the user sees the "正在理解问题..."
// status text the moment a session is created, instead of waiting for the first SSE
// pipeline node to arrive.
const showCollapsedRoot = computed(
  () =>
    steps.value.length > 0 ||
    hasThinking.value ||
    hasAnswer.value ||
    Boolean(props.session?.is_completed) ||
    showPrePipelineWait.value,
)

const showExpandedTimeline = computed(() => {
  if (!showCollapsedRoot.value) return true
  return userExpanded.value
})

const showDoneRow = computed(() => {
  const turnDone = hasDoneAnswer.value || Boolean(props.session?.is_completed)
  if (!turnDone) return false
  if (steps.value.length > 0 && !allStepsDone.value) return false
  return true
})

// Only show the thinking row once the backend actually streams thinking events.
// Do not pre-empt during the model phase — that flashes "思考" even when thinking is disabled.
const showThinkingStep = computed(() => hasThinkingEvent.value)

const thinkingPending = computed(
  () =>
    showThinkingStep.value &&
    !hasThinking.value &&
    !hasAnswer.value &&
    !props.session?.is_completed,
)

const isThinkingStreaming = computed(
  () =>
    showThinkingStep.value &&
    thinkingExpanded.value &&
    !hasAnswer.value &&
    !props.session?.is_completed,
)

const visible = computed(
  () =>
    steps.value.length > 0 ||
    showPrePipelineWait.value ||
    showThinkingStep.value ||
    hasAnswer.value ||
    Boolean(props.session?.is_completed),
)

// Pipeline status state machine: 5 stages that drive the collapsed toolbar status text.
// Precedence (highest first): completed > answering > thinking > searching > understanding.
type PipelineStatus =
  | 'understanding'
  | 'searching'
  | 'thinking'
  | 'answering'
  | 'completed'

const hasQuestionUnderstood = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return false
  return stream.some((event) => event.type === 'question_understood')
})

const hasKnowledgeRetrieved = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return false
  return stream.some((event) => event.type === 'knowledge_retrieved')
})

const hasAnswerStarted = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return false
  return stream.some((event) => {
    if (event.type !== 'answer' || event.superseded) return false
    const content = event.content
    return typeof content === 'string' && content.trim().length > 0
  })
})

// 「回答真正完成」的判定：answer 事件 done=true 才算完成（不含 superseded 的前置撤回）。
// hasAnswerStarted 只要求 answer 流式已输出内容，done=true 是流式自然结束的信号。
const hasDoneAnswer = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return false
  return stream.some((event) => {
    if (event.type !== 'answer' || event.superseded) return false
    return event.done === true
  })
})

const turnComplete = computed(
  () => Boolean(props.session?.is_completed) || hasDoneAnswer.value,
)

const turnDurationMs = computed(() => {
  const stream = props.session?.agentEventStream
  if (!Array.isArray(stream)) return 0
  const complete = stream.find((event) => event.type === 'agent_complete')
  if (!complete) return 0
  const raw = Number(complete.total_duration_ms)
  return Number.isFinite(raw) && raw > 0 ? raw : 0
})

// 与 AgentStreamDisplay.vue#L1911 保持一致：先把 ms 格式化为 ms/s/m Xs，再传给 i18n 模板。
function formatDuration(ms?: number): string {
  if (!ms) return '0s'
  if (ms < 1000) return `${ms}ms`
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  return `${minutes}m ${remainingSeconds}s`
}

const pipelineStatus = computed<PipelineStatus>(() => {
  if (turnComplete.value) return 'completed'
  if (hasAnswerStarted.value) return 'answering'
  if (hasThinking.value) return 'thinking'
  if (
    hasKnowledgeRetrieved.value ||
    hasQuestionUnderstood.value ||
    steps.value.length > 0
  ) {
    return 'searching'
  }
  return 'understanding'
})

const toolbarStatusText = computed(() => {
  switch (pipelineStatus.value) {
    case 'understanding':
      return t('agentStream.ragPipeline.queryUnderstanding')
    case 'searching':
      return t('agentStream.ragPipeline.searchingKnowledge')
    case 'thinking':
      return t('agentStream.ragPipeline.thinking')
    case 'answering':
      return t('agentStream.ragPipeline.answering')
    case 'completed':
      return t('agentStream.ragPipeline.completedWithDuration')
  }
})

// 仅在 completed 状态下返回「耗时 Xs / in Xs」；其他状态下为空字符串，配合模板 v-if 隐藏。
const toolbarDurationText = computed(() => {
  if (pipelineStatus.value !== 'completed') return ''
  return t('agentStream.ragPipeline.completedDuration', {
    duration: formatDuration(turnDurationMs.value),
  })
})

// aria-label 需要同时包含状态文字和耗时，否则屏幕阅读器读不完整。
const toolbarAriaLabel = computed(() =>
  [toolbarStatusText.value, toolbarDurationText.value].filter(Boolean).join(' ')
)

const referenceSummaryText = computed(() => {
  const docCount = referenceSections.value.find((section) => section.id === 'documents')?.items.length ?? 0

  if (docCount > 0) {
    return t('chat.referencesDocCount', { count: docCount })
  }

  return ''
})

function toggleReferencesDrawer() {
  const refs = props.session?.knowledge_references
  if (!referencesDrawer || !refs?.length) return
  referencesDrawer.toggle({
    references: refs,
    highlight: null,
    messageId: props.session?.id ? String(props.session.id) : '',
    sourceKey: `rag:${props.session?.id || refs.map((item) => item.knowledge_id || item.knowledge_title).join('|')}`,
  })
}

function handleStepClick(step: { canOpenReferences?: boolean }) {
  if (!step.canOpenReferences) return
  toggleReferencesDrawer()
}

function toggleExpanded() {
  userExpanded.value = !userExpanded.value
}

function toggleThinking() {
  if (!showThinkingStep.value || !thinkingContent.value) return
  thinkingExpanded.value = !thinkingExpanded.value
}

function scrollThinkingDetailToBottom() {
  nextTick(() => {
    if (!rootElement.value) return
    rootElement.value.querySelectorAll('.thinking-detail-content').forEach((el) => {
      const htmlEl = el as HTMLElement
      htmlEl.scrollTop = htmlEl.scrollHeight
    })
  })
}

watch(thinkingPending, (pending) => {
  if (pending) {
    thinkingExpanded.value = true
  }
})

watch(hasAnswer, (answered) => {
  if (answered && hasThinking.value) {
    thinkingExpanded.value = false
  }
})

watch(thinkingContent, () => {
  if (!isThinkingStreaming.value) return
  scrollThinkingDetailToBottom()
})

watch(thinkingExpanded, (expanded) => {
  if (!expanded || !isThinkingStreaming.value) return
  scrollThinkingDetailToBottom()
})
</script>

<style scoped lang="less">
@import '@/components/css/chat-timeline-loading.less';

.rag-pipeline-progress {
  --agent-step-text-size: 14px;
  --agent-step-summary-size: 13px;
  --agent-step-line-color: color-mix(in srgb, var(--td-text-color-primary) 16%, transparent);
  --agent-step-icon-color: var(--td-text-color-placeholder);

  margin: 0;
}

.tree-container {
  margin: 0 0 8px;
  position: relative;
}

.tree-root {
  margin-bottom: 0;

  .tree-root-toolbar {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    width: 100%;
    min-width: 0;
  }

  .tree-root-expand {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    padding: 0;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--td-text-color-secondary);
    font-size: 14px;
    line-height: 22px;
    cursor: pointer;
    flex: 0 1 auto;
    min-width: 0;
    max-width: 100%;

    &:hover {
      background: transparent;
      color: var(--td-text-color-primary);
    }

    span:not(:first-child) {
      display: inline-flex;
      align-items: center;
      gap: 6px;

      &::before {
        content: '';
        width: 3px;
        height: 3px;
        border-radius: 50%;
        background: currentColor;
        opacity: 0.65;
        flex-shrink: 0;
      }
    }
  }

  .tree-root-status,
  .tree-root-reference {
    flex: 0 1 auto;
    min-width: 0;
    white-space: nowrap;
  }

  .tree-root-status {
    font-variant-numeric: tabular-nums;
  }

  .tree-root-expand__icon {
    flex-shrink: 0;
    font-size: 14px;
    color: currentColor;
  }

}

.tree-children {
  position: relative;
  padding-left: 0;
  margin-top: 0;
  margin-left: 10px;
}

.tree-children-expanded {
  margin-top: 14px;
}

.tree-child {
  position: relative;
  padding-left: 42px;
  padding-bottom: 0;
  margin-bottom: 18px;

  &::before {
    content: '';
    position: absolute;
    left: 9px;
    top: 22px;
    bottom: -18px;
    width: 0;
    border-left: 1px solid var(--agent-step-line-color);
  }

  .tree-branch {
    display: none;
  }

  &.tree-child-last {
    margin-bottom: 0;

    &::before {
      content: none;
    }
  }
}

.tool-event {
  .action-card {
    position: relative;
    background: transparent;
    border: 0;
    box-shadow: none;

    &.has-reference-trigger {
      cursor: pointer;

      &:hover {

        .action-name,
        .results-summary-text {
          color: var(--td-text-color-primary);
        }
      }
    }
  }

  .action-header {
    display: flex;
    align-items: center;
    min-height: 24px;
    padding: 0;
    cursor: pointer;
    user-select: none;

    &.no-results {
      cursor: default;
    }
  }

  .action-title {
    display: flex;
    align-items: center;
    gap: 12px;
    position: relative;
    flex: 0 1 auto;
    min-width: 0;

    .action-show-icon {
      flex-shrink: 0;
      margin-left: 2px;
    }
  }

  .action-title-icon {
    position: absolute;
    left: -42px;
    top: 3px;
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    color: var(--agent-step-icon-color);
  }

  .action-name {
    font-size: var(--agent-step-text-size);
    line-height: 1.55;
    font-weight: 400;
    color: var(--td-text-color-secondary);
    word-break: break-word;
    max-width: min(680px, 100%);
  }
}

.search-results-summary-fixed {
  padding: 2px 0 0 0;

  .results-summary-text {
    font-size: var(--agent-step-summary-size);
    font-weight: 400;
    color: var(--td-text-color-secondary);
    line-height: 1.5;

    :deep(strong) {
      color: var(--td-text-color-secondary);
      font-weight: 500;
    }
  }
}

.rag-thinking-step {
  .thinking-loading {
    padding: 4px 0 0;
  }

  .thinking-detail-content {
    margin-top: 4px;
    padding: 0;
    font-size: var(--agent-step-summary-size);
    font-weight: 400;
    color: var(--td-text-color-placeholder);
    line-height: 1.55;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
  }

  .action-pending .action-name {
    color: var(--td-text-color-secondary);
  }
}

@media (max-width: 640px) {
  .tree-root {
    .tree-root-toolbar {
      gap: 8px;
    }

    .tree-root-expand {
      max-width: 100%;
    }
  }
}
</style>
