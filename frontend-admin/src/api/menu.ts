// menu API — 默认调 `GET /api/v1/menu` 真实接口。
//
// 响应结构（见 docs/api/menu-api-contract-2026-08-12.md v1.1）：
//   { success: true, data: { tree: MenuNode[] }, error: null }
//
// 响应拦截器已 unwrap axios 响应，所以 fetchMenu 拿到的 response 就是后端响应体。

import { get } from '@/utils/request'
import type { MenuNode } from '@/types/menu'

/**
 * 拉取当前用户的菜单树。
 *
 * 真实接口 `GET /api/v1/menu` 响应结构为 `{ success, data: { tree }, error }`，
 * 通过 `data.tree` 拿到 `MenuNode[]`。接口失败时返回空数组（前端菜单降级为空）。
 */
export async function fetchMenu(): Promise<MenuNode[]> {
  try {
    // 响应拦截器已 unwrap axios 响应，所以 response 就是后端的响应体：
    //   { success, data: { tree }, error } —— 详见接口契约 §1.3。
    const response = await get('/api/v1/menu')
    const tree = (response as any)?.data?.tree
    return Array.isArray(tree) ? (tree as MenuNode[]) : []
  } catch (err) {
    console.warn('[menu] fetch failed, falling back to empty list:', err)
    return []
  }
}
