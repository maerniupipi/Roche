<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from "vue";
import { useRoute } from 'vue-router';
import { MessagePlugin } from "tdesign-vue-next";
import { useSettingsStore } from '@/stores/settings';
import { useMenuStore } from '@/stores/menu';
import { searchKnowledge, batchQueryKnowledge, listKnowledgeTags } from '@/api/knowledge-base';
import { stopSession } from '@/api/chat';
import KnowledgeBaseSelector from './KnowledgeBaseSelector.vue';
import { useChatResourcesStore } from '@/stores/chatResources';
import { useI18n } from 'vue-i18n';
import type { MentionItem, MentionItemType, MentionRequestItem } from '@/types/mention';

import ConfidentialityAcknowledgement from '@/components/ConfidentialityAcknowledgement.vue'

const route = useRoute();
const settingsStore = useSettingsStore();
const menuStore = useMenuStore();
const chatResources = useChatResourcesStore();
const { t } = useI18n();

let query = ref("");
const showKbSelector = ref(false);

// 手动触发的保密确认弹窗状态（与登录后自动弹的全局强制弹窗文案共用）
const confidentialityDialogVisible = ref(false);

// Mention related state
const showMention = ref(false);
const mentionQuery = ref("");
const mentionItems = ref<MentionItem[]>([]);
/** 文件 ID -> 知识库 ID（用于批量查询时传 kb_id） */
const fileIdToKbId = ref<Record<string, string>>({});
const mentionActiveIndex = ref(0);
const textareaRef = ref<any>(null); // Ref to t-textarea component
const mentionSelectorRef = ref<any>(null);
const mentionHasMore = ref(false);
const mentionGroupCounts = ref<Partial<Record<MentionItemType, number>>>({});
// 当前 @ 会话可见的 KB ID 集合（含工具兼容性过滤），分页加载文件时复用，
// 避免 append 请求把不兼容 KB 的文件漏进来。`null` 表示"不受限制"（非智能体场景）
const mentionAllowedKbIds = ref<Set<string> | null>(null);
const mentionLoading = ref(false);
const mentionOffset = ref(0);
const MENTION_PAGE_SIZE = 20;

const props = defineProps({
  isReplying: {
    type: Boolean,
    required: false
  },
  sessionId: {
    type: String,
    required: false
  },
  assistantMessageId: {
    type: String,
    required: false
  }
});

const selectedKbIds = computed(() => settingsStore.settings.selectedKnowledgeBases || []);
const selectedFileIds = computed(() => settingsStore.settings.selectedFiles || []);
const selectedTags = computed(() => settingsStore.settings.selectedTags || []);

// 已就绪的知识库（来自知识域级缓存）
const knowledgeBases = computed(() => chatResources.validKnowledgeBases);
const fileList = ref<Array<{ id: string; name: string }>>([]);

const selectedKbs = computed(() => {
  return knowledgeBases.value.filter(kb => selectedKbIds.value.includes(kb.id));
});

const selectedFiles = computed(() => {
  // If we have file details in fileList, use them.
  // Otherwise we might show ID or Loading...
  return selectedFileIds.value.map((id: string) => {
    const found = fileList.value.find(f => f.id === id);
    return found || { id, name: 'Loading...' };
  });
});

// 合并所有选中项（用于输入框内显示）
const allSelectedItems = computed(() => {
  const allKbs = selectedKbs.value.map(kb => ({
    ...kb,
    type: 'kb' as const,
    kbType: kb.type,
    isAgentConfigured: false,
  }));

  const files = selectedFiles.value.map((f: { id: string; name: string }) => ({
    ...f,
    type: 'file' as const,
    isAgentConfigured: false,
  }));

  const tags = selectedTags.value.map((tag: any) => ({
    id: tag.id,
    name: tag.name,
    type: 'tag' as const,
    kbId: tag.kbId,
    kbName: tag.kbName,
    description: tag.kbName || '',
    isAgentConfigured: false,
  }));

  return [...allKbs, ...files, ...tags];
});

// 输入框的 placeholder
const inputPlaceholder = computed(() => t('input.placeholder'));

// 加载当前用户可访问的知识库列表。
const loadKnowledgeBases = async (force = false) => {
  try {
    await chatResources.ensureKnowledgeBases(force);
    const validKbs = knowledgeBases.value;

    const validKbIds = new Set(validKbs.map((kb: any) => kb.id));
    const currentSelectedIds = settingsStore.settings.selectedKnowledgeBases || [];
    const validSelectedIds = currentSelectedIds.filter((id: string) => validKbIds.has(id));

    if (validSelectedIds.length !== currentSelectedIds.length) {
      settingsStore.selectKnowledgeBases(validSelectedIds);
    }
  } catch (error) {
    console.error('Failed to load knowledge bases:', error);
  }
};

const loadFiles = async () => {
  const ids = selectedFileIds.value;
  if (ids.length === 0) return;

  const missingIds = ids.filter((id: string) => !fileList.value.find(f => f.id === id));
  if (missingIds.length === 0) return;

  try {
    // 按 kb_id 分组，确保文档查询落在正确的知识库范围。
    const byKbId = new Map<string, string[]>();
    const noKbId: string[] = [];
    missingIds.forEach((id: string) => {
      const kbId = fileIdToKbId.value[id];
      if (kbId) {
        if (!byKbId.has(kbId)) byKbId.set(kbId, []);
        byKbId.get(kbId)!.push(id);
      } else {
        noKbId.push(id);
      }
    });

    const allNewFiles: Array<{ id: string; name: string }> = [];
    const runBatch = async (batchIds: string[], kbId?: string) => {
      const query = new URLSearchParams();
      batchIds.forEach((id: string) => query.append('ids', id));
      const res: any = await batchQueryKnowledge(query.toString(), kbId);
      if (res.data && Array.isArray(res.data)) {
        res.data.forEach((f: any) => allNewFiles.push({ id: f.id, name: f.title || f.file_name }));
      }
    };

    for (const [kbId, batchIds] of byKbId) {
      await runBatch(batchIds, kbId);
    }
    if (noKbId.length > 0) {
      await runBatch(noKbId);
    }
    if (allNewFiles.length > 0) {
      fileList.value = [...fileList.value, ...allNewFiles];
    }
  } catch (e) {
    console.error("Failed to load files", e);
  }
};

watch(selectedFileIds, () => {
  loadFiles();
}, { immediate: true });

// Mention Logic
let lastMentionQuery = '';
const loadMentionItems = async (q: string, resetIndex = true, append = false) => {
  console.log('[Mention] loadMentionItems called with query:', q, 'append:', append);

  if (!append) {
    mentionOffset.value = 0;
  }

  let kbItems: any[] = [];
  let tagItems: MentionItem[] = [];
  if (!append) {
    const availableKbs: any[] = [...knowledgeBases.value];

    mentionAllowedKbIds.value = new Set(availableKbs.map((kb: any) => String(kb.id)));

    const kbs = availableKbs.filter((kb: any) =>
      !q || (kb.name && kb.name.toLowerCase().includes(q.toLowerCase()))
    );
    kbItems = await Promise.all(kbs.map(async (kb: any) => {
      const kbType = kb.type || 'document';
      let count = kbType === 'faq' ? Number(kb.chunk_count || 0) : Number(kb.knowledge_count || 0);
      if (!count) {
        const detail = await chatResources.fetchKnowledgeBaseById(kb.id);
        if (detail) {
          count = detail.type === 'faq'
            ? Number(detail.chunk_count || 0)
            : Number(detail.knowledge_count || 0);
        }
      }
      return {
        id: kb.id,
        name: kb.name,
        type: 'kb' as const,
        kbType: kbType === 'faq' ? 'faq' as const : 'document' as const,
        count,
      };
    }));
    mentionGroupCounts.value.kb = kbItems.length;

    const tagKeyword = q.trim();
    try {
      const tagResults = await Promise.all(availableKbs.map(async (kb: any) => {
        const res: any = await listKnowledgeTags(kb.id, { page: 1, page_size: 20, keyword: tagKeyword || undefined });
        const payload = res?.data ?? res;
        const list = Array.isArray(payload?.data) ? payload.data : (Array.isArray(payload) ? payload : []);
        return list.map((tag: any) => ({
          id: tag.id,
          name: tag.name,
          type: 'tag' as const,
          kbId: kb.id,
          kbName: kb.name,
        }));
      }));
      tagItems = tagResults.flat();
      mentionGroupCounts.value.tag = tagItems.length;
    } catch (e) {
      console.error('[Mention] listKnowledgeTags error:', e);
      tagItems = [];
    }
  }

  // Fetch Files from API
  let fileItems: any[] = [];

  const fileSearchKeyword = q.trim();
  mentionLoading.value = true;
  try {
    const searchOptions = {
      recent: !fileSearchKeyword,
    };
    const res: any = await searchKnowledge(
      fileSearchKeyword,
      mentionOffset.value,
      MENTION_PAGE_SIZE,
      undefined,
      searchOptions
    );
    console.log('[Mention] searchKnowledge response:', res);
    if (res.data && Array.isArray(res.data)) {
      const files = res.data;
      const rawTotal = typeof res.total === 'number' ? res.total : undefined;
      const apiPageSize = res.data.length;
      fileItems = files.map((f: any) => {
        const kbId = f.knowledge_base_id ?? f.kb_id;
        return {
          id: f.id,
          name: f.title || f.file_name,
          type: 'file' as const,
          kbName: f.knowledge_base_name || '',
          kbId: kbId || undefined,
        };
      });
      if (!append && rawTotal != null) {
        mentionGroupCounts.value.file = rawTotal;
      }
    }
    mentionHasMore.value = res.has_more || false;
    mentionOffset.value += fileItems.length;
  } catch (e) {
    console.error('[Mention] searchKnowledge error:', e);
    mentionHasMore.value = false;
  } finally {
    mentionLoading.value = false;
  }

  if (append) {
    // Append file items to existing list
    mentionItems.value = [...mentionItems.value, ...fileItems];
  } else {
    mentionItems.value = [...kbItems, ...tagItems, ...fileItems];
  }
  console.log('[Mention] Total items:', mentionItems.value.length, { kbItems: kbItems.length, fileItems: fileItems.length, tagItems: tagItems.length });

  // Only reset index if query changed or explicitly requested
  if (resetIndex || q !== lastMentionQuery) {
    mentionActiveIndex.value = 0;
  }
  // Ensure index is within bounds
  if (mentionActiveIndex.value >= mentionItems.value.length) {
    mentionActiveIndex.value = Math.max(0, mentionItems.value.length - 1);
  }
  lastMentionQuery = q;
};

const getTextareaEl = () => {
  if (!textareaRef.value) return null;
  // If it's a native element
  if (textareaRef.value instanceof HTMLTextAreaElement) return textareaRef.value;
  // If it's a component wrapper
  const el = textareaRef.value.$el || textareaRef.value;
  if (!el) return null;
  if (el.tagName === 'TEXTAREA') return el as HTMLTextAreaElement;
  return el.querySelector('textarea');
};

const closeMentionSelector = (e: MouseEvent) => {
  const target = e.target as HTMLElement;
  // 如果点击的是输入框区域，不关闭 Mention 列表（由光标逻辑控制）
  if (target.closest('.rich-input-container')) {
    return;
  }
  showMention.value = false;
};

onMounted(() => {
  // 并行拉取；若 platform 已预取且缓存未过期则直接复用
  void Promise.all([
    loadKnowledgeBases(),
  ]);

  // 从持久化恢复 fileId -> kbId（仅保留当前仍选中的文件）。
  const persisted = settingsStore.settings.selectedFileKbMap;
  const ids = settingsStore.settings.selectedFiles || [];
  if (persisted && typeof persisted === 'object' && ids.length > 0) {
    const next: Record<string, string> = {};
    ids.forEach((id: string) => {
      if (persisted[id]) next[id] = persisted[id];
    });
    fileIdToKbId.value = next;
  }

  // 如果从知识库内部进入，自动选中该知识库
  const kbId = (route.params as any)?.kbId as string;
  if (kbId && !selectedKbIds.value.includes(kbId)) {
    settingsStore.addKnowledgeBase(kbId);
  }

  const prefill = menuStore.consumePrefillQuery();
  if (prefill) {
    query.value = prefill;
    nextTick(() => {
      const textarea = getTextareaEl();
      if (textarea) textarea.focus();
    });
  }

  // 监听点击外部关闭下拉菜单
  document.addEventListener('click', closeMentionSelector);
});

onUnmounted(() => {
  document.removeEventListener('click', closeMentionSelector);
});

// 监听路由变化
watch(() => route.params.kbId, (newKbId) => {
  if (newKbId && typeof newKbId === 'string' && !selectedKbIds.value.includes(newKbId)) {
    settingsStore.addKnowledgeBase(newKbId);
  }
});

const emit = defineEmits<{
  (e: 'send-msg', query: string, modelId: string, mentionedItems: MentionRequestItem[]): void;
  (e: 'stop-generation'): void;
}>();

const createSession = async (val: string) => {
  if (!val.trim()) {
    MessagePlugin.info(t('input.messages.enterContent'));
    return;
  }
  if (props.isReplying) {
    return MessagePlugin.error(t('input.messages.replying'));
  }

  // 获取@提及的知识库和文件信息
  const mentionedItems: MentionRequestItem[] = allSelectedItems.value.map(item => ({
    id: item.id,
    name: item.name,
    type: item.type,
    kb_type: item.type === 'kb' ? (item.kbType || 'document') : undefined,
    kb_id: item.kbId,
    kb_name: item.kbName,
    service_id: item.serviceId,
  }));

  // Blur the textarea BEFORE emitting, so that when the parent navigates away
  // and Vue unmounts this component, TDesign's blur handler won't fire on a
  // detached DOM element (which causes getComputedStyle to throw).
  const textarea = getTextareaEl();
  if (textarea) textarea.blur();
  emit('send-msg', val, '', mentionedItems);

  clearvalue();
}

const clearvalue = () => {
  // Guard: only clear when the textarea DOM element is still mounted,
  // otherwise TDesign's autosize will call getComputedStyle on a non-Element.
  if (!getTextareaEl()) return;
  query.value = "";
}

const onKeydown = (val: string, event: { e: { preventDefault(): unknown; keyCode: number; shiftKey: any; ctrlKey: any; }; }) => {
  if (showMention.value) {
    if (event.e.keyCode === 38) { // Up
      event.e.preventDefault();
      mentionSelectorRef.value?.moveActive(-1);
      return;
    }
    if (event.e.keyCode === 40) { // Down
      event.e.preventDefault();
      mentionSelectorRef.value?.moveActive(1);
      return;
    }
    if (event.e.keyCode === 13) { // Enter
      event.e.preventDefault();
      mentionSelectorRef.value?.confirmActive();
      return;
    }
    if (event.e.keyCode === 27) { // Esc
      if (mentionSelectorRef.value?.leaveGroup()) {
        return;
      }
      showMention.value = false;
      return;
    }
  }

  if ((event.e.keyCode == 13 && event.e.shiftKey) || (event.e.keyCode == 13 && event.e.ctrlKey)) {
    return;
  }
  if (event.e.keyCode == 13) {
    event.e.preventDefault();
    createSession(val)
  }
}

const handleStop = async () => {
  if (!props.sessionId) {
    MessagePlugin.warning(t('input.messages.sessionMissing'));
    return;
  }

  if (!props.assistantMessageId) {
    console.error('[Stop] Assistant message ID is empty');
    MessagePlugin.warning(t('input.messages.messageMissing'));
    return;
  }

  console.log('[Stop] Stopping generation for message:', props.assistantMessageId);

  // 发送 stop 事件，通知父组件立即清除 loading 状态
  emit('stop-generation');

  try {
    await stopSession(props.sessionId, props.assistantMessageId);
    MessagePlugin.success(t('input.messages.stopSuccess'));
  } catch (error) {
    console.error('Failed to stop session:', error);
    MessagePlugin.error(t('input.messages.stopFailed'));
  }
}

defineExpose({
  triggerSend(text: string) {
    if (!text.trim()) return;
    query.value = text;
  }
});

</script>
<template>
  <div class="answers-input">
    <!-- 富文本输入框容器 -->
    <div class="rich-input-container" data-guide="chat-input">
      <!-- 实际输入框 -->
      <t-textarea ref="textareaRef" v-model="query" :placeholder="inputPlaceholder" name="description" :autosize="true"
        @keydown="onKeydown" />

      <!-- 控制栏（放在 rich-input-container 内，相对输入框边框定位） -->
      <div class="control-bar">
        <!-- 左侧控制按钮（暂时保留占位容器，后续可能再增加 KB/mention 触发按钮） -->
        <div class="control-left"></div>

        <!-- 右侧控制按钮组 -->
        <div class="control-right">
          <!-- 停止按钮（仅在回复中时显示） -->
          <t-tooltip v-if="isReplying" :content="$t('input.stopGeneration')" placement="top">
            <div @click="handleStop" class="control-btn stop-btn">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                <rect x="5" y="5" width="6" height="6" rx="1" />
              </svg>
            </div>
          </t-tooltip>

          <!-- 发送按钮 -->
          <div v-if="!isReplying" @click="createSession(query)" class="control-btn send-btn" data-guide="chat-send"
            :class="{ 'disabled': !query.length }">
            <img src="../assets/img/sending-aircraft.svg" :alt="$t('input.send')" />
          </div>
        </div>
      </div>
    </div>

    <!-- 输入框下方的免责声明（所有使用 InputField 的页面都会展示） -->
    <div class="answers-disclaimer">
      <span>{{ $t('input.disclaimer') }}</span>
      <button type="button" class="answers-disclaimer__link" @click="confidentialityDialogVisible = true">
        {{ $t('confidentialityAck.title') }}
      </button>
    </div>
    <!-- 知识库选择下拉（使用 Teleport 传送到 body，避免父容器定位影响） -->
    <Teleport to="body">
      <KnowledgeBaseSelector v-model:visible="showKbSelector" @close="showKbSelector = false" />
    </Teleport>
    <ConfidentialityAcknowledgement v-model="confidentialityDialogVisible" />
  </div>
</template>
<style scoped lang="less">
.answers-input {
  position: absolute;
  z-index: 99;
  bottom: 60px;
  left: 50%;
  transform: translateX(-50%);
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;

}

/* 输入框下方的免责声明文字 */
.answers-disclaimer {
  width: 100%;
  max-width: 800px;
  padding: 0 24px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-placeholder);
  text-align: center;

  &__link {
    color: var(--td-brand-color);
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
    font: inherit;

    &:hover {
      opacity: 0.8;
    }
  }
}

/* 富文本输入框容器 */
.rich-input-container:deep {
  position: relative;
  width: 100%;
  max-width: 800px;
  background: var(--td-bg-color-container, #FFF);
  border-radius: 12px;
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04), 0 8px 16px -4px rgba(0, 0, 0, 0.06);

  &:focus-within {
    border-color: var(--td-brand-color, #0B41CD);
  }

  .t-textarea {
    padding: 12px 0;
  }
}

:deep(.t-textarea__inner) {
  width: 100%;
  max-height: 200px !important;
  min-height: 120px !important;
  resize: none;
  color: var(--td-text-color-primary, #000000e6);
  font-size: 16px;
  font-weight: 400;
  line-height: 24px;
  font-family: var(--app-font-family);
  padding: 0 16px 30px;
  border-radius: 12px;
  border: none;
  box-sizing: border-box;
  background: transparent;
  box-shadow: none;

  &:focus {
    border: none;
    box-shadow: none;
  }

  &::placeholder {
    color: var(--td-text-color-placeholder, #00000066);
    font-family: var(--app-font-family);
    font-size: 16px;
    font-weight: 400;
    line-height: 24px;
  }
}

/* 控制栏 */
.control-bar {
  position: absolute;
  bottom: 12px;
  left: 16px;
  right: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  max-height: 56px;
  z-index: 10;
  background: linear-gradient(to bottom, rgba(255, 255, 255, 0) 0%, var(--td-bg-color-container, #fff) 40%, var(--td-bg-color-container, #fff) 100%);
  pointer-events: auto;
  padding-top: 8px;

}

.control-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  flex-wrap: wrap;
  min-width: 0;
}

.control-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px 10px;
  border-radius: 6px;
  color: var(--td-text-color-secondary, #666);
  cursor: pointer;
  transition: background 0.12s, color 0.12s;
  user-select: none;
  flex-shrink: 0;

  &.disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

.control-icon {
  width: 18px;
  height: 18px;
}

.control-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.stop-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  background: rgba(20, 130, 250, 0.08);
  color: var(--td-brand-color);
  border: 1.5px solid rgba(20, 130, 250, 0.2);
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;

  &:hover {
    background: rgba(20, 130, 250, 0.12);
    border-color: var(--td-brand-color);
  }

  &:active {
    background: rgba(20, 130, 250, 0.15);
  }

  svg {
    display: none;
  }

  &::before {
    content: '';
    width: 12px;
    height: 12px;
    background: var(--td-brand-color);
    border-radius: 50%;
    display: block;
    animation: stopBtnPulse 1.5s ease-in-out infinite;
  }
}

@keyframes stopBtnPulse {

  0%,
  100% {
    transform: scale(1);
    opacity: 1;
  }

  50% {
    transform: scale(0.75);
    opacity: 0.6;
  }
}

.send-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  background-color: var(--td-brand-color);

  // &:hover:not(.disabled) {
  //   background-color: var(--td-brand-color-active);
  // }

  // &.disabled {
  //   background-color: var(--td-success-color-light);
  // }

  img {
    width: 16px;
    height: 16px;
  }
}

</style>
