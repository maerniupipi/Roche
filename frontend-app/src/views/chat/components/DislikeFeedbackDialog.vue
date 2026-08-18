<template>
  <t-dialog :visible="visible" :header="$t('agentStream.feedback.dialog.title')" :footer="false" width="634px"
    placement="center" attach="body" :z-index="2600" :close-on-overlay-click="!submitting"
    :close-on-esc-keydown="!submitting" class="dislike-feedback-dialog" @update:visible="onVisibleUpdate">
    <div class="dislike-feedback-dialog__body">
      <ul class="dislike-feedback-dialog__options" role="radiogroup">
        <li v-for="option in reasonOptions" :key="option.key" class="dislike-feedback-dialog__option"
          :class="{ 'is-selected': selectedReason === option.key }" role="radio"
          :aria-checked="selectedReason === option.key" tabindex="0" @click="selectReason(option.key)"
          @keydown.enter.prevent="selectReason(option.key)" @keydown.space.prevent="selectReason(option.key)">
          <span class="dislike-feedback-dialog__option-radio" aria-hidden="true" />
          <span class="dislike-feedback-dialog__option-label">{{ option.label }}</span>
        </li>
      </ul>

      <!-- 「其他」选项被选中时才显示输入框 -->
      <div v-if="selectedReason === 'other'" class="dislike-feedback-dialog__other">
        <t-textarea v-model="otherComment" :placeholder="$t('agentStream.feedback.dialog.otherPlaceholder')"
          :autosize="{ minRows: 3, maxRows: 6 }" :maxlength="500" show-limit-number />
      </div>
    </div>

    <div class="dislike-feedback-dialog__footer">
      <t-button theme="primary" :loading="submitting" @click="handleConfirm">
        {{ submitting
          ? $t('agentStream.feedback.dialog.submitting')
          : $t('agentStream.feedback.dialog.confirm') }}
      </t-button>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-mobile-vue'
import { useI18n } from 'vue-i18n'
import {
  submitMessageFeedback,
  type DislikeReasonKey,
  type FeedbackSubmitResult,
} from '@/api/chat/feedback'

interface DislikeFeedbackDialogProps {
  visible: boolean
  /** 当前回答对应的 message_id，用于上报接口；占位期间可空 */
  messageId?: string
  /** 暴露给父级使用的提交回调，父级可借此埋点或刷新本地状态 */
  onSubmitted?: (result: FeedbackSubmitResult) => void
}

const props = defineProps<DislikeFeedbackDialogProps>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'submitted', result: FeedbackSubmitResult): void
}>()

const { t } = useI18n()

type ReasonOption = { key: DislikeReasonKey; label: string }

const reasonOptions = computed<ReasonOption[]>(() => [
  { key: 'factual_error', label: t('agentStream.feedback.dialog.reasons.factual_error') },
  { key: 'logic_confusion', label: t('agentStream.feedback.dialog.reasons.logic_confusion') },
  { key: 'outdated', label: t('agentStream.feedback.dialog.reasons.outdated') },
  { key: 'format_error', label: t('agentStream.feedback.dialog.reasons.format_error') },
  { key: 'too_long', label: t('agentStream.feedback.dialog.reasons.too_long') },
  { key: 'repetitive', label: t('agentStream.feedback.dialog.reasons.repetitive') },
  { key: 'other', label: t('agentStream.feedback.dialog.reasons.other') },
])

const selectedReason = ref<DislikeReasonKey | null>(null)
const otherComment = ref('')
const submitting = ref(false)

// 弹窗关闭 / 重开时重置本地状态，避免上一次的填写残留。
watch(
  () => props.visible,
  (open) => {
    if (open) {
      selectedReason.value = null
      otherComment.value = ''
      submitting.value = false
    }
  },
)

const selectReason = (key: DislikeReasonKey) => {
  if (submitting.value) return
  selectedReason.value = key
}

const onVisibleUpdate = (next: boolean) => {
  if (!next && submitting.value) return
  emit('update:visible', next)
}

const handleConfirm = async () => {
  if (submitting.value) return

  // 校验：必须选一项；选「其他」时必须填自由文本
  if (!selectedReason.value) {
    MessagePlugin.warning(t('agentStream.feedback.dialog.commentRequired'))
    return
  }
  let comment = ''
  if (selectedReason.value === 'other') {
    comment = otherComment.value.trim()
    if (!comment) {
      MessagePlugin.warning(t('agentStream.feedback.dialog.commentRequired'))
      return
    }
  }

  submitting.value = true
  try {
    const result = await submitMessageFeedback({
      // 占位期间允许 messageId 缺省；后端接口 ready 后建议必填。
      message_id: props.messageId ?? '',
      rating: 'dislike',
      reason: selectedReason.value,
      comment: comment || undefined,
    })

    if (!result.success) {
      MessagePlugin.error(result.message || t('agentStream.feedback.dialog.submitFailed'))
      return
    }

    MessagePlugin.success(t('agentStream.feedback.dialog.submitted'))
    props.onSubmitted?.(result)
    emit('submitted', result)
    emit('update:visible', false)
  } catch (err: any) {
    // eslint-disable-next-line no-console
    console.error('[feedback] submit failed:', err)
    MessagePlugin.error(t('agentStream.feedback.dialog.submitFailed'))
  } finally {
    submitting.value = false
  }
}
</script>

<style lang="less" scoped>
.dislike-feedback-dialog__body {
  margin-top: 16px;
  border-radius: 12px;
  padding: 16px;
  background: white;
}

.dislike-feedback-dialog__options {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.dislike-feedback-dialog__option {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 8px;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.15s ease, border-color 0.15s ease;
  outline: none;

  &:hover:not(.is-disabled) {
    background-color: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
  }

  &:focus-visible {
    border-color: var(--td-brand-color);
  }

  &.is-selected {
    .dislike-feedback-dialog__option-radio {
      &::after {
        transform: scale(1);
      }
    }
  }
}

.dislike-feedback-dialog__option-radio {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 1px solid color-mix(in srgb, var(--td-text-color-primary) 30%, transparent);
  position: relative;
  flex-shrink: 0;
  transition: border-color 0.15s ease;

  &::after {
    content: '';
    position: absolute;
    inset: 3px;
    border-radius: 50%;
    background-color: var(--td-brand-color);
    transform: scale(0);
    transition: transform 0.15s ease;
  }

  .is-selected & {
    border-color: var(--td-brand-color);
  }
}

.dislike-feedback-dialog__option-label {
  font-size: 14px;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.dislike-feedback-dialog__other {
  margin: 10px 0 0 42px;
}

.dislike-feedback-dialog__footer {
  display: flex;
  justify-content: center;
  padding-top: 12px;
}
</style>