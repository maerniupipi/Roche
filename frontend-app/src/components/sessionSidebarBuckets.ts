import type { SessionForGrouping } from './sessionGrouping'

export const SIDEBAR_BUCKET_PAGE_SIZE = 30

export interface SidebarSessionBucket {
  key: 'web'
  label: string
  page: number
  total: number
  items: SessionForGrouping[]
  loaded: boolean
  loading: boolean
}

export interface BucketDefinition {
  key: 'web'
  label: string
}

export function createEmptyBucket(def: BucketDefinition): SidebarSessionBucket {
  return { ...def, page: 0, total: 0, items: [], loaded: false, loading: false }
}

export function bucketHasMore(bucket: SidebarSessionBucket): boolean {
  return bucket.items.length < bucket.total
}

export function buildBucketDefinitions(label: string): BucketDefinition[] {
  return [{ key: 'web', label }]
}

export function mergeBucketPage(
  bucket: SidebarSessionBucket,
  rows: SessionForGrouping[],
  total: number,
  page: number,
): SidebarSessionBucket {
  const seen = new Set(bucket.items.map((session) => session.id))
  const merged = [...bucket.items]
  for (const row of rows) {
    if (seen.has(row.id)) continue
    seen.add(row.id)
    merged.push(row)
  }
  return { ...bucket, page, total, items: merged, loaded: true, loading: false }
}

export function flattenBucketItems(
  buckets: Record<string, SidebarSessionBucket>,
  order: string[],
): SessionForGrouping[] {
  const out: SessionForGrouping[] = []
  const seen = new Set<string>()
  for (const key of order) {
    const bucket = buckets[key]
    if (!bucket) continue
    for (const item of bucket.items) {
      if (seen.has(item.id)) continue
      seen.add(item.id)
      out.push(item)
    }
  }
  return out
}

export function prependSessionToWebBucket(
  bucket: SidebarSessionBucket,
  session: SessionForGrouping,
): SidebarSessionBucket {
  if (bucket.items.some((row) => row.id === session.id)) return bucket
  return { ...bucket, items: [session, ...bucket.items], total: bucket.total + 1, loaded: true }
}

export function removeSessionFromBuckets(
  buckets: Record<string, SidebarSessionBucket>,
  sessionId: string,
): Record<string, SidebarSessionBucket> {
  const next: Record<string, SidebarSessionBucket> = {}
  for (const [key, bucket] of Object.entries(buckets)) {
    const items = bucket.items.filter((session) => session.id !== sessionId)
    next[key] = {
      ...bucket,
      items,
      total: Math.max(0, bucket.total - (items.length < bucket.items.length ? 1 : 0)),
    }
  }
  return next
}
