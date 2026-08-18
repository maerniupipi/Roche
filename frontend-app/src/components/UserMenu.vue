<template>
  <div class="user-menu">
    <!-- 左侧：展示型头像 + 用户信息（点击不响应，纯展示） -->
    <div class="user-info" data-guide="user-menu">
      <t-avatar :image="userAvatar" size="40px">{{ userInitial }}</t-avatar>
      <div class="user-summary">
        <strong>{{ userName }}</strong>
        <span>{{ userEmail }}</span>
      </div>
    </div>

    <!-- 右侧：独立的「?」问号按钮 = NewUserGuide 入口 -->
    <button
      type="button"
      class="user-help-button"
      data-guide="user-help"
      :aria-label="t('newUserGuide.reopen')"
      @click.stop="reopenGuide"
    >
      <t-icon name="help-circle" size="22px" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { getCurrentUser, userInfoFromApi } from '@/api/auth'
import { openNewUserGuide } from '@/config/contextualGuides'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const authStore = useAuthStore()

const userName = computed(() => authStore.user?.username || t('common.defaultUser'))
const userEmail = computed(() => authStore.user?.email || '')
const userAvatar = computed(() => authStore.user?.avatar || '')
const userInitial = computed(() => userName.value.slice(0, 1).toUpperCase())

function reopenGuide(): void {
  openNewUserGuide()
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

onMounted(() => {
  void loadUser()
})
</script>

<style scoped lang="less">
.user-menu {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 12px 16px;
  gap: 12px;
  box-sizing: border-box;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;          /* 允许 ellipsis 收缩 */
  background: transparent;
  border: 0;
  padding: 0;
  text-align: left;
  cursor: default;        /* 纯展示型，点击不响应 */
}

.user-summary {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;

  strong,
  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 14px;
    line-height: 1.3;
  }

  strong {
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  span {
    color: var(--td-text-color-secondary);
    font-size: 12px;
  }
}

.user-help-button {
  flex-shrink: 0;
  width: 44px;            /* 触摸目标 ≥44px */
  height: 44px;
  border-radius: 50%;
  border: 0;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.15s ease;

  &:active {
    background: rgba(0, 0, 0, 0.06);
  }
}
</style>
