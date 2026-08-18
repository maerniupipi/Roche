<template>
  <t-dialog :visible="visible" :header="headerText" :confirm-btn="{
    content: $t('roles.knowledgeOfficer.submit'),
    loading: submitting,
  }" :cancel-btn="$t('roles.knowledgeOfficer.cancel')" :on-confirm="handleConfirm" :on-close="handleCloseAttempt"
    :on-cancel="handleCloseAttempt" width="560px" @update:visible="emit('update:visible', $event)">
    <p class="dialog-subtitle">{{ subtitleText }}</p>

    <div class="scope-form">
      <div class="scope-field">
        <label>{{ $t('roles.knowledgeOfficer.domainLabel') }}</label>
        <t-select v-model="selectedDomainIds" :options="domainOptions" :loading="domainsLoading"
          :placeholder="$t('roles.knowledgeOfficer.domainPlaceholder')" filterable clearable multiple
          class="full-width-input">
          <template #empty>
            <p class="empty-hint">{{ $t('roles.knowledgeOfficer.noDomains') }}</p>
          </template>
        </t-select>
      </div>

      <div class="scope-field">
        <label>{{ $t('roles.knowledgeOfficer.baseLabel') }}</label>
        <t-select v-model="selectedBaseIds" :options="baseOptions" :loading="loadingBases"
          :placeholder="$t('roles.knowledgeOfficer.basePlaceholder')" filterable clearable multiple
          class="full-width-input">
          <template #empty>
            <p class="empty-hint">
              {{ $t('roles.knowledgeOfficer.noBases') }}
            </p>
          </template>
        </t-select>
      </div>

      <p v-if="selectedDomainIds.length === 0 && selectedBaseIds.length === 0" class="warning-hint">
        {{ $t('roles.knowledgeOfficer.emptyScopeWarning') }}
      </p>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { useKnowledgeDomains } from '@/composables/useKnowledgeDomains'

const props = withDefaults(
  defineProps<{
    visible: boolean
    userName?: string
    defaultDomainIds?: string[]
    defaultBaseIds?: string[]
    mode?: 'single' | 'batch'
    userCount?: number
  }>(),
  {
    mode: 'single',
    userCount: 0,
  },
)

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'confirm', payload: { domainIds: string[]; baseIds: string[] }): void
  (event: 'cancel'): void
}>()

const { t } = useI18n()

const headerText = computed(() =>
  props.mode === 'batch'
    ? t('roles.knowledgeOfficer.batchScopeTitle')
    : t('roles.knowledgeOfficer.scopeTitle'),
)
const subtitleText = computed(() =>
  props.mode === 'batch'
    ? t('roles.knowledgeOfficer.batchScopeSubtitle', { count: props.userCount ?? 0 })
    : t('roles.knowledgeOfficer.scopeSubtitle', { name: props.userName ?? '' }),
)

const { domains, loading: domainsLoading, load } = useKnowledgeDomains()

const allBases = ref<Array<{ id: string; name: string; knowledge_domain_id?: number | string }>>([])
const loadingBases = ref(false)
const selectedDomainIds = ref<string[]>([])
const selectedBaseIds = ref<string[]>([])
const submitting = ref(false)

// t-select multiple 的 options 形态：value 统一为字符串，与 selectedDomainIds / selectedBaseIds 类型一致。
const domainOptions = computed(() =>
  domains.value.map((d) => ({ label: d.name, value: String(d.id) })),
)
// 域 id → 域名的查表，用来给知识库选项拼前缀。
const domainNameById = computed(() => {
  const map = new Map<string, string>()
  for (const d of domains.value) map.set(String(d.id), d.name)
  return map
})
// 多域场景下库名可能同名，label 拼上「[域] - [库]」帮助区分；
// 单域 / 未选域时直接展示库名，避免冗余。
const baseOptions = computed(() => {
  const showDomain = selectedDomainIds.value.length > 1
  const names = domainNameById.value
  return filteredBases.value.map((kb) => {
    const domId = kb.knowledge_domain_id == null ? null : String(kb.knowledge_domain_id)
    const domainName = domId != null ? names.get(domId) : undefined
    const label = showDomain && domainName ? `${domainName} - ${kb.name}` : kb.name
    return { label, value: String(kb.id) }
  })
})

const filteredBases = computed(() => {
  if (selectedDomainIds.value.length === 0) return allBases.value
  const picked = new Set(selectedDomainIds.value)
  return allBases.value.filter((kb) => {
    const domId = kb.knowledge_domain_id == null ? null : String(kb.knowledge_domain_id)
    return domId != null && picked.has(domId)
  })
})

watch(
  () => props.visible,
  async (visible) => {
    if (!visible) return
    selectedDomainIds.value = [...(props.defaultDomainIds ?? [])]
    selectedBaseIds.value = [...(props.defaultBaseIds ?? [])]
    submitting.value = false
    await load()
    await fetchAllBases()
  },
)

// 选域变化时清空库选择：避免范围错位（保留在所选域里的库也由过滤生效）。
watch(selectedDomainIds, () => {
  const allowed = new Set(filteredBases.value.map((kb) => String(kb.id)))
  selectedBaseIds.value = selectedBaseIds.value.filter((id) => allowed.has(id))
})

async function fetchAllBases(): Promise<void> {
  loadingBases.value = true
  try {
    const res: any = await listKnowledgeBases()
    const raw: any[] = res?.data && Array.isArray(res.data) ? res.data : []
    allBases.value = raw
      .map((kb: any) => ({
        id: String(kb.id),
        name: kb.name ?? String(kb.id),
        knowledge_domain_id: kb.knowledge_domain_id ?? kb.domain_id ?? null,
      }))
      .filter((kb) => kb.id.length > 0)
  } catch (e) {
    allBases.value = []
  } finally {
    loadingBases.value = false
  }
}

async function handleConfirm(): Promise<void> {
  if (submitting.value) return
  submitting.value = true
  try {
    emit('confirm', {
      domainIds: [...selectedDomainIds.value],
      baseIds: [...selectedBaseIds.value],
    })
  } finally {
    submitting.value = false
  }
}

function handleCloseAttempt(): void {
  if (submitting.value) return
  emit('cancel')
  emit('update:visible', false)
}
</script>

<style scoped lang="less">
.dialog-subtitle {
  margin: 0 0 20px;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
}

.scope-form {
  display: grid;
  gap: 18px;
}

.scope-field {
  display: grid;
  gap: 8px;

  label {
    color: var(--td-text-color-primary);
    font-weight: 500;
  }
}

.full-width-input {
  width: 100%;
}

.empty-hint {
  margin: 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.warning-hint {
  margin: 0;
  color: var(--td-warning-color);
  font-size: 13px;
}
</style>