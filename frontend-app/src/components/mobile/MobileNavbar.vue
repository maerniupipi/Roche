<template>
  <t-navbar class="mobile-navbar" :fixed="false" :safe-area-inset-top="true" :visible="true">
    <!-- 左：Drawer 触发按钮 -->
    <template #left>
      <div type="button" class="mobile-navbar__btn" :title="t('menu.openSidebar')" :aria-label="t('menu.openSidebar')"
        @click="handleOpenDrawer">
        <t-icon name="menu" size="22px" />
      </div>
    </template>
    <!-- 中：Logo SVG · v6 路由条件渲染（仅 /platform/creatChat 展示） -->
    <template #title>
      <div v-if="showCenterLogo" class="mobile-navbar__center">
        <img :src="logoUrl" class="mobile-navbar__logo" :alt="t('platform.title')" @click="handleLogoClick" />
      </div>
    </template>

    <!-- 右：新对话按钮 -->
    <template #right>
      <div type="button" class="mobile-navbar__btn" :title="t('menu.newChat')" :aria-label="t('menu.newChat')"
        @click="handleNewChat">
        <t-icon name="add" size="22px" />
      </div>
    </template>
  </t-navbar>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import logoUrl from '@/assets/img/logo.svg'
import { useMobileMenu } from '@/composables/useMobileMenu'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const mobileMenu = useMobileMenu()

// v6 路由条件渲染：仅 /platform/creatChat 展示中间 Logo
// /platform/chat/* 历史对话页不展示 Logo（中间区域留空）
const showCenterLogo = computed(() => route.path === '/platform/creatChat')

function handleOpenDrawer(): void {
  mobileMenu.open()
}

function handleNewChat(): void {
  // 跳到新建对话页；creatChat.vue 内部已处理「清空当前对话」语义
  void router.push('/platform/creatChat')
}

function handleLogoClick(): void {
  // Logo 点击回新建对话首页（与原 menu.vue .logo_box 行为一致）
  void router.push('/platform/creatChat')
}
</script>

<style scoped lang="less">
.mobile-navbar:deep {
  .t-navbar__content {
    background: transparent;
  }

  .t-navbar__center {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 44px;
  }

  .t-navbar__title {
    padding: 0;
  }
}

.mobile-navbar__center {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 23px;
  pointer-events: auto;
}

.mobile-navbar__logo {
  height: 100%;
  width: auto;
  cursor: pointer;
  user-select: none;
  -webkit-user-drag: none;
}

.mobile-navbar__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  margin: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-primary);
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
  transition: background-color 0.15s ease;
}
</style>