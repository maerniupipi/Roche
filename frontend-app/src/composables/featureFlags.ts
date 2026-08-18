import { ref, type Ref } from 'vue'

const STORAGE_PREFIX = 'feature.'

function readStored(key: string): boolean {
  try {
    return localStorage.getItem(STORAGE_PREFIX + key) === 'true'
  } catch {
    return false
  }
}

function writeStored(key: string, value: boolean): void {
  try {
    localStorage.setItem(STORAGE_PREFIX + key, String(value))
  } catch {
    // 无痕模式 / 配额已满 → 仅保留内存值，下次刷新会丢失。
  }
}

/**
 * 定义一个具名布尔型功能开关，状态自动持久化到 localStorage。
 *
 * 返回值中的 `value` 是 `Ref<boolean>`，在模板里 Vue 会自动解包，
 * 调用方只需按名称导入即可直接使用 —— 不需要调用组合式函数，也不需要解构。
 *
 * 新增开关：在本文件末尾追加一行。开关 UI 调用 `set` 方法进行切换。
 *
 *   export const [isDebugger, setisDebugger] = defineFeatureFlag('isDebugger')
 *
 *   // 任意组件中
 *   import { isDebugger } from '@/composables/featureFlags'
 *   <div v-if="isDebugger">…</div>
 */
export function defineFeatureFlag(
  key: string,
): readonly [Ref<boolean>, (next: boolean) => void] {
  const value = ref(readStored(key))
  const set = (next: boolean): void => {
    value.value = next
    writeStored(key, next)
  }
  return [value, set] as const
}

// --- 功能开关注册区 -------------------------------------------------------
// 在这里新增开关。注意：不要随意改动 key 字符串 —— 一旦变更，已上线用户的
// 历史偏好会被静默重置。
export const [isDebugger, setisDebugger] = defineFeatureFlag('isDebugger')