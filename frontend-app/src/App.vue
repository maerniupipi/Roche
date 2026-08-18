<script setup lang="ts">
import { computed, nextTick, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-mobile-vue'
import { useAuthStore } from '@/stores/auth'
import { getCurrentUser, userInfoFromApi } from '@/api/auth'
import { notifyLoginSuccess } from '@/utils/loginNotify'
import {
  clearSSOCallbackFragment,
  decodeSSOResult,
  readSSOCallback,
} from '@/utils/sso'

import enUSConfig from 'tdesign-mobile-vue/es/locale/en_US'
import zhCNConfig from 'tdesign-mobile-vue/es/locale/zh_CN'
import NewUserGuide from '@/components/NewUserGuide.vue'
import SecurityWatermark from '@/components/SecurityWatermark.vue'


const { locale, t, tm } = useI18n()
const router = useRouter()
const authStore = useAuthStore()

const tdLocaleMap: Record<string, object> = {
  'en-US': enUSConfig,
  'zh-CN': zhCNConfig,
}
const tdGlobalConfig = computed(() => tdLocaleMap[locale.value] || enUSConfig)

const syncSSOUser = async () => {
  const response = await getCurrentUser()
  if (!response.success || !response.data?.user) {
    throw new Error(response.message || 'Failed to get user information')
  }
  authStore.setUser(userInfoFromApi(response.data.user))
}

const persistSSOLoginResponse = async (response: any) => {
  if (!response.token) throw new Error(response.message || t('auth.samlLoginFailed'))

  authStore.setToken(response.token)
  await syncSSOUser()
  await nextTick()
  await router.replace('/platform/creatChat')
}

const handleGlobalSSOCallback = async () => {
  const callback = readSSOCallback()
  if (!callback) return

  if (callback.error) {
    clearSSOCallbackFragment()
    await router.replace('/login')
    MessagePlugin.error(callback.errorDescription || t('auth.samlLoginFailed'))
    return
  }

  try {
    if (!callback.result) throw new Error(t('auth.samlLoginFailed'))
    const response = decodeSSOResult(callback.result)
    if (!response.success) throw new Error(response.message || t('auth.samlLoginFailed'))

    clearSSOCallbackFragment()
    await persistSSOLoginResponse(response)
    notifyLoginSuccess(response, t, tm)
  } catch (error: any) {
    console.error('Global SAML callback handling failed:', error)
    authStore.logout()
    clearSSOCallbackFragment()
    await router.replace('/login')
    MessagePlugin.error(error.message || t('auth.samlLoginFailed'))
  }
}

onMounted(handleGlobalSSOCallback)
</script>

<template>
  <t-config-provider :globalConfig="tdGlobalConfig">
    <div id="app">
      <RouterView />
      <NewUserGuide />
      <SecurityWatermark />
    </div>
  </t-config-provider>
</template>

<style lang="less" scoped>
html {
  color-scheme: light dark;
}

body,
html,
#app {
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  font-size: 14px;
  font-family: var(--app-font-family);
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  background: var(--td-bg-color-page);
  color: var(--td-text-color-primary);
  box-sizing: border-box;
}

#app {
  * {
    box-sizing: border-box;
    -webkit-tap-highlight-color: transparent;
  }
}
</style>
