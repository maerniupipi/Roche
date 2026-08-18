import { post, get, put } from '@/utils/request'
import i18n from '@/i18n'

const t = (key: string) => i18n.global.t(key)

interface APIEnvelope<T> {
  code: number
  msg?: string
  data: T
}

function unwrapAuthResponse<T>(response: T | APIEnvelope<T>): T {
  const candidate = response as APIEnvelope<T>
  if (
    candidate
    && typeof candidate === 'object'
    && typeof candidate.code === 'number'
    && Object.prototype.hasOwnProperty.call(candidate, 'data')
  ) {
    if (candidate.code !== 0) {
      throw new Error(candidate.msg || 'Authentication request failed')
    }
    return candidate.data
  }
  return response as T
}

// 用户登录接口
export interface LoginRequest {
  email: string
  password: string
}

export interface LoginResponse {
  success: boolean
  message?: string
  user?: {
    id: string
    username: string
    email: string
    avatar?: string
    is_system_admin?: boolean
    is_knowledge_domain_admin?: boolean
    is_active: boolean
    created_at: string
    updated_at: string
  }
  token?: string
}

export interface SAMLAuthURLResponse {
  success: boolean
  authorization_url?: string
  relay_state?: string
  message?: string
}

export interface SAMLConfigResponse {
  success: boolean
  enabled: boolean
  provider_display_name?: string
  message?: string
}

export type RegistrationRole = 'viewer' | 'system_admin'

export interface RegisterRequest {
  username: string
  email: string
  password: string
  role?: RegistrationRole
}

export interface RegisterResponse {
  success: boolean
  message?: string
  user?: LoginResponse['user']
}

export interface RegistrationConfigResponse {
  success: boolean
  enabled: boolean
  role_selection_enabled: boolean
  default_role: RegistrationRole
  roles: RegistrationRole[]
  message?: string
}

// 用户偏好（与后端 types.UserPreferences 对齐，字段可选 = 没显式设置过）。
// 新加 key 时记得：后端 service.UpdateUserPreferences 也要在 merge 分支里
// 处理；前端调用方按需读 / 默认值降级。
export interface UserPreferences {
  enable_memory?: boolean
}

// 用户信息接口
export interface UserInfo {
  id: string
  username: string
  email: string
  avatar?: string
  preferences?: UserPreferences
  is_system_admin?: boolean
  is_knowledge_domain_admin?: boolean
  created_at: string
  updated_at: string
}

/**
 * 把后端返回的 user JSON 规范化成前端 UserInfo。
 *
 * 历史上有 4 处独立的 setUser 调用（Login、autoSetup、token rehydrate、
 * /auth/me 主动 refresh）各自手写字段白名单，每加一个 user 字段都要在
 * 4 处同步——否则该字段就被悄悄过滤掉。is_system_admin 上线时就因为
 * 漏拷一处而看不到「系统管理」入口；这个工厂存在的目的就是杜绝同类
 * 漏拷再发生。**新增 user 字段请只改这里**。
 *
 * 字段读取统一走 `=== true` 而不是 `|| false`，对偶发非 boolean
 * 类型（后端某天传 1/0 或字符串）做严格收敛，避免把 truthy 字符串
 * 误判为权限通过。
 */
export function userInfoFromApi(u: any): UserInfo {
  return {
    id: u?.id || '',
    username: u?.username || '',
    email: u?.email || '',
    avatar: u?.avatar,
    is_system_admin: u?.is_system_admin === true,
    is_knowledge_domain_admin: u?.is_knowledge_domain_admin === true,
    preferences: u?.preferences,
    created_at: u?.created_at || new Date().toISOString(),
    updated_at: u?.updated_at || new Date().toISOString(),
  }
}

// 知识管理域信息接口
export interface KnowledgeDomainInfo {
  id: string
  code?: string
  name: string
  description?: string
  status?: string
  storage_quota?: number
  storage_used?: number
  created_at: string
  updated_at: string
  knowledge_bases?: KnowledgeBaseInfo[]
}

// 知识库信息接口
export interface KnowledgeBaseInfo {
  id: string
  name: string
  description: string
  knowledge_domain_id: string
  // creator_id records who originally created the KB; it is not an ACL.
  // Nullable for rows created before creator tracking was introduced.
  creator_id?: string
  // creator_name 由后端 list 接口批量回填（username 优先，退化到 email），
  // 仅用于列表卡片来源徽章；缺失代表无法解析（已删除 / 老数据）。
  creator_name?: string
  created_at: string
  updated_at: string
  document_count?: number
  chunk_count?: number
}

// 模型信息接口
export interface ModelInfo {
  id: string
  name: string
  type: string
  source: string
  description?: string
  is_default?: boolean
  created_at: string
  updated_at: string
}

/**
 * 用户登录
 */
export async function login(data: LoginRequest): Promise<LoginResponse> {
  try {
    const response = await post('/api/v1/auth/login', data)
    return unwrapAuthResponse<LoginResponse>(response)
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.loginFailed')
    }
  }
}

export async function register(data: RegisterRequest): Promise<RegisterResponse> {
  try {
    const response = await post('/api/v1/auth/register', data)
    const user = unwrapAuthResponse<LoginResponse['user']>(response)
    return { success: true, user }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('auth.registerFailed')
    }
  }
}

export async function getRegistrationConfig(): Promise<RegistrationConfigResponse> {
  try {
    const response = await get('/api/v1/auth/registration/config')
    return unwrapAuthResponse<RegistrationConfigResponse>(response)
  } catch (error: any) {
    return {
      success: false,
      enabled: false,
      role_selection_enabled: false,
      default_role: 'viewer',
      roles: [],
      message: error.message || t('auth.registerFailed')
    }
  }
}

/**
 * 获取 SAML SP 发起登录所需的 IdP 跳转地址。
 */
export async function getSAMLAuthorizationURL(
  redirectURI: string,
): Promise<SAMLAuthURLResponse> {
  try {
    const params = new URLSearchParams({ redirect_uri: redirectURI })
    const response = await get(`/api/v1/auth/saml/url?${params.toString()}`)
    return unwrapAuthResponse<SAMLAuthURLResponse>(response)
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.loginFailed'),
    }
  }
}

/**
 * 获取当前 Auth Service 的 SAML 登录开关和显示名称。
 */
export async function getSAMLConfig(): Promise<SAMLConfigResponse> {
  try {
    const response = await get('/api/v1/auth/saml/config')
    const config = unwrapAuthResponse<Omit<SAMLConfigResponse, 'success'>>(response)
    return { success: true, ...config }
  } catch (error: any) {
    return {
      success: false,
      enabled: false,
      message: error.message || t('error.auth.loginFailed'),
    }
  }
}

/**
 * 获取当前用户信息
 */
export async function getCurrentUser(): Promise<{ success: boolean; data?: { user: UserInfo }; message?: string }> {
  try {
    const response = await get('/api/v1/auth/me')
    const data = unwrapAuthResponse<{ user: UserInfo }>(response)
    return { success: true, data }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.getUserFailed')
    }
  }
}

/**
 * 更新当前用户的偏好设置（PATCH 语义：只发要改的字段，后端只覆盖发了的 key，
 * 其它 key 保持不变）。后端会返回更新后的完整 preferences 对象。
 */
export async function updateMyPreferences(
  patch: Partial<UserPreferences>,
): Promise<{ success: boolean; data?: UserPreferences; message?: string }> {
  try {
    const response = await put('/api/v1/auth/me/preferences', patch)
    const data = unwrapAuthResponse<UserPreferences>(response)
    return { success: true, data }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.updatePreferencesFailed'),
    }
  }
}

/**
 * 刷新Token
 */
export async function refreshToken(): Promise<{ success: boolean; data?: { token: string }; message?: string }> {
  try {
    const response = await post('/api/v1/auth/refresh', {})
    const data = unwrapAuthResponse<{ access_token?: string }>(response)
    if (data.access_token) {
      return {
        success: true,
        data: { token: data.access_token },
      }
    }

    return {
      success: false,
      message: t('error.auth.refreshTokenFailed'),
    }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.refreshTokenFailed')
    }
  }
}

/**
 * 用户登出
 */
export async function logout(): Promise<{ success: boolean; message?: string }> {
  try {
    await post('/api/v1/auth/logout', {})
    return {
      success: true
    }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.logoutFailed')
    }
  }
}

/**
 * 验证Token有效性
 */
export async function validateToken(): Promise<{ success: boolean; valid?: boolean; message?: string }> {
  try {
    const response = await get('/api/v1/auth/validate')
    const data = unwrapAuthResponse<{ valid?: boolean }>(response)
    return { success: true, ...data }
  } catch (error: any) {
    return {
      success: false,
      valid: false,
      message: error.message || t('error.auth.validateTokenFailed')
    }
  }
}
