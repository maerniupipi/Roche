<template>
  <div class="answer-records-page page-shell page-shell--no-scroll">
    <!-- 筛选区 -->
    <PageSearchFilters v-model:search="state.filters.username"
      :search-placeholder="t('answerRecords.filters.usernamePlaceholder')">
      <template #default>
        <t-select v-model="state.filters.channel" :placeholder="t('answerRecords.filters.channel')" clearable>
          <t-option :value="'web'" :label="t('answerRecords.platform.web')" />
          <t-option :value="'app'" :label="t('answerRecords.platform.app')" />
        </t-select>
        <t-select v-model="state.filters.feedback" :placeholder="t('answerRecords.filters.feedback')" clearable>
          <t-option :value="'like'" :label="t('answerRecords.feedback.like')" />
          <t-option :value="'dislike'" :label="t('answerRecords.feedback.dislike')" />
          <t-option :value="'none'" :label="t('answerRecords.feedback.none')" />
        </t-select>
        <t-date-range-picker v-model="state.filters.timeRange" :placeholder="t('answerRecords.filters.timeRange')"
          clearable />
      </template>
      <template #trailing>
        <t-button theme="primary" :loading="state.loading" @click="onApply">
          {{ t('common.search') }}
        </t-button>
        <t-button theme="default" variant="base" @click="onReset">
          {{ t('common.reset') }}
        </t-button>
      </template>
    </PageSearchFilters>

    <!-- 表格区 -->
    <div class="answer-records-page__table">
      <PageTable :data="state.items" :columns="columns" :loading="state.loading" :pagination="state.pagination"
        @page-change="onPageChange">
        <template #platform="{ row }">
          <t-tag :theme="(row as AnswerRecordItem).channel === 'web' ? 'primary' : 'success'" size="small">
            {{ t(`answerRecords.platform.${(row as AnswerRecordItem).channel}`) }}
          </t-tag>
        </template>
        <template #username="{ row }">
          <span>{{ (row as AnswerRecordItem).username }}</span>
        </template>
        <template #sessionTitle="{ row }">
          <span>{{ (row as AnswerRecordItem).session_title || '-' }}</span>
        </template>
        <template #kbNames="{ row }">
          <span>{{ formatKbNames((row as AnswerRecordItem).knowledge_bases) }}</span>
        </template>
        <template #rating="{ row }">
          <t-tag :theme="feedbackTheme((row as AnswerRecordItem).feedback?.rating || 'none')" size="small">
            {{ t(`answerRecords.feedback.${(row as AnswerRecordItem).feedback?.rating || 'none'}`) }}
          </t-tag>
        </template>
        <template #actions="{ row }">
          <t-link theme="primary"
            :disabled="!((row as AnswerRecordItem).feedback && (row as AnswerRecordItem).feedback?.rating !== 'none')"
            @click="onFeedbackClick((row as AnswerRecordItem))">
            {{ t('answerRecords.actions.viewFeedback') }}
          </t-link>
        </template>
      </PageTable>
    </div>

    <!-- 底部操作栏 -->
    <footer class="answer-records-page__footer">
      <div class="answer-records-page__actions">
        <t-button theme="primary" :loading="state.exporting" @click="onExport">
          {{ t('answerRecords.actions.export') }}
        </t-button>
      </div>
    </footer>

    <!-- 反馈详情对话框 -->
    <FeedbackDetailDialog v-model:visible="state.dialogVisible" :feedback="state.currentFeedback" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import PageSearchFilters from '@/components/common/PageSearchFilters.vue'
import PageTable from '@/components/common/PageTable.vue'
import type { PageTableColumn, PageTablePagination } from '@/components/common/PageTable.vue'
import FeedbackDetailDialog from './FeedbackDetailDialog.vue'
import {
  listAnswerRecords,
  exportAnswerRecords,
  type AnswerRecordItem,
  type AnswerRecordFeedbackDetail,
  type AnswerRecordChannel,
  type AnswerRecordFeedbackRating,
} from '@/api/system'

type DateRange = [string, string] | []

interface AnswerRecordFilters {
  username: string
  channel: '' | AnswerRecordChannel
  feedback: '' | AnswerRecordFeedbackRating
  timeRange: DateRange
}

interface AnswerRecordPageState {
  filters: AnswerRecordFilters
  items: AnswerRecordItem[]
  loading: boolean
  exporting: boolean
  pagination: PageTablePagination
  dialogVisible: boolean
  currentFeedback: AnswerRecordFeedbackDetail | null
}

const { t } = useI18n()

/**
 * 把 t-date-range-picker 的字符串数组转换为后端 RFC3339。
 * 结束时间补到当日 23:59:59.999，避免按「天」过滤漏掉当天后半段记录。
 * 输入格式（t-date-range-picker 默认）：YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD。
 */
function convertTimeRange(range: DateRange): { start_time?: string; end_time?: string } {
  if (!range || range.length !== 2 || !range[0] || !range[1]) return {}
  const start = new Date(range[0])
  const end = new Date(range[1])
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return {}
  end.setHours(23, 59, 59, 999)
  return {
    start_time: start.toISOString(),
    end_time: end.toISOString(),
  }
}

function formatKbNames(arr?: string[]): string {
  if (!arr || arr.length === 0) return '-'
  return arr.join('、')
}

function feedbackTheme(rating: AnswerRecordFeedbackRating): 'success' | 'danger' | 'default' {
  if (rating === 'like') return 'success'
  if (rating === 'dislike') return 'danger'
  return 'default'
}

const state = reactive<AnswerRecordPageState>({
  filters: {
    username: '',
    channel: '',
    feedback: '',
    timeRange: [],
  },
  items: [],
  loading: false,
  exporting: false,
  pagination: {
    page: 1,
    pageSize: 20,
    total: 0,
  },
  dialogVisible: false,
  currentFeedback: null,
})

/** columns 包成 computed 让 title 跟随 locale 自动重算。 */
const columns = computed<PageTableColumn<AnswerRecordItem>[]>(() => [
  { key: 'platform', title: t('answerRecords.columns.platform'), width: 100 },
  { key: 'username', title: t('answerRecords.columns.username'), width: 120, ellipsis: true },
  { key: 'sessionTitle', title: t('answerRecords.columns.sessionTitle'), width: 200, ellipsis: true },
  { key: 'question', title: t('answerRecords.columns.question'), minWidth: 240, ellipsis: true },
  { key: 'kbNames', title: t('answerRecords.columns.knowledgeBases'), minWidth: 200, ellipsis: true },
  { key: 'rating', title: t('answerRecords.columns.feedback'), width: 110 },
  { key: 'askedAt', title: t('answerRecords.columns.askedAt'), width: 170 },
  { key: 'actions', title: t('answerRecords.columns.actions'), width: 140 },
])

async function loadList(): Promise<void> {
  state.loading = true
  try {
    const { start_time, end_time } = convertTimeRange(state.filters.timeRange)
    const { data: res }: any = await listAnswerRecords({
      username: state.filters.username || undefined,
      channel: (state.filters.channel || undefined) as AnswerRecordChannel | undefined,
      feedback: (state.filters.feedback || undefined) as AnswerRecordFeedbackRating | undefined,
      start_time,
      end_time,
      page: state.pagination.page,
      page_size: state.pagination.pageSize,
    })
    state.items = res.data || []
    state.pagination.total = res.total || 0
    state.pagination.page = res.page || state.pagination.page
    state.pagination.pageSize = res.page_size || state.pagination.pageSize
  } catch {
    state.items = []
    state.pagination.total = 0
    MessagePlugin.error(t('answerRecords.messages.loadFailed'))
  } finally {
    state.loading = false
  }
}

function onApply(): void {
  state.pagination.page = 1
  loadList()
}

function onReset(): void {
  state.filters.username = ''
  state.filters.channel = ''
  state.filters.feedback = ''
  state.filters.timeRange = []
  state.pagination.page = 1
  loadList()
}

function onPageChange(p: { page: number; pageSize: number }): void {
  state.pagination.page = p.page
  state.pagination.pageSize = p.pageSize
  loadList()
}

function onFeedbackClick(row: AnswerRecordItem): void {
  if (!row.feedback || row.feedback.rating === 'none') {
    MessagePlugin.info(t('answerRecords.messages.noFeedback'))
    return
  }
  state.currentFeedback = row.feedback
  state.dialogVisible = true
}

function triggerDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

async function onExport(): Promise<void> {
  state.exporting = true
  try {
    const { start_time, end_time } = convertTimeRange(state.filters.timeRange)
    const { blob, filename } = await exportAnswerRecords({
      username: state.filters.username || undefined,
      channel: (state.filters.channel || undefined) as AnswerRecordChannel | undefined,
      feedback: (state.filters.feedback || undefined) as AnswerRecordFeedbackRating | undefined,
      start_time,
      end_time,
    })
    triggerDownload(blob, filename)
    MessagePlugin.success(t('answerRecords.messages.exportSuccess'))
  } catch {
    MessagePlugin.error(t('answerRecords.messages.exportFailed'))
  } finally {
    state.exporting = false
  }
}

onMounted(() => {
  loadList()
})
</script>

<style lang="less" scoped>
@import '@/assets/styles/page-shared.less';

// 覆盖 page-shell 让其不滚动，由内部表格 body 滚动
.page-shell--no-scroll {
  overflow: hidden;
  padding-bottom: 0;
}

.answer-records-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 16px;
}

.answer-records-page__table {
  flex: 1 1 auto;
  min-height: 0;
}

.answer-records-page__footer {
  background: var(--td-bg-color-container, #fff);
  border-top: 1px solid var(--td-component-stroke, #e7e7e7);
  padding: 12px 0;
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.answer-records-page__actions {
  display: flex;
  gap: 8px;
}
</style>
