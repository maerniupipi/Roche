<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick, h } from "vue";
import { storeToRefs } from 'pinia';
import { useRoute, useRouter } from 'vue-router';
import { onBeforeRouteUpdate } from 'vue-router';
import { MessagePlugin } from 'tdesign-mobile-vue';
import { useSettingsStore } from '@/stores/settings';
import { useUIStore } from '@/stores/ui';
import { useMenuStore } from '@/stores/menu';
import { searchKnowledge, batchQueryKnowledge, listKnowledgeTags } from '@/api/knowledge-base';
import { stopSession } from '@/api/chat';
import { type ModelConfig } from '@/api/model';
import { type CustomAgent, BUILTIN_QUICK_ANSWER_ID, BUILTIN_SMART_REASONING_ID } from '@/api/agent';
import { useChatResourcesStore } from '@/stores/chatResources';
import { useEditorResourcesStore } from '@/stores/editorResources';
import { useI18n } from 'vue-i18n';
import AttachmentUpload, { type AttachmentFile } from './AttachmentUpload.vue';
import { toolsConsumeFiles } from '@/utils/tool-capabilities';
import { getAgentNotReadyReasonKeys, type AgentNotReadyReasonKey, } from '@/utils/agent-readiness';
import { formatLocalizedList } from '@/utils/format-list';
import type { MentionItem, MentionItemType, MentionRequestItem } from '@/types/mention';
import { useVisualViewport } from '@/composables/useVisualViewport';
const { viewportHeight } = useVisualViewport();
const route = useRoute();
const router = useRouter();
const settingsStore = useSettingsStore();
const uiStore = useUIStore();
const menuStore = useMenuStore();
const chatResources = useChatResourcesStore();
const editorResources = useEditorResourcesStore();
const { agents, allModels, chatModels: availableModels, } = storeToRefs(chatResources);
const { t, locale } = useI18n();

let query = ref("");

// Image upload state
const uploadedImages = ref<Array<{ file: File; preview: string }>>([]);

// Attachment upload state
const attachmentUploadRef = ref<InstanceType<typeof AttachmentUpload>>();
const uploadedAttachments = ref<AttachmentFile[]>([]);
const CHAT_FILE_DROP_EVENT = 'rochekap:chat-file-drop';

const isImageFile = (file: File) => {
  if (file.type.startsWith('image/')) {
    return true;
  }
  const fileName = file.name.toLowerCase();
  return ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp'].some(ext => fileName.endsWith(ext));
};

const handleDroppedFiles = (files: File[]) => {
  if (!files.length) return;

  const imageFiles = files.filter(isImageFile);
  const attachmentFiles = files.filter(file => !isImageFile(file));

  if (imageFiles.length > 0) {
    if (isImageUploadEnabledByAgent.value) {
      addImageFiles(imageFiles);
    } else {
      MessagePlugin.warning(t('input.imageUploadDisabledByAgent'));
    }
  }

  if (attachmentFiles.length > 0) {
    attachmentUploadRef.value?.addFiles(attachmentFiles);
  }
};

const handleChatFileDrop = (event: Event) => {
  const customEvent = event as CustomEvent<{ files?: File[] }>;
  const files = customEvent.detail?.files;
  if (!files || files.length === 0) return;
  handleDroppedFiles(files);
};

const addImageFiles = (files: File[]) => {
  if (!isImageUploadEnabledByAgent.value) return;
  const allowed = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];
  const maxSize = 10 * 1024 * 1024;
  for (const file of files) {
    if (uploadedImages.value.length >= 5) {
      MessagePlugin.warning(t('chat.imageTooMany'));
      break;
    }
    if (!allowed.includes(file.type)) {
      MessagePlugin.warning(t('chat.imageTypeSizeError'));
      continue;
    }
    if (file.size > maxSize) {
      MessagePlugin.warning(t('chat.imageTypeSizeError'));
      continue;
    }
    uploadedImages.value.push({ file, preview: URL.createObjectURL(file) });
  }
};

const showAgentModeSelector = ref(false);
const agentModeButtonRef = ref<HTMLElement>();
const agentModeDropdownStyle = ref<Record<string, string>>({});

const selectedAgentId = computed({
  get: () => settingsStore.selectedAgentId || BUILTIN_QUICK_ANSWER_ID,
  set: (val: string) => settingsStore.selectAgent(val)
});
const selectedAgent = computed(() => {
  const mine = agents.value.find(a => a.id === selectedAgentId.value);
  if (mine) return mine;
  return {
    id: BUILTIN_QUICK_ANSWER_ID,
    name: t('input.normalMode'),
    is_builtin: true,
    config: { agent_mode: 'quick-answer' as const }
  } as CustomAgent;
});

// 判断是否有智能体配置（包括内置智能体）
const hasAgentConfig = computed(() => {
  const agent = selectedAgent.value;
  if (agent?.is_builtin) {
    const builtinAgent = agents.value.find(a => a.id === agent.id);
    return !!builtinAgent?.config;
  }
  return !!agent?.config;
});

// 获取当前智能体的实际配置（内置智能体从 agents 列表获取）
const currentAgentConfig = computed(() => {
  const agent = selectedAgent.value;
  if (agent?.is_builtin) {
    const builtinAgent = agents.value.find(a => a.id === agent.id);
    return builtinAgent?.config || {};
  }
  return agent?.config || {};
});

// 智能体切换不会改变知识访问范围；该范围始终来自当前用户授权。
watch(selectedAgentId, (newAgentId, oldAgentId) => {
  if (settingsStore._isApplyingSessionState) return;
  if (newAgentId !== oldAgentId && oldAgentId !== undefined) {
    // 若 @ 面板已打开，刷新当前用户可访问的资源。
    if (showMention.value) {
      loadMentionItems(mentionQuery.value, true);
    }
    // Clear images when switching to an agent that doesn't support image upload
    if (!isImageUploadEnabledByAgent.value && uploadedImages.value.length > 0) {
      uploadedImages.value.forEach(img => URL.revokeObjectURL(img.preview));
      uploadedImages.value = [];
    }
  }
}, { immediate: true });

// 智能体配置的模型 ID
const agentModelId = computed(() => {
  if (!hasAgentConfig.value) return null;
  return currentAgentConfig.value?.model_id || null;
});

// 智能体支持的文件类型（空数组表示支持所有类型）
const agentSupportedFileTypes = computed(() => {
  if (!hasAgentConfig.value) return [];
  return currentAgentConfig.value?.supported_file_types || [];
});

// 智能体配置的工具列表，驱动 @ 菜单的 KB 兼容性过滤
const agentAllowedTools = computed<string[]>(() => {
  if (!hasAgentConfig.value) return [];
  return currentAgentConfig.value?.allowed_tools || [];
});

type SelectionMode = 'all' | 'selected' | 'none';

const normalizeSelectionMode = (mode?: string): SelectionMode => {
  return mode === 'all' || mode === 'selected' || mode === 'none' ? mode : 'none';
};

const agentSkillsSelectionMode = computed<SelectionMode>(() => {
  if (!settingsStore.isAgentStreamMode || !hasAgentConfig.value) return 'none';
  return normalizeSelectionMode(currentAgentConfig.value?.skills_selection_mode);
});

const agentSelectedSkills = computed<string[]>(() => {
  if (agentSkillsSelectionMode.value !== 'selected') return [];
  return currentAgentConfig.value?.selected_skills || [];
});

const isSkillAllowedByAgent = (skillName: string) => {
  if (!settingsStore.isAgentStreamMode || !editorResources.skillsAvailable) return false;
  const mode = agentSkillsSelectionMode.value;
  if (mode === 'none') return false;
  if (mode === 'selected') return agentSelectedSkills.value.includes(skillName);
  return true;
};

// 切换智能体时清理不允许的 Skill @mention
watch([selectedAgentId, agentSkillsSelectionMode], ([newAgentId], [oldAgentId]) => {
  if (settingsStore._isApplyingSessionState) return;
  if (newAgentId === oldAgentId || oldAgentId === undefined) return;

  const skillsMode = agentSkillsSelectionMode.value;
  if (skillsMode === 'none') {
    settingsStore.settings.selectedSkills = [];
  } else if (skillsMode === 'selected') {
    const allowed = new Set(agentSelectedSkills.value);
    settingsStore.settings.selectedSkills = (settingsStore.settings.selectedSkills || [])
      .filter(name => allowed.has(name));
  }
});

// 智能体是否启用了图片上传（多模态）
const isImageUploadEnabledByAgent = computed(() => {
  if (!hasAgentConfig.value) return false;
  return currentAgentConfig.value?.image_upload_enabled === true;
});

// 模型选择是否被智能体锁定 - 已移除锁定逻辑，允许用户自由切换模型
const isModelLockedByAgent = computed(() => {
  return false;
});

// Mention related state
const showMention = ref(false);
const mentionQuery = ref("");
const mentionItems = ref<MentionItem[]>([]);
/** 文件 ID -> 知识库 ID（用于批量查询时传 kb_id） */
const fileIdToKbId = ref<Record<string, string>>({});
const mentionActiveIndex = ref(0);
const textareaRef = ref<any>(null); // Ref to t-textarea component
const mentionSelectorRef = ref<any>(null);
const isComposing = ref(false);
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

const isAgentStreamMode = computed(() => settingsStore.isAgentStreamMode);
const selectedKbIds = computed(() => settingsStore.settings.selectedKnowledgeBases || []);
const selectedFileIds = computed(() => settingsStore.settings.selectedFiles || []);
const selectedTags = computed(() => settingsStore.settings.selectedTags || []);
const selectedSkillNames = computed(() => settingsStore.settings.selectedSkills || []);

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

const skillMentionItems = computed<MentionItem[]>(() => {
  return selectedSkillNames.value
    .filter((name: string) => isSkillAllowedByAgent(name))
    .map((name: string) => {
      const skill = editorResources.skills.find(s => s.name === name);
      return {
        id: name,
        name: skill?.name || name,
        type: 'skill' as const,
        skillName: name,
        description: skill?.description || '',
      };
    });
});



// 合并所有选中项（用于输入框内显示）
const allSelectedItems = computed(() => {
  const knowledgeItems: MentionItem[] = [];
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

  if (!isAgentStreamMode.value) {
    knowledgeItems.push(...allKbs, ...files, ...tags);
  }
  return [...knowledgeItems, ...skillMentionItems.value];
});

// 移除选中项（智能体配置的项也可以移除）
const removeSelectedItem = (item: MentionItem) => {
  if (item.type === 'kb') {
    settingsStore.removeKnowledgeBase(item.id);
  } else if (item.type === 'file') {
    settingsStore.removeFile(item.id);
  } else if (item.type === 'tag') {
    settingsStore.removeTag(item.id, item.kbId);
  } else if (item.type === 'skill') {
    settingsStore.removeSkill(item.skillName || item.id);
  }
};

// 使用 computed 从 store 读取，并通过 setter 同步回 store
const selectedModelId = computed({
  get: () => settingsStore.conversationModels.selectedChatModelId || '',
  set: (val: string) => settingsStore.updateConversationModels({ selectedChatModelId: val })
});
const modelsLoading = ref(false);
const showModelSelector = ref(false);
const modelButtonRef = ref<HTMLElement>();
const modelDropdownStyle = ref<Record<string, string>>({});

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

// 加载当前用户可访问的智能体。
const loadAgents = async (force = false) => {
  try {
    await chatResources.ensureAgents(force);
  } catch (error) {
    console.error('Failed to load agents:', error);
  }
};

// LAST_CHAT_MODEL_KEY scopes the per-user "last selected chat model"
// to localStorage. The previous implementation wrote this back to the
// the removed knowledge-domain conversation configuration endpoint, which (a) required
// Admin+ to mutate, so a Viewer switching models in the
// chat input got a 403, and (b) silently overwrote the knowledgeDomain default
// for everyone else. localStorage is per-user-per-browser, which is
// what "remember my last pick" actually wants.
const LAST_CHAT_MODEL_KEY = 'roche_kap_last_chat_model_id'

const readLastChatModelID = (): string => {
  try {
    return localStorage.getItem(LAST_CHAT_MODEL_KEY) || ''
  } catch {
    return ''
  }
}

const writeLastChatModelID = (id: string) => {
  try {
    if (id) {
      localStorage.setItem(LAST_CHAT_MODEL_KEY, id)
    } else {
      localStorage.removeItem(LAST_CHAT_MODEL_KEY)
    }
  } catch {
    // localStorage may be disabled in incognito mode; ignore.
  }
}

// Initial chat-model selection priority: per-user last pick
// (localStorage) > current store value (e.g. carried over from
// settings page) > first available model. The knowledgeDomain-level
// conversation-config used to feed summary_model_id/rerank_model_id
// into the dropdown, but those fields were removed: per-user last pick
// belongs in localStorage, agent-level model belongs on the agent.
const initChatModelSelection = () => {
  const lastPick = readLastChatModelID();
  const currentSelectedModel = settingsStore.conversationModels.selectedChatModelId;
  const initialSelection = lastPick || currentSelectedModel || '';
  settingsStore.updateConversationModels({
    summaryModelId: initialSelection,
    selectedChatModelId: initialSelection,
    rerankModelId: '',
  });
  if (!selectedModelId.value) {
    selectedModelId.value = initialSelection;
  }
  ensureModelSelection();
};

const loadChatModels = async (force = false) => {
  if (modelsLoading.value) return;
  modelsLoading.value = true;
  try {
    await chatResources.ensureChatModels(force);
    ensureModelSelection();
  } catch (error) {
    console.error('Failed to load chat models:', error);
    chatResources.invalidate('models');
  } finally {
    modelsLoading.value = false;
  }
};

const ensureModelSelection = () => {
  if (selectedModelId.value) {
    return;
  }
  const lastPick = readLastChatModelID();
  if (lastPick) {
    selectedModelId.value = lastPick;
    return;
  }
  if (availableModels.value.length > 0) {
    selectedModelId.value = availableModels.value[0].id || '';
  }
};

// 智能体身份或其数据到位时，把对话模型同步到智能体配置的 model_id。
// 修复场景：导航离开再返回时，initChatModelSelection 会用 localStorage 的 lastPick
// 覆盖智能体绑定的 model_id，此时需要拉回 agent 模型。
// 但若用户在本页手动改过模型（lastPick 与 agent 默认不同且当前选中即为 lastPick），
// 则保留用户选择，避免 creatChat → chat 跳转后把模型 B 冲回智能体默认 A。
watch(
  [selectedAgentId, agentModelId],
  ([, newModelId]) => {
    if (!newModelId || newModelId.trim() === '') return;

    const lastPick = readLastChatModelID();

    if (
      lastPick &&
      selectedModelId.value === lastPick &&
      lastPick !== newModelId
    ) {
      return;
    }

    if (newModelId !== selectedModelId.value) {
      selectedModelId.value = newModelId;
    }
  },
  { immediate: true }
);

const handleGoToConversationModels = () => {
  showModelSelector.value = false;
  // 设置里的「模型」分组已被移除，原先的「去设置新增模型」入口不再可达。
  // 弹个提示而不是跳到空白的设置页，避免用户摸不着头脑。
  MessagePlugin.info(t('chatInput.addModelUnavailable'));
};

const handleModelChange = (value: string | number | Array<string | number> | undefined) => {
  const normalized = Array.isArray(value) ? value[0] : value;
  const val = normalized !== undefined && normalized !== null ? String(normalized) : '';

  if (!val) {
    selectedModelId.value = '';
    return;
  }
  if (val === '__add_model__') {
    selectedModelId.value = readLastChatModelID();
    handleGoToConversationModels();
    return;
  }

  // The chat-level model picker now persists per-user-per-browser via
  // localStorage instead of writing to the knowledgeDomain-shared KV. This is what
  // "remember my last pick" should always have meant — the previous PUT
  // The removed conversation configuration endpoint required Admin+, so a Viewer
  // switching models from the chat input got a 403.
  writeLastChatModelID(val);
  selectedModelId.value = val;
  showModelSelector.value = false;

  settingsStore.updateConversationModels({
    summaryModelId: val,
    selectedChatModelId: val,
    rerankModelId: '',
  });
};

const selectedModel = computed(() => {
  return availableModels.value.find(model => model.id === selectedModelId.value);
});

const modelDisplayName = (model: ModelConfig) => {
  const displayName = model.display_name?.trim();
  return displayName || model.name;
};

const updateModelDropdownPosition = () => {
  const anchor = modelButtonRef.value;
  if (!anchor) {
    modelDropdownStyle.value = {
      position: 'fixed',
      top: '50%',
      left: '50%',
      transform: 'translate(-50%, -50%)',
    };
    return;
  }

  // Normalize coordinates to CSS pixels. F3 已删除 zoom 适配，
  // getBoundingClientRect 现在直接返回 CSS 像素。
  const rect = anchor.getBoundingClientRect();
  console.log('[Model Dropdown] Button rect:', {
    top: rect.top,
    bottom: rect.bottom,
    left: rect.left,
    right: rect.right,
    width: rect.width,
    height: rect.height
  });

  const dropdownWidth = 280;
  const offsetY = 8;
  const vw = window.innerWidth;
  const vh = window.innerHeight;

  // 左对齐到触发元素的左边缘
  // 使用 Math.floor 而不是 Math.round，避免像素对齐问题
  let left = Math.floor(rect.left);

  // 边界处理：不超出视口左右（留 16px margin）
  const minLeft = 16;
  const maxLeft = Math.max(16, vw - dropdownWidth - 16);
  left = Math.max(minLeft, Math.min(maxLeft, left));

  // 垂直定位：紧贴按钮，使用合理的高度避免空白
  const preferredDropdownHeight = 280; // 优选高度（紧凑且够用）
  const maxDropdownHeight = 360; // 最大高度
  const minDropdownHeight = 200; // 最小高度
  const topMargin = 20; // 顶部留白
  const spaceBelow = vh - rect.bottom; // 下方剩余空间
  const spaceAbove = rect.top; // 上方剩余空间

  console.log('[Model Dropdown] Space check:', {
    spaceBelow,
    spaceAbove,
    windowHeight: vh
  });

  let actualHeight: number;
  let shouldOpenBelow: boolean;

  // 优先考虑下方空间
  if (spaceBelow >= minDropdownHeight + offsetY) {
    // 下方有足够空间，向下弹出
    actualHeight = Math.min(preferredDropdownHeight, spaceBelow - offsetY - 16);
    shouldOpenBelow = true;
    console.log('[Model Dropdown] Position: below button', { actualHeight });
  } else {
    // 向上弹出，优先使用 preferredHeight，必要时才扩展到 maxHeight
    const availableHeight = spaceAbove - offsetY - topMargin;
    if (availableHeight >= preferredDropdownHeight) {
      // 有足够空间显示优选高度
      actualHeight = preferredDropdownHeight;
    } else {
      // 空间不够，使用可用空间（但不小于最小高度）
      actualHeight = Math.max(minDropdownHeight, availableHeight);
    }
    shouldOpenBelow = false;
    console.log('[Model Dropdown] Position: above button', { actualHeight });
  }

  // 根据弹出方向使用不同的定位方式
  if (shouldOpenBelow) {
    // 向下弹出：使用 top 定位，左对齐
    const top = Math.floor(rect.bottom + offsetY);
    console.log('[Model Dropdown] Opening below, top:', top);
    modelDropdownStyle.value = {
      position: 'fixed !important',
      width: `${dropdownWidth}px`,
      left: `${left}px`,
      top: `${top}px`,
      maxHeight: `${actualHeight}px`,
      transform: 'none !important',
      margin: '0 !important',
      padding: '0 !important'
    };
  } else {
    // 向上弹出：使用 bottom 定位，左对齐
    const bottom = vh - rect.top + offsetY;
    console.log('[Model Dropdown] Opening above, bottom:', bottom);
    modelDropdownStyle.value = {
      position: 'fixed !important',
      width: `${dropdownWidth}px`,
      left: `${left}px`,
      bottom: `${bottom}px`,
      maxHeight: `${actualHeight}px`,
      transform: 'none !important',
      margin: '0 !important',
      padding: '0 !important'
    };
  }

  console.log('[Model Dropdown] Applied style:', modelDropdownStyle.value);
};

// Mention Logic
let lastMentionQuery = '';
const loadMentionItems = async (q: string, resetIndex = true, append = false) => {
  console.log('[Mention] loadMentionItems called with query:', q, 'append:', append);

  if (!append) {
    mentionOffset.value = 0;
  }

  // 普通 RAG 可用 @ 缩小本轮知识范围；Agent 模式的知识范围固定来自
  // 当前用户授权，因此 Agent 的 @ 菜单只展示 Skill。
  const allowKnowledgeMention = !isAgentStreamMode.value;
  let kbItems: any[] = [];
  let tagItems: MentionItem[] = [];
  let skillItems: MentionItem[] = [];
  if (!append) {
    const availableKbs: any[] = allowKnowledgeMention ? [...knowledgeBases.value] : [];

    mentionAllowedKbIds.value = allowKnowledgeMention
      ? new Set(availableKbs.map((kb: any) => String(kb.id)))
      : null;

    if (allowKnowledgeMention) {
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
    } else {
      mentionGroupCounts.value.kb = 0;
      mentionGroupCounts.value.tag = 0;
    }

    const skillsMode = agentSkillsSelectionMode.value;
    if (skillsMode !== 'none') {
      await editorResources.ensureSkills();
      skillItems = editorResources.skills
        .filter(skill => isSkillAllowedByAgent(skill.name))
        .map(skill => ({
          id: skill.name,
          name: skill.name,
          type: 'skill' as const,
          skillName: skill.name,
          description: skill.description || '',
        }))
        .filter(skill => {
          if (!q) return true;
          const keyword = q.toLowerCase();
          return skill.name.toLowerCase().includes(keyword)
            || (skill.description || '').toLowerCase().includes(keyword);
        });
    }
  }

  // Fetch Files from API
  // 文件候选同样只受当前用户授权范围约束。
  let fileItems: any[] = [];
  const toolsAllowFiles = !hasAgentConfig.value || toolsConsumeFiles(agentAllowedTools.value);
  const shouldLoadFiles = allowKnowledgeMention && toolsAllowFiles;

  // 空关键词时显式请求最近文件；有关键词时返回匹配文件。
  // `recent=true` 只用于浏览态，避免其他搜索调用漏传关键词时静默退化为最近列表。
  const fileSearchKeyword = q.trim();
  if (shouldLoadFiles) {
    mentionLoading.value = true;
    try {
      const fileTypesParam = agentSupportedFileTypes.value.length > 0 ? agentSupportedFileTypes.value : undefined;
      const searchOptions = {
        recent: !fileSearchKeyword,
      };
      const res: any = await searchKnowledge(
        fileSearchKeyword,
        mentionOffset.value,
        MENTION_PAGE_SIZE,
        fileTypesParam,
        searchOptions
      );
      console.log('[Mention] searchKnowledge response:', res);
      if (res.data && Array.isArray(res.data)) {
        let files = res.data;
        const rawTotal = typeof res.total === 'number' ? res.total : undefined;
        const apiPageSize = res.data.length;
        // 按当前 @ 会话的兼容 KB 集合过滤：
        //   - 非智能体场景：`mentionAllowedKbIds` 为 null，跳过；
        //   - 智能体场景：'selected' 会把 ID 收敛到用户勾的 KB，
        //     'all' 会收敛到"兼容"的 KB，'none' 根本走不到这里（shouldLoadFiles=false）。
        //   这样分页 append 也能用同一份集合，不再只处理 'selected' 分支。
        if (mentionAllowedKbIds.value) {
          const allowed = mentionAllowedKbIds.value;
          files = files.filter((f: any) => {
            const kbId = f.knowledge_base_id ?? f.kb_id;
            return kbId != null && allowed.has(String(kbId));
          });
        }
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
        if (!append) {
          const clientFiltered = !!mentionAllowedKbIds.value && fileItems.length < apiPageSize;
          if (!clientFiltered && rawTotal != null) {
            mentionGroupCounts.value.file = rawTotal;
          } else {
            delete mentionGroupCounts.value.file;
          }
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
  } else {
    mentionHasMore.value = false;
  }

  if (append) {
    // Append file items to existing list
    mentionItems.value = [...mentionItems.value, ...fileItems];
  } else {
    mentionItems.value = [...kbItems, ...tagItems, ...skillItems, ...fileItems];
  }
  console.log('[Mention] Total items:', mentionItems.value.length, { kbItems: kbItems.length, fileItems: fileItems.length, tagItems: tagItems.length, skillItems: skillItems.length });

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

const onInput = (_val: string | InputEvent) => {
  // 如果正在输入法组合中，不处理搜索逻辑，等待 compositionend
  if (isComposing.value) return;
};

const onCompositionStart = () => {
  isComposing.value = true;
};

const onCompositionEnd = (e: CompositionEvent) => {
  isComposing.value = false;
  // 手动触发 onInput 逻辑
  // 注意：在 compositionend 时，v-model 可能还没更新，或者已经更新但我们需要用最新值
  // TDesign textarea 可能需要 nextTick
  nextTick(() => {
    onInput(query.value);
  });
};


const toggleModelSelector = () => {
  // 如果智能体锁定了模型，不允许打开选择器
  if (isModelLockedByAgent.value) {
    MessagePlugin.warning(t('input.modelLockedByAgent'));
    return;
  }

  // 互斥：关闭其他
  showMention.value = false;
  showAgentModeSelector.value = false;

  showModelSelector.value = !showModelSelector.value;
  if (showModelSelector.value) {
    if (!availableModels.value.length) {
      loadChatModels();
    }
    // 多次更新位置确保准确
    nextTick(() => {
      updateModelDropdownPosition();
      requestAnimationFrame(() => {
        updateModelDropdownPosition();
        setTimeout(() => {
          updateModelDropdownPosition();
        }, 50);
      });
    });
  }
};

const closeModelSelector = () => {
  showModelSelector.value = false;
};

// 关闭 Agent 模式选择器（点击外部）
const closeAgentModeSelector = () => {
  showAgentModeSelector.value = false;
};

const closeMentionSelector = (e: MouseEvent) => {
  const target = e.target as HTMLElement;
  // 如果点击的是输入框区域，不关闭 Mention 列表（由光标逻辑控制）
  if (target.closest('.rich-input-container')) {
    return;
  }
  showMention.value = false;
};

// 窗口事件处理器
let resizeHandler: (() => void) | null = null;
let scrollHandler: (() => void) | null = null;

onMounted(() => {
  // 并行拉取；若 platform 已预取且缓存未过期则直接复用
  initChatModelSelection();
  void Promise.all([
    loadKnowledgeBases(),
    loadChatModels(),
    loadAgents(),
  ]);
  window.addEventListener(CHAT_FILE_DROP_EVENT, handleChatFileDrop as EventListener);

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
  document.addEventListener('click', closeAgentModeSelector);
  document.addEventListener('click', closeModelSelector);
  document.addEventListener('click', closeMentionSelector);

  // 监听窗口大小变化和滚动，重新计算位置
  resizeHandler = () => {
    if (showModelSelector.value) {
      updateModelDropdownPosition();
    }
    if (showAgentModeSelector.value) {
      updateAgentModeDropdownPosition();
    }
  };
  scrollHandler = () => {
    if (showModelSelector.value) {
      updateModelDropdownPosition();
    }
    if (showAgentModeSelector.value) {
      updateAgentModeDropdownPosition();
    }
  };

  window.addEventListener('resize', resizeHandler, { passive: true });
  window.addEventListener('scroll', scrollHandler, { passive: true, capture: true });
});

onUnmounted(() => {
  window.removeEventListener(CHAT_FILE_DROP_EVENT, handleChatFileDrop as EventListener);
  document.removeEventListener('click', closeAgentModeSelector);
  document.removeEventListener('click', closeModelSelector);
  document.removeEventListener('click', closeMentionSelector);
  if (resizeHandler) {
    window.removeEventListener('resize', resizeHandler);
  }
  if (scrollHandler) {
    window.removeEventListener('scroll', scrollHandler, { capture: true });
  }
});

// 监听路由变化
watch(() => route.params.kbId, (newKbId) => {
  if (newKbId && typeof newKbId === 'string' && !selectedKbIds.value.includes(newKbId)) {
    settingsStore.addKnowledgeBase(newKbId);
  }
});

watch(() => uiStore.showSettingsModal, (visible, prevVisible) => {
  if (prevVisible && !visible) {
    loadChatModels(true);
  }
});

watch([selectedKbIds, selectedFileIds], ([kbIds, fileIds]) => {
  if (!kbIds.length && !fileIds.length) {
    closeModelSelector();
  }
}, { deep: true });

const emit = defineEmits<{
  (e: 'send-msg', query: string, modelId: string, mentionedItems: MentionRequestItem[], imageFiles: File[], attachmentFiles: AttachmentFile[]): void;
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

  if (!chatResources.isFresh('models')) {
    await loadChatModels()
  }

  // 发送前校验当前选中的智能体（含默认快速问答）是否已配置完成
  const agentToCheck = selectedAgent.value;
  let actualAgent = agentToCheck;
  if (agentToCheck.is_builtin) {
    let builtin = agents.value.find(a => a.id === selectedAgentId.value);
    if (!builtin) {
      await loadAgents();
      builtin = agents.value.find(a => a.id === selectedAgentId.value);
    }
    actualAgent = builtin || agentToCheck;
  }
  const isAgentMode = actualAgent.config?.agent_mode === 'smart-reasoning';
  const { keys: notReadyKeys, labels: notReadyReasons } = collectAgentNotReadyReasons(
    actualAgent,
    isAgentMode,
  );
  if (notReadyReasons.length > 0) {
    showAgentNotReadyMessage(
      actualAgent,
      notReadyReasons,
      notReadyKeys,
    );
    return;
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
    skill_name: item.skillName,
  }));
  const imageFiles = uploadedImages.value.map(img => img.file);
  const attachmentFiles = uploadedAttachments.value;

  // Blur the textarea BEFORE emitting, so that when the parent navigates away
  // and Vue unmounts this component, TDesign's blur handler won't fire on a
  // detached DOM element (which causes getComputedStyle to throw).
  const textarea = getTextareaEl();
  if (textarea) textarea.blur();
  emit('send-msg', val, selectedModelId.value, mentionedItems, imageFiles, attachmentFiles);

  // Clean up image previews
  uploadedImages.value.forEach(img => URL.revokeObjectURL(img.preview));
  uploadedImages.value = [];

  // Clean up attachments
  attachmentUploadRef.value?.clear();
  uploadedAttachments.value = [];

  clearvalue();
}

const updateAgentModeDropdownPosition = () => {
  const anchor = agentModeButtonRef.value;

  if (!anchor) {
    agentModeDropdownStyle.value = {
      position: 'fixed',
      top: '50%',
      left: '50%',
      transform: 'translate(-50%, -50%)'
    };
    return;
  }

  // Normalize coordinates to CSS pixels. F3 已删除 zoom 适配，
  // getBoundingClientRect 现在直接返回 CSS 像素。
  const rect = anchor.getBoundingClientRect();
  const dropdownWidth = 200;
  const offsetY = 8;
  const vw = window.innerWidth;
  const vh = window.innerHeight;

  // 水平位置：左对齐
  let left = Math.floor(rect.left);
  const minLeft = 16;
  const maxLeft = Math.max(16, vw - dropdownWidth - 16);
  left = Math.max(minLeft, Math.min(maxLeft, left));

  // 垂直位置：紧贴按钮，使用合理的高度避免空白
  const preferredDropdownHeight = 140; // Agent 模式选择器内容较少，用更小的优选高度
  const maxDropdownHeight = 150;
  const minDropdownHeight = 100;
  const topMargin = 20;
  const spaceBelow = vh - rect.bottom;
  const spaceAbove = rect.top;

  console.log('[Agent Dropdown] Space check:', {
    spaceBelow,
    spaceAbove,
    windowHeight: vh
  });

  let actualHeight: number;

  // 优先考虑下方空间
  if (spaceBelow >= minDropdownHeight + offsetY) {
    // 下方有足够空间，向下弹出
    actualHeight = Math.min(preferredDropdownHeight, spaceBelow - offsetY - 16);
    const top = Math.floor(rect.bottom + offsetY);

    agentModeDropdownStyle.value = {
      position: 'fixed !important',
      width: `${dropdownWidth}px`,
      left: `${left}px`,
      top: `${top}px`,
      maxHeight: `${actualHeight}px`,
      transform: 'none !important',
      margin: '0 !important',
      padding: '0 !important',
    };
    console.log('[Agent Dropdown] Position: below button', { actualHeight });
  } else {
    // 向上弹出，使用 bottom 定位确保紧贴按钮
    const availableHeight = spaceAbove - offsetY - topMargin;
    if (availableHeight >= preferredDropdownHeight) {
      actualHeight = preferredDropdownHeight;
    } else {
      actualHeight = Math.max(minDropdownHeight, availableHeight);
    }

    const bottom = vh - rect.top + offsetY;

    agentModeDropdownStyle.value = {
      position: 'fixed !important',
      width: `${dropdownWidth}px`,
      left: `${left}px`,
      bottom: `${bottom}px`, // 使用 bottom 定位，确保紧贴按钮
      maxHeight: `${actualHeight}px`,
      transform: 'none !important',
      margin: '0 !important',
      padding: '0 !important',
    };
    console.log('[Agent Dropdown] Position: above button', { actualHeight, bottom });
  }
};

const toggleAgentModeSelector = () => {
  // 互斥
  showMention.value = false;
  showModelSelector.value = false;

  showAgentModeSelector.value = !showAgentModeSelector.value;
  if (showAgentModeSelector.value) {
    if (!chatResources.isFresh('agents')) {
      void loadAgents(true);
    }
    // 多次更新位置确保准确
    nextTick(() => {
      updateAgentModeDropdownPosition();
      requestAnimationFrame(() => {
        updateAgentModeDropdownPosition();
        setTimeout(() => {
          updateAgentModeDropdownPosition();
        }, 50);
      });
    });
  }
}

const handleAgentNotReady = (
  agent: CustomAgent,
  labels: string[],
  keys: AgentNotReadyReasonKey[],
) => {
  showAgentNotReadyMessage(agent, labels, keys);
};

const handleSelectAgent = async (agent: CustomAgent) => {
  if (!chatResources.isFresh('models')) {
    await loadChatModels()
  }

  // 根据智能体的 agent_mode 判断是否为 Agent 模式
  const isAgentType = agent.config?.agent_mode === 'smart-reasoning';

  // 统一检查智能体是否就绪（内置和自定义智能体使用相同逻辑）
  const actualAgent = agent.is_builtin
    ? (agents.value.find(a => a.id === agent.id) || agent)
    : agent;

  const { keys: notReadyKeys, labels: notReadyReasons } = collectAgentNotReadyReasons(
    actualAgent,
    isAgentType,
  );

  if (notReadyReasons.length > 0) {
    showAgentModeSelector.value = false;
    showAgentNotReadyMessage(actualAgent, notReadyReasons, notReadyKeys);
    return;
  }

  settingsStore.selectAgent(agent.id);
  settingsStore.toggleAgent(!!isAgentType);

  // 同步智能体的配置状态：模型、知识库由 watch 同步
  // 1. 同步模型（选中的对话模型随智能体切换）
  const agentModel = agent.config?.model_id;
  if (agentModel && agentModel.trim() !== '') {
    selectedModelId.value = agentModel;
  } else {
    const lastPick = readLastChatModelID();
    if (lastPick) {
      selectedModelId.value = lastPick;
    }
  }

  showAgentModeSelector.value = false;

  // Only the two "mode-entry" built-ins are re-branded as "Normal / Agent Mode"
  // in the dropdown — the switched-on/off toasts only make sense for them.
  // Other built-ins (such as data analyst) share `is_builtin`
  // but should fall back to the generic agentSelected toast like custom agents,
  // otherwise selecting another built-in incorrectly says
  // "Switched to Intelligent Reasoning".
  const isModeBuiltin =
    agent.id === BUILTIN_QUICK_ANSWER_ID || agent.id === BUILTIN_SMART_REASONING_ID;
  const message = isModeBuiltin
    ? (isAgentType ? t('input.messages.agentSwitchedOn') : t('input.messages.agentSwitchedOff'))
    : t('input.messages.agentSelected', { name: agent.name });
  MessagePlugin.success(message);
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

  // 退格键：当输入框为空且有选中项时，删除最后一个选中项
  if (event.e.keyCode === 8) { // Backspace
    const textarea = getTextareaEl();
    if (textarea && textarea.selectionStart === 0 && textarea.selectionEnd === 0 && query.value === '') {
      const items = allSelectedItems.value;
      if (items.length > 0) {
        event.e.preventDefault();
        const lastItem = items[items.length - 1];
        removeSelectedItem(lastItem);
        return;
      }
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

const onPaste = (e: ClipboardEvent) => {
  const items = e.clipboardData?.items;
  if (!items) return;
  const imageFiles: File[] = [];
  for (const item of items) {
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile();
      if (file) imageFiles.push(file);
    }
  }
  if (imageFiles.length > 0 && isImageUploadEnabledByAgent.value) {
    e.preventDefault();
    addImageFiles(imageFiles);
  }
};

const onDrop = (e: DragEvent) => {
  e.preventDefault();
  const files = e.dataTransfer?.files;
  if (!files || files.length === 0) return;
  handleDroppedFiles(Array.from(files));
};

const onDragOver = (e: DragEvent) => {
  e.preventDefault();
};

const formatAgentNotReadyReasons = (
  reasonKeys: AgentNotReadyReasonKey[],
  isBuiltin: boolean,
): string[] => {
  return reasonKeys.map((key) => {
    if (key === 'summary_model') {
      return isBuiltin
        ? t('input.agentMissingSummaryModel')
        : t('input.customAgentMissingSummaryModel');
    }
    if (key === 'rerank_model') {
      return isBuiltin
        ? t('input.agentMissingRerankModel')
        : t('input.customAgentMissingRerankModel');
    }
    return t('input.agentMissingAllowedTools');
  });
};

const collectAgentNotReadyReasons = (
  agent: CustomAgent,
  isAgentMode: boolean,
): { keys: AgentNotReadyReasonKey[]; labels: string[] } => {
  const keys = getAgentNotReadyReasonKeys(agent.config, allModels.value, {
    isAgentMode,
  });
  return {
    keys,
    labels: formatAgentNotReadyReasons(keys, agent.is_builtin),
  };
};

// 显示智能体未就绪的消息（统一处理内置和自定义智能体）
const showAgentNotReadyMessage = (
  agent: CustomAgent,
  reasons: string[],
  reasonKeys?: AgentNotReadyReasonKey[],
) => {
  const reasonsText = formatLocalizedList(reasons, locale.value)

  const messageContent = h('div', { style: 'display: flex; flex-direction: column; gap: 8px; max-width: 320px;' }, [
    h(
      'span',
      { style: 'color: var(--td-text-color-primary); line-height: 1.5;' },
      t('input.agentNotReadyDetail', { agentName: agent.name, reasons: reasonsText }),
    ),
  ]);

  MessagePlugin.warning({
    content: () => messageContent,
    duration: 5000
  });
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

onBeforeRouteUpdate((to, from, next) => {
  clearvalue()
  next()
})

defineExpose({
  triggerSend(text: string) {
    if (!text.trim()) return;
    query.value = text;
    // nextTick(() => createSession(text));
  }
});

</script>
<template>
  <div class="answers-input sticky-ios-fix" @drop="onDrop" @dragover="onDragOver">
    <div class="rich-input-container" data-guide="chat-input">
      <div class="control-bar">
        <img src="../assets/img/voiceIcon.svg" :alt="$t('input.send')" />
      </div>
      <!-- 实际输入框 -->
      <t-textarea ref="textareaRef" v-model="query" :placeholder="inputPlaceholder" name="description"
        :autosize="{ minRows: 1, maxRows: 3 }" @keydown="onKeydown" @input="onInput"
        @compositionstart="onCompositionStart" @compositionend="onCompositionEnd" @paste="onPaste" />
      <div class="control-bar">
        <!-- 右侧控制按钮组 -->
        <!-- <div class="control-right"> -->
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

        <!-- </div> -->
      </div>
    </div>

    <!-- 输入框下方的免责声明（所有使用 InputField 的页面都会展示） -->
    <div class="answers-disclaimer">{{ $t('input.disclaimer') }}</div>
  </div>
</template>
<style scoped lang="less">
@import './css/chat-resource-chips.less';

.answers-input {
  z-index: 99;
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.answers-disclaimer {
  width: 100%;
  max-width: 100%;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  text-align: center;
}

.rich-input-container:deep {
  // position: relative;
  display: flex;
  align-items: stretch;
  justify-content: space-between;
  width: 100%;
  padding: 12px;
  max-height: calc(v-bind(viewportHeight) * 0.6px);
  overflow-y: auto;
  background: var(--td-bg-color-container, #FFF);
  border-radius: 12px;
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04), 0 8px 16px -4px rgba(0, 0, 0, 0.06);

  &:focus-within {
    border-color: var(--td-brand-color, #0B41CD);
  }

  .t-textarea {
    padding: 0;
  }
}

.selected-tags-inline {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  padding: 6px 12px 6px;
  border-bottom: 1px solid var(--td-component-stroke, #dcdcdc);
  background: var(--td-bg-color-container, #fff);
  border-radius: 11px 11px 0 0;
}

.mention-chip {
  .chat-resource-chip-surface();

  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 26px;
  padding: 3px 7px 3px 6px;
  border-radius: var(--td-radius-medium, 6px);
  box-sizing: border-box;
  font-size: 12px;
  font-weight: 500;
  cursor: default;
  transition: background 0.15s, border-color 0.15s;
  line-height: 18px;

  &:hover {
    .chat-resource-chip-hover();
  }
}

.mention-chip__icon-wrap {
  position: relative;
  display: inline-flex;
  width: 16px;
  height: 16px;
  flex: 0 1 auto;
  min-width: 0;
  align-items: center;
  justify-content: center;
}

.mention-chip__icon {
  font-size: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: inherit;
}

.mention-chip__name {
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: currentColor;
}

.mention-chip__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  margin-left: 1px;
  border-radius: 50%;
  font-size: 14px;
  line-height: 1;
  font-weight: 400;
  cursor: pointer;
  opacity: 0.5;
  transition: opacity 0.15s, background 0.15s, color 0.15s;
  color: currentColor;
  flex-shrink: 0;
}

.mention-chip:hover .mention-chip__remove {
  opacity: 0.85;
}

.mention-chip__remove:hover {
  opacity: 1;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-primary, #1f2937);
}

/* 标签表面保持中性，仅用图标颜色表达资源类型。 */
.mention-chip--kb {
  color: var(--td-text-color-primary);
}

.mention-chip--kb .mention-chip__icon-wrap {
  color: var(--td-brand-color, #0B41CD);
}

.mention-chip--faq {
  color: var(--td-text-color-primary);
}

.mention-chip--faq .mention-chip__icon-wrap {
  color: var(--rochekap-faq-color, #0052d9);
}

.mention-chip--file {
  color: var(--td-text-color-primary);
}

.mention-chip--file .mention-chip__icon-wrap {
  color: var(--td-text-color-secondary, #6b7280);
}

.mention-chip--tag,
.mention-chip--tool {
  color: var(--td-text-color-primary);
}

.mention-chip--tag .mention-chip__icon-wrap {
  color: #9f7aea;
}

.mention-chip--tool .mention-chip__icon-wrap {
  color: #b7791f;
}

/* 智能体预配置：虚线边框区分 */
.mention-chip--agent {
  border-style: dashed;
  border-color: var(--td-component-border);
}

.rich-input-container:not(:has(.selected-tags-inline)) :deep(.t-textarea__inner) {
  border-radius: 12px;
}

.control-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  max-height: 56px;
  z-index: 10;
  background: linear-gradient(to bottom, rgba(255, 255, 255, 0) 0%, var(--td-bg-color-container, #fff) 40%, var(--td-bg-color-container, #fff) 100%);
  pointer-events: auto;

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
  padding: 4px;
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

.agent-mode-btn {
  height: 28px;
  padding: 0 10px;
  min-width: auto;
  font-weight: 500;
  position: relative;
  border: .5px solid var(--td-component-border, #e7e7e7);
}

.agent-icon {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

.agent-btn-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 5px;
  flex-shrink: 0;
  color: var(--td-text-color-secondary, #666);
}

.agent-mode-text {
  font-size: 13px;
  color: var(--td-text-color-secondary, #666);
  font-weight: 500;
  white-space: nowrap;
  margin: 0 4px;
}

.control-icon {
  width: 18px;
  height: 18px;
}

.kb-btn {
  height: 28px;
  width: 30px;
  padding: 0;
  min-width: 30px;
  position: relative;

  &.active {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
    box-shadow: inset 0 0 0 1px var(--td-component-stroke);

    &:hover {
      background: var(--td-bg-color-secondarycontainer-hover);
    }
  }

  &.agent-controlled {
    cursor: not-allowed;
    opacity: 0.85;

    &:hover {
      background: var(--td-bg-color-secondarycontainer, #f5f5f5);
    }

    &.active:hover {
      background: var(--td-bg-color-secondarycontainer);
    }
  }
}

.kb-count {
  position: absolute;
  top: -5px;
  right: -5px;
  min-width: 15px;
  height: 15px;
  padding: 0 3px;
  background: var(--td-brand-color);
  color: var(--td-text-color-anti, #fff);
  font-size: 9px;
  font-weight: 600;
  line-height: 15px;
  border: 2px solid var(--td-bg-color-container);
  border-radius: var(--td-radius-round, 999px);
  box-sizing: content-box;
  display: flex;
  align-items: center;
  justify-content: center;
}

.kb-btn-text {
  font-size: 13px;
  color: var(--td-text-color-secondary, #666);
  font-weight: 500;
  white-space: nowrap;
}

.kb-btn.active .kb-btn-text {
  color: var(--td-brand-color);
}

/* Image upload */
.image-upload-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  min-width: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  color: var(--td-text-color-secondary, #666);

  &:hover {
    background: var(--td-bg-color-secondarycontainer-hover, #f0f0f0);
    color: var(--td-text-color-primary, #333);
  }

  &.active {
    background: rgba(20, 130, 250, 0.1);
    color: #0B41CD;
  }

  .image-count {
    position: absolute;
    top: -2px;
    right: -2px;
    background: #0B41CD;
    color: #fff;
    font-size: 10px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    line-height: 1;
  }
}

/* Attachment upload */
.attachment-upload-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  min-width: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  color: var(--td-text-color-secondary, #666);

  &:hover {
    background: var(--td-bg-color-secondarycontainer-hover, #f0f0f0);
    color: var(--td-text-color-primary, #333);
  }

  &.active {
    background: rgba(20, 130, 250, 0.1);
    color: #0B41CD;
  }

  .attachment-count {
    position: absolute;
    top: -2px;
    right: -2px;
    background: #0B41CD;
    color: #fff;
    font-size: 10px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    line-height: 1;
  }
}

.image-preview-bar {
  display: flex;
  gap: 8px;
  padding: 8px 12px 4px;
  flex-wrap: wrap;
}

.image-preview-item {
  position: relative;
  width: 60px;
  height: 60px;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--td-border-level-1-color, #e7e7e7);

  .image-preview-thumb {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .image-preview-remove {
    position: absolute;
    top: 2px;
    right: 2px;
    width: 16px;
    height: 16px;
    background: rgba(0, 0, 0, 0.5);
    color: #fff;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    cursor: pointer;
    line-height: 1;

    &:hover {
      background: rgba(0, 0, 0, 0.7);
    }
  }
}

:global(.input-field-tooltip) {
  .t-popup__content {
    box-shadow: var(--td-shadow-2);
    border: .5px solid var(--td-component-border, #e7e7e7);
  }
}

:global(.tooltip-with-link) {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: 220px;
  font-size: 12px;
  color: var(--td-text-color-primary, #333);
}

:global(.tooltip-with-link a) {
  color: var(--td-brand-color);
  font-weight: 500;
  text-decoration: none;
}

:global(.tooltip-with-link a:hover) {
  text-decoration: underline;
}

.dropdown-arrow {
  width: 10px;
  height: 10px;
  margin-left: 2px;
  transition: transform 0.12s;

  &.rotate {
    transform: rotate(180deg);
  }
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

/* 模型显示样式 */
.model-display {
  display: flex;
  align-items: center;
  margin-left: auto;
  flex-shrink: 0;

  &.agent-controlled {
    .model-selector-trigger {
      cursor: not-allowed;
      opacity: 0.5;
    }
  }
}

.model-selector-trigger {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  min-width: 100px;
  height: 22px;
  border-radius: 6px;
  border: .5px solid var(--td-component-border, #e7e7e7);
  transition: background 0.12s, border-color 0.12s;
  cursor: pointer;

  &:hover {
    background: var(--td-bg-color-secondarycontainer-hover, #e6e6e6);
  }

  &.disabled {
    opacity: 0.5;
    cursor: not-allowed;

    &:hover {
      background: var(--td-bg-color-secondarycontainer, #f5f5f5);
    }
  }
}

.model-selector-name {
  flex: 1;
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary, #666);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-dropdown-arrow {
  width: 10px;
  height: 10px;
  color: var(--td-text-color-placeholder, #999);
  flex-shrink: 0;
  transition: transform 0.12s;

  &.rotate {
    transform: rotate(180deg);
  }
}

.model-selector-trigger.disabled .model-dropdown-arrow {
  color: var(--td-text-color-placeholder, #999);
}

.model-selector-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: transparent;
  touch-action: none;
}

.model-selector-dropdown {
  position: fixed !important;
  z-index: 10000;
  background: var(--td-bg-color-container);
  border: .5px solid var(--td-component-border);
  border-radius: 10px;
  box-shadow: var(--td-shadow-2);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
  padding: 0 !important;
  transform: none !important;
  transform-origin: top left;
  animation: modelSelectorFadeIn 0.15s ease-out;
}

@keyframes modelSelectorFadeIn {
  from {
    opacity: 0;
    transform: scale(0.98);
  }

  to {
    opacity: 1;
    transform: scale(1);
  }
}

.model-selector-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-bottom: .5px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
}

.model-selector-content {
  flex: 1;
  min-height: 0;
  max-height: 260px;
  overflow-y: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
  padding: 6px 8px;
}

.model-selector-add {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 6px;
  border: .5px solid transparent;
  background: transparent;
  color: var(--td-brand-color);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.12s;

  .add-icon {
    font-size: 14px;
    line-height: 1;
    font-weight: 400;
  }

  &:hover {
    color: var(--td-brand-color-hover);
    background: var(--td-bg-color-secondarycontainer);
  }
}

.model-option {
  display: flex;
  align-items: center;
  padding: 6px 8px;
  cursor: pointer;
  transition: background 0.12s;
  border-radius: 6px;
  margin-bottom: 4px;

  &:last-child {
    margin-bottom: 0;
  }

  &:hover,
  &.selected {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.empty {
    color: var(--td-text-color-placeholder);
    cursor: default;
    text-align: center;
    padding: 20px 8px;

    &:hover {
      background: transparent;
    }
  }
}

.model-option-left {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
}

.model-option-icon {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--td-text-color-secondary);
}

.model-option-name-wrap {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  flex: 1;
}

.model-option-name {
  font-size: 12px;
  color: var(--td-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.model-option-raw-name {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

/* Agent 模式选择下拉菜单 */
.agent-mode-selector-overlay {
  position: fixed;
  inset: 0;
  z-index: 9998;
  background: transparent;
  touch-action: none;
}

.agent-mode-selector-dropdown {
  position: fixed !important;
  z-index: 9999;
  background: var(--td-bg-color-container, #fff);
  border-radius: 10px;
  box-shadow: var(--td-shadow-2, 0 6px 28px rgba(15, 23, 42, 0.08));
  border: 1px solid var(--td-component-border, #e7e9eb);
  overflow: hidden;
  padding: 6px 8px;
  min-width: 200px;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
  padding: 0 !important;
  transform: none !important;
}

.agent-mode-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  cursor: pointer;
  transition: background 0.12s;
  border-radius: 6px;
  position: relative;
  margin: 4px 6px;

  &:hover:not(.disabled) {
    background: var(--td-bg-color-container-hover, #f6f8f7);
  }

  &.disabled {
    opacity: 0.6;
    cursor: not-allowed;

    &:hover {
      background: transparent;
    }
  }

  &.selected {
    background: var(--td-brand-color-light, #eefdf5);

    .agent-mode-option-name {
      color: var(--td-success-color);
      font-weight: 700;
    }
  }
}

.agent-mode-option-main {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  min-width: 0;
}

.agent-mode-option-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-primary, #222);
  line-height: 1.4;
  transition: color 0.12s;
}

.agent-mode-option-desc {
  font-size: 11px;
  color: var(--td-text-color-secondary, #8b9196);
  line-height: 1.3;
}

.check-icon {
  width: 14px;
  height: 14px;
  color: var(--td-success-color);
  flex-shrink: 0;
  margin-left: 6px;
}

.agent-mode-warning {
  display: flex;
  align-items: center;
  margin-left: 6px;

  .warning-icon {
    color: var(--td-warning-color);
    font-size: 14px;
  }
}

.agent-mode-footer {
  padding: 6px 10px;
  border-top: 1px solid var(--td-component-border, #f2f4f5);
  margin-top: 2px;
  background: var(--td-bg-color-secondarycontainer, #fafcfc);
}

.agent-mode-link {
  color: var(--td-success-color);
  text-decoration: none;
  font-size: 11px;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  transition: all 0.12s;

  &:hover {
    color: var(--td-brand-color-active);
    text-decoration: underline;
  }
}
</style>
