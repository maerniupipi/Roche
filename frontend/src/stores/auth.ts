import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { UserInfo } from '@/api/auth'
import { userInfoFromApi } from '@/api/auth'
import { reloadFontFromStorage } from '@/composables/useFont'
import { reloadThemeFromStorage } from '@/composables/useTheme'
import { resetMigrationLatch } from '@/composables/preferenceStorage'
import { useChatResourcesStore } from '@/stores/chatResources'
import { useEditorResourcesStore } from '@/stores/editorResources'

function clearSessionResourceCaches() {
  useChatResourcesStore().invalidate()
  useEditorResourcesStore().invalidate()
}

function reloadUserPreferences() {
  resetMigrationLatch()
  reloadFontFromStorage()
  reloadThemeFromStorage()
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<UserInfo | null>(null)
  const token = ref('')
  const isLiteMode = ref(false)

  const isLoggedIn = computed(() => Boolean(token.value && user.value))
  const currentUserId = computed(() => user.value?.id || '')
  const isSystemAdmin = computed(() => user.value?.is_system_admin === true)
  const isKnowledgeDomainAdmin = computed(
    () => user.value?.is_knowledge_domain_admin === true,
  )
  const canManageKnowledge = computed(
    () => isSystemAdmin.value || isKnowledgeDomainAdmin.value,
  )

  const setUser = (userData: UserInfo) => {
    const previousID = user.value?.id
    user.value = userData
    localStorage.setItem('roche_kap_user', JSON.stringify(userData))
    if (previousID !== userData.id) reloadUserPreferences()

    if (userData.preferences) {
      import('@/stores/settings')
        .then(({ useSettingsStore }) => {
          useSettingsStore().hydrateFromUserPreferences(userData.preferences)
        })
        .catch(() => undefined)
    }
  }

  const setToken = (value: string) => {
    token.value = value
    localStorage.setItem('roche_kap_token', value)
  }

  const refreshFromAuthMe = async (): Promise<boolean> => {
    try {
      const { getCurrentUser } = await import('@/api/auth')
      const response = await getCurrentUser()
      if (!response.success || !response.data?.user) return false
      setUser(userInfoFromApi(response.data.user))
      return true
    } catch {
      return false
    }
  }

  const setLiteMode = (value: boolean) => {
    isLiteMode.value = value
    if (value) localStorage.setItem('roche_kap_lite_mode', 'true')
    else localStorage.removeItem('roche_kap_lite_mode')
  }

  const logout = () => {
    user.value = null
    token.value = ''
    isLiteMode.value = false
    clearSessionResourceCaches()

    for (const key of [
      'roche_kap_user',
      'roche_kap_token',
      'roche_kap_refresh_token',
      'roche_kap_lite_mode',
    ]) {
      localStorage.removeItem(key)
    }
    // Clean up legacy KB keys left over from previous builds.
    localStorage.removeItem('roche_kap_knowledge_bases')
    localStorage.removeItem('roche_kap_current_kb')
    sessionStorage.removeItem('roche_kap_lite_last_path')
    reloadUserPreferences()
  }

  const initFromStorage = () => {
    const storedUser = localStorage.getItem('roche_kap_user')
    const storedToken = localStorage.getItem('roche_kap_token')

    if (storedUser) {
      try {
        user.value = userInfoFromApi(JSON.parse(storedUser))
      } catch {
        localStorage.removeItem('roche_kap_user')
      }
    }
    token.value = storedToken || ''
    // Remove credentials left by pre-cookie builds. Refresh tokens are now
    // held only in an HttpOnly cookie and cannot be read by JavaScript.
    localStorage.removeItem('roche_kap_refresh_token')

    // Drop any legacy KB keys carried over from earlier sessions.
    localStorage.removeItem('roche_kap_knowledge_bases')
    localStorage.removeItem('roche_kap_current_kb')
    isLiteMode.value = localStorage.getItem('roche_kap_lite_mode') === 'true'
  }

  initFromStorage()

  return {
    user,
    token,
    isLiteMode,
    isLoggedIn,
    currentUserId,
    isSystemAdmin,
    isKnowledgeDomainAdmin,
    canManageKnowledge,
    setUser,
    setToken,
    refreshFromAuthMe,
    setLiteMode,
    logout,
    initFromStorage,
  }
})
