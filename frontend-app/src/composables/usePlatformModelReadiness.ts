import { computed, onMounted, ref, watch } from 'vue'
import { useUIStore } from '@/stores/ui'
import { useChatResourcesStore } from '@/stores/chatResources'
import {
  evaluatePlatformModelReadiness,
  type PlatformModelReadiness,
} from '@/utils/platformModelReadiness'

export function usePlatformModelReadiness() {
  const uiStore = useUIStore()
  const chatResources = useChatResourcesStore()
  const readiness = ref<PlatformModelReadiness | null>(null)
  const loaded = ref(false)
  const loading = ref(false)

  const refresh = async (force = false) => {
    loading.value = true
    try {
      await chatResources.ensureModels(force)
      readiness.value = evaluatePlatformModelReadiness(chatResources.allModels)
    } finally {
      loading.value = false
      loaded.value = true
    }
  }

  onMounted(() => void refresh())
  watch(
    () => uiStore.showSettingsModal,
    (open, wasOpen) => {
      if (wasOpen && !open) void refresh(true)
    },
  )

  return {
    readiness,
    loaded,
    loading,
    refresh,
    isReadyForDocumentKb: computed(
      () => readiness.value?.isReadyForDocumentKb ?? false,
    ),
    isReadyForAgent: computed(() => readiness.value?.isReadyForAgent ?? false),
    hasChat: computed(() => readiness.value?.hasChat ?? false),
  }
}
