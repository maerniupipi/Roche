<template>
  <div ref="rootElement" class="agent-stream-display" :class="{ 'is-rag-mode': ragMode }">

    <!-- Collapsed intermediate steps (tree root) -->
    <div v-if="shouldShowCollapsedSteps" class="tree-container">
      <div class="tool-event">
        <div class="action-card tree-root" @click="toggleIntermediateSteps">
          <div class="action-header">
            <div class="action-title">
              <span class="action-title-icon icon-mask" :style="maskIconStyle(agentIcon)" aria-hidden="true" />
              <span class="action-name tree-root-summary" v-html="intermediateStepsSummaryHtml"></span>
              <div class="action-show-icon">
                <t-icon :name="showIntermediateSteps ? 'chevron-down' : 'chevron-right'" />
              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- Tree children (intermediate steps) -->
      <div v-if="showIntermediateSteps" class="tree-children">
        <template v-for="(event, index) in visibleIntermediateEvents" :key="getEventKey(event, index)">
          <div v-if="event && event.type" class="tree-child"
            :class="{ 'tree-child-last': !isConversationDone && index === visibleIntermediateEvents.length - 1 }">
            <div class="tree-branch"></div>
            <div class="tree-child-content">
              <!-- Question Understood Event (一次性 done 节点) -->
              <div v-if="event.type === 'question_understood'" class="tool-event">
                <div class="action-card no-results">
                  <div class="action-header">
                    <div class="action-title">
                      <t-icon class="action-title-icon" name="check-circle" />
                      <span class="action-name">{{ t('agentStream.toolStatus.queryUnderstandDone') }}</span>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Knowledge Retrieved Event (一次性 done 节点) -->
              <div v-else-if="event.type === 'knowledge_retrieved'" class="tool-event">
                <div class="action-card no-results">
                  <div class="action-header">
                    <div class="action-title">
                      <t-icon class="action-title-icon" name="search" />
                      <span class="action-name">{{ t('agentStream.ragPipeline.searchDone') }}</span>
                      <span v-if="event.query" class="action-summary">「{{ event.query }}」</span>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Plan Task Change Event -->
              <div v-if="event.type === 'plan_task_change'" class="plan-task-change-event">
                <div class="plan-task-change-card">
                  <div class="plan-task-change-content">
                    <strong>{{ $t('agent.taskLabel') }}</strong> {{ event.task }}
                  </div>
                </div>
              </div>

              <!-- Thinking Event (streaming / merged). When a round's retracted
                   preamble was folded in, it becomes the card title and the
                   reasoning is the expandable body. -->
              <div v-if="event.type === 'thinking'" class="tool-event">
                <div class="action-card" :class="{ 'action-pending': isThinkingActive(event.event_id) }">
                  <div class="action-header" @click="toggleEvent(event.event_id)">
                    <div class="action-title">
                      <span class="action-title-icon icon-mask" :style="maskIconStyle(thinkingIcon)"
                        aria-hidden="true" />
                      <span v-if="event.title" class="action-name action-preamble-title">{{ event.title }}</span>
                      <span v-else-if="isEventExpanded(event.event_id)" class="action-name">{{ $t('agent.think')
                        }}</span>
                      <span v-else-if="getThinkingSummary(event)" class="action-summary">{{ getThinkingSummary(event)
                      }}</span>
                    </div>
                  </div>
                  <div v-if="event.content && isEventExpanded(event.event_id)" class="action-details">
                    <div class="thinking-detail-content markdown-content">
                      <div v-html="renderMarkdownContent(event.content)"></div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Thinking Tool Call -->
              <div v-else-if="event.type === 'tool_call' && event.tool_name === 'thinking'" class="tool-event">
                <div class="action-card"
                  :class="{ 'action-pending': event.pending || isThinkingActive(event.tool_call_id) }">
                  <div class="action-header" @click="toggleEvent(event.tool_call_id)">
                    <div class="action-title">
                      <span class="action-title-icon icon-mask" :style="maskIconStyle(thinkingIcon)"
                        aria-hidden="true" />
                      <span class="action-name">{{ $t('agent.think') }}</span>
                      <span v-if="event.tool_data?.thought_number" class="action-badge">{{
                        event.tool_data.thought_number }}/{{ event.tool_data.total_thoughts }}</span>
                      <span v-if="getThinkingSummary(event) && !isEventExpanded(event.tool_call_id)"
                        class="action-summary">{{ getThinkingSummary(event) }}</span>
                    </div>
                  </div>
                  <div v-if="event.tool_data?.thought && isEventExpanded(event.tool_call_id)" class="action-details">
                    <div class="thinking-detail-content markdown-content">
                      <div v-html="renderMarkdownContent(event.tool_data.thought)"></div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Tool Call Event (non-thinking) -->
              <div v-else-if="event.type === 'tool_call'" class="tool-event">
                <div class="action-card" :class="{
                  'action-pending': event.pending,
                  'action-error': event.success === false,
                  'reference-trigger': canOpenToolReferences(event)
                }" :role="canOpenToolReferences(event) ? 'button' : undefined"
                  :tabindex="canOpenToolReferences(event) ? 0 : undefined" @click="handleActionCardClick(event)"
                  @keydown.enter="handleActionCardClick(event)" @keydown.space.prevent="handleActionCardClick(event)">
                  <div class="action-header" @click.stop="handleActionHeaderClick(event)"
                    :class="{ 'no-results': !hasActionResult(event) }">
                    <div class="action-title">
                      <t-icon v-if="event.tool_name" class="action-title-icon"
                        :name="getToolIconName(event.tool_name)" />
                      <t-tooltip v-if="event.tool_name === 'todo_write' && event.tool_data?.steps"
                        :content="t('agent.updatePlan')" placement="top">
                        <span class="action-name">{{ $t('agent.updatePlan') }}</span>
                      </t-tooltip>
                      <t-tooltip v-else :content="getToolTitle(event)" placement="top">
                        <span class="action-name">{{ getToolTitle(event) }}</span>
                      </t-tooltip>
                    </div>
                  </div>

                  <div v-if="!event.pending && event.tool_name === 'todo_write' && event.tool_data?.steps"
                    class="plan-status-summary-fixed">
                    <div class="plan-status-text">
                      <template v-for="(part, partIndex) in getPlanStatusItems(event)" :key="partIndex">
                        <t-icon :name="part.icon" :class="['status-icon', part.class]" />
                        <span>{{ part.label }} {{ part.count }}</span>
                        <span v-if="partIndex < getPlanStatusItems(event).length - 1" class="separator">·</span>
                      </template>
                    </div>
                  </div>

                  <div
                    v-if="!event.pending && (event.tool_name === 'search_knowledge' || event.tool_name === 'knowledge_search') && event.tool_data"
                    class="search-results-summary-fixed">
                    <div class="results-summary-text" v-html="getSearchResultsSummary(event)"></div>
                  </div>

                  <div v-if="!event.pending && event.tool_name === 'grep_chunks' && event.tool_data"
                    class="search-results-summary-fixed grep-summary">
                    <div class="results-summary-text" v-html="getGrepResultsSummary(event.tool_data)"></div>
                  </div>

                  <div v-if="!event.pending && event.tool_name === 'list_knowledge_chunks' && event.tool_data"
                    class="search-results-summary-fixed knowledge-chunks-summary">
                    <div class="results-summary-text" v-html="getKnowledgeChunksSummary(event.tool_data)"></div>
                  </div>

                  <div v-if="isEventExpanded(event.tool_call_id) && !event.pending && hasExpandableResults(event)"
                    class="action-details">
                    <div v-if="event.display_type && event.tool_data" class="tool-result-wrapper">
                      <ToolResultRenderer :display-type="event.display_type" :tool-data="event.tool_data"
                        :output="event.output" :arguments="event.arguments" />
                    </div>
                    <div v-else-if="event.output" class="tool-output-wrapper">
                      <div class="fallback-header">
                        <span class="fallback-label">{{ $t('chat.rawOutputLabel') }}</span>
                      </div>
                      <div class="detail-output-wrapper">
                        <div class="detail-output">{{ event.output }}</div>
                      </div>
                    </div>
                    <!-- Raw arguments hidden for user-friendly display -->
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>
        <div v-if="isConversationDone" class="tree-child tree-child-last agent-step-done">
          <div class="tree-branch"></div>
          <div class="tree-child-content">
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

    <!-- Event Stream (non-tree mode: before answer starts, or answer events) -->
    <div v-if="!ragMode || displayEvents.length > 0 || showAgentActivityIndicator" ref="streamingStepsContainer"
      class="streaming-steps-container" :class="{
        'streaming-steps-constrained': !answerEverStarted && !isConversationDone,
        'is-streaming-timeline': showStreamingTimeline
      }">
      <template v-for="(event, index) in displayEvents" :key="getEventKey(event, index)">
        <div v-if="event && event.type" class="event-item" :class="{
          'event-answer': event.type === 'answer',
          'tree-child': isStreamingTimelineEvent(event),
          'tree-child-last': isStreamingTimelineEvent(event) && !showAgentActivityIndicator && index === lastStreamingTimelineEventIndex
        }">
          <div v-if="isStreamingTimelineEvent(event)" class="tree-branch"></div>
          <div :class="{ 'tree-child-content': isStreamingTimelineEvent(event) }">

            <!-- Question Understood Event (一次性 done 节点，样式与折叠树一致) -->
            <div v-if="event.type === 'question_understood'" class="tool-event">
              <div class="action-card no-results">
                <div class="action-header">
                  <div class="action-title">
                    <t-icon class="action-title-icon" name="check-circle" />
                    <span class="action-name">{{ t('agentStream.toolStatus.queryUnderstandDone') }}</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- Knowledge Retrieved Event (一次性 done 节点，样式与折叠树一致) -->
            <div v-else-if="event.type === 'knowledge_retrieved'" class="tool-event">
              <div class="action-card no-results">
                <div class="action-header">
                  <div class="action-title">
                    <t-icon class="action-title-icon" name="search" />
                    <span class="action-name">{{ t('agentStream.ragPipeline.searchDone') }}</span>
                    <span v-if="event.query" class="action-summary">「{{ event.query }}」</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- Thinking Event (streaming / merged). A folded preamble (retracted
             from the answer area) is shown as the card title; the reasoning is
             the expandable body. -->
            <div v-if="event.type === 'thinking'" class="tool-event">
              <div class="action-card" :class="{ 'action-pending': isThinkingActive(event.event_id) }">
                <div class="action-header" @click="toggleEvent(event.event_id)">
                  <div class="action-title">
                    <span class="action-title-icon icon-mask" :style="maskIconStyle(thinkingIcon)" aria-hidden="true" />
                    <span v-if="event.title" class="action-name action-preamble-title">{{ event.title }}</span>
                    <span v-else class="action-name">{{ $t('agent.think') }}</span>
                    <span v-if="!event.title && getThinkingSummary(event) && !isEventExpanded(event.event_id)"
                      class="action-summary">{{ getThinkingSummary(event) }}</span>
                  </div>
                </div>
                <div v-if="event.content && isEventExpanded(event.event_id)" class="action-details">
                  <div class="thinking-detail-content markdown-content">
                    <div v-html="renderMarkdownContent(event.content)"></div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Thinking Tool Call -->
            <div v-else-if="event.type === 'tool_call' && event.tool_name === 'thinking'" class="tool-event">
              <div class="action-card"
                :class="{ 'action-pending': event.pending || isThinkingActive(event.tool_call_id) }">
                <div class="action-header" @click="toggleEvent(event.tool_call_id)">
                  <div class="action-title">
                    <span class="action-title-icon icon-mask" :style="maskIconStyle(thinkingIcon)" aria-hidden="true" />
                    <span class="action-name">{{ $t('agent.think') }}</span>
                    <span v-if="event.tool_data?.thought_number" class="action-badge">{{ event.tool_data.thought_number
                      }}/{{ event.tool_data.total_thoughts }}</span>
                    <span v-if="getThinkingSummary(event) && !isEventExpanded(event.tool_call_id)"
                      class="action-summary">{{ getThinkingSummary(event) }}</span>
                  </div>
                </div>
                <div v-if="event.tool_data?.thought && isEventExpanded(event.tool_call_id)" class="action-details">
                  <div class="thinking-detail-content markdown-content">
                    <div v-html="renderMarkdownContent(event.tool_data.thought)"></div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Answer Event -->
            <div v-else-if="event.type === 'answer' && (event.done || (event.content && event.content.trim()))"
              class="answer-event">
              <div v-if="event.content && event.content.trim()" class="answer-content markdown-content">
                <div v-stable-html="renderAnswerContent(event === activeAnswerEventRef ? typedAnswer : event.content)">
                </div>
              </div>
              <div v-if="event.done && event.content && event.content.trim()" class="answer-toolbar">
                <t-button size="small" variant="outline" shape="round" :loading="copyingAnswer"
                  @click.stop="handleCopyAnswer(event)" :title="$t('agent.copy')">
                  <t-icon name="copy" />
                </t-button>
                <t-button size="small" variant="outline" shape="round" :disabled="!props.userQuery"
                  @click.stop="handleRegenerate" :title="$t('chat.regenerate')">
                  <t-icon name="edit-2" />
                </t-button>
                <!-- 点赞 / 点踩：先在前端落库状态；接口占位见 src/api/chat/feedback.ts -->
                <t-button size="small" variant="outline" shape="round" class="feedback-btn feedback-btn--like"
                  :class="{ 'is-active': currentFeedback === 'like', 'is-pulsing': pulseRating === 'like' }"
                  :disabled="feedbackSubmitting" :title="currentFeedback === 'like'
                    ? $t('agentStream.feedback.alreadyLiked')
                    : $t('agentStream.feedback.likeTip')" :aria-label="$t('agentStream.feedback.like')"
                  @click.stop="handleLike(event)">
                  <t-icon name="thumb-up" />
                </t-button>
                <t-button size="small" variant="outline" shape="round" class="feedback-btn feedback-btn--dislike"
                  :class="{ 'is-active': currentFeedback === 'dislike', 'is-pulsing': pulseRating === 'dislike' }"
                  :disabled="feedbackSubmitting" :title="currentFeedback === 'dislike'
                    ? $t('agentStream.feedback.alreadyDisliked')
                    : $t('agentStream.feedback.dislikeTip')" :aria-label="$t('agentStream.feedback.dislike')"
                  @click.stop="openDislikeDialog(event)">
                  <t-icon name="thumb-down" />
                </t-button>
                <div v-if="answerReferenceSummary" size="small" variant="outline" shape="round"
                  class="answer-references-btn" :title="answerReferenceSummary" role="button" tabindex="0"
                  @click.stop="handleOpenReferences()" @keydown.enter.prevent="handleOpenReferences()"
                  @keydown.space.prevent="handleOpenReferences()">
                  <span class="answer-references-btn__label">{{ answerReferenceSummary }}</span>
                  <t-icon name="caret-right-small" />
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
      <div v-if="showRequestInfo && isConversationDone && !hasDoneAnswerContent" class="answer-toolbar">
        <ChatRequestInfoButton :session="session" :session-id="sessionId" />
      </div>
      <!-- Loading Indicator (inside container so it scrolls into view) -->
      <!-- <div v-if="showAgentActivityIndicator" class="tree-child tree-child-last streaming-loading-node">
        <div class="tree-branch"></div>
        <div class="tree-child-content">
          <div class="loading-indicator">
            <div class="loading-typing">
              <span></span>
              <span></span>
              <span></span>
            </div>
          </div>
        </div>
      </div> -->
    </div>
  </div>
  <!-- 点踩反馈弹窗（接口占位见 src/api/chat/feedback.ts） -->
  <DislikeFeedbackDialog v-model:visible="dislikeDialogVisible" :message-id="session.id"
    :session-id="session.session_id" @submitted="handleFeedbackSubmitted" />
  <!-- 引用 hover 浮层（与历史消息共用同一组件） -->
  <ChatCitationFloat :float="citationFloat" :on-enter="cancelCitationClose" :on-leave="scheduleCitationClose" />

  <!-- Image Preview -->
  <picturePreview :reviewImg="imagePreviewVisible" :reviewUrl="imagePreviewUrl" @closePreImg="closeImagePreview" />

</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, onUpdated, nextTick } from 'vue';

import { marked } from 'marked';
import 'katex/dist/katex.min.css';
import ToolResultRenderer from './ToolResultRenderer.vue';
import ChatRequestInfoButton from '@/components/ChatRequestInfoButton.vue';
import ChatCitationFloat from '@/components/ChatCitationFloat.vue';
import picturePreview from '@/components/picture-preview.vue';
import DislikeFeedbackDialog from './DislikeFeedbackDialog.vue';
import {
  submitMessageFeedback,
  cancelMessageFeedback,
  type FeedbackRating,
  type FeedbackSubmitResult,
} from '@/api/chat/feedback';
import { countGrepDocuments, groupGrepChunkResults } from '@/utils/grepResultsGroup';
import { getKnowledgeChunksSummaryHtml } from '@/utils/knowledgeChunksDisplay';
import { useChatCitationPopover } from '@/composables/useChatCitationPopover';
import { useChatReferencesDrawer } from '@/composables/useChatReferencesDrawer';
import { buildReferenceSections, type KnowledgeReferenceLike } from '@/utils/referenceSources';
import { buildCitationEnrichedCopyText, resolveCitationChunkId, resolveCitationChunks, stripCitationTagsForCopy } from '@/utils/citationMarkdown';
import { buildAgentAnswerExportFileName, buildAgentAnswerExportHtml } from '@/utils/agentAnswerHtmlExport';
import { getCitationChunkCache, setCitationChunkCache } from '@/utils/citationChunkCache';
import { getChunkByIdOnly } from '@/api/knowledge-base';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import i18n from '@/i18n';
import { hydrateProtectedFileImages, clearProtectedFileFailureCache, sanitizeMarkdownHTML } from '@/utils/security';
import { unwrapFinalAnswerWrappers, thinkingEqualsAnswer } from '@/utils/finalAnswer';
import { getAgentToolIconName } from '@/utils/agent-tool-icons';
import { getQueryText } from '@/utils/agent-tool-display';
import {
  copyTextToClipboard,
  replaceIncompleteMermaidWithPlaceholder,
  prepareStreamingMermaidMarkdown,
  extractFirstMermaidCode,
  injectCachedMermaidSvg,
} from '@/utils/chatMessageShared';
import {
  configureMarkedForChatMarkdown,
  renderChatMarkdown,
  wrapChatMarkdownTables,
} from '@/utils/chatMarkdownRenderer';
import {
  createMermaidCodeRenderer,
  ensureMermaidInitialized,
  enhanceMarkdownContainer,
  renderMermaidToSvg,
} from '@/utils/mermaidShared';
import { attachMarkdownEnhancementListeners, refreshMarkdownEnhancements } from '@/utils/markdownEnhancements';
import { useTypewriter } from '@/composables/useTypewriter';
import { vStableHtml } from '@/directives/stableHtml';
import { isDebugger } from '@/composables/featureFlags';

const getToolIconName = getAgentToolIconName;


const { t } = useI18n();

ensureMermaidInitialized();

const TOOL_NAME_KEYS: Record<string, string> = {
  search_knowledge: 'agentStream.tools.searchKnowledge',
  knowledge_search: 'agentStream.tools.searchKnowledge',
  grep_chunks: 'agentStream.tools.grepChunks',
  get_document_info: 'agentStream.tools.getDocumentInfo',
  list_knowledge_chunks: 'agentStream.tools.listKnowledgeChunks',
  get_related_documents: 'agentStream.tools.getRelatedDocuments',
  get_document_content: 'agentStream.tools.getDocumentContent',
  todo_write: 'agentStream.tools.todoWrite',
  knowledge_graph_extract: 'agentStream.tools.knowledgeGraphExtract',
  thinking: 'agentStream.tools.thinking',
  image_analysis: 'agentStream.tools.imageAnalysis',
  query_understand: 'agentStream.tools.queryUnderstand',
  query_knowledge_graph: 'agentStream.tools.queryKnowledgeGraph',
  read_skill: 'agentStream.tools.readSkill',
  execute_skill_script: 'agentStream.tools.executeSkillScript',
  data_analysis: 'agentStream.tools.dataAnalysis',
  data_schema: 'agentStream.tools.dataSchema',
  database_query: 'agentStream.tools.databaseQuery',
};

const getLocalizedToolName = (toolName?: string | null): string => {
  if (!toolName) return t('agent.toolFallback');
  const key = TOOL_NAME_KEYS[toolName];
  if (key) return t(key);

  return toolName;
};

const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;
const ID_LABEL_RE = /\b(knowledge_base_id|knowledge_id|chunk_id|knowledge_base_ids)\s*[:=]\s*/gi;

const sanitizeForDisplay = (text: string): string => {
  if (!text) return text;
  let result = text;
  for (const [name, i18nKey] of Object.entries(TOOL_NAME_KEYS)) {
    result = result.replaceAll(name, i18n.global.t(i18nKey));
  }
  result = result.replace(ID_LABEL_RE, '');
  result = result.replace(UUID_RE, '');
  // Remove empty inline code like `` or ` ` while preserving triple-backtick
  // fenced code blocks (```). Without the lookaround the greedy pair match
  // would eat two of the three fence backticks and break code block rendering.
  result = result.replace(/(?<!`)`[ \t]*`(?!`)/g, '');
  result = result.replace(/\(\s*\)/g, '');
  return result;
};

// 根元素引用
const rootElement = ref<HTMLElement | null>(null);

const streamingStepsContainer = ref<HTMLElement | null>(null);

// 图片预览状态
const imagePreviewVisible = ref(false);
const imagePreviewUrl = ref('');

const openImagePreview = (url: string) => {
  imagePreviewUrl.value = url;
  imagePreviewVisible.value = true;
};

const closeImagePreview = () => {
  imagePreviewVisible.value = false;
};

// Import icons
import agentIcon from '@/assets/img/agent.svg';
import thinkingIcon from '@/assets/img/Frame3718.svg';

interface SessionData {
  id?: string;
  request_id?: string;
  debugRequest?: Record<string, unknown>;
  isAgentMode?: boolean;
  agentEventStream?: any[];
  knowledge_references?: any[];
  [key: string]: unknown;
}

const props = defineProps<{
  session: SessionData;
  sessionId?: string;
  userQuery?: string;
  ragMode?: boolean;
}>();

const emit = defineEmits<{
  /**
   * 点击「重新生成」时触发：把当前用户问题回填到输入框。
   * 由父级 chat/index.vue 监听，最终调用 Input-field 的 triggerSend。
   */
  (e: 'regenerate', query: string): void;
}>();

const showRequestInfo = computed(
  () => !!(props.session?.request_id || props.session?.id),
);

const {
  float: citationFloat,
  rebind: rebindCitations,
  cancelClose: cancelCitationClose,
  scheduleClose: scheduleCitationClose,
} = useChatCitationPopover(rootElement, {
  getKnowledgeReferences: () => props.session?.knowledge_references,
  sessionId: () => props.sessionId,
});

const referencesDrawer = useChatReferencesDrawer();

const openReferencesDrawer = (
  highlight?: { url?: string; chunkId?: string },
  refsOverride?: KnowledgeReferenceLike[] | null,
) => {
  const refs = refsOverride?.length ? refsOverride : props.session?.knowledge_references
  if (!referencesDrawer || !refs?.length) return false
  referencesDrawer.open({
    references: refs,
    highlight: highlight || null,
    messageId: props.session?.id,
  })
  return true
}

// 与 RagPipelineProgress 组件内"检索知识库结果"按钮一致：仅统计 document 类型的引用，
// 在 answer-toolbar 上展示一个快捷入口，点击后打开 references drawer。
const answerReferenceSummary = computed(() => {
  const sections = buildReferenceSections(props.session?.knowledge_references)
  const docCount = sections.find((section) => section.id === 'documents')?.items.length ?? 0
  if (docCount <= 0) return ''
  return t('chat.referencesDocCount', { count: docCount })
})

// 「参考 N 篇资料」按钮：点击时切换 references drawer 的显隐。
// 已打开且 sourceKey 仍然指向同一个 message 时再次点击会收起（与内嵌 action-card 行为一致）。
function handleOpenReferences() {
  if (!referencesDrawer) return
  const refs = props.session?.knowledge_references
  if (!refs?.length) return
  referencesDrawer.toggle({
    references: refs,
    highlight: null,
    messageId: props.session?.id,
    sourceKey: `answer-toolbar:${props.session?.id || 'session'}`,
  })
}

const mergeDocumentReferences = (refs: KnowledgeReferenceLike[]): KnowledgeReferenceLike[] => {
  const merged = new Map<string, KnowledgeReferenceLike & { contentParts?: string[] }>();

  for (const ref of refs) {
    const key = ref.knowledge_id || ref.knowledge_title || ref.id;
    if (!key) continue;

    const existing = merged.get(key);
    const content = String(ref.content || '').trim();
    if (!existing) {
      merged.set(key, {
        ...ref,
        id: ref.knowledge_id || ref.id || key,
        content,
        contentParts: content ? [content] : [],
      });
      continue;
    }

    if (content && !existing.contentParts?.includes(content)) {
      existing.contentParts = [...(existing.contentParts || []), content];
      existing.content = existing.contentParts.slice(0, 3).join('\n\n');
    }
  }

  return Array.from(merged.values()).map(({ contentParts, ...ref }) => ref);
};

const cleanToolOutputContent = (output: unknown): string => {
  const raw = typeof output === 'string' ? output : '';
  return raw
    .replace(/<\/?knowledge_chunks[^>]*>/gi, '')
    .replace(/<\/?chunk[^>]*>/gi, '')
    .trim();
};

const getToolKnowledgeBaseId = (toolData: any): string | undefined => {
  if (typeof toolData?.knowledge_base_id === 'string' && toolData.knowledge_base_id) {
    return toolData.knowledge_base_id;
  }
  const kbIds = Array.isArray(toolData?.knowledge_base_ids) ? toolData.knowledge_base_ids : [];
  if (kbIds.length === 1 && typeof kbIds[0] === 'string' && kbIds[0]) {
    return kbIds[0];
  }
  return undefined;
};

const formatToolResultContent = (value: unknown): string => {
  if (typeof value === 'string') return value.trim();
  if (value == null) return '';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
};


const getToolReferenceItems = (event: any): KnowledgeReferenceLike[] => {
  if (!event || event.pending) return [];
  const toolName = event.tool_name;
  const toolData = event.tool_data;

  if (!toolData) return [];

  if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    const results = Array.isArray(toolData.results) ? toolData.results : [];
    const fallbackKnowledgeBaseId = getToolKnowledgeBaseId(toolData);
    return mergeDocumentReferences(results
      .filter((item: any) => item?.chunk_id || item?.knowledge_id)
      .map((item: any, index: number) => ({
        id: item.chunk_id || `${item.knowledge_id}-${item.result_index ?? index + 1}`,
        knowledge_id: item.knowledge_id,
        knowledge_title: item.faq_standard_question || item.knowledge_title,
        knowledge_base_id: item.knowledge_base_id || fallbackKnowledgeBaseId,
        chunk_index: item.result_index ?? index + 1,
        chunk_type: item.chunk_type,
        content: item.content || '',
      })));
  }

  if (toolName === 'grep_chunks') {
    const chunkResults = Array.isArray(toolData.chunk_results) ? toolData.chunk_results : [];
    if (chunkResults.length) {
      return groupGrepChunkResults(chunkResults)
        .filter((group) => group.knowledge_id || group.title)
        .map((group, index) => ({
          id: group.knowledge_id || group.key,
          knowledge_id: group.knowledge_id,
          knowledge_title: group.title,
          chunk_index: index + 1,
          chunk_type: group.is_faq ? 'faq' : undefined,
          content: group.chunks.map((chunk) => chunk.content).filter(Boolean).slice(0, 3).join('\n\n') || group.match_snippet || '',
        }));
    }

    const knowledgeResults = Array.isArray(toolData.knowledge_results) ? toolData.knowledge_results : [];
    return mergeDocumentReferences(knowledgeResults
      .filter((item: any) => item?.knowledge_id)
      .map((item: any, index: number) => ({
        id: item.knowledge_id,
        knowledge_id: item.knowledge_id,
        knowledge_title: item.faq_question || item.knowledge_title,
        knowledge_base_id: item.knowledge_base_id,
        chunk_index: index + 1,
        content: item.match_snippet || '',
      })));
  }

  if (toolName === 'list_knowledge_chunks') {
    const chunks = Array.isArray(toolData.chunks) ? toolData.chunks : [];
    if (chunks.length) {
      return mergeDocumentReferences(chunks
        .filter((item: any) => item?.content)
        .map((item: any, index: number) => ({
          id: item.chunk_id || item.id || `${toolData.knowledge_id || 'doc'}-${index + 1}`,
          knowledge_id: item.knowledge_id || toolData.knowledge_id,
          knowledge_title: toolData.faq_question || toolData.knowledge_title || toolData.knowledge_id,
          knowledge_base_id: item.knowledge_base_id || toolData.knowledge_base_id,
          chunk_index: item.chunk_index ?? item.index ?? index + 1,
          chunk_type: item.chunk_type || (toolData.faq_question ? 'faq' : undefined),
          content: item.content || '',
        })));
    }

    const output = cleanToolOutputContent(event.output);
    if (!output) return [];
    return [{
      id: toolData.faq_id || toolData.knowledge_id || event.tool_call_id,
      knowledge_id: toolData.knowledge_id,
      knowledge_title: toolData.faq_question || toolData.knowledge_title || toolData.knowledge_id || getToolDescription(event),
      knowledge_base_id: toolData.knowledge_base_id,
      chunk_type: toolData.faq_question ? 'faq' : undefined,
      content: output,
    }];
  }

  return [];
};

const canOpenToolReferences = (event: any): boolean => getToolReferenceItems(event).length > 0;

const openToolReferences = (event: any): boolean => {
  const refs = getToolReferenceItems(event);
  if (!referencesDrawer || !refs.length) return false;
  referencesDrawer.toggle({
    references: refs,
    highlight: null,
    messageId: props.session?.id,
    sourceKey: `tool:${props.session?.id || 'session'}:${event.tool_call_id || event.event_id || event.tool_name || 'references'}`,
  });
  return true;
};

configureMarkedForChatMarkdown();

// Event stream
const eventStream = computed(() => props.session?.agentEventStream || []);

// Expanded events tracking (for tool calls and thinking events)
const expandedEvents = ref<Set<string>>(new Set());

// Track IDs of thinking events that are currently "active" (latest, not yet followed by non-thinking)
const activeThinkingIds = ref<Set<string>>(new Set());
// Reactive version number to force template re-evaluation when activeThinkingIds changes
const activeThinkingVersion = ref(0);

const isThinkingActive = (eventId: string): boolean => {
  // Reference version to create reactive dependency
  void activeThinkingVersion.value;
  return activeThinkingIds.value.has(eventId);
};

// Watch event stream to auto-expand thinking events and auto-collapse when non-thinking follows
watch(eventStream, (stream) => {
  if (!stream || !Array.isArray(stream)) return;

  // Scan stream to find thinking events to expand and collapse
  const newActiveIds = new Set<string>();

  // Walk backwards to find the trailing thinking block
  let inTrailingThinking = true;
  for (let i = stream.length - 1; i >= 0; i--) {
    const event = stream[i];
    if (!event) continue;

    const isThinking = event.type === 'thinking' ||
      (event.type === 'tool_call' && event.tool_name === 'thinking');
    const id = event.type === 'thinking' ? event.event_id : event.tool_call_id;

    if (inTrailingThinking && isThinking && id) {
      newActiveIds.add(id);
      // Auto-expand if not yet known
      expandedEvents.value.add(id);
    } else if (!isThinking) {
      inTrailingThinking = false;
    }
  }

  // Collapse thinking events that were active before but are no longer trailing
  for (const oldId of activeThinkingIds.value) {
    if (!newActiveIds.has(oldId)) {
      expandedEvents.value.delete(oldId);
    }
  }

  activeThinkingIds.value = newActiveIds;
  activeThinkingVersion.value++;

  nextTick(async () => {
    await hydrateProtectedFileImages(rootElement.value);
    await enhanceMarkdownContainer(rootElement.value);
    // Auto-scroll thinking detail content to bottom during streaming
    if (newActiveIds.size > 0 && rootElement.value) {
      const els = rootElement.value.querySelectorAll('.thinking-detail-content');
      els.forEach((el: Element) => {
        const htmlEl = el as HTMLElement;
        if (htmlEl.scrollHeight > htmlEl.clientHeight) {
          htmlEl.scrollTop = htmlEl.scrollHeight;
        }
      });
    }
    // Auto-scroll the steps container to the bottom while it is still height-
    // capped (steps-only phase). Once answer text appears the cap is released
    // and the container grows with the page, so internal scrolling is moot.
    if (!answerEverStarted.value && streamingStepsContainer.value) {
      const el = streamingStepsContainer.value;
      if (el.scrollHeight > el.clientHeight) {
        el.scrollTop = el.scrollHeight;
      }
    }
  });
}, { immediate: true, deep: true });

// State for intermediate steps collapse
const showIntermediateSteps = ref(false);

// Track whether a non-superseded answer is streaming. Plain content streams
// optimistically as an `answer` event (rendered answer-style in the answer
// area). If the round turns out to be a tool round, that event is marked
// `superseded` and retracted into the steps — so a superseded segment must NOT
// count as "answer started", otherwise the answer-only view would stick after
// the preamble was retracted.
const hasAnswerStarted = computed(() => {
  const stream = eventStream.value;
  if (!stream || !Array.isArray(stream)) return false;
  return stream.some((e: any) => e.type === 'answer' && !e.superseded && e.content && e.content.trim());
});

// Whether ANY answer text has ever appeared this turn — including a preamble
// that was later superseded (its content stays in the stream). Used to release
// the live container's height cap. Unlike hasAnswerStarted this is monotonic:
// it does not flip back when a preamble is retracted, so the container does not
// shrink back to the capped height (which would look like a jump). Once the
// model starts producing answer-style text, give it full height to breathe.
const answerEverStarted = computed(() => {
  const stream = eventStream.value;
  if (!stream || !Array.isArray(stream)) return false;
  return stream.some((e: any) => e.type === 'answer' && e.content && e.content.trim());
});

const agentDurationMs = ref<number>(0);
watch(eventStream, (stream) => {
  if (!stream || !Array.isArray(stream)) return;

  // Check for agent_complete event with authoritative duration from backend
  if (agentDurationMs.value === 0) {
    const completeEvent = stream.find((e: any) => e.type === 'agent_complete' && e.total_duration_ms);
    if (completeEvent) {
      agentDurationMs.value = completeEvent.total_duration_ms;
    }
  }
}, { deep: true, immediate: true });


// Check if conversation is done (based on answer event with done=true or stop event)
const isConversationDone = computed(() => {
  const stream = eventStream.value;
  if (!stream || stream.length === 0) {
    console.log('[Collapse] No stream or empty stream');
    return false;
  }

  // Check for stop event (user cancelled)
  const stopEvent = stream.find((e: any) => e.type === 'stop');
  if (stopEvent) {
    console.log('[Collapse] Found stop event, conversation done');
    return true;
  }

  const completeEvent = stream.find((e: any) => e.type === 'agent_complete');
  if (completeEvent) {
    console.log('[Collapse] Found complete event, conversation done');
    return true;
  }

  // Check for answer event with done=true. Exclude superseded preambles: a
  // retracted tool-round preamble is also closed with done=true, but the agent
  // keeps running, so it must not mark the whole conversation as finished.
  const answerEvents = stream.filter((e: any) => e.type === 'answer' && !e.superseded);
  const doneAnswer = answerEvents.find((e: any) => e.done === true);

  return !!doneAnswer;
});

// history resume 场景（刷新页面后从 continue-stream 恢复）：即便 is_completed 仍为 false，
// 历史事件已存在并代表"已生成的输出"。此时 answer 文本应当 snap 到完整内容，不能按
// typewriter 逐字重打，否则会出现内容一段段蹦出的闪烁感。
const isHistoryResuming = computed(() => {
  // __historyWasInFlight 在 useChatStreamHandler.handleMsgList 中设置，仅作用于
  // "history 重建但未完成"的 assistant 消息，普通会话为 undefined。
  return Boolean((props as any).session?.__historyWasInFlight)
});

const streamingMermaidSvgCache = ref<string | null>(null);
let streamingMermaidRenderTask: Promise<void> | null = null;
let streamingMermaidRenderId = 0;

const activeAnswerMarkdown = computed(() => {
  const stream = eventStream.value;
  if (!stream?.length) return '';
  const answers = stream.filter((e: any) => e.type === 'answer' && !e.superseded);
  const active = answers.find((e: any) => !e.done) ?? answers[answers.length - 1];
  return typeof active?.content === 'string' ? active.content : '';
});

// The answer event whose text is currently streaming. The template renders the
// smoothed typewriter text for this event and the raw content for any others.
const activeAnswerEventRef = computed(() => {
  const stream = eventStream.value;
  if (!stream?.length) return null;
  const answers = stream.filter((e: any) => e.type === 'answer' && !e.superseded);
  return answers.find((e: any) => !e.done) ?? answers[answers.length - 1] ?? null;
});

// Smooth the streamed answer into a steady typewriter cadence (shared with the
// non-Agent markdown path). History reloads arrive already complete and snap to
// full instead of replaying.
const shouldSnapTypedAnswer = computed(
  () => isConversationDone.value || isHistoryResuming.value,
)
const { displayed: typedAnswer } = useTypewriter(
  () => activeAnswerMarkdown.value,
  () => shouldSnapTypedAnswer.value,
);

const cacheStreamingMermaidSvg = async () => {
  if (streamingMermaidSvgCache.value) return;
  const code = extractFirstMermaidCode(activeAnswerMarkdown.value);
  if (!code) return;

  if (!streamingMermaidRenderTask) {
    streamingMermaidRenderTask = (async () => {
      const svg = await renderMermaidToSvg(code, `mermaid-agent-stream-${++streamingMermaidRenderId}`);
      if (svg) streamingMermaidSvgCache.value = svg;
    })().finally(() => {
      streamingMermaidRenderTask = null;
    });
  }

  await streamingMermaidRenderTask;
};

watch(isConversationDone, (done) => {
  if (!done) {
    streamingMermaidSvgCache.value = null;
    streamingMermaidRenderTask = null;
  }
});

watch(streamingMermaidSvgCache, () => {
  nextTick(() => refreshMarkdownEnhancements(rootElement.value));
});

watch(activeAnswerMarkdown, () => {
  if (isConversationDone.value || streamingMermaidSvgCache.value) return;
  void cacheStreamingMermaidSvg();
});

// When the turn finishes, clear the failed-fetch cooldown and re-hydrate once.
// Files referenced mid-stream (e.g. exported images) may only become available
// at completion; throttling stops the chunk-by-chunk 404 spam during streaming,
// and this final pass guarantees they load without waiting out the cooldown.
//
// Gate this on the typewriter having fully revealed the answer: when done flips,
// the smoothed text may still be catching up, so the <img> tag is not in the DOM
// yet. Hydrating too early would find nothing and leave a permanent placeholder
// (until a manual reload). Waiting for full reveal guarantees the image exists.
const answerFullyRendered = computed(
  () => isConversationDone.value && typedAnswer.value.length >= activeAnswerMarkdown.value.length,
);
watch(answerFullyRendered, (ready) => {
  if (!ready) return;
  // Clear before this reactive update renders, so a source that returned 404
  // mid-stream gets one real final-attempt <img> node instead of remaining
  // suppressed by the missing-source cache.
  clearProtectedFileFailureCache();
  nextTick(async () => {
    await hydrateProtectedFileImages(rootElement.value);
  });
});

// Agent: dots until the turn completes. RAG: pipeline dots before answer; answer stream dots after.
const showAgentActivityIndicator = computed(() => {
  if (isConversationDone.value) return false;
  if (props.ragMode) return hasAnswerStarted.value;
  return true;
});

const isStreamingTimelineEvent = (event: any): boolean => {
  return !isConversationDone.value && event?.type && event.type !== 'answer';
};

const showStreamingTimeline = computed(() => {
  return displayEvents.value.some((event: any) => isStreamingTimelineEvent(event)) || showAgentActivityIndicator.value;
});

const lastStreamingTimelineEventIndex = computed(() => {
  if (isConversationDone.value) return -1;
  for (let i = displayEvents.value.length - 1; i >= 0; i -= 1) {
    if (isStreamingTimelineEvent(displayEvents.value[i])) return i;
  }
  return -1;
});

// Whether a completed answer with content is rendered (its toolbar hosts the
// request-info button inline, so the standalone toolbar should not duplicate it)
const hasDoneAnswerContent = computed(() => {
  const stream = eventStream.value;
  if (!stream || stream.length === 0) return false;
  return stream.some(
    (e: any) => e.type === 'answer' && e.done && e.content && e.content.trim()
  );
});

// Find the final content to display (last thinking or answer)
const finalContent = computed(() => {
  const stream = eventStream.value;
  if (!stream || stream.length === 0) {
    return null;
  }

  if (!isConversationDone.value) {
    return null;
  }

  // Check if there's a (non-superseded) answer event with content. Superseded
  // preambles carry content too, but they were retracted into the steps and are
  // not the final answer, so they must not count here.
  const answerEvents = stream.filter((e: any) => e.type === 'answer' && !e.superseded);
  const hasAnswerContent = answerEvents.some((e: any) => e.content && e.content.trim());

  if (hasAnswerContent) {
    return { type: 'answer' };
  }

  // Do NOT fall back to re-rendering the last thinking event when the
  // intermediate-steps tree already shows it — that would duplicate the
  // thinking card below the tree. The fallback is only meaningful for
  // legacy conversations where the tree is absent. Also skip for
  // user-stopped conversations which have no final answer to fall back to.
  if (shouldShowCollapsedSteps.value) {
    return null;
  }
  const wasStopped = stream.some((e: any) => e.type === 'stop');
  if (wasStopped) {
    return null;
  }

  // Fallback: if no answer content (e.g. the model ended with only reasoning),
  // use last thinking as final content
  const thinkingEvents = stream.filter((e: any) => e.type === 'thinking' && e.content && e.content.trim());
  if (thinkingEvents.length > 0) {
    const lastThinking = thinkingEvents[thinkingEvents.length - 1];
    const doneAnswer = answerEvents.find((e: any) => e.done === true);
    return {
      type: 'thinking',
      event_id: lastThinking.event_id,
      showAnswerToolbar: !!doneAnswer
    };
  }

  return null;
});

// Count intermediate steps (after merging consecutive thinking events, matching what user sees in tree)
const intermediateStepsCount = computed(() => {
  if (!hasAnswerStarted.value && !isConversationDone.value) return 0;
  // Count only thinking and tool_call events (exclude plan_task_change, etc.)
  return intermediateEvents.value.filter(
    (e: any) => e.type === 'thinking' || e.type === 'tool_call'
  ).length;
});

// Number of reasoning rounds (thinking cards) and tool invocations. We report
// these separately instead of summing them into one opaque "step" count, which
// over-counts what the user perceives as agent loops (a single loop emits one
// thinking card plus its tool calls).
const reasoningRoundsCount = computed(() => {
  if (!hasAnswerStarted.value && !isConversationDone.value) return 0;
  return intermediateEvents.value.filter((e: any) => e.type === 'thinking').length;
});

const toolCallsCount = computed(() => {
  if (!hasAnswerStarted.value && !isConversationDone.value) return 0;
  return intermediateEvents.value.filter((e: any) => e.type === 'tool_call').length;
});

const intermediateStepsSummary = computed(() => {
  if (!eventStream.value) {
    return '';
  }

  const rounds = reasoningRoundsCount.value;
  const tools = toolCallsCount.value;
  const elapsed = agentDurationMs.value;

  const parts: string[] = [];
  if (rounds > 0) {
    parts.push(t('agent.reasoningRounds', { rounds }));
  }
  if (tools > 0) {
    parts.push(t('agent.toolCalls', { tools }));
  }
  // Fallback to a generic step count if neither bucket has anything (shouldn't
  // normally happen once the tree is shown).
  if (parts.length === 0) {
    parts.push(t('agent.stepsCompleted', { steps: intermediateStepsCount.value }));
  }

  if (elapsed > 0) {
    parts.push(t('agent.durationSuffix', { duration: formatDuration(elapsed) }));
  }

  return parts.join(t('agent.stepSummarySeparator'));
});

// HTML version of intermediate steps summary with colored numbers
const intermediateStepsSummaryHtml = computed(() => {
  return intermediateStepsSummary.value;
});

// Should show the collapsed steps indicator (tree root). Collapse ONLY once the
// conversation is done. RAG quick-answer mode never shows the tool tree —
// intermediate progress is handled by RagPipelineProgress and disappears once
// references or the answer arrive.
const shouldShowCollapsedSteps = computed(() => {
  if (props.ragMode) return false
  const hasSteps = intermediateStepsCount.value > 0;
  return hasSteps && isConversationDone.value;
});

// Check if event is a "deep thinking" type (either streaming thinking or thinking tool call)
const isThinkingLikeEvent = (event: any): boolean => {
  if (event.type === 'thinking') return true;
  if (event.type === 'tool_call' && event.tool_name === 'thinking') return true;
  return false;
};

// Extract thinking content from an event
const getThinkingContent = (event: any): string => {
  if (event.type === 'thinking') return event.content || '';
  if (event.type === 'tool_call' && event.tool_name === 'thinking') {
    return event.tool_data?.thought || event.output || '';
  }
  return '';
};

// Get a short summary snippet from thinking content for display in the header
const getThinkingSummary = (event: any): string => {
  const content = getThinkingContent(event);
  if (!content) return '';
  const cleaned = sanitizeForDisplay(content)
    .replace(/^#+\s+/gm, '')
    .replace(/\*\*/g, '')
    .replace(/\*/g, '')
    .replace(/`/g, '')
    .replace(/\n+/g, ' ')
    .trim();
  if (cleaned.length <= 50) return cleaned;
  return cleaned.slice(0, 50) + '...';
};

// Helper: build the full result list with plan_task_change injections and thinking merging
const buildFullEventList = (stream: any[]) => {
  const validStream = stream.filter((e: any) => e && typeof e === 'object' && e.type);
  let lastTask: string | null = null;
  const result: any[] = [];

  for (let i = 0; i < validStream.length; i++) {
    const event = validStream[i];
    if (event.type === 'tool_call' && event.tool_name === 'todo_write' && event.tool_data?.task) {
      const currentTask = event.tool_data.task;
      if (lastTask === null || currentTask !== lastTask) {
        result.push({
          type: 'plan_task_change',
          task: currentTask,
          event_id: `plan-task-change-${event.tool_call_id || i}`,
          timestamp: event.timestamp || Date.now()
        });
      }
      lastTask = currentTask;
    }

    // Merge consecutive thinking-like events
    if (isThinkingLikeEvent(event) && result.length > 0) {
      const prev = result[result.length - 1];
      if (isThinkingLikeEvent(prev)) {
        const prevContent = prev._mergedContent || getThinkingContent(prev);
        const curContent = getThinkingContent(event);

        // Deduplicate: when a tool_call thinking event's thought content was
        // already delivered via streaming thinking events (same text), skip it.
        if (curContent && prevContent && prevContent.includes(curContent)) {
          continue;
        }
        if (curContent && prevContent && curContent.includes(prevContent)) {
          // Current fully contains previous — replace instead of appending
          result[result.length - 1] = {
            type: 'thinking',
            event_id: prev.event_id,
            content: curContent,
            thinking: prev.thinking || event.thinking,
            timestamp: prev.timestamp,
            _mergedContent: curContent,
          };
          continue;
        }

        // Normal merge: combine non-overlapping content
        const merged = [prevContent, curContent].filter(Boolean).join('\n\n');
        result[result.length - 1] = {
          type: 'thinking',
          event_id: prev.event_id,
          content: merged,
          thinking: prev.thinking || event.thinking,
          timestamp: prev.timestamp,
          _mergedContent: merged,
        };
        continue;
      }
    }

    result.push(event);
  }

  // Relocate each retracted (superseded) answer — a tool round's optimistic
  // preamble that was pulled out of the answer area — into that round's
  // thinking card as its TITLE, with the reasoning as the body (one card per
  // round). A lone preamble (model has no separate reasoning channel) becomes a
  // title-only thinking card. Non-superseded answers stay as `answer` and are
  // rendered in the answer area, never here.
  const folded: any[] = [];
  for (const e of result) {
    if (e.type === 'answer' && e.superseded) {
      const preambleText = typeof e.content === 'string' ? e.content : '';
      const prev = folded[folded.length - 1];
      if (prev && prev.type === 'thinking' && !prev.title) {
        folded[folded.length - 1] = { ...prev, title: preambleText };
        continue;
      }
      // No reasoning channel: title-only thinking card (same chrome as merged
      // rounds). Rounds with reasoning_content merge preamble into prev.title.
      folded.push({
        type: 'thinking',
        event_id: e.event_id,
        title: preambleText,
        content: '',
        thinking: false,
        timestamp: e.timestamp,
      });
      continue;
    }
    folded.push(e);
  }

  // Drop thinking cards that are entirely empty (no title and no body). Some
  // models emit "\n\n" before a tool call (e.g. qwen3 blank lines between
  // [assistant] and tool_calls), which would otherwise show an empty "思考"
  // card. Keep cards that carry a title (a relocated preamble) even with no
  // reasoning body.
  return folded.filter((e: any) => {
    if (e.type !== 'thinking') return true;
    const content = typeof e.content === 'string' ? e.content : '';
    const title = typeof e.title === 'string' ? e.title : '';
    return content.trim().length > 0 || title.trim().length > 0;
  });
};

// IDs of thinking events that should NOT be rendered in the intermediate-
// steps tree because their content is already shown as the final answer.
// Two cases produce duplicates:
//   1. `promotedThinkingEventId` — agent loop ended via natural-stop with
//      no answer event at all; we promote the trailing thinking into a
//      virtual answer card (see displayEvents) and must hide the source
//      thinking from the tree.
//   2. Natural-stop path on the backend streams answer chunks as thought
//      events first, then re-emits the *same* content as one big answer
//      event. The merged thinking event in the tree would duplicate the
//      answer card, so detect content-equivalence and hide it.
const hiddenThinkingEventIds = computed<Set<string>>(() => {
  const hidden = new Set<string>();
  const stream = eventStream.value;
  if (!stream || !Array.isArray(stream)) return hidden;

  // Case 1: trailing thinking promoted to answer (no answer events present).
  const final = finalContent.value;
  if (final && final.type === 'thinking') {
    const hasRealAnswer = stream.some(
      (e: any) => e.type === 'answer' && !e.superseded && e.content && e.content.trim()
    );
    if (!hasRealAnswer && final.event_id) {
      hidden.add(final.event_id);
    }
  }

  // Case 2: natural-stop duplicates — answer events carry the same content
  // already streamed as thinking chunks. Compare merged thinking events
  // against the concatenated answer content and hide on match. Superseded
  // preambles are excluded: they are the retracted tool-round narration, not
  // the final answer, and are intentionally shown in the steps as titles.
  const answerContent = stream
    .filter((e: any) => e.type === 'answer' && !e.superseded && e.content)
    .map((e: any) => e.content)
    .join('');
  if (answerContent.trim()) {
    const merged = buildFullEventList(stream);
    for (const e of merged) {
      if (e.type !== 'thinking' || !e.event_id) continue;
      if (hidden.has(e.event_id)) continue;
      // Hide a step card that duplicates the final answer. Match the body, or a
      // title-only card (a relocated preamble) whose title equals the answer —
      // but keep cards that still carry a distinct reasoning body so the
      // reasoning stays visible.
      const bodyMatches = e.content && thinkingEqualsAnswer(e.content, answerContent);
      const titleOnlyMatches = e.title && !(e.content && e.content.trim()) &&
        thinkingEqualsAnswer(e.title, answerContent);
      if (bodyMatches || titleOnlyMatches) {
        hidden.add(e.event_id);
      }
    }
  }

  return hidden;
});

// Intermediate events (tree children: everything except answer)
const intermediateEvents = computed(() => {
  const stream = eventStream.value;
  if (!stream || !Array.isArray(stream)) return [];
  const result = buildFullEventList(stream);
  const hidden = hiddenThinkingEventIds.value;
  return result.filter((e: any) => {
    if (e.type === 'answer' || e.type === 'agent_complete') return false;
    if (e.type === 'thinking' && e.event_id && hidden.has(e.event_id)) return false;
    return true;
  });
});

const visibleIntermediateEvents = computed(() => {
  return intermediateEvents.value.filter((e: any) => {
    if (!e) return false;
    if (e.type === 'thinking') return false;
    if (e.type === 'tool_call' && e.tool_name === 'thinking') return false;
    // 折叠树只保留问题理解 / 知识检索 / 思考 / 思考工具调用四种轻量节点；
    // plan_task_change、tool_call（非思考）已经有原生的 PlanStatus / SearchResults 汇总，
    // 不需要在折叠树里再复制一份。
    if (e.type === 'plan_task_change') return false;
    if (e.type === 'tool_call') return false;
    return true;
  });
});

// Events to display (non-tree: before answer starts show all, after answer starts show only answer)
const displayEvents = computed(() => {
  const stream = eventStream.value;
  if (!stream || !Array.isArray(stream)) {
    return [];
  }

  const result = buildFullEventList(stream);

  // Quick-answer RAG: pipeline steps and model thinking live in RagPipelineProgress;
  // here we only render the final answer stream.
  if (props.ragMode) {
    return result.filter((e: any) => e.type === 'answer');
  }

  // While the conversation is still running, keep the same lightweight tool-log
  // surface as the completed tree. Raw thinking narration is noisy during
  // streaming; the active state is represented by the compact activity dots.
  if (!isConversationDone.value) {
    return result.filter((e: any) => {
      if (e.type === 'thinking') return false;
      if (e.type === 'tool_call' && e.tool_name === 'thinking') return false;
      // 流式 timeline 不重复渲染折叠树里已经有的 plan_task_change / tool_call。
      if (e.type === 'plan_task_change') return false;
      if (e.type === 'tool_call') return false;
      return true;
    });
  }

  // Done: the steps live in the collapsed tree; show only the answer here.
  const answerEvents = result.filter((e: any) => e.type === 'answer');
  if (answerEvents.length > 0) {
    return answerEvents;
  }

  // If the intermediate-steps tree is active, all thinking/tool_call events
  // are already rendered there. Showing anything else here would duplicate
  // them. This covers both the user-stopped case and any completion path
  // that didn't produce an answer event.
  if (shouldShowCollapsedSteps.value) {
    return [];
  }

  // Fallback: if no answer events, show last thinking (legacy compatibility)
  const final = finalContent.value;
  if (!final) {
    return result;
  }

  if (final.type === 'thinking') {
    // The agent loop ended via natural-stop (the model wrote its answer as
    // free text). Synthesize a virtual
    // `answer` event from the trailing thinking content so it renders with
    // the answer card UI (expanded markdown + copy/add toolbar) rather than
    // the collapsed "思考" card. The original thinking event is still in
    // the intermediate-steps tree when applicable.
    const thinking = result.find((e: any) =>
      e.type === 'thinking' && e.event_id === final.event_id
    );
    if (!thinking || !thinking.content) return result;
    return [{
      type: 'answer',
      event_id: thinking.event_id,
      content: thinking.content,
      done: true,
      _promoted_from_thinking: true,
    }];
  }

  return result;
});

// Get unique key for event
const getEventKey = (event: any, index: number): string => {
  if (!event) return `event-${index}`;
  if (event.event_id) return `event-${event.event_id}`;
  if (event.tool_call_id) return `tool-${event.tool_call_id}`;
  if (event.type === 'tool_approval_required' && event.pending_id) {
    return `approval-${event.pending_id}`;
  }

  return `event-${index}-${event.type || 'unknown'}`;
};

const toggleIntermediateSteps = () => {
  showIntermediateSteps.value = !showIntermediateSteps.value;
  nextTick(async () => {
    if (rootElement.value) {
      await hydrateProtectedFileImages(rootElement.value);
    }
  });
};

const toggleEvent = (eventId: string) => {
  if (expandedEvents.value.has(eventId)) {
    expandedEvents.value.delete(eventId);
  } else {
    expandedEvents.value.add(eventId);
  }
};

const handleActionHeaderClick = (event: any) => {
  if (canOpenToolReferences(event)) {
    openToolReferences(event);
    return;
  }
  if (hasExpandableResults(event) && event.tool_call_id) {
    toggleEvent(event.tool_call_id);
  }
};

const handleActionCardClick = (event: any) => {
  if (!canOpenToolReferences(event)) return;
  openToolReferences(event);
};

const isEventExpanded = (eventId: string): boolean => {
  return expandedEvents.value.has(eventId);
};

const isReferenceDrawerTool = (toolName?: string | null): boolean =>
  toolName === 'search_knowledge' ||
  toolName === 'knowledge_search' ||
  toolName === 'grep_chunks' ||
  toolName === 'list_knowledge_chunks';

const hasExpandableResults = (event: any): boolean => {
  if (isReferenceDrawerTool(event?.tool_name)) return false;
  return hasResults(event);
};

const hasActionResult = (event: any): boolean => canOpenToolReferences(event) || hasExpandableResults(event);

// Check if search/grep tools have results
const hasResults = (event: any): boolean => {
  if (!event || !event.tool_data) return true; // Default to true for other tools

  const toolName = event.tool_name;

  // For knowledge search tools
  if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    const count = event.tool_data.results?.length || event.tool_data.count || 0;
    return count > 0;
  }

  // For grep tools
  if (toolName === 'grep_chunks') {
    const totalMatches = event.tool_data.total_matches || 0;
    const resultCount = event.tool_data.result_count || 0;
    return totalMatches > 0 || resultCount > 0;
  }

  // list_knowledge_chunks: summary is inline below the header (no expandable body)
  if (toolName === 'list_knowledge_chunks') {
    return false;
  }

  // For other tools, always allow expansion
  return true;
};

const onRootClick = (e: Event) => {
  const target = e.target as HTMLElement;
  if (!target) return;

  // Handle image clicks -> open preview (only for images inside markdown/answer content, not icons)
  if (target.tagName === 'IMG') {
    const imgEl = target as HTMLImageElement;
    if (imgEl.closest('.markdown-content') || imgEl.closest('.answer-content')) {
      const src = imgEl.getAttribute('src');
      if (src) {
        e.preventDefault();
        e.stopPropagation();
        openImagePreview(src);
        return;
      }
    }
  }

  // Handle KB citation clicks -> open references drawer when available
  const kbEl = target.closest?.('.citation-kb') as HTMLElement | null;
  if (kbEl && kbEl.getAttribute('data-chunk-id')) {
    e.preventDefault();
    e.stopPropagation();
    const rawChunkId = kbEl.getAttribute('data-chunk-id') || '';
    const title = kbEl.getAttribute('data-doc') || '';
    const kbId = kbEl.getAttribute('data-kb-id') || '';
    const chunkId =
      resolveCitationChunkId(
        rawChunkId,
        { doc: title, kbId },
        props.session?.knowledge_references,
      ) || rawChunkId;
    openReferencesDrawer({ chunkId });
    return;
  }

};

const onRootKeydown = (e: KeyboardEvent) => {
  const target = e.target as HTMLElement;
  if (!target) return;

  // Handle KB citation keyboard -> open references drawer when available
  const kbEl = target.closest?.('.citation-kb') as HTMLElement | null;
  if (kbEl) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const rawChunkId = kbEl.getAttribute('data-chunk-id') || '';
      const title = kbEl.getAttribute('data-doc') || '';
      const kbId = kbEl.getAttribute('data-kb-id') || '';
      const chunkId =
        resolveCitationChunkId(
          rawChunkId,
          { doc: title, kbId },
          props.session?.knowledge_references,
        ) || rawChunkId;
      openReferencesDrawer({ chunkId });
    }
    return;
  }

};

onMounted(() => {
  nextTick(async () => {
    const root = rootElement.value;
    if (!root) return;
    root.addEventListener('click', onRootClick, true);
    const keydownListener: EventListener = (evt: Event) => onRootKeydown(evt as KeyboardEvent);
    (root as any).__citationKeydown__ = keydownListener;
    root.addEventListener('keydown', keydownListener, true);
    rebindCitations();
    await hydrateProtectedFileImages(rootElement.value);
  });
});

onBeforeUnmount(() => {
  const root = rootElement.value;
  if (!root) return;
  root.removeEventListener('click', onRootClick, true);
  const keydownListener: EventListener | undefined = (root as any).__citationKeydown__;
  if (keydownListener) {
    root.removeEventListener('keydown', keydownListener, true);
    delete (root as any).__citationKeydown__;
  }
});

onUpdated(() => {
  nextTick(async () => {
    rebindCitations();
    // Hydrate protected-file images (e.g. local:// exports) as soon as the
    // typewriter reveals their <img> into the DOM, so they show in real time
    // mid-stream instead of waiting for the turn to finish. Hydration is cheap
    // and idempotent: blob results are cached per URL, in-flight fetches are
    // de-duped, and failures back off for a cooldown — so a not-yet-ready file
    // simply retries later (and the answerFullyRendered pass is the backstop).
    await hydrateProtectedFileImages(rootElement.value);
  });
});

// 自定义渲染器 - 支持 Mermaid
const agentRenderer = new marked.Renderer();
agentRenderer.code = createMermaidCodeRenderer('mermaid-agent');

const prepareAgentMarkdown = (markdown: string, cachedSvgHtml?: string | null): string => {
  const mermaidSafe = !isConversationDone.value
    ? prepareStreamingMermaidMarkdown(markdown, cachedSvgHtml ?? streamingMermaidSvgCache.value)
    : replaceIncompleteMermaidWithPlaceholder(markdown);
  return mermaidSafe.replace(/<(?:kb|web)\b[^>]*$/i, '');
};

const renderAgentMarkdown = (
  content: unknown,
  escapeMarkdown: (markdown: string) => string,
): string => {
  const contentStr = typeof content === 'string' ? content : String(content || '');
  if (!contentStr.trim()) return '';

  return renderChatMarkdown(contentStr, {
    renderer: agentRenderer,
    escapeMarkdown,
    sanitizeHtml: sanitizeMarkdownHTML,
    streaming: !isConversationDone.value,
    knowledgeReferences: props.session?.knowledge_references,
    cachedMermaidSvgHtml: streamingMermaidSvgCache.value,
    prepareMarkdown: prepareAgentMarkdown,
    injectCachedMermaidSvg,
  });
};

// 单次渲染 Markdown 内容（替代 token-by-token，修复 KaTeX 公式在 streaming 时闪烁消失的问题）
const renderMarkdownContent = (content: unknown): string => {
  return renderAgentMarkdown(content, sanitizeForDisplay);
};

// Renders an answer event's content. Strips final-answer wrappers
// (e.g. <answer>…</answer>, "Final Answer:") that some models wrap their
// plain-text answer in, then delegates to the standard markdown renderer.
const renderAnswerContent = (content: unknown): string => {
  const contentStr = typeof content === 'string' ? content : String(content || '');
  return renderMarkdownContent(unwrapFinalAnswerWrappers(contentStr));
};

// Legacy Markdown rendering function (kept for summaries)
const renderMarkdown = (content: unknown): string => {
  const contentStr = typeof content === 'string' ? content : String(content || '');
  if (!contentStr.trim()) return '';

  try {
    return renderAgentMarkdown(content, (text) => text);
  } catch (e) {
    console.error('Markdown rendering error:', e, 'Content:', contentStr.substring(0, 100));
    return contentStr.replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }
};

// 渲染 Mermaid 图表的函数
const renderMermaidDiagrams = async () => {
  await enhanceMarkdownContainer(rootElement.value);
};

// Tool summary - extract key info to display externally
const getToolSummary = (event: any): string => {
  if (!event || event.pending || !event.success) return '';

  const toolName = event.tool_name;
  const toolData = event.tool_data;

  // For search tools, don't return summary here - it will be displayed in SearchResults component
  if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    return '';
  } else if (toolName === 'get_document_info') {
    if (toolData?.title) {
      return t('agentStream.toolSummary.getDocument', { title: toolData.title });
    }
  } else if (toolName === 'list_knowledge_chunks') {
    if (toolData?.faq_question) {
      return t('agentStream.toolSummary.listFaqEntry', { question: toolData.faq_question });
    }
    if (toolData?.fetched_chunks !== undefined) {
      const title = toolData?.knowledge_title || toolData?.knowledge_id || t('agentStream.toolSummary.document');
      return t('agentStream.toolSummary.listChunks', { title, fetched: toolData.fetched_chunks, total: toolData.total_chunks ?? '?' });
    }
  } else if (toolName === 'todo_write') {
    // Extract steps from tool data
    const steps = toolData?.steps;
    if (Array.isArray(steps)) {
      const inProgress = steps.filter((s: any) => s.status === 'in_progress').length;
      const pending = steps.filter((s: any) => s.status === 'pending').length;
      const completed = steps.filter((s: any) => s.status === 'completed').length;

      const parts = [];
      if (inProgress > 0) parts.push(`🚀 ${t('agentStream.plan.inProgress')} ${inProgress}`);
      if (pending > 0) parts.push(`📋 ${t('agentStream.plan.pending')} ${pending}`);
      if (completed > 0) parts.push(`✅ ${t('agentStream.plan.completed')} ${completed}`);

      return parts.join(' · ');
    }
  } else if (toolName === 'thinking') {
    // Return truthy value to trigger rendering, actual content rendered in template
    return toolData?.thought ? t('agentStream.toolSummary.deepThinking') : '';
  }

  return '';
};

// Get plan status parts for todo_write tool header
const getPlanStatusParts = (event: any) => {
  if (!event || !event.tool_data?.steps) {
    return { inProgress: 0, pending: 0, completed: 0 };
  }

  const steps = event.tool_data.steps;
  if (!Array.isArray(steps)) {
    return { inProgress: 0, pending: 0, completed: 0 };
  }

  return {
    inProgress: steps.filter((s: any) => s.status === 'in_progress').length,
    pending: steps.filter((s: any) => s.status === 'pending').length,
    completed: steps.filter((s: any) => s.status === 'completed').length
  };
};

// Get plan status items for display with icons
const getPlanStatusItems = (event: any) => {
  const parts = getPlanStatusParts(event);
  const items: Array<{ icon: string; class: string; label: string; count: number }> = [];

  if (parts.inProgress > 0) {
    items.push({
      icon: 'play-circle-filled',
      class: 'in-progress',
      label: t('agentStream.plan.inProgress'),
      count: parts.inProgress
    });
  }

  if (parts.pending > 0) {
    items.push({
      icon: 'time',
      class: 'pending',
      label: t('agentStream.plan.pending'),
      count: parts.pending
    });
  }

  if (parts.completed > 0) {
    items.push({
      icon: 'check-circle-filled',
      class: 'completed',
      label: t('agentStream.plan.completed'),
      count: parts.completed
    });
  }

  return items;
};

// Get plan status summary for todo_write tool header (deprecated, use getPlanStatusParts instead)
const getPlanStatusSummary = (event: any): string => {
  const parts = getPlanStatusParts(event);
  const textParts = [];
  if (parts.inProgress > 0) textParts.push(`🚀 ${t('agentStream.plan.inProgress')} ${parts.inProgress}`);
  if (parts.pending > 0) textParts.push(`📋 ${t('agentStream.plan.pending')} ${parts.pending}`);
  if (parts.completed > 0) textParts.push(`✅ ${t('agentStream.plan.completed')} ${parts.completed}`);
  return textParts.length > 0 ? textParts.join(' · ') : '';
};

/** Render SVG assets in the channel / brand color via CSS mask. */
function maskIconStyle(src: string, size = 18): Record<string, string> {
  if (!src) return {}
  const url = `url("${src}")`
  return {
    width: `${size}px`,
    height: `${size}px`,
    WebkitMaskImage: url,
    maskImage: url,
  }
}

// Get search results summary text (returns HTML with colored numbers)
const getSearchResultsSummary = (event: any): string => {
  if (!event || !event.tool_data) return '';

  const toolData = event.tool_data;
  const count = Number(toolData.results?.length ?? toolData.count ?? 0) || 0;
  if (count === 0) return t('agentStream.search.noResults');

  // Build summary text
  let summary = '';
  const kbCount = toolData.kb_counts ? Object.keys(toolData.kb_counts).length : 0;
  if (kbCount > 0) {
    summary = t('agentStream.search.foundResultsFromFiles', { count: `<strong>${count}</strong>`, files: `<strong>${kbCount}</strong>` });
  } else {
    summary = t('agentStream.search.foundResults', { count: `<strong>${count}</strong>` });
  }
  return summary;
};

// Get grep results summary text (returns HTML with colored numbers)
const getGrepResultsSummary = (toolData: any): string => {
  if (!toolData) return '';

  const totalChunks = Number(toolData.total_matches ?? 0) || 0;
  const docCount = countGrepDocuments(toolData);

  if (totalChunks === 0) {
    return t('agentStream.search.noResults');
  }

  return t('agentStream.search.grepSummary', {
    chunks: `<strong>${totalChunks}</strong>`,
    docs: `<strong>${docCount}</strong>`,
  });
};

const getKnowledgeChunksSummary = (toolData: any): string => {
  return getKnowledgeChunksSummaryHtml(t, toolData);
};

// Get tool title - prefer summary over description, add query for search tools
const getToolTitle = (event: any): string => {
  if (event.pending) {
    if (event.tool_name === 'image_analysis') {
      return t('agentStream.toolStatus.imageAnalyzing');
    }
    const localizedName = getLocalizedToolName(event.tool_name);
    return t('agentStream.toolStatus.calling', { name: localizedName });
  }

  const toolName = event.tool_name;
  const isSearchTool = toolName === 'search_knowledge' || toolName === 'knowledge_search';
  const isGrepTool = toolName === 'grep_chunks';

  // For search tools, use description with query text
  if (isSearchTool) {
    const baseTitle = getToolDescription(event);
    const queryText =
      getQueryText(event.arguments) ||
      getQueryText(event.tool_data);
    if (queryText) {
      return `${baseTitle}：「${queryText}」`;
    }
    return baseTitle;
  }

  // For grep tools, use description with patterns
  if (isGrepTool) {
    const baseTitle = getToolDescription(event);
    // Try to get patterns from arguments or tool_data
    let patterns: string[] = [];
    if (event.arguments && typeof event.arguments === 'object') {
      if (Array.isArray(event.arguments.queries)) {
        patterns = event.arguments.queries;
      } else if (Array.isArray(event.arguments.patterns)) {
        patterns = event.arguments.patterns;
      } else if (event.arguments.query) {
        patterns = [event.arguments.query];
      } else if (event.arguments.pattern) {
        patterns = [event.arguments.pattern];
      }
    } else if (event.tool_data) {
      if (Array.isArray(event.tool_data.queries)) {
        patterns = event.tool_data.queries;
      } else if (Array.isArray(event.tool_data.patterns)) {
        patterns = event.tool_data.patterns;
      } else if (event.tool_data.query) {
        patterns = [event.tool_data.query];
      } else if (event.tool_data.pattern) {
        patterns = [event.tool_data.pattern];
      }
    }
    if (patterns.length > 0) {
      // Show up to 2 patterns in title
      const displayPatterns = patterns.slice(0, 2);
      const patternText = displayPatterns.join('、');
      const moreText = patterns.length > 2 ? ` +${patterns.length - 2}` : '';
      return `${baseTitle}：「${patternText}${moreText}」`;
    }
    return baseTitle;
  }

  // Use tool summary if available
  const summary = getToolSummary(event);
  return summary || getToolDescription(event);
};

// Tool description
const getToolDescription = (event: any): string => {
  if (event.pending) {
    if (event.tool_name === 'image_analysis') {
      return t('agentStream.toolStatus.imageAnalyzing');
    }
    if (event.tool_name === 'query_understand') {
      return t('agentStream.toolStatus.queryUnderstanding');
    }
    const localizedName = getLocalizedToolName(event.tool_name);
    return t('agentStream.toolStatus.calling', { name: localizedName });
  }

  const success = event.success === true;
  const toolName = event.tool_name;

  if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    return success ? t('agentStream.toolStatus.searchKb') : t('agentStream.toolStatus.searchKbFailed');
  } else if (toolName === 'grep_chunks') {
    return success ? t('agentStream.toolStatus.grepSearch') : t('agentStream.toolStatus.grepSearchFailed');
  } else if (toolName === 'get_document_info') {
    return success ? t('agentStream.toolStatus.getDocInfo') : t('agentStream.toolStatus.getDocInfoFailed');
  } else if (toolName === 'get_document_content') {
    return success ? t('agentStream.toolStatus.viewDocument') : t('agentStream.toolStatus.calledFailed', { name: t('agentStream.toolStatus.viewDocument') });
  } else if (toolName === 'thinking') {
    return success ? t('agentStream.toolStatus.thinkingDone') : t('agentStream.toolStatus.thinkingFailed');
  } else if (toolName === 'todo_write') {
    return success ? t('agentStream.toolStatus.updateTodos') : t('agentStream.toolStatus.updateTodosFailed');
  } else if (toolName === 'image_analysis') {
    return success ? t('agentStream.toolStatus.imageAnalysisDone') : t('agentStream.toolStatus.imageAnalysisFailed');
  } else if (toolName === 'query_understand') {
    return success ? t('agentStream.toolStatus.queryUnderstandDone') : t('agentStream.toolStatus.calledFailed', { name: getLocalizedToolName(toolName) });
  } else {
    const localizedName = getLocalizedToolName(toolName);
    return success ? t('agentStream.toolStatus.called', { name: localizedName }) : t('agentStream.toolStatus.calledFailed', { name: localizedName });
  }
};

// Helper functions
const formatDuration = (ms?: number): string => {
  if (!ms) return '0s';
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds}s`;
};

const formatJSON = (obj: any): string => {
  try {
    if (typeof obj === 'string') {
      // Try to parse if it's a JSON string
      try {
        const parsed = JSON.parse(obj);
        return JSON.stringify(parsed, null, 2);
      } catch {
        return obj;
      }
    }
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
};

// Helper function to get actual content (from answer or last thinking).
// Strips final-answer wrappers (e.g. <answer>…</answer>, "Final Answer:")
// so callers like copy and add-to-knowledge get clean text.
const getActualContent = (answerEvent: any): string => {
  // First try to get content from answer event
  const answerContent = (answerEvent?.content || '').trim();
  if (answerContent) {
    return unwrapFinalAnswerWrappers(answerContent).trim();
  }

  // If answer is empty, try to get from last thinking
  const stream = eventStream.value;
  if (stream && Array.isArray(stream)) {
    const thinkingEvents = stream.filter((e: any) => e.type === 'thinking' && e.content && e.content.trim());
    if (thinkingEvents.length > 0) {
      const lastThinking = thinkingEvents[thinkingEvents.length - 1];
      return unwrapFinalAnswerWrappers((lastThinking.content || '').trim()).trim();
    }
  }

  return '';
};

const copyingAnswer = ref(false);
const exportingAnswer = ref(false);

const loadCitationChunkContentForCopy = async (chunkId: string): Promise<string> => {
  const scope = props.sessionId || props.session?.id || 'default';
  const cached = getCitationChunkCache(scope, chunkId);
  if (cached?.content) return cached.content;

  try {
    const response = await getChunkByIdOnly(chunkId);
    const content = String(response?.data?.content || '').trim();
    if (!content) {
      const error = t('agentStream.citation.notFound');
      setCitationChunkCache(scope, chunkId, { content: '', error });
      throw new Error(error);
    }
    setCitationChunkCache(scope, chunkId, { content });
    return content;
  } catch (error) {
    if (!getCitationChunkCache(scope, chunkId)?.error) {
      setCitationChunkCache(scope, chunkId, {
        content: '',
        error: t('agentStream.citation.loadFailed'),
      });
    }
    throw error;
  }
};

// 「重新生成」：把原始问题回填到输入框，由父级 chat/index.vue 调用
// inputFieldRef.value?.triggerSend(query) 落地。
const handleRegenerate = () => {
  const query = (props.userQuery || '').trim();
  if (!query) return;
  emit('regenerate', query);
};

const handleCopyAnswer = async (answerEvent: any) => {
  if (copyingAnswer.value) return;

  const content = getActualContent(answerEvent);
  if (!content) {
    MessagePlugin.warning(t('agentStream.copy.emptyContent'));
    return;
  }

  try {
    copyingAnswer.value = true;
    const result = await buildCitationEnrichedCopyText(
      content,
      props.session?.knowledge_references,
      loadCitationChunkContentForCopy,
    );
    // 剔除内联引用标签（<kb/>、<web/>）和追加的 <kb_chunk_contents> 参考块，
    // 只把干净的纯文本答案放进剪贴板。
    await copyTextToClipboard(stripCitationTagsForCopy(result.text));
    if (result.failedChunkIds.length) {
      MessagePlugin.warning(t('agentStream.copy.partialSuccess', {
        count: result.failedChunkIds.length,
      }));
    } else {
      MessagePlugin.success(t('agentStream.copy.success'));
    }
  } catch (err) {
    console.error('Copy failed:', err);
    MessagePlugin.error(t('agentStream.copy.failed'));
  } finally {
    copyingAnswer.value = false;
  }
};

const downloadHtmlFile = (html: string, fileName: string) => {
  const blobUrl = URL.createObjectURL(new Blob([html], { type: 'text/html;charset=utf-8' }));
  const link = document.createElement('a');
  link.href = blobUrl;
  link.download = fileName;
  link.style.display = 'none';
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(blobUrl), 1000);
};

const handleExportAnswerHtml = async (answerEvent: any) => {
  if (exportingAnswer.value) return;

  const content = getActualContent(answerEvent);
  if (!content) {
    MessagePlugin.warning(t('agentStream.copy.emptyContent'));
    return;
  }

  const question = String(props.userQuery || '').trim();
  const previewWindow = window.open('', '_blank');

  try {
    exportingAnswer.value = true;
    const chunks = await resolveCitationChunks(
      content,
      props.session?.knowledge_references,
      loadCitationChunkContentForCopy,
    );
    const html = buildAgentAnswerExportHtml({
      title: question || t('agentStream.exportHtml.defaultTitle'),
      answerHtml: renderAnswerContent(content),
      chunks,
      labels: {
        references: t('agentStream.exportHtml.references'),
        close: t('agentStream.exportHtml.close'),
        unavailable: t('agentStream.exportHtml.unavailable'),
      },
      locale: String(i18n.global.locale.value || 'zh-CN'),
    });
    const fileName = buildAgentAnswerExportFileName(question);

    if (previewWindow) {
      previewWindow.document.open();
      previewWindow.document.write(html);
      previewWindow.document.close();
    }
    downloadHtmlFile(html, fileName);

    const failedCount = chunks.filter((chunk) => chunk.failed).length;
    if (failedCount) {
      MessagePlugin.warning(t('agentStream.exportHtml.partialSuccess', { count: failedCount }));
    } else {
      MessagePlugin.success(t('agentStream.exportHtml.success'));
    }
  } catch (error) {
    previewWindow?.close();
    console.error('HTML export failed:', error);
    MessagePlugin.error(t('agentStream.exportHtml.failed'));
  } finally {
    exportingAnswer.value = false;
  }
};

// ----------------------------------------------------------------
// 点赞 / 点踩反馈
// ----------------------------------------------------------------
// 当前回答（每条消息实例对应一个组件）所收到的反馈状态：
//   null       —— 未表态
//   'like'     —— 已点赞
//   'dislike'  —— 已点踩（已提交）
// 占位：本期未连通后端反馈接口（见 src/api/chat/feedback.ts），仅在
// 本地记录用户选择，避免重复提交；后续接口 ready 时可直接沿用。
const currentFeedback = ref<FeedbackRating | null>(null);
const feedbackSubmitting = ref(false);

// 点踩弹窗显隐 & 当前正在打分的 message_id
const dislikeDialogVisible = ref(false);
const dislikeDialogMessageId = ref<string | undefined>(undefined);

/** 历史消息加载 / 切换会话时，从 session.feedback 同步按钮初始态；
 *  SessionData 有索引签名，这里用 (props.session as any) 兜住未知键。
 *  后续若在 SessionData 正式加入 feedback 字段，可去掉断言。 */
watch(
  () => (props.session as any)?.feedback,
  (fb) => {
    if (fb && (fb.rating === 'like' || fb.rating === 'dislike')) {
      currentFeedback.value = fb.rating
    } else {
      currentFeedback.value = null
    }
  },
  { immediate: true },
)

// 成功提交后给 icon 一个短暂的"pop"动效：
// - pulseRating 在动画结束前会被清空，保证连点或切换时也能再次触发
// - 动画 600ms，时长与下面 CSS keyframes 对齐
const pulseRating = ref<FeedbackRating | null>(null);
let pulseTimer: ReturnType<typeof setTimeout> | null = null
const triggerPulse = (rating: FeedbackRating) => {
  pulseRating.value = rating
  if (pulseTimer) clearTimeout(pulseTimer)
  pulseTimer = setTimeout(() => {
    pulseRating.value = null
    pulseTimer = null
  }, 600)
}
onBeforeUnmount(() => {
  if (pulseTimer) clearTimeout(pulseTimer)
})

/** 从 answerEvent 上尝试解析 message_id（不同上游结构兜底）。 */
const resolveMessageIdForFeedback = (answerEvent: any): string | undefined => {
  if (!answerEvent || typeof answerEvent !== 'object') return undefined
  const candidates = [
    answerEvent.message_id,
    answerEvent.messageId,
    answerEvent.id,
    props.session?.id,
  ]
  for (const v of candidates) {
    if (typeof v === 'string' && v.length) return v
  }
  return undefined
}

/** 点赞：再点切换（PUT 提交/覆盖、DELETE 取消）；接口成功后样式才更新。
 *  - 当前是 'like' → DELETE 取消，currentFeedback 置 null（不做 pop 动效，避免取消也有"弹一下"的正向语义）
 *  - 当前不是 'like' → PUT 提交（同时覆盖 dislike 场景） */
const handleLike = async (answerEvent: any) => {
  if (feedbackSubmitting.value) return

  const sessionId = (props.session?.session_id as string) ?? ''
  const messageId = resolveMessageIdForFeedback(answerEvent) ?? ''

  feedbackSubmitting.value = true
  try {
    if (currentFeedback.value === 'like') {
      // 取消点赞：DELETE 语义；成功后由调用方自行清空本地状态
      const result = await cancelMessageFeedback({ session_id: sessionId, message_id: messageId })
      if (!result.success) {
        MessagePlugin.error(result.message || t('agentStream.feedback.dialog.submitFailed'))
        return
      }
      currentFeedback.value = null
    } else {
      // 提交 / 覆盖点赞：PUT 语义
      const result = await submitMessageFeedback({
        session_id: sessionId,
        message_id: messageId,
        rating: 'like',
      })
      // ★ 关键：仅在接口成功后才落样式 & 触发动效
      if (!result.success) {
        MessagePlugin.error(result.message || t('agentStream.feedback.dialog.submitFailed'))
        return
      }
      // 用后端回传的 rating 作为权威值（PUT 覆盖语义）
      currentFeedback.value = result.data?.rating ?? 'like'
      triggerPulse('like')
    }
  } catch (err) {
    console.error('[feedback] like submit failed:', err)
    MessagePlugin.error(t('agentStream.feedback.dialog.submitFailed'))
  } finally {
    feedbackSubmitting.value = false
  }
}

/** 点踩：再点切换。
 *  - 当前是 'dislike' → DELETE 直接取消（不开弹窗：用户已表达过点踩意图，再点 = 收回）
 *  - 当前不是 'dislike' → 打开弹窗收集原因；用户点确认后由弹窗内部 submit，
 *    最终由 handleFeedbackSubmitted 决定是否落样式。 */
const openDislikeDialog = async (answerEvent: any) => {
  if (feedbackSubmitting.value) return

  const sessionId = (props.session?.session_id as string) ?? ''
  const messageId = resolveMessageIdForFeedback(answerEvent) ?? ''

  if (currentFeedback.value === 'dislike') {
    feedbackSubmitting.value = true
    try {
      const result = await cancelMessageFeedback({ session_id: sessionId, message_id: messageId })
      if (!result.success) {
        MessagePlugin.error(result.message || t('agentStream.feedback.dialog.submitFailed'))
        return
      }
      currentFeedback.value = null
    } catch (err) {
      console.error('[feedback] dislike cancel failed:', err)
      MessagePlugin.error(t('agentStream.feedback.dialog.submitFailed'))
    } finally {
      feedbackSubmitting.value = false
    }
    return
  }

  dislikeDialogMessageId.value = messageId
  dislikeDialogVisible.value = true
}

/** 弹窗提交成功后的统一收尾。
 *  失败或 data 缺失时一律不更新 currentFeedback、不触发 pop；
 *  后端未回传 record 时按弹窗语义回退到 'dislike'，与 handleLike 的 fallback 逻辑保持对称。 */
const handleFeedbackSubmitted = (result: FeedbackSubmitResult) => {
  if (!result.success) return
  // 用后端回传的 rating 作为权威值（PUT 覆盖语义）；
  // 后端未回传 record 时回退到 'dislike'（弹窗场景下唯一可能的 rating）。
  const rating = result.data?.rating ?? 'dislike'
  currentFeedback.value = rating
  triggerPulse(rating)
}
</script>

<style lang="less" scoped>
@import '../../../components/css/chat-markdown.less';
@import '../../../components/css/chat-message-shared.less';
@import '../../../components/css/chat-citations.less';
@import '../../../components/css/chat-timeline-loading.less';

.agent-stream-display {
  display: flex;
  flex-direction: column;
  gap: 0;
  margin-bottom: 10px;
  position: relative;

  // 点赞 / 点踩按钮：在 .t-button 既有 outline 风格基础上覆盖 icon 颜色，
  // 选中态用品牌色突出，未选中态用中性色。
  .feedback-btn {

    // 让 thumb-up / thumb-down 图标颜色随 active 状态切换
    :deep(.t-icon) {
      transition: color 0.15s ease;
    }

    &.is-active {
      :deep(.t-icon) {
        color: var(--td-brand-color);
      }

      border-color: color-mix(in srgb, var(--td-brand-color) 35%, transparent);
      background-color: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
    }
  }

  .feedback-btn--dislike.is-active {
    :deep(.t-icon) {
      // 点踩单独走 warning 色调，与点赞区分
      color: var(--td-error-color, var(--td-brand-color));
    }

    border-color: color-mix(in srgb, var(--td-error-color, var(--td-brand-color)) 35%, transparent);
    background-color: color-mix(in srgb, var(--td-error-color, var(--td-brand-color)) 6%, transparent);
  }

  // 成功提交后给 icon 一个 pop 动效——通过临时类 is-pulsing 控制，pulseRating
  // 在动画结束后会被清空，从而保证后续再次成功时仍能复触发。
  .feedback-btn.is-pulsing :deep(.t-icon) {
    animation: feedback-icon-pop 600ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  @keyframes feedback-icon-pop {
    0% {
      transform: scale(1) rotate(0deg);
    }

    35% {
      transform: scale(1.4) rotate(-8deg);
    }

    60% {
      transform: scale(0.92) rotate(4deg);
    }

    80% {
      transform: scale(1.08) rotate(-2deg);
    }

    100% {
      transform: scale(1) rotate(0deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .feedback-btn.is-pulsing :deep(.t-icon) {
      animation: none;
    }
  }

  --agent-step-text-size: 14px;
  --agent-step-summary-size: 13px;
  --agent-step-line-color: color-mix(in srgb, var(--td-text-color-primary) 16%, transparent);
  --agent-step-icon-color: var(--td-text-color-placeholder);
  --stream-brand-2: color-mix(in srgb, var(--td-brand-color) 2%, transparent);
  --stream-brand-3: color-mix(in srgb, var(--td-brand-color) 3%, transparent);
  --stream-brand-4: color-mix(in srgb, var(--td-brand-color) 4%, transparent);
  --stream-brand-5: color-mix(in srgb, var(--td-brand-color) 5%, transparent);
  --stream-brand-6: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
  --stream-brand-8: color-mix(in srgb, var(--td-brand-color) 8%, transparent);
  --stream-brand-10: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
  --stream-brand-12: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  --stream-brand-15: color-mix(in srgb, var(--td-brand-color) 15%, transparent);
  --stream-brand-20: color-mix(in srgb, var(--td-brand-color) 20%, transparent);

  &.is-rag-mode {
    margin-top: 0;
  }

}

// Streaming steps container
.streaming-steps-container {
  position: relative;

  &.is-streaming-timeline {
    margin-top: 8px;
  }

  &.streaming-steps-constrained {
    max-height: 400px;
    overflow-y: auto;

    &::-webkit-scrollbar {
      width: 4px;
    }

    &::-webkit-scrollbar-track {
      background: transparent;
    }

    &::-webkit-scrollbar-thumb {
      background: var(--td-bg-color-component-disabled);
      border-radius: 2px;

      &:hover {
        background: var(--td-text-color-placeholder);
      }
    }
  }
}

// Event items (flat, no timeline)
.event-item {
  position: relative;
  margin-bottom: 8px;

  &.event-answer {
    // answer 事件无特殊左侧装饰
  }
}

// ============ Tree View ============
.tree-container {
  margin: 0 0 16px;
  position: relative;
}

.tree-root {
  cursor: pointer;
  color: var(--td-text-color-secondary);
  margin-bottom: 0;
}

.tree-root-summary {
  :deep(strong) {
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.icon-mask {
  display: inline-block;
  flex-shrink: 0;
  background-color: var(--agent-step-icon-color);
  mask-size: contain;
  mask-repeat: no-repeat;
  mask-position: center;
  -webkit-mask-size: contain;
  -webkit-mask-repeat: no-repeat;
  -webkit-mask-position: center;
}

.tree-children {
  position: relative;
  padding-left: 0;
  margin-top: 14px;
  margin-left: 10px;
  max-height: none;
  overflow-y: visible;
  border-left: 0;
}

.tree-child {
  position: relative;
  padding-left: 42px;
  padding-bottom: 0;
  margin-bottom: 18px;

  // vertical trunk line (continues for non-last children)
  // bottom: -6px extends the line through the margin-bottom gap between siblings
  &::before {
    content: '';
    position: absolute;
    left: 9px;
    top: 22px;
    bottom: -18px;
    width: 0;
    border-left: 1px solid var(--agent-step-line-color);
  }

  // horizontal branch connector
  .tree-branch {
    display: none;
  }

  // last child: vertical line only goes to the branch, then stops
  &.tree-child-last {
    margin-bottom: 0;

    &::before {
      content: none;
    }
  }
}

.tree-child-content {
  // child content area
}

// Thinking detail content (inside action-details)
.thinking-detail-content {
  padding: 7px 0 0 30px;
  font-size: var(--agent-step-summary-size);
  color: var(--td-text-color-secondary);
  line-height: 1.6;
  max-height: none;
  overflow-y: visible;

  &.markdown-content {
    // Compact thinking-panel Markdown is intentionally not the chat answer body.
    // Answer Markdown typography belongs to chat-markdown.less.
    .chat-citation-pills();
  }
}

// Answer Event - 无边框，直接显示内容
.answer-event {
  animation: fadeInUp 0.25s ease-out;
  min-height: 20px;

  .fallback-icon-btn {
    color: var(--td-text-color-disabled) !important;
    border-color: var(--td-component-stroke) !important;

    &:hover {
      color: var(--td-text-color-placeholder) !important;
      border-color: var(--td-component-border) !important;
    }
  }

  .answer-content {
    &.markdown-content {
      // Chat Markdown visual styles are centralized in chat-markdown.less.
      // Do not add element-level Markdown rules here; update the shared mixin.
      .chat-markdown-typography();
      .chat-citation-pills();

      :deep(img) {
        background-color: var(--td-bg-color-secondarycontainer);
        /* 加载时的占位背景色 */
      }
    }
  }

  .answer-toolbar {
    margin-top: 10px;
  }

  .answer-references-btn {
    border-left: 1px solid var(--td-gray-color-2);
    color: var(--td-text-color-secondary);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    margin-left: 10px;

    span {
      padding-left: 20px;
      padding-right: 10px;
    }

    &:hover {
      color: var(--td-text-color-primary);
    }
  }
}

// Tool Event
.tool-event {
  animation: fadeInUp 0.25s ease-out;

  .action-card {
    background: transparent;
    border-radius: 0;
    border: 0;
    border-left: 0;
    overflow: visible;
    position: relative;
    transition: border-color 0.2s ease;
    box-shadow: none;

    >* {
      position: relative;
      z-index: 1;
    }

    &:hover {
      background: transparent;
    }

    &.reference-trigger {
      cursor: pointer;

      &:hover {

        .action-name,
        .results-summary-text {
          color: var(--td-text-color-primary);
        }
      }
    }

    &.action-error {
      color: var(--td-error-color);
    }

    &.action-pending {
      opacity: 1;
      box-shadow: none;
      background: transparent;
    }
  }

  .tool-summary {
    padding: 6px 12px;
    font-size: 12px;
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-container);
    border-top: 1px solid var(--td-component-stroke);
    line-height: 1.6;
    font-weight: 500;
    animation: slideIn 0.2s ease-out;

    .tool-summary-markdown {
      // Compact tool summaries have local spacing by design; full chat answer
      // Markdown typography belongs to chat-markdown.less.
      font-weight: 400;
      line-height: 1.6;
      color: var(--td-text-color-primary);

      :deep(p) {
        margin: 3px 0;
        color: var(--td-text-color-primary);
      }

      :deep(ul),
      :deep(ol) {
        margin: 3px 0;
        padding-left: 18px;
      }

      :deep(code) {
        background: var(--td-bg-color-secondarycontainer);
        padding: 2px 5px;
        border-radius: 3px;
        font-size: 11px;
        color: var(--td-brand-color);
        font-weight: 500;
      }

      :deep(strong) {
        font-weight: 600;
        color: var(--td-text-color-primary);
      }
    }
  }
}

.action-header {
  display: flex;
  align-items: center;
  padding: 0;
  color: var(--td-text-color-primary);
  font-weight: 400;
  min-height: 24px;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.15s ease;

  &:hover {
    background-color: transparent;
  }

  &.no-results {
    cursor: default;

    &:hover {
      background-color: transparent;
    }
  }
}

.action-title {
  display: flex;
  align-items: center;
  gap: 12px;
  position: relative;
  flex: 1;
  min-width: 0;

  .action-title-icon {
    flex-shrink: 0;

    &.t-icon {
      width: 18px;
      height: 18px;
      color: var(--agent-step-icon-color);
    }
  }

  :deep(.t-tooltip) {
    flex: 0 1 auto;
    min-width: 0;
  }

  .action-show-icon {
    flex-shrink: 0;
    margin-left: 2px;
  }

  .action-name {
    white-space: nowrap;
    font-size: var(--agent-step-text-size);
    line-height: 1.55;
    font-weight: 400;
    color: var(--td-text-color-secondary);
  }

  // Retracted preamble used as the card title: allow it to wrap to its full
  // text (it carries meaning) and use primary text color, while the reasoning
  // body stays in the collapsible details.
  .action-preamble-title {
    white-space: normal;
    word-break: break-word;
    font-size: var(--agent-step-text-size);
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }

  .action-badge {
    display: inline-flex;
    align-items: center;
    padding: 0 6px;
    height: 18px;
    border-radius: 9px;
    background: var(--stream-brand-10);
    color: color-mix(in srgb, var(--td-brand-color) 80%, var(--td-text-color-secondary));
    font-size: 11px;
    font-weight: 500;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .action-summary {
    color: var(--td-text-color-secondary);
    font-size: var(--agent-step-summary-size);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex-shrink: 1;
  }
}


@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes slideInDown {
  from {
    opacity: 0;
    transform: translateY(-8px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateX(-6px);
  }

  to {
    opacity: 1;
    transform: translateX(0);
  }
}

// Loading 动画关键帧
@keyframes dotBounce {

  0%,
  80%,
  100% {
    transform: scale(1);
    opacity: 0.6;
  }

  40% {
    transform: scale(1.3);
    opacity: 1;
  }
}

@keyframes spin {
  0% {
    transform: rotate(0deg);
  }

  100% {
    transform: rotate(360deg);
  }
}

@keyframes pulse {

  0%,
  100% {
    transform: scale(1);
    opacity: 0.8;
  }

  50% {
    transform: scale(1.5);
    opacity: 0.3;
  }
}

@keyframes typingBounce {

  0%,
  60%,
  100% {
    transform: translate3d(0, 0, 0);
  }

  30% {
    transform: translate3d(0, -5px, 0);
  }
}

@keyframes wave {

  0%,
  40%,
  100% {
    transform: scaleY(0.4);
  }

  20% {
    transform: scaleY(1);
  }
}

@keyframes pulseBorder {

  0%,
  100% {
    border-left-color: var(--td-brand-color);
    box-shadow: 0 1px 3px var(--stream-brand-6);
  }

  50% {
    border-left-color: var(--td-brand-color);
    box-shadow: 0 1px 4px var(--stream-brand-12);
  }
}

@keyframes shakeError {

  0%,
  100% {
    transform: translateX(0);
  }

  10%,
  30%,
  50%,
  70%,
  90% {
    transform: translateX(-2px);
  }

  20%,
  40%,
  60%,
  80% {
    transform: translateX(2px);
  }
}

@keyframes actionPendingShimmer {
  0% {
    transform: translateX(-90%);
  }

  50% {
    transform: translateX(-5%);
  }

  100% {
    transform: translateX(90%);
  }
}

.action-name {
  font-size: var(--agent-step-text-size);
  font-weight: 400;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  max-width: 100%;
  vertical-align: middle;
}

.action-show-icon {
  font-size: 12px;
  padding: 0 2px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.action-details {
  padding: 0;
  border-top: 0;
  background: transparent;
  display: flex;
  flex-direction: column;
}

.tool-result-wrapper {
  margin: 0;
}

.search-results-summary-fixed {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 2px 0 0 0;
  background: transparent;
  border-top: 0;

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

.plan-status-summary-fixed {
  padding: 2px 0 0 0;
  background: transparent;
  border-top: 0;

  .plan-status-text {
    font-size: var(--agent-step-summary-size);
    font-weight: 400;
    color: var(--td-text-color-secondary);
    line-height: 1.5;
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;

    .status-icon {
      font-size: 14px;
      flex-shrink: 0;

      &.in-progress {
        color: var(--td-brand-color);
      }

      &.pending {
        color: var(--td-warning-color);
      }

      &.completed {
        color: var(--td-brand-color);
      }
    }

    .separator {
      color: var(--td-text-color-placeholder);
      margin: 0 4px;
    }

    span:not(.separator) {
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
  }
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

.plan-task-change-event {
  min-height: 24px;

  .plan-task-change-card {
    padding: 0;
    background: transparent;
    border-radius: 0;
    border: 0;
    font-size: var(--agent-step-text-size);
    color: var(--td-text-color-secondary);
    line-height: 1.55;

    .plan-task-change-content {
      strong {
        color: var(--td-text-color-secondary);
        font-weight: 400;
        margin-right: 6px;
      }
    }
  }
}

.tool-output-wrapper {
  margin: 10px 0;
  padding: 0 8px;

  .fallback-header {
    display: flex;
    align-items: center;
    margin-bottom: 8px;
    padding: 0 4px;

    .fallback-label {
      font-size: 11px;
      color: var(--td-text-color-secondary);
      font-weight: 500;
      line-height: 1.5;
    }
  }

  .detail-output-wrapper {
    position: relative;
    background: var(--td-bg-color-secondarycontainer);
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    overflow: hidden;
    margin: 0;
    padding: 0;

    .detail-output {
      font-family: var(--app-font-family-mono);
      font-size: 11px;
      color: var(--td-text-color-primary);
      padding: 12px;
      margin: 0;
      white-space: pre-wrap;
      word-break: break-word;
      line-height: 1.6;
      max-height: 400px;
      overflow-y: auto;
      overflow-x: auto;
      background: var(--td-bg-color-container);
      display: block;

      &::-webkit-scrollbar {
        width: 6px;
        height: 6px;
      }

      &::-webkit-scrollbar-track {
        background: var(--td-bg-color-secondarycontainer);
        border-radius: 3px;
      }

      &::-webkit-scrollbar-thumb {
        background: var(--td-bg-color-component-disabled);
        border-radius: 3px;

        &:hover {
          background: var(--td-bg-color-component-disabled);
        }
      }
    }
  }
}

.tool-arguments-wrapper {
  margin-top: 8px;
  padding: 0 10px;
  margin-bottom: 8px;

  .arguments-header {
    margin-bottom: 6px;

    .arguments-label {
      font-size: 12px;
      font-weight: 600;
      color: var(--td-text-color-secondary);
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
  }

  .detail-code {
    font-size: 12px;
    background: var(--td-bg-color-container);
    padding: 10px;
    border-radius: 6px;
    font-family: var(--app-font-family-mono);
    color: var(--td-text-color-primary);
    margin: 0;
    overflow-x: auto;
    border: 1px solid var(--td-component-stroke);
    line-height: 1.5;
  }
}

.loading-indicator {
  display: flex;
  align-items: center;
  min-height: 24px;
  padding: 0;
  margin-top: 0;
  position: relative;
  animation: fadeInUp 0.3s ease-out;

  // 方案1: 三个跳动的圆点
  .loading-dots {
    display: flex;
    align-items: center;
    gap: 6px;

    span {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--td-brand-color);
      animation: dotBounce 1.4s ease-in-out infinite;

      &:nth-child(1) {
        animation-delay: -0.32s;
      }

      &:nth-child(2) {
        animation-delay: -0.16s;
      }

      &:nth-child(3) {
        animation-delay: 0s;
      }
    }
  }

  // 打字机效果
  .loading-typing {
    display: flex;
    align-items: center;
    gap: 4px;

    span {
      width: 4px;
      height: 4px;
      border-radius: 50%;
      background: var(--td-text-color-placeholder);
      animation: typingBounce 1.4s ease-in-out infinite;
      // Composite each dot so the bounce stays smooth and ghost-free while the
      // streaming answer relayouts every token.
      will-change: transform;
      backface-visibility: hidden;

      &:nth-child(1) {
        animation-delay: 0s;
      }

      &:nth-child(2) {
        animation-delay: 0.2s;
      }

      &:nth-child(3) {
        animation-delay: 0.4s;
      }
    }
  }

  // 方案5: 波浪线
  .loading-wave {
    display: flex;
    align-items: center;
    gap: 3px;

    span {
      width: 3px;
      height: 16px;
      background: var(--td-brand-color);
      border-radius: 2px;
      animation: wave 1.2s ease-in-out infinite;

      &:nth-child(1) {
        animation-delay: 0s;
      }

      &:nth-child(2) {
        animation-delay: 0.1s;
      }

      &:nth-child(3) {
        animation-delay: 0.2s;
      }

      &:nth-child(4) {
        animation-delay: 0.3s;
      }

      &:nth-child(5) {
        animation-delay: 0.4s;
      }
    }
  }

  .botanswer_loading_gif {
    width: 24px;
    height: 18px;
    margin-left: 0;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

// Final step layout override: keep agent reasoning/tool output visually close to
// Claude's compact timeline instead of boxed cards.
.agent-stream-display {
  .tool-event {
    .action-card {
      background: transparent;
      border: 0;
      border-left: 0;
      border-radius: 0;
      box-shadow: none;
      overflow: visible;

      &:hover {
        background: transparent;
      }

      &.action-error {
        color: var(--td-error-color);
      }

      &.action-pending {
        background: transparent;
      }
    }

    .action-header {
      padding: 0;

      &:hover {
        background: transparent;
      }
    }
  }

  .action-details {
    border-top: 0;
    background: transparent;
  }

  .thinking-detail-content {
    padding: 7px 0 0 0;
    font-size: var(--agent-step-summary-size);
    color: var(--td-text-color-secondary);
    max-height: none;
    overflow-y: visible;
  }

  .search-results-summary-fixed,
  .plan-status-summary-fixed {
    padding: 2px 0 0 0;
    background: transparent;
    border-top: 0;
  }

  .search-results-summary-fixed .results-summary-text,
  .plan-status-summary-fixed .plan-status-text {
    font-size: var(--agent-step-summary-size);
    font-weight: 400;
    color: var(--td-text-color-secondary);
  }

  .search-results-summary-fixed .results-summary-text :deep(strong) {
    color: var(--td-text-color-secondary);
    font-weight: 500;
  }

  .action-title {
    gap: 12px;
    position: relative;
  }

  .tree-root .action-title {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex: 0 1 auto;
    min-width: 0;
  }

  .tree-root .action-title-icon {
    display: none;
  }

  .icon-mask {
    background-color: var(--agent-step-icon-color);
  }

  .action-title .action-title-icon {
    color: var(--agent-step-icon-color);
    width: 18px;
    height: 18px;
  }

  .tree-child .action-title-icon {
    position: absolute;
    left: -42px;
    top: 3px;
  }

  .action-title .action-name,
  .action-name,
  .action-preamble-title {
    font-size: var(--agent-step-text-size);
    font-weight: 400;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }

  .tree-root .action-name {
    font-size: 14px;
    color: var(--td-text-color-secondary);
  }

  .action-summary {
    font-size: var(--agent-step-summary-size);
    color: var(--td-text-color-placeholder);
  }

  .plan-task-change-card {
    padding: 0;
    background: transparent;
    border: 0;
    border-radius: 0;
    font-size: var(--agent-step-text-size);
    color: var(--td-text-color-secondary);
  }
}
</style>
