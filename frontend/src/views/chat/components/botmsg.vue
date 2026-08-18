<template>
  <div class="bot_msg">
    <div style="display: flex;flex-direction: column; gap:8px">
      <RagPipelineProgress :session="session" />
      <AgentStreamDisplay :session="session" :session-id="sessionId" :user-query="userQuery" :rag-mode="true"
        @regenerate="(q) => emit('regenerate', q)" />
      <!-- <deepThink :deepSession="session" v-if="session.showThink && !session.isAgentMode"></deepThink> -->
    </div>
    <!-- <picturePreview :reviewImg="reviewImg" :reviewUrl="reviewUrl" @closePreImg="closePreImg"></picturePreview> -->
    <Teleport to="body">
      <ChatCitationFloat :float="citationFloat" :on-enter="cancelCitationClose" :on-leave="scheduleCitationClose" />
    </Teleport>
  </div>
</template>
<script setup>
import { onMounted, onBeforeUnmount, watch, computed, ref, reactive, nextTick, onUpdated } from 'vue';
import 'katex/dist/katex.min.css';
import docInfo from './docInfo.vue';
import deepThink from './deepThink.vue';
import AgentStreamDisplay from './AgentStreamDisplay.vue';
import RagPipelineProgress from './RagPipelineProgress.vue';
import ChatRequestInfoButton from '@/components/ChatRequestInfoButton.vue';
import ChatCitationFloat from '@/components/ChatCitationFloat.vue';
import picturePreview from '@/components/picture-preview.vue';
import { sanitizeMarkdownHTML, safeMarkdownToHTML, createSafeImage, isValidImageURL, hydrateProtectedFileImages } from '@/utils/security';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import {
  copyTextToClipboard,
} from '@/utils/chatMessageShared';
import {
  createChatMarkdownRenderer,
  renderChatMarkdown,
} from '@/utils/chatMarkdownRenderer';
import { stripCitationTagsForCopy } from '@/utils/citationMarkdown';
import {
  createMermaidCodeRenderer,
  ensureMermaidInitialized,
  renderMermaidInContainer,
  enhanceMarkdownContainer,
} from '@/utils/mermaidShared';
import { refreshMarkdownEnhancements } from '@/utils/markdownEnhancements';
import { useChatCitationPopover } from '@/composables/useChatCitationPopover';
import { useTypewriter } from '@/composables/useTypewriter';
import { vStableHtml } from '@/directives/stableHtml';
import { isDebugger } from '@/composables/featureFlags';

ensureMermaidInitialized();

const mentionTagClass = (item) => {
  if (item.type === 'kb') return item.kb_type === 'faq' ? 'faq-tag' : 'kb-tag';
  return `${item.type || 'file'}-tag`;
};

const mentionTagIcon = (item) => {
  if (item.type === 'tag') return 'tag';
  if (item.type === 'skill') return 'bookmark';
  return 'file';
};

const emit = defineEmits(['scroll-bottom', 'regenerate'])
const { t } = useI18n()
let parentMd = ref()
const { float: citationFloat, rebind: rebindCitations, cancelClose: cancelCitationClose, scheduleClose: scheduleCitationClose } = useChatCitationPopover(parentMd, {
  getKnowledgeReferences: () => props.session?.knowledge_references,
  sessionId: () => props.sessionId,
});
let reviewUrl = ref('')
let reviewImg = ref(false)
let isImgLoading = ref(false);
const props = defineProps({
  // 必填项
  content: {
    type: String,
    required: false
  },
  session: {
    type: Object,
    required: false
  },
  userQuery: {
    type: String,
    required: false,
    default: ''
  },
  isFirstEnter: {
    type: Boolean,
    required: false
  },
  sessionId: {
    type: String,
    default: ''
  }
});

const showRequestInfo = computed(() => !!(props.session?.request_id || props.session?.id));

const preview = (url) => {
  nextTick(() => {
    reviewUrl.value = url;
    reviewImg.value = true
  })
}

const closePreImg = () => {
  reviewImg.value = false
  reviewUrl.value = '';
}

const markdownRenderer = createChatMarkdownRenderer({
  codeRenderer: createMermaidCodeRenderer('mermaid-botmsg'),
  imageRenderer: ({ href, title, text }) => createSafeImage(href, text || '', title || ''),
  invalidImageHtml: () => `<p>${t('error.invalidImageLink')}</p>`,
  isValidImageUrl: isValidImageURL,
});

// 计算属性：将 Markdown 文本转换为 tokens
const mentionedItems = computed(() => {
  return props.session?.mentioned_items || [];
});

// Smooth the streamed answer into a steady typewriter cadence (shared with the
// Agent path). Copy/toolbar still read the full content; only display is paced.
const answerText = computed(() => {
  const text = props.content || props.session?.content || '';
  return typeof text === 'string' ? text : '';
});
// "历史已存在但未完成"的 resume-stream 场景需要 snap 到完整文本，避免逐字重打；
// 仅"全新流式回答"才走 typewriter 节奏。
const shouldSnapAnswer = computed(() => {
  if (props.session?.__historyWasInFlight) return true
  return Boolean(props.session?.is_completed)
});
const { displayed: typedAnswer } = useTypewriter(
  () => answerText.value,
  () => shouldSnapAnswer.value,
);

// 区分"实时流式分段输出"与"刷新重连后渲染已有历史"。只有全新流式才走 Markdown
// streaming 容错分支；history resume 必须按已完成渲染，否则不完整引用/标签残留
// 会让样式错乱。
const shouldRenderMessageAsStreaming = computed(() => {
  if (props.session?.__historyWasInFlight) return false
  return !props.session?.is_completed
});

// 单次渲染整个 Markdown 内容（替代 token-by-token，修复 KaTeX 公式在 streaming 时闪烁消失的问题）
const renderedHTML = computed(() => {
  const text = typedAnswer.value;
  if (!text || typeof text !== 'string') return '';
  return renderChatMarkdown(text, {
    renderer: markdownRenderer,
    escapeMarkdown: safeMarkdownToHTML,
    sanitizeHtml: sanitizeMarkdownHTML,
    streaming: shouldRenderMessageAsStreaming.value,
    knowledgeReferences: props.session?.knowledge_references,
  });
});

// 计算属性：判断是否有实际内容（非空且不只是空白）
const hasActualContent = computed(() => {
  const text = props.content || props.session?.content || '';
  return text && text.trim().length > 0;
});

// 获取实际内容
const getActualContent = () => {
  return (props.content || props.session?.content || '').trim();
};

// 复制回答内容
const handleCopyAnswer = async () => {
  const content = getActualContent();
  if (!content) {
    MessagePlugin.warning(t('chat.emptyContentWarning'));
    return;
  }

  try {
    // 剔除内联引用标签（<kb/>、<web/>），只把干净的纯文本答案放进剪贴板。
    await copyTextToClipboard(stripCitationTagsForCopy(content));
    MessagePlugin.success(t('chat.copySuccess'));
  } catch (err) {
    console.error('复制失败:', err);
    MessagePlugin.error(t('chat.copyFailed'));
  }
};

// 处理 markdown-content 中图片的点击事件
const handleMarkdownImageClick = (e) => {
  const target = e.target;
  if (target && target.tagName === 'IMG') {
    const src = target.getAttribute('src');
    if (src) {
      e.preventDefault();
      e.stopPropagation();
      preview(src);
    }
  }
};

watch(renderedHTML, () => {
  nextTick(() => {
    rebindCitations();
  });
});

// 渲染 Mermaid 图表的函数
onUpdated(() => {
  nextTick(async () => {
    await hydrateProtectedFileImages(parentMd.value);
    refreshMarkdownEnhancements(parentMd.value);
    if (props.session?.is_completed) {
      await renderMermaidInContainer(parentMd.value);
    }
  });
});

onMounted(async () => {
  // 为 markdown-content 中的图片添加点击事件
  nextTick(async () => {
    if (parentMd.value) {
      parentMd.value.addEventListener('click', handleMarkdownImageClick, true);
    }
    rebindCitations();
    await hydrateProtectedFileImages(parentMd.value);
    await enhanceMarkdownContainer(parentMd.value);
  });
});

onBeforeUnmount(() => {
  if (parentMd.value) {
    parentMd.value.removeEventListener('click', handleMarkdownImageClick, true);
  }
});
</script>
<style lang="less" scoped>
@import '../../../components/css/chat-markdown.less';
@import '../../../components/css/chat-message-shared.less';
@import '../../../components/css/chat-citations.less';

.rag-answer-stack {
  display: flex;
  flex-direction: column;
  gap: 0;
}

// 内容包装器 - 与 Agent 模式的 answer 样式一致
.content-wrapper {
  padding: 2px 0;
}

.markdown-content {
  // Chat Markdown visual styles are centralized in chat-markdown.less.
  // Do not add element-level Markdown rules here; update the shared mixin.
  .chat-markdown-typography();
  .chat-citation-pills();
}

.mentioned_items {
  .chat-mentioned-items();
}

.mentioned_tag {
  .chat-mentioned-tag();
}

.fallback-icon-btn {
  color: var(--td-text-color-disabled) !important;
  border-color: var(--td-component-stroke) !important;

  &:hover {
    color: var(--td-text-color-placeholder) !important;
    border-color: var(--td-component-border) !important;
  }
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(8px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.ai-markdown-img {
  max-width: 80%;
  max-height: 300px;
  width: auto;
  height: auto;
  border-radius: 8px;
  display: block;
  cursor: pointer;
  object-fit: contain;
  margin: 8px 0 8px 16px;
  border: 0.5px solid var(--td-component-stroke);
  transition: transform 0.2s ease;

  &:hover {
    transform: scale(1.02);
  }
}

.bot_msg {
  // background: var(--td-bg-color-container);
  border-radius: 4px;
  color: var(--td-text-color-primary);
  font-size: 16px;
  // padding: 10px 12px;
  margin-right: auto;
  max-width: 100%;
  box-sizing: border-box;
}

.botanswer_laoding_gif {
  width: 24px;
  height: 18px;
  margin-left: 16px;
}

.thinking-loading {
  padding: 8px 0;
}

.loading-indicator {
  padding: 8px 0;
}

.loading-typing {
  display: flex;
  align-items: center;
  gap: 4px;

  span {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--td-brand-color);
    animation: typingBounce 1.4s ease-in-out infinite;
    // Composite the dots so the bounce stays smooth and ghost-free while the
    // answer relayouts each streamed token.
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

@keyframes typingBounce {

  0%,
  60%,
  100% {
    transform: translate3d(0, 0, 0);
  }

  30% {
    transform: translate3d(0, -6px, 0);
  }
}

.img_loading {
  background: var(--td-bg-color-container-hover);
  height: 230px;
  width: 230px;
  color: var(--td-text-color-placeholder);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  font-size: 12px;
  gap: 4px;
  margin-left: 16px;
  border-radius: 8px;
}

:deep(.t-loading__gradient-conic) {
  background: conic-gradient(from 90deg at 50% 50%, #fff 0deg, #676767 360deg) !important;

}
</style>
