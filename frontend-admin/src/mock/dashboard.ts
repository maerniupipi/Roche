// 仪表盘静态页面 mock 数据。
// 数据完全按照 Figma 设计稿（node-id=254-76）提供，便于页面结构验证。
// 等待后端 `/api/v1/dashboard/*` 接口就位后由真实数据替换。

// ===== 顶部区块：6 个知识库 KPI =====
export interface KbKpiItem {
  key: 'published' | 'uploaded' | 'failed' | 'scheduled' | 'delisted' | 'archived'
  title: string
  value: number
  unit: string
  iconText: string
  iconBg: string
  iconColor: string
  bgColor: string
  subtitle?: string
}

// ===== 中部区块：6 个问答基本数据 =====
export interface BasicMetricItem {
  key: 'totalQa' | 'askerCount' | 'satisfaction' | 'startAnswer' | 'completeAnswer' | 'avgAnswer'
  label: string
  primary: { label: string; value: string }
  secondary: { label: string; value: string }
}

// ===== 双柱状图每日趋势 =====
export interface TrendPoint {
  date: string
  value: number
}

export interface TrendSeries {
  name: string
  unit: 'count' | 'percent'
  color: string
  points: TrendPoint[]
}

// ===== 饼图 / 环图通用数据 =====
export interface PieDatum {
  name: string
  value: number
  color: string
}

// ===== 有效回答环图（带中心数字）=====
export interface EffectiveAnswerRing {
  total: number
  effectivePercent: number
  fallbackPercent: number
  effectiveColor: string
  fallbackColor: string
}

// ===== Top 10 用户 / 文档榜 =====
export interface TopEntry {
  rank: number
  name: string
  barValue: number // 用于进度条相对长度（像素或比例）
  isFirst?: boolean // 仅文档榜 #1 用强调色
}

// ===== 产品反馈横向条形图 =====
export interface FeedbackReason {
  label: string
  count: number
  highlight: string
}

// ===== 触发兜底话术问题 =====
export interface FallbackQuestion {
  id: string
  question: string
  department?: string
}

// ===== 仪表盘整体 mock =====
export interface DashboardMockData {
  kbKpis: KbKpiItem[]
  basicMetrics: BasicMetricItem[]
  monthlyTrend: TrendSeries
  satisfactionTrend: TrendSeries
  startAnswerTrend: TrendSeries
  domainDistribution: PieDatum[]
  crossDomainDistribution: PieDatum[]
  effectiveAnswerRing: EffectiveAnswerRing
  topUsers: TopEntry[]
  topDocs: TopEntry[]
  feedbackReasons: FeedbackReason[]
  fallbackQuestions: FallbackQuestion[]
  // 选择器默认值
  defaults: {
    dateRange: [string, string] // 近 30 天
    department: string
    knowledgeBase: string
  }
}

const build17DayTrend = (base: number, jitter: number, prefix: string): TrendPoint[] => {
  // 7 月 1 日 ~ 7 月 17 日 17 天，base+jitter 范围内的伪随机稳定序列
  const data: TrendPoint[] = []
  for (let i = 1; i <= 17; i += 1) {
    const day = String(i).padStart(2, '0')
    const v = Math.round(base + Math.sin(i * 0.7) * jitter)
    data.push({ date: `${prefix}-${day}`, value: Math.max(0, v) })
  }
  return data
}

export const mockDashboard: DashboardMockData = {
  kbKpis: [
    { key: 'published', title: '已上架文件数', value: 150, unit: '个', iconText: '架', iconBg: 'rgba(11,65,205,0.12)', iconColor: '#0B41CD', bgColor: 'rgba(11,65,205,0.06)' },
    { key: 'uploaded', title: '上传成功文件数', value: 12, unit: '个', iconText: '上', iconBg: 'rgba(0,124,107,0.12)', iconColor: '#007C6B', bgColor: 'rgba(0,124,107,0.06)' },
    { key: 'failed', title: '上传失败文件数', value: 0, unit: '个', iconText: '败', iconBg: 'rgba(204,0,51,0.12)', iconColor: '#CC0033', bgColor: 'rgba(204,0,51,0.06)' },
    { key: 'scheduled', title: '预约上架文件数', value: 30, unit: '个', iconText: '预', iconBg: 'rgba(188,54,240,0.12)', iconColor: '#BC36F0', bgColor: 'rgba(188,54,240,0.06)' },
    { key: 'delisted', title: '已下架文件数', value: 200, unit: '个', iconText: '下', iconBg: 'rgba(255,125,41,0.12)', iconColor: '#FF7D29', bgColor: 'rgba(255,125,41,0.06)' },
    { key: 'archived', title: '归档文件数', value: 248, unit: '个', iconText: '档', iconBg: 'rgba(88,99,128,0.12)', iconColor: '#586380', bgColor: 'rgba(88,99,128,0.06)' },
  ],

  basicMetrics: [
    {
      key: 'totalQa',
      label: '问答总数',
      primary: { label: '7 月提问数', value: '3,260' },
      secondary: { label: '8 月提问数', value: '1,860' },
    },
    {
      key: 'askerCount',
      label: '提问人数',
      primary: { label: '7 月', value: '820' },
      secondary: { label: '8 月', value: '480' },
    },
    {
      key: 'satisfaction',
      label: '对话满意度',
      primary: { label: '好评率', value: '92.3%' },
      secondary: { label: '差评率', value: '7.7%' },
    },
    {
      key: 'startAnswer',
      label: '开始回答',
      primary: { label: '本月', value: '3,210' },
      secondary: { label: '环比', value: '+12.4%' },
    },
    {
      key: 'completeAnswer',
      label: '完成回答',
      primary: { label: '本月', value: '3,180' },
      secondary: { label: '完成率', value: '99.1%' },
    },
    {
      key: 'avgAnswer',
      label: '平均回答',
      primary: { label: '平均时长', value: '2.4s' },
      secondary: { label: '平均 token', value: '486' },
    },
  ],

  monthlyTrend: {
    name: '7 月问答趋势',
    unit: 'count',
    color: '#0B41CD',
    points: build17DayTrend(1800, 600, '07'),
  },
  satisfactionTrend: {
    name: '对话满意度',
    unit: 'percent',
    color: '#0B41CD',
    points: build17DayTrend(85, 8, '07').map((p) => ({ date: p.date, value: Math.min(99, p.value / 10) })),
  },
  startAnswerTrend: {
    name: '开始回答',
    unit: 'percent',
    color: '#0B41CD',
    points: build17DayTrend(75, 12, '07').map((p) => ({ date: p.date, value: Math.min(100, p.value / 10) })),
  },

  domainDistribution: [
    { name: '多领域回答', value: 90, color: '#0B41CD' },
    { name: '单领域回答', value: 10, color: '#DDE5FA' },
  ],
  crossDomainDistribution: [
    { name: 'DoA', value: 86, color: '#0B41CD' },
    { name: 'Compliance', value: 14, color: '#DDE5FA' },
  ],
  effectiveAnswerRing: {
    total: 1000,
    effectivePercent: 98,
    fallbackPercent: 2,
    effectiveColor: '#0B41CD',
    fallbackColor: '#DDE5FA',
  },

  topUsers: [
    { rank: 1, name: 'Steve Rogers', barValue: 309 },
    { rank: 2, name: 'Zora Christopher', barValue: 270 },
    { rank: 3, name: 'Helen Park', barValue: 240 },
    { rank: 4, name: 'Tony Stark', barValue: 215 },
    { rank: 5, name: 'Bruce Banner', barValue: 198 },
    { rank: 6, name: 'Natasha Romanoff', barValue: 182 },
    { rank: 7, name: 'Wanda Maxim', barValue: 165 },
    { rank: 8, name: 'Peter Parker', barValue: 148 },
    { rank: 9, name: 'Stephen Strange', barValue: 132 },
    { rank: 10, name: 'Carol Danvers', barValue: 118 },
  ],

  topDocs: [
    { rank: 1, name: 'Roche_Group_DoA_CN_v2.pdf', barValue: 282, isFirst: true },
    { rank: 2, name: 'Group_Functions_DoA_CN_v2.pdf', barValue: 256 },
    { rank: 3, name: 'Compliance_Handbook_2026.pdf', barValue: 230 },
    { rank: 4, name: 'Finance_Policy_v3.2.pdf', barValue: 208 },
    { rank: 5, name: 'Procurement_Guidelines.pdf', barValue: 186 },
    { rank: 6, name: 'Travel_Expense_v2.pdf', barValue: 168 },
    { rank: 7, name: 'HR_Handbook_2026.pdf', barValue: 152 },
    { rank: 8, name: 'Tax_Compliance_v4.pdf', barValue: 138 },
    { rank: 9, name: 'Data_Privacy_v2.pdf', barValue: 124 },
    { rank: 10, name: 'Vendor_Risk_v3.pdf', barValue: 110 },
  ],

  feedbackReasons: [
    { label: '事实性错误', count: 10, highlight: '#3067EB' },
    { label: '逻辑混乱', count: 9, highlight: '#3067EB' },
    { label: '时效性差', count: 8, highlight: '#3067EB' },
    { label: '格式错误', count: 7, highlight: '#3067EB' },
    { label: '回复过长', count: 6, highlight: '#3067EB' },
    { label: '内容重复', count: 5, highlight: '#3067EB' },
    { label: '其他', count: 4, highlight: '#3067EB' },
  ],

  fallbackQuestions: [
    { id: 'fbq-1', question: '财务现在的负责人是谁?', department: '财务' },
    { id: 'fbq-2', question: '报销单据提交后多久能到账?', department: '财务' },
    { id: 'fbq-3', question: '差旅住宿报销上限是多少?', department: '财务' },
    { id: 'fbq-4', question: '本月最新合规通知在哪里查看?', department: '合规' },
    { id: 'fbq-5', question: '供应商准入标准最新版是哪一份?', department: '采购' },
    { id: 'fbq-6', question: '员工培训记录如何归档?', department: 'HR' },
  ],

  defaults: {
    dateRange: ['2026-07-13', '2026-08-12'],
    department: '财务',
    knowledgeBase: '全部知识库',
  },
}