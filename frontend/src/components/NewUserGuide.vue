<template>
  <SpotlightGuide v-model:active="active" :steps="steps" step-i18n-prefix="newUserGuide.steps"
    labels-prefix="newUserGuide" @finish="onFinish" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import SpotlightGuide from '@/components/SpotlightGuide.vue'
import { GLOBAL_USER_GUIDE_KEY, OPEN_NEW_USER_GUIDE_EVENT } from '@/config/contextualGuides'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'
import type { SpotlightGuideStep } from '@/types/spotlightGuide'

const uiStore = useUIStore()
const authStore = useAuthStore()

const steps = computed<SpotlightGuideStep[]>(() => [
  { key: 'welcome' },
  {
    key: 'chat',
    target: '[data-guide="nav-creatChat"]',
    placement: 'right',
    before: () => uiStore.expandSidebar(),
  },
  {
    key: 'settings',
    target: '[data-guide="user-menu"]',
    placement: 'right',
    before: () => uiStore.expandSidebar(),
  },
  { key: 'done' },
])

const active = ref(false)

const onFinish = () => {
  localStorage.setItem(GLOBAL_USER_GUIDE_KEY, '1')
}

const open = () => {
  active.value = true
}

const handleOpenEvent = () => {
  if (!authStore.isLoggedIn) return
  if (active.value) return
  open()
}

onMounted(() => {
  // window.addEventListener(OPEN_NEW_USER_GUIDE_EVENT, handleOpenEvent)
  // if (!authStore.isLoggedIn) return
  // if (localStorage.getItem(GLOBAL_USER_GUIDE_KEY) !== '1') {
  //   window.setTimeout(() => {
  //     if (localStorage.getItem(GLOBAL_USER_GUIDE_KEY) !== '1' && authStore.isLoggedIn) {
  //       open()
  //     }
  //   }, 700)
  // }
  // 暂时禁用自动弹出 — 仅保留手动入口（UserMenu 中的"新手引导"按钮触发）

})

onBeforeUnmount(() => {
  window.removeEventListener(OPEN_NEW_USER_GUIDE_EVENT, handleOpenEvent)
})
</script>
