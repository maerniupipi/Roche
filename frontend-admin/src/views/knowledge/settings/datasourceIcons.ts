import feishuIcon from '@/assets/img/datasource-feishu.ico'
import notionIcon from '@/assets/img/datasource-notion.ico'
import googleDriveIcon from '@/assets/img/datasource-google-drive.svg'
import yuqueIcon from '@/assets/img/datasource-yuque.ico'
import rssIcon from '@/assets/img/datasource-rss.svg'

export const datasourceIconMap: Record<string, string> = {
  feishu: feishuIcon,
  notion: notionIcon,
  google_drive: googleDriveIcon,
  yuque: yuqueIcon,
  rss: rssIcon,
}

export function getDatasourceIconUrl(type: string): string | undefined {
  return datasourceIconMap[type]
}
