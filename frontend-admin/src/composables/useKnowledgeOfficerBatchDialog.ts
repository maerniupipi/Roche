// 批量知识官范围选择 + 二次确认的状态机：
//   设知识官：scopeDialog（选范围）→ confirmDialog → onConfirm(true, scope)
//   取消知识官：跳过 scopeDialog → confirmDialog → onConfirm(false)
// 与单行流程保持一致的两阶段语义，把"是否设/取消 + 范围选择"集中在这一个 composable。
import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { Composer } from 'vue-i18n'

import type { KnowledgeOfficerScope } from '@/composables/useUserRoles'

export interface UseKnowledgeOfficerBatchDialogOptions {
  // 实际调用批量接口；返回 true 表示已成功执行，dialog 会关闭。
  onConfirm: (
    isOfficer: boolean,
    scope?: KnowledgeOfficerScope,
  ) => Promise<boolean>
  // 当前选中的"待操作"用户数（用于 i18n 占位）；用 getter 保持响应式。
  getUserCount: () => number
  t: Composer['t']
}

export interface UseKnowledgeOfficerBatchDialogResult {
  // scope dialog 状态
  scopeDialogVisible: Ref<boolean>
  // confirm dialog 状态
  confirmVisible: Ref<boolean>
  confirmTitle: ComputedRef<string>
  confirmContent: ComputedRef<string>
  // 当前会受影响用户数（从 options.getUserCount 派生的 computed，便于模板直接绑定）
  userCount: ComputedRef<number>
  // 触发入口
  openSet: () => void
  openUnset: () => void
  // scope dialog 事件
  onScopeConfirm: (payload: { domainIds: string[]; baseIds: string[] }) => void
  onScopeCancel: () => void
  // confirm dialog 事件
  onConfirm: () => Promise<void>
  onCancel: () => void
}

export function useKnowledgeOfficerBatchDialog(
  options: UseKnowledgeOfficerBatchDialogOptions,
): UseKnowledgeOfficerBatchDialogResult {
  const { onConfirm, getUserCount, t } = options

  const isOfficer = ref(false)
  const pendingScope = ref<KnowledgeOfficerScope | null>(null)

  const scopeDialogVisible = ref(false)
  const confirmVisible = ref(false)

  const confirmTitle = computed(() =>
    t('roles.knowledgeOfficer.batchConfirmTitle'),
  )
  const userCount = computed(() => getUserCount())
  const confirmContent = computed(() => {
    const count = userCount.value
    return isOfficer.value
      ? t('roles.knowledgeOfficer.batchSetMessage', { count })
      : t('roles.knowledgeOfficer.batchUnsetMessage', { count })
  })

  function openSet(): void {
    isOfficer.value = true
    pendingScope.value = null
    scopeDialogVisible.value = true
  }

  function openUnset(): void {
    isOfficer.value = false
    pendingScope.value = null
    scopeDialogVisible.value = false
    confirmVisible.value = true
  }

  function onScopeConfirm(payload: {
    domainIds: string[]
    baseIds: string[]
  }): void {
    pendingScope.value = {
      knowledgeDomainIds: payload.domainIds,
      knowledgeBaseIds: payload.baseIds,
    }
    scopeDialogVisible.value = false
    confirmVisible.value = true
  }

  function onScopeCancel(): void {
    scopeDialogVisible.value = false
    pendingScope.value = null
  }

  async function onConfirmAction(): Promise<void> {
    const ok = await onConfirm(
      isOfficer.value,
      pendingScope.value ?? undefined,
    )
    if (ok) {
      confirmVisible.value = false
      pendingScope.value = null
    }
  }

  function onCancel(): void {
    confirmVisible.value = false
    pendingScope.value = null
  }

  return {
    scopeDialogVisible,
    confirmVisible,
    confirmTitle,
    confirmContent,
    userCount,
    openSet,
    openUnset,
    onScopeConfirm,
    onScopeCancel,
    onConfirm: onConfirmAction,
    onCancel,
  }
}