<template>
  <header class="platform-header">
    <!-- <div class="platform-header__left">
      <slot name="left" />
    </div> -->

    <div class="platform-header__right">
      <!-- 中英文切换：单个 pill 按钮，点击在 zh-CN / en-US 之间互切 -->
      <!-- <button type="button" class="lang-switcher" :title="t('header.languageSwitch')"
        :aria-label="t('header.languageSwitch')" @click="toggleLocale">
        <span class="lang-switcher__label">{{ currentLocaleLabel }}</span>
      </button> -->
      <!-- 用户菜单（来自原 menu.vue 底部） -->
      <UserMenu />
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import UserMenu from '@/components/UserMenu.vue'

const { locale, t } = useI18n()

// 只暴露 zh-CN / en-US 两个目标，避免出现不在 messages 里的 fallback 值
const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
type SupportedLocale = typeof SUPPORTED_LOCALES[number]

const currentLocale = computed<SupportedLocale>(() => {
  const v = String(locale.value || '')
  return (SUPPORTED_LOCALES as readonly string[]).includes(v)
    ? (v as SupportedLocale)
    : 'zh-CN'
})

// 按钮上展示的内容：跟着 currentLocale 走，反映“当前”语言名
// 因为依赖 t()，vue-i18n 的 locale 变化会自动触发重算 → 切换后立刻变文本
const currentLocaleLabel = computed(() =>
  currentLocale.value === 'zh-CN'
    ? t('header.languageZh')
    : t('header.languageEn')
)

// 顺序：zh-CN <-> en-US，互切
const TOGGLE_NEXT: Record<SupportedLocale, SupportedLocale> = {
  'zh-CN': 'en-US',
  'en-US': 'zh-CN',
}

function toggleLocale(): void {
  setLocale(TOGGLE_NEXT[currentLocale.value])
}

function setLocale(next: SupportedLocale): void {
  if (locale.value === next) return
  // 与 src/i18n/index.ts 读取的存储 key 保持一致，让下次启动自动恢复
  try {
    localStorage.setItem('locale', next)
  } catch {
    // ignore storage errors (e.g. private mode, quota exceeded)
  }
  // 同步 <html lang> 属性，方便屏幕阅读器 / 浏览器拼写检查
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('lang', next.startsWith('zh') ? 'zh-CN' : 'en')
  }
  // 切换 i18n.locale —— vue-i18n 的 locale 是响应式的，所有用 t() / $t() /
  // i18n.global.t() 在 computed/watch/模板里的引用都会立刻同步刷新。
  // menu store 也已经 watch 了 locale.value，无需 reload。
  // 避免 location.reload 是因为整页重载会闪一下空白，并丢失未保存的 UI 状态。
  locale.value = next
}
</script>

<style scoped lang="less">
.platform-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  height: 56px;
  // padding: 0 20px;
  box-sizing: border-box;
  flex-shrink: 0;
  z-index: 10;
}

.platform-header__left {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: px;
}

.platform-header__right {
  display: flex;
  width: 100%;
  align-items: center;
  // gap: 4px;
}

/* 中英文切换：单一圆角胶囊按钮，点击在 zh-CN / en-US 之间互切；
   文案随 currentLocale 变化自动重渲染 */
.lang-switcher {
  appearance: none;
  border: 1px solid;
  background: transparent;
  cursor: pointer;
  height: 32px;
  width: 32px;
  padding: 0 10px;
  border-radius: 2px;
  display: inline-flex;
  white-space: nowrap;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-primary);
  font-size: 16px;
  font-weight: 600;
  font-family: var(--app-font-family);
  letter-spacing: 0.2px;
  line-height: 1;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease, transform 0.15s ease;

  &:hover {
    background: var(--td-brand-color-light, var(--td-bg-color-container-hover, var(--td-gray-color-1)));
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
  }

  &:active {
    transform: scale(0.97);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color-focus, var(--td-brand-color));
    outline-offset: 1px;
  }
}

.lang-switcher__label {
  display: inline-block;
  line-height: 1;
  transition: opacity 0.12s ease;
}


.platform-header :deep(.user-button) {
  // width: auto;
  padding: 4px 8px;
}

// .platform-header :deep(.user-dropdown) {
//   /* 与「menu 底部」相反：这里 UserMenu 在顶部，向下弹出才不会被裁掉。
//      同时清掉 sidebarCollapsed=collapsed 时的 left:calc(100%+8px) 偏移。 */
//   top: calc(100% + 8px);
//   bottom: auto;
//   left: auto;
//   right: 0;
//   width: 280px;
//   /* 让浮层在 page header 之上，确保不被相邻的 dropdown / chat panel 截断 */
//   z-index: 1100;
// }
</style>
