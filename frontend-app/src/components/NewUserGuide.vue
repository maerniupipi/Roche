<template>
  <SpotlightGuide v-model:active="active" :steps="steps" step-i18n-prefix="newUserGuide.steps"
    labels-prefix="newUserGuide" @finish="onFinish" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import SpotlightGuide from '@/components/SpotlightGuide.vue'
import { GLOBAL_USER_GUIDE_KEY, OPEN_NEW_USER_GUIDE_EVENT } from '@/config/contextualGuides'
import { useAuthStore } from '@/stores/auth'
import { useMobileMenu } from '@/composables/useMobileMenu'
import type { SpotlightGuideStep } from '@/types/spotlightGuide'

const authStore = useAuthStore()
const mobileMenu = useMobileMenu()

const steps = computed<SpotlightGuideStep[]>(() => [
  { key: 'welcome' },
  {
    key: 'chat',
    target: '[data-guide="nav-creatChat"]',
    placement: 'top',
    before: () => mobileMenu.open(),
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
  window.addEventListener(OPEN_NEW_USER_GUIDE_EVENT, handleOpenEvent)
  if (!authStore.isLoggedIn) return
  if (localStorage.getItem(GLOBAL_USER_GUIDE_KEY) !== '1') {
    window.setTimeout(() => {
      if (localStorage.getItem(GLOBAL_USER_GUIDE_KEY) !== '1' && authStore.isLoggedIn) {
        open()
      }
    }, 700)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener(OPEN_NEW_USER_GUIDE_EVENT, handleOpenEvent)
})
</script>
