// filepath: src/main.ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'

import TDesign from 'tdesign-mobile-vue'
import 'tdesign-mobile-vue/es/style/index.css'

import '@/assets/theme/theme.css'
import '@/assets/dropdown-menu.less'
import '@/components/css/suggested-questions.less'

import '@/assets/overrides/TDesign.less'
// 全局触摸目标 ≥ 44px
import '@/assets/overrides/mobile-touch.less'
// 全局 safe-area utility class
import '@/assets/overrides/safe-area.less'
// 全局滚动行为 + iOS sticky 兜底
import '@/assets/overrides/scroll.less'

import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'
import i18n from './i18n'
import { initTheme } from '@/composables/useTheme'
import { installTDesignIconOfflineGuard } from '@/utils/tdesign-icon-offline'
import { installAutofillGuard } from '@/utils/disable-autofill'
// 移动端 rem 设备适配（必须在 mount 之前）
import '@/utils/rem'

installTDesignIconOfflineGuard()

initTheme()

const app = createApp(App)

// 全局错误处理：捕获未处理的组件错误，防止白屏
app.config.errorHandler = (err, instance, info) => {
  console.error('[RocheKAP] Unhandled Vue error:', err, '\nComponent:', instance, '\nInfo:', info)
}

app.use(TDesign)
app.use(createPinia())
app.use(router)
app.use(i18n)

// 等首屏路由（含导航守卫、Lite 自动登录）完成后再挂载，避免先闪默认页再跳转
router.isReady().finally(() => {
  app.mount('#app')
  installAutofillGuard()
})
