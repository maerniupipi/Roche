<!--
  Global inline document preview dialog.

  Mounted once at the chat surface level (see `views/chat/index.vue`). Both
  `<ChatCitationFloat>` and `<ChatReferencesDrawer>` call
  `useInlineDocumentPreview().open(...)` to trigger this dialog, so there is
  only one dialog instance regardless of which UI surface initiates it.

  The dialog itself is teleported to <body> because the floating surfaces
  that open it are also teleported; we want the modal to sit above every
  other surface including any active floating panels.
-->
<template>
  <Teleport to="body">
    <t-dialog v-if="ctx" v-model:visible="ctx.visible.value" placement="center" :width="840"
      :footer="false" :close-on-overlay-click="false" class="inline-document-preview-dialog" @close="ctx.close()">
      <template #header>
        <span class="inline-document-preview-dialog__title">
          {{ ctx.item.value?.fileName || t('chat.inlineDocumentPreviewTitle') }}
        </span>
      </template>
      <div v-if="ctx.loading.value" class="inline-document-preview-dialog__loading">
        <t-loading size="medium" />
        <span class="inline-document-preview-dialog__loading-text">{{ t('preview.loading') }}</span>
      </div>
      <DocumentPreview v-else-if="ctx.visible.value && ctx.item.value" :key="ctx.item.value.knowledgeId"
        :knowledge-id="ctx.item.value.knowledgeId" :file-name="ctx.item.value.fileName"
        :file-type="ctx.item.value.fileType" :active="ctx.visible.value" />
    </t-dialog>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import DocumentPreview from '@/components/document-preview.vue'
import { useInlineDocumentPreview } from '@/composables/useInlineDocumentPreview'

const ctx = useInlineDocumentPreview()
const { t } = useI18n()
</script>

<style lang="less">
.inline-document-preview-dialog {
  .t-dialog__header {
    padding-bottom: 16px;
    font-weight: 600;
    text-align: center !important;
    border-bottom: 1px solid var(--td-component-stroke, #e7e7e7) !important;
  }

  .t-dialog__header-content {
    min-width: 0;
    width: calc(100% - 48px);
    overflow: hidden;
    justify-content: center;
  }

  &__title {
    display: block;
    width: 100%;
    min-width: 0;
    overflow: hidden;
    text-align: center;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.inline-document-preview-dialog__loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 80px 20px;
  color: var(--td-text-color-placeholder);
}

.inline-document-preview-dialog__loading-text {
  font-size: 13px;
}
</style>