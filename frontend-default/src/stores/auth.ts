import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { KnowledgeBaseInfo, UserInfo } from '@/api/auth'
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
  const knowledgeBases = ref<KnowledgeBaseInfo[]>([])
  const currentKnowledgeBase = ref<KnowledgeBaseInfo | null>(null)
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

  const setKnowledgeBases = (items: KnowledgeBaseInfo[]) => {
    knowledgeBases.value = Array.isArray(items) ? items : []
    localStorage.setItem('roche_kap_knowledge_bases', JSON.stringify(knowledgeBases.value))
  }

  const setCurrentKnowledgeBase = (knowledgeBase: KnowledgeBaseInfo | null) => {
    currentKnowledgeBase.value = knowledgeBase
    if (knowledgeBase) {
      localStorage.setItem('roche_kap_current_kb', JSON.stringify(knowledgeBase))
    } else {
      localStorage.removeItem('roche_kap_current_kb')
    }
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
    knowledgeBases.value = []
    currentKnowledgeBase.value = null
    isLiteMode.value = false
    clearSessionResourceCaches()

    for (const key of [
      'roche_kap_user',
      'roche_kap_token',
      'roche_kap_refresh_token',
      'roche_kap_knowledge_bases',
      'roche_kap_current_kb',
      'roche_kap_lite_mode',
    ]) {
      localStorage.removeItem(key)
    }
    sessionStorage.removeItem('roche_kap_lite_last_path')
    reloadUserPreferences()
  }

  const initFromStorage = () => {
    const storedUser = localStorage.getItem('roche_kap_user')
    const storedToken = localStorage.getItem('roche_kap_token')
    const storedKnowledgeBases = localStorage.getItem('roche_kap_knowledge_bases')
    const storedCurrentKnowledgeBase = localStorage.getItem('roche_kap_current_kb')

    if (storedUser) {
      try {
        user.value = userInfoFromApi(JSON.parse(storedUser))
      } catch {
        localStorage.removeItem('roche_kap_user')
      }
    }
    token.value = storedToken || ''
    // Refresh tokens are held only in the Auth Service HttpOnly cookie.
    localStorage.removeItem('roche_kap_refresh_token')

    if (storedKnowledgeBases) {
      try {
        const parsed = JSON.parse(storedKnowledgeBases)
        knowledgeBases.value = Array.isArray(parsed) ? parsed : []
      } catch {
        localStorage.removeItem('roche_kap_knowledge_bases')
      }
    }
    if (storedCurrentKnowledgeBase) {
      try {
        currentKnowledgeBase.value = JSON.parse(storedCurrentKnowledgeBase)
      } catch {
        localStorage.removeItem('roche_kap_current_kb')
      }
    }
    isLiteMode.value = localStorage.getItem('roche_kap_lite_mode') === 'true'
  }

  initFromStorage()

  return {
    user,
    token,
    knowledgeBases,
    currentKnowledgeBase,
    isLiteMode,
    isLoggedIn,
    currentUserId,
    isSystemAdmin,
    isKnowledgeDomainAdmin,
    canManageKnowledge,
    setUser,
    setToken,
    setKnowledgeBases,
    setCurrentKnowledgeBase,
    refreshFromAuthMe,
    setLiteMode,
    logout,
    initFromStorage,
  }
})
