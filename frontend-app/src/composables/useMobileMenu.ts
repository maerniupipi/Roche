// filepath: src/composables/useMobileMenu.ts
//
// Stage B · Drawer 开关状态 composable（v6）
//
// 单一职责：管理移动端 Drawer 的开关状态 + 当前激活会话 ID。
// 整个 store 是模块级单例（getCurrentInstance == null 时返回同一份 ref），
// 任意调用 useMobileMenu() 的组件都共享同一份状态。
//
// 范围限定：
//   - **不**含 modalStack（v3 取消全局弹窗栈）
//   - **不**依赖路由表清理（不动 routes.ts）
//   - **不**写 router.beforeEach 守卫拦截深链
import { ref, type Ref } from 'vue'

// 模块级共享 ref —— 移动端整个生命周期只存在一个 Drawer 实例
const isOpen: Ref<boolean> = ref(false)
const activeSessionId: Ref<string | null> = ref(null)

export interface MobileMenuApi {
  isOpen: Ref<boolean>
  activeSessionId: Ref<string | null>
  open: () => void
  close: () => void
  toggle: () => void
  setActiveSessionId: (id: string | null) => void
}

export function useMobileMenu(): MobileMenuApi {
  return {
    isOpen,
    activeSessionId,
    open: () => {
      isOpen.value = true
    },
    close: () => {
      isOpen.value = false
    },
    toggle: () => {
      isOpen.value = !isOpen.value
    },
    setActiveSessionId: (id: string | null) => {
      activeSessionId.value = id
    },
  }
}