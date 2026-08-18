import { del, get, post, put } from '@/utils/request'

export type GrantSubjectType = 'user' | 'org_unit'
export type KnowledgeResourceType = 'knowledge_base' | 'folder' | 'knowledge'
export type KnowledgePermission = 'read' | 'manage'
export type GrantEffect = 'allow' | 'deny'

export interface KnowledgeAccessResource {
  type: KnowledgeResourceType
  id: string
  name?: string
}

export interface OrgUnit {
  id: string
  parent_id?: string
  code: string
  name: string
  status: 'active' | 'inactive'
  source: 'manual' | 'workday' | 'bootstrap'
  sort_order: number
  attributes?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface GrantUser {
  id: string
  email: string
  username: string
  avatar?: string
  is_active: boolean
}

export interface OrgUnitMember {
  user_id: string
  email: string
  username: string
  avatar?: string
  org_unit_id: string
  is_primary: boolean
  status: 'active' | 'inactive'
  source: 'manual' | 'workday' | 'bootstrap'
  membership_id: number
}

export interface UserOrgMembership {
  id?: number
  user_id?: string
  org_unit_id: string
  is_primary: boolean
  status?: 'active' | 'inactive'
  source?: 'manual' | 'workday' | 'bootstrap'
}

export interface KnowledgeResourceGrant {
  id: number
  knowledge_domain_id: number
  knowledge_base_id: string
  resource_type: KnowledgeResourceType
  resource_id: string
  resource_name?: string
  resource_path?: string
  subject_type: GrantSubjectType
  subject_id: string
  subject_name: string
  subject_detail?: string
  permission: KnowledgePermission
  effect: GrantEffect
  inherit_to_children: boolean
  granted_by?: string
  created_at: string
  updated_at: string
}

export interface KnowledgeResourceGrantInput {
  subject_type: GrantSubjectType
  subject_id: string
  permission: KnowledgePermission
  effect: GrantEffect
  inherit_to_children?: boolean
}

export interface KnowledgeGrantSubjects {
  org_units: OrgUnit[]
  users: GrantUser[]
}

interface ListResponse<T> {
  success: boolean
  data?: T
  message?: string
}

interface SimpleResponse {
  success: boolean
  message?: string
}

export async function listKnowledgeResourceGrants(
  knowledgeBaseId: string,
  resourceType: KnowledgeResourceType,
  resourceId: string,
): Promise<ListResponse<KnowledgeResourceGrant[]>> {
  return await get(
    `/api/v1/knowledge-bases/${knowledgeBaseId}/resources/${resourceType}/${resourceId}/grants`,
  ) as unknown as ListResponse<KnowledgeResourceGrant[]>
}

export async function listKnowledgeResourceGrantSubjects(
  knowledgeBaseId: string,
  resourceType: KnowledgeResourceType,
  resourceId: string,
  search = '',
  limit = 100,
): Promise<ListResponse<KnowledgeGrantSubjects>> {
  const query = new URLSearchParams({
    search,
    limit: String(limit),
  })
  return await get(
    `/api/v1/knowledge-bases/${knowledgeBaseId}/resources/${resourceType}/${resourceId}/grant-subjects?${query}`,
  ) as unknown as ListResponse<KnowledgeGrantSubjects>
}

export async function upsertKnowledgeResourceGrant(
  knowledgeBaseId: string,
  resourceType: KnowledgeResourceType,
  resourceId: string,
  input: KnowledgeResourceGrantInput,
): Promise<SimpleResponse> {
  return await put(
    `/api/v1/knowledge-bases/${knowledgeBaseId}/resources/${resourceType}/${resourceId}/grants`,
    input,
  ) as unknown as SimpleResponse
}

export async function revokeKnowledgeResourceGrant(
  knowledgeBaseId: string,
  grantId: number,
): Promise<SimpleResponse> {
  return await del(
    `/api/v1/knowledge-bases/${knowledgeBaseId}/resource-grants/${grantId}`,
  ) as unknown as SimpleResponse
}

export async function listOrgUnits(): Promise<ListResponse<OrgUnit[]>> {
  return await get('/api/v1/enterprise/org-units') as unknown as ListResponse<OrgUnit[]>
}

export async function searchGrantUsers(
  search = '',
  limit = 100,
): Promise<ListResponse<GrantUser[]>> {
  const query = new URLSearchParams({
    search,
    limit: String(limit),
  })
  return await get(`/api/v1/enterprise/users?${query}`) as unknown as ListResponse<GrantUser[]>
}

export async function listOrgUnitMembers(
  orgUnitId: string,
): Promise<ListResponse<OrgUnitMember[]>> {
  return await get(
    `/api/v1/enterprise/org-units/${orgUnitId}/members`,
  ) as unknown as ListResponse<OrgUnitMember[]>
}

export async function listUserOrgMemberships(
  userId: string,
): Promise<ListResponse<UserOrgMembership[]>> {
  return await get(
    `/api/v1/enterprise/users/${userId}/org-memberships`,
  ) as unknown as ListResponse<UserOrgMembership[]>
}

export async function replaceUserOrgMemberships(
  userId: string,
  memberships: Array<Pick<UserOrgMembership, 'org_unit_id' | 'is_primary'>>,
): Promise<SimpleResponse> {
  return await put(`/api/v1/enterprise/users/${userId}/org-memberships`, {
    memberships,
  }) as unknown as SimpleResponse
}

export async function createOrgUnit(
  unit: Pick<OrgUnit, 'code' | 'name'> & Partial<Pick<OrgUnit, 'parent_id' | 'status' | 'sort_order' | 'attributes'>>,
): Promise<ListResponse<OrgUnit>> {
  return await post('/api/v1/enterprise/org-units', unit) as unknown as ListResponse<OrgUnit>
}

export async function updateOrgUnit(
  orgUnitId: string,
  unit: Pick<OrgUnit, 'code' | 'name'> & Partial<Pick<OrgUnit, 'parent_id' | 'status' | 'sort_order' | 'attributes'>>,
): Promise<SimpleResponse> {
  return await put(`/api/v1/enterprise/org-units/${orgUnitId}`, unit) as unknown as SimpleResponse
}

export async function deleteOrgUnit(orgUnitId: string): Promise<SimpleResponse> {
  return await del(`/api/v1/enterprise/org-units/${orgUnitId}`) as unknown as SimpleResponse
}
