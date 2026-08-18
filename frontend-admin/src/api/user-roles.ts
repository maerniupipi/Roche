// 用户角色配置模块的 API 数据访问层，对接后端 /api/v1/system/admin/users/*。
import { get, post, put } from '@/utils/request'
import type {
  UserRole,
  UserRoleFilters,
  UserRolePagination,
} from '@/types/userRole'

const BASE = '/api/v1/system/admin/users'

// 后端响应里可能给 string[] / number[] / 缺失 / null，统一归一为 string[]，
// 方便 UI 渲染与选择器回填（用户列表返回的 id 形如 "u-123"）。
function toStringArray(raw: unknown): string[] {
  if (!Array.isArray(raw)) return []
  return raw
    .map((x) => (typeof x === 'string' ? x : typeof x === 'number' ? String(x) : ''))
    .filter((s) => s.length > 0)
}

// 把后端 wire 字段（snake_case）映射到前端 UserRole（camelCase）。
// 这里集中处理"列表返回时可能携带但需要转名"的字段，避免在组件里到处写 if。
function mapWireToUserRole(raw: UserRole): UserRole {
  const snake = raw as unknown as {
    knowledge_domain_ids?: unknown
    knowledge_base_ids?: unknown
  }
  return {
    ...raw,
    knowledgeDomainIds: toStringArray(snake.knowledge_domain_ids),
    knowledgeBaseIds: toStringArray(snake.knowledge_base_ids),
  }
}

export interface ListUserRolesParams {
  filters: UserRoleFilters
  pagination: UserRolePagination
}

export interface ListUserRolesResult {
  items: UserRole[]
  total: number
}

// 列表查询：把过滤项里非空的值透传到 query string；'true'/'false' 用字面量。
// 角色与状态过滤项 allowEmpty 模式下空串代表"未选择"，这里用 if 直接跳过。
function buildListQuery(
  filters: UserRoleFilters,
  pagination: UserRolePagination,
): string {
  const params = new URLSearchParams()
  params.set('offset', String((pagination.page - 1) * pagination.pageSize))
  params.set('limit', String(pagination.pageSize))
  if (filters.query) params.set('query', filters.query)
  if (filters.account) params.set('account', filters.account)
  if (filters.employee_name) params.set('employee_name', filters.employee_name)
  if (filters.is_knowledge_officer) params.set('is_knowledge_officer', filters.is_knowledge_officer)
  if (filters.is_operator) params.set('is_operator', filters.is_operator)
  if (filters.department) params.set('department', filters.department)
  if (filters.status) params.set('status', filters.status)
  return params.toString()
}

/** 列表查询用户 */
export async function listUserRoles(
  params: ListUserRolesParams,
): Promise<ListUserRolesResult> {
  const qs = buildListQuery(params.filters, params.pagination)
  const url = qs ? `${BASE}?${qs}` : BASE
  const response = await get<{ total: number; users: UserRole[] }>(url)
  return {
    items: (response.users ?? []).map(mapWireToUserRole),
    total: response.total ?? 0,
  }
}

/** 创建用户：单字段 payload（user_id 或 email 二选一）。 */
export interface CreateUserPayload {
  user_id?: string
  email?: string
}

export async function createUser(payload: CreateUserPayload): Promise<void> {
  await post(BASE, payload)
}

/** 封禁/取消封禁：单字段 user_id。 */
export async function banUser(userId: string): Promise<void> {
  await post(`${BASE}/ban`, { user_id: userId })
}

export async function unbanUser(userId: string): Promise<void> {
  await post(`${BASE}/unban`, { user_id: userId })
}

/** 运维员角色：role_operator 0|1（0=取消，1=设为）。 */
export interface SetOperatorRolePayload {
  role_operator: 0 | 1
}

export async function setOperatorRolesSingle(
  userId: string,
  payload: SetOperatorRolePayload,
): Promise<void> {
  await put(`${BASE}/roles/operator/single`, { user_id: userId, ...payload })
}

export async function setOperatorRoles(
  userIds: string[],
  payload: SetOperatorRolePayload,
): Promise<void> {
  await put(`${BASE}/roles/operator`, { user_ids: userIds, ...payload })
}

/** 知识官角色与范围：role_knowledge_officer 0|1 + 知识域/知识库范围（仅在 role_knowledge_officer=1 时使用）。 */
export interface KnowledgeOfficerScopePayload {
  role_knowledge_officer: 0 | 1
  knowledge_domain_ids?: string[]
  knowledge_base_ids?: string[]
}

export async function setKnowledgeOfficerRolesSingle(
  userId: string,
  payload: KnowledgeOfficerScopePayload,
): Promise<void> {
  await put(`${BASE}/roles/single`, { user_id: userId, ...payload })
}

export async function setKnowledgeOfficerRoles(
  userIds: string[],
  payload: KnowledgeOfficerScopePayload,
): Promise<void> {
  await put(`${BASE}/roles`, { user_ids: userIds, ...payload })
}
