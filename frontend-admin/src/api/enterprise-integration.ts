import { get, post } from '@/utils/request'

export type WorkdaySyncStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export interface WorkdaySyncRun {
  id: string
  provider: string
  connection_key: string
  mode: 'full' | 'incremental'
  status: WorkdaySyncStatus
  counters?: Record<string, number>
  error_code?: string
  error_summary?: string
  started_at?: string
  finished_at?: string
  created_at: string
}

export interface WorkdayOrgUnit {
  id: string
  external_org_id: string
  parent_external_org_id?: string
  org_unit_id?: string
  name: string
  org_type?: string
  status: 'active' | 'inactive'
  attributes?: Record<string, unknown>
  last_seen_at: string
}

export interface WorkdayWorker {
  id: string
  external_worker_id: string
  user_id?: string
  primary_org_external_id?: string
  manager_external_worker_id?: string
  corporate_email?: string
  worker_status: 'active' | 'inactive' | 'leave'
  attributes?: Record<string, unknown>
  last_seen_at: string
}

interface WorkdaySyncResponse {
  success: boolean
  data?: WorkdaySyncRun
  message?: string
}

interface WorkdayOrgUnitResponse {
  success: boolean
  data?: WorkdayOrgUnit[]
  message?: string
}

interface WorkdayWorkerResponse {
  success: boolean
  data?: WorkdayWorker[]
  total?: number
  message?: string
}

export async function triggerWorkdayFullSync(): Promise<WorkdaySyncResponse> {
  return await post('/api/v1/system/admin/integrations/workday/sync', {
    mode: 'full',
  }) as unknown as WorkdaySyncResponse
}

export async function getWorkdaySyncRun(runId: string): Promise<WorkdaySyncResponse> {
  return await get(
    `/api/v1/system/admin/integrations/workday/runs/${encodeURIComponent(runId)}`,
  ) as unknown as WorkdaySyncResponse
}

export async function listWorkdayOrgUnits(): Promise<WorkdayOrgUnitResponse> {
  return await get(
    '/api/v1/system/admin/integrations/workday/directory/org-units',
  ) as unknown as WorkdayOrgUnitResponse
}

export async function listWorkdayWorkers(
  search = '',
  limit = 1000,
  offset = 0,
): Promise<WorkdayWorkerResponse> {
  const query = new URLSearchParams({
    search,
    offset: String(offset),
    limit: String(limit),
  })
  return await get(
    `/api/v1/system/admin/integrations/workday/directory/workers?${query}`,
  ) as unknown as WorkdayWorkerResponse
}
