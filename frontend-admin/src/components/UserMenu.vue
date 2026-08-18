<template>
  <div ref="menuRef" class="user-menu" :class="{ collapsed: uiStore.sidebarCollapsed }">
    <button type="button" class="user-button" data-guide="user-menu" @click.stop="menuVisible = !menuVisible">
      <t-avatar :image="userAvatar" size="32px">{{ userInitial }}</t-avatar>
      <div v-if="!uiStore.sidebarCollapsed" class="user-summary">
        <strong>{{ userName }}</strong>
        <span>{{ userEmail }}</span>
      </div>
      <t-icon v-if="!uiStore.sidebarCollapsed" :name="menuVisible ? 'chevron-up' : 'chevron-down'" />
    </button>

    <Transition name="dropdown">
      <div v-if="menuVisible" class="user-dropdown" @click.stop>
        <!-- <div class="identity">
          <t-avatar :image="userAvatar" size="38px">{{ userInitial }}</t-avatar>
          <div>
            <strong>{{ userName }}</strong>
            <span>{{ userEmail }}</span>
          </div>
          <t-tag v-if="currentRole" size="small" variant="light">
            {{ currentRoleLabel }}
          </t-tag>
        </div> -->

        <button type="button" class="menu-item" @click="reopenGuide">
          <t-icon name="help-circle" />
          <span>{{ $t('newUserGuide.reopen') }}</span>
        </button>
        <button type="button" class="menu-item" @click="openSettings">
          <t-icon name="setting" />
          <span>{{ $t('general.allSettings') }}</span>
        </button>

        <div class="divider" />
        <button type="button" class="menu-item danger" @click="handleLogout">
          <t-icon name="logout" />
          <span>{{ $t('auth.logout') }}</span>
        </button>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { getCurrentUser, logout as logoutApi, userInfoFromApi } from '@/api/auth'
import { openNewUserGuide } from '@/config/contextualGuides'
import { useRoleLabel } from '@/composables/useRoleLabel'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'
import { isDebugger } from '@/composables/featureFlags';

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const uiStore = useUIStore()
const { formatRole } = useRoleLabel()

const menuRef = ref<HTMLElement>()
const menuVisible = ref(false)

const userName = computed(() => authStore.user?.username || t('common.defaultUser'))
const userEmail = computed(() => authStore.user?.email || '')
const userAvatar = computed(() => authStore.user?.avatar || '')
const userInitial = computed(() => userName.value.slice(0, 1).toUpperCase())
const currentRole = computed(() =>
  authStore.isSystemAdmin
    ? 'system_admin'
    : authStore.isKnowledgeDomainAdmin
      ? 'knowledge_domain_admin'
      : 'viewer',
)
const currentRoleLabel = computed(() => formatRole(currentRole.value))

function canSee(role: 'viewer' | 'admin'): boolean {
  return role === 'viewer' ? authStore.isLoggedIn : authStore.canManageKnowledge
}

function closeMenu(): void {
  menuVisible.value = false
}

function openSettings(): void {
  closeMenu()
  uiStore.openSettings()
  void router.push('/platform/settings')
}

function reopenGuide(): void {
  closeMenu()
  openNewUserGuide()
}

async function handleLogout(): Promise<void> {
  closeMenu()
  try {
    await logoutApi()
  } catch (error) {
    console.error('Logout API failed:', error)
  }
  authStore.logout()
  MessagePlugin.success(t('auth.logout'))
  await router.push('/login')
}

async function loadUser(): Promise<void> {
  try {
    const response = await getCurrentUser()
    if (!response.success || !response.data?.user) return
    authStore.setUser(userInfoFromApi(response.data.user))
  } catch (error) {
    console.error('Failed to load user info:', error)
  }
}

function handleClickOutside(event: MouseEvent): void {
  if (!menuRef.value?.contains(event.target as Node)) closeMenu()
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  void loadUser()
})

onUnmounted(() => document.removeEventListener('click', handleClickOutside))
</script>

<style scoped lang="less">
.user-menu {
  position: relative;
  width: 100%;
}

.user-button,
.menu-item {
  width: 100%;
  border: 0;
  background: transparent;
  color: var(--td-text-color-primary);
  cursor: pointer;
}

.user-button {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px;
  border-radius: 6px;
  text-align: left;
}

.user-summary {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;

  strong,
  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span {
    color: var(--td-text-color-secondary);
    font-size: 12px;
  }
}

.user-dropdown {
  position: absolute;
  z-index: 2000;
  right: 0;
  left: 0;
  bottom: calc(100% + 8px);
  width: auto;
  padding: 8px;
  box-sizing: border-box;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  box-shadow: var(--td-shadow-2);
}

.collapsed .user-dropdown {
  right: auto;
  bottom: 0;
  left: calc(100% + 8px);
  width: 280px;
}

.identity {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 8px 12px;

  div {
    display: flex;
    min-width: 0;
    // flex: 1;
    flex-direction: column;
  }

  span {
    overflow: hidden;
    color: var(--td-text-color-secondary);
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  :deep(.t-tag) {
    flex-shrink: 0;
  }
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 6px;
  text-align: left;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &.danger {
    color: var(--td-error-color);
  }
}

.divider {
  height: 1px;
  margin: 6px 0;
  background: var(--td-component-stroke);
}

.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
