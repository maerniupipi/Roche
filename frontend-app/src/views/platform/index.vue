<!--
  filepath: src/views/platform/index.vue
  Stage B · platform layout 重写（v6）

  不再渲染桌面 Layout（Aside / Header / Menu）。
  直接渲染 <MobileAppShell>，内部包含：
    - <MobileNavbar />（三段式）
    - <RouterView />  → 对话页 / 新建对话首页
    - <MobileDrawer />（左侧历史 + UserMenu）

  保留逻辑：
    - 全局拖拽文件 → 触发 chat-file-drop 事件（被 chat 页面监听）
    - provide('app:reload', reloadApp)（被 chat 页面软刷新用）
    - 后台预取 chat input 资源
-->
<template>
  <div
    class="platform-mobile"
    ref="dropzone"
    :class="{ 'platform-mobile--creat-chat': isCreatChatRoute }"
  >
    <MobileAppShell />

    <!-- 全局文件拖拽遮罩（chat 页面会监听 rochekap:chat-file-drop） -->
    <div class="upload-mask" v-show="ismask">
      <input
        type="file"
        style="display: none"
        ref="uploadInput"
        accept=".pdf,.docx,.doc,.pptx,.ppt,.epub,.mhtml,.txt,.md,.jpg,.jpeg,.png,.csv,.xls,.xlsx"
      />
      <UploadMask></UploadMask>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, provide, ref } from 'vue'
import { useRoute } from 'vue-router'
import MobileAppShell from '@/components/mobile/MobileAppShell.vue'
import UploadMask from '@/components/upload-mask.vue'
import { useChatResourcesStore } from '@/stores/chatResources'

const route = useRoute()
const ismask = ref(false)
const uploadInput = ref()
const dropzone = ref<HTMLElement | null>(null)

const isRouterAlive = ref(true)
const reloadApp = (): void => {
  isRouterAlive.value = false
  nextTick(() => {
    isRouterAlive.value = true
  })
}
provide('app:reload', reloadApp)

// 仅在 Wails 桌面端运行时拦截 Cmd/Ctrl+R
const isWailsDesktop =
  typeof window !== 'undefined' && !!(window as any).runtime?.EventsOn

const handleGlobalKeyDown = (e: KeyboardEvent): void => {
  if (!isWailsDesktop) return
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'r') {
    e.preventDefault()
    reloadApp()
  }
}

// 全局文件拖拽 → 仅在 chat 路由下转发
let dragCounter = 0
const CHAT_DROP_ROUTE_NAMES = new Set(['chat', 'globalCreatChat'])

const isChatDropRoute = (): boolean => {
  return CHAT_DROP_ROUTE_NAMES.has(String(route.name || ''))
}

const CREAT_CHAT_ROUTE_NAMES = new Set(['globalCreatChat'])

const isCreatChatRoute = computed<boolean>(() =>
  CREAT_CHAT_ROUTE_NAMES.has(String(route.name || '')),
)

const collectDroppedFiles = async (event: DragEvent): Promise<File[]> => {
  const dataTransferFiles = event.dataTransfer?.files
    ? Array.from(event.dataTransfer.files)
    : []
  if (dataTransferFiles.length > 0) {
    return dataTransferFiles
  }

  const dataTransferItems = event.dataTransfer?.items
    ? Array.from(event.dataTransfer.items)
    : []
  if (dataTransferItems.length === 0) {
    return []
  }

  const files = await Promise.all(
    dataTransferItems.map(
      (item) =>
        new Promise<File | null>((resolve) => {
          const fileEntry = (item as any).webkitGetAsEntry?.()
          if (fileEntry?.isFile && typeof fileEntry.file === 'function') {
            fileEntry.file(
              (file: File) => resolve(file),
              () => resolve(null),
            )
            return
          }
          resolve(null)
        }),
    ),
  )

  return files.filter((file): file is File => file instanceof File)
}

const isFileDrag = (event: DragEvent): boolean => {
  const types = event.dataTransfer?.types
  if (!types) return false
  return Array.from(types).includes('Files')
}

const handleGlobalDragEnter = (event: DragEvent): void => {
  if (!isFileDrag(event)) return
  event.preventDefault()
  dragCounter++
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'all'
  }
  ismask.value = true
}

const handleGlobalDragOver = (event: DragEvent): void => {
  if (!isFileDrag(event)) return
  event.preventDefault()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy'
  }
}

const handleGlobalDragLeave = (event: DragEvent): void => {
  if (!isFileDrag(event)) return
  event.preventDefault()
  dragCounter--
  if (dragCounter === 0) {
    ismask.value = false
  }
}

const handleGlobalDrop = async (event: DragEvent): Promise<void> => {
  if (!isFileDrag(event)) return
  event.preventDefault()
  dragCounter = 0
  ismask.value = false

  const droppedFiles = await collectDroppedFiles(event)
  if (droppedFiles.length === 0) {
    return
  }

  if (isChatDropRoute()) {
    event.stopPropagation()
    window.dispatchEvent(
      new CustomEvent('rochekap:chat-file-drop', {
        detail: { files: droppedFiles },
      }),
    )
    return
  }
}

onMounted(() => {
  document.addEventListener('dragenter', handleGlobalDragEnter, true)
  document.addEventListener('dragover', handleGlobalDragOver, true)
  document.addEventListener('dragleave', handleGlobalDragLeave, true)
  document.addEventListener('drop', handleGlobalDrop, true)
  document.addEventListener('keydown', handleGlobalKeyDown)
  // 后台预取对话输入栏资源，进入 creatChat / chat 时复用缓存
  void useChatResourcesStore().prefetchChatInput()
})

onUnmounted(() => {
  document.removeEventListener('dragenter', handleGlobalDragEnter, true)
  document.removeEventListener('dragover', handleGlobalDragOver, true)
  document.removeEventListener('dragleave', handleGlobalDragLeave, true)
  document.removeEventListener('drop', handleGlobalDrop, true)
  document.removeEventListener('keydown', handleGlobalKeyDown)
  dragCounter = 0
})
</script>

<style lang="less" scoped>
.platform-mobile {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container);
  overflow: hidden;
}

// 仅在新建会话页（creatChat）展示顶部装饰背景 —— 与原桌面行为保持一致
.platform-mobile--creat-chat {
  background: var(--td-bg-color-container) url('@/assets/img/chatBg.png') no-repeat top center;
  background-size: 100% auto;
}

.upload-mask {
  background-color: rgba(255, 255, 255, 0.8);
  position: fixed;
  width: 100%;
  height: 100%;
  z-index: 999;
  display: flex;
  justify-content: center;
  align-items: center;
}

img {
  -webkit-user-drag: none;
  -khtml-user-drag: none;
  -moz-user-drag: none;
  -o-user-drag: none;
  user-drag: none;
}
</style>
