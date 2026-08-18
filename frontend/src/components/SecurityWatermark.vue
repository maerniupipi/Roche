<script setup lang="ts">
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'

/**
 * 全站低对比度视觉隐形水印
 *
 * 设计要点：
 * - 文字颜色用 rgba(0, 0, 0, 0.006)（约 1.5/255 不透明度）
 * - 在任何背景上合成的最终像素与背景相差约 1 个灰度值（≈0.4%）
 * - 人眼在屏幕上几乎不可分辨
 * - 截图后用 PS「色阶 / 曲线 / 对比度」即可放大该差异，水印清晰可读
 * - 格式与现有 document-preview.vue 一致：
 *   `${preview.watermarkPrefix} ${username} · ${YYYY-MM-DD HH:mm} ${preview.watermarkSuffix}`
 *
 * 注意：
 * - 组件本身不渲染任何 DOM，水印层由脚本直接挂到 document.body
 * - pointer-events: none 不拦截任何交互
 * - z-index 9998（< NewUserGuide 引导遮罩，确保引导盖在水印之上）
 */

const { t } = useI18n()
const { user } = storeToRefs(useAuthStore())

const Z_INDEX = 9998
const CELL_WIDTH = 120
const CELL_HEIGHT = 90
const ROTATE_DEG = -22
const FONT_SIZE = 6
const TEXT_ALPHA = 0.006 // 视觉隐形；与背景差 ≈ 1.5 / 255
const ROLL_INTERVAL_MS = 60_000 // 时间戳每分钟滚动

let containerEl: HTMLDivElement | null = null
let timer: number | null = null

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function getWatermarkUser(): string {
  const u = user.value as { username?: string; email?: string } | null
  return u?.username || u?.email || 'anonymous'
}

function getWatermarkTimestamp(): string {
  const d = new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function buildPrefixLine(): string {
  return `${t('preview.watermarkPrefix')} ${getWatermarkUser()} ${t('preview.watermarkSuffix')}`
}

function buildCellDataUrl(): string {
  const dpr = Math.max(1, window.devicePixelRatio || 1)
  const canvas = document.createElement('canvas')
  canvas.width = CELL_WIDTH * dpr
  canvas.height = CELL_HEIGHT * dpr

  const ctx = canvas.getContext('2d')
  if (!ctx) return ''

  ctx.scale(dpr, dpr)
  ctx.font = `${FONT_SIZE}px sans-serif`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  // 极低对比度：合成后像素与背景相差约 1.5/255，肉眼无法察觉
  ctx.fillStyle = `rgba(0, 0, 0, ${TEXT_ALPHA})`

  // 与现有 document-preview.vue 视觉风格一致：旋转 -22°
  ctx.translate(CELL_WIDTH / 2, CELL_HEIGHT / 2)
  ctx.rotate((ROTATE_DEG * Math.PI) / 180)

  // 第 1 行：前缀 + 用户 + 后缀
  ctx.fillText(buildPrefixLine(), 0, -10)
  // 第 2 行：时间戳
  ctx.fillText(getWatermarkTimestamp(), 0, 0)

  return canvas.toDataURL('image/png')
}

function rebuild(): void {
  // 清理旧水印
  if (containerEl) {
    containerEl.remove()
    containerEl = null
  }
  // 未登录不渲染
  if (!user.value) return

  const dataUrl = buildCellDataUrl()
  if (!dataUrl) return

  const div = document.createElement('div')
  div.dataset.watermark = 'visual-invisible'
  // 用 JSON.stringify 转义，避免 url() 中带特殊符号破坏 CSS
  div.style.cssText = [
    'position:fixed',
    'inset:0',
    'pointer-events:none',
    `z-index:${Z_INDEX}`,
    `background-image:url(${JSON.stringify(dataUrl)})`,
    'background-repeat:repeat',
    'background-attachment:fixed',
    'background-position:-90px -20px',
  ].join(';')

  document.body.appendChild(div)
  containerEl = div
}

function startTicker(): void {
  stopTicker()
  timer = window.setInterval(rebuild, ROLL_INTERVAL_MS)
}

function stopTicker(): void {
  if (timer != null) {
    clearInterval(timer)
    timer = null
  }
}

onMounted(() => {
  rebuild()
  startTicker()
})

// 用户登录 / 登出 / 切换时立即重建水印内容
watch(user, () => rebuild())

onBeforeUnmount(() => {
  stopTicker()
  if (containerEl) {
    containerEl.remove()
    containerEl = null
  }
})
</script>

<template>
  <div v-if="false"></div>
</template>