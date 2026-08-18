// 用户角色配置模块 — 与后端 /api/v1/system/admin/users 响应 UserInfo 对齐。
// 字段命名同时考虑后端 wire 字段（snake_case 的 role_* 系列）和前端语义（is_*）。
// 这里保留前端 isXxx 形式以便消费方用惯用的 boolean 写法；
// 真正提交到后端时由 API 层在装/拆 payload 时做转换。

export type UserRoleStatus = 'active' | 'blacklisted' | 'inactive'

// 后端状态码如下，前端只在过滤项下拉中允许三种值的透传：
//   0 → active（正常）
//   1 → blacklisted（已拉黑/封禁）
//   2 → inactive（未激活/停用）
export const USER_ROLE_STATUS_VALUES = {
  active: 0,
  blacklisted: 1,
  inactive: 2,
} as const

export interface UserRole {
  // 身份与账号
  id: string
  employeeId: string
  account: string
  username: string
  englishName: string
  chineseName: string
  email: string

  // 状态（0|1|2）；banned_at / banned_by 仅在 status === 1 时有值
  status: UserRoleStatus
  bannedAt?: string | null
  bannedBy?: string | null

  // 角色标记
  isKnowledgeOfficer: boolean
  isKnowledgeDomainAdmin: boolean
  isSystemAdmin: boolean
  isOperationsAdmin: boolean

  // 知识官管理范围（仅 isKnowledgeOfficer=true 时由后端返回；前端拼成 camelCase 数组）
  knowledgeDomainIds?: string[]
  knowledgeBaseIds?: string[]

  // 部门
  departmentName: string
  departmentId: string
  departmentCode?: string

  // 元数据
  preferences?: Record<string, unknown>
  createdAt?: string
  updatedAt?: string
}

// 列表查询过滤条件；字段名与后端 query 参数一致，方便 API 层直接透传。
// 列表下拉用空串表示"未选择"，传递时不附加到 query。
export interface UserRoleFilters {
  query: string
  account: string
  employee_name: string
  is_knowledge_officer: '' | 'true' | 'false'
  is_operator: '' | 'true' | 'false'
  department: string
  status: '' | UserRoleStatus
}

export interface UserRolePagination {
  page: number
  pageSize: number
  total: number
}

export const DEFAULT_USER_ROLE_FILTERS: UserRoleFilters = {
  query: '',
  account: '',
  employee_name: '',
  is_knowledge_officer: '',
  is_operator: '',
  department: '',
  status: '',
}

export const DEFAULT_USER_ROLE_PAGINATION: UserRolePagination = {
  page: 1,
  pageSize: 10,
  total: 0,
}

// 部门下拉项：用户管理场景下需要的不只是 id/name，还要带 code 用于设置时回传。
export interface DepartmentOption {
  id: string
  name: string
  code?: string
}
