<template>
  <div class="aside_box" :class="{ 'aside_box--collapsed': uiStore.sidebarCollapsed }">
    <!-- 展开时：Logo + 搜索/折叠按钮同行 -->
    <div class="logo_row" v-if="!uiStore.sidebarCollapsed">
      <div class="logo_box" @click="router.push(defaultPlatformPath)" style="cursor: pointer;">
        <div class="brand-lockup" aria-label="Roche Knowledge Agent Platform">
          <img src="@/assets/img/logo.svg" alt="Roche Logo">
          <!-- <span class="brand-name">ROCHE</span>
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


    <!-- 折叠时右侧拖拽展开手柄 -->
    <div v-if="uiStore.sidebarCollapsed" class="sidebar-drag-handle" @mousedown="onDragHandleMouseDown" />

    <!-- 上半部分：后端下发的菜单项（侧栏顶部） -->
    <div class="menu_top">
      <div class="menu_box" v-for="(item, index) in topMenuItems" :key="index">
        <t-tooltip :content="item.title" placement="right" :disabled="!uiStore.sidebarCollapsed">
          <div @click="handleMenuClick(item)" :data-guide="`nav-${item.path || item.id}`"
            :class="['menu_item', isMenuItemActive(item.path || item.id) ? 'menu_item_active' : '']">
            <div class="menu_item-box">
              <div class="menu_icon">
                <img class="icon" :src="getImgSrc(item.iconName)" alt="">
              </div>
              <template v-if="!uiStore.sidebarCollapsed">
                <span class="menu_title" :title="item.title">{{ item.title }}</span>
                <!-- 知识库项右侧的 chevron：仅展开 + 菜单 store 有 children 时显示。
                     识别谓词同时看 id（mock 中 KB 父节点没 path）和 path（真实数据可能填 path），避免单边错位。 -->
                <span v-if="isKbMenuItem(item) && (item.children?.length ?? 0) > 0"
                  :class="['kb-menu-chevron', knowledgeBasesExpanded ? 'rotate-180' : '']"
                  :aria-expanded="knowledgeBasesExpanded" role="button" @click.stop="toggleKnowledgeBasesExpanded">
                  <t-icon name="chevron-down" size="14px" />
                </span>
              </template>
            </div>
          </div>
        </t-tooltip>

        <!-- 知识库二级菜单：菜单 store 直接下发 children（含 path / 标题）。
             不再依赖 useKnowledgeDomains composable，避免后端不可达时子菜单消失。 -->
        <ul
          v-if="isKbMenuItem(item) && knowledgeBasesExpanded && !uiStore.sidebarCollapsed && (item.children?.length ?? 0) > 0"
          class="kb-submenu">
          <li v-for="child in item.children" :key="child.id">
            <router-link :to="child.path || ''"
              :class="['kb-submenu-item', isKbDomainActive(child.path || '') ? 'kb-submenu-item-active' : '']"
              :title="usemenuStore.displayTitle(child)">
              <span class="kb-submenu-text">{{ usemenuStore.displayTitle(child) }}</span>
            </router-link>
          </li>
        </ul>
      </div>
    </div>
    <div class="menu_bottom" />
    <!-- <UserMenu /> -->
    <PlatformHeader />
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { onMounted, onBeforeUnmount, watch, computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { logout as logoutApi } from '@/api/auth';
import { useMenuStore } from '@/stores/menu';
import type { MenuNode } from '@/types/menu';
import { useAuthStore } from '@/stores/auth';
import { useUIStore } from '@/stores/ui';
import { useCommandPaletteStore } from '@/stores/commandPalette';
import { MessagePlugin } from "tdesign-vue-next";
import { useI18n } from 'vue-i18n';
import UserMenu from '@/components/UserMenu.vue'
import PlatformHeader from '@/components/PlatformHeader.vue'

const { t } = useI18n();
const usemenuStore = useMenuStore();
const authStore = useAuthStore();
const uiStore = useUIStore();
const commandPaletteStore = useCommandPaletteStore();

// Platform-aware label for the ⌘K hint. navigator.platform is deprecated but
// the alternatives (userAgentData.platform) aren't universally available yet;
// this check is good enough for Mac vs. non-Mac.
const isMacLike = typeof navigator !== 'undefined' && /Mac|iPod|iPhone|iPad/.test(navigator.platform || '');
const cmdModKeyLabel = isMacLike ? '⌘' : 'Ctrl';
const route = useRoute();
const router = useRouter();

// 侧栏展示用 MenuItem（与 menu store 的 MenuNode 保持平移）。
// children 透传菜单 store 的子节点（KB 一级项会带 4 个 domain children）。
type MenuItem = {
  title: string
  icon: string
  iconName: string
  path: string
  id?: string
  children?: MenuNode[]
}

// 识别"知识库"一级菜单项。
const isKbMenuItem = (item: MenuItem): boolean => {
  if (item.id === 'knowledge-bases' || item.id === 'knowledge') return true
  const path = item.path || ''
  return path === 'knowledge-bases' || path.startsWith('/platform/knowledge-bases')
}

const { topLevelNodes } = storeToRefs(usemenuStore);

// 是否可以管理所有知识域
const canManageKnowledge = computed(() => authStore.canManageKnowledge);

const defaultPlatformPath = computed(() => {
  // 优先用菜单 store 第一项的 path（按角色下发）
  const first = usemenuStore.firstNavigableNode
  if (first?.path) return first.path
  // 菜单 store 还没加载好（首次访问 / 异步加载未完成）→ 兜底到 dashboard，
  // 保持和 router 的 defaultAuthenticatedPath fallback 一致。
  return '/platform/dashboard'
})

// 知识库二级菜单展开状态。
// - 进入 platform 时默认收起，首次点击展开。
// - 当前路由属于 knowledgeBaseList / knowledgeBaseListByDomain 时
//   默认展开（让用户直接看到自己在哪个知识域）。
const knowledgeBasesExpanded = ref(false);

// KB 一级菜单涉及的 vue-router 路由名：侧栏激活态判定共用。
const KB_ROUTE_NAMES = [
  'knowledgeBaseList',
  'knowledgeBaseListByDomain',
  'knowledgeBaseDetail',
  'knowledgeBaseSettings',
] as const

// 当前 vue-router 是否处于 KB 路由（任一 KB 路由名）。
const isKbRouteActive = (): boolean => {
  const name = route.name
  return typeof name === 'string' && (KB_ROUTE_NAMES as readonly string[]).includes(name)
}

// 识别一个菜单路径是否属于 KB 一级菜单（用于激活态判定）。
// 与 isKbMenuItem 互为镜像：
//   isKbMenuItem    接"节点对象"，判定这是不是 KB 父节点（用于模板 v-if）
//   isKbActivePath  接"路径字符串"，判定这条路径在激活态判定里是否走 KB 路由名匹配
// 兼容多种下发形态：'knowledge-bases' / 'knowledge' / '/platform/knowledge-bases' / '/platform/knowledge-bases/xxx'。
const isKbActivePath = (itemPath: unknown): boolean => {
  if (typeof itemPath !== 'string') return false
  return itemPath === 'knowledge-bases' ||
    itemPath === 'knowledge' ||
    itemPath === '/platform/knowledge-bases' ||
    itemPath.startsWith('/platform/knowledge-bases/')
}

// 统一的菜单项激活状态判断。
// 菜单 store 下发的 path 是完整路径（如 '/platform/dashboard'），
// 默认分支直接跟 route.path 完全匹配；KB / settings 走路由名匹配。
const isMenuItemActive = (itemPath: any): boolean => {
  const currentPath = route.path
  if (isKbActivePath(itemPath)) return isKbRouteActive()
  if (itemPath === 'settings') return route.name === 'settings'
  return currentPath === itemPath
};

// KB 详情页通过 CustomEvent 'active-kb-domain-changed' 把当前 KB 的 domainId 派发给侧栏。
// 详情页路由（/platform/knowledge-bases/:kbId）下 route.path 不会匹配侧栏 child.path，
// 因此侧栏需要这块共享状态做激活态判定；离开详情页时由下面的 watch 清空。
const activeKbDomainId = ref<number | null>(null)

const parseKbDomainIdFromPath = (p: string | undefined | null): number | null => {
  if (!p) return null
  const m = /^\/platform\/knowledge-bases\/domain\/(\d+)/.exec(p)
  return m ? Number(m[1]) : null
}

const handleActiveKbDomainChanged = (event: Event) => {
  const detail = (event as CustomEvent<{ domainId: number | null }>).detail
  activeKbDomainId.value = detail?.domainId ?? null
}

// 判断某个具体知识域子项是否处于激活态（用于子菜单高亮）。
// - 列表子路由：child.path 与 route.path 都用 parseKbDomainIdFromPath 抽出 numeric id 后比对，
//   比字符串完全匹配更鲁棒（容忍后端路径格式细节差异）。
// - KB 详情页：路由没有 domain 段，回退到 activeKbDomainId 判定。
const isKbDomainActive = (childPath: string): boolean => {
  const childDomainId = parseKbDomainIdFromPath(childPath)
  if (childDomainId == null) return false
  const listMatch = parseKbDomainIdFromPath(route.path)
  if (listMatch != null) return listMatch === childDomainId
  return activeKbDomainId.value === childDomainId
}

// 统一的图标激活状态判断
const getIconActiveState = (itemPath: string) => {
  return {
    isKbActive: isKbActivePath(itemPath) && isKbRouteActive(),
    isSettingsActive: itemPath === 'settings' && route.name === 'settings',
  };
};

// 侧栏顶部要渲染的菜单项：
// 取后端下发的全量 topLevelNodes，并把 settings / logout 过滤掉（settings
// 走 UserMenu 弹窗，logout 走 UserMenu 里的退出按钮）。
const topMenuItems = computed<MenuItem[]>(() => {
  const nodes = topLevelNodes.value ?? []
  return nodes
    .filter((node) => {
      const path = node.path || ''
      return path !== 'settings' && path !== 'logout'
    })
    .map((node) => {
      // 激活态 key：path 优先于 id。
      // - 非 KB 菜单节点都有 path（mock: '/platform/dashboard' 等），传给 default 分支
      //   做 currentPath === itemPath 比对，能命中。
      // - KB 父节点在 mock 里 path 为空、id 为 'knowledge-bases'，会落到 switch case
      //   用 route.name 匹配，仍然高亮。
      // 历史实现是 id || path，导致 dashboard/recommend-questions/roles 等的 id
      // （如 'dashboard'）被传进 default 分支，跟 route.path('/platform/dashboard') 永远不等，
      // 这些一级菜单永远不高亮。改为 path || id 修复该 bug。
      const isActive = isMenuItemActive(node.path || node.id || '')
      // icon key（如 "dashboard" / "knowledge-bases"）映射为本地 svg 文件名；
      // 激活时使用 "-active" 高亮版本，缺图时降级到 prefixIcon。
      const baseIcon = node.icon || 'prefixIcon'
      const fileName = isActive ? `${baseIcon}-active.svg` : `${baseIcon}.svg`
      return {
        title: usemenuStore.displayTitle(node),
        icon: baseIcon,
        iconName: fileName,
        path: node.path || '',
        id: node.id,
        children: node.children,   // 透传给模板渲染二级菜单
      }
    })
});

// 处理知识域列表刷新事件（来自 KnowledgeDomainCreateDialog 等）。
// 改为强制 reload 菜单 store，因为二级菜单现在直接用 store 的 children。
const onKnowledgeDomainChanged = () => {
  usemenuStore.loadMenu(true)
}

onMounted(() => {
  // 进入 KB 相关路由时默认展开二级菜单；其他位置默认收起。
  if (isMenuItemActive('knowledge-bases')) {
    knowledgeBasesExpanded.value = true
  }

  // 加载菜单树（fire-and-forget；loadMenu 内部有 loaded 防重，多处触发是幂等的）。
  // router 的 defaultAuthenticatedPath 会读 firstNavigableNode 来决定默认页；
  // 这里也要触发一次，否则用户登录后侧栏永远是空的。
  // 二级菜单的渲染数据直接来自 store.children，不需要再单独加载知识域。
  usemenuStore.loadMenu()
  window.addEventListener('knowledge-domain-changed', onKnowledgeDomainChanged)
  window.addEventListener('active-kb-domain-changed', handleActiveKbDomainChanged as EventListener)
});

onBeforeUnmount(() => {
  window.removeEventListener('knowledge-domain-changed', onKnowledgeDomainChanged)
  window.removeEventListener('active-kb-domain-changed', handleActiveKbDomainChanged as EventListener)
})

watch([() => route.name], () => {
  // 进入 KB 相关路由时自动展开二级菜单。
  if (isMenuItemActive('knowledge-bases') && !knowledgeBasesExpanded.value) {
  // 路由变化时清理上一个详情页留下的 activeKbDomainId：
  // 只在 KB 详情路由期间保留值，离开详情页/列表页时主动重置为 null，
  // 避免从 KB 详情跳到非 KB 页面后仍误激活对应 domain 子菜单。
  if (route.name !== 'knowledgeBaseDetail') {
    activeKbDomainId.value = null
  }
    knowledgeBasesExpanded.value = true
  }

  // 路由变化时更新图标状态
  getIcon(typeof route.name === 'string' ? route.name as string : (route.name ? String(route.name) : ''));
});
let knowledgeIcon = ref('zhishiku-active.svg');
let logoutIcon = ref('logout.svg');
let settingIcon = ref('setting.svg');
const getIcon = (path: string) => {
  // 根据当前路由状态更新所有图标
  const kbActiveState = getIconActiveState('knowledge-bases');
  const settingsActiveState = getIconActiveState('settings');

  // 知识库图标：只在知识库页面显示蓝色
  knowledgeIcon.value = kbActiveState.isKbActive ? 'zhishiku-active.svg' : 'zhishiku.svg';

  // 设置图标：只在设置页面显示蓝色
  settingIcon.value = settingsActiveState.isSettingsActive ? 'setting-active.svg' : 'setting.svg';

  // 退出图标：始终显示默认
  logoutIcon.value = 'logout.svg';
}
getIcon(typeof route.name === 'string' ? route.name as string : (route.name ? String(route.name) : ''))

const handleMenuClick = async (item: MenuItem) => {
  const path = item.path
  if (isKbMenuItem(item)) {
    // KB 一级项：展开二级菜单 + 跳第一个子项。
    // 子项数据从 item.children 取（菜单 store 已下发），不再依赖 knowledgeDomains composable。
    knowledgeBasesExpanded.value = true
    const first = item.children?.[0]
    if (first?.path) {
      await router.push(first.path)
    }
    return
  }
  // settings / dashboard / recommend-questions / answer-records / roles / exchange-rate
  // 等都通过 gotopage(path) 走 router.push。
  // 详细路径以菜单 store 下发为准。
  gotopage(path)
}

// 切换知识域子菜单展开状态（chevron 自身点击）
const toggleKnowledgeBasesExpanded = () => {
  knowledgeBasesExpanded.value = !knowledgeBasesExpanded.value
}

// 处理退出登录确认
const handleLogout = () => {
  gotopage('logout')
}

const gotopage = async (path: string) => {
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
  }
  // 兼容两种来源：
  //   - 菜单 store 下发的完整路径，如 '/platform/dashboard'
  //     （后端 GET /api/v1/menu 下发的完整路径）
  //   - 老菜单可能存子路径，如 'dashboard'
  // 拼前缀前先判断，避免重复变成 '/platform//platform/exchange-rate'。
  const target = path.startsWith('/platform/') ? path : `/platform/${path}`
  router.push(target);
  getIcon(path)
}

const getImgSrc = (url: string) => {
  return new URL(`/src/assets/img/${url}`, import.meta.url).href;
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
  transition: width 0.25s ease;
  min-width: 0.25s ease;
  position: relative;

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
      flex-direction: column;
      min-width: 0;
      color: var(--td-text-color-primary);

      .brand-name {
        font-size: 21px;
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
    background: var(--menu-bg-active) !important;

    .menu_icon,
    .menu_title {
      color: var(--td-brand-color) !important;
    }
  }

  .menu_box:has(.kb-submenu-item-active) .menu_item_active {
    background: transparent !important;
  }

  .menu_item_c_active {

    .menu_icon,
    .menu_title {
      color: var(--td-text-color-primary);
    }
  }

  .menu_item {
    display: flex;
    align-items: center;
    height: 40px;
    margin: 0 0 4px 0;
    padding: 0 10px;
    cursor: pointer;
    user-select: none;
    color: var(--td-text-color-primary);

    .menu_item-box {
      display: flex;
      align-items: center;
      width: 100%;
      gap: 10px;
      overflow: hidden;
    }

    .menu_icon {
      width: var(--sidebar-icon-size);
      height: var(--sidebar-icon-size);
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: center;

      .icon {
        width: 18px;
        height: 18px;
      }
    }

    .menu_title {
      flex: 1;
      min-width: 0;
      font-size: 14px;
      line-height: 20px;
      color: var(--td-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .menu_item:hover:not(.menu_item_active) {
    background: var(--menu-bg-hover);
  }
}

.new_chat {
  display: flex;
  align-items: center;
  height: 40px;
  margin: 0 0 4px 0;
  padding: 0 10px;
  cursor: pointer;
  user-select: none;
  border-radius: 4px;

  .menu_item-box {
    display: flex;
    align-items: center;
    width: 100%;
    gap: 10px;
    overflow: hidden;
  }

  .menu_icon {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;

    img {
      width: 18px;
      height: 18px;
    }
  }

  .menu_title {
    flex: 1;
    font-size: 14px;
    color: var(--td-text-color-primary);
  }

  &:hover {
    background: var(--menu-bg-hover);
  }
}

.upload-file-btn {
  background: var(--td-brand-color-light);
  border-radius: 4px;

  &:hover {
    background: var(--td-brand-color-light-hover, var(--td-brand-color-light));
    color: var(--td-brand-color);
  }
}

.upload-file-area {
  margin: 8px 10px 16px;
  padding: 12px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  text-align: center;
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
    background: var(--td-bg-color-container-hover);
  }
}

.session-group-by-wrap {
  margin: 0 10px 4px;
}

.session-group-by-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.session-item {
  display: flex;
  align-items: center;
  height: 32px;
  margin: 0 0 2px 0;
  padding: 0 8px 0 10px;
  border-radius: 4px;
  cursor: pointer;
  user-select: none;

  .session-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 13px;
    color: var(--td-text-color-primary);
  }

  .session-actions {
    flex-shrink: 0;
    opacity: 0;
    transition: opacity 0.15s ease;
  }

  &:hover .session-actions {
    opacity: 1;
  }
}

.session-item:hover {
  background: var(--menu-bg-hover);
}

.session-item_active {
  background: var(--menu-bg-active);

  .session-title {
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.empty-sessions {
  padding: 12px 10px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  text-align: center;
}

.user-menu-wrap {
  position: relative;
  padding: 6px 0;
}

.user-menu-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  margin: 0 4px;
  padding: 0 10px;
  border-radius: 4px;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.15s ease;

  &:hover {
    background: var(--menu-bg-hover);
  }
}

.user-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
}

.user-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--td-text-color-primary);
}

.badge-dot {
  width: 18px;
  height: 18px;
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

// 知识库菜单项右侧的 chevron —— 展开时旋转 180°。
// 与 menu_item-box 的 flex 布局协作，靠右对齐；hover 高亮以提示可点击。
.kb-menu-chevron {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 4px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: transform 0.2s ease, background-color 0.2s ease, color 0.2s ease;

  &.rotate-180 {
    transform: rotate(180deg);
  }

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }
}

// 知识域二级菜单（侧栏折叠时不显示，整体由父 v-if 守卫）。
.kb-submenu {
  list-style: none;
  margin: 2px 0 4px;
  padding: 0;

}

.kb-submenu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 10px 0 14px;
  margin: 4px 0;
  // 视觉上缩进：与父菜单的图标对齐点 = 14 + 18/2 ≈ 23，取 26 让小圆点更清晰
  padding-left: calc(var(--sidebar-text-inset));
  border-radius: 4px;
  font-size: 13px;
  line-height: 20px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  user-select: none;
  text-decoration: none;
  transition: background-color 0.15s ease, color 0.15s ease;

  &:hover {
    background: var(--menu-bg-hover);
    color: var(--td-text-color-primary);
  }

  &-active {
    background: var(--menu-bg-active);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.kb-submenu-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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