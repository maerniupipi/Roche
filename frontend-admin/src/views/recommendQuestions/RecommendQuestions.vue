<template>
  <div class="exchange-page">
    <header class="exchange-page__header">
      <iconTitle :title="t('recommendQuestions.title')" />
    </header>

    <section class="exchange-card">
      <div class="recommend-card">
        <div v-for="(item, index) in items" :key="index" class="recommend-card__item">
          <div class="recommend-card__heading">
            {{ t('recommendQuestions.moduleTitle', { index: index + 1 }) }}
          </div>

          <div class="recommend-card__field">
            <t-input v-model="item.question" :placeholder="t('recommendQuestions.questionPlaceholder')" />
          </div>

          <div class="recommend-card__field recommend-card__field--row">
            <span class="recommend-card__label">
              {{ t('recommendQuestions.customAnswerToggleLabel') }}
            </span>
            <t-switch v-model="item.useCustomAnswer" />
          </div>

          <div v-if="item.useCustomAnswer" class="recommend-card__field">
            <t-textarea v-model="item.customAnswer" :placeholder="t('recommendQuestions.customAnswerPlaceholder')"
              :autosize="{ minRows: 3, maxRows: 6 }" />
          </div>
        </div>

        <p v-if="hasDuplicate" class="recommend-card__error">
          {{ t('recommendQuestions.duplicateError') }}
        </p>
      </div>
    </section>

    <footer class="exchange-page-footer">
      <button type="button" class="exchange-page__confirm" :disabled="isConfirmDisabled" @click="handleConfirm">
        {{ t('recommendQuestions.confirm') }}
      </button>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import iconTitle from '@/components/common/iconTitle.vue'
import {
  listSuggestedQuestions,
  updateSuggestedQuestionsConfig,
  type SuggestedQuestion,
  type SuggestedQuestionItemInput,
} from '@/api/recommendQuestions'

interface RecommendQuestionItem {
  id?: string
  question: string
  useCustomAnswer: boolean
  customAnswer: string
  sortOrder: 1 | 2 | 3
}

// 三个固定槽位：sort_order 与 1/2/3 一一对应，新增不传 id 让后端生成。
const items = reactive<RecommendQuestionItem[]>([
  { id: undefined, question: '', useCustomAnswer: false, customAnswer: '', sortOrder: 1 },
  { id: undefined, question: '', useCustomAnswer: false, customAnswer: '', sortOrder: 2 },
  { id: undefined, question: '', useCustomAnswer: false, customAnswer: '', sortOrder: 3 },
])

const isSubmitting = ref(false)
const { t } = useI18n()

// 已填题目 trim 后两两不同；空槽位不参与比较。
const hasDuplicate = computed(() => {
  const filled = items.filter((i) => i.question.trim()).map((i) => i.question.trim())
  return new Set(filled).size !== filled.length
})

// 禁用条件：正在提交 / 题目重复 / 任一题空 / custom 模式下自定义回答空。
const isConfirmDisabled = computed(() =>
  isSubmitting.value
  || hasDuplicate.value
  || items.some((i) => !i.question.trim())
  || items.some((i) => i.useCustomAnswer && !i.customAnswer.trim()),
)

// 进入页面时拉取当前配置：按 sort_order 升序填入三个槽位，缺失槽位保留默认。
async function loadExistingConfig(): Promise<void> {
  try {
    const res = await listSuggestedQuestions()
    const questions = res.data?.questions ?? []
    if (questions.length === 0) return
    const sorted: SuggestedQuestion[] = [...questions].sort((a, b) => a.sort_order - b.sort_order)
    sorted.forEach((q, idx) => {
      const slot = items[idx]
      if (!slot) return
      slot.id = q.id
      slot.question = q.question
      slot.useCustomAnswer = q.answer_mode === 'custom'
      slot.customAnswer = q.custom_answer ?? ''
    })
  } catch (err) {
    // 加载失败静默降级，不打扰用户首次进入页面。
    console.warn('[recommendQuestions] load existing config failed', err)
  }
}

onMounted(() => {
  void loadExistingConfig()
})

async function handleConfirm(): Promise<void> {
  if (isConfirmDisabled.value) return
  isSubmitting.value = true
  const payload: SuggestedQuestionItemInput[] = items.map((it) => ({
    id: it.id,
    question: it.question.trim(),
    // 关闭开关（自动生成）时按接口约定传空串，开启后传 trim 后的自定义回答。
    answer_mode: it.useCustomAnswer ? 'custom' : 'generated',
    custom_answer: it.useCustomAnswer ? it.customAnswer.trim() : '',
    sort_order: it.sortOrder,
  }))
  try {
    await updateSuggestedQuestionsConfig(payload)
    MessagePlugin.success(t('recommendQuestions.submitSuccess'))
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('recommendQuestions.submitFailed'))
  } finally {
    isSubmitting.value = false
  }
}
</script>

<style lang="less" scoped>
@import '@/assets/styles/page-shared.less';

.recommend-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.recommend-card__item {
  background: rgba(11, 65, 205, 0.04);
  border-radius: 10px;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;

  &:not(:last-child) {
    margin-bottom: 12px;
  }
}

.recommend-card__heading {
  font-size: 14px;
  font-weight: 600;
}

.recommend-card__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.recommend-card__field--row {
  flex-direction: row;
  align-items: center;
  gap: 12px;
}

.recommend-card__label {
  font-size: 13px;
  color: #21201f;
}

.recommend-card__error {
  margin: 0;
  font-size: 13px;
  color: #d54941;
}
</style>
