<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="kb-create-simple-overlay" @click.self="handleClose">
        <div class="kb-create-simple-modal" role="dialog" aria-modal="true">
          <!-- 关闭按钮 -->
          <button class="kb-create-simple-modal__close" type="button" :aria-label="$t('general.close')"
            @click="handleClose">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
          </button>

          <h2 class="kb-create-simple-modal__title">{{ $t('knowledgeList.create') }}</h2>
          <p class="kb-create-simple-modal__subtitle">
            {{ $t('knowledgeList.simpleModal.subtitle') }}
          </p>

          <div class="kb-create-simple-modal__form">
            <div class="kb-create-simple-modal__form-item">
              <label class="kb-create-simple-modal__label required">
                {{ $t('knowledgeList.simpleModal.nameLabel') }}
              </label>
              <t-input v-model="name" :placeholder="$t('knowledgeList.simpleModal.namePlaceholder')"
                :maxlength="50" :disabled="saving" clearable @enter="handleCreate" />
            </div>
            <div class="kb-create-simple-modal__form-item">
              <label class="kb-create-simple-modal__label">
                {{ $t('knowledgeList.simpleModal.descriptionLabel') }}
              </label>
              <t-textarea v-model="description" :placeholder="$t('knowledgeList.simpleModal.descriptionPlaceholder')"
                :maxlength="200" :autosize="{ minRows: 3, maxRows: 6 }" :disabled="saving" />
            </div>
          </div>

          <div class="kb-create-simple-modal__footer">
            <t-button variant="outline" theme="default" :disabled="saving" @click="handleClose">
              {{ $t('common.cancel') }}
            </t-button>
            <t-button theme="primary" :loading="saving" :disabled="!canSubmit" @click="handleCreate">
              {{ $t('knowledgeList.simpleModal.submit') }}
            </t-button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { createKnowledgeBase } from '@/api/knowledge-base'
import { useChatResourcesStore } from '@/stores/chatResources'
import { useEditorResourcesStore } from '@/stores/editorResources'

const props = defineProps<{
  visible: boolean
  /** 必填：当前路由对应的知识域 ID。新建知识库必须归属到某个知识域。 */
  knowledgeDomainId?: number
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'success', kbId: string): void
}>()

const { t } = useI18n()
const chatResources = useChatResourcesStore()
const editorResources = useEditorResourcesStore()

const name = ref('')
const description = ref('')
const saving = ref(false)

const canSubmit = computed(
  () => !saving.value && !!props.knowledgeDomainId && name.value.trim().length > 0,
)

const resetForm = () => {
  name.value = ''
  description.value = ''
}

const handleClose = () => {
  if (saving.value) return
  emit('update:visible', false)
}

// 打开时刷新默认值（models / storage），关闭时 300ms 后清表单（与现编辑器风格一致）
watch(
  () => props.visible,
  async (val) => {
    if (val) {
      resetForm()
      try {
        await Promise.all([
          chatResources.ensureModels(),
          editorResources.ensureStorageEngine(),
        ])
      } catch (e) {
        console.warn('[KnowledgeBaseCreateSimpleModal] failed to refresh defaults', e)
      }
    } else {
      setTimeout(resetForm, 300)
    }
  },
  { immediate: true },
)

const pickDefaultModelId = (
  type: 'Embedding' | 'KnowledgeQA',
): string | undefined => {
  const list = chatResources.allModels || []
  const matched = list.filter((m: any) => m.type === type)
  const def = matched.find((m: any) => m.is_default)
  return def?.id ?? matched[0]?.id ?? undefined
}

const buildSubmitData = () => {
  const embeddingModelId = pickDefaultModelId('Embedding')
  const summaryModelId = pickDefaultModelId('KnowledgeQA')
  const provider = editorResources.storageConfig?.default_provider || 'local'

  return {
    knowledge_domain_id: props.knowledgeDomainId as number,
    name: name.value.trim(),
    description: description.value.trim() || undefined,
    type: 'document' as const,
    embedding_model_id: embeddingModelId,
    summary_model_id: summaryModelId,
    chunking_config: {
      chunk_size: 512,
      chunk_overlap: 80,
      separators: ['\n\n', '\n', '。', '！', '？', ';', '；'],
      enable_parent_child: true,
      parent_chunk_size: 4096,
      child_chunk_size: 384,
      strategy: 'auto',
      token_limit: 0,
      languages: [],
    },
    storage_provider_config: { provider },
    storage_config: { provider }, // legacy dual-write
    indexing_strategy: {
      vector_enabled: true,
      keyword_enabled: true,
      graph_enabled: false,
    },
    vlm_config: { enabled: false, model_id: '' },
    asr_config: { enabled: false, model_id: '', language: '' },
    extract_config: {
      enabled: false,
      text: '',
      tags: [],
      nodes: [],
      relations: [],
    },
    question_generation_config: { enabled: true, question_count: 3 },
  }
}

const handleCreate = async () => {
  if (!canSubmit.value) {
    if (!props.knowledgeDomainId) {
      MessagePlugin.warning(t('knowledgeEditor.messages.domainRequired'))
      emit('update:visible', false)
    }
    return
  }

  const data = buildSubmitData()
  if (!data.embedding_model_id || !data.summary_model_id) {
    MessagePlugin.warning(t('knowledgeList.simpleModal.modelsMissing'))
    return
  }

  saving.value = true
  try {
    const res: any = await createKnowledgeBase(data as any)
    const kbId = res?.data?.id ?? res?.data?.kb_id ?? res?.data?.knowledge_base_id
    if (res?.success && kbId) {
      MessagePlugin.success(t('knowledgeEditor.messages.createSuccess'))
      emit('success', String(kbId))
      emit('update:visible', false)
    } else {
      MessagePlugin.error(res?.message || t('knowledgeEditor.messages.createFailed'))
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('knowledgeEditor.messages.createFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped lang="less">
.kb-create-simple-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  z-index: 2500;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.kb-create-simple-modal {
  position: relative;
  width: 100%;
  max-width: 460px;
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.18);
  padding: 28px 28px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.kb-create-simple-modal__close {
  position: absolute;
  top: 12px;
  right: 12px;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }
}

.kb-create-simple-modal__title {
  margin: 0;
  padding-right: 32px;
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 18px;
  font-weight: 600;
  line-height: 26px;
}

.kb-create-simple-modal__subtitle {
  margin: -8px 0 4px;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 13px;
  line-height: 20px;
}

.kb-create-simple-modal__form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.kb-create-simple-modal__form-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.kb-create-simple-modal__label {
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 14px;
  font-weight: 500;
  line-height: 22px;

  &.required::before {
    content: '*';
    color: var(--td-error-color);
    margin-right: 4px;
  }
}

.kb-create-simple-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 12px;
  margin-top: 4px;
  border-top: 1px solid var(--td-component-stroke);
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;

  .kb-create-simple-modal {
    transition: transform 0.2s ease, opacity 0.2s ease;
  }
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;

  .kb-create-simple-modal {
    transform: scale(0.96);
    opacity: 0;
  }
}

@media (max-width: 520px) {
  .kb-create-simple-modal {
    padding: 24px 20px 16px;
  }
}
</style>