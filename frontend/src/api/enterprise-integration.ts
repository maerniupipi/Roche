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

interface WorkdaySyncResponse {
  success: boolean
  data?: WorkdaySyncRun
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
