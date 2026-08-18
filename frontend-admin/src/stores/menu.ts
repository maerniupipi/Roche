// 菜单 store：菜单完全由后端下发（GET /api/v1/menu，按角色返回），前端仅做展示与导航。
// 保留首屏状态（isFirstSession / firstQuery / prefillQuery 等），chat 模块仍依赖。
//

import { ref, computed, watch } from 'vue'
import { defineStore } from 'pinia'
import i18n from '@/i18n'
import { fetchMenu } from '@/api/menu'
import type { MenuNode } from '@/types/menu'

export const useMenuStore = defineStore('menuStore', () => {
  // 完整菜单树（按 order 排序前）
  const tree = ref<MenuNode[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const loaded = ref(false)

  // 首屏状态（chat 模块依赖，保留）
  const isFirstSession = ref(false)
  const firstQuery = ref('')
  const firstMentionedItems = ref<any[]>([])
  const firstModelId = ref('')
  const firstImageFiles = ref<any[]>([])
  const firstAttachmentFiles = ref<any[]>([])
  const prefillQuery = ref('')

  /**
   * 按当前 locale 选择要展示的标题。
   * 设计为 computed method（而非 getter）以兼容 store getter mock 场景。
   */
  const displayTitle = (node: MenuNode): string => {
    const isEn = i18n.global.locale.value === 'en-US'
    return isEn && node.titleEn ? node.titleEn : (node.title || node.titleEn || '')
  }

  /**
   * 拉取当前用户的菜单树，并把 visible=true 的节点按 order 排序。
   * 失败时降级为空数组（保留旧返回，便于前端 header 兜底）。
   */
  const loadMenu = async (force = false): Promise<void> => {
    if (loading.value && !force) return
    if (loaded.value && !force) return
    loading.value = true
    error.value = null
    try {
      const nodes = await fetchMenu()
      tree.value = nodes
        .filter((n) => n.visible)
        .map((n) => ({
          ...n,
          children: (n.children ?? [])
            .filter((c) => c.visible)
            .slice()
            .sort((a, b) => a.order - b.order),
        }))
        .sort((a, b) => a.order - b.order)
      loaded.value = true
    } catch (e: any) {
      error.value = e?.message || 'Failed to load menu'
    } finally {
      loading.value = false
    }
  }

  const reset = () => {
    tree.value = []
    loaded.value = false
    loading.value = false
    error.value = null
  }

  /** 一级菜单（侧栏直接展示的项）。 */
  const topLevelNodes = computed(() => tree.value)

  /** 找第一个有 path 的一级节点，用于登录后默认重定向。 */
  const firstNavigableNode = computed<MenuNode | null>(() => {
    for (const node of tree.value) {
      if (node.path) return node
      if (node.children?.length) {
        const firstChild = node.children.find((c) => c.path)
        if (firstChild) return firstChild
      }
    }
    return null
  })

  const findByPath = (path: string): MenuNode | null => {
    for (const node of tree.value) {
      if (node.path === path) return node
      if (node.children?.length) {
        const child = node.children.find((c) => c.path === path)
        if (child) return child
      }
    }
    return null
  }

  // Locale 切换时不需要重新拉数据（标题直接走 displayTitle）。
  // 但如果后端未来按 locale 下发，我们仍然保留这个 watcher，届时 schema 一变就能立刻接入。
  watch(
    () => i18n.global.locale.value,
    () => {
      // 故意留空：displayTitle 已经是 reactive
    },
  )

  const changeIsFirstSession = (payload: boolean) => {
    isFirstSession.value = payload
  }

  const changeFirstQuery = (
    payload: string,
    mentionedItems: any[] = [],
    modelId: string = '',
    imageFiles: any[] = [],
    attachmentFiles: any[] = [],
  ) => {
    firstQuery.value = payload
    firstMentionedItems.value = mentionedItems
    firstModelId.value = modelId
    firstImageFiles.value = imageFiles
    firstAttachmentFiles.value = attachmentFiles
  }

  const setPrefillQuery = (q: string) => {
    prefillQuery.value = q
  }

  const consumePrefillQuery = () => {
    const q = prefillQuery.value
    prefillQuery.value = ''
    return q
  }

  return {
    tree,
    topLevelNodes,
    firstNavigableNode,
    loading,
    error,
    loaded,
    isFirstSession,
    firstQuery,
    firstMentionedItems,
    firstModelId,
    firstImageFiles,
    firstAttachmentFiles,
    prefillQuery,
    loadMenu,
    reset,
    displayTitle,
    findByPath,
    changeIsFirstSession,
    changeFirstQuery,
    setPrefillQuery,
    consumePrefillQuery,
  }
})
