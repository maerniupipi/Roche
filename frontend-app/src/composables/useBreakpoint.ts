// filepath: src/composables/useBreakpoint.ts
import { computed, ref } from 'vue'
import type { Ref } from 'vue'

/**
 * 移动端 3 档断点（与 src/assets/breakpoints.less 对齐）：
 *   sm : < 768      （手机：portrait + 大部分横屏）
 *   md : 768-1023   （小平板：iPad mini / portrait iPad）
 *   xl : ≥ 1024     （大屏：iPad 横屏、桌面浏览器、平板分屏）
 *
 * 注意：项目已不再保留桌面形态。这里仅用于"小屏/中屏/大屏"的运行时 UI 微调
 * （如列数、字号、间距），不再有"桌面 vs 移动"的双形态分支。
 */
export type Breakpoint = 'sm' | 'md' | 'xl'

const BREAKPOINTS: Array<[Breakpoint, number]> = [
  ['xl', 1024],
  ['md', 768],
]

function resolveBreakpoint(width: number): Breakpoint {
  for (const [name, min] of BREAKPOINTS) {
    if (width >= min) return name
  }
  return 'sm'
}

/**
 * 模块级单例：所有组件共享同一个 widthRef，避免每个调用方都注册 resize 监听
 */
const widthRef = ref(typeof window !== 'undefined' ? window.innerWidth : 1024)
const breakpointRef = ref<Breakpoint>(resolveBreakpoint(widthRef.value))

let initialized = false
let rafId: number | null = null

function handleResize() {
  // 合并到下一帧，避免拖动窗口时频繁更新
  if (rafId !== null) return
  rafId = requestAnimationFrame(() => {
    rafId = null
    const next = window.innerWidth
    if (next !== widthRef.value) {
      widthRef.value = next
      const bp = resolveBreakpoint(next)
      if (bp !== breakpointRef.value) breakpointRef.value = bp
    }
  })
}

function ensureListener() {
  if (initialized || typeof window === 'undefined') return
  initialized = true
  window.addEventListener('resize', handleResize, { passive: true })
  // 横竖屏切换 / 折叠屏
  window.addEventListener('orientationchange', handleResize, { passive: true })
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', handleResize, { passive: true })
  }
}

/**
 * useBreakpoint —— 实时响应窗口宽度的响应式断点 composable
 *
 * 模块级单例：所有调用方共享同一份 widthRef / breakpointRef，仅注册一次
 * resize 监听，零额外开销。
 */
export function useBreakpoint() {
  ensureListener()

  const breakpoint: Ref<Breakpoint> = breakpointRef
  const width: Ref<number> = widthRef

  const isSm = computed(() => breakpointRef.value === 'sm')
  const isMd = computed(() => breakpointRef.value === 'md')
  const isXl = computed(() => breakpointRef.value === 'xl')
  // 移动端项目里"phone-like" 即 sm 档
  const isPhone = computed(() => breakpointRef.value === 'sm')
  const isTablet = computed(() => breakpointRef.value === 'md' || breakpointRef.value === 'xl')

  return {
    breakpoint,
    width,
    isSm,
    isMd,
    isXl,
    isPhone,
    isTablet,
  }
}

/**
 * 调试用：在测试环境或 dev 工具面板卸载监听
 */
export function disposeBreakpointListener() {
  if (!initialized || typeof window === 'undefined') return
  initialized = false
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('orientationchange', handleResize)
  if (window.visualViewport) {
    window.visualViewport.removeEventListener('resize', handleResize)
  }
  if (rafId !== null) {
    cancelAnimationFrame(rafId)
    rafId = null
  }
}
