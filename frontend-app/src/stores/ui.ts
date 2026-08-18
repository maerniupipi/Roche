// filepath: src/stores/ui.ts
import { defineStore } from 'pinia'

/**
 * 移动端 UI 全局状态（Phase A 精简后）
 *
 * 已移除：
 *   - deviceType / breakpoint / viewportWidth     （设备形态已不再动态决策）
 *   - bindResponsiveState / forceDeviceType / clearDeviceTypePref
 *   - isMobile / isDesktop / isCompactViewport getters
 *
 * 仅保留 UI 层需要的全局状态：设置弹窗与侧边栏折叠。
 * 设备断点请直接 import useBreakpoint()（响应式 ref 共享单例）。
 */
export const useUIStore = defineStore('ui', {
  state: () => ({
    showSettingsModal: false,
    settingsInitialSection: null as string | null,
    settingsInitialSubSection: null as string | null,
    sidebarCollapsed: localStorage.getItem('sidebar_collapsed') === 'true',
  }),

  actions: {
    openSettings(section?: string, subSection?: string) {
      this.settingsInitialSection = section || null
      this.settingsInitialSubSection = subSection || null
      this.showSettingsModal = true
    },

    closeSettings() {
      this.showSettingsModal = false
      this.settingsInitialSection = null
      this.settingsInitialSubSection = null
    },

    toggleSettings() {
      this.showSettingsModal = !this.showSettingsModal
    },

    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      localStorage.setItem('sidebar_collapsed', String(this.sidebarCollapsed))
    },

    collapseSidebar() {
      this.sidebarCollapsed = true
      localStorage.setItem('sidebar_collapsed', 'true')
    },

    expandSidebar() {
      this.sidebarCollapsed = false
      localStorage.setItem('sidebar_collapsed', 'false')
    },
  },
})
