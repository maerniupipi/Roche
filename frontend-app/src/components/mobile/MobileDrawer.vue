<template>
  <t-drawer v-model:visible="drawerVisible" placement="left" :close-on-overlay-click="true" :show-overlay="true"
    :destroy-on-close="false" :z-index="1000">
    <div class="mobile-drawer__topbar">
      <div class="mobile-drawer__logo" role="button" tabindex="0" :aria-label="t('platform.title')"
        @click="handleLogoClick" @keydown.enter.prevent="handleLogoClick">
        <img :src="logoUrl" class="mobile-drawer__logo-img" alt="Roche Logo" />
      </div>

      <div class="mobile-drawer__topbar-actions">
        <button type="button" class="lang-switcher" :title="t('header.languageSwitch')"
          :aria-label="t('header.languageSwitch')" @click="toggleLocale">
          <span class="lang-switcher__label">{{ currentLocaleLabel }}</span>
        </button>
        <button type="button" class="mobile-drawer__close" :title="t('common.close')" :aria-label="t('common.close')"
          @click="closeDrawer">
          <t-icon name="close" size="20px" />
        </button>
      </div>
    </div>
    <div class="mobile-drawer__body" ref="bodyContainer">
      <!-- 顶部固定区：与 menu.vue#topMenuItems 完全对齐的快捷入口（不参与滚动） -->
      <div class="mobile-drawer__menu-block">
        <div v-for="item in topMenuItems" :key="item.path" class="mobile-drawer__menu-item"
          :class="{ 'mobile-drawer__menu-item--active': isMenuItemActive(item.path) }" :data-guide="`nav-${item.path}`">
          <template v-if="item.titleKey === 'menu.newChat'">
            <t-button theme="primary" block @click="handleMenuClick(item.path)">
              <template #icon><t-icon name="add" size="14px" /></template>
              {{ item.title }}
            </t-button>
          </template>
          <template v-else>
            <button type="button" class="mobile-drawer__menu-button" @click="handleMenuClick(item.path)">
              <t-icon :name="item.icon === 'prefixIcon' ? 'chat' : item.icon" size="18px" />
              <span class="mobile-drawer__menu-title" :title="item.title">{{ item.title }}</span>
            </button>
          </template>
        </div>
      </div>

      <!-- 历史对话区：仅内部滚动（flex: 1 + min-height: 0 + overflow-y: auto） -->
      <div class="mobile-drawer__history" v-if="allSessions.length > 0 || sessionListBooting">
        <div class="mobile-drawer__history-title" role="button" tabindex="0" :aria-expanded="historyExpanded"
          @click="toggleHistoryExpanded" @keydown.enter.prevent="toggleHistoryExpanded"
          @keydown.space.prevent="toggleHistoryExpanded">
          <span class="mobile-drawer__history-title-label">{{ t('menu.historyTitle') }}</span>
          <span class="mobile-drawer__history-caret"
            :class="{ 'mobile-drawer__history-caret--collapsed': !historyExpanded }">
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" xmlns="http://www.w3.org/2000/svg"
              aria-hidden="true">
              <path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"
                stroke-linejoin="round" />
            </svg>
          </span>
        </div>

        <Transition name="history-fade">
          <div v-if="historyExpanded" ref="scrollContainer" class="mobile-drawer__history-content"
            @scroll="handleScroll">
            <template v-if="sessionListBooting && allSessions.length === 0">
              <div v-for="n in 4" :key="'skel-' + n" class="mobile-drawer__row">
                <t-skeleton animation="gradient" :row-col="[{ width: '100%', height: '14px' }]" />
              </div>
            </template>

            <template v-else-if="groupedSessions.length === 0">
              <div class="mobile-drawer__empty">{{ t('menu.noSessions') }}</div>
            </template>

            <template v-else>
              <template v-for="group in groupedSessions" :key="group.key">
                <div v-if="group.label" class="mobile-drawer__group-label">
                  {{ group.label }}
                </div>
                <div v-for="sessionItem in group.items" :key="sessionItem.id" class="mobile-drawer__row"
                  :class="{ 'mobile-drawer__row--active': sessionItem.path === currentSecondpath }">
                  <SessionSidebarRow :item="sessionItem" :batch-mode="false" :active-path="currentSecondpath"
                    :selected-ids="[]" :menu-options="buildSessionMenuOptions(sessionItem)"
                    @navigate="gotoChat(sessionItem.path)" @toggle-select="() => { }"
                    @menu-click="(data) => handleSessionMenuClick(data, sessionItem)" @hover-in="() => { }"
                    @hover-out="() => { }" />
                </div>
              </template>
              <div v-if="bucket?.loading" class="mobile-drawer__loading">
                <t-loading size="small" />
              </div>
            </template>
          </div>
        </Transition>
      </div>
    </div>
    <template #footer>
      <div class="mobile-drawer__footer">
        <UserMenu />
      </div>
    </template>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  Button as TButton,
  Icon as TIcon,
  MessagePlugin,
  Skeleton as TSkeleton,
  Loading as TLoading,
} from 'tdesign-mobile-vue'
import logoUrl from '@/assets/img/logo.svg'
import SessionSidebarRow from '@/components/SessionSidebarRow.vue'
import UserMenu from '@/components/UserMenu.vue'
import { useMobileMenu } from '@/composables/useMobileMenu'
import {
  buildBucketDefinitions,
  createEmptyBucket,
  flattenBucketItems,
  mergeBucketPage,
  prependSessionToWebBucket,
  removeSessionFromBuckets,
  SIDEBAR_BUCKET_PAGE_SIZE,
  type SidebarSessionBucket,
} from '@/components/sessionSidebarBuckets'
import {
  classifyDateBucket,
  groupSessionsByDate,
  type DateBucketKey,
  type SessionForGrouping,
} from '@/components/sessionGrouping'
import {
  clearSessionMessages,
  delSession,
  getSessionsList,
  pinSession,
  unpinSession,
} from '@/api/chat'
import { logout as logoutApi } from '@/api/auth'
import { isDebugger } from '@/composables/featureFlags'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore, type MenuItem } from '@/stores/menu'
import { useUIStore } from '@/stores/ui'

const { locale, t } = useI18n()
const route = useRoute()
const router = useRouter()
const mobileMenu = useMobileMenu()

// === Drawer 可见性（绑定到 composable） ===
const drawerVisible = computed<boolean>({
  get: () => mobileMenu.isOpen.value,
  set: (v) => {
    if (v) mobileMenu.open()
    else mobileMenu.close()
  },
})

function closeDrawer(): void {
  mobileMenu.close()
}

// === Logo 点击回首页 ===
function handleLogoClick(): void {
  closeDrawer()
  void router.push('/platform/creatChat')
}

// === 中英文切换（迁移自 PlatformHeader.vue） ===
const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

const currentLocale = computed<SupportedLocale>(() => {
  const v = String(locale.value || '')
  return (SUPPORTED_LOCALES as readonly string[]).includes(v)
    ? (v as SupportedLocale)
    : 'zh-CN'
})

const currentLocaleLabel = computed(() =>
  currentLocale.value === 'zh-CN'
    ? t('header.languageZh')
    : t('header.languageEn'),
)

const TOGGLE_NEXT: Record<SupportedLocale, SupportedLocale> = {
  'zh-CN': 'en-US',
  'en-US': 'zh-CN',
}

function setLocale(next: SupportedLocale): void {
  if (locale.value === next) return
  try {
    localStorage.setItem('locale', next)
  } catch {
    // ignore storage errors
  }
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('lang', next.startsWith('zh') ? 'zh-CN' : 'en')
  }
  locale.value = next
}

function toggleLocale(): void {
  setLocale(TOGGLE_NEXT[currentLocale.value])
}

// === Store / 路由实例 ===
const usemenuStore = useMenuStore()
const authStore = useAuthStore()
const uiStore = useUIStore()
const { menuArr, visibleMenuArr } = storeToRefs(usemenuStore)

// === 菜单项激活态判断（与 menu.vue 一致） ===
function isMenuItemActive(itemPath: string): boolean {
  const currentRoute = route.name
  switch (itemPath) {
    case 'creatChat':
      return currentRoute === 'globalCreatChat'
    case 'settings':
      return currentRoute === 'settings'
    default:
      return itemPath === currentSecondpath.value
  }
}

// 上半部分菜单项 —— 与 menu.vue#topMenuItems 完全一致
// 过滤规则：仅 creatChat 始终显示，agents 仅在调试版可见。
// Settings/Logout 在桌面 menu.vue 通过顶层循环渲染，但移动端我们已在 UserMenu
// dropdown 中暴露，故此处过滤掉。
const topMenuItems = computed<MenuItem[]>(() =>
  (visibleMenuArr.value as unknown as MenuItem[]).filter((item) =>
    (item.path === 'agents' && isDebugger.value) || item.path === 'creatChat',
  ),
)

// 历史会话区域：展开 / 收起（与 menu.vue 默认一致，初始展开）
const historyExpanded = ref(true)
function toggleHistoryExpanded(): void {
  console.log(111)
  historyExpanded.value = !historyExpanded.value
}

function handleMenuClick(path: string): void {
  closeDrawer()
  void router.push(`/platform/${path}`)
}

async function handleLogout(): Promise<void> {
  try {
    await logoutApi()
  } catch (error) {
    // 即使后端失败，也继续本地清理
    console.error('Logout API failed:', error)
  }
  authStore.logout()
  MessagePlugin.success(t('auth.logout'))
  closeDrawer()
  await router.push('/login')
}

// === 历史对话列表 ===
const currentSecondpath = computed(() => String(route.params.chatid || ''))

interface SessionRow {
  id: string
  path: string
  title: string
  is_pinned?: boolean
  created_at?: string
  updated_at?: string
  description?: string
}

const sessionBuckets = ref<Record<string, SidebarSessionBucket>>({})
const bucketOrder = ref<string[]>([])
const sessionListBooting = ref(false)
const scrollContainer = ref<HTMLElement | null>(null)

const bucket = computed<SidebarSessionBucket | undefined>(() => {
  const key = bucketOrder.value[0]
  return key ? sessionBuckets.value[key] : undefined
})

const allSessions = computed<SessionRow[]>(() =>
  flattenBucketItems(sessionBuckets.value, bucketOrder.value).map((item) => ({
    id: item.id,
    path: `chat/${item.id}`,
    title: item.title || t('menu.newSession'),
    is_pinned: !!item.is_pinned,
    created_at: item.created_at,
    updated_at: item.updated_at,
    description: item.description || '',
  })),
)

const dateBucketLabels = computed<Record<DateBucketKey, string>>(() => ({
  pinned: t('time.pinned'),
  today: t('time.today'),
  yesterday: t('time.yesterday'),
  last7Days: t('time.last7Days'),
  last30Days: t('time.last30Days'),
  lastYear: t('time.lastYear'),
  earlier: t('time.earlier'),
}))

const groupedSessions = computed(() => {
  if (!allSessions.value.length) return []
  return groupSessionsByDate(
    allSessions.value,
    dateBucketLabels.value,
    (session) => classifyDateBucket(session.updated_at || session.created_at),
  )
})

const mapSessionRow = (item: any): SessionRow => ({
  id: item.id,
  path: `chat/${item.id}`,
  title: item.title || t('menu.newSession'),
  is_pinned: !!item.is_pinned,
  created_at: item.created_at,
  updated_at: item.updated_at,
  description: item.description || '',
})

/**
 * 把 menuStore 中 creatChat 子节点映射成 bucket 行（与 menu.vue 完全一致）。
 * creatChat.vue 创建会话时先把对象 push 进 menuArr[creatChat].children，
 * 再 router.push('/platform/chat/{id}')，所以这里以 sessionId 在 menuStore 为乐观
 * 写入源，把该行补到 web bucket 头部。
 */
const menuChildToSessionRow = (item: Record<string, unknown>): SessionForGrouping & { path: string } => {
  const id = String(item.id)
  return {
    id,
    path: typeof item.path === 'string' ? item.path : `chat/${id}`,
    title: typeof item.title === 'string' ? item.title : undefined,
    is_pinned: !!item.is_pinned,
    created_at: typeof item.created_at === 'string' ? item.created_at : undefined,
    updated_at: typeof item.updated_at === 'string' ? item.updated_at : undefined,
    description: typeof item.description === 'string' ? item.description : '',
  }
}

const sessionExistsInBuckets = (sessionId: string): boolean =>
  Object.values(sessionBuckets.value).some((b) => b.items.some((row) => row.id === sessionId))

/** 创建会话后 menuStore 已乐观写入，但侧栏实际渲染自 sessionBuckets —— 需补齐。 */
const ensureSessionInSidebar = (sessionId: string): void => {
  if (!sessionId || sessionExistsInBuckets(sessionId)) return
  const web = sessionBuckets.value.web
  if (!web) return
  const chatMenu = (menuArr.value as unknown as MenuItem[]).find((item) => item.path === 'creatChat')
  const fromStore = (chatMenu?.children as Record<string, unknown>[] | undefined)?.find(
    (item) => item.id === sessionId,
  )
  if (!fromStore) return
  sessionBuckets.value = {
    ...sessionBuckets.value,
    web: prependSessionToWebBucket(web, menuChildToSessionRow(fromStore)),
  }
}

/** 任何对 buckets 的修改（init/load/delete/pin）都同步回 menuStore —— 与 menu.vue 完全一致 */
const syncMenuStoreFromBuckets = (): void => {
  usemenuStore.clearMenuArr()
  const flat = flattenBucketItems(sessionBuckets.value, bucketOrder.value)
  flat.forEach((item) => usemenuStore.updatemenuArr(item))
}

/** 监听 chat/index.vue 在生成会话标题后 dispatch 的 'session-title-updated' 事件 —— 与 menu.vue 一致 */
const handleSessionTitleUpdated = (event: Event): void => {
  const detail = (event as CustomEvent<{ sessionId?: string; title?: string }>).detail
  if (!detail?.sessionId || !detail.title) return
  const next: Record<string, SidebarSessionBucket> = {}
  for (const [key, b] of Object.entries(sessionBuckets.value)) {
    next[key] = {
      ...b,
      items: b.items.map((row) =>
        row.id === detail.sessionId ? { ...row, title: detail.title, isNoTitle: false } : row,
      ),
    }
  }
  sessionBuckets.value = next
}

async function loadBucketPage(key: string, page?: number): Promise<void> {
  const b = sessionBuckets.value[key]
  if (!b || b.loading) return

  const nextPage = page ?? b.page + 1
  sessionBuckets.value = {
    ...sessionBuckets.value,
    [key]: { ...b, loading: true },
  }

  try {
    const res: any = await getSessionsList(nextPage, SIDEBAR_BUCKET_PAGE_SIZE)
    const rows = (res?.data || []).map((item: any) => mapSessionRow(item))
    const current = sessionBuckets.value[key]
    sessionBuckets.value = {
      ...sessionBuckets.value,
      [key]: mergeBucketPage(current, rows, res?.total ?? rows.length, nextPage),
    }
    syncMenuStoreFromBuckets()
  } catch {
    const current = sessionBuckets.value[key]
    sessionBuckets.value = {
      ...sessionBuckets.value,
      [key]: { ...current, loading: false, loaded: true },
    }
  }
}

async function initSessionBuckets(): Promise<void> {
  sessionListBooting.value = true
  const defs = buildBucketDefinitions(t('menu.myChats'))
  bucketOrder.value = defs.map((def) => def.key)
  const buckets: Record<string, SidebarSessionBucket> = {}
  for (const def of defs) {
    buckets[def.key] = createEmptyBucket(def)
  }
  sessionBuckets.value = buckets
  await loadBucketPage('web', 1)
  sessionListBooting.value = false
  syncMenuStoreFromBuckets()
  // 深链刷新场景：当前路由已经是 chat/:id，把 menuStore 那条补到 buckets 头部
  const initialChatId = route.params.chatid as string | undefined
  if (initialChatId) {
    ensureSessionInSidebar(initialChatId)
  }
}

async function ensureBucketFillsViewport(key: string): Promise<void> {
  const MAX_ITERATIONS = 20
  for (let i = 0; i < MAX_ITERATIONS; i++) {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    const container = scrollContainer.value
    const b = sessionBuckets.value[key]
    if (!container || !b || !bucketHasMore(b) || b.loading) break
    const hasOverflow = container.scrollHeight > container.clientHeight + 1
    if (hasOverflow) break
    const prevCount = b.items.length
    await loadBucketPage(key)
    if ((sessionBuckets.value[key]?.items.length ?? 0) <= prevCount) break
  }
}

function bucketHasMore(b: SidebarSessionBucket): boolean {
  return b.items.length < b.total
}

const handleScroll = (() => {
  let timer: ReturnType<typeof setTimeout> | null = null
  return () => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(async () => {
      const container = scrollContainer.value
      const key = bucketOrder.value[0]
      const b = key ? sessionBuckets.value[key] : undefined
      if (!container || !b || !bucketHasMore(b) || b.loading) return
      const { scrollTop, scrollHeight, clientHeight } = container
      const hasOverflow = scrollHeight > clientHeight + 1
      if (!hasOverflow) {
        await ensureBucketFillsViewport(key)
        return
      }
      const isNearBottom = scrollHeight - (scrollTop + clientHeight) < 100
      if (!isNearBottom) return
      await loadBucketPage(key)
    }, 200)
  }
})()

// === Session 菜单选项（pin/clear/delete） ===
function buildSessionMenuOptions(item: SessionRow) {
  const options: any[] = []
  if (item.is_pinned) {
    options.push({
      content: t('menu.unpin'),
      value: 'unpin',
      prefixIcon: () => h(TIcon, { name: 'pin', size: '16px' }),
    })
  } else {
    options.push({
      content: t('menu.pin'),
      value: 'pin',
      prefixIcon: () => h(TIcon, { name: 'pin', size: '16px' }),
    })
  }
  if (isDebugger.value) {
    options.push({
      content: t('menu.clearMessages'),
      value: 'clearMessages',
      prefixIcon: () => h(TIcon, { name: 'clear', size: '16px' }),
    })
  }
  options.push({
    content: t('upload.deleteRecord'),
    value: 'delete',
    theme: 'error',
    prefixIcon: () => h(TIcon, { name: 'delete', size: '16px' }),
  })
  return options
}

function handleSessionMenuClick(data: { value: string }, item: SessionRow): void {
  const v = data?.value
  if (v === 'delete') {
    delSession(item.id).then((res: any) => {
      if (res?.success) {
        sessionBuckets.value = removeSessionFromBuckets(sessionBuckets.value, item.id)
        syncMenuStoreFromBuckets()
        if (item.id === route.params.chatid) {
          void router.push('/platform/creatChat')
        }
      } else {
        MessagePlugin.error(t('chat.deleteSessionFailed'))
      }
    })
  } else if (v === 'clearMessages') {
    clearSessionMessages(item.id).then((res: any) => {
      if (res?.success) {
        MessagePlugin.success(t('menu.clearMessagesSuccess'))
        if (item.id === route.params.chatid) {
          window.dispatchEvent(
            new CustomEvent('session-messages-cleared', { detail: { sessionId: item.id } }),
          )
        }
      } else {
        MessagePlugin.error(t('menu.clearMessagesFailed'))
      }
    })
  } else if (v === 'pin' || v === 'unpin') {
    const pin = v === 'pin'
    const call = pin ? pinSession(item.id) : unpinSession(item.id)
    call.then((res: any) => {
      if (res?.success) {
        const next: Record<string, SidebarSessionBucket> = {}
        for (const [k, b] of Object.entries(sessionBuckets.value)) {
          next[k] = {
            ...b,
            items: b.items.map((row) =>
              row.id === item.id
                ? { ...row, is_pinned: pin, pinned_at: pin ? new Date().toISOString() : null }
                : row,
            ),
          }
        }
        sessionBuckets.value = next
        syncMenuStoreFromBuckets()
      } else {
        MessagePlugin.error(pin ? t('menu.pinFailed') : t('menu.unpinFailed'))
      }
    })
  }
}

function gotoChat(path: string): void {
  closeDrawer()
  void router.push(`/platform/${path}`)
}

// 首次打开 Drawer 时加载列表
watch(
  () => mobileMenu.isOpen.value,
  async (open) => {
    if (open && allSessions.value.length === 0 && !sessionListBooting.value) {
      await initSessionBuckets()
      const key = bucketOrder.value[0]
      if (key) await ensureBucketFillsViewport(key)
    }
  },
)

// 路由变化时检测新建会话（creatChat → chat/{id}）。
// 复用 menu.vue 的 watch 逻辑：路由到 chat 路由且 chatid 不在 buckets 时，从 menuStore 补齐。
watch(
  [() => route.name, () => route.params],
  () => {
    const nameStr =
      typeof route.name === 'string'
        ? route.name
        : route.name
          ? String(route.name)
          : ''
    const newChatId = (route.params as { chatid?: string }).chatid
    if (nameStr === 'chat' && newChatId) {
      ensureSessionInSidebar(newChatId)
      mobileMenu.setActiveSessionId(newChatId)
    } else {
      mobileMenu.setActiveSessionId(null)
    }
  },
)

onMounted(() => {
  // 监听 chat/index.vue 在生成会话标题后 dispatch 的 'session-title-updated' 事件
  window.addEventListener('session-title-updated', handleSessionTitleUpdated)

  // 初次加载
  if (allSessions.value.length === 0 && !sessionListBooting.value) {
    void initSessionBuckets()
  }
})

onUnmounted(() => {
  window.removeEventListener('session-title-updated', handleSessionTitleUpdated)
})
</script>

<style lang="less">
.t-drawer:has(.mobile-drawer__body) {
  --td-drawer-width: 80vw;
  width: 80vw !important;
  max-width: 320px !important;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-sizing: border-box;
  pointer-events: auto !important;

  .t-drawer__sidebar {
    display: none !important;
  }

  .t-drawer__footer {
    flex: none !important;
    padding: 0 !important;
    border-top: 0 !important;
  }
}

/* === 顶部条 === */
.mobile-drawer__topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 44px;
  padding: 0 12px 0 16px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--td-component-stroke);
}

.mobile-drawer__logo {
  display: flex;
  align-items: center;
  cursor: pointer;
  user-select: none;
  height: 28px;
}

.mobile-drawer__logo-img {
  height: 28px;
  width: auto;
  -webkit-user-drag: none;
}

.mobile-drawer__topbar-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.mobile-drawer__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  margin: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-primary);
  cursor: pointer;
  transition: background-color 0.15s ease;

  &:hover {
    background: var(--td-gray-color-1, rgba(0, 0, 0, 0.04));
  }

  &:active {
    transform: scale(0.96);
  }
}

/* === 中英文切换（迁移自 PlatformHeader.vue） === */
.lang-switcher {
  appearance: none;
  border: 1px solid var(--td-component-stroke);
  background: transparent;
  cursor: pointer;
  height: 32px;
  min-width: 40px;
  padding: 0 10px;
  border-radius: 6px;
  display: inline-flex;
  white-space: nowrap;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-primary);
  font-size: 13px;
  font-weight: 600;
  font-family: var(--app-font-family);
  letter-spacing: 0.2px;
  line-height: 1;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;

  &:hover {
    background: var(--td-gray-color-1, rgba(0, 0, 0, 0.04));
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
  }

  &:active {
    transform: scale(0.97);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 1px;
  }
}

.lang-switcher__label {
  display: inline-block;
  line-height: 1;
}

/* === 中部历史对话列表 === */
.mobile-drawer__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 0;
}

/* 顶部固定区（新建聊天等快捷入口）—— 不参与滚动 */
.mobile-drawer__menu-block {
  flex-shrink: 0;
  padding: 8px 0;
}

.mobile-drawer__group-label {
  padding: 12px 16px 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
}

.mobile-drawer__row {
  padding: 0 8px;
}

.mobile-drawer__row--active {
  background: var(--td-brand-color-light, var(--td-bg-color-container-hover));
  border-radius: 6px;
}

.mobile-drawer__skeleton-row {
  padding: 12px 16px;
}

.mobile-drawer__empty {
  padding: 24px 16px;
  text-align: center;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.mobile-drawer__loading {
  display: flex;
  justify-content: center;
  padding: 12px 0;
}

/* === 底部 UserMenu 粘性 === */
.mobile-drawer__footer {
  flex-shrink: 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}

:deep(.user-menu) {
  width: 100%;
  position: static;
}

:deep(.user-button) {
  width: 100%;
  justify-content: flex-start;
  padding: 8px 12px;
}

/* ============ topMenuItems (对齐 menu.vue#topMenuItems) ============ */
.mobile-drawer__menu-item {
  padding: 0 12px;
  margin-bottom: 8px;
}

.mobile-drawer__menu-item:last-child {
  margin-bottom: 0;
}

.mobile-drawer__menu-button {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 12px;
  border-radius: 6px;
  background: transparent;
  border: 0;
  color: var(--td-text-color-primary);
  cursor: pointer;
  font-size: 14px;
  line-height: 20px;
  text-align: left;
  font-family: inherit;
  transition: background-color 0.15s ease;
}

.mobile-drawer__menu-button:hover {
  background: var(--td-gray-color-1, rgba(0, 0, 0, 0.04));
}

.mobile-drawer__menu-item--active .mobile-drawer__menu-button {
  background: var(--td-brand-color-light, rgba(0, 82, 217, 0.08));
  color: var(--td-brand-color);
}

.mobile-drawer__menu-title {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ============ history-title / collapsible section (对齐 menu.vue history-title) ============ */
.mobile-drawer__history {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.mobile-drawer__history-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
  padding: 10px 16px 6px;
  cursor: pointer;
  user-select: none;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  font-weight: 500;
}

.mobile-drawer__history-title-label {
  letter-spacing: 0.02em;
}

.mobile-drawer__history-caret {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-secondary);
  transition: transform 0.2s ease;
}

.mobile-drawer__history-caret--collapsed {
  transform: rotate(-90deg);
}

/* 关键：history-content 自己 overflow-y: auto，独立滚动。
 * 配合外层 flex 1 + min-height: 0，确保占满剩余空间。
 * 不再用 max-height 2000px 截断，避免长列表被切。 */
.mobile-drawer__history-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  overscroll-behavior: contain;
}

/* history-fade transition (v-if，仅淡入淡出，无高度动画) */
.history-fade-enter-active,
.history-fade-leave-active {
  transition: opacity 0.18s ease;
}

.history-fade-enter-from,
.history-fade-leave-to {
  opacity: 0;
}
</style>