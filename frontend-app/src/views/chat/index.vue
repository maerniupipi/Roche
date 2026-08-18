<template>
  <div class="chat" :class="{
    'is-sidebar-collapsed': uiStore.sidebarCollapsed,
    'has-references-panel': referencesDrawerVisible,
  }">
    <div ref="scrollContainer" class="chat_scroll_box" @scroll="handleScroll">
      <div class="msg_list">
        <!-- 消息列表骨架屏 -->
        <div v-if="historyLoading && messagesList.length === 0" class="msg-skeleton-list">
          <div class="msg-skeleton msg-skeleton-user">
            <t-skeleton animation="gradient" :row-col="[{ width: '45%', height: '36px', type: 'rect' }]" />
          </div>
          <div class="msg-skeleton msg-skeleton-bot">
            <t-skeleton animation="gradient"
              :row-col="[{ width: '80%', height: '16px' }, { width: '100%', height: '16px' }, { width: '60%', height: '16px' }]" />
          </div>
          <div class="msg-skeleton msg-skeleton-user">
            <t-skeleton animation="gradient" :row-col="[{ width: '35%', height: '36px', type: 'rect' }]" />
          </div>
          <div class="msg-skeleton msg-skeleton-bot">
            <t-skeleton animation="gradient"
              :row-col="[{ width: '70%', height: '16px' }, { width: '90%', height: '16px' }]" />
          </div>
        </div>
        <!-- 推荐问题卡片 - 仅在新会话（无消息）时展示 -->
        <SuggestedQuestions v-if="messagesList.length === 0 && !loading && !historyLoading" :enabled="!historyLoading"
          @select="handleSuggestedQuestionClick" />
        <div v-for="(session, index) in messagesList"
          :key="session.id || `${session.role}-${session.created_at}-${index}`" class="msg-item-wrapper">

          <div v-if="session.role == 'user'">
            <usermsg :content="session.content" :mentioned_items="session.mentioned_items" :images="session.images"
              :attachments="session.attachments">
            </usermsg>
          </div>
          <div v-if="session.role == 'assistant' && shouldRenderAssistantMessage(session)">
            <botmsg :content="session.content" :session="session" :session-id="session_id"
              :user-query="getUserQuery(index)" @scroll-bottom="scrollToBottom" :isFirstEnter="isFirstEnter"
              @regenerate="handleRegenerate"></botmsg>
          </div>
        </div>
        <div v-if="showGlobalTypingIndicator" style="height: 41px;display: flex;align-items: center;padding-left: 4px;">
          <div class="loading-typing">
            <span></span>
            <span></span>
            <span></span>
          </div>
        </div>
      </div>
    </div>
    <div class="input-container">
      <transition name="scroll-btn-fade">
        <div v-show="userHasScrolledUp" class="scroll-to-bottom-btn" @click="onClickScrollToBottom">
          <t-icon name="chevron-down" size="20px" />
        </div>
      </transition>
      <InputField ref="inputFieldRef"
        @send-msg="(query, modelId, mentionedItems, imageFiles, attachmentFiles) => sendMsg(query, modelId, mentionedItems, imageFiles, attachmentFiles)"
        @stop-generation="handleStopGeneration" :isReplying="isReplying" :sessionId="session_id"
        :assistantMessageId="currentAssistantMessageId"></InputField>
    </div>
  </div>
  <ChatReferencesDrawer />
  <InlineDocumentPreviewDialog />
</template>
<script setup>
import { storeToRefs } from 'pinia';
import { ref, onMounted, onBeforeMount, onUnmounted, nextTick, watch, reactive, computed } from 'vue';
import { useRoute, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router';
import InputField from '../../components/Input-field.vue';
import botmsg from './components/botmsg.vue';
import usermsg from './components/usermsg.vue';
import { getMessageList, getSession } from "@/api/chat/index";
import { useStream } from '../../api/chat/streame'
import { useMenuStore } from '@/stores/menu';
import { useSettingsStore } from '@/stores/settings';
import { MessagePlugin } from 'tdesign-mobile-vue';
import { useI18n } from 'vue-i18n';
import { useUIStore } from '@/stores/ui';
import { useChatStreamHandler } from '@/composables/useChatStreamHandler';
import { useStickyBottomOnResize } from '@/composables/useStickyBottomOnResize';
import { clearCitationChunkCache } from '@/utils/citationChunkCache';
import ChatReferencesDrawer from '@/components/ChatReferencesDrawer.vue';
import InlineDocumentPreviewDialog from '@/components/InlineDocumentPreviewDialog.vue';
import { provideChatReferencesDrawer } from '@/composables/useChatReferencesDrawer';
import { provideInlineDocumentPreview } from '@/composables/useInlineDocumentPreview';
import SuggestedQuestions from '@/components/SuggestedQuestions.vue';

const referencesDrawer = provideChatReferencesDrawer();
const { visible: referencesDrawerVisible } = referencesDrawer;

// Shared inline document preview dialog (used by both <ChatCitationFloat> and
// <ChatReferencesDrawer>). Providing it here means the dialog is mounted
// exactly once at the chat surface, not per-component.
provideInlineDocumentPreview();

const usemenuStore = useMenuStore();
const useSettingsStoreInstance = useSettingsStore();

// Whether the active chat session is using the Agent pipeline (not quick-answer).
const isAgentStreamSession = () => {
  return useSettingsStoreInstance.isAgentStreamMode;
};

const uiStore = useUIStore();
const { t } = useI18n();
const { firstQuery, firstMentionedItems, firstModelId, firstImageFiles, firstAttachmentFiles } = storeToRefs(usemenuStore);
const { onChunk, error, startStream, stopStream, lastStreamRequest } = useStream();
/** Snapshot of the in-flight HTTP request for attaching to the next assistant message. */
const pendingStreamDebug = ref(null);

const buildStreamDebugPayload = () => {
  const meta = lastStreamRequest.value;
  if (!meta) return null;
  return {
    requestId: meta.requestId,
    url: meta.url,
    method: meta.method,
    body: meta.body,
    sentAt: meta.sentAt,
    sessionId: session_id.value,
  };
};

const attachStreamDebugToMessage = (message) => {
  if (!message) return;
  const payload = pendingStreamDebug.value || buildStreamDebugPayload();
  if (!payload) return;
  if (payload.requestId && !message.request_id) {
    message.request_id = payload.requestId;
  }
  message.debugRequest = payload;
};
const route = useRoute();
const session_id = ref(route.params.chatid);

// 拉 session 详情，并按其 last_request_state 把输入栏状态恢复到当时的发起态。
const loadSessionAndHydrate = async (sid) => {
  if (!sid) return;
  try {
    const sessionRes = await getSession(sid);
    if (sessionRes?.data) {
      const lastState = sessionRes.data.last_request_state;
      if (lastState) {
        // 先把当前的"全局默认"快照下来，再用 session 状态覆盖；
        // 离开会话时会从快照还原，避免本会话的状态污染新建对话。
        useSettingsStoreInstance.snapshotAsDefaultsIfNeeded();
        useSettingsStoreInstance.applyLastRequestState(lastState);
      }
    }
  } catch (error) {
    console.error('Failed to load session data:', error);
  }
};
const inputFieldRef = ref();
const created_at = ref('');
const limit = ref(20);
const messagesList = reactive([]);
const isReplying = ref(false);
const currentAssistantMessageId = ref(''); // 当前正在生成的 assistant message ID
const scrollLock = ref(false);
const isFirstEnter = ref(true);
const loading = ref(false);
const historyLoading = ref(true);
const historyLoadingMore = ref(false);
const hasMoreHistory = ref(true);
let fullContent = ref('')
const scrollContainer = ref(null)
const userHasScrolledUp = ref(false)
const SCROLL_BOTTOM_THRESHOLD = 80

const isNearBottom = () => {
  if (!scrollContainer.value) return true;
  const { scrollTop, scrollHeight, clientHeight } = scrollContainer.value;
  return scrollHeight - scrollTop - clientHeight < SCROLL_BOTTOM_THRESHOLD;
}

const handleSuggestedQuestionClick = (question) => {
  if (inputFieldRef.value?.triggerSend) {
    inputFieldRef.value.triggerSend(question);
  } else {
    sendMsg(question);
  }
};

// 「重新生成」：把用户原始问题重新塞回输入框，由 AgentStreamDisplay 的重新生成按钮触发。
const handleRegenerate = (query) => {
  if (inputFieldRef.value?.triggerSend) {
    inputFieldRef.value.triggerSend(query);
  }
};

// 防抖包装，切换知识库/文件时300ms内不重复请求
const debouncedFetchSuggestions = () => {
  if (historyLoading.value || messagesList.length > 0) return;
  if (suggestedDebounceTimer) clearTimeout(suggestedDebounceTimer);
  suggestedDebounceTimer = setTimeout(() => { fetchSuggestedQuestionsIfNeeded(); }, 300);
};

// 监听 Agent / 知识库 / 文件 / 标签 / Skill @mention，重新获取推荐问题
watch(
  () => ({
    agentId: useSettingsStoreInstance.selectedAgentId,
    kbs: useSettingsStoreInstance.settings.selectedKnowledgeBases,
    files: useSettingsStoreInstance.settings.selectedFiles,
    tags: useSettingsStoreInstance.settings.selectedTags,
    skills: useSettingsStoreInstance.settings.selectedSkills,
  }),
  debouncedFetchSuggestions,
  { deep: true },
);

function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

const getUserQuery = (index) => {
  if (index <= 0) {
    return '';
  }
  const previous = messagesList[index - 1];
  if (previous && previous.role === 'user') {
    return previous.content || '';
  }
  return '';
};

watch([() => route.params], async (newvalue) => {
  isFirstEnter.value = true;
  if (newvalue[0].chatid) {
    if (!firstQuery.value) {
      scrollLock.value = false;
    }
    messagesList.splice(0);
    session_id.value = newvalue[0].chatid;
    clearCitationChunkCache();

    // 切换会话时，重置状态
    historyLoading.value = true;
    historyLoadingMore.value = false;
    hasMoreHistory.value = true;
    created_at.value = '';
    loading.value = false;
    isReplying.value = false;
    currentAssistantMessageId.value = '';
    userHasScrolledUp.value = false;

    // 跨会话切换：先把旧会话覆盖前的全局默认还原，再让新会话重新拍快照
    // 并应用自己的 last_request_state（在 loadSessionAndHydrate 内部完成）。
    useSettingsStoreInstance.restoreDefaultsIfSnapshotted();

    await loadSessionAndHydrate(session_id.value);
    let data = {
      session_id: session_id.value,
      created_at: '',
      limit: limit.value
    }
    getmsgList(data);
  }
});
const scrollToBottom = (force = false) => {
  if (!force && userHasScrolledUp.value) return;
  nextTick(() => {
    if (scrollContainer.value) {
      scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight;
    }
  })
}
const onClickScrollToBottom = () => {
  userHasScrolledUp.value = false;
  scrollToBottom(true);
}

// Images and other rich Markdown content can grow after the SSE chunk that
// introduced them. Follow those delayed height changes while the user remains
// at the live edge; preserve position when they intentionally scroll upward.
useStickyBottomOnResize(scrollContainer, userHasScrolledUp, scrollToBottom);

const debounce = (fn, delay) => {
  let timer
  return (...args) => {
    clearTimeout(timer)
    timer = setTimeout(() => fn(...args), delay)
  }
}
const onChatScrollTop = () => {
  if (scrollLock.value || historyLoadingMore.value || !hasMoreHistory.value) return;
  if (!scrollContainer.value) return;
  const { scrollTop, scrollHeight } = scrollContainer.value;
  isFirstEnter.value = false
  if (scrollTop <= 0) {
    let data = {
      session_id: session_id.value,
      created_at: created_at.value,
      limit: limit.value
    }
    getmsgList(data, true, scrollHeight);
  }
}
const debouncedScrollTop = debounce(onChatScrollTop, 500);
let lastScrollTop = 0;
const handleScroll = () => {
  const el = scrollContainer.value;
  if (el) {
    const currentTop = el.scrollTop;
    // Only an actual upward scroll detaches from the live edge. Content that
    // grows after a chunk (images, diagrams) keeps scrollTop fixed and would
    // otherwise fire a stale scroll event that falsely marks the user as
    // scrolled up, killing the auto-follow during streaming.
    if (currentTop < lastScrollTop - 1) {
      userHasScrolledUp.value = !isNearBottom();
    } else if (isNearBottom()) {
      userHasScrolledUp.value = false;
    }
    lastScrollTop = currentTop;
  }
  debouncedScrollTop();
};

const fetchMessageList = (data) => getMessageList(data);

const {
  findLastMessage,
  shouldRenderAssistantMessage,
  shouldShowGlobalTypingIndicator,
  handleMsgList,
  processStreamChunk,
  prepareForNewOutgoingMessage,
  markInFlightAssistantStopped,
} = useChatStreamHandler({
  messagesList,
  loading,
  isReplying,
  currentAssistantMessageId,
  fullContent,
  isAgentStreamSession,
  scrollToBottom,
  onError: (msg) => MessagePlugin.error(msg),
  preserveIncompleteStreamReactive: true,
  isFirstEnter,
  scrollContainer,
  debug: import.meta.env.DEV,
  onAfterMsgList: async () => {
    const lastMessage = messagesList[messagesList.length - 1];
    if (lastMessage && !lastMessage.is_completed) {
      isReplying.value = true;
      if (lastMessage.role === 'assistant') {
        currentAssistantMessageId.value = lastMessage.id;
        console.log('[Continue Stream] Set assistant message ID:', lastMessage.id);
      }
      await startStream({
        session_id: session_id.value,
        query: lastMessage.id,
        method: 'GET',
        url: '/api/v1/sessions/continue-stream',
      });
    }
  },
  onAgentQuery: (data, existingMessage) => {
    pendingStreamDebug.value = buildStreamDebugPayload();
    if (existingMessage) attachStreamDebugToMessage(existingMessage);
  },
  onMessageCreated: (message) => attachStreamDebugToMessage(message),
  onMessageUpdated: (message, payload) => {
    attachStreamDebugToMessage(message);
    if (payload?.is_completed) pendingStreamDebug.value = null;
  },
  onAgentAnswerDone: (message) => {
    attachStreamDebugToMessage(message);
    pendingStreamDebug.value = null;
  },
  onAgentChunkBound: (message) => {
    attachStreamDebugToMessage(message);
    pendingStreamDebug.value = null;
  },
});

const showGlobalTypingIndicator = computed(() =>
  shouldShowGlobalTypingIndicator(messagesList, loading.value, false),
);

const getmsgList = (data, isScrollType = false, scrollHeight) => {
  if (isScrollType) {
    if (historyLoadingMore.value || !hasMoreHistory.value) return;
    historyLoadingMore.value = true;
  }
  fetchMessageList(data).then(async (res) => {
    const batch = res?.data;
    if (!batch?.length) {
      if (isScrollType) {
        hasMoreHistory.value = false;
      }
      return;
    }
    // 非滚动加载完成后，<SuggestedQuestions> 会通过 enabled 从 false 变 true 自行触发抓取
    const nextCursor = batch[0].created_at;
    if (isScrollType && created_at.value && nextCursor === created_at.value) {
      hasMoreHistory.value = false;
      return;
    }
    if (batch.length < limit.value) {
      hasMoreHistory.value = false;
    }
    created_at.value = nextCursor;
    await handleMsgList(batch, isScrollType, scrollHeight);
  }).catch((err) => {
    console.error('Failed to load messages:', err);
    if (isScrollType) {
      hasMoreHistory.value = false;
    }
  }).finally(() => {
    historyLoading.value = false;
    historyLoadingMore.value = false;
  })
}

// 发送消息
// 处理停止生成事件 - 立即清除 loading 状态
const handleStopGeneration = () => {
  console.log('[Stop Generation] Immediately clearing loading state');
  stopStream();
  loading.value = false;
  isReplying.value = false;
  // 标记当前 assistant 为已结束，避免下一条 query 复用该消息行
  markInFlightAssistantStopped(currentAssistantMessageId.value);
  // 保留 currentAssistantMessageId，Input-field 仍需用它调用 stop API
};

const sendMsg = async (value, modelId = '', mentionedItems = [], imageFiles = [], attachmentFiles = []) => {
  stopStream();
  prepareForNewOutgoingMessage();
  isReplying.value = true;
  loading.value = true;

  // Convert images to base64 data URIs for backend processing and local display
  let imageAttachments = [];
  let userImages = [];
  if (imageFiles && imageFiles.length > 0) {
    try {
      for (const file of imageFiles) {
        const dataURI = await fileToBase64(file);
        imageAttachments.push({ data: dataURI });
        userImages.push({ url: dataURI });
      }
    } catch (e) {
      console.error('[Image] Failed to read images:', e);
      loading.value = false;
      isReplying.value = false;
      return;
    }
  }

  // Convert attachment files to base64 for backend processing
  let attachmentUploads = [];
  if (attachmentFiles && attachmentFiles.length > 0) {
    try {
      for (const attachment of attachmentFiles) {
        const reader = new FileReader();
        const base64Promise = new Promise((resolve, reject) => {
          reader.onload = () => {
            const result = reader.result;
            // Extract base64 content (remove data:...;base64, prefix)
            const base64 = result.split(',')[1];
            resolve(base64);
          };
          reader.onerror = reject;
          reader.readAsDataURL(attachment.file);
        });
        const base64Data = await base64Promise;
        attachmentUploads.push({
          data: base64Data,
          file_name: attachment.name,
          file_size: attachment.size
        });
      }
    } catch (e) {
      console.error('[Attachment] Failed to read attachments:', e);
      loading.value = false;
      isReplying.value = false;
      return;
    }
  }

  // 将@提及的知识库和文件信息存入用户消息
  messagesList.push({ content: value, role: 'user', mentioned_items: mentionedItems, images: userImages, attachments: attachmentFiles.map(a => ({ file_name: a.name, file_size: a.size, file_type: '.' + a.name.split('.').pop()?.toLowerCase() })), channel: 'web' });
  userHasScrolledUp.value = false;
  scrollToBottom(true);

  // Get agent mode status from settings store (prefer selectedAgentId for builtins)
  const agentEnabled = useSettingsStoreInstance.isAgentStreamMode;

  // Get knowledge_base_ids from settings store (selected by user via KnowledgeBaseSelector)
  // Merge @mentioned KB/file IDs so retrieval uses the same targets user @mentioned.
  const sidebarKbIds = useSettingsStoreInstance.settings.selectedKnowledgeBases || [];
  const sidebarFileIds = useSettingsStoreInstance.settings.selectedFiles || [];
  const kbIdSet = new Set(sidebarKbIds);
  const fileIdSet = new Set(sidebarFileIds);
  for (const item of mentionedItems || []) {
    if (!item?.id) continue;
    if (item.type === 'kb' && !kbIdSet.has(item.id)) {
      kbIdSet.add(item.id);
    } else if (item.type === 'file' && !fileIdSet.has(item.id)) {
      fileIdSet.add(item.id);
    }
  }
  const kbIds = [...kbIdSet];
  const knowledgeIds = [...fileIdSet];
  const tagIds = [...new Set((mentionedItems || []).filter(item => item.type === 'tag' && item.id).map(item => item.id))];
  const skillNames = [...new Set((mentionedItems || []).filter(item => item.type === 'skill' && item.id).map(item => item.skill_name || item.id))];

  // Get selected agent ID.
  const selectedAgentId = useSettingsStoreInstance.selectedAgentId || '';

  const endpoint = agentEnabled ? '/api/v1/agent-chat' : '/api/v1/knowledge-chat';

  const requestSkillNames = agentEnabled ? skillNames : [];

  await startStream({
    session_id: session_id.value,
    knowledge_base_ids: kbIds,
    knowledge_ids: knowledgeIds,
    agent_enabled: agentEnabled,
    agent_id: selectedAgentId,
    summary_model_id: modelId,
    skill_names: requestSkillNames,
    tag_ids: tagIds,
    mentioned_items: mentionedItems,
    images: imageAttachments.length > 0 ? imageAttachments : undefined,
    attachment_uploads: attachmentUploads.length > 0 ? attachmentUploads : undefined,
    query: value,
    method: 'POST',
    url: endpoint,
  });
}

// Watch for stream errors and show message
watch(error, (newError) => {
  if (!newError) return;
  MessagePlugin.error(newError);
  isReplying.value = false;
  loading.value = false;
  // 清空当前 assistant message ID
  currentAssistantMessageId.value = '';
});

onChunk((data) => {
  if (data.response_type === 'session_title') {
    const title = data.content || data.data?.title;
    if (title && data.data?.session_id) {
      console.log('[Session Title Update]', {
        session_id: data.data.session_id,
        title: title,
      });
      usemenuStore.updatasessionTitle(data.data.session_id, title);
      usemenuStore.changeIsFirstSession(false);
      window.dispatchEvent(new CustomEvent('session-title-updated', {
        detail: { sessionId: data.data.session_id, title },
      }));
    }
    return;
  }
  processStreamChunk(data);
});

const handleSessionCleared = (e) => {
  if (e.detail?.sessionId === session_id.value) {
    messagesList.splice(0);
    created_at.value = '';
    hasMoreHistory.value = true;
    historyLoadingMore.value = false;
    // messagesList 变空后 <SuggestedQuestions> 重新挂载，自行触发首次抓取
  }
};

onBeforeMount(async () => {
  const agentIdFromQuery = route.query.agent_id && String(route.query.agent_id);
  if (agentIdFromQuery) {
    useSettingsStoreInstance.selectAgent(agentIdFromQuery);
  }
  // 必须在 Input-field onMounted 之前完成：按 session.last_request_state 恢复输入栏
  await loadSessionAndHydrate(session_id.value);
});

onMounted(async () => {
  window.addEventListener('session-messages-cleared', handleSessionCleared);
  messagesList.splice(0);

  // 初始化状态：加载历史消息时不应显示loading
  loading.value = false;
  isReplying.value = false;

  if (firstQuery.value) {
    scrollLock.value = true;
    historyLoading.value = false;
    if (firstModelId.value) {
      useSettingsStoreInstance.updateConversationModels({
        summaryModelId: firstModelId.value,
        selectedChatModelId: firstModelId.value,
        rerankModelId: '',
      });
    }
    sendMsg(firstQuery.value, firstModelId.value || '', firstMentionedItems.value || [], firstImageFiles.value || [], firstAttachmentFiles.value || []);
    usemenuStore.changeFirstQuery('', [], '', [], []);
  } else {
    scrollLock.value = false;
    hasMoreHistory.value = true;
    historyLoadingMore.value = false;
    let data = {
      session_id: session_id.value,
      created_at: '',
      limit: limit.value
    }
    getmsgList(data)
  }
})
const clearData = () => {
  stopStream();
  referencesDrawer.close();
  isReplying.value = false;
  fullContent.value = '';
}
onUnmounted(() => {
  window.removeEventListener('session-messages-cleared', handleSessionCleared);
});
onBeforeRouteLeave((to, from, next) => {
  clearData()
  // 离开聊天会话 → 还原"用户全局默认"，避免旧会话的请求态泄漏到新建对话。
  useSettingsStoreInstance.restoreDefaultsIfSnapshotted();
  next()
})
onBeforeRouteUpdate((to, from, next) => {
  clearData()
  // 仅"会话 → 会话"会落到这里；跨会话覆盖的还原放到 route.params 的 watch 里，
  // 因为新会话的 getSession 也在那边触发，便于保证 restore→snapshot→apply 顺序。
  next()
})
</script>
<style lang="less" scoped>
.chat {
  font-size: 16px;
  padding: 12px 16px;
  box-sizing: border-box;
  flex: 1;
  min-height: 0;
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  width: 100%;
  max-width: 100%;

  @media (min-width: 960px) {
    transition: padding-right 0.3s cubic-bezier(0.22, 0.61, 0.36, 1);
  }

  &.has-references-panel {
    @media (min-width: 960px) {
      padding-right: 420px;
      box-sizing: border-box;
    }
  }

}

.chat_scroll_box {
  flex: 1;
  min-height: 0;
  width: 100%;
  overflow-y: auto;

  scrollbar-width: auto;
  scrollbar-color: auto;
}

.scroll-to-bottom-btn {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  bottom: calc(100% + 15px);
  z-index: 10;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: var(--td-text-color-secondary);
  transition: all 0.2s ease;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  }

  &:active {
    transform: translateX(-50%) scale(0.92);
  }
}

.scroll-btn-fade-enter-active,
.scroll-btn-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.scroll-btn-fade-enter-from,
.scroll-btn-fade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(8px);
}

@keyframes contentFadeIn {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.msg-skeleton-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 100%;
  padding: 16px 0;
  animation: contentFadeIn 0.3s ease-out;
}

.msg-skeleton-user {
  display: flex;
  justify-content: flex-end;
}

.msg-skeleton-bot {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-left: 4px;
}

.input-container {
  // min-height: 115px;
  flex-shrink: 0;
  margin: 0 auto;
  width: 100%;
  max-width: 100%;
  box-sizing: border-box;
  position: relative;
  padding-bottom: env(safe-area-inset-bottom, 0);
}

.msg_list {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 100%;
  flex: 1;
  margin: 0 auto;
  width: 100%;

  .msg-item-wrapper {
    contain: layout style;
  }

  .botanswer_laoding_gif {
    width: 24px;
    height: 18px;
    margin-left: 16px;
  }

  .loading-typing {
    display: flex;
    align-items: center;
    gap: 4px;

    span {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--td-text-color-placeholder);
      animation: typingBounce 1.4s ease-in-out infinite;

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
}

@keyframes typingBounce {

  0%,
  60%,
  100% {
    transform: translateY(0);
  }

  30% {
    transform: translateY(-8px);
  }
}
</style>
