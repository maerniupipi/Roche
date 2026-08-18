<template>
  <t-dialog
    :visible="visible"
    :header="$t('knowledgeDomain.create.dialogTitle')"
    :confirm-btn="{
      content: $t('knowledgeDomain.create.submit'),
      loading: saving,
    }"
    :cancel-btn="$t('knowledgeDomain.create.cancel')"
    width="520px"
    @update:visible="emit('update:visible', $event)"
    @confirm="handleConfirm"
  >
    <p class="dialog-subtitle">{{ $t('knowledgeDomain.create.dialogSubtitle') }}</p>
    <div class="domain-form">
      <label for="knowledge-domain-name">
        {{ $t('knowledgeDomain.create.nameLabel') }}
        <span class="required">*</span>
      </label>
      <t-input
        id="knowledge-domain-name"
        v-model="form.name"
        :maxlength="128"
        :placeholder="$t('knowledgeDomain.create.namePlaceholder')"
        @enter="handleConfirm"
      />

      <label for="knowledge-domain-description">
        {{ $t('knowledgeDomain.create.descriptionLabel') }}
      </label>
      <t-textarea
        id="knowledge-domain-description"
        v-model="form.description"
        :maxlength="512"
        :autosize="{ minRows: 3, maxRows: 6 }"
        :placeholder="$t('knowledgeDomain.create.descriptionPlaceholder')"
      />
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  createKnowledgeDomain,
  type KnowledgeDomainInfo,
} from '@/api/knowledge-domain'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'created', value: KnowledgeDomainInfo): void
}>()

const { t } = useI18n()
const saving = ref(false)
const form = reactive({
  name: '',
  description: '',
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    form.name = ''
    form.description = ''
  },
)

async function handleConfirm(): Promise<void> {
  const name = form.name.trim()
  if (!name) {
    MessagePlugin.warning(t('knowledgeDomain.create.nameRequired'))
    return
  }

  saving.value = true
  try {
    const response = await createKnowledgeDomain({
      name,
      description: form.description.trim() || undefined,
    })
    if (!response.success || !response.data) {
      throw new Error(response.message || t('knowledgeDomain.create.failed'))
    }
    MessagePlugin.success(t('knowledgeDomain.create.success'))
    emit('created', response.data)
    emit('update:visible', false)
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('knowledgeDomain.create.failed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped lang="less">
.dialog-subtitle {
  margin: 0 0 20px;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
}

.domain-form {
  display: grid;
  gap: 10px;

  label {
    margin-top: 6px;
    color: var(--td-text-color-primary);
    font-weight: 500;
  }
}

.required {
  color: var(--td-error-color);
}
</style>
