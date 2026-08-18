import { get, post, put } from '@/utils/request'
import i18n from '@/i18n'

const t = (key: string) => i18n.global.t(key)

// 知识域信息接口
export interface KnowledgeDomainInfo {
  id: number
  code: string
  name: string
  description?: string
  status?: string
  storage_quota?: number
  storage_used?: number
  created_at: string
  updated_at: string
}

// 搜索知识域参数
export interface SearchKnowledgeDomainsParams {
  keyword?: string
  knowledge_domain_id?: number
  page?: number
  page_size?: number
}

// 搜索知识域响应
export interface SearchKnowledgeDomainsResponse {
  success: boolean
  data?: {
    items: KnowledgeDomainInfo[]
    total: number
    page: number
    page_size: number
  }
  message?: string
}

export async function listKnowledgeDomains(): Promise<{
  success: boolean
  data?: { items: KnowledgeDomainInfo[] }
  message?: string
}> {
  try {
    return await get('/api/v1/knowledge-domains') as unknown as {
      success: boolean
      data?: { items: KnowledgeDomainInfo[] }
      message?: string
    }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.knowledgeDomain.listFailed'),
    }
  }
}

/**
 * 获取所有知识域列表（仅系统管理员）
 * @deprecated 建议使用 searchKnowledgeDomains 代替，支持分页和搜索
 */
export async function listAllKnowledgeDomains(): Promise<{ success: boolean; data?: { items: KnowledgeDomainInfo[] }; message?: string }> {
  try {
    const response = await get('/api/v1/knowledge-domains/all')
    return response as unknown as { success: boolean; data?: { items: KnowledgeDomainInfo[] }; message?: string }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.knowledgeDomain.listFailed')
    }
  }
}

/**
 * 更新知识域信息（目前暴露名称、描述两个字段的编辑入口）。
 * 后端 `PUT /knowledge-domains/:id` 用指针字段区分"未传"和"显式空串"，未传的列不会
 * 被改动；这里也按需选择性传 `name` / `description`，互不影响。
 * 权限：knowledgeDomain admin。
 */
export async function updateKnowledgeDomain(
  knowledgeDomainId: number,
  payload: { name?: string; description?: string },
): Promise<{ success: boolean; data?: KnowledgeDomainInfo; message?: string }> {
  try {
    const response = await put(`/api/v1/knowledge-domains/${knowledgeDomainId}`, payload)
    return response as unknown as { success: boolean; data?: KnowledgeDomainInfo; message?: string }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.knowledgeDomain.updateFailed'),
    }
  }
}

/**
 * 创建知识域。仅系统管理员可调用。
 */
export async function createKnowledgeDomain(
  payload: { name: string; description?: string },
): Promise<{ success: boolean; data?: KnowledgeDomainInfo; message?: string }> {
  try {
    const response = await post('/api/v1/knowledge-domains', payload)
    return response as unknown as { success: boolean; data?: KnowledgeDomainInfo; message?: string }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.knowledgeDomain.createFailed'),
    }
  }
}

/**
 * 搜索知识域（支持分页、关键词搜索和知识域 ID 过滤）
 */
export async function searchKnowledgeDomains(params: SearchKnowledgeDomainsParams = {}): Promise<SearchKnowledgeDomainsResponse> {
  try {
    const queryParams = new URLSearchParams()
    if (params.keyword) {
      queryParams.append('keyword', params.keyword)
    }
    if (params.knowledge_domain_id) {
      queryParams.append('knowledge_domain_id', String(params.knowledge_domain_id))
    }
    if (params.page) {
      queryParams.append('page', String(params.page))
    }
    if (params.page_size) {
      queryParams.append('page_size', String(params.page_size))
    }
    const queryString = queryParams.toString()
    const url = `/api/v1/knowledge-domains/search${queryString ? '?' + queryString : ''}`
    const response = await get(url)
    return response as unknown as SearchKnowledgeDomainsResponse
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.knowledgeDomain.searchFailed')
    }
  }
}
