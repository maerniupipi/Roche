<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, reactive, computed, nextTick } from "vue";
import { MessagePlugin } from "tdesign-vue-next";
import DocContent from "@/components/doc-content.vue";
import useKnowledgeBase from '@/hooks/useKnowledgeBase';
import { useRoute, useRouter } from 'vue-router';
import EmptyKnowledge from '@/components/empty-knowledge.vue';
import ContextualGuide from '@/components/ContextualGuide.vue';
import KnowledgeBreadcrumb, { type KBreadcrumbSegment } from '@/components/KnowledgeBreadcrumb.vue';
import { createSessions } from "@/api/chat/index";
import { useMenuStore } from '@/stores/menu';
import { useUIStore } from '@/stores/ui';
import { useAuthStore } from '@/stores/auth';
import { useChatResourcesStore } from '@/stores/chatResources';
import { useEditorResourcesStore } from '@/stores/editorResources';
import KnowledgeBaseEditorModal from './KnowledgeBaseEditorModal.vue';
const usemenuStore = useMenuStore();
const uiStore = useUIStore();
const authStore = useAuthStore();
const chatResources = useChatResourcesStore();
const editorResources = useEditorResourcesStore();
const router = useRouter();
import {
  batchQueryKnowledge,
  uploadKnowledgeFile,
  createKnowledgeFromURL,
  reparseKnowledge,
  cancelKnowledgeParse,
  batchReparseKnowledge,
  getKnowledgeSpans,
  getKnowledgeDetails,
} from "@/api/knowledge-base/index";
import { knowledgeSpansPayloadHasTrace } from '@/utils/knowledgeTrace';
import FAQEntryManager from './components/FAQEntryManager.vue';
import DocumentListView from './components/DocumentListView.vue';
import DocumentBatchBar from './components/DocumentBatchBar.vue';
import KbUploadSourceDropdown from './components/KbUploadSourceDropdown.vue';
import type { KnowledgeProcessOverrides } from '@/types/knowledgeProcess';
import { useUploadConfirmStore, type UploadConfirmResult } from '@/stores/uploadConfirm';
import {
  isKnowledgeParseInFlight,
  knowledgeNeedsStatusPolling,
} from './knowledgeStatusPolling';
import { listMoveTargets, moveKnowledge, getKnowledgeMoveProgress } from '@/api/knowledge-base';
import { useI18n } from 'vue-i18n';
import { useMarqueeSelect } from '@/hooks/useMarqueeSelect';
import { useKnowledgeDomains } from '@/composables/useKnowledgeDomains';
import type { ParserEngineInfo } from '@/api/system';
const route = useRoute();
const { t } = useI18n();
const kbId = computed(() => (route.params as any).kbId as string || '');
const kbInfo = ref<any>(null);
// 根据当前 KB 所属的知识域，从模块级单例缓存中取出对应的知识域名称。
// 缓存由侧栏 menu.vue 负责加载与刷新（监听 `knowledge-domain-changed` 事件），
// 这里直接复用；kbInfo 尚未加载完或缓存未就绪时由 fallback 处理。
const { domains: knowledgeDomains, load: loadDomains } = useKnowledgeDomains();
const currentKbDomainId = computed<number | null>(() => {
  const raw = kbInfo.value?.knowledge_domain_id;
  if (raw == null) return null;
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : null;
});
const currentKbDomainName = computed<string | null>(() => {
  if (currentKbDomainId.value == null) return null;
  return knowledgeDomains.value.find(d => d.id === currentKbDomainId.value)?.name ?? null;
});
const uploadSourceRef = ref<InstanceType<typeof KbUploadSourceDropdown> | null>(null);
const uploading = ref(false);
const kbLoading = ref(false);
const docListLoading = ref(true);
const isFAQ = computed(() => (kbInfo.value?.type || '') === 'faq');
const missingStorageEngine = computed(() => {
  if (!kbInfo.value || isFAQ.value) return false
  const spc = kbInfo.value.storage_provider_config
  return !spc || !spc.provider
})
const parserEngines = computed<ParserEngineInfo[]>(() => editorResources.parserEngines);

const supportedFileTypes = computed<Set<string>>(() => {
  const engines = parserEngines.value
  if (!engines.length) return new Set<string>()

  const rules: { file_types: string[]; engine: string }[] =
    kbInfo.value?.chunking_config?.parser_engine_rules || []

  const ruleMap = new Map<string, string>()
  for (const r of rules) {
    for (const ft of r.file_types) ruleMap.set(ft, r.engine)
  }

  const available = new Set<string>()
  const availableEngineNames = new Set(
    engines.filter(e => e.Available !== false).map(e => e.Name)
  )

  for (const engine of engines) {
    for (const ft of engine.FileTypes || []) {
      if (available.has(ft)) continue

      const explicitEngine = ruleMap.get(ft)
      if (explicitEngine) {
        if (availableEngineNames.has(explicitEngine)) available.add(ft)
      } else {
        if (engine.Available !== false) available.add(ft)
      }
    }
  }
  return available
})

const acceptFileTypes = computed(() =>
  [...supportedFileTypes.value].map(t => '.' + t).join(',')
)

const unsupportedFileTypes = computed<string[]>(() => {
  const engines = parserEngines.value
  if (!engines.length) return []

  const allTypes = new Set<string>()
  for (const engine of engines) {
    for (const ft of engine.FileTypes || []) allTypes.add(ft)
  }

  const supported = supportedFileTypes.value
  return [...allTypes].filter(ft => !supported.has(ft)).sort()
})

const goToParserSettings = () => {
  if (kbId.value) {
    uiStore.openKBSettings(kbId.value, 'parser')
  }
}

// Management is determined by the department/system administrator role or the
// effective permission returned by the server. Creator metadata is not an ACL.
const effectiveKBPermission = computed(() => kbInfo.value?.my_permission || '');

const canEdit = computed(() => {
  if (authStore.canManageKnowledge) return true;
  return effectiveKBPermission.value === 'manage';
});

const canMutateKnowledge = computed(() => canEdit.value);

const knowledgeList = ref<Array<{ id: string; name: string; type?: string }>>([]);
let { cardList, total, moreIndex, details, getKnowled, onVisibleChange: _onVisibleChange, getCardDetails, getfDetails } = useKnowledgeBase(kbId.value)

const showKbDetailContextualGuide = computed(() => {
  return Boolean(kbId.value)
    && !isFAQ.value
    && canEdit.value
    && !docListLoading.value
    && cardList.value.length === 0;
});

const onVisibleChange = (visible: boolean) => {
  _onVisibleChange(visible);
  if (!visible) {
    moveMenuMode.value = 'normal';
  }
};

/** Per-knowledge cache: whether /spans has a real trace (see knowledgeSpansPayloadHasTrace). */
const traceAvailableById = reactive<Record<string, boolean>>({});
const traceProbeInflight = new Set<string>();

function clearTraceAvailabilityCache() {
  for (const key of Object.keys(traceAvailableById)) {
    delete traceAvailableById[key];
  }
  traceProbeInflight.clear();
}

// Parse phases where the backend pipeline is still actively running
// (primary parse OR post-process fan-out). Trace data exists and the
// UI should treat the row as "in flight" rather than terminal.
function isParseInFlight(status?: string): boolean {
  return isKnowledgeParseInFlight(status);
}

function isTraceMenuVisible(item: KnowledgeCard): boolean {
  if (!item?.id) return false;
  if (isParseInFlight(item.parse_status)) {
    return true;
  }
  return traceAvailableById[item.id] === true;
}

async function probeTraceAvailable(item: KnowledgeCard) {
  const id = item.id;
  if (!id || traceProbeInflight.has(id)) return;
  if (isParseInFlight(item.parse_status)) {
    traceAvailableById[id] = true;
    return;
  }
  if (Object.prototype.hasOwnProperty.call(traceAvailableById, id)) return;
  traceProbeInflight.add(id);
  try {
    const res: any = await getKnowledgeSpans(id);
    traceAvailableById[id] = !!(res?.success && knowledgeSpansPayloadHasTrace(res.data));
  } catch {
    traceAvailableById[id] = false;
  } finally {
    traceProbeInflight.delete(id);
  }
}

let isCardDetails = ref(false);
let timeout: ReturnType<typeof setTimeout> | null = null;
let knowledgeScroll = ref()
let page = 1;
let pageSize = 35;
let scrollLoading = false;
const resetPage = () => { page = 1; scrollLoading = false; };

// Move state — inline in card menu
const moveMenuMode = ref<'normal' | 'targets' | 'confirm'>('normal');
const moveKnowledgeId = ref('');
const moveTargetKbs = ref<any[]>([]);
const moveTargetsLoading = ref(false);
const moveSelectedTargetId = ref('');
const moveSelectedTargetName = ref('');
const moveMode = ref<'reuse_vectors' | 'reparse'>('reuse_vectors');
const moveSubmitting = ref(false);
let movePollTimer: ReturnType<typeof setInterval> | null = null;

// Multi-select state — shared between the document list view.
// Vue 3.5 tracks Set#add/delete natively, so direct mutation is reactive.
const selectedIds = ref<Set<string>>(new Set());
let lastSelectedIndex = -1;
const batchReparsing = ref(false);
// IDs submitted for async batch reparse; hold optimistic pending until the worker updates DB.
const pendingReparseAck = ref<Set<string>>(new Set());

const applyOptimisticBatchReparse = (ids: string[]) => {
  const idSet = new Set(ids);
  for (const card of cardList.value) {
    if (!idSet.has(card.id)) continue;
    pendingReparseAck.value.add(card.id);
    card.parse_status = 'pending';
    card.summary_status = undefined;
    card.description = '';
    delete traceAvailableById[card.id];
    traceAvailableById[card.id] = true;
  }
};

const syncReparseAckFromServer = (ids: string[]) => {
  for (const id of ids) {
    if (!pendingReparseAck.value.has(id)) continue;
    const card = cardList.value.find((c) => c.id === id);
    if (card && isParseInFlight(card.parse_status)) {
      pendingReparseAck.value.delete(id);
    }
  }
};

const awaitBatchReparseReflection = async (ids: string[]) => {
  const maxPolls = 30;
  const delayMs = 400;
  for (let i = 0; i < maxPolls && pendingReparseAck.value.size > 0; i++) {
    await loadKnowledgeFiles(kbId.value);
    syncReparseAckFromServer(ids);
    applyOptimisticBatchReparse(Array.from(pendingReparseAck.value));
    await new Promise<void>((r) => setTimeout(r, delayMs));
  }
  pendingReparseAck.value.clear();
};

const confirmBatchReparse = async () => {
  if (batchReparsing.value || selectedIds.value.size === 0) return;
  const allIds = Array.from(selectedIds.value);
  const ids = allIds.filter((id) => {
    const item = cardList.value.find((c) => c.id === id);
    return !item || !isParseInFlight(item.parse_status);
  });
  const skipped = allIds.length - ids.length;
  if (ids.length === 0) {
    MessagePlugin.info(t('knowledgeBase.rebuildInProgress'));
    return;
  }
  if (skipped > 0) {
    MessagePlugin.warning(t('knowledgeBase.batchReparseSkippedInFlight', { count: skipped }));
  }
  batchReparsing.value = true;
  try {
    const res: any = await batchReparseKnowledge(kbId.value, ids);
    if (res?.success) {
      MessagePlugin.success(t('knowledgeBase.batchReparseSuccess', { count: ids.length }));
      applyOptimisticBatchReparse(ids);
      clearSelection();
      batchMode.value = false;
      void awaitBatchReparseReflection(ids);
    } else {
      MessagePlugin.error(res?.message || t('knowledgeBase.batchReparseFailed'));
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('knowledgeBase.batchReparseFailed'));
  } finally {
    batchReparsing.value = false;
  }
};


let docSearchDebounce: number | null = null;
const docSearchKeyword = ref('');
const selectedFileType = ref('');
const fileTypeOptions = computed(() => [
  { label: t('knowledgeBase.allFileTypes'), value: '' },
  { label: 'PDF', value: 'pdf' },
  { label: 'DOCX', value: 'docx' },
  { label: 'DOC', value: 'doc' },
  { label: 'PPTX', value: 'pptx' },
  { label: 'PPT', value: 'ppt' },
  { label: 'EPUB', value: 'epub' },
  { label: 'MHTML', value: 'mhtml' },
  { label: 'TXT', value: 'txt' },
  { label: 'MD', value: 'md' },
  { label: 'URL', value: 'url' },
  { label: t('knowledgeBase.typeManual'), value: 'manual' },
  { label: 'MP3', value: 'mp3' },
  { label: 'WAV', value: 'wav' },
  { label: 'M4A', value: 'm4a' },
  { label: 'FLAC', value: 'flac' },
  { label: 'OGG', value: 'ogg' },
]);
const selectedParseStatus = ref('');
const parseStatusOptions = computed(() => [
  { label: t('knowledgeBase.allParseStatuses'), value: '' },
  { label: t('knowledgeBase.parseStatusPending'), value: 'pending' },
  { label: t('knowledgeBase.parseStatusProcessing'), value: 'processing' },
  { label: t('knowledgeBase.parseStatusCompleted'), value: 'completed' },
  { label: t('knowledgeBase.parseStatusFailed'), value: 'failed' },
]);
const selectedSource = ref('');
// Source filter combines ingestion channels and the "manual"/"url" virtual
// sources that the backend routes onto the `type` column.
const sourceOptions = computed(() => [
  { label: t('knowledgeBase.allSources'), value: '' },
  { label: t('knowledgeBase.sourceUpload'), value: 'web' },
  { label: t('knowledgeBase.sourceUrl'), value: 'url' },
  { label: t('knowledgeBase.sourceManual'), value: 'manual' },
  { label: t('knowledgeBase.sourceApi'), value: 'api' },
  { label: t('knowledgeBase.sourceBrowserExtension'), value: 'browser_extension' },
  { label: t('knowledgeBase.channelFeishu'), value: 'feishu' },
  { label: t('knowledgeBase.channelNotion'), value: 'notion' },
  { label: t('knowledgeBase.channelYuque'), value: 'yuque' },
]);
// Date range as [start, end] in "YYYY-MM-DD" form (t-date-range-picker default).
const updatedTimeRange = ref<string[]>([]);
// Disable any date after today so users cannot filter into the future.
const disableFutureDate = { after: new Date(new Date().setHours(23, 59, 59, 999)) };
const filterParams = computed(() => {
  const [start, end] = updatedTimeRange.value || [];
  return {
    keyword: docSearchKeyword.value ? docSearchKeyword.value.trim() : undefined,
    file_type: selectedFileType.value || undefined,
    parse_status: selectedParseStatus.value || undefined,
    source: selectedSource.value || undefined,
    start_time: start ? `${start} 00:00:00` : undefined,
    end_time: end ? `${end} 23:59:59` : undefined,
  };
});
const getPageSize = () => {
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
  const itemHeight = 148;
  let itemsInView = Math.floor(viewportHeight / itemHeight) * 5;
  pageSize = Math.max(35, itemsInView);
}
getPageSize()

const loadKnowledgeFiles = (kbIdValue: string): Promise<void> => {
  if (!kbIdValue) return Promise.resolve();
  if (!isFAQ.value) {
    docListLoading.value = true;
  }
  return getKnowled(
    {
      page: 1,
      page_size: pageSize,
      ...filterParams.value,
    },
    kbIdValue,
  ).finally(() => {
    if (isCurrentKb(kbIdValue) && !isFAQ.value) {
      docListLoading.value = false;
    }
  });
};

const isCurrentKb = (targetKbId: string) => targetKbId === kbId.value;

const loadKnowledgeBaseInfo = async (targetKbId: string, force = false) => {
  if (!targetKbId) {
    kbInfo.value = null;
    cardList.value = [];
    total.value = 0;
    return;
  }
  kbLoading.value = true;
  try {
    // 先确保知识域单例缓存就绪：直接刷新详情页 URL 时 menu.vue 不触发
    // useKnowledgeDomains.load()，若不显式 await，_domains 会是空数组，
    // 后面 currentKbDomainName.find() 永远拿不到 name，面包屑退化为空 loading。
    // 已 loaded 时该调用是 no-op，幂等。
    await loadDomains();
    const data = await chatResources.fetchKnowledgeBaseById(targetKbId, force);
    if (!isCurrentKb(targetKbId)) return;

    kbInfo.value = data;
    if (!isFAQ.value) {
      loadKnowledgeFiles(targetKbId);
    } else {
      cardList.value = [];
      total.value = 0;
    }
  } catch (error) {
    if (!isCurrentKb(targetKbId)) return;

    console.error('Failed to load knowledge base info:', error);
    kbInfo.value = null;
    cardList.value = [];
    total.value = 0;
  } finally {
    if (isCurrentKb(targetKbId)) {
      kbLoading.value = false;
    }
  }
};

const loadKnowledgeList = async () => {
  try {
    await chatResources.ensureKnowledgeBases();
    knowledgeList.value = chatResources.rawKnowledgeBases.map((item: any) => ({
      id: String(item.id),
      name: item.name,
      type: item.type || 'document',
    }));
  } catch (error) {
    console.error('Failed to load knowledge list:', error);
  }
};

// 监听路由参数变化，重新获取知识库内容
watch(() => kbId.value, (newKbId, oldKbId) => {
  if (!newKbId) {
    kbInfo.value = null;
    cardList.value = [];
    total.value = 0;
    return;
  }
  if (newKbId === oldKbId && kbInfo.value) return;

  if (newKbId !== oldKbId) {
    clearTraceAvailabilityCache();
    cardList.value = [];
    total.value = 0;
    docListLoading.value = true;
    resetPage();
  }
  loadKnowledgeBaseInfo(newKbId);
}, { immediate: true });

// 监听文档搜索关键词变化
watch(docSearchKeyword, (newVal, oldVal) => {
  if (newVal === oldVal) return;
  if (docSearchDebounce) {
    window.clearTimeout(docSearchDebounce);
  }
  docSearchDebounce = window.setTimeout(() => {
    if (kbId.value) {
      resetPage();
      loadKnowledgeFiles(kbId.value);
    }
  }, 300);
});

// 监听文件类型筛选变化
watch(selectedFileType, (newVal, oldVal) => {
  if (newVal === oldVal) return;
  if (kbId.value) {
    resetPage();
    loadKnowledgeFiles(kbId.value);
  }
});

// 监听解析状态/来源/更新时间范围筛选变化（与文件类型行为一致）
watch([selectedParseStatus, selectedSource, updatedTimeRange], () => {
  if (kbId.value) {
    resetPage();
    loadKnowledgeFiles(kbId.value);
  }
}, { deep: true });

// 监听文件上传事件
const handleFileUploaded = (event: CustomEvent) => {
  const uploadedKbId = event.detail.kbId;
  console.log('接收到文件上传事件，上传的知识库ID:', uploadedKbId, '当前知识库ID:', kbId.value);
  if (uploadedKbId && uploadedKbId === kbId.value && !isFAQ.value) {
    console.log('匹配当前知识库，开始刷新文件列表');
    // 如果上传的文件属于当前知识库，使用 loadKnowledgeFiles 刷新文件列表
    resetPage(); // Reset page counter when reloading files after upload
    loadKnowledgeFiles(uploadedKbId);
  }
};


// 监听从菜单触发的URL导入事件
const handleOpenURLImportDialog = (event: CustomEvent) => {
  const eventKbId = event.detail.kbId;
  console.log('接收到URL导入对话框打开事件，知识库ID:', eventKbId, '当前知识库ID:', kbId.value);
  if (eventKbId && eventKbId === kbId.value && !isFAQ.value) {
    if (ensureDocumentKbReady()) {
      uploadSourceRef.value?.openUrlDialog();
    }
  }
};

// 把当前 KB 的 domainId 暴露给侧栏 menu.vue：
// 详情页路由（/platform/knowledge-bases/:kbId）下 route.path 不会匹配侧栏 child.path，
// menu.vue 通过监听 'active-kb-domain-changed' 事件把 activeKbDomainId 写入并据此判定子菜单激活态。
// 仅在 currentKbDomainId 变化时触发，避免重复 dispatch。
watch(currentKbDomainId, (domainId) => {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('active-kb-domain-changed', {
    detail: { domainId },
  }))
})

// Auto-open document detail when navigated with ?knowledge_id=xxx.
// Note: this runs both when the KB page mounts with a query param AND when a
// subsequent in-page navigation (e.g. from the global command palette) only
// changes the query without re-mounting the component — in that case kbId is
// the same and cardList may already be populated, so relying solely on the
// cardList watcher misses the trigger.
const pendingKnowledgeId = ref<string | null>(
  (route.query.knowledge_id as string) || null
);

const tryAutoOpenDocument = () => {
  if (!pendingKnowledgeId.value || !cardList.value?.length) return;
  const targetId = pendingKnowledgeId.value;
  pendingKnowledgeId.value = null;
  const card = cardList.value.find((c: KnowledgeCard) => c.id === targetId);
  if (card) {
    nextTick(() => openCardDetails(card));
  } else {
    nextTick(() => {
      openCardDetails({ id: targetId } as KnowledgeCard);
    });
  }
};

// React to later ?knowledge_id= changes on the same KB route (no remount).
watch(
  () => route.query.knowledge_id,
  (newId) => {
    if (typeof newId !== 'string' || !newId) return;
    pendingKnowledgeId.value = newId;
    // cardList is almost always already loaded at this point; if not, the
    // cardList watcher below will pick it up.
    tryAutoOpenDocument();
  },
);

// Dispatched by the global command palette when the user picks a chunk that
// lives in the KB they are already viewing — vue-router dedupes identical
// navigations, so we rely on this event instead of a URL change.
const handleOpenKnowledgeEvent = (e: Event) => {
  const detail = (e as CustomEvent<{ kbId: string; knowledgeId: string }>).detail;
  if (!detail || !detail.knowledgeId) return;
  if (detail.kbId && detail.kbId !== kbId.value) return;
  pendingKnowledgeId.value = detail.knowledgeId;
  tryAutoOpenDocument();
};

onMounted(() => {
  loadKnowledgeList();
  editorResources.ensureParserEngines();

  window.addEventListener('knowledgeFileUploaded', handleFileUploaded as EventListener);
  window.addEventListener('openURLImportDialog', handleOpenURLImportDialog as EventListener);
  window.addEventListener('rochekap:open-knowledge', handleOpenKnowledgeEvent as EventListener);
});

onUnmounted(() => {
  window.removeEventListener('knowledgeFileUploaded', handleFileUploaded as EventListener);
  window.removeEventListener('openURLImportDialog', handleOpenURLImportDialog as EventListener);
  window.removeEventListener('rochekap:open-knowledge', handleOpenKnowledgeEvent as EventListener);
  stopMovePoll();
  if (timeout !== null) {
    clearTimeout(timeout);
    timeout = null;
  }
});
watch(() => cardList.value, (newValue) => {
  if (isFAQ.value) return;
  docListLoading.value = false;

  // Auto-open document if navigated with ?knowledge_id=xxx
  if (pendingKnowledgeId.value && newValue?.length) {
    tryAutoOpenDocument();
  }

  let analyzeList = [];
  // Filter items that need polling: parsing in progress OR summary generation in progress
  analyzeList = newValue.filter(needsStatusPolling);
  if (timeout !== null) {
    clearTimeout(timeout);
    timeout = null;
  }
  if (analyzeList.length) {
    updateStatus(analyzeList)
  }

}, { deep: true })
type KnowledgeCard = {
  id: string;
  knowledge_base_id?: string;
  parse_status: string;
  summary_status?: string;
  description?: string;
  file_name?: string;
  original_file_name?: string;
  display_name?: string;
  title?: string;
  type?: string;
  updated_at?: string;
  file_type?: string;
  isMore?: boolean;
  metadata?: any;
  error_message?: string;
};
// needsStatusPolling decides whether a card row is still "in flight"
// enough that the doc list should keep refreshing it. Keep in sync with
// the backend lifecycle: pending / processing are the primary parse
// phase, finalizing is the post-process fan-out (summary / question /
// graph extract still running), and a `completed` row whose summary
// hasn't landed yet keeps polling so the description fills in.
const needsStatusPolling = (item: KnowledgeCard) => {
  return knowledgeNeedsStatusPolling(item);
};

const updateStatus = (analyzeList: KnowledgeCard[]) => {
  if (timeout !== null) {
    clearTimeout(timeout);
    timeout = null;
  }
  if (!analyzeList.length) return;

  let query = ``;
  for (let i = 0; i < analyzeList.length; i++) {
    query += `ids=${analyzeList[i].id}&`;
  }
  timeout = setTimeout(() => {
    batchQueryKnowledge(query).then((result: any) => {
      let hasChanges = false;
      if (result.success && result.data) {
        (result.data as KnowledgeCard[]).forEach((item: KnowledgeCard) => {
          const index = cardList.value.findIndex(card => card.id == item.id);
          if (index == -1) return;

          let parseStatus = item.parse_status;
          if (pendingReparseAck.value.has(item.id)) {
            if (isParseInFlight(item.parse_status)) {
              pendingReparseAck.value.delete(item.id);
            } else {
              parseStatus = 'pending';
            }
          }

          if (cardList.value[index].parse_status !== parseStatus ||
            cardList.value[index].summary_status !== item.summary_status ||
            cardList.value[index].description !== item.description) {
            // Always update the card data
            cardList.value[index].parse_status = parseStatus;
            cardList.value[index].summary_status = item.summary_status;
            cardList.value[index].description = item.description;
            delete traceAvailableById[item.id];
            hasChanges = true;
          }
        });
      }
      // If there are no changes, the watch won't trigger, so we must manually poll again
      // Even if there are changes, we can manually poll again just to be safe.
      // The watch will clear this timeout if it triggers.
      const stillPending = cardList.value.filter(needsStatusPolling);
      if (stillPending.length > 0) {
        updateStatus(stillPending);
      }
    }).catch((_err) => {
      // 错误处理
      const stillPending = cardList.value.filter(needsStatusPolling);
      if (stillPending.length > 0) {
        updateStatus(stillPending);
      }
    });
  }, 1500);
};


// 恢复文档处理状态（用于刷新后恢复）

const closeDoc = () => {
  isCardDetails.value = false;
};
const openCardDetails = (item: KnowledgeCard) => {
  isCardDetails.value = true;
  getCardDetails(item);
};

const closeCardMoreMenu = (index: number) => {
  if (cardList.value?.[index]) {
    cardList.value[index].isMore = false;
  }
  moreIndex.value = -1;
};

const onReparseMenuClick = (index: number, item: KnowledgeCard) => {
  if (isParseInFlight(item.parse_status)) {
    MessagePlugin.info(t('knowledgeBase.rebuildInProgress'));
  }
};

const handleMoveKnowledge = async (item: KnowledgeCard) => {
  moveKnowledgeId.value = item.id;
  moveMenuMode.value = 'targets';
  moveTargetsLoading.value = true;
  moveTargetKbs.value = [];
  try {
    const res: any = await listMoveTargets(kbId.value);
    moveTargetKbs.value = res.data || [];
  } catch {
    moveTargetKbs.value = [];
  } finally {
    moveTargetsLoading.value = false;
  }
};

const handleMoveSelectTarget = (kb: any) => {
  moveSelectedTargetId.value = kb.id;
  moveSelectedTargetName.value = kb.name;
  moveMode.value = 'reuse_vectors';
  moveMenuMode.value = 'confirm';
};

const handleMoveBack = () => {
  if (moveMenuMode.value === 'confirm') {
    moveMenuMode.value = 'targets';
  } else {
    moveMenuMode.value = 'normal';
  }
};

const handleMoveConfirm = async () => {
  if (!moveSelectedTargetId.value || moveSubmitting.value) return;
  moveSubmitting.value = true;
  try {
    const res: any = await moveKnowledge({
      knowledge_ids: [moveKnowledgeId.value],
      source_kb_id: kbId.value,
      target_kb_id: moveSelectedTargetId.value,
      mode: moveMode.value,
    });
    const taskId = res.data?.task_id;
    MessagePlugin.info(t('knowledgeBase.moveStarted'));
    // Close the card menu
    moveMenuMode.value = 'normal';
    cardList.value.forEach(c => { c.isMore = false; });

    if (taskId) {
      startMovePoll(taskId);
    } else {
      moveSubmitting.value = false;
      resetPage(); // Reset page counter when reloading files after move
      loadKnowledgeFiles(kbId.value);
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('knowledgeBase.moveFailed'));
    moveSubmitting.value = false;
  }
};

const startMovePoll = (taskId: string) => {
  if (movePollTimer) clearInterval(movePollTimer);
  movePollTimer = setInterval(async () => {
    try {
      const res: any = await getKnowledgeMoveProgress(taskId);
      const data = res.data;
      if (!data) return;
      if (data.status === 'completed') {
        stopMovePoll();
        moveSubmitting.value = false;
        const failed = data.failed || 0;
        if (failed > 0) {
          MessagePlugin.warning(t('knowledgeBase.moveCompletedWithErrors', { success: (data.processed || 0) - failed, failed }));
        } else {
          MessagePlugin.success(t('knowledgeBase.moveCompleted'));
        }
        resetPage(); // Reset page counter when reloading files after move completion
        loadKnowledgeFiles(kbId.value);
      } else if (data.status === 'failed') {
        stopMovePoll();
        moveSubmitting.value = false;
        MessagePlugin.error(t('knowledgeBase.moveFailed'));
      }
    } catch {
      // ignore poll errors
    }
  }, 2000);
};

const stopMovePoll = () => {
  if (movePollTimer) {
    clearInterval(movePollTimer);
    movePollTimer = null;
  }
};

const manualEditorSuccess = ({ kbId: savedKbId }: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => {
  if (savedKbId === kbId.value && !isFAQ.value) {
    resetPage(); // Reset page counter when reloading files after manual edit
    loadKnowledgeFiles(savedKbId);
  }
};

const documentTitle = computed(() => {
  if (kbInfo.value?.name) {
    return `${kbInfo.value.name} · ${t('knowledgeEditor.document.title')}`;
  }
  return t('knowledgeEditor.document.title');
});

const ensureDocumentKbReady = () => {
  if (isFAQ.value) {
    MessagePlugin.warning(t('knowledgeBase.operationNotSupportedForType'));
    return false;
  }
  if (!kbId.value) {
    MessagePlugin.warning(t('knowledgeEditor.messages.missingId'));
    return false;
  }
  if (!kbInfo.value || !kbInfo.value.summary_model_id) {
    MessagePlugin.warning(t('knowledgeBase.notInitialized'));
    return false;
  }
  // Embedding model only required when RAG indexing is enabled
  const strategy = (kbInfo.value as any).indexing_strategy
  const needsEmbedding = !strategy || strategy.vector_enabled || strategy.keyword_enabled
  if (needsEmbedding && !kbInfo.value.embedding_model_id) {
    MessagePlugin.warning(t('knowledgeBase.notInitialized'));
    return false;
  }
  if (missingStorageEngine.value) {
    MessagePlugin.warning(t('knowledgeBase.missingStorageEngineUpload'));
    return false;
  }
  return true;
};


const IMAGE_EXTENSIONS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp'];
const AUDIO_EXTENSIONS = ['mp3', 'wav', 'm4a', 'flac', 'ogg'];

const uploadConfirmStore = useUploadConfirmStore();

const getUploadFolderPath = (file: File) => {
  const relativePath = (file as any).webkitRelativePath;
  return relativePath
    ? relativePath.split('/').slice(0, -1).filter(Boolean).join('/')
    : '';
};

const showUploadResultMessages = (
  successCount: number,
  failCount: number,
  totalCount: number,
  mode: 'document' | 'folder',
) => {
  if (mode === 'folder') {
    if (failCount === 0) {
      MessagePlugin.success(t('knowledgeBase.uploadAllSuccess', { count: successCount }));
    } else if (successCount > 0) {
      MessagePlugin.warning(t('knowledgeBase.uploadPartialSuccess', { success: successCount, fail: failCount }));
    } else {
      MessagePlugin.error(t('knowledgeBase.uploadAllFailed'));
    }
    return;
  }

  if (totalCount === 1) {
    if (successCount === 1) {
      MessagePlugin.success(t('knowledgeBase.uploadSuccess'));
    }
    return;
  }

  if (failCount === 0) {
    MessagePlugin.success(t('knowledgeBase.allUploadSuccess', { count: successCount }));
  } else if (successCount > 0) {
    MessagePlugin.warning(t('knowledgeBase.partialUploadSuccess', { success: successCount, fail: failCount }));
  } else {
    MessagePlugin.error(t('knowledgeBase.allUploadFailed', { count: failCount }));
  }
};

const executeUploadBatch = async (
  files: File[],
  options: { processConfig?: KnowledgeProcessOverrides } = {},
) => {
  const targetKbId = kbId.value;
  if (!targetKbId || files.length === 0) {
    return { successCount: 0, failCount: files.length };
  }

  let successCount = 0;
  let failCount = 0;
  const totalCount = files.length;
  const hasFolderPaths = files.some((file) => {
    const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath;
    return !!relativePath;
  });

  for (const file of files) {
    try {
      const uploadData: {
        file: File
        folder_path?: string
        process_config?: KnowledgeProcessOverrides
      } = { file };

      const folderPath = getUploadFolderPath(file);
      if (folderPath) uploadData.folder_path = folderPath;
      if (options.processConfig) {
        uploadData.process_config = options.processConfig;
      }

      const responseData: any = await uploadKnowledgeFile(targetKbId, uploadData);
      const isSuccess = responseData?.success || responseData?.code === 200 || responseData?.status === 'success' || (!responseData?.error && responseData);
      if (isSuccess) {
        successCount++;
      } else {
        failCount++;
        if (totalCount === 1) {
          let errorMessage = t('knowledgeBase.uploadFailed');
          if (responseData?.error?.message) {
            errorMessage = responseData.error.message;
          } else if (responseData?.message) {
            errorMessage = responseData.message;
          }
          if (responseData?.code === 'duplicate_file' || responseData?.error?.code === 'duplicate_file') {
            errorMessage = t('knowledgeBase.fileExists');
          }
          MessagePlugin.error(errorMessage);
        }
      }
    } catch (error: any) {
      failCount++;
      if (totalCount === 1) {
        let errorMessage = error?.error?.message || error?.message || t('knowledgeBase.uploadFailed');
        if (error?.code === 'duplicate_file') {
          errorMessage = t('knowledgeBase.fileExists');
        }
        MessagePlugin.error(errorMessage);
      }
    }
  }

  if (successCount > 0) {
    window.dispatchEvent(new CustomEvent('knowledgeFileUploaded', {
      detail: { kbId: targetKbId },
    }));
  }

  showUploadResultMessages(successCount, failCount, totalCount, hasFolderPaths ? 'folder' : 'document');
  return { successCount, failCount };
};

const executeUrlImport = async (url: string, processConfig?: KnowledgeProcessOverrides) => {
  const targetKbId = kbId.value;
  if (!targetKbId) {
    MessagePlugin.error(t('error.missingKbId'));
    return;
  }

  try {
    const responseData: any = await createKnowledgeFromURL(targetKbId, {
      url,
      process_config: processConfig,
    });
    window.dispatchEvent(new CustomEvent('knowledgeFileUploaded', {
      detail: { kbId: targetKbId },
    }));
    const isSuccess = responseData?.success || responseData?.code === 200 || responseData?.status === 'success' || (!responseData?.error && responseData);
    if (isSuccess) {
      MessagePlugin.success(t('knowledgeBase.urlImportSuccess'));
    } else {
      let errorMessage = t('knowledgeBase.urlImportFailed');
      if (responseData?.error?.message) {
        errorMessage = responseData.error.message;
      } else if (responseData?.message) {
        errorMessage = responseData.message;
      }
      if (responseData?.code === 'duplicate_url' || responseData?.error?.code === 'duplicate_url') {
        errorMessage = t('knowledgeBase.urlExists');
      }
      MessagePlugin.error(errorMessage);
    }
  } catch (error: any) {
    let errorMessage = error?.error?.message || error?.message || t('knowledgeBase.urlImportFailed');
    if (error?.code === 'duplicate_url') {
      errorMessage = t('knowledgeBase.urlExists');
    }
    MessagePlugin.error(errorMessage);
  }
};

const handleUploadConfirmResult = async (result: UploadConfirmResult) => {
  if (result.mode === 'manual') {
    return;
  }

  const files = result.files || [];
  const urls = result.urls || [];
  const processConfig = result.processConfig;

  if (files.length > 0) {
    const hasFolderPaths = files.some((file) => {
      const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath;
      return !!relativePath && relativePath.split('/').length > 2;
    });
    if (hasFolderPaths) {
      MessagePlugin.info(t('knowledgeBase.uploadingFolder', { total: files.length }));
    }
    await executeUploadBatch(files, { processConfig });
  }

  for (const url of urls) {
    await executeUrlImport(url, processConfig);
  }
};

const openUploadConfirmDialog = async (files: File[], urls: string[] = []) => {
  if (!kbInfo.value) return;
  if (files.length === 0 && urls.length === 0) return;
  try {
    const result = await uploadConfirmStore.open({
      mode: 'file',
      kbInfo: kbInfo.value,
      files,
      urls,
      acceptFileTypes: acceptFileTypes.value,
      supportedFileTypes: [...supportedFileTypes.value],
    });
    await handleUploadConfirmResult(result);
  } catch {
    // cancelled
  }
};

const handleUploadSourceFiles = (files: File[]) => {
  if (!ensureDocumentKbReady()) return;
  if (files.length === 0) return;
  openUploadConfirmDialog(files);
};

const handleUploadSourceUrl = (url: string) => {
  if (!ensureDocumentKbReady()) return;
  openUploadConfirmDialog([], [url]);
};

const handleManualCreate = () => {
  if (!ensureDocumentKbReady()) return;
  uiStore.openManualEditor({
    mode: 'create',
    kbId: kbId.value,
    status: 'draft',
    onSuccess: manualEditorSuccess,
  });
};

const handleOpenKBSettings = () => {
  if (!kbId.value) {
    MessagePlugin.warning(t('knowledgeEditor.messages.missingId'));
    return;
  }
  uiStore.openKBSettings(kbId.value);
};

// 跳到当前 KB 所属的知识域子路由，而不是默认的「第一个域」：
// 这样点击面包屑第一级（显示当前域名称）的体验与文字一致。
const handleNavigateToKbList = () => {
  const domainId = currentKbDomainId.value;
  if (domainId != null) {
    router.push({ name: 'knowledgeBaseListByDomain', params: { domainId: String(domainId) } });
    return;
  }
  router.push('/platform/knowledge-bases');
};

// 面包屑的路由决策：KnowledgeBreadcrumb 保持纯展示，路由目标由这里根据段 key 决定。
// KB 详情页 3 段：知识库列表 / 当前知识域 / 当前 KB（不可点击）。
const detailBreadcrumbSegments = computed<KBreadcrumbSegment[]>(() => [
  { key: 'root', label: t('menu.knowledgeBase') },
  {
    key: 'domain',
    label: currentKbDomainName.value ?? '',
    loading: !currentKbDomainName.value,
  },
  {
    key: 'kb',
    label: kbInfo.value?.name ?? '',
    loading: !kbInfo.value,
    disabled: true,
  },
]);
const onBreadcrumbNavigate = (seg: KBreadcrumbSegment) => {
  if (seg.key === 'root') {
    router.push('/platform/knowledge-bases');
  } else if (seg.key === 'domain') {
    handleNavigateToKbList();
  }
};

const handleKnowledgeDropdownSelect = (data: { value: string }) => {
  if (!data?.value) return;
  if (data.value === kbId.value) return;
  router.push(`/platform/knowledge-bases/${data.value}`);
};

const handleManualEdit = (index: number, item: KnowledgeCard) => {
  if (isFAQ.value) return;
  if (cardList.value[index]) {
    cardList.value[index].isMore = false;
  }
  uiStore.openManualEditor({
    mode: 'edit',
    kbId: item.knowledge_base_id || kbId.value,
    knowledgeId: item.id,
    onSuccess: manualEditorSuccess,
  });
};

// Opens ONLY the trace drawer for this card — does NOT pop the
// document detail drawer behind it. The trace drawer attaches to
// body so it renders independent of its host's visibility; we just
// need `details` populated so the timeline component knows which
// knowledge_id to fetch. getCardDetails resets details synchronously
// then fills asynchronously, so we re-stamp the id/parse_status
// right after the call to avoid the brief empty-id window that
// would otherwise prevent the drawer from mounting.
const docContentRef = ref<any>(null);
const handleViewTrace = (index: number, item: KnowledgeCard) => {
  if (cardList.value[index]) {
    cardList.value[index].isMore = false;
  }
  moreIndex.value = -1;
  getCardDetails(item);
  details.id = item.id;
  details.parse_status = item.parse_status;
  nextTick(() => {
    docContentRef.value?.openTimeline?.();
  });
};

const confirmRebuildKnowledge = async (index: number, item: KnowledgeCard) => {
  if (isFAQ.value) return;
  if (!canEdit.value) return;
  if (!item?.id) {
    MessagePlugin.warning(t('knowledgeEditor.messages.missingId'));
    return;
  }
  if (isParseInFlight(item.parse_status)) {
    MessagePlugin.info(t('knowledgeBase.rebuildInProgress'));
    return;
  }
  closeCardMoreMenu(index);

  // No KB context to seed the dialog defaults — fall back to a direct reparse
  // that reuses the overrides stored at upload time.
  if (!kbInfo.value) {
    await submitReparse(item.id);
    return;
  }

  // Prefill the confirm dialog with the overrides this doc was last parsed with.
  let processOverrides: KnowledgeProcessOverrides | null = item.metadata?.process_overrides ?? null;
  let fileName = item.file_name || item.title || '';
  let fileType = item.file_type || '';
  try {
    const detail: any = await getKnowledgeDetails(item.id);
    if (detail?.success && detail.data) {
      processOverrides = detail.data.metadata?.process_overrides ?? processOverrides;
      fileName = detail.data.file_name || detail.data.title || fileName;
      fileType = detail.data.file_type || fileType;
    }
  } catch {
    // fall back to the list item's fields
  }

  try {
    const result = await uploadConfirmStore.open({
      mode: 'reparse',
      kbInfo: kbInfo.value,
      reparse: { knowledgeId: item.id, fileName, fileType, processOverrides },
    });
    if (result.mode === 'reparse' && result.reparse) {
      await submitReparse(result.reparse.knowledgeId, result.processConfig);
    }
  } catch {
    // cancelled
  }
};

const submitReparse = async (id: string, processConfig?: KnowledgeProcessOverrides) => {
  try {
    await reparseKnowledge(id, processConfig ? { process_config: processConfig } : undefined);
    delete traceAvailableById[id];
    traceAvailableById[id] = true;
    MessagePlugin.success(t('knowledgeBase.rebuildSubmitted'));
    resetPage();
    loadKnowledgeFiles(kbId.value);
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('knowledgeBase.rebuildFailed'));
  }
};

const handleScroll = () => {
  if (isFAQ.value) return;
  if (docListLoading.value) return;
  if (scrollLoading) return;
  const currentKbId = kbId.value;
  if (!currentKbId) return;
  const element = knowledgeScroll.value;
  if (element) {
    let pageNum = Math.ceil(total.value / pageSize)
    const { scrollTop, scrollHeight, clientHeight } = element;
    if (scrollTop + clientHeight >= scrollHeight - 10) {
      if (cardList.value.length < total.value && page < pageNum) {
        page++;
        scrollLoading = true;
        getKnowled({ page, page_size: pageSize, ...filterParams.value }, currentKbId).finally(() => {
          if (isCurrentKb(currentKbId)) {
            scrollLoading = false;
          }
        });
      }
    }
  }
};
const getDoc = (page: number) => {
  getfDetails(details.id, page)
};

const toggleSelectRow = (id: string, checked: boolean, shiftKey?: boolean) => {
  const items = cardList.value || [];
  const idx = items.findIndex((i: KnowledgeCard) => i.id === id);
  if (shiftKey && lastSelectedIndex >= 0 && idx >= 0) {
    const [s, e] = idx < lastSelectedIndex
      ? [idx, lastSelectedIndex]
      : [lastSelectedIndex, idx];
    for (let i = s; i <= e; i++) {
      if (checked) selectedIds.value.add(items[i].id);
      else selectedIds.value.delete(items[i].id);
    }
  } else {
    if (checked) selectedIds.value.add(id);
    else selectedIds.value.delete(id);
  }
  lastSelectedIndex = idx;
};

const toggleSelectAll = (checked: boolean) => {
  if (checked) {
    for (const item of cardList.value || []) selectedIds.value.add(item.id);
  } else {
    for (const item of cardList.value || []) selectedIds.value.delete(item.id);
  }
};

const clearSelection = () => {
  selectedIds.value.clear();
  lastSelectedIndex = -1;
};

// Batch (multi-select) mode mirrors the session list's "批量管理" UX: while off,
// no checkbox is rendered so the title doesn't jitter on hover; while on,
// checkboxes are persistent and clicking a card toggles its selection.
const batchMode = ref(false);
const toggleBatchMode = () => {
  batchMode.value = !batchMode.value;
  if (!batchMode.value) clearSelection();
};
// "取消选择" / 退出批量管理：清空选择，并退出 grid 视图下的批量模式。
const handleBatchCancel = () => {
  clearSelection();
  batchMode.value = false;
};
// Triggered from a card / row "..." menu — match the session-list UX where
// the menu item simply opens batch mode (no auto-selection).
const handleEnterBatchFromCard = (item: any) => {
  if (item) item.isMore = false;
  moreIndex.value = -1;
  clearSelection();
  batchMode.value = true;
};
const {
  onContainerMouseDown: onDocMarqueeMouseDown,
  marqueeVisible: docMarqueeVisible,
  marqueeMode: docMarqueeMode,
  boxStyle: docMarqueeBoxStyle,
  shouldSuppressClick: shouldSuppressDocClick,
} = useMarqueeSelect({
  containerRef: knowledgeScroll,
  itemSelector: '.knowledge-card[data-select-id], .doc-list-row[data-select-id]',
  selectedIds,
  getItemId: (el) => el.dataset.selectId || null,
  enabled: computed(() => canEdit.value && !isFAQ.value && cardList.value.length > 0),
  onSelectionStart: () => {
    batchMode.value = true;
  },
});

const isManualDraftKnowledge = (item: KnowledgeCard) =>
  item.type === 'manual' && item.parse_status === 'draft';

const openKnowledgeItem = (item: KnowledgeCard) => {
  if (shouldSuppressDocClick()) return;
  if (canEdit.value && isManualDraftKnowledge(item)) {
    const index = cardList.value.findIndex((c) => c.id === item.id);
    if (index >= 0) {
      handleManualEdit(index, item);
      return;
    }
  }
  openCardDetails(item);
};

const confirmCancelParseKnowledge = async (item: KnowledgeCard) => {
  if (!item?.id) return;
  try {
    await cancelKnowledgeParse(item.id);
    MessagePlugin.success(t('knowledgeBase.cancelParseSubmitted'));
    loadKnowledgeFiles(kbId.value);
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('knowledgeBase.cancelParseFailed'));
  }
};

// Bridge list-view actions back to existing per-card handlers.
const handleListAction = (
  action: 'edit' | 'reparse' | 'cancel-parse' | 'move' | 'view-trace' | 'batch-manage',
  item: KnowledgeCard,
) => {
  const idx = (cardList.value || []).findIndex((i: KnowledgeCard) => i.id === item.id);
  if (action === 'edit') return handleManualEdit(idx, item);
  if (action === 'reparse') return confirmRebuildKnowledge(idx, item);
  if (action === 'cancel-parse') return confirmCancelParseKnowledge(item);
  if (action === 'move') return handleMoveKnowledge(item);
  if (action === 'view-trace') return handleViewTrace(idx, item);
  if (action === 'batch-manage') return handleEnterBatchFromCard(item);
};

// Clear selection on filter/kb change to avoid acting on hidden items.
watch(
  [docSearchKeyword, selectedFileType, selectedParseStatus, selectedSource, updatedTimeRange, kbId],
  () => {
    clearSelection();
  },
);

// After cardList reloads: stable keys rely on correct indices for shift-range; clamp anchor index.
watch(cardList, () => {
  const items = cardList.value || [];
  const n = items.length;
  if (lastSelectedIndex >= n) {
    lastSelectedIndex = n > 0 ? n - 1 : -1;
  }
  if (moreIndex.value >= n) {
    moreIndex.value = -1;
  }
  if (selectedIds.value.size === 0) return;
  const visible = new Set(items.map((i: KnowledgeCard) => i.id));
  for (const id of selectedIds.value) {
    if (!visible.has(id)) selectedIds.value.delete(id);
  }
}, { deep: false });

// 处理知识库编辑成功后的回调
const handleKBEditorSuccess = (kbIdValue: string) => {
  chatResources.invalidateKnowledgeBaseDetail(kbIdValue);
  chatResources.invalidate('knowledgeBases');
  loadKnowledgeList();
  if (kbIdValue === kbId.value) {
    loadKnowledgeBaseInfo(kbIdValue, true);
  }
};

const getTitle = (session_id: string, value: string) => {
  const now = new Date().toISOString();
  let obj = {
    title: t('knowledgeBase.newSession'),
    path: `chat/${session_id}`,
    id: session_id,
    isMore: false,
    isNoTitle: true,
    created_at: now,
    updated_at: now
  };
  usemenuStore.changeIsFirstSession(true);
  usemenuStore.changeFirstQuery(value);
  router.push(`/platform/chat/${session_id}`);
};

async function createNewSession(value: string): Promise<void> {
  // Session 不再和知识库绑定，直接创建 Session
  createSessions({}).then(res => {
    if (res.data && res.data.id) {
      getTitle(res.data.id, value);
    } else {
      // 错误处理
      console.error(t('knowledgeBase.createSessionFailed'));
    }
  }).catch(error => {
    console.error(t('knowledgeBase.createSessionError'), error);
  });
}
</script>

<template>
  <template v-if="!isFAQ">
    <div class="knowledge-layout">
      <div class="document-header">
        <div class="document-header-title">
          <div class="document-title-row">
            <KnowledgeBreadcrumb :segments="detailBreadcrumbSegments" @navigate="onBreadcrumbNavigate" />
          </div>
        </div>
      </div>
      <div class="knowledge-main">
        <div class="doc-card-area">
            <div class="doc-filter-bar">
              <t-input v-model.trim="docSearchKeyword" :placeholder="$t('knowledgeBase.docSearchPlaceholder')" clearable
                class="doc-search-input" @clear="loadKnowledgeFiles(kbId)" @enter="loadKnowledgeFiles(kbId)">
                <template #prefix-icon>
                  <t-icon name="search" size="16px" />
                </template>
              </t-input>
              <div class="doc-filter-bar__filters">
                <div class="doc-filter-field">
                  <t-select v-model="selectedFileType" :options="fileTypeOptions"
                    :placeholder="$t('knowledgeBase.fileTypeFilter')" class="doc-type-select doc-filter-field__control"
                    clearable>
                    <template #prefixIcon>
                      <t-icon name="file" size="16px" />
                    </template>
                  </t-select>
                </div>
                <div class="doc-filter-field">
                  <t-select v-model="selectedParseStatus" :options="parseStatusOptions"
                    :placeholder="$t('knowledgeBase.parseStatusFilter')"
                    class="doc-type-select doc-filter-field__control" clearable>
                    <template #prefixIcon>
                      <t-icon name="check-circle" size="16px" />
                    </template>
                  </t-select>
                </div>
                <div class="doc-filter-field">
                  <t-select v-model="selectedSource" :options="sourceOptions"
                    :placeholder="$t('knowledgeBase.sourceFilter')" class="doc-type-select doc-filter-field__control"
                    clearable>
                    <template #prefixIcon>
                      <t-icon name="link" size="16px" />
                    </template>
                  </t-select>
                </div>
                <div class="doc-filter-field doc-filter-field--wide">
                  <t-date-range-picker v-model="updatedTimeRange"
                    :placeholder="[$t('knowledgeBase.updatedTimeFrom'), $t('knowledgeBase.updatedTimeTo')]"
                    :disable-date="disableFutureDate" class="doc-date-range doc-filter-field__control" clearable
                    allow-input>
                    <template #prefixIcon>
                      <t-icon name="time" size="16px" />
                    </template>
                  </t-date-range-picker>
                </div>
              </div>
              <div class="doc-filter-bar__trailing">
                <div v-if="canEdit" class="doc-filter-actions">
                  <KbUploadSourceDropdown ref="uploadSourceRef" :accept-file-types="acceptFileTypes"
                    :supported-file-types="[...supportedFileTypes]" include-manual trigger-icon="file-add"
                    trigger-class="content-bar-icon-btn" data-guide="kb-detail-add-doc"
                    :tooltip="t('knowledgeBase.addDocument')" placement="bottom-right" @files="handleUploadSourceFiles"
                    @url="handleUploadSourceUrl" @manual="handleManualCreate" />
                </div>
              </div>
            </div>
            <div class="doc-scroll-container"
              :class="{ 'is-empty': !cardList.length && !docListLoading, 'is-marquee-active': docMarqueeVisible }"
              ref="knowledgeScroll" @scroll="handleScroll" @mousedown="onDocMarqueeMouseDown">
              <div v-if="docMarqueeVisible" class="doc-marquee-box"
                :class="{ 'is-add': docMarqueeMode === 'add', 'is-subtract': docMarqueeMode === 'subtract' }"
                :style="docMarqueeBoxStyle" aria-hidden="true" />
              <!-- 文档骨架屏 -->
              <div v-if="docListLoading && cardList.length === 0" class="doc-card-list doc-card-list-animated">
                <div v-for="n in 8" :key="'doc-skel-' + n" class="knowledge-card knowledge-card-skeleton">
                  <div class="card-content">
                    <div class="card-content-nav">
                      <t-skeleton animation="gradient" :row-col="[{ width: '70%', height: '18px' }]" />
                    </div>
                    <t-skeleton animation="gradient"
                      :row-col="[{ width: '100%', height: '14px' }, { width: '60%', height: '14px' }]" />
                  </div>
                  <div class="card-bottom">
                    <t-skeleton animation="gradient"
                      :row-col="[[{ width: '80px', height: '14px' }, { width: '40px', height: '18px', type: 'rect' }]]" />
                  </div>
                </div>
              </div>
              <template v-else-if="cardList.length">
                <DocumentListView :items="cardList" :selected-ids="selectedIds" :can-edit="canEdit"
                  :can-mutate-knowledge="canMutateKnowledge" :trace-visible-ids="traceAvailableById"
                  :move-menu-mode="moveMenuMode" :move-target-kbs="moveTargetKbs"
                  :move-targets-loading="moveTargetsLoading" :move-selected-target-name="moveSelectedTargetName"
                  :move-mode="moveMode" :move-submitting="moveSubmitting" @open="(item: any) => openKnowledgeItem(item)"
                  @toggle-row="toggleSelectRow" @toggle-all="toggleSelectAll"
                  @action="(action: any, item: any) => handleListAction(action, item)"
                  @probe-trace="(item: any) => probeTraceAvailable(item)"
                  @move-select-target="(kb: any) => handleMoveSelectTarget(kb)" @move-back="handleMoveBack"
                  @move-confirm="handleMoveConfirm" @update:move-mode="(mode: any) => moveMode = mode"
                  @reset-move-state="moveMenuMode = 'normal'" />
              </template>
              <template v-else-if="!docListLoading">
                <div class="doc-empty-state">
                  <EmptyKnowledge />
                </div>
              </template>
            </div>
            <div class="doc-batch-bar-anchor" v-show="batchMode || selectedIds.size > 0">
              <DocumentBatchBar :count="selectedIds.size" :reparse-loading="batchReparsing"
                :visible="batchMode || selectedIds.size > 0" @cancel="handleBatchCancel"
                @reparse="confirmBatchReparse" />
            </div>
          </div>
        </div>

      <!-- Document content drawer -->
      <DocContent ref="docContentRef" :visible="isCardDetails" :details="details" :canEditKB="canEdit"
        @closeDoc="closeDoc" @getDoc="getDoc">
      </DocContent>
    </div>
  </template>
  <template v-else>
    <div class="faq-manager-wrapper">
      <FAQEntryManager v-if="kbId" :kb-id="kbId" />
    </div>
  </template>

  <!-- 知识库编辑器（创建/编辑统一组件） -->
  <KnowledgeBaseEditorModal :visible="uiStore.showKBEditorModal" :mode="uiStore.kbEditorMode"
    :kb-id="uiStore.currentKBId || undefined" :initial-type="uiStore.kbEditorType"
    @update:visible="(val) => val ? null : uiStore.closeKBEditor()" @success="handleKBEditorSuccess" />

  <ContextualGuide tour="kbDetail" :when="showKbDetailContextualGuide" />

</template>
<style scoped lang="less">
.knowledge-layout {
  display: flex;
  flex-direction: column;
  margin: 0 16px 0 4px;
  gap: 20px;
  height: 100%;
  flex: 1;
  width: 100%;
  min-width: 0;
  padding: 20px;
  box-sizing: border-box;
}

// 与列表页一致：浅灰底圆角区，左侧筛选为白底卡片
.knowledge-main {
  display: flex;
  flex: 1;
  min-height: 0;
  background: transparent;
  border: none;
}

// 标签筛选浮层：点击工具栏入口展开，不占文档列表横向空间
.doc-card-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  position: relative;
  /* 作为批量工具栏悬浮的定位上下文 */
}

.doc-filter-bar {
  padding: 0 0 12px 0;
  flex-shrink: 0;
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-areas:
    'search trailing'
    'filters filters';
  gap: 8px 12px;
  align-items: center;

  .doc-search-input {
    grid-area: search;
    min-width: 0;
    width: 100%;
  }

  &__filters {
    grid-area: filters;
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
    overflow-x: auto;
    flex-wrap: nowrap;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: thin;
    scrollbar-color: rgba(0, 0, 0, 0.15) transparent;

    &::-webkit-scrollbar {
      height: 4px;
    }

    &::-webkit-scrollbar-thumb {
      background-color: rgba(0, 0, 0, 0.15);
      border-radius: 2px;
    }
  }

  &__trailing {
    grid-area: trailing;
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  @media (min-width: 1280px) {
    display: flex;
    flex-direction: row;
    flex-wrap: nowrap;
    gap: 12px;

    &__filters {
      flex: 0 1 auto;
      overflow-x: visible;
    }
  }

  .doc-filter-field {
    width: 140px;
    flex-shrink: 0;

    &--wide {
      width: 280px;
    }

    &__control {
      width: 100%;
    }
  }

  @media (min-width: 1280px) {
    .doc-search-input {
      flex: 1 1 220px;
      min-width: 220px;
    }
  }

  .doc-type-select {
    width: 100%;
  }

  .doc-date-range {
    width: 100%;

    // TDesign focuses both the outer popup reference and inner inputs, which
    // visually stacks into a "double border" — drop the inner shadow.
    :deep(.t-input--focused),
    :deep(.t-is-focused) {
      box-shadow: none;
    }
  }

  .doc-filter-actions {
    flex-shrink: 0;

    :deep(.content-bar-icon-btn) {
      color: var(--td-text-color-secondary);
      background: transparent;
      border: none;

      &:hover {
        color: var(--td-brand-color);
        background: var(--td-bg-color-secondarycontainer);
      }
    }
  }

  :deep(.t-input) {
    font-size: 13px;
    background-color: var(--td-bg-color-secondarycontainer);
    border-color: transparent;
    border-radius: 6px;
    box-shadow: none !important;

    &:hover,
    &:focus,
    &.t-is-focused {
      border-color: var(--td-brand-color);
      background-color: var(--td-bg-color-container);
      box-shadow: none !important;
    }
  }

  :deep(.t-select) {
    .t-input {
      font-size: 13px;
      background-color: var(--td-bg-color-secondarycontainer);
      border-color: transparent;
      border-radius: 6px;
      box-shadow: none !important;

      &:hover,
      &.t-is-focused {
        border-color: var(--td-brand-color);
        background-color: var(--td-bg-color-container);
        box-shadow: none !important;
      }
    }
  }
}

.doc-scroll-container {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;

  &.is-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    overflow-y: hidden;
  }

  &.is-marquee-active {
    cursor: crosshair;
  }
}

.doc-marquee-box {
  position: absolute;
  z-index: 4;
  pointer-events: none;
  border: 1px solid var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  border-radius: 2px;

  &.is-add {
    border-color: var(--td-brand-color);
    background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  }

  &.is-subtract {
    border-color: var(--td-error-color-6);
    background: color-mix(in srgb, var(--td-error-color-6) 12%, transparent);
  }
}

/* 批量条悬浮在滚动区底部，不挤占列表高度 */
.doc-batch-bar-anchor {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 12px;
  z-index: 6;
  display: flex;
  justify-content: center;
  padding: 0 16px;
  pointer-events: none;

  &>* {
    pointer-events: auto;
  }
}

// Header 样式（无底部分割线，留更多空间给下方内容区）
.document-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  flex-shrink: 0;

  .document-header-title {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .document-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .kb-title-actions {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    margin-left: 4px;
  }

  h2 {
    margin: 0;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
  }

  .document-subtitle {
    margin: 0;
    color: var(--td-text-color-placeholder);
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 400;
    line-height: 20px;
  }

  .parser-hint {
    display: flex;
    align-items: center;
    gap: 4px;
    margin: 2px 0 0;
    color: var(--td-warning-color);
    font-size: 12px;
    line-height: 1.4;
    cursor: pointer;
    transition: color 0.15s ease;

    &:hover {
      color: var(--td-warning-color-active);

      .parser-hint-link {
        text-decoration: underline;
      }
    }

    .parser-hint-icon {
      font-size: 12px;
      flex-shrink: 0;
    }

    .parser-hint-link {
      color: var(--td-brand-color);
      margin-left: 2px;
      white-space: nowrap;
    }
  }

  .storage-engine-warning {
    display: flex;
    align-items: center;
    gap: 4px;
    margin: 2px 0 0;
    color: var(--td-warning-color);
    font-size: 12px;
    line-height: 1.4;
    cursor: pointer;
    transition: color 0.15s ease;

    &:hover {
      color: var(--td-warning-color-active);

      .warning-link {
        text-decoration: underline;
      }
    }

    .warning-icon {
      font-size: 12px;
      flex-shrink: 0;
    }

    .warning-link {
      color: var(--td-brand-color);
      margin-left: 2px;
      white-space: nowrap;
    }
  }
}


.document-upload-input {
  display: none;
}

.kb-settings-button {
  width: 30px;
  height: 30px;
  border: none;
  border-radius: 50%;
  background: var(--td-bg-color-secondarycontainer);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: all 0.2s ease;
  padding: 0;

  &:hover:not(:disabled) {
    background: var(--td-success-color-light);
    color: var(--td-brand-color);
    box-shadow: none;
  }

  &:disabled {
    cursor: not-allowed;
    opacity: 0.4;
  }

  :deep(.t-icon) {
    font-size: 18px;
  }
}

.card-bottom-right {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  overflow: hidden;
}

.faq-manager-wrapper {
  flex: 1;
  min-height: 0;
  padding: 24px 32px;
  overflow-y: auto;
  margin: 0 16px 0 4px;
}

@media (max-width: 1250px) and (min-width: 1045px) {
  .answers-input {
    transform: translateX(-329px);
  }

  :deep(.t-textarea__inner) {
    width: 654px !important;
  }
}

@media (max-width: 1045px) {
  .answers-input {
    transform: translateX(-250px);
  }

  :deep(.t-textarea__inner) {
    width: 500px !important;
  }
}

@media (max-width: 750px) {
  .answers-input {
    transform: translateX(-182px);
  }

  :deep(.t-textarea__inner) {
    width: 340px !important;
  }
}

@media (max-width: 600px) {
  .answers-input {
    transform: translateX(-164px);
  }

  :deep(.t-textarea__inner) {
    width: 300px !important;
  }
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

.doc-card-list {
  box-sizing: border-box;
  display: grid;
  // 文档卡片信息量较大（标题 + 摘要 + 标签/类型），保持稍宽的最小列宽，避免一行塞太多导致内容拥挤。
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
  align-content: flex-start;
  width: 100%;

  &.doc-card-list-animated {
    animation: contentFadeIn 0.32s ease-out;
  }
}

.knowledge-card-skeleton {
  cursor: default;

  .card-content {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 10px 14px 8px;
  }

  .card-content-nav {
    margin-bottom: 8px;
  }

  .card-bottom {
    flex-shrink: 0;
    margin-top: auto;
    width: 100%;
    padding: 0 14px;
    box-sizing: border-box;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-top: 1px solid var(--td-component-stroke);
  }
}

.doc-empty-state {
  flex: 1;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  min-height: 100%;
}


.card-menu {
  display: flex;
  flex-direction: column;
  min-width: 140px;
  gap: 1px;
}

.card-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  transition: all 0.15s cubic-bezier(0.2, 0, 0, 1);
  border-radius: 6px;
  font-size: 14px;
  line-height: 20px;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &:active {
    background: var(--td-bg-color-container-active);
    transform: scale(0.98);
  }

  .icon {
    font-size: 16px;
    color: var(--td-text-color-secondary);
    transition: all 0.15s cubic-bezier(0.2, 0, 0, 1);
  }

  &:hover .icon {
    color: var(--td-text-color-primary);
  }

  &.danger {
    color: var(--td-error-color-6);
    margin-top: 4px;
    position: relative;

    &::before {
      content: '';
      position: absolute;
      top: -3px;
      left: 8px;
      right: 8px;
      height: 1px;
      background: var(--td-component-stroke);
    }

    .icon {
      color: var(--td-error-color-6);
    }

    &:hover {
      background: var(--td-error-color-1);
      color: var(--td-error-color-6);

      .icon {
        color: var(--td-error-color-6);
      }
    }

    &:active {
      background: var(--td-error-color-2);
    }
  }
}

.move-menu {
  min-width: 220px;
  max-width: 280px;
  max-height: 360px;
  overflow-y: auto;

  .move-menu-header {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 12px;
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    border-bottom: 1px solid var(--td-component-stroke);
    cursor: pointer;

    &:hover {
      background: var(--td-bg-color-container-hover);
    }
  }

  .move-menu-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px 0;
  }

  .move-menu-empty {
    padding: 12px 16px;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    text-align: center;
    line-height: 1.5;
  }

  .move-target-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .move-target-count {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }

  .move-confirm-body {
    padding: 8px;

    .move-target-info {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 6px 8px;
      background: var(--td-bg-color-container-hover);
      border-radius: 6px;
      font-size: 13px;
      color: var(--td-text-color-secondary);
      margin-bottom: 8px;
    }

    .move-mode-item {
      display: flex;
      align-items: flex-start;
      gap: 6px;
      padding: 6px 8px;
      border-radius: 6px;
      cursor: pointer;
      margin-bottom: 4px;

      &:hover {
        background: var(--td-bg-color-container-hover);
      }

      &.active {
        background: var(--td-brand-color-light);
      }

      .move-mode-text {
        display: flex;
        flex-direction: column;
        gap: 2px;

        .move-mode-label {
          font-size: 13px;
          font-weight: 500;
          color: var(--td-text-color-primary);
        }

        .move-mode-desc {
          font-size: 11px;
          color: var(--td-text-color-placeholder);
          line-height: 1.4;
        }
      }
    }

    .move-confirm-actions {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      margin-top: 8px;
    }
  }
}

.card-draft {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  flex-shrink: 0;
}

.card-draft-tip {
  color: var(--td-warning-color);
  font-size: 11px;
}

.knowledge-card {
  min-width: 240px;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-border);
  height: 136px;
  border-radius: 8px;
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
  background: var(--td-bg-color-container);
  position: relative;
  cursor: pointer;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;

  /* 仅在批量管理模式下渲染 checkbox，常态下不占位，避免标题在 hover 时右滑 */
  .card-nav-check {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 29px;
    margin-right: 8px;
    cursor: pointer;

    .card-select-checkbox {
      margin: 0;
      line-height: 0;

      :deep(.t-checkbox) {
        align-items: center;
      }

      :deep(.t-checkbox__label) {
        display: none !important;
        width: 0 !important;
        min-width: 0 !important;
        margin: 0 !important;
        padding: 0 !important;
      }

      :deep(.t-checkbox__input) {
        margin: 0;
      }

      :deep(.t-checkbox__input-wrapper) {
        margin: 0;
      }
    }
  }

  .card-content {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 10px 14px 8px;
  }

  .card-analyze {
    flex-shrink: 0;
    height: 52px;
    display: flex;
    align-items: flex-start;
  }

  .card-analyze-loading {
    display: block;
    color: var(--td-brand-color);
    font-size: 14px;
    margin-top: 2px;
  }

  .card-analyze-txt {
    color: var(--td-brand-color);
    font-family: var(--app-font-family);
    font-size: 11px;
    margin-left: 8px;
  }

  // In-flight / failed: only status text + trace icon open the drawer.
  .card-analyze-trace {
    height: auto;
    min-height: 0;
    align-items: center;
    gap: 2px;
  }

  .card-analyze-trace-link {
    cursor: pointer;

    &:hover {
      text-decoration: underline;
    }
  }

  .card-analyze-trace-btn {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    margin: 0;
    padding: 2px;
    border: none;
    background: transparent;
    color: var(--td-brand-color);
    cursor: pointer;
    line-height: 1;
    border-radius: 4px;

    :deep(.t-icon) {
      font-size: 14px;
    }

    &:hover {
      background: var(--td-bg-color-component-hover);
    }
  }

  .card-analyze.failure .card-analyze-trace-btn {
    color: var(--td-error-color);
  }

  .failure {
    color: var(--td-error-color);
  }

  .card-content-nav {
    flex-shrink: 0;
    display: flex;
    align-items: flex-start;
    gap: 0;
    margin-bottom: 6px;
  }

  .card-content-title {
    flex: 1;
    min-width: 0;
    height: 24px;
    line-height: 24px;
    display: inline-block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 600;
    letter-spacing: 0.01em;
    margin-right: 8px;
  }

  .more-wrap {
    flex-shrink: 0;
    display: flex;
    width: 25px;
    height: 25px;
    justify-content: center;
    align-items: center;
    border-radius: 5px;
    cursor: pointer;
  }

  .more-wrap:hover {
    background: var(--td-component-stroke);
  }

  .more-icon {
    width: 14px;
    height: 14px;
  }

  .active-more {
    background: var(--td-component-stroke);
  }

  .card-content-txt {
    flex: 1;
    min-height: 0;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    color: var(--td-text-color-secondary);
    font-family: var(--app-font-family);
    font-size: 12px;
    font-weight: 400;
    line-height: 19px;
  }

  .card-bottom {
    flex-shrink: 0;
    margin-top: auto;
    padding: 0 14px;
    box-sizing: border-box;
    height: 32px;
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--td-bg-color-container);
    border-top: 1px solid var(--td-component-stroke);
  }

  .card-time {
    flex-shrink: 0;
    color: var(--td-text-color-secondary);
    font-family: var(--app-font-family);
    font-size: 12px;
    font-weight: 400;
    white-space: nowrap;
  }

  .card-type {
    flex-shrink: 0;
    color: var(--td-text-color-placeholder);
    font-family: var(--app-font-family);
    font-size: 11px;
    font-weight: 500;
    padding: 0;
    background: transparent;
    letter-spacing: 0.02em;
  }
}

.card-bottom-right {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  overflow: hidden;
}

.knowledge-card:hover {
  border-color: color-mix(in srgb, var(--td-component-stroke) 55%, var(--td-brand-color));
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.07);
}

/* 悬停知识卡片时跟随鼠标的详情气泡 */
.knowledge-card-hover-popover {
  position: fixed;
  z-index: 9999;
  pointer-events: none;
  min-width: 220px;
  max-width: 360px;
  padding: 12px 14px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  font-family: var(--app-font-family);
  transition: opacity 0.15s ease;
  will-change: transform;

  /* 防止气泡内容抖动 */
  backface-visibility: hidden;
  -webkit-backface-visibility: hidden;
  transform: translateZ(0);
  -webkit-transform: translateZ(0);

  .card-popover-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin-bottom: 8px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-popover-status {
    font-size: 12px;
    margin-bottom: 6px;
    display: flex;
    align-items: center;
    gap: 6px;

    &.parsing {
      color: var(--td-brand-color);
    }

    &.failure {
      color: var(--td-error-color);
    }

    &.draft {
      color: var(--td-warning-color);
    }
  }

  .card-popover-desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    line-height: 1.5;
    margin-bottom: 8px;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 5;
    line-clamp: 5;
    overflow: hidden;
  }

  .card-popover-error-msg {
    display: block;
    margin-top: 4px;
    font-size: 11px;
    color: var(--td-error-color);
    opacity: 0.95;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 280px;
  }

  .card-popover-source {
    font-size: 11px;
    color: var(--td-brand-color);
    margin-bottom: 6px;
    display: flex;
    align-items: center;
    gap: 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }

  .card-popover-extra {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
    font-size: 11px;
    color: var(--td-text-color-secondary);
    margin-bottom: 6px;
  }

  .card-popover-created,
  .card-popover-size {
    flex-shrink: 0;
  }

  .card-popover-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    font-size: 11px;
    color: var(--td-text-color-secondary);
  }

  .card-popover-channel {
    padding: 1px 6px;
    background: var(--td-warning-color-light);
    color: var(--td-warning-color);
    border-radius: 4px;
  }

  .card-popover-type {
    padding: 1px 6px;
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-secondary);
    border-radius: 4px;
  }

  .card-popover-hint {
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--td-component-stroke);
    font-size: 11px;
    color: var(--td-text-color-secondary);
  }
}

.url-import-form {
  padding: 8px 0;

  .url-input-label {
    color: var(--td-text-color-primary);
    font-size: 14px;
    font-weight: 500;
    margin-bottom: 8px;
  }

  .url-input-tip {
    color: var(--td-text-color-secondary);
    font-size: 12px;
    margin-top: 8px;
    line-height: 1.5;
  }
}

.knowledge-card-upload {
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 14px;
  font-weight: 400;
  cursor: pointer;

  .btn-upload {
    margin: 33px auto 0;
    width: 112px;
    height: 32px;
    border: 1px solid var(--td-component-border);
    display: flex;
    justify-content: center;
    align-items: center;
    margin-bottom: 24px;
  }

  .svg-icon-download {
    margin-right: 8px;
  }
}

.upload-described {
  color: var(--td-text-color-disabled);
  font-family: var(--app-font-family);
  font-size: 12px;
  font-weight: 400;
  text-align: center;
  display: block;
  width: 188px;
  margin: 0 auto;
}

.del-card {
  vertical-align: middle;
}
</style>
