// Knowledge domain mock — 与 menu.ts 配合使用。
// 真实域由后端 `GET /api/v1/knowledge-domains` 下发。

import type { MenuNode } from '@/types/menu'

export const mockDomains: MenuNode[] = [
  {
    id: '1',
    title: '财务',
    titleEn: 'Finance',
    order: 10,
    visible: true,
  },
  {
    id: '2',
    title: '合规',
    titleEn: 'Compliance',
    order: 20,
    visible: true,
  },
  // {
  //   id: 'domain-hr',
  //   title: 'HR',
  //   titleEn: 'HR',
  //   order: 30,
  //   visible: true,
  // },
  // {
  //   id: 'domain-legal',
  //   title: '法务',
  //   titleEn: 'Legal',
  //   order: 40,
  //   visible: true,
  // },
]
