import type { SpotlightGuideStep } from '@/types/spotlightGuide'

export const GLOBAL_USER_GUIDE_KEY = 'rochekap:new-user-guide-done:v1'
export const OPEN_NEW_USER_GUIDE_EVENT = 'rochekap:open-new-user-guide'

export function openNewUserGuide() {
  window.dispatchEvent(new CustomEvent(OPEN_NEW_USER_GUIDE_EVENT))
}

export const KB_EDITOR_FOCUS_SECTION_EVENT = 'rochekap:kb-editor-focus-section'

export type ContextualGuideTourId = 'chat'

const focusKbEditorSection = (section: string) => {
  window.dispatchEvent(
    new CustomEvent(KB_EDITOR_FOCUS_SECTION_EVENT, { detail: { section } }),
  )
}

const focusKbEditorBasic = () => focusKbEditorSection('basic')

export interface ContextualGuideTourConfig {
  storageKey: string
  stepI18nPrefix: string
  steps: SpotlightGuideStep[]
  /** 首次展示前的延迟（毫秒） */
  openDelayMs: number
  /** 完成本引导时一并标记为已完成的其他引导 */
  alsoCompleteTours?: ContextualGuideTourId[]
}

export const CONTEXTUAL_GUIDE_TOURS: Record<ContextualGuideTourId, ContextualGuideTourConfig> = {
  chat: {
    storageKey: 'rochekap:contextual-guide-chat:v1',
    stepI18nPrefix: 'contextualGuide.chat.steps',
    openDelayMs: 800,
    steps: [
      {
        key: 'input',
        target: '[data-guide="chat-input"]',
        placement: 'top',
      },
      {
        key: 'send',
        target: '[data-guide="chat-send"]',
        placement: 'top',
      },
      { key: 'done' },
    ],
  },
}

export function isContextualGuideDone(tourId: ContextualGuideTourId): boolean {
  return localStorage.getItem(CONTEXTUAL_GUIDE_TOURS[tourId].storageKey) === '1'
}

export function markContextualGuideDone(tourId: ContextualGuideTourId) {
  const config = CONTEXTUAL_GUIDE_TOURS[tourId]
  localStorage.setItem(config.storageKey, '1')
  config.alsoCompleteTours?.forEach((id) => {
    localStorage.setItem(CONTEXTUAL_GUIDE_TOURS[id].storageKey, '1')
  })
}

export function isGlobalUserGuideDone(): boolean {
  return localStorage.getItem(GLOBAL_USER_GUIDE_KEY) === '1'
}
