<template>
  <div class="mobile-app-shell" :class="{ 'mobile-app-shell--chat-bg': isCreatChat }">
    <MobileNavbar />
    <main class="mobile-app-shell__main">
      <RouterView />
    </main>
    <MobileDrawer />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import MobileNavbar from '@/components/mobile/MobileNavbar.vue'
import MobileDrawer from '@/components/mobile/MobileDrawer.vue'

const route = useRoute()
// 仅在新建会话页（creatChat）展示顶部装饰背景 —— 与原桌面行为保持一致
const isCreatChat = computed(() => route.path === '/platform/creatChat')
</script>

<style scoped lang="less">
.mobile-app-shell {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container);
  padding: 0;
  overflow: hidden;

  // 仅在新建会话页（creatChat）展示顶部装饰背景 —— 与原桌面行为保持一致
  &--chat-bg {
    background: var(--td-bg-color-container) url('@/assets/img/chatBg.png') no-repeat top center;
    background-size: 100% auto;
  }
}

.mobile-app-shell__main {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

// 路由切换淡入
.mobile-fade-enter-active,
.mobile-fade-leave-active {
  transition: opacity 0.18s ease;
}

.mobile-fade-enter-from,
.mobile-fade-leave-to {
  opacity: 0;
}
</style>