<template>
  <t-dialog
    :visible="visible"
    :header="t('answerRecords.feedbackDetail.title')"
    :width="560"
    :footer="false"
    @close="onClose"
    @update:visible="(v) => emit('update:visible', v)"
  >
    <div v-if="feedback" class="feedback-detail-dialog">
      <div class="feedback-detail-dialog__field">
        <label>{{ t('answerRecords.feedbackDetail.rating') }}</label>
        <t-tag
          :theme="feedback.rating === 'dislike' ? 'danger' : feedback.rating === 'like' ? 'success' : 'default'"
          size="small"
        >
          {{ t(`answerRecords.feedback.${feedback.rating}`) }}
        </t-tag>
      </div>
      <div
        v-if="feedback.reason_zh || feedback.reason_en || feedback.reason"
        class="feedback-detail-dialog__field"
      >
        <label>{{ t('answerRecords.feedbackDetail.reason') }}</label>
        <span>{{ feedback.reason_zh || feedback.reason_en || feedback.reason }}</span>
      </div>
      <div v-if="feedback.comment" class="feedback-detail-dialog__field">
        <label>{{ t('answerRecords.feedbackDetail.comment') }}</label>
        <span>{{ feedback.comment }}</span>
      </div>
      <div v-if="feedback.created_at" class="feedback-detail-dialog__field">
        <label>{{ t('answerRecords.feedbackDetail.createdAt') }}</label>
        <span>{{ formatDateTime(feedback.created_at) }}</span>
      </div>
      <div
        v-if="feedback.updated_at && feedback.updated_at !== feedback.created_at"
        class="feedback-detail-dialog__field"
      >
        <label>{{ t('answerRecords.feedbackDetail.updatedAt') }}</label>
        <span>{{ formatDateTime(feedback.updated_at) }}</span>
      </div>
    </div>
    <div v-else class="feedback-detail-dialog__empty">
      {{ t('common.noData') }}
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AnswerRecordFeedbackDetail } from '@/api/system'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  feedback: AnswerRecordFeedbackDetail | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

function onClose() {
  emit('update:visible', false)
}

function formatDateTime(s: string): string {
  if (!s) return ''
  try {
    const d = new Date(s)
    if (Number.isNaN(d.getTime())) return s
    return d.toLocaleString()
  } catch {
    return s
  }
}
</script>

<style lang="less" scoped>
.feedback-detail-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;

  &__field {
    display: flex;
    flex-direction: column;
    gap: 6px;

    label {
      font-size: 13px;
      color: var(--td-text-color-secondary, #666);
    }

    span {
      font-size: 14px;
      color: var(--td-text-color-primary, #21201f);
      word-break: break-word;
    }
  }

  &__empty {
    text-align: center;
    color: var(--td-text-color-secondary, #999);
    padding: 24px;
  }
}
</style>