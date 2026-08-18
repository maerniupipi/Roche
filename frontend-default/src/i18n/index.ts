import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.ts'
import enUS from './locales/en-US.ts'
import { loadSupportedLocale } from './locale.ts'

const messages = {
  'zh-CN': zhCN,
  'en-US': enUS
}

// 从 localStorage 中读取已保存的语言设置，默认为中文
const savedLocale = loadSupportedLocale(localStorage)

const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'zh-CN',
  globalInjection: true,
  // Some translations intentionally embed `<strong>` markup (e.g. agent step summaries).
  // We render them via v-html with our own sanitization, so silence vue-i18n's HTML warning
  // to avoid flooding the console and slowing renders during history loads.
  warnHtmlMessage: false,
  messages
})

export default i18n
