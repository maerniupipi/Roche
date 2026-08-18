<template>
  <div ref="rowRef"
    :class="['submenu_item', !batchMode && activePath === item.path ? 'submenu_item_active' : '', batchMode && selectedIds.includes(item.id) ? 'submenu_item_selected' : '', batchMode ? 'submenu_item_batch' : '',]"
    :style="rowStyle" @mouseenter="emit('hover-in')" @mouseleave="emit('hover-out')" @click="handleRowClick"
    @touchstart="handleTouchStart" @touchend="handleTouchEnd" @touchmove="handleTouchMove" @touchcancel="resetTouch">
    <t-checkbox v-if="batchMode" class="batch-checkbox" :checked="selectedIds.includes(item.id)" @click.stop
      @change="emit('toggle-select')" />
    <span class="submenu_title" :class="batchMode ? 'submenu_title--batch' : ''" :title="item.title">
      <t-icon v-if="item.is_pinned" name="pin" class="submenu_pin_icon" />
      <span class="submenu_title-text">{{ item.title }}</span>
    </span>
  </div>
  <Teleport to="body">
    <Transition name="lp-mirror">
      <div v-if="touchTriggered && !batchMode" ref="mirrorRef" class="longpress-mirror" :style="mirrorStyle"
        @click.stop>
        <t-icon v-if="item.is_pinned" name="pin" class="submenu_pin_icon" />
        <span class="submenu_title-text">{{ item.title }}</span>
      </div>
    </Transition>
    <Transition name="lp-fade">
      <div v-if="touchTriggered && !batchMode" class="longpress-backdrop" @click="dismissLongPress" />
    </Transition>
    <Transition name="lp-pop">
      <div v-if="touchTriggered && !batchMode" ref="popoverRef" class="longpress-popover" :style="popoverStyle"
        @click.stop>
        <button v-for="option in menuOptions" :key="option.value" type="button" class="longpress-popover__item"
          :class="{ 'longpress-popover__item--error': option.theme === 'error' }" @click="handleActionClick(option)">
          <component :is="option.prefixIcon" v-if="option.prefixIcon" class="longpress-popover__icon" />
          <span>{{ option.content }}</span>
        </button>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'

interface SessionMenuOption {
  content: string
  value: string
  theme?: 'default' | 'success' | 'warning' | 'error' | 'primary'
  prefixIcon?: any
}

const props = defineProps<{
  item: { id: string; path: string; title: string; is_pinned?: boolean }
  batchMode: boolean
  activePath: string
  selectedIds: string[]
  menuOptions: SessionMenuOption[]
  /** 嵌入渠道文件夹下的会话（样式与聊天区会话共用文案列对齐） */
  nested?: boolean
}>()

const emit = defineEmits<{
  (e: 'navigate'): void
  (e: 'toggle-select'): void
  (e: 'menu-click', data: { value: string }): void
  (e: 'hover-in'): void
  (e: 'hover-out'): void
}>()

const { t } = useI18n()

// === 桌面 click / 触屏 click 兼容 ===
function handleRowClick(): void {
  if (props.batchMode) {
    emit('toggle-select')
  } else {
    emit('navigate')
  }
}

// === 长按唤起自定义 Popover（mobile） ===
const LONG_PRESS_MS = 500
const MOVE_TOLERANCE_PX = 10
const POPOVER_GAP_PX = 10
const VIEWPORT_MARGIN_PX = 8
const ITEM_HEIGHT_PX = 48
const CANCEL_HEIGHT_PX = 48
const POPOVER_WIDTH_PX = 220

const rowRef = ref<HTMLElement | null>(null)
const popoverRef = ref<HTMLElement | null>(null)
const mirrorRef = ref<HTMLElement | null>(null)
const popoverStyle = ref<Record<string, string>>({})
const mirrorStyle = ref<Record<string, string>>({})
const cancelLabel = t('common.cancel')

// 长按时原 row 隐藏（保留布局占位），由 mirror 在 backdrop 之上承接视觉高亮
const rowStyle = computed(() => {
  if (touchTriggered.value && !props.batchMode) {
    return { opacity: '0', pointerEvents: 'none' as const }
  }
  return {}
})

const touchStartedAt = ref(0)
const touchStartX = ref(0)
const touchStartY = ref(0)
const touchTriggered = ref(false)
let longPressTimer: ReturnType<typeof setTimeout> | null = null

function clearLongPressTimer(): void {
  if (longPressTimer !== null) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}

function resetTouch(): void {
  touchStartedAt.value = 0
  touchTriggered.value = false
  clearLongPressTimer()
}

function dismissLongPress(): void {
  resetTouch()
}

function estimatePopoverHeight(): number {
  // 预估值：菜单项数 + 取消按钮 + 内边距
  return props.menuOptions.length * ITEM_HEIGHT_PX + CANCEL_HEIGHT_PX + 16
}

function updateLongPressLayout(): void {
  const row = rowRef.value
  if (!row) return
  const rect = row.getBoundingClientRect()
  const vw = window.innerWidth
  const vh = window.innerHeight

  // === 1) Mirror 行定位：覆盖原 row 的视口位置（由 Teleport 到 body 脱离 Drawer stacking context）===
  mirrorStyle.value = {
    top: `${rect.top}px`,
    left: `${rect.left + 10}px`,
    width: `${rect.width}px`,
    height: `${rect.height - 20}px`,
  }

  // === 2) Popover 定位：水平居中 row，垂直优先下方，clamp 到 viewport ===
  const popoverHeight = popoverRef.value?.offsetHeight ?? estimatePopoverHeight()
  let left = rect.left + rect.width / 2 - POPOVER_WIDTH_PX
  left = Math.max(VIEWPORT_MARGIN_PX, Math.min(left, vw - POPOVER_WIDTH_PX - VIEWPORT_MARGIN_PX))

  let top: number
  if (rect.bottom + popoverHeight + POPOVER_GAP_PX < vh) {
    top = rect.bottom + POPOVER_GAP_PX
  } else if (rect.top - popoverHeight - POPOVER_GAP_PX > VIEWPORT_MARGIN_PX) {
    top = rect.top - popoverHeight - POPOVER_GAP_PX
  } else {
    top = Math.max(VIEWPORT_MARGIN_PX, Math.min(rect.bottom + POPOVER_GAP_PX, vh - popoverHeight - VIEWPORT_MARGIN_PX))
  }

  popoverStyle.value = {
    top: `${top}px`,
    left: `${left}px`,
    width: `${POPOVER_WIDTH_PX}px`,
  }
}

function handleTouchStart(event: TouchEvent): void {
  if (props.batchMode) return
  // 关键：不要在 touchstart 调 preventDefault
  //   —— 否则会同步拦截浏览器对后续 touchmove 的滚动处理，
  //   导致整个 history 列表在真实移动端滚得非常卡顿。
  // 长按文本选择 / 复制菜单 由 CSS 的 user-select + -webkit-touch-callout 拦截，
  // 自定义长按 Popover 仍由 500ms 计时器触发，与默认行为无关。
  const t = event.touches[0]
  if (!t) return
  touchStartedAt.value = Date.now()
  touchStartX.value = t.clientX
  touchStartY.value = t.clientY
  touchTriggered.value = false
  clearLongPressTimer()
  longPressTimer = setTimeout(() => {
    touchTriggered.value = true
    // 等 popover 完成首帧渲染后定位
    nextTick(() => {
      updateLongPressLayout()
    })
  }, LONG_PRESS_MS)
}

function handleTouchMove(_event: TouchEvent): void {
  // 反馈层已弹出后，用户手指移动（如滑向 Popover 选项）不应取消反馈
  if (touchTriggered.value) return
  if (!touchStartedAt.value) return
  const t = _event.touches[0]
  if (!t) return
  if (
    Math.abs(t.clientX - touchStartX.value) > MOVE_TOLERANCE_PX ||
    Math.abs(t.clientY - touchStartY.value) > MOVE_TOLERANCE_PX
  ) {
    // 拖动超过阈值 → 取消长按
    resetTouch()
  }
}

function handleTouchEnd(_event: TouchEvent): void {
  // 反馈层已弹出后：touchend 不应主动关闭，必须点 backdrop / cancel / option 才关闭
  if (touchTriggered.value) {
    // 顺手阻止一次浏览器合成的 click，避免误触发 row 跳转
    _event.preventDefault()
    return
  }
  // 未触发长按：清理计时器，依赖 click 事件做 navigate
  resetTouch()
}

function handleActionClick(option: SessionMenuOption): void {
  resetTouch()
  emit('menu-click', { value: option.value })
}

onBeforeUnmount(() => {
  clearLongPressTimer()
})
</script>

<style scoped lang="less">
.submenu_item {
  position: relative;
  z-index: 1;
  border-radius: 10px;
  // 禁止移动端浏览器默认长按文本选择 / 复制 / 气泡菜单
  -webkit-user-select: none;
  user-select: none;
  -webkit-touch-callout: none;
  touch-action: manipulation;
}

.submenu_title {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 12px;
  min-width: 0;
  overflow: hidden;

  .submenu_pin_icon {
    flex: 0 0 auto;
  }

  .submenu_title-text {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

/* === 长按反馈层 === */

/* 1. 镜像高亮行：Teleport 到 body，z-index 高于 backdrop，确保浮在毛玻璃背景之上 */
//   让用户能看到这一行的具体信息（pin icon + title 文本）。
//   自身不能加 backdrop-filter，否则会再次模糊自身内容 */
.longpress-mirror {
  position: fixed;
  z-index: 4100;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 10px;
  background: var(--td-bg-color-container);
  box-shadow:
    0 8px 24px rgba(0, 0, 0, 0.18),
    0 0 0 2px var(--td-brand-color);
  transform: scale(1.06);
  transform-origin: center center;

  .submenu_pin_icon {
    flex: 0 0 20px;
  }

  .submenu_title-text {
    flex: 1 1 auto;
    min-width: 0;
    font-size: 14px;
    line-height: 20px;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

/* 2. 毛玻璃背景 */
.longpress-backdrop {
  position: fixed;
  inset: 0;
  z-index: 4000;
  background: rgba(15, 18, 22, 0.42);
  backdrop-filter: blur(2px) saturate(120%);
  -webkit-backdrop-filter: blur(2px) saturate(120%);
}

/* 3. 跟随长按行位置的 Popover（z-index 高于 mirror，视觉上不重叠） */
.longpress-popover {
  position: fixed;
  z-index: 4200;
  border-radius: 14px;
  background: var(--td-bg-color-container);
  box-shadow:
    0 16px 40px rgba(0, 0, 0, 0.22),
    0 0 0 1px var(--td-component-stroke);
  overflow: hidden;

  &__item {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 48px;
    padding: 0 16px;
    border: 0;
    background: transparent;
    color: var(--td-text-color-primary);
    font-size: 15px;
    line-height: 22px;
    text-align: left;
    cursor: pointer;
    transition: background-color 0.15s ease;

    &:active {
      background: var(--td-bg-color-container-hover);
    }

    &--error {
      color: var(--td-error-color);
    }
  }

  &__icon {
    flex: 0 0 auto;
    display: inline-flex;
  }

  &__cancel {
    display: block;
    width: 100%;
    min-height: 48px;
    padding: 0;
    border: 0;
    border-top: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-container);
    color: var(--td-text-color-secondary);
    font-size: 15px;
    cursor: pointer;
    transition: background-color 0.15s ease;

    &:active {
      background: var(--td-bg-color-container-hover);
    }
  }
}

/* === Transitions === */
.lp-fade-enter-active,
.lp-fade-leave-active {
  transition: opacity 0.2s ease;
}

.lp-fade-enter-from,
.lp-fade-leave-to {
  opacity: 0;
}

.lp-mirror-enter-active,
.lp-mirror-leave-active {
  transition:
    opacity 0.18s cubic-bezier(0.4, 0, 0.2, 1),
    transform 0.18s cubic-bezier(0.4, 0, 0.2, 1);
  transform-origin: center center;
}

.lp-mirror-enter-from,
.lp-mirror-leave-to {
  opacity: 0;
  transform: scale(1);
}

.lp-pop-enter-active,
.lp-pop-leave-active {
  transition:
    opacity 0.18s cubic-bezier(0.4, 0, 0.2, 1),
    transform 0.18s cubic-bezier(0.4, 0, 0.2, 1);
  transform-origin: top center;
}

.lp-pop-enter-from,
.lp-pop-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.96);
}
</style>
