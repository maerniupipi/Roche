// MenuNode — 单个菜单节点（同时支持一级和二级）。
//
// 后端在 API 契约 v1.1 里规定所有 menu 节点必须同时返回：
//   - title   中文标题（默认/兜底）
//   - titleEn 英文标题（切换英文环境时使用）
//   - icon    前端 SVG 文件名（不含路径），树形 SVG 资源集中在 src/assets/img/
//   - path    前端路由路径（叶节点必须，二级分组可为空）
//   - order   排序权重（数字越小越靠前）
//   - visible 是否对当前用户可见（false → 跳过）
//   - meta    任意附加元数据（透传给前端）
//   - children  二级节点（KB 之类的二级分组用 children，平台级扁平节点无 children）
//
// 字段含义详见 `docs/api/menu-api-contract-2026-08-12.md`。

export interface MenuNode {
  /** 节点唯一 ID（前端用做 key；后端可视为 path 的派生 id）。 */
  id?: string
  /** 中文标题（默认 / 兜底文案）。 */
  title: string
  /** 英文标题（英文环境使用）。 */
  titleEn: string
  /** 前端 SVG 文件名（不含路径和后缀），如 "dashboard"。 */
  icon?: string
  /** 前端路由路径，叶节点必填，二级分组节点可为空。 */
  path?: string
  /** 排序权重，数字越小越靠前。 */
  order: number
  /** 是否对当前用户可见。 */
  visible: boolean
  /** 任意附加元数据（透传）。 */
  meta?: Record<string, unknown>
  /** 二级子节点。一级平台级菜单为空数组。 */
  children?: MenuNode[]
}
