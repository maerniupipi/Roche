import { get, post, del } from '@/utils/request'

export type FavoriteResourceType = 'kb' | 'agent'

export interface FavoriteEntry {
  user_id: string
  resource_type: FavoriteResourceType
  resource_id: string
  created_at: string
}

export function listFavorites(type: FavoriteResourceType) {
  return get<{ success: boolean; data: FavoriteEntry[] }>(
    `/api/v1/user/favorites?type=${encodeURIComponent(type)}`
  )
}

export function addFavorite(type: FavoriteResourceType, id: string) {
  return post('/api/v1/user/favorites', { type, id })
}

export function removeFavorite(type: FavoriteResourceType, id: string) {
  return del(`/api/v1/user/favorites/${encodeURIComponent(type)}/${encodeURIComponent(id)}`)
}
