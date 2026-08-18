<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="settings-overlay">
        <div class="settings-modal">
          <!-- 关闭按钮 -->
          <button class="close-btn" @click="handleClose" :aria-label="$t('general.close')">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
          </button>

          <div class="settings-container">
            <!-- 左侧导航 -->
            <div class="settings-sidebar">
              <div class="sidebar-header">
                <h2 class="sidebar-title">{{ $t('general.settings') }}</h2>
              </div>
              <div class="settings-nav">
                <template v-for="group in navGroups" :key="group.key">
                  <div class="nav-group-title">{{ group.label }}</div>
                  <template v-for="item in group.items" :key="item.key">
                    <div :class="['nav-item', {
                      'active': currentSection === item.key,
                      'has-submenu': item.children && item.children.length > 0,
                      'expanded': expandedMenus.includes(item.key)
                    }]" @click="handleNavClick(item)">
                      <span v-if="item.emoji" class="nav-icon nav-icon-emoji">{{ item.emoji }}</span>
                      <t-icon v-else :name="item.icon" class="nav-icon" />
                      <span class="nav-label">{{ item.label }}</span>
                      <t-icon v-if="item.children && item.children.length > 0"
                        :name="expandedMenus.includes(item.key) ? 'chevron-down' : 'chevron-right'"
                        class="expand-icon" />
                    </div>

                    <!-- 子菜单 -->
                    <Transition name="submenu">
                      <div v-if="item.children && expandedMenus.includes(item.key)" class="submenu">
                        <div v-for="(child, childIndex) in item.children" :key="childIndex"
                          :class="['submenu-item', { 'active': currentSubSection === child.key }]"
                          @click.stop="handleSubMenuClick(item.key, child.key)">
                          <span class="submenu-label">{{ child.label }}</span>
                        </div>
                      </div>
                    </Transition>
                  </template>
                </template>
              </div>
            </div>

            <!-- 右侧内容区域 -->
            <div class="settings-content">
              <div class="content-wrapper" :class="{
                'content-wrapper--wide': currentSection === 'members' || currentSection === 'enterprise-org',
              }">
                <!-- 角色不允许访问当前 section（例如 deep-link 进入受限页面）—— 优先于具体 section 渲染。
                     正常导航走 navItems filter 不会到这里，但 watch(navItems) 的 fallback 会在角色降级
                     的瞬间触发；这一段做兜底兼容旧 URL。 -->
                <div v-if="!canSeeSection(currentSection)" class="section role-denied">
                  <div class="role-denied-icon">
                    <t-icon name="lock-on" size="48px" />
                  </div>
                  <div class="role-denied-title">{{ $t('settings.roleDenied.title') }}</div>
                  <div class="role-denied-desc">{{ $t('settings.roleDenied.desc') }}</div>
                </div>
                <template v-else>
                  <!-- 企业组织 -->
                  <div v-if="currentSection === 'enterprise-org'" class="section">
                    <EnterpriseOrganizations />
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import EnterpriseOrganizations from './EnterpriseOrganizations.vue'

const route = useRoute()
const router = useRouter()
const uiStore = useUIStore()
const authStore = useAuthStore()
const { t } = useI18n()

const currentSection = ref<string>('enterprise-org')
const currentSubSection = ref<string>('')
const expandedMenus = ref<string[]>([])

type NavItem = {
  key: string
  icon: string
  label: string
  emoji?: string
  children?: Array<{ key: string; label: string }>
}

type NavGroup = {
  key: string
  label: string
  items: NavItem[]
}

// 设置二级导航的最低可见角色：和 internal/router/router.go 的守卫矩阵对齐。
// 当前侧边栏只剩企业组织一项，权限矩阵由 SYSTEM_ADMIN_SECTIONS 统辖；
// 之后若再加 viewer 可访问的写面板，在这里追加 key 与角色阈值。
type RoleKey = 'viewer' | 'admin'
const SECTION_MIN_ROLE: Record<string, RoleKey> = {}

const SYSTEM_ADMIN_SECTIONS = new Set<string>([
  'enterprise-org',
])

const normalizeSettingsSection = (section: string) => {
  return section
}

const canSeeSection = (key: string): boolean => {
  if (SYSTEM_ADMIN_SECTIONS.has(key)) {
    return authStore.isSystemAdmin
  }
  const min = SECTION_MIN_ROLE[key] ?? 'viewer'
  // Hidden system administrators bypass knowledgeDomain-role checks.
  // 管理员看不到自己有权操作的入口（参考 KnowledgeDomainMembers.vue 的 canManage）。
  if (authStore.isSystemAdmin) return true
  return min === 'viewer' ? authStore.isLoggedIn : authStore.canManageKnowledge
}

const navItems = computed(() => {
  // 一律走 SECTION_MIN_ROLE 表，避免 ad-hoc 权限判断散落在多处。
  // 服务端在每条路由上仍以 g.Viewer/Admin 为准，这里只决定 UI 是
  // 否露入口；改动入口规则请同步更新 SECTION_MIN_ROLE 注释里的对照路由。
  const all: NavItem[] = [
    { key: 'enterprise-org', icon: 'organization', label: t('enterpriseOrg.title') },
  ]
  // viewer 入口然后角色一返回又消失，先卡住不渲染更稳，跟原先 members
  // 入口的策略一致。
  return all.filter((it) => canSeeSection(it.key))
})

const navGroups = computed<NavGroup[]>(() => {
  const itemMap = new Map(navItems.value.map((item) => [item.key, item]))
  const pickItems = (keys: string[]) => keys.map((key) => itemMap.get(key)).filter(Boolean) as NavItem[]
  return [
    {
      key: 'workspace',
      label: t('settings.navGroups.workspace'),
      items: pickItems(['enterprise-org']),
    },
  ].filter((group) => group.items.length > 0)
})

// 导航项点击处理
const handleNavClick = (item: any) => {
  if (item.children && item.children.length > 0) {
    // 有子菜单，切换展开状态
    const index = expandedMenus.value.indexOf(item.key)
    if (index > -1) {
      expandedMenus.value.splice(index, 1)
    } else {
      expandedMenus.value.push(item.key)
    }
    currentSubSection.value = item.children[0].key
  } else {
    currentSubSection.value = ''
  }

  // 切换到对应页面
  currentSection.value = item.key
}

// 子菜单点击处理
const handleSubMenuClick = (parentKey: string, childKey: string) => {
  currentSection.value = parentKey
  currentSubSection.value = childKey

  // 滚动到对应的模型类型区域
  setTimeout(() => {
    const element = document.querySelector(`[data-model-type="${childKey}"]`)
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, 100)
}

// 控制弹窗显示
const visible = computed(() => {
  return route.path === '/platform/settings' || uiStore.showSettingsModal
})

// 关闭弹窗
const handleClose = () => {
  uiStore.closeSettings()
  // 如果当前路由是设置页，返回上一页
  if (route.path === '/platform/settings') {
    router.back()
  }
}

// 监听初始导航设置
watch(() => uiStore.settingsInitialSection, (section) => {
  if (section && visible.value) {
    const normalizedSection = normalizeSettingsSection(section)
    currentSection.value = normalizedSection
    const navItem = (navItems.value as any[]).find((item) => item.key === normalizedSection)
    if (navItem && navItem.children && navItem.children.length > 0) {
      if (!expandedMenus.value.includes(section)) {
        expandedMenus.value.push(section)
      }
      currentSubSection.value = uiStore.settingsInitialSubSection || navItem.children[0].key
      if (uiStore.settingsInitialSubSection) {
        setTimeout(() => {
          const element = document.querySelector(`[data-model-type="${uiStore.settingsInitialSubSection}"]`)
          if (element) {
            element.scrollIntoView({ behavior: 'smooth', block: 'start' })
          }
        }, 300)
      }
    } else {
      currentSubSection.value = ''
    }
  }
}, { immediate: true })

watch(
  () => [visible.value, route.query.section],
  ([isVisible, section]) => {
    if (!isVisible || typeof section !== 'string') return
    currentSection.value = normalizeSettingsSection(section)
    currentSubSection.value = ''
  },
  { immediate: true },
)

// 用户角色刷新后，原本可见的 admin-only 面板可能消失。
// 如果 currentSection 落到了不再显示的 key 上，就回退到第一个可见项。
watch(navItems, (items) => {
  if (!items.some((item) => item.key === currentSection.value)) {
    currentSection.value = items[0]?.key || 'enterprise-org'
    currentSubSection.value = ''
  }
})

// ESC 键关闭
const handleEscape = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && visible.value) {
    handleClose()
  }
}

// 处理快捷导航事件
const handleSettingsNav = (e: CustomEvent) => {
  const { section, subsection } = e.detail
  if (section) {
    const normalizedSection = normalizeSettingsSection(section)
    currentSection.value = normalizedSection
    // 如果有子菜单，自动展开
    const navItem = (navItems.value as any[]).find((item: any) => item.key === normalizedSection)
    if (navItem && navItem.children && navItem.children.length > 0) {
      if (!expandedMenus.value.includes(section)) {
        expandedMenus.value.push(section)
      }
      // 如果有 subsection，选中对应的子菜单项
      currentSubSection.value = subsection || navItem.children[0].key
    }
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleEscape)
  window.addEventListener('settings-nav', handleSettingsNav as EventListener)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleEscape)
  window.removeEventListener('settings-nav', handleSettingsNav as EventListener)
})
</script>

<style lang="less" scoped>
/* 遮罩层 */
.settings-overlay {
  position: fixed;
  inset: 0;
  z-index: 1100;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  backdrop-filter: blur(4px);
}

/* 弹窗容器 */
.settings-modal {
  position: relative;
  width: 100%;
  // 1080×780 trades a touch of small-screen real estate for noticeably
  // less cramped tables (member list, system settings rows). Outer
  // padding is 20px so 1080 + 40 = 1120, comfortably within typical
  // laptops (1280+). Below 1100px viewport the `width: 100%` kicks in
  // and the modal shrinks to fit minus the 20px padding.
  max-width: 1080px;
  height: 780px;
  max-height: calc(100vh - 40px);
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* 关闭按钮 */
.close-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 32px;
  height: 32px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  z-index: 10;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }
}

.settings-container {
  display: flex;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

/* 左侧导航栏：略紧凑于最初版，字号与留白适中 */
.settings-sidebar {
  width: 208px;
  background-color: var(--td-bg-color-settings-modal);
  border-right: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sidebar-header {
  padding: 16px 14px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
}

.sidebar-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0;
}

.settings-nav {
  padding: 8px 8px 12px;
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.nav-group-title {
  padding: 9px 14px 4px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.nav-item {
  display: flex;
  align-items: center;
  padding: 6px 12px;
  margin-bottom: 2px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  font-size: 14px;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    background-color: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background-color: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.nav-icon {
  margin-right: 9px;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: inherit;
}

.nav-icon-emoji {
  font-size: 14px;
  line-height: 1;
}

.nav-label {
  flex: 1;
}

.expand-icon {
  margin-left: 4px;
  font-size: 14px;
  transition: transform 0.2s ease;
}

/* 子菜单 */
.submenu {
  margin-left: 28px;
  margin-bottom: 3px;
  overflow: hidden;
}

.submenu-item {
  padding: 5px 12px;
  margin-bottom: 2px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  font-size: 13px;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    background-color: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background-color: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.submenu-label {
  display: block;
}

/* 子菜单动画 */
.submenu-enter-active,
.submenu-leave-active {
  transition: all 0.2s ease;
}

.submenu-enter-from {
  opacity: 0;
  max-height: 0;
}

.submenu-enter-to {
  opacity: 1;
  max-height: 300px;
}

.submenu-leave-from {
  opacity: 1;
  max-height: 300px;
}

.submenu-leave-to {
  opacity: 0;
  max-height: 0;
}

/* 右侧内容区域 */
.settings-content {
  flex: 1;
  overflow-y: auto;
  background-color: var(--td-bg-color-container);
}

.content-wrapper {
  // Bumped from 600 to 760 when the modal grew from 900→1080 (see
  // .settings-modal). Without this, single-column panes (General,
  // KnowledgeDomain, API key, …) leave a wide right-hand gutter inside the
  // wider modal. 760 keeps comfortable reading-width on long
  // descriptions without the form fields stretching to the full
  // panel width — which would look stranger than a small gutter.
  max-width: 760px;
  padding: 40px 48px;

  /* 成员 / 审计表格列多，600px 会把操作列挤到贴边；铺满右侧内容列更稳。 */
  &--wide {
    max-width: none;
    width: 100%;
    padding: 32px 36px 40px;
    box-sizing: border-box;
  }

  &--full {
    max-width: none;
    width: 100%;
    padding: 30px 34px 40px;
    box-sizing: border-box;
  }
}

.section {
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 弹窗动画 */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .settings-modal,
.modal-leave-active .settings-modal {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .settings-modal,
.modal-leave-to .settings-modal {
  transform: scale(0.95);
  opacity: 0;
}

/* 滚动条样式 */
.settings-nav::-webkit-scrollbar,
.settings-content::-webkit-scrollbar {
  width: 6px;
}

.settings-nav::-webkit-scrollbar-track {
  background: var(--td-bg-color-secondarycontainer);
}

.settings-nav::-webkit-scrollbar-thumb {
  background: var(--td-gray-color-5);
  border-radius: 3px;
}

.settings-nav::-webkit-scrollbar-thumb:hover {
  background: var(--td-gray-color-6);
}

.settings-content::-webkit-scrollbar-track {
  background: var(--td-bg-color-container);
}

.settings-content::-webkit-scrollbar-thumb {
  background: var(--td-gray-color-5);
  border-radius: 3px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background: var(--td-gray-color-6);
}

.role-denied {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 64px 24px;
  gap: 12px;
  min-height: 240px;

  .role-denied-icon {
    color: var(--td-text-color-placeholder);
  }

  .role-denied-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .role-denied-desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    max-width: 360px;
    line-height: 1.6;
  }
}
</style>
