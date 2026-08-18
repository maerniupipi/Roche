<template>
  <Teleport to="body">
    <div v-if="float.visible" class="chat-citation-float" :style="{ top: `${float.top}px`, left: `${float.left}px` }"
      @mouseenter="onEnter?.()" @mouseleave="onLeave?.()">
      <template v-if="float.type === 'web'">
        <div class="chat-citation-float__header">
          <div class="chat-citation-float__title" :title="float.title || float.url">{{ float.title || float.url }}</div>
        </div>
        <a v-if="float.url" class="chat-citation-float__link" :href="float.url" target="_blank"
          rel="noopener noreferrer">{{ float.url }}</a>
      </template>
      <template v-else>
        <div class="chat-citation-float__header">
          <div class="chat-citation-float__title" :title="float.title">{{ float.title }}</div>
          <button v-if="float.knowledgeId" type="button" variant="outline" class="chat-citation-float__preview-btn"
            :disabled="isPreviewLoading" @click.stop="openDocumentPreview">
            <span>{{ previewLabel }}</span> <t-icon name="jump" size="14px" />
          </button>
        </div>
        <div v-if="float.loading" class="chat-citation-float__muted">{{ loadingText }}</div>
        <div v-else-if="float.error" class="chat-citation-float__error">{{ float.error }}</div>
        <div v-else class="chat-citation-float__body">{{ float.content }}</div>
      </template>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useInlineDocumentPreview } from '@/composables/useInlineDocumentPreview'
import type { CitationFloatState } from '@/composables/useChatCitationPopover'

const props = defineProps<{
  float: CitationFloatState
  onEnter?: () => void
  onLeave?: () => void
}>()

const { t } = useI18n()
const loadingText = t('common.loading')
const previewLabel = t('chat.viewDocument')

const inlineDocumentPreview = useInlineDocumentPreview()

const isPreviewLoading = computed(() => {
  const ctx = inlineDocumentPreview
  if (!ctx) return false
  return ctx.loading.value && ctx.item.value?.knowledgeId === props.float.knowledgeId
})

async function openDocumentPreview() {
  const ctx = inlineDocumentPreview
  const knowledgeId = props.float.knowledgeId
  if (!ctx || !knowledgeId) return

  // Close the hover float immediately so it does not remain behind the dialog.
  props.float.visible = false
  await ctx.open({
    knowledgeId,
    fallbackFileName: props.float.fileName || props.float.title,
  })
}
</script>

<style lang="less">
@import './css/chat-citations.less';
</style>