<template>
  <t-select
    :value="modelValue ?? undefined"
    :loading="loading"
    :options="options"
    :placeholder="$t('knowledgeEditor.messages.domainRequired')"
    class="knowledge-domain-selector"
    @change="handleChange"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { listKnowledgeDomains, type KnowledgeDomainInfo } from '@/api/knowledge-domain'

const props = defineProps<{
  modelValue: number | null
  variant?: 'toolbar'
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: number | null): void
}>()

const loading = ref(false)
const domains = ref<KnowledgeDomainInfo[]>([])
const options = computed(() =>
  domains.value.map(domain => ({ label: domain.name, value: domain.id })),
)

const handleChange = (value: string | number) => {
  const id = Number(value)
  emit('update:modelValue', Number.isFinite(id) && id > 0 ? id : null)
}

onMounted(async () => {
  loading.value = true
  try {
    const response = await listKnowledgeDomains()
    if (!response.success) {
      throw new Error(response.message || 'Failed to load knowledge domains')
    }
    domains.value = response.data?.items || []
    if (!props.modelValue && domains.value.length > 0) {
      emit('update:modelValue', domains.value[0].id)
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || 'Failed to load knowledge domains')
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.knowledge-domain-selector {
  width: 240px;
}
</style>
