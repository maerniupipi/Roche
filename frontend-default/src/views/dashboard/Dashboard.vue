<template>
  <div class="dashboard-page">
    <div class="dashboard-header">
      <h2 class="dashboard-title">数据看板</h2>
    </div>

    <!-- 知识库文件统计 -->
    <div class="dashboard-section">
      <div class="section-filter">
        <span class="filter-label">知识库</span>
        <t-select
          v-model="selectedKbId"
          :options="kbOptions"
          placeholder="请选择知识库"
          clearable
          style="width: 220px"
          @change="loadKbStats"
        />
      </div>
      <div class="stat-card-grid">
        <div class="stat-card stat-card--blue">
          <div class="stat-card-label">已上架文件数</div>
          <div class="stat-card-value">{{ kbStats.published_count }}</div>
          <div v-if="kbStats.scheduled_publish_count > 0" class="stat-card-sub">
            长期未更新文件：{{ kbStats.scheduled_publish_count }}
          </div>
        </div>
        <div class="stat-card stat-card--green">
          <div class="stat-card-label">上传成功文件数</div>
          <div class="stat-card-value">{{ kbStats.upload_success_count }}</div>
        </div>
        <div class="stat-card stat-card--red">
          <div class="stat-card-label">上传失败文件数</div>
          <div class="stat-card-value">{{ kbStats.upload_failed_count }}</div>
        </div>
        <div class="stat-card stat-card--plain">
          <div class="stat-card-label">预约上架文件数</div>
          <div class="stat-card-value">{{ kbStats.scheduled_publish_count }}</div>
        </div>
        <div class="stat-card stat-card--plain">
          <div class="stat-card-label">已下架文件数</div>
          <div class="stat-card-value">{{ kbStats.unpublished_count }}</div>
        </div>
        <div class="stat-card stat-card--plain">
          <div class="stat-card-label">归档文件数</div>
          <div class="stat-card-value">{{ kbStats.archived_count }}</div>
        </div>
      </div>
    </div>

    <!-- 问答统计 -->
    <div class="dashboard-section">
      <div class="section-filter">
        <t-select
          v-model="selectedDomainId"
          :options="domainOptions"
          placeholder="请选择知识域"
          clearable
          style="width: 220px"
          @change="loadChatStats"
        />
        <t-date-range-picker
          v-model="chatDateRange"
          :placeholder="['开始日期', '结束日期']"
          style="width: 260px"
          @change="loadChatStats"
        />
      </div>
      <div class="metric-row">
        <div class="metric-item">
          <div class="metric-label">平均开始回答时长</div>
          <div class="metric-value">{{ chatStats.avg_first_response_sec.toFixed(1) }}s</div>
        </div>
        <div class="metric-item">
          <div class="metric-label">平均完成回答时长</div>
          <div class="metric-value">{{ chatStats.avg_complete_sec.toFixed(1) }}s</div>
        </div>
      </div>
      <div class="chart-card">
        <div class="chart-title">问答总数</div>
        <div ref="questionChartRef" class="chart-body"></div>
      </div>
      <div class="chart-card">
        <div class="chart-title">提问人数</div>
        <div ref="userChartRef" class="chart-body"></div>
      </div>
      <div class="chart-card">
        <div class="chart-title">对话满意度</div>
        <div ref="satisfactionChartRef" class="chart-body"></div>
      </div>
    </div>

    <!-- 总览 -->
    <div class="dashboard-section">
      <div class="section-filter">
        <t-date-range-picker
          v-model="overviewDateRange"
          :placeholder="['开始日期', '结束日期']"
          style="width: 260px"
          @change="loadOverview"
        />
      </div>
      <div class="overview-grid">
        <div class="chart-card overview-pie">
          <div class="chart-title">问题领域分布</div>
          <div ref="domainPieRef" class="chart-body"></div>
        </div>
        <div class="chart-card overview-pie">
          <div class="chart-title">跨领域回答占比</div>
          <div ref="crossDomainPieRef" class="chart-body"></div>
        </div>
      </div>
      <div class="chart-card">
        <div class="chart-title with-action">
          <span>热门文档</span>
          <t-button variant="text" shape="square" @click="downloadHotDocs">
            <t-icon name="download" />
          </t-button>
        </div>
        <div class="rank-list">
          <div
            v-for="doc in overview.top_documents"
            :key="doc.rank"
            class="rank-row"
          >
            <span class="rank-index">{{ doc.rank }}.</span>
            <span class="rank-name" :title="doc.title">{{ doc.title }}</span>
            <span class="rank-bar-wrap">
              <span class="rank-bar" :style="{ width: hotDocBarWidth(doc.hit_count) }"></span>
            </span>
            <span class="rank-value">{{ doc.hit_count }}</span>
          </div>
        </div>
      </div>
      <div class="chart-card">
        <div class="chart-title">产品反馈</div>
        <div ref="feedbackChartRef" class="chart-body"></div>
      </div>
      <div class="overview-grid">
        <div class="chart-card">
          <div class="chart-title with-action">
            <span>提问用户榜</span>
            <t-button variant="text" shape="square" @click="downloadTopUsers">
              <t-icon name="download" />
            </t-button>
          </div>
          <div class="rank-list">
            <div
              v-for="user in overview.top_users"
              :key="user.rank"
              class="rank-row"
            >
              <span class="rank-index">{{ user.rank }}.</span>
              <span class="rank-name">{{ user.user_name }}</span>
              <span class="rank-bar-wrap">
                <span class="rank-bar" :style="{ width: topUserBarWidth(user.question_count) }"></span>
              </span>
            </div>
          </div>
        </div>
        <div class="chart-card overview-pie">
          <div class="chart-title">有效回答率</div>
          <div ref="validRateChartRef" class="chart-body"></div>
        </div>
      </div>
      <div class="chart-card">
        <div class="chart-title with-action">
          <span>触发兜底话术问题</span>
          <t-button variant="outline" size="small" @click="showFallbackDetail">查看详情</t-button>
        </div>
        <div class="fallback-list">
          <div
            v-for="q in overview.fallback_questions"
            :key="q.rank"
            class="fallback-row"
          >
            <span class="fallback-index">{{ q.rank }}.</span>
            <span class="fallback-content">{{ q.content }}</span>
          </div>
          <div v-if="overview.fallback_questions.length === 0" class="empty-tip">暂无数据</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue';
import * as echarts from 'echarts';
import { MessagePlugin } from 'tdesign-vue-next';
import { listKnowledgeBases } from '@/api/knowledge-base';
import { listKnowledgeDomains } from '@/api/knowledge-domain';
import {
  getKnowledgeBaseStats,
  getChatStats,
  getOverview,
  type KnowledgeBaseStats,
  type ChatStats,
  type OverviewData,
} from '@/api/dashboard';

const today = new Date();
const weekAgo = new Date(today.getTime() - 6 * 24 * 60 * 60 * 1000);
const formatDate = (d: Date) => d.toISOString().split('T')[0];

const selectedKbId = ref('');
const kbOptions = ref<{ label: string; value: string }[]>([{ label: '全部', value: '' }]);
const selectedDomainId = ref<number | undefined>(undefined);
const domainOptions = ref<{ label: string; value: number }[]>([]);
const chatDateRange = ref([formatDate(weekAgo), formatDate(today)]);
const overviewDateRange = ref([formatDate(weekAgo), formatDate(today)]);

const kbStats = ref<KnowledgeBaseStats>({
  published_count: 0,
  upload_success_count: 0,
  upload_failed_count: 0,
  scheduled_publish_count: 0,
  unpublished_count: 0,
  archived_count: 0,
});

const chatStats = ref<ChatStats>({
  avg_first_response_sec: 0,
  avg_complete_sec: 0,
  daily: [],
});

const overview = ref<OverviewData>({
  domain_distribution: [],
  cross_domain_single: 0,
  cross_domain_multi: 0,
  top_documents: [],
  product_feedback: [],
  top_users: [],
  valid_answer_count: 0,
  fallback_answer_count: 0,
  fallback_questions: [],
});

const questionChartRef = ref<HTMLElement | null>(null);
const userChartRef = ref<HTMLElement | null>(null);
const satisfactionChartRef = ref<HTMLElement | null>(null);
const domainPieRef = ref<HTMLElement | null>(null);
const crossDomainPieRef = ref<HTMLElement | null>(null);
const feedbackChartRef = ref<HTMLElement | null>(null);
const validRateChartRef = ref<HTMLElement | null>(null);

let questionChart: echarts.ECharts | null = null;
let userChart: echarts.ECharts | null = null;
let satisfactionChart: echarts.ECharts | null = null;
let domainPieChart: echarts.ECharts | null = null;
let crossDomainPieChart: echarts.ECharts | null = null;
let feedbackChart: echarts.ECharts | null = null;
let validRateChart: echarts.ECharts | null = null;

const hotDocMax = computed(() => {
  if (!overview.value.top_documents.length) return 1;
  return Math.max(...overview.value.top_documents.map(d => d.hit_count));
});

const topUserMax = computed(() => {
  if (!overview.value.top_users.length) return 1;
  return Math.max(...overview.value.top_users.map(u => u.question_count));
});

function hotDocBarWidth(count: number) {
  return `${(count / hotDocMax.value) * 100}%`;
}

function topUserBarWidth(count: number) {
  return `${(count / topUserMax.value) * 100}%`;
}

async function loadKnowledgeBases() {
  try {
    const res: any = await listKnowledgeBases();
    const list = res.data || [];
    kbOptions.value = [{ label: '全部', value: '' }, ...list.map((kb: any) => ({ label: kb.name, value: kb.id }))];
  } catch (e) {
    MessagePlugin.error('加载知识库失败');
  }
}

async function loadKnowledgeDomains() {
  try {
    const res: any = await listKnowledgeDomains();
    const list = res.data?.items || [];
    domainOptions.value = list.map((d: any) => ({ label: d.name, value: d.id }));
  } catch (e) {
    MessagePlugin.error('加载知识域失败');
  }
}

async function loadKbStats() {
  try {
    const res: any = await getKnowledgeBaseStats(selectedKbId.value || undefined);
    kbStats.value = res.data;
  } catch (e) {
    MessagePlugin.error('加载知识库统计失败');
  }
}

async function loadChatStats() {
  if (!chatDateRange.value || chatDateRange.value.length !== 2) return;
  try {
    const res: any = await getChatStats({
      knowledge_domain_id: selectedDomainId.value,
      start_date: chatDateRange.value[0],
      end_date: chatDateRange.value[1],
    });
    chatStats.value = res.data;
    renderChatCharts();
  } catch (e) {
    MessagePlugin.error('加载问答统计失败');
  }
}

async function loadOverview() {
  if (!overviewDateRange.value || overviewDateRange.value.length !== 2) return;
  try {
    const res: any = await getOverview({
      knowledge_domain_id: selectedDomainId.value,
      start_date: overviewDateRange.value[0],
      end_date: overviewDateRange.value[1],
    });
    overview.value = res.data;
    renderOverviewCharts();
  } catch (e) {
    MessagePlugin.error('加载总览数据失败');
  }
}

function renderChatCharts() {
  const dates = chatStats.value.daily.map(d => d.date.slice(5));
  const questions = chatStats.value.daily.map(d => d.question_count);
  const users = chatStats.value.daily.map(d => d.unique_users);
  const satisfaction = chatStats.value.daily.map(d => (d.satisfaction_pct || 95).toFixed(1));

  if (questionChart) {
    questionChart.setOption({
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: dates },
      yAxis: { type: 'value' },
      series: [{
        data: questions,
        type: 'bar',
        itemStyle: { color: '#5b8ff9' },
        barWidth: '40%',
      }],
    });
  }

  if (userChart) {
    userChart.setOption({
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: dates },
      yAxis: { type: 'value' },
      series: [{
        data: users,
        type: 'line',
        smooth: true,
        itemStyle: { color: '#5b8ff9' },
        areaStyle: { opacity: 0.1, color: '#5b8ff9' },
      }],
    });
  }

  if (satisfactionChart) {
    satisfactionChart.setOption({
      tooltip: { trigger: 'axis', formatter: '{b}: {c}%' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: dates },
      yAxis: { type: 'value', min: 80, max: 100, axisLabel: { formatter: '{value}%' } },
      series: [{
        data: satisfaction,
        type: 'line',
        smooth: true,
        itemStyle: { color: '#5b8ff9' },
      }],
    });
  }
}

function renderOverviewCharts() {
  if (domainPieChart) {
    domainPieChart.setOption({
      tooltip: { trigger: 'item' },
      legend: { top: '5%', left: 'center' },
      series: [{
        name: '问题领域',
        type: 'pie',
        radius: ['40%', '70%'],
        avoidLabelOverlap: false,
        itemStyle: { borderRadius: 5, borderColor: '#fff', borderWidth: 2 },
        label: { show: true, formatter: '{b}\n{c}' },
        data: overview.value.domain_distribution,
      }],
    });
  }

  if (crossDomainPieChart) {
    crossDomainPieChart.setOption({
      tooltip: { trigger: 'item' },
      legend: { top: '5%', left: 'center' },
      series: [{
        name: '跨领域回答',
        type: 'pie',
        radius: '60%',
        data: [
          { name: '单领域回答', value: overview.value.cross_domain_single },
          { name: '多领域回答', value: overview.value.cross_domain_multi },
        ],
      }],
    });
  }

  if (feedbackChart) {
    feedbackChart.setOption({
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'value' },
      yAxis: {
        type: 'category',
        data: [...overview.value.product_feedback].reverse().map(f => f.category),
      },
      series: [{
        type: 'bar',
        data: [...overview.value.product_feedback].reverse().map(f => f.count),
        itemStyle: { color: '#5b8ff9' },
        barWidth: '50%',
        label: { show: true, position: 'right' },
      }],
    });
  }

  if (validRateChart) {
    const valid = overview.value.valid_answer_count;
    const fallback = overview.value.fallback_answer_count;
    validRateChart.setOption({
      tooltip: { trigger: 'item' },
      legend: { bottom: '5%', left: 'center' },
      series: [{
        name: '有效回答率',
        type: 'pie',
        radius: ['50%', '70%'],
        avoidLabelOverlap: false,
        label: {
          show: true,
          position: 'center',
          formatter: () => {
            const total = valid + fallback || 1;
            return `${((valid / total) * 100).toFixed(0)}%`;
          },
          fontSize: 18,
          fontWeight: 'bold',
        },
        data: [
          { name: '有效回答', value: valid },
          { name: '触发兜底话术', value: fallback },
        ],
      }],
    });
  }
}

function downloadHotDocs() {
  const lines = overview.value.top_documents.map(d => `${d.rank}\t${d.title}\t${d.hit_count}`);
  downloadAsFile(lines.join('\n'), 'hot-documents.txt');
}

function downloadTopUsers() {
  const lines = overview.value.top_users.map(u => `${u.rank}\t${u.user_name}\t${u.question_count}`);
  downloadAsFile(lines.join('\n'), 'top-users.txt');
}

function downloadAsFile(content: string, filename: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function showFallbackDetail() {
  MessagePlugin.info(`共 ${overview.value.fallback_questions.length} 条兜底问题`);
}

function initCharts() {
  if (questionChartRef.value) questionChart = echarts.init(questionChartRef.value);
  if (userChartRef.value) userChart = echarts.init(userChartRef.value);
  if (satisfactionChartRef.value) satisfactionChart = echarts.init(satisfactionChartRef.value);
  if (domainPieRef.value) domainPieChart = echarts.init(domainPieRef.value);
  if (crossDomainPieRef.value) crossDomainPieChart = echarts.init(crossDomainPieRef.value);
  if (feedbackChartRef.value) feedbackChart = echarts.init(feedbackChartRef.value);
  if (validRateChartRef.value) validRateChart = echarts.init(validRateChartRef.value);

  const resize = () => {
    questionChart?.resize();
    userChart?.resize();
    satisfactionChart?.resize();
    domainPieChart?.resize();
    crossDomainPieChart?.resize();
    feedbackChart?.resize();
    validRateChart?.resize();
  };
  window.addEventListener('resize', resize);
  return () => window.removeEventListener('resize', resize);
}

onMounted(async () => {
  await Promise.all([loadKnowledgeBases(), loadKnowledgeDomains()]);
  const cleanup = initCharts();
  await Promise.all([loadKbStats(), loadChatStats(), loadOverview()]);
  onUnmounted(cleanup);
});

onUnmounted(() => {
  questionChart?.dispose();
  userChart?.dispose();
  satisfactionChart?.dispose();
  domainPieChart?.dispose();
  crossDomainPieChart?.dispose();
  feedbackChart?.dispose();
  validRateChart?.dispose();
});
</script>

<style scoped lang="less">
.dashboard-page {
  padding: 20px;
  overflow-y: auto;
  background: #f5f7fa;
  min-height: 100%;
}

.dashboard-header {
  margin-bottom: 16px;
}

.dashboard-title {
  font-size: 20px;
  font-weight: 600;
  color: #1d2129;
  margin: 0;
}

.dashboard-section {
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 16px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.section-filter {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.filter-label {
  font-size: 14px;
  color: #4e5969;
}

.stat-card-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.stat-card {
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  padding: 16px;
  background: #fff;
}

.stat-card--blue {
  border-color: #5b8ff9;
  background: #f6f8ff;
}

.stat-card--green {
  border-color: #36b37e;
  background: #f0fff6;
}

.stat-card--red {
  border-color: #f5222d;
  background: #fff5f5;
}

.stat-card--plain {
  border-color: #e5e6eb;
}

.stat-card-label {
  font-size: 14px;
  color: #4e5969;
  margin-bottom: 8px;
}

.stat-card-value {
  font-size: 32px;
  font-weight: 600;
  color: #1d2129;
}

.stat-card-sub {
  margin-top: 8px;
  font-size: 12px;
  color: #f5222d;
  background: rgba(245, 34, 45, 0.08);
  padding: 4px 8px;
  border-radius: 4px;
  display: inline-block;
}

.metric-row {
  display: flex;
  gap: 24px;
  margin-bottom: 16px;
}

.metric-item {
  flex: 1;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  background: #f7f8fa;
  border-radius: 8px;
}

.metric-label {
  font-size: 14px;
  color: #4e5969;
}

.metric-value {
  font-size: 24px;
  font-weight: 600;
  color: #1d2129;
}

.chart-card {
  background: #f7f8fa;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}

.chart-title {
  font-size: 16px;
  font-weight: 600;
  color: #1d2129;
  margin-bottom: 12px;
  padding-left: 12px;
  border-left: 4px solid #5b8ff9;
}

.chart-title.with-action {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.chart-body {
  width: 100%;
  height: 260px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.overview-pie .chart-body {
  height: 240px;
}

.rank-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.rank-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}

.rank-index {
  width: 24px;
  color: #86909c;
  flex-shrink: 0;
}

.rank-name {
  width: 220px;
  color: #1d2129;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex-shrink: 0;
}

.rank-bar-wrap {
  flex: 1;
  height: 8px;
  background: #e5e6eb;
  border-radius: 4px;
  overflow: hidden;
}

.rank-bar {
  display: block;
  height: 100%;
  background: #5b8ff9;
  border-radius: 4px;
}

.rank-value {
  width: 48px;
  text-align: right;
  color: #86909c;
  flex-shrink: 0;
}

.fallback-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.fallback-row {
  display: flex;
  gap: 8px;
  font-size: 14px;
  color: #1d2129;
}

.fallback-index {
  width: 24px;
  color: #86909c;
  flex-shrink: 0;
}

.empty-tip {
  color: #86909c;
  font-size: 14px;
  text-align: center;
  padding: 24px 0;
}
</style>
