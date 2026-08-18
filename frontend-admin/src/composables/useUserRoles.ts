// 用户角色配置模块状态管理（对接 /api/v1/system/admin/users）。
// 列表与状态字段直接对齐后端 UserInfo；批量操作内部从 useAuthStore 跳过自己。
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import {
  banUser,
  listUserRoles,
  setKnowledgeOfficerRoles,
  setKnowledgeOfficerRolesSingle,
  setOperatorRoles,
  setOperatorRolesSingle,
  unbanUser,
} from '@/api/user-roles'
import {
  DEFAULT_USER_ROLE_FILTERS,
  DEFAULT_USER_ROLE_PAGINATION,
  type DepartmentOption,
  type UserRole,
  type UserRoleFilters,
  type UserRolePagination,
  type UserRoleStatus,
} from '@/types/userRole'

export interface SelectionStats {
  empty: boolean
  ko: boolean
  nonKo: boolean
  oa: boolean
  nonOa: boolean
  blacklisted: boolean
  nonBlacklisted: boolean
}

export interface UseUserRolesResult {
  items: Ref<UserRole[]>
  loading: Ref<boolean>
  error: Ref<string>
  filters: Ref<UserRoleFilters>
  pagination: Ref<UserRolePagination>
  selectedIds: Ref<string[]>
  selectedRows: ComputedRef<UserRole[]>
  selectionStats: ComputedRef<SelectionStats>
  departmentOptions: ComputedRef<DepartmentOption[]>
  filteredTotal: ComputedRef<number>

  load: () => Promise<void>
  applyFilter: () => Promise<void>
  resetFilter: () => Promise<void>
  reload: () => Promise<void>

  toggleBlacklist: (row: UserRole) => Promise<void>
  toggleOpsAdmin: (row: UserRole) => Promise<void>
  // scope 可选：传了就带上 knowledgeDomainIds/knowledgeBaseIds（设为时由 KnowledgeScopeDialog 提供）；
  // 不传时只切换 role_knowledge_officer 字段（取消时不需要范围）。
  toggleKnowledgeOfficer: (
    row: UserRole,
    scope?: KnowledgeOfficerScope,
  ) => Promise<void>

  batchSetOperationsAdmin: (isAdmin: boolean) => Promise<boolean>
  batchSetBlacklisted: (isBlacklisted: boolean) => Promise<boolean>
  // 知识官批量：scope 仅在 isOfficer=true 时由批量知识官 dialog 提供；isOfficer=false 时不传。
  batchSetKnowledgeOfficer: (
    isOfficer: boolean,
    scope?: KnowledgeOfficerScope,
  ) => Promise<boolean>
}

function extractErrorMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  if (typeof e === 'object' && e !== null && 'message' in e) {
    const m = (e as { message?: unknown }).message
    if (typeof m === 'string') return m
  }
  return String(e)
}

// 设为知识官时附带的管理范围；由 KnowledgeScopeDialog 提供，取消时为 undefined。
export interface KnowledgeOfficerScope {
  knowledgeDomainIds: string[]
  knowledgeBaseIds: string[]
}

export function useUserRoles(): UseUserRolesResult {
  const { t } = useI18n()
  const authStore = useAuthStore()

  const items = ref<UserRole[]>([])
  const loading = ref(false)
  const error = ref('')

  const filters = ref<UserRoleFilters>({ ...DEFAULT_USER_ROLE_FILTERS })
  const pagination = ref<UserRolePagination>({ ...DEFAULT_USER_ROLE_PAGINATION })

  const selectedIds = ref<string[]>([])

  // 部门下拉：从前一次列表响应聚合 departmentId/departmentName/departmentCode 去重，
  // 不再额外调用部门接口；列表为空时下拉自然为空，下一次 load 后会重新聚合。
  const departmentOptions = computed<DepartmentOption[]>(() => {
    const map = new Map<string, DepartmentOption>()
    for (const u of items.value) {
      if (!u.departmentId || map.has(u.departmentId)) continue
      map.set(u.departmentId, {
        id: u.departmentId,
        name: u.departmentName,
        code: u.departmentCode,
      })
    }
    return [...map.values()]
  })

  const filteredTotal = computed(() => pagination.value.total)

  const selectedRows = computed(() =>
    items.value.filter((r) => selectedIds.value.includes(r.id)),
  )

  // 后端 status 是数字 0/1/2；前端 i18n 键期望 'active'/'inactive'/'blacklisted'。
  // 在写入 items 之前做规范化，避免模板里 t(`roles.status.${row.status}`) 拼出 'roles.status.0'。
  const STATUS_VALUE_TO_KEY: Record<number, UserRoleStatus> = {
    0: 'active',
    1: 'blacklisted',
    2: 'inactive',
  }

  function normalizeStatus(raw: unknown): UserRoleStatus {
    if (typeof raw === 'number' && raw in STATUS_VALUE_TO_KEY) {
      return STATUS_VALUE_TO_KEY[raw]
    }
    if (raw === 'active' || raw === 'blacklisted' || raw === 'inactive') {
      return raw
    }
    if (import.meta.env.DEV) {
      console.warn('[useUserRoles] unexpected user status value:', raw)
    }
    return 'inactive'
  }

  // 已选集合的归类：所有行都满足才返回 true，用于批量按钮的禁用判断。
  const selectionStats = computed<SelectionStats>(() => {
    const rows = selectedRows.value
    if (rows.length === 0) {
      return {
        empty: true,
        ko: false,
        nonKo: false,
        oa: false,
        nonOa: false,
        blacklisted: false,
        nonBlacklisted: false,
      }
    }
    return {
      empty: false,
      ko: rows.every((r) => r.isKnowledgeOfficer),
      nonKo: rows.every((r) => !r.isKnowledgeOfficer),
      oa: rows.every((r) => r.isOperationsAdmin),
      nonOa: rows.every((r) => !r.isOperationsAdmin),
      blacklisted: rows.every((r) => r.status === 'blacklisted'),
      nonBlacklisted: rows.every((r) => r.status !== 'blacklisted'),
    }
  })

  async function load(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const result = await listUserRoles({
        filters: filters.value,
        pagination: pagination.value,
      })
      items.value = result.items.map((u) => ({
        ...u,
        status: normalizeStatus(u.status),
      }))
      pagination.value.total = result.total
    } catch (e: unknown) {
      error.value = extractErrorMessage(e)
      items.value = []
      pagination.value.total = 0
      MessagePlugin.error(t('roles.messages.loadFailed'))
    } finally {
      loading.value = false
    }
  }

  async function applyFilter(): Promise<void> {
    pagination.value.page = 1
    selectedIds.value = []
    await load()
  }

  async function resetFilter(): Promise<void> {
    filters.value = { ...DEFAULT_USER_ROLE_FILTERS }
    pagination.value.page = 1
    selectedIds.value = []
    await load()
  }

  async function reload(): Promise<void> {
    await load()
  }

  async function toggleBlacklist(row: UserRole): Promise<void> {
    try {
      if (row.status === 'blacklisted') {
        await unbanUser(row.id)
      } else {
        await banUser(row.id)
      }
      await reload()
    } catch (e: unknown) {
      MessagePlugin.error(t('roles.messages.operationFailed', { msg: extractErrorMessage(e) }))
    }
  }

  async function toggleOpsAdmin(row: UserRole): Promise<void> {
    try {
      await setOperatorRolesSingle(row.id, {
        role_operator: row.isOperationsAdmin ? 0 : 1,
      })
      await reload()
    } catch (e: unknown) {
      MessagePlugin.error(t('roles.messages.operationFailed', { msg: extractErrorMessage(e) }))
    }
  }

  async function toggleKnowledgeOfficer(
    row: UserRole,
    scope?: KnowledgeOfficerScope,
  ): Promise<void> {
    // 设为时 scope 由 KnowledgeScopeDialog 提供；取消时不需要范围。
    try {
      const willSet = !row.isKnowledgeOfficer
      const payload: Parameters<typeof setKnowledgeOfficerRolesSingle>[1] = {
        role_knowledge_officer: willSet ? 1 : 0,
      }
      if (willSet && scope) {
        payload.knowledge_domain_ids = scope.knowledgeDomainIds
        payload.knowledge_base_ids = scope.knowledgeBaseIds
      }
      await setKnowledgeOfficerRolesSingle(row.id, payload)
      await reload()
    } catch (e: unknown) {
      MessagePlugin.error(t('roles.messages.operationFailed', { msg: extractErrorMessage(e) }))
    }
  }

  // 批量操作：统一在入口处过滤掉自己（避免 UI 禁用失效时误把自己一并提交）。
  function filterOutSelf(ids: string[]): string[] {
    const selfId = authStore.currentUserId
    if (!selfId) return ids
    return ids.filter((id) => id !== selfId)
  }

  async function batchSetOperationsAdmin(isAdmin: boolean): Promise<boolean> {
    const stats = selectionStats.value
    if (stats.empty) {
      MessagePlugin.warning(t('roles.messages.selectRequired'))
      return false
    }
    if (isAdmin && !stats.nonOa) {
      MessagePlugin.warning(t('roles.messages.selectInconsistent'))
      return false
    }
    if (!isAdmin && !stats.oa) {
      MessagePlugin.warning(t('roles.messages.selectInconsistent'))
      return false
    }
    const targets = filterOutSelf(selectedIds.value)
    if (targets.length === 0) {
      MessagePlugin.warning(t('roles.messages.cannotOperateSelf'))
      return false
    }
    try {
      await setOperatorRoles(targets, { role_operator: isAdmin ? 1 : 0 })
      MessagePlugin.success(t('roles.messages.operationSuccess'))
      await reload()
      return true
    } catch (e: unknown) {
      MessagePlugin.error(t('roles.messages.operationFailed', { msg: extractErrorMessage(e) }))
      return false
    }
  }

  async function batchSetBlacklisted(isBlacklisted: boolean): Promise<boolean> {
    const stats = selectionStats.value
    if (stats.empty) {
      MessagePlugin.warning(t('roles.messages.selectRequired'))
      return false
    }
    if (isBlacklisted && !stats.nonBlacklisted) {
      MessagePlugin.warning(t('roles.messages.selectInconsistent'))
      return false
    }
    if (!isBlacklisted && !stats.blacklisted) {
      MessagePlugin.warning(t('roles.messages.selectInconsistent'))
      return false
    }
    const targets = filterOutSelf(selectedIds.value)
    if (targets.length === 0) {
      MessagePlugin.warning(t('roles.messages.cannotOperateSelf'))
      return false
    }
    // 后端未提供批量 ban/unban 端点，逐个调用单接口；后续如提供批量端点可替换为单次调用。
    let success = 0
    let lastError = ''
    for (const id of targets) {
      try {
        if (isBlacklisted) {
          await banUser(id)
        } else {
          await unbanUser(id)
        }
        success += 1
      } catch (e: unknown) {
        lastError = extractErrorMessage(e)
      }
    }
    if (success > 0) {
      MessagePlugin.success(t('roles.messages.operationSuccess'))
      await reload()
    } else {
      MessagePlugin.error(t('roles.messages.operationFailed', { msg: lastError }))
    }
    return success > 0
  }

  async function batchSetKnowledgeOfficer(
    isOfficer: boolean,
    scope?: KnowledgeOfficerScope,
  ): Promise<boolean> {
    const stats = selectionStats.value
    if (stats.empty) {
      MessagePlugin.warning(t('roles.messages.selectRequired'))
      return false
    }
    if (isOfficer && !stats.nonKo) {
      MessagePlugin.warning(t('roles.messages.selectInconsistent'))
      return false
    }
    if (!isOfficer && !stats.ko) {
      MessagePlugin.warning(t('roles.messages.selectInconsistent'))
      return false
    }
    const targets = filterOutSelf(selectedIds.value)
    if (targets.length === 0) {
      MessagePlugin.warning(t('roles.messages.cannotOperateSelf'))
      return false
    }
    // 设为时 scope 必须存在（由批量知识官 dialog 提供）；取消时不需要范围。
    if (isOfficer && !scope) {
      MessagePlugin.warning(t('roles.messages.selectRequired'))
      return false
    }
    const payload: Parameters<typeof setKnowledgeOfficerRoles>[1] = {
      role_knowledge_officer: isOfficer ? 1 : 0,
    }
    if (isOfficer && scope) {
      payload.knowledge_domain_ids = scope.knowledgeDomainIds
      payload.knowledge_base_ids = scope.knowledgeBaseIds
    }
    try {
      await setKnowledgeOfficerRoles(targets, payload)
      MessagePlugin.success(t('roles.messages.operationSuccess'))
      await reload()
      return true
    } catch (e: unknown) {
      MessagePlugin.error(t('roles.messages.operationFailed', { msg: extractErrorMessage(e) }))
      return false
    }
  }

  // 翻页或改 pageSize 都会触发重新拉取；筛选条件变化由 applyFilter 主动 load。
  watch(
    () => [pagination.value.page, pagination.value.pageSize] as const,
    () => {
      void load()
    },
  )

  return {
    items,
    loading,
    error,
    filters,
    pagination,
    selectedIds,
    selectedRows,
    selectionStats,
    departmentOptions,
    filteredTotal,

    load,
    applyFilter,
    resetFilter,
    reload,

    toggleBlacklist,
    toggleOpsAdmin,
    toggleKnowledgeOfficer,

    batchSetOperationsAdmin,
    batchSetBlacklisted,
    batchSetKnowledgeOfficer,
  }
}