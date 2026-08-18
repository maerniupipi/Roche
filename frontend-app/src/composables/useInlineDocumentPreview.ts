import { inject, provide, ref, type InjectionKey, type Ref } from 'vue'
import { MessagePlugin } from 'tdesign-mobile-vue'
import { getKnowledgeDetails } from '@/api/knowledge-base'

export type InlineDocumentPreviewEntry = {
  knowledgeId: string
  fileName: string
  fileType: string
}

export type InlineDocumentPreviewOpenOptions = {
  knowledgeId: string
  fallbackFileName?: string
  fallbackFileType?: string
}

export type InlineDocumentPreviewContext = {
  visible: Ref<boolean>
  loading: Ref<boolean>
  item: Ref<InlineDocumentPreviewEntry | null>
  open: (options: InlineDocumentPreviewOpenOptions) => Promise<void>
  close: () => void
}

const INLINE_PREVIEW_FALLBACK_TYPE = ''

const INLINE_DOCUMENT_PREVIEW_KEY: InjectionKey<InlineDocumentPreviewContext> = Symbol(
  'inlineDocumentPreview',
)

export function provideInlineDocumentPreview(): InlineDocumentPreviewContext {
  const visible = ref(false)
  const loading = ref(false)
  const item = ref<InlineDocumentPreviewEntry | null>(null)

  function close() {
    visible.value = false
    // Defer clearing the entry so <DocumentPreview> unmounts gracefully;
    // otherwise the <Watermark>/blob URL release races the dialog disappear
    // animation and leaks blob URLs.
    setTimeout(() => {
      item.value = null
      loading.value = false
    }, 200)
  }

  async function open(options: InlineDocumentPreviewOpenOptions) {
    const { knowledgeId } = options
    if (!knowledgeId) return
    loading.value = true
    visible.value = true
    try {
      const res: any = await getKnowledgeDetails(knowledgeId)
      const data = res?.data ?? res
      const fileName: string =
        data?.file_name ||
        data?.title ||
        options.fallbackFileName ||
        knowledgeId
      const fileType: string =
        data?.file_type ||
        options.fallbackFileType ||
        (typeof fileName === 'string' && fileName.includes('.')
          ? fileName.split('.').pop()!
          : INLINE_PREVIEW_FALLBACK_TYPE)
      item.value = { knowledgeId, fileName, fileType }
    } catch (err: any) {
      visible.value = false
      item.value = null
      MessagePlugin.error(err?.message || 'Failed to load document preview')
    } finally {
      loading.value = false
    }
  }

  const ctx: InlineDocumentPreviewContext = {
    visible,
    loading,
    item,
    open,
    close,
  }

  provide(INLINE_DOCUMENT_PREVIEW_KEY, ctx)
  return ctx
}

export function useInlineDocumentPreview(): InlineDocumentPreviewContext | null {
  return inject(INLINE_DOCUMENT_PREVIEW_KEY, null)
}