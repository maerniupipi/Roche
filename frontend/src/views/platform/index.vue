<template>
  <div class="main" ref="dropzone" :class="{ 'platform-content--creat-chat': isCreatChatRoute }">
    <Menu></Menu>
    <div class="platform-content">
      <div v-if="isRouterAlive" class="platform-route-outlet">
        <RouterView />
      </div>
    </div>
    <div class="upload-mask" v-show="ismask">
      <input type="file" style="display: none" ref="uploadInput"
        accept=".pdf,.docx,.doc,.pptx,.ppt,.epub,.mhtml,.txt,.md,.jpg,.jpeg,.png,.csv,.xls,.xlsx" />
      <UploadMask></UploadMask>
    </div>
    <!-- 全局设置模态框，供所有 platform 子路由使用 -->
    <Settings v-if="canConfigurePlatform" />
  </div>
</template>
<script setup lang="ts">
import Menu from '@/components/menu.vue'
import { computed, ref, onMounted, onUnmounted, nextTick, provide } from 'vue';
import { useRoute } from 'vue-router'
import UploadMask from '@/components/upload-mask.vue'
import Settings from '@/views/settings/Settings.vue'
import { useChatResourcesStore } from '@/stores/chatResources'
import { useAuthStore } from '@/stores/auth'

const route = useRoute();
let ismask = ref(false)
let uploadInput = ref();
const authStore = useAuthStore();
const canConfigurePlatform = computed(() => authStore.isSystemAdmin);

const isRouterAlive = ref(true)
const reloadApp = () => {
  isRouterAlive.value = false
  nextTick(() => {
    isRouterAlive.value = true
  })
}
provide('app:reload', reloadApp)

// 仅在 Wails 桌面端运行时拦截 Cmd/Ctrl+R：
// 桌面端没有浏览器地址栏，整页重载会白屏，所以用前端软刷新替代。
// 浏览器（含 Web 版 / 非 Lite 部署）里不拦截，交给浏览器做真正的整页刷新，
// 否则会出现左侧菜单、全局设置、Pinia store 等不随"刷新"一起重置的问题。
// @ts-ignore
const isWailsDesktop = typeof window !== 'undefined' && !!(window as any).runtime?.EventsOn

const handleGlobalKeyDown = (e: KeyboardEvent) => {
  if (!isWailsDesktop) return
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'r') {
    e.preventDefault()
    reloadApp()
  }
}

// 用于跟踪拖拽进入/离开的计数器，解决子元素触发 dragleave 的问题
let dragCounter = 0;

const CHAT_DROP_ROUTE_NAMES = new Set(['chat', 'globalCreatChat']);

const isChatDropRoute = () => {
  return CHAT_DROP_ROUTE_NAMES.has(String(route.name || ''));
}

// 新建会话页（带背景图）—— 与 active chat 区分开
const CREAT_CHAT_ROUTE_NAMES = new Set(['globalCreatChat']);

const isCreatChatRoute = computed(() => CREAT_CHAT_ROUTE_NAMES.has(String(route.name || '')));

const collectDroppedFiles = async (event: DragEvent): Promise<File[]> => {
  const dataTransferFiles = event.dataTransfer?.files ? Array.from(event.dataTransfer.files) : [];
  if (dataTransferFiles.length > 0) {
    return dataTransferFiles;
  }

  const dataTransferItems = event.dataTransfer?.items ? Array.from(event.dataTransfer.items) : [];
  if (dataTransferItems.length === 0) {
    return [];
  }

  const files = await Promise.all(dataTransferItems.map(item => new Promise<File | null>((resolve) => {
    const fileEntry = (item as any).webkitGetAsEntry?.();
    if (fileEntry?.isFile && typeof fileEntry.file === 'function') {
      fileEntry.file((file: File) => resolve(file), () => resolve(null));
      return;
    }
    resolve(null);
  })));

  return files.filter((file): file is File => file instanceof File);
}

// isFileDrag distinguishes an OS file drag (the only thing the global upload
// drop zone cares about) from an in-app element drag. Element drags carry
// only "text/*" types, never
// "Files", so we bail out and let the originating component handle the drop.
const isFileDrag = (event: DragEvent): boolean => {
  const types = event.dataTransfer?.types
  if (!types) return false
  return Array.from(types).includes('Files')
}

// 全局拖拽事件处理
const handleGlobalDragEnter = (event: DragEvent) => {
  if (!isFileDrag(event)) return;
  event.preventDefault();
  dragCounter++;
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'all';
  }
  ismask.value = true;
}

const handleGlobalDragOver = (event: DragEvent) => {
  if (!isFileDrag(event)) return;
  event.preventDefault();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy';
  }
}

const handleGlobalDragLeave = (event: DragEvent) => {
  if (!isFileDrag(event)) return;
  event.preventDefault();
  dragCounter--;
  if (dragCounter === 0) {
    ismask.value = false;
  }
}

const handleGlobalDrop = async (event: DragEvent) => {
  if (!isFileDrag(event)) return;
  event.preventDefault();
  dragCounter = 0;
  ismask.value = false;

  const droppedFiles = await collectDroppedFiles(event);
  if (droppedFiles.length === 0) {
    return;
  }

  if (isChatDropRoute()) {
    event.stopPropagation();
    window.dispatchEvent(new CustomEvent('rochekap:chat-file-drop', {
      detail: { files: droppedFiles }
    }));
    return;
  }
}

// 组件挂载时添加全局事件监听器
onMounted(() => {
    document.addEventListener('dragenter', handleGlobalDragEnter, true);
    document.addEventListener('dragover', handleGlobalDragOver, true);
    document.addEventListener('dragleave', handleGlobalDragLeave, true);
    document.addEventListener('drop', handleGlobalDrop, true);
    // 后台预取对话输入栏资源，进入 creatChat / chat 时复用缓存
    void useChatResourcesStore().prefetchChatInput()
});

// 组件卸载时移除全局事件监听器
onUnmounted(() => {
    document.removeEventListener('dragenter', handleGlobalDragEnter, true);
    document.removeEventListener('dragover', handleGlobalDragOver, true);
    document.removeEventListener('dragleave', handleGlobalDragLeave, true);
    document.removeEventListener('drop', handleGlobalDrop, true);
    dragCounter = 0;
});
</script>
<style lang="less" scope>
.main {
  display: flex;
  align-items: stretch;
  width: 100%;
  height: 100%;
  min-width: 600px;
  min-height: 0;
  /* 统一整页背景，让左侧菜单与右侧内容区视觉连贯 */
  background: var(--td-bg-color-container);
}

/* 右侧内容列：竖直堆叠 PlatformHeader + 路由出口 */
.platform-content {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 仅在新建会话页（creatChat）展示顶部装饰背景 */
.platform-content--creat-chat {
  background: var(--td-bg-color-container) url('@/assets/img/chatBg.png') no-repeat top center;
  background-size: 100% auto;
}

/* 右侧路由区：占满剩余宽度与整列高度，并把 min-height:0 传给子页面以便内部 flex 滚动 */
.platform-route-outlet {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
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
