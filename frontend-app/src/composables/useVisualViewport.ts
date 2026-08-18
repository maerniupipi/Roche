import { onMounted, onUnmounted, ref } from 'vue'

/**
 * 监听 window.visualViewport 高度变化
 * ============================================================
 * 用途：处理移动端键盘弹出场景——input 容器需要根据 visualViewport.height 调整 max-height。
 *
 * 注意：
 * - iOS Safari / Android Chrome 均支持 window.visualViewport
 * - 'resize' 事件在 keyboard 弹出/收起时触发
 * - 需要在 onMounted 中调用，onUnmounted 中清理
 *
 * 用法：
 * ```vue
 * <script setup lang="ts">
 * import { useVisualViewport } from '@/composables/useVisualViewport'
 * const { viewportHeight } = useVisualViewport()
 * </script>
 *
 * <style scoped>
 * .input-container {
 *   max-height: calc(v-bind(viewportHeight) * 0.6px);
 *   overflow-y: auto;
 * }
 * </style>
 * ```
 */
export function useVisualViewport() {
  const viewportHeight = ref(
    typeof window !== 'undefined'
      ? window.visualViewport?.height ?? window.innerHeight
      : 0,
  )

  function update(): void {
    if (window.visualViewport) {
      viewportHeight.value = window.visualViewport.height
    } else {
      viewportHeight.value = window.innerHeight
    }
  }

  onMounted(() => {
    update()
    if (window.visualViewport) {
      window.visualViewport.addEventListener('resize', update, { passive: true })
    } else {
      window.addEventListener('resize', update, { passive: true })
    }
  })

  onUnmounted(() => {
    if (window.visualViewport) {
      window.visualViewport.removeEventListener('resize', update)
    } else {
      window.removeEventListener('resize', update)
    }
  })

  return { viewportHeight }
}