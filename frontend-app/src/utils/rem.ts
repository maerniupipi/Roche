/**
 * 移动端 rem 设备适配
 * ============================================================
 * 核心：设置根 <html> 字号为 viewport 宽度 / 设计稿宽度 * 基础字号（37.5px）
 * 设计稿基准：375px（项目代码与 TDesign mobile 库设计稿同基准）
 *
 * 视觉效果（375 设计稿基准）：
 * - 320 (iPhone SE 1)     → 32px 根字号  (0.85x)
 * - 375 (iPhone 标准)     → 37.5px       (1.00x)
 * - 414 (iPhone Plus)     → 41.4px       (1.10x)
 * - 430 (iPhone 14 Pro Max) → 43px       (1.15x)
 * - 750 (iPad 横屏)       → 75px         (2.00x, 上限锁死)
 * - 1024 (iPad mini 横)   → 75px         (2.00x, 上限锁死)
 * - 1366 (iPad Pro 横)    → 75px         (2.00x, 上限锁死)
 *
 * 调用时机：main.ts 入口立即执行 + window resize
 *
 * 注意：
 * - 必须在 app.mount() 之前调用，否则首屏渲染时根字号未设置
 * - iPad 1024+ 通过 Math.min() 锁到 750，避免字号过夸张（iPad Pro 视觉 2x 是设计取舍）
 * - postcss-pxtorem 已将项目代码 + TDesign mobile 库的 px → rem，根字号变化时 UI 等比缩放
 */

const BASE_WIDTH = 375
const BASE_FONT_SIZE = 37.5
const MAX_WIDTH = 750  // iPad 横屏锁上限（避免 iPad Pro 1024+ 字号过大）

export function setRootFontSize(): void {
  if (typeof document === 'undefined') return
  const w = Math.min(document.documentElement.clientWidth, MAX_WIDTH)
  const rem = BASE_FONT_SIZE * (w / BASE_WIDTH)
  document.documentElement.style.fontSize = `${rem}px`
}

// 入口立即执行（保证首次 setProperty 完成）
setRootFontSize()

// resize 时同步根字号（横竖屏切换 / 浏览器窗口尺寸变化）
window.addEventListener('resize', setRootFontSize, { passive: true })