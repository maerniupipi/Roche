<template>
  <div class="aside_box" :class="{ 'aside_box--collapsed': uiStore.sidebarCollapsed }">
    <!-- 展开时：Logo + 搜索/折叠按钮同行 -->
    <div class="logo_row" v-if="!uiStore.sidebarCollapsed">
      <div class="logo_box" @click="router.push(defaultPlatformPath)" style="cursor: pointer;">
        <div class="brand-lockup" aria-label="ROCO">
          <img src="@/assets/img/logo.svg" alt="Roche Logo">
          <span class="brand-name">ROCO</span>
          <!-- 
                    <span class="brand-product">Knowledge Agent Platform</span> -->
        </div>
      </div>
      <div class="logo_actions">
        <div class="sidebar-toggle" @click="uiStore.toggleSidebar" :title="t('menu.collapseSidebar')">
          <svg viewBox="0 0 20 20" width="18" height="18" fill="none" xmlns="http://www.w3.org/2000/svg">
            <rect x="1.5" y="1.5" width="17" height="17" rx="3" stroke="currentColor" stroke-width="1.2" />
            <line x1="7.5" y1="1.5" x2="7.5" y2="18.5" stroke="currentColor" stroke-width="1.2" />
            <line x1="4" y1="7.5" x2="4" y2="12.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" />
          </svg>
        </div>
      </div>
    </div>
    <!-- 折叠时：展开按钮 -->
    <t-tooltip v-else :content="t('menu.expandSidebar')" placement="right">
      <div class="menu_item sidebar-toggle-item" @click="uiStore.toggleSidebar">
        <div class="menu_item-box">
          <div class="menu_icon">
            <svg class="icon" viewBox="0 0 20 20" width="20" height="20" fill="none" xmlns="http://www.w3.org/2000/svg">
              <rect x="1.5" y="1.5" width="17" height="17" rx="3" stroke="currentColor" stroke-width="1.2" />
              <line x1="7.5" y1="1.5" x2="7.5" y2="18.5" stroke="currentColor" stroke-width="1.2" />
              <line x1="5" y1="10" x2="3" y2="8" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" />
              <line x1="5" y1="10" x2="3" y2="12" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" />
            </svg>
          </div>
        </div>
      </div>
    </t-tooltip>

    <!-- 知识域选择器：仅在用户可切换知识域时显示 -->

    <!-- 折叠时右侧拖拽展开手柄 -->
    <div v-if="uiStore.sidebarCollapsed" class="sidebar-drag-handle" @mousedown="onDragHandleMouseDown" />

    <!-- 上半部分：新对话吸顶 + 智能体/历史会话随滚动一起滚走 -->
    <div class="menu_top" ref="scrollContainer" @scroll="handleScroll">
      <div class="menu_box" :class="{ 'menu_box--sticky': item.children && !uiStore.sidebarCollapsed }"
        v-for="(item, index) in topMenuItems" :key="index">
        <t-tooltip :content="item.title" placement="right" :disabled="!uiStore.sidebarCollapsed">
          <div @click="handleMenuClick(item.path)" @mouseenter="mouseenteMenu(item.path)"
            @mouseleave="mouseleaveMenu(item.path)" :data-guide="`nav-${item.path}`"
            :class="['menu_item', item.childrenPath && item.childrenPath == currentpath ? 'menu_item_c_active' : isMenuItemActive(item.path) ? 'menu_item_active' : '']">
            <div class="menu_item-box">
              <div class="menu_icon" v-if="uiStore.sidebarCollapsed || item.titleKey !== 'menu.newChat'">
                <img class="icon"
                  :src="getImgSrc(item.icon == 'logout' ? logoutIcon : item.icon == 'setting' ? settingIcon : prefixIcon)"
                  alt="">
              </div>
              <template v-if="!uiStore.sidebarCollapsed">
                <template v-if="item.titleKey === 'menu.newChat'">
                  <t-button style="width: 100%">
                    <template #icon> <t-icon name="add" size="14px" /> </template>{{ item.title }}
                  </t-button>
                </template>
                <span v-else class="menu_title" :title="item.title">{{ item.title }}</span>
              </template>
            </div>
          </div>
        </t-tooltip>
      </div>
      <!-- 历史会话：按日期分组展示 -->
      <div class="submenu" v-if="!uiStore.sidebarCollapsed">
        <div class="history-title" role="button" tabindex="0" :aria-expanded="historyExpanded"
          @click="toggleHistoryExpanded" @keydown.enter.prevent="toggleHistoryExpanded"
          @keydown.space.prevent="toggleHistoryExpanded">
          <span class="history-title__label">{{ t('menu.historyTitle') }}</span>
          <span class="history-title__caret" :class="{ 'history-title__caret--collapsed': !historyExpanded }">
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" xmlns="http://www.w3.org/2000/svg"
              aria-hidden="true">
              <path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"
                stroke-linejoin="round" />
            </svg>
          </span>
        </div>
        <Transition name="history-collapse">
          <div v-show="historyExpanded" class="history-content">
            <template v-if="sessionListBooting && !hasAnySession">
              <div v-for="n in 4" :key="'skel-' + n" class="submenu_item_p session-chat-row">
                <div class="session-list-row session-list-row--flat">
                  <t-skeleton animation="gradient" class="session-list-row__body"
                    :row-col="[{ width: '100%', height: '14px' }]" />
                </div>
              </div>
            </template>
            <div v-else class="session-filtered-list">
              <template v-if="activeBucket?.loading && !activeBucket.loaded && filteredGroupedSessions.length === 0">
                <div v-for="n in 4" :key="'bucket-skel-' + n" class="submenu_item_p session-chat-row">
                  <div class="session-list-row session-list-row--flat">
                    <t-skeleton animation="gradient" class="session-list-row__body"
                      :row-col="[{ width: '100%', height: '14px' }]" />
                  </div>
                </div>
              </template>
              <template v-else-if="activeBucket?.loaded && filteredGroupedSessions.length === 0">
                <div class="submenu_empty">{{ t('menu.noSessions') }}</div>
              </template>
              <template v-else>
                <template v-for="group in filteredGroupedSessions" :key="group.key">
                  <div v-if="group.label" class="timeline_header session-list-row session-list-row--flat">
                    <span class="session-list-row__body">
                      <span class="timeline_header-label">{{ group.label }}</span>
                    </span>
                  </div>
                  <div v-for="subitem in group.items" :key="subitem.id" class="submenu_item_p session-chat-row" :class="{
                    'session-chat-row--active': !batchMode && subitem.path === currentSecondpath,
                    'session-chat-row--selected': batchMode && batchSelectedIds.includes(subitem.id),
                  }">
                    <div class="session-list-row session-list-row--flat">
                      <div class="session-list-row__body">
                        <SessionSidebarRow :item="subitem" :batch-mode="batchMode" :active-path="currentSecondpath"
                          :selected-ids="batchSelectedIds" :menu-options="buildSessionMenuOptions(subitem)"
                          @navigate="gotopage(subitem.path)" @toggle-select="toggleBatchSelect(subitem.id)"
                          @menu-click="handleSessionMenuClick($event, subitem)"
                          @hover-in="mouseenteBotDownr(subitem.id)" @hover-out="mouseleaveBotDown" />
                      </div>
                    </div>
                  </div>
                </template>
                <div v-if="activeBucket?.loading && filteredGroupedSessions.length > 0"
                  class="session-list-loading session-list-row session-list-row--flat">
                  <span class="session-list-row__body">
                    <t-loading size="small" />
                  </span>
                </div>
              </template>
            </div>
          </div>
        </Transition>
      </div>

      <!-- 批量管理底部操作条 -->
      <div v-if="batchMode && !uiStore.sidebarCollapsed" class="batch-inline-footer">
        <div class="batch-footer-left">
          <t-checkbox :checked="isAllBatchSelected" :indeterminate="isBatchIndeterminate"
            @change="toggleBatchSelectAll">
            {{ t('batchManage.selectAll') }}
          </t-checkbox>
        </div>
        <div class="batch-footer-right">
          <t-button size="small" variant="text" @click="exitBatchMode">
            {{ t('batchManage.cancel') }}
          </t-button>
          <t-button size="small" theme="danger" variant="base" :disabled="batchSelectedIds.length === 0"
            :loading="batchDeleting" @click="handleInlineBatchDelete">
            {{ t('batchManage.delete') }}{{ batchSelectedIds.length > 0 ? `(${batchDisplayCount})` : '' }}
          </t-button>
        </div>
      </div>
    </div>


    <!-- 下半部分：占位（用户菜单已迁移到 PlatformHeader） -->
    <div class="menu_bottom" />
    <PlatformHeader />

    <!-- 重命名会话弹窗：菜单项触发 → 输入新名称 → 提交后通过 session-title-updated 事件回写到侧栏 -->
    <t-dialog v-model:visible="renameDialogVisible" :header="t('menu.renameDialogTitle')" placement="center"
      :width="420" :confirm-btn="t('menu.renameConfirm')" :cancel-btn="t('menu.renameCancel')"
      :on-confirm="confirmRename" :on-close="cancelRename" :on-cancel="cancelRename"
      :close-on-overlay-click="!renameSubmitting" :close-on-escape-keydown="!renameSubmitting"
      :loading="renameSubmitting" destroy-on-close>
      <t-input style="margin-top: 10px;" v-model:value="renameInput" :placeholder="t('menu.renamePlaceholder')"
        :maxlength="60" :disabled="renameSubmitting" autofocus @enter="confirmRename" />
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { onMounted, onUnmounted, watch, computed, ref, h, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getSessionsList, delSession, batchDelSessions, deleteAllSessions, clearSessionMessages, pinSession, unpinSession, updateSessionTitle } from "@/api/chat/index";
import { useChatResourcesStore } from '@/stores/chatResources';
import SessionSidebarRow from './SessionSidebarRow.vue';
import PlatformHeader from '@/components/PlatformHeader.vue'
import {
  SIDEBAR_BUCKET_PAGE_SIZE,
  buildBucketDefinitions,
  bucketHasMore,
  createEmptyBucket,
  flattenBucketItems,
  mergeBucketPage,
  prependSessionToWebBucket,
  removeSessionFromBuckets,
  type SidebarSessionBucket,
} from './sessionSidebarBuckets';
import type { SessionForGrouping } from './sessionGrouping';
import {
  classifyDateBucket,
  groupSessionsByDate,
  type DateBucketKey,
} from './sessionGrouping';
import { logout as logoutApi } from '@/api/auth';
import { useMenuStore } from '@/stores/menu';
import { useAuthStore } from '@/stores/auth';
import { useUIStore } from '@/stores/ui';
import { MessagePlugin, DialogPlugin, Icon as TIcon } from "tdesign-vue-next";
import pinIconUrl from '@/assets/img/pin.svg';
import deleteIconUrl from '@/assets/img/delete-red.svg';
import editIconUrl from '@/assets/img/edit.svg';
import { useI18n } from 'vue-i18n';
import { getSystemInfo } from '@/api/system';
import { isDebugger } from '@/composables/featureFlags';

const chatResources = useChatResourcesStore();
const { t } = useI18n();
const usemenuStore = useMenuStore();
const authStore = useAuthStore();
const uiStore = useUIStore();

const route = useRoute();
const router = useRouter();
const currentpath = ref('');
const total = ref(0);
const sessionBuckets = ref<Record<string, SidebarSessionBucket>>({});
const bucketOrder = ref<string[]>([]);
let bucketRequestToken = 0;
const sessionListBooting = ref(false);
const currentSecondpath = ref('');
const scrollContainer = ref<HTMLElement | null>(null);
const activeSessionBucketKey = ref('web');
const sessionListCanScroll = ref(false);
const activeBucket = computed(() => sessionBuckets.value[activeSessionBucketKey.value]);
const hasAnySession = computed(() =>
  Object.values(sessionBuckets.value).some((bucket) => bucket.items.length > 0),
);
type MenuItem = { title: string; titleKey: string; icon: string; path: string; childrenPath?: string; children?: any[] };
const { menuArr, visibleMenuArr } = storeToRefs(usemenuStore);
let activeSubmenu = ref<string>('');
const isLiteEdition = ref(false);

// 批量管理状态
const batchMode = ref(false)

// 历史会话区域：展开 / 收起状态（默认展开，保持原有行为）
const historyExpanded = ref(true)
const toggleHistoryExpanded = () => {
  historyExpanded.value = !historyExpanded.value
}
const batchSelectedIds = ref<string[]>([])
const batchDeleting = ref(false)

const allSessionIds = computed(() => {
  const chatMenu = (menuArr.value as unknown as MenuItem[]).find((item: MenuItem) => item.path === 'creatChat');
  if (!chatMenu?.children) return [];
  return (chatMenu.children as any[]).map((s: any) => s.id);
})

const isAllBatchSelected = computed(() =>
  allSessionIds.value.length > 0 && batchSelectedIds.value.length === allSessionIds.value.length
)

const isBatchIndeterminate = computed(() =>
  batchSelectedIds.value.length > 0 && batchSelectedIds.value.length < allSessionIds.value.length
)

const batchDisplayCount = computed(() =>
  isAllBatchSelected.value ? total.value : batchSelectedIds.value.length
)

// 是否可以管理所有知识域
const isSystemAdmin = computed(() => authStore.isSystemAdmin);
const defaultPlatformPath = computed(() => '/platform/creatChat');

// 统一的菜单项激活状态判断
const isMenuItemActive = (itemPath: string): boolean => {
  const currentRoute = route.name;

  switch (itemPath) {
    case 'creatChat':
      return currentRoute === 'globalCreatChat';
    case 'settings':
      return currentRoute === 'settings';
    default:
      return itemPath === currentpath.value;
  }
};

// 统一的图标激活状态判断
const getIconActiveState = (itemPath: string) => {
  const currentRoute = route.name;

  return {
    isCreatChatActive: itemPath === 'creatChat' && currentRoute === 'globalCreatChat',
    isSettingsActive: itemPath === 'settings' && currentRoute === 'settings',
    isChatActive: itemPath === 'chat' && currentRoute === 'chat'
  };
};

// 分离上下两部分菜单（使用 visibleMenuArr 以便 lite 模式过滤 logout）
const topMenuItems = computed<MenuItem[]>(() => {
  return (visibleMenuArr.value as unknown as MenuItem[]).filter((item: MenuItem) =>
    (item.path === 'agents' && isDebugger.value) || item.path === 'creatChat'
  );
});

// 进行中的置顶/取消置顶请求，避免重复点击
const pinningIds = ref<Set<string>>(new Set())

// 「聊天」区内按日期分组（当前筛选来源）
const dateBucketLabels = computed<Record<DateBucketKey, string>>(() => ({
  pinned: t('time.pinned'),
  today: t('time.today'),
  yesterday: t('time.yesterday'),
  last7Days: t('time.last7Days'),
  last30Days: t('time.last30Days'),
  lastYear: t('time.lastYear'),
  earlier: t('time.earlier'),
}));

const filteredGroupedSessions = computed(() => {
  const bucket = activeBucket.value;
  if (!bucket?.items.length) return [];
  return groupSessionsByDate(
    bucket.items.map((item) => ({
      ...item,
      path: `chat/${item.id}`,
      title: item.title || '',
    })),
    dateBucketLabels.value,
    (session) => classifyDateBucket(session.updated_at || session.created_at),
  );
});

const refreshSessionListScrollability = async () => {
  await nextTick();
  const container = scrollContainer.value;
  sessionListCanScroll.value = !!container && container.scrollHeight > container.clientHeight + 1;
};

/** 列表未撑满滚动区时自动续页（按当前可见 DOM 测量，避免折叠导致误判） */
const ensureBucketFillsViewport = async (key: string) => {
  const MAX_ITERATIONS = 20;
  for (let i = 0; i < MAX_ITERATIONS; i++) {
    await nextTick();
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    const container = scrollContainer.value;
    const bucket = sessionBuckets.value[key];
    if (!container || !bucket || !bucketHasMore(bucket) || bucket.loading) break;

    const hasOverflow = container.scrollHeight > container.clientHeight + 1;
    if (hasOverflow) break;

    const prevCount = bucket.items.length;
    await loadBucketPage(key);
    if ((sessionBuckets.value[key]?.items.length ?? 0) <= prevCount) break;
  }
};

const mouseenteBotDownr = (val: string) => {
  activeSubmenu.value = val;
}
const mouseleaveBotDown = () => {
  activeSubmenu.value = '';
}

const enterBatchMode = () => {
  batchMode.value = true
  batchSelectedIds.value = []
}

const exitBatchMode = () => {
  batchMode.value = false
  batchSelectedIds.value = []
}

const toggleBatchSelect = (id: string) => {
  const idx = batchSelectedIds.value.indexOf(id)
  if (idx > -1) {
    batchSelectedIds.value.splice(idx, 1)
  } else {
    batchSelectedIds.value.push(id)
  }
}

const toggleBatchSelectAll = (checked: boolean) => {
  batchSelectedIds.value = checked ? [...allSessionIds.value] : []
}

const handleInlineBatchDelete = () => {
  if (batchSelectedIds.value.length === 0) return
  const isDeleteAll = isAllBatchSelected.value
  const displayCount = batchDisplayCount.value
  const confirmDialog = DialogPlugin.confirm({
    header: t('batchManage.deleteConfirmTitle'),
    body: isDeleteAll
      ? t('batchManage.deleteAllConfirmBody') || t('batchManage.deleteConfirmBody', { count: displayCount })
      : t('batchManage.deleteConfirmBody', { count: displayCount }),
    confirmBtn: { content: t('batchManage.delete'), theme: 'danger' as const },
    cancelBtn: t('batchManage.cancel'),
    theme: 'warning',
    onConfirm: async () => {
      batchDeleting.value = true
      try {
        let res: any
        if (isDeleteAll) {
          res = await deleteAllSessions()
        } else {
          res = await batchDelSessions([...batchSelectedIds.value])
        }
        if (res && res.success === true) {
          if (isDeleteAll) {
            usemenuStore.clearMenuArr();
            total.value = 0;
            await getMessageList();
          } else {
            let next = sessionBuckets.value;
            for (const id of batchSelectedIds.value) {
              next = removeSessionFromBuckets(next, id);
            }
            sessionBuckets.value = next;
            syncMenuStoreFromBuckets();
          }
          const currentChatId = route.params.chatid as string;
          if (currentChatId && (isDeleteAll || batchSelectedIds.value.includes(currentChatId))) {
            router.push('/platform/creatChat');
          }
          batchSelectedIds.value = []
          MessagePlugin.success(t('batchManage.deleteSuccess'))
          exitBatchMode()
        } else {
          MessagePlugin.error(t('batchManage.deleteFailed'))
        }
      } catch {
        MessagePlugin.error(t('batchManage.deleteFailed'))
      }
      batchDeleting.value = false
      confirmDialog.destroy()
    },
  })
}

const handleSessionMenuClick = (data: { value: string }, item: any) => {
  if (data?.value === 'delete') {
    delCard(item);
  } else if (data?.value === 'clearMessages') {
    clearMessages(item);
  } else if (data?.value === 'batchManage') {
    enterBatchMode()
  } else if (data?.value === 'pin' || data?.value === 'unpin') {
    togglePin(item, data.value === 'pin');
  } else if (data?.value === 'rename') {
    openRenameDialog(item);
  }
};

const buildSessionMenuOptions = (item: any) => {
  const options: any[] = [];
  if (item.is_pinned) {
    options.push({
      content: t('menu.unpin'),
      value: 'unpin',
      prefixIcon: () => h('img', { src: pinIconUrl, alt: '', style: { width: '16px', height: '16px' } }),
    });
  } else {
    options.push({
      content: t('menu.pin'),
      value: 'pin',
      prefixIcon: () => h('img', { src: pinIconUrl, alt: '', style: { width: '16px', height: '16px' } }),
    });
  }
  // 「清空消息」「批量管理」仅在调试版（isDebugger）下开放
  if (isDebugger.value) {
    options.push(
      { content: t('menu.clearMessages'), value: 'clearMessages', prefixIcon: () => h(TIcon, { name: 'clear', size: '16px' }) },
      { content: t('menu.batchManage'), value: 'batchManage', prefixIcon: () => h(TIcon, { name: 'queue', size: '16px' }) },
    );
  }
  options.push({
    content: t('menu.renameSession'),
    value: 'rename',
    prefixIcon: () => h('img', { src: editIconUrl, alt: '', style: { width: '16px', height: '16px' } }),
  });
  options.push({
    content: t('upload.deleteRecord'), value: 'delete', theme: 'error',
    prefixIcon: () => h('img', { src: deleteIconUrl, alt: '', style: { width: '16px', height: '16px' } }),
  });
  return options;
};

const updateSessionInBuckets = (
  sessionId: string,
  patch: Partial<{ is_pinned: boolean; pinned_at: string | null; title: string; isNoTitle?: boolean }>,
) => {
  const next: Record<string, SidebarSessionBucket> = {};
  for (const [key, bucket] of Object.entries(sessionBuckets.value)) {
    next[key] = {
      ...bucket,
      items: bucket.items.map((row) => (row.id === sessionId ? { ...row, ...patch } : row)),
    };
  }
  sessionBuckets.value = next;
  syncMenuStoreFromBuckets();
};

const togglePin = (item: any, pin: boolean) => {
  if (pinningIds.value.has(item.id)) return;
  pinningIds.value.add(item.id);

  const call = pin ? pinSession(item.id) : unpinSession(item.id);
  call.then((res: any) => {
    if (res && res.success) {
      updateSessionInBuckets(item.id, {
        is_pinned: pin,
        pinned_at: pin ? new Date().toISOString() : null,
      });
    } else {
      MessagePlugin.error(pin ? t('menu.pinFailed') : t('menu.unpinFailed'));
    }
  }).catch(() => {
    MessagePlugin.error(pin ? t('menu.pinFailed') : t('menu.unpinFailed'));
  }).finally(() => {
    pinningIds.value.delete(item.id);
  });
};

const clearMessages = (item: any) => {
  clearSessionMessages(item.id).then((res: any) => {
    if (res && res.success) {
      MessagePlugin.success(t('menu.clearMessagesSuccess'));
      if (item.id === route.params.chatid) {
        window.dispatchEvent(new CustomEvent('session-messages-cleared', { detail: { sessionId: item.id } }));
      }
    } else {
      MessagePlugin.error(t('menu.clearMessagesFailed'));
    }
  }).catch(() => {
    MessagePlugin.error(t('menu.clearMessagesFailed'));
  });
};

const delCard = (item: any) => {
  delSession(item.id).then((res: any) => {
    if (res && (res as any).success) {
      sessionBuckets.value = removeSessionFromBuckets(sessionBuckets.value, item.id);
      syncMenuStoreFromBuckets();

      if (item.id == route.params.chatid) {
        router.push('/platform/creatChat');
      }
    } else {
      MessagePlugin.error(t('chat.deleteSessionFailed'));
    }
  })
}

// Rename dialog state. `renameTargetId` is the session being renamed; the
// dialog stays open on API failure so the user can correct and retry without
// re-opening the menu.
const renameDialogVisible = ref(false);
const renameTargetId = ref<string | null>(null);
const renameTargetTitle = ref('');
const renameInput = ref('');
const renameSubmitting = ref(false);

const openRenameDialog = (item: any) => {
  renameTargetId.value = item?.id ?? null;
  renameTargetTitle.value = item?.title ?? '';
  renameInput.value = (item?.title ?? '').trim();
  renameSubmitting.value = false;
  renameDialogVisible.value = true;
};

const cancelRename = () => {
  if (renameSubmitting.value) return;
  renameDialogVisible.value = false;
  renameTargetId.value = null;
  renameTargetTitle.value = '';
  renameInput.value = '';
};

const confirmRename = async () => {
  const next = renameInput.value.trim();
  const sessionId = renameTargetId.value;
  if (!sessionId) return;
  if (!next) {
    MessagePlugin.warning(t('menu.renameEmpty'));
    return;
  }
  if (next === renameTargetTitle.value) {
    // No-op rename: just close to avoid hitting the backend.
    renameDialogVisible.value = false;
    return;
  }
  renameSubmitting.value = true;
  try {
    const res: any = await updateSessionTitle(sessionId, next);
    if (res && res.success) {
      updateSessionInBuckets(sessionId, { title: next, isNoTitle: false });
      window.dispatchEvent(new CustomEvent('session-title-updated', {
        detail: { sessionId, title: next },
      }));
      MessagePlugin.success(t('menu.renameSuccess'));
      renameDialogVisible.value = false;
    } else {
      MessagePlugin.error(t('menu.renameFailed'));
    }
  } catch {
    MessagePlugin.error(t('menu.renameFailed'));
  } finally {
    renameSubmitting.value = false;
  }
};


const debounce = (fn: (...args: any[]) => void, delay: number) => {
  let timer: ReturnType<typeof setTimeout>
  return (...args: any[]) => {
    clearTimeout(timer)
    timer = setTimeout(() => fn(...args), delay)
  }
}
const mapSessionRow = (item: any) => ({
  title: item.title ? item.title : t('menu.newSession'),
  path: `chat/${item.id}`,
  id: item.id,
  isMore: false,
  isNoTitle: item.title ? false : true,
  created_at: item.created_at,
  updated_at: item.updated_at,
  is_pinned: !!item.is_pinned,
  pinned_at: item.pinned_at || null,
  description: item.description || '',
});

const syncMenuStoreFromBuckets = () => {
  usemenuStore.clearMenuArr();
  const flat = flattenBucketItems(sessionBuckets.value, bucketOrder.value);
  flat.forEach((item) => usemenuStore.updatemenuArr(item));
  total.value = flat.length;
};

const menuChildToSessionRow = (item: Record<string, unknown>): SessionForGrouping & { path: string } => {
  const id = String(item.id);
  return {
    id,
    path: typeof item.path === 'string' ? item.path : `chat/${id}`,
    title: typeof item.title === 'string' ? item.title : undefined,
    is_pinned: !!item.is_pinned,
    created_at: typeof item.created_at === 'string' ? item.created_at : undefined,
    updated_at: typeof item.updated_at === 'string' ? item.updated_at : undefined,
    description: typeof item.description === 'string' ? item.description : '',
  };
};

const sessionExistsInBuckets = (sessionId: string) =>
  Object.values(sessionBuckets.value).some((bucket) => bucket.items.some((row) => row.id === sessionId));

/** 创建会话后 menuStore 已乐观写入，但列表实际渲染自 sessionBuckets，需补齐。 */
const ensureSessionInSidebar = (sessionId: string) => {
  if (!sessionId || sessionExistsInBuckets(sessionId)) return;

  const web = sessionBuckets.value.web;
  if (!web) return;

  const chatMenu = (menuArr.value as unknown as MenuItem[]).find((item) => item.path === 'creatChat');
  const fromStore = (chatMenu?.children as Record<string, unknown>[] | undefined)
    ?.find((item) => item.id === sessionId);
  if (!fromStore) return;

  sessionBuckets.value = {
    ...sessionBuckets.value,
    web: prependSessionToWebBucket(web, menuChildToSessionRow(fromStore)),
  };
  total.value = flattenBucketItems(sessionBuckets.value, bucketOrder.value).length;
};

const rebuildBucketDefinitions = () => buildBucketDefinitions(t('menu.myChats'));

const loadBucketPage = async (key: string, page?: number, token?: number) => {
  const activeToken = token ?? bucketRequestToken;
  const bucket = sessionBuckets.value[key];
  if (!bucket || bucket.loading) return;

  const nextPage = page ?? bucket.page + 1;
  sessionBuckets.value = {
    ...sessionBuckets.value,
    [key]: { ...bucket, loading: true },
  };

  try {
    const res: any = await getSessionsList(nextPage, SIDEBAR_BUCKET_PAGE_SIZE);
    if (activeToken !== bucketRequestToken) return;
    const rows = (res?.data || []).map((item: any) => mapSessionRow(item));
    const current = sessionBuckets.value[key];
    sessionBuckets.value = {
      ...sessionBuckets.value,
      [key]: mergeBucketPage(current, rows, res?.total ?? rows.length, nextPage),
    };
    syncMenuStoreFromBuckets();
    await refreshSessionListScrollability();
  } catch {
    if (activeToken !== bucketRequestToken) return;
    const current = sessionBuckets.value[key];
    sessionBuckets.value = {
      ...sessionBuckets.value,
      [key]: { ...current, loading: false, loaded: true },
    };
  }
};

const initSessionBuckets = async () => {
  const token = ++bucketRequestToken;
  sessionListBooting.value = true;

  const defs = rebuildBucketDefinitions();
  bucketOrder.value = defs.map((def) => def.key);
  const buckets: Record<string, SidebarSessionBucket> = {};
  for (const def of defs) {
    buckets[def.key] = createEmptyBucket(def);
  }
  sessionBuckets.value = buckets;

  await loadBucketPage('web', 1, token);

  if (token === bucketRequestToken) {
    sessionListBooting.value = false;
    syncMenuStoreFromBuckets();
    await ensureBucketFillsViewport('web');
    await refreshSessionListScrollability();
  }
};

const getMessageList = async () => {
  await initSessionBuckets();
};

// 滚动到底时为当前筛选来源加载下一页
const checkScrollBottom = async () => {
  const container = scrollContainer.value;
  const key = activeSessionBucketKey.value;
  const bucket = sessionBuckets.value[key];
  if (!container || !bucket || !bucketHasMore(bucket) || bucket.loading) return;

  const { scrollTop, scrollHeight, clientHeight } = container;
  const hasOverflow = scrollHeight > clientHeight + 1;
  if (!hasOverflow) {
    await ensureBucketFillsViewport(key);
    return;
  }

  const isNearBottom = scrollHeight - (scrollTop + clientHeight) < 100;
  if (!isNearBottom) return;

  await loadBucketPage(key);
};

const handleScroll = debounce(checkScrollBottom, 200);

const handleSessionTitleUpdated = (event: Event) => {
  const detail = (event as CustomEvent<{ sessionId?: string; title?: string }>).detail;
  if (!detail?.sessionId || !detail.title) return;
  updateSessionInBuckets(detail.sessionId, { title: detail.title, isNoTitle: false });
};

onMounted(async () => {
  const routeName = typeof route.name === 'string' ? route.name : (route.name ? String(route.name) : '')
  currentpath.value = routeName;
  if (route.params.chatid) {
    currentSecondpath.value = `chat/${route.params.chatid}`;
  }

  window.addEventListener('session-title-updated', handleSessionTitleUpdated);

  isLiteEdition.value = authStore.isLiteMode
  getSystemInfo().then(res => {
    if (res.data?.edition === 'lite') {
      isLiteEdition.value = true
      authStore.setLiteMode(true)
    }
  }).catch(() => { })

  await getMessageList();
  const initialChatId = route.params.chatid as string | undefined;
  if (initialChatId) {
    ensureSessionInSidebar(initialChatId);
  }
});

onUnmounted(() => {
  window.removeEventListener('session-title-updated', handleSessionTitleUpdated);
});

watch([() => route.name, () => route.params], (newvalue, oldvalue) => {
  const nameStr = typeof newvalue[0] === 'string' ? (newvalue[0] as string) : (newvalue[0] ? String(newvalue[0]) : '')
  currentpath.value = nameStr;
  if (newvalue[1].chatid) {
    currentSecondpath.value = `chat/${newvalue[1].chatid}`;
  } else {
    currentSecondpath.value = "";
  }

  // 创建新会话时 creatChat 会先 updataMenuChildren，再跳转 chat/:id。
  // 侧栏实际渲染 sessionBuckets，需按 buckets 判断是否缺失，不能把 menuStore 当真相来源。
  const newChatId = (newvalue[1] as any)?.chatid as string | undefined;
  if (nameStr === 'chat' && newChatId) {
    ensureSessionInSidebar(newChatId);
  }

  // 路由变化时更新图标状态（不涉及对话列表）
  getIcon(nameStr);
});
let prefixIcon = ref('prefixIcon.svg');
let logoutIcon = ref('logout.svg');
let settingIcon = ref('setting.svg');
let pathPrefix = ref(route.name)
const getIcon = (path: string) => {
  // 根据当前路由状态更新所有图标
  const creatChatActiveState = getIconActiveState('creatChat');
  const settingsActiveState = getIconActiveState('settings');

  // 对话图标：只在对话创建页面显示蓝色，其他情况显示默认
  prefixIcon.value = creatChatActiveState.isCreatChatActive ? 'prefixIcon-green.svg' : 'prefixIcon.svg';

  // 设置图标：只在设置页面显示蓝色
  settingIcon.value = settingsActiveState.isSettingsActive ? 'setting-green.svg' : 'setting.svg';

  // 退出图标：始终显示默认
  logoutIcon.value = 'logout.svg';
}
getIcon(typeof route.name === 'string' ? route.name as string : (route.name ? String(route.name) : ''))
const handleMenuClick = async (path: string) => {
  if (path === 'settings') {
    // 设置菜单项：打开设置弹窗并跳转路由
    uiStore.openSettings()
    router.push('/platform/settings')
  } else {
    gotopage(path)
  }
}

// 处理退出登录确认
const handleLogout = () => {
  gotopage('logout')
}

const gotopage = async (path: string) => {
  pathPrefix.value = path;
  // 处理退出登录
  if (path === 'logout') {
    try {
      // 调用后端API注销
      await logoutApi();
    } catch (error) {
      // 即使API调用失败，也继续执行本地清理
      console.error('注销API调用失败:', error);
    }
    // 清理所有状态和本地存储
    authStore.logout();
    MessagePlugin.success(t('menu.logoutSuccess'));
    router.push('/login');
    return;
  } else {
    if (path === 'creatChat') {
      router.push('/platform/creatChat')
    } else {
      router.push(`/platform/${path}`);
    }
  }
  getIcon(path)
}

const getImgSrc = (url: string) => {
  return new URL(`/src/assets/img/${url}`, import.meta.url).href;
}

const mouseenteMenu = (path: string) => {
}
const mouseleaveMenu = (path: string) => {
}

const onDragHandleMouseDown = (e: MouseEvent) => {
  e.preventDefault()
  const startX = e.clientX
  const expandThreshold = 40

  const onMouseMove = (ev: MouseEvent) => {
    if (ev.clientX - startX > expandThreshold) {
      uiStore.expandSidebar()
      cleanup()
    }
  }
  const onMouseUp = () => cleanup()
  const cleanup = () => {
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}


</script>
<style lang="less" scoped>
.aside_box {
  // 侧栏水平栅格：图标列与文案列统一对齐（Logo / 菜单 / 会话分组 / 会话行）
  --sidebar-inset-x: 14px;
  --sidebar-icon-size: 18px;
  --sidebar-channel-icon: 14px;
  --sidebar-icon-gap: 8px;
  --sidebar-text-inset: calc(var(--sidebar-inset-x) + var(--sidebar-icon-size) + var(--sidebar-icon-gap)); // 40px

  min-width: 260px;
  width: 260px;
  padding: 8px 6px 6px;
  background: var(--td-bg-color-sidebar);
  box-sizing: border-box;
  /* Avoid 100vh because <html> carries a `zoom` multiplier for font-size
       control; 100vh is evaluated against the unscaled viewport and then
       scaled, so at "large" the sidebar would extend past the window. The
       ancestor chain (html/body/#app/.main) is already height: 100%. */
  height: 100%;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--td-component-stroke);
  box-shadow: 1px 0 0 rgba(0, 0, 0, 0.02);
  transition: width 0.25s ease, min-width 0.25s ease;
  position: relative;

  // macOS Wails 桌面：红绿灯位于 HiddenInset 标题栏区域，需让出顶部空间
  html.wails-desktop & {
    padding-top: 30px;
  }

  &--collapsed {
    min-width: 60px;
    width: 60px;
    padding: 8px 3px 6px;
    overflow: visible;

    .menu_item {
      justify-content: center;
      padding: 9px 0;

      .menu_item-box {
        justify-content: center;
        width: auto;
      }

      .menu_icon {
        margin-right: 0;
      }
    }

    .menu_bottom {
      align-items: center;
    }

    .menu_top {
      margin-right: 0;
      padding-right: 0;
    }

    :deep(.platform-header) {
      height: auto;

      .user-button {
        justify-content: center;
      }
    }

    :deep(.platform-header__right) {
      flex-direction: column;
      align-items: center;
      gap: 4px;
    }
  }

  .logo_row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 50px;
    flex-shrink: 0;
    padding: 0 10px 0 var(--sidebar-inset-x);
  }

  .sidebar-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    cursor: pointer;
    color: var(--td-text-color-secondary);
    border-radius: 4px;
    transition: background-color 0.2s ease;
    box-sizing: border-box;

    &:hover {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-primary);
    }
  }

  .sidebar-drag-handle {
    position: absolute;
    top: 0;
    right: -3px;
    width: 6px;
    height: 100%;
    cursor: ew-resize;
    z-index: 10;

    &:hover {
      background: var(--td-brand-color-light);
    }
  }

  .logo_box {
    display: flex;
    align-items: center;
    flex: 1;
    min-width: 0;
    overflow: hidden;

    .brand-lockup {
      display: flex;
      // flex-direction: column;
      align-items: center;
      gap: 10px;
      min-width: 0;
      color: var(--td-text-color-primary);

      .brand-name {
        font-size: 22px;
        line-height: 1;
        font-weight: 700;
        letter-spacing: 0;
      }

      .brand-product {
        margin-top: 3px;
        overflow: hidden;
        font-size: 9px;
        line-height: 1.2;
        font-weight: 500;
        color: var(--td-text-color-secondary);
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }

  }

  .logo_img {
    margin-left: 24px;
    width: 30px;
    height: 30px;
    margin-right: 7.25px;
  }

  .logo_txt {
    transform: rotate(0.049deg);
    color: var(--td-text-color-primary);
    font-family: Arial, "Helvetica Neue", sans-serif;
    font-size: 24.12px;
    font-style: normal;
    font-weight: W7;
    line-height: 21.7px;
  }

  .menu_top {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    overflow-x: hidden;
    min-height: 0;
    // 抵消 .aside_box 的右内边距，让滚动条贴近面板右缘；
    // 等量 padding 补回，保证列表文字位置不变。
    margin-right: -4px;
    padding-right: 4px;
    margin-top: 10px;
    // Claude 风格细滚动条：默认透明，悬浮时显示一条圆角细灰条
    scrollbar-width: thin;
    scrollbar-color: transparent transparent;
    transition: scrollbar-color 0.2s ease;

    &::-webkit-scrollbar {
      width: 6px;
    }

    &::-webkit-scrollbar-track {
      background: transparent;
    }

    &::-webkit-scrollbar-thumb {
      background-color: transparent;
      border-radius: 6px;
      transition: background-color 0.2s ease;
    }

    &:hover {
      scrollbar-color: var(--td-scrollbar-color, rgba(0, 0, 0, 0.18)) transparent;

      &::-webkit-scrollbar-thumb {
        background-color: var(--td-scrollbar-color, rgba(0, 0, 0, 0.18));
      }
    }

    &::-webkit-scrollbar-thumb:hover {
      background-color: var(--td-scrollbar-hover-color, rgba(0, 0, 0, 0.32));
    }
  }

  .menu_bottom {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
  }

  .menu_box {
    display: flex;
    flex-direction: column;

    // 「新对话」吸顶：作为滚动容器(.menu_top)的直接子级，滚动时钉在顶部，
    // 知识库、智能体及历史列表一起从其下方滚走。背景遮挡滚动内容。
    &--sticky {
      position: sticky;
      top: 0;
      z-index: 2;
      background: var(--td-bg-color-sidebar);
    }
  }


  .upload-file-wrap {
    padding: 6px;
    border-radius: 3px;
    height: 32px;
    width: 32px;
    box-sizing: border-box;
  }

  .upload-file-wrap:hover {
    background-color: var(--td-brand-color-light);
    color: var(--td-brand-color);

  }

  .upload-file-icon {
    width: 20px;
    height: 20px;
    color: var(--td-text-color-secondary);
  }

  .active-upload {
    color: var(--td-brand-color);
  }

  .menu_item_active {
    border-radius: 4px;
    background: var(--td-bg-color-secondarycontainer) !important;

    .menu_icon,
    .menu_title {
      color: var(--td-brand-color) !important;
    }
  }

  .menu_item_c_active {

    .menu_icon,
    .menu_title {
      color: var(--td-text-color-primary);
    }
  }

  .menu_p {
    height: 46px;
    padding: 3px 0;
    box-sizing: border-box;
  }

  .menu_item {
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 38px;
    padding: 8px 10px 8px var(--sidebar-inset-x);
    box-sizing: border-box;
    margin-bottom: 2px;
    border-radius: 4px;
    transition: background-color 0.2s ease;

    .menu_item-box {
      display: flex;
      align-items: center;
    }

    &:hover {
      border-radius: 4px;
      background: var(--td-bg-color-container-hover);

      .menu_icon,
      .menu_title {
        color: var(--td-text-color-primary);
      }
    }
  }

  .menu_icon {
    display: flex;
    flex: 0 0 var(--sidebar-icon-size);
    width: var(--sidebar-icon-size);
    margin-right: var(--sidebar-icon-gap);
    color: var(--td-text-color-secondary);

    .icon {
      width: 18px;
      height: 18px;
      overflow: hidden;
    }
  }

  .menu_title {
    color: var(--td-text-color-primary);
    text-overflow: ellipsis;
    font-family: var(--app-font-family);
    font-size: 14px;
    font-style: normal;
    font-weight: 600;
    line-height: 20px;
    overflow: hidden;
    white-space: nowrap;
    max-width: 120px;
    flex: 1;
  }

  .submenu {
    font-family: var(--app-font-family);
    font-size: 14px;
    font-style: normal;
    min-width: 0;
    padding-top: 3px;
    margin-top: 10px;
    border-top: 1px solid var(--td-component-stroke);
  }

  .menu_item_active:has(button),
  .menu_item:has(button) {
    background: transparent !important;

    &:hover {
      background: transparent !important;
    }
  }

  :deep(.submenu_pin_icon) {
    width: 12px;
    height: 12px;
    margin-right: 4px;
    vertical-align: middle;
    flex-shrink: 0;
  }

  .submenu_source_icon {
    width: 14px;
    height: 14px;
    margin-right: 0px;
    vertical-align: middle;
    object-fit: contain;
    flex-shrink: 0;
    // 默认淡化处理，避免未选中状态下彩色图标与灰色标题不协调；
    // 悬浮或选中时恢复彩色，交互时才引人注意。
    filter: grayscale(1);
    opacity: 0.55;
    transition: filter 0.15s ease, opacity 0.15s ease;
  }

  :deep(.submenu_item:hover .submenu_source_icon),
  :deep(.submenu_item_active .submenu_source_icon) {
    filter: none;
    opacity: 1;
  }

  .history-title {
    padding: 10px 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    cursor: pointer;
    user-select: none;
    border-radius: 6px;
    margin: 2px 4px 0;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .history-title__label {
    flex: 1;
    min-width: 0;
  }

  .history-title__caret {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-secondary, #666);
    transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1), color 0.15s ease;

    &--collapsed {
      transform: rotate(-90deg);
    }
  }

  .history-title:hover .history-title__caret {
    color: var(--td-text-color-primary, #333);
  }

  // 展开/收起过渡：使用 max-height + opacity 实现平滑的高度变化
  .history-collapse-enter-active,
  .history-collapse-leave-active {
    transition: max-height 0.32s cubic-bezier(0.4, 0, 0.2, 1),
      opacity 0.22s ease,
      transform 0.32s cubic-bezier(0.4, 0, 0.2, 1);
    overflow: hidden;
    will-change: max-height, opacity, transform;
  }

  .history-collapse-enter-from,
  .history-collapse-leave-to {
    max-height: 0;
    opacity: 0;
    transform: translateY(-4px);
  }

  .history-collapse-enter-to,
  .history-collapse-leave-from {
    max-height: 1600px;
    opacity: 1;
    transform: translateY(0);
  }

  // 列表行统一栅格：左缘 inset-x + 图标槽 18px + 间距 8px → 文案列与主菜单文字对齐
  .session-list-row {
    display: flex;
    align-items: center;
    gap: var(--sidebar-icon-gap);
    padding: 0 10px 0 var(--sidebar-inset-x);
    min-width: 0;
    box-sizing: border-box;
  }

  .session-list-row__icon {
    flex: 0 0 var(--sidebar-icon-size);
    width: var(--sidebar-icon-size);
    height: var(--sidebar-icon-size);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .session-list-row__body {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
  }

  // 聊天区分组标题 / 会话行：与「聊天」节标题同列左对齐，不再预留图标槽
  .session-list-row--flat {
    padding-left: var(--sidebar-inset-x);
    gap: 0;
  }

  .session-list-loading {
    display: flex;
    align-items: center;
    min-height: 26px;
    color: var(--td-text-color-placeholder);
  }

  .timeline_header {
    font-family: var(--app-font-family);
    font-size: 11px;
    font-weight: 600;
    color: var(--td-text-color-disabled);
    padding-top: 4px;
    padding-bottom: 1px;
    margin-top: 0;
    line-height: 16px;
    user-select: none;
  }

  .timeline_header-label {
    white-space: nowrap;
  }

  .submenu_item_p {
    padding: 0;
    box-sizing: border-box;
    min-width: 0;
    overflow: hidden;

    &.session-chat-row .session-list-row {
      min-height: 30px;
      border-radius: 6px;
      transition: background 0.15s ease, color 0.15s ease;
    }

    &.session-chat-row:hover .session-list-row {
      background: var(--td-bg-color-container-hover);

      :deep(.menu-more) {
        color: var(--td-text-color-primary);
      }

      :deep(.menu-more-wrap) {
        opacity: 1;
      }
    }

    &.session-chat-row--active .session-list-row {
      background: var(--td-bg-color-container-hover);

      :deep(.submenu_item) {
        color: var(--td-brand-color);
      }

      :deep(.menu-more) {
        color: var(--td-text-color-primary);
      }

      :deep(.menu-more-wrap) {
        opacity: 1;
      }
    }

    &.session-chat-row--selected .session-list-row {
      background: rgba(11, 65, 205, 0.05);
    }
  }

  // SessionSidebarRow 为子组件，需 :deep 才能让标题省略号生效
  :deep(.submenu_item) {
    cursor: pointer;
    display: flex;
    align-items: center;
    color: var(--td-text-color-primary);
    font-weight: 400;
    font-size: 14px;
    line-height: 20px;
    height: 100%;
    width: 100%;
    padding: 6px 0;
    position: relative;
    min-width: 0;
    background: transparent;

    .submenu_title {
      display: flex;
      align-items: center;
      flex: 1 1 auto;
      min-width: 0;
      overflow: hidden;
    }

    .submenu_title-text {
      flex: 1 1 auto;
      min-width: 0;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }

    .menu-more-wrap {
      opacity: 0;
      transition: opacity 0.2s ease;
      flex-shrink: 0;
    }

    .menu-more {
      display: inline-block;
      font-weight: bold;
      color: var(--td-brand-color);
    }

    .submenu_title--batch {
      margin-left: 4px;
    }

    &.submenu_item_batch {
      padding-left: 0;
    }
  }

  :deep(.submenu_item_batch) {
    cursor: pointer;
    user-select: none;
  }

  .batch-checkbox {
    flex-shrink: 0;
  }

}

.batch-inline-footer {
  position: sticky;
  bottom: 0;
  z-index: 2;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);

  .batch-footer-left {
    display: flex;
    align-items: center;
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }

  .batch-footer-right {
    display: flex;
    align-items: center;
    gap: 6px;
  }
}

/* 知识库下拉菜单样式 */
.kb-dropdown-icon {
  margin-left: auto;
  color: var(--td-text-color-secondary);
  transition: transform 0.3s ease, color 0.2s ease;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;

  &.rotate-180 {
    transform: rotate(180deg);
  }

  &:hover {
    color: var(--td-brand-color);
  }

  &.active {
    color: var(--td-brand-color);
  }

  &.active:hover {
    color: var(--td-brand-color-active);
  }

  svg {
    width: 12px;
    height: 12px;
    transition: inherit;
  }
}

.kb-dropdown-menu {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  box-shadow: var(--td-shadow-2);
  z-index: 1000;
  max-height: 200px;
  overflow-y: auto;
}

.kb-dropdown-item {
  padding: 8px 16px;
  cursor: pointer;
  transition: background-color 0.2s ease;
  font-size: 14px;
  color: var(--td-text-color-primary);

  &:hover {
    background-color: var(--td-bg-color-container-hover);
  }

  &.active {
    background-color: var(--td-brand-color-light);
    color: var(--td-brand-color);
    font-weight: 500;
  }

  &:first-child {
    border-radius: 6px 6px 0 0;
  }

  &:last-child {
    border-radius: 0 0 6px 6px;
  }
}

.menu_item-box {
  display: flex;
  align-items: center;
  width: 100%;
  position: relative;
}

/* Empty state when there are no sessions. */
.submenu_empty {
  padding: 24px 14px;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  user-select: none;
}

// 顶部 logo_row 右侧的图标按钮组（搜索 + 折叠），与折叠按钮风格一致
.logo_actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.header-icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  flex-shrink: 0;
  cursor: pointer;
  border-radius: 6px;
  color: var(--td-text-color-secondary);
  transition: background-color 0.2s ease;
  box-sizing: border-box;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  .header-icon-img {
    width: 18px;
    height: 18px;
    display: block;
  }
}

// 深色 tooltip 内容：标签 + 浅灰快捷键内联
.cmdk-tip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;

  .cmdk-tip-label {
    font-size: 13px;
  }

  .cmdk-tip-keys {
    font-size: 13px;
    opacity: 0.6;
    letter-spacing: 0.5px;
  }
}

.menu-pending-badge {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  margin-left: 6px;
  border-radius: 9px;
  background: rgba(250, 173, 20, 0.2);
  color: var(--td-warning-color);
  font-size: 12px;
  font-weight: 600;
  line-height: 18px;
  text-align: center;
  flex-shrink: 0;
}

.menu_box {
  position: relative;
}
</style>
<style lang="less">
// 下拉菜单样式已统一至 @/assets/dropdown-menu.less

// 退出登录确认框样式
:deep(.t-popconfirm) {
  .t-popconfirm__content {
    background: var(--td-bg-color-container);
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    box-shadow: var(--td-shadow-3);
    padding: 12px 16px;
    font-size: 14px;
    color: var(--td-text-color-primary);
    max-width: 200px;
  }

  .t-popconfirm__arrow {
    border-bottom-color: var(--td-component-stroke);
  }

  .t-popconfirm__arrow::after {
    border-bottom-color: var(--td-bg-color-container);
  }

  .t-popconfirm__buttons {
    margin-top: 8px;
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }

  .t-button--variant-outline {
    border-color: var(--td-component-border);
    color: var(--td-text-color-secondary);
  }

  .t-button--theme-danger {
    background-color: var(--td-error-color);
    border-color: var(--td-error-color);
  }

  .t-button--theme-danger:hover {
    background-color: var(--td-error-color);
    border-color: var(--td-error-color);
  }
}
</style>
