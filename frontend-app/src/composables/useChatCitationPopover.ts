import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getChunkByIdOnly } from '@/api/knowledge-base'
import { resolveCitationChunkId, type CitationKnowledgeRef } from '@/utils/citationMarkdown'
import {
  getCitationChunkCache,
  setCitationChunkCache,
} from '@/utils/citationChunkCache'
import { useChatReferencesDrawer } from '@/composables/useChatReferencesDrawer'

export { clearCitationChunkCache } from '@/utils/citationChunkCache'

type FloatState = {
  visible: boolean
  type: 'kb' | 'web'
  top: number
  left: number
  title: string
  content: string
  url: string
  loading: boolean
  error: string
  // Populated for KB citations so the float can offer a "view document" action
  // that opens the shared inline preview dialog. `knowledgeId` is the parent
  // document id (resolved from `knowledge_id` on the matched reference);
  // `fileName` falls back to the title if the API lookup fails.
  knowledgeId: string
  fileName: string
}

export type CitationFloatState = FloatState

export type ChatCitationPopoverOptions = {
  getKnowledgeReferences?: () => CitationKnowledgeRef[] | null | undefined
  sessionId?: () => string | undefined
}

export function useChatCitationPopover(
  rootRef: Ref<HTMLElement | null>,
  options?: ChatCitationPopoverOptions,
) {
  const { t } = useI18n()
  const referencesDrawer = useChatReferencesDrawer()

  const getCacheScope = () => options?.sessionId?.() || 'default'

  const float = ref<FloatState>({
    visible: false,
    type: 'kb',
    top: 0,
    left: 0,
    title: '',
    content: '',
    url: '',
    loading: false,
    error: '',
    knowledgeId: '',
    fileName: '',
  })

  let hoverTimer: number | null = null
  let closeTimer: number | null = null

  const positionFor = (el: HTMLElement, offsetY = 0) => {
    const rect = el.getBoundingClientRect()
    float.value.top = rect.bottom + window.scrollY + 6 + offsetY
    float.value.left = Math.min(rect.left + window.scrollX, window.innerWidth - 320)
  }

  const openWeb = (el: HTMLElement) => {
    const url = el.getAttribute('data-url') || ''
    float.value.type = 'web'
    float.value.url = url
    float.value.title = el.querySelector('.tip-title')?.textContent || ''
    float.value.content = ''
    float.value.loading = false
    float.value.error = ''
    // Clear KB-only fields so the preview button cannot leak across types.
    float.value.knowledgeId = ''
    float.value.fileName = ''
    float.value.visible = true
    positionFor(el)
  }

  const fetchChunkContent = (chunkId: string) => getChunkByIdOnly(chunkId)

  const openKb = async (el: HTMLElement) => {
    const rawChunkId = el.getAttribute('data-chunk-id') || ''
    const title = el.getAttribute('data-doc') || ''
    const kbId = el.getAttribute('data-kb-id') || ''
    const refs = options?.getKnowledgeReferences?.() || []
    const chunkId = resolveCitationChunkId(
      rawChunkId,
      { doc: title, kbId },
      refs,
    ) || rawChunkId
    if (!chunkId) return
    // Resolve the parent document so the inline preview button can fetch it.
    // The model output may reference the chunk by DOC-N / FAQ-N / numeric
    // indexes; resolveCitationChunkId maps those back to a real chunk UUID,
    // and we look that up against `references` to grab `knowledge_id`.
    // Match by chunk id first. Some response shapes expose the same chunk
    // under `chunk_ids` instead of `id`, so also match those identifiers and
    // finally fall back to the document title.
    const matchedRef = refs.find((r) =>
      r?.id === chunkId || (r?.id && r.id === rawChunkId),
    ) || refs.find((r) =>
      title && (r?.knowledge_title === title || r?.knowledge_filename === title),
    )
    const knowledgeId = matchedRef?.knowledge_id || ''
    float.value.type = 'kb'
    float.value.title = title
    float.value.url = ''
    float.value.knowledgeId = knowledgeId
    float.value.fileName =
      matchedRef?.knowledge_title || matchedRef?.knowledge_filename || title
    float.value.visible = true
    positionFor(el, 4)

    const scope = getCacheScope()
    const cached = getCitationChunkCache(scope, chunkId)
    if (cached) {
      // Restore document metadata together with cached content; otherwise a
      // cache hit would skip the lookup and hide the preview button.
      if (cached.knowledgeId) float.value.knowledgeId = cached.knowledgeId
      if (cached.fileName) float.value.fileName = cached.fileName
      float.value.content = cached.content
      float.value.error = cached.error || ''
      float.value.loading = false
      return
    }

    float.value.loading = true
    float.value.error = ''
    float.value.content = ''
    try {
      const res = await fetchChunkContent(chunkId)
      const chunk = res?.data || {}
      // The reference list is not always available or may use a different
      // identifier shape. The chunk endpoint is authoritative for the parent
      // document id, so use it as a fallback for the preview action.
      if (!float.value.knowledgeId && chunk.knowledge_id) {
        float.value.knowledgeId = String(chunk.knowledge_id)
      }
      if (!float.value.fileName) {
        float.value.fileName = String(chunk.knowledge_title || chunk.knowledge_filename || title)
      }
      const content = String(chunk.content || '').trim()
      if (!content) {
        const msg = t('agentStream.citation.notFound')
        setCitationChunkCache(scope, chunkId, {
          content: '',
          error: msg,
          knowledgeId: float.value.knowledgeId,
          fileName: float.value.fileName,
        })
        float.value.error = msg
        return
      }
      setCitationChunkCache(scope, chunkId, {
        content,
        knowledgeId: float.value.knowledgeId,
        fileName: float.value.fileName,
      })
      float.value.content = content
    } catch {
      const msg = t('agentStream.citation.loadFailed')
      setCitationChunkCache(scope, chunkId, { content: '', error: msg })
      float.value.error = msg
    } finally {
      float.value.loading = false
    }
  }

  const scheduleClose = () => {
    if (closeTimer) window.clearTimeout(closeTimer)
    closeTimer = window.setTimeout(() => {
      const hoveredCitation = document.querySelector('.citation-kb:hover, .citation-web:hover')
      const hoveredPopup = document.querySelector('.chat-citation-float:hover')
      if (!hoveredCitation && !hoveredPopup) {
        float.value.visible = false
      }
    }, 120)
  }

  const cancelClose = () => {
    if (closeTimer) {
      window.clearTimeout(closeTimer)
      closeTimer = null
    }
  }

  const onMouseOver = (e: Event) => {
    const target = e.target as HTMLElement
    const kbEl = target.closest?.('.citation-kb') as HTMLElement | null
    const webEl = target.closest?.('.citation-web') as HTMLElement | null
    if (!kbEl && !webEl) return
    cancelClose()
    if (hoverTimer) window.clearTimeout(hoverTimer)
    hoverTimer = window.setTimeout(() => {
      if (kbEl) void openKb(kbEl)
      else if (webEl) openWeb(webEl)
    }, kbEl ? 80 : 40)
  }

  const onMouseOut = (e: Event) => {
    const rt = (e as MouseEvent).relatedTarget as HTMLElement | null
    if (rt?.closest?.('.citation-kb, .citation-web, .chat-citation-float')) return
    if (hoverTimer) {
      window.clearTimeout(hoverTimer)
      hoverTimer = null
    }
    scheduleClose()
  }

  const openDrawerForCitation = (payload: { url?: string; chunkId?: string }) => {
    const refs = options?.getKnowledgeReferences?.() || []
    if (!referencesDrawer || !refs.length) return false
    referencesDrawer.open({
      references: refs,
      highlight: payload,
    })
    return true
  }

  const onClick = (e: Event) => {
    const target = e.target as HTMLElement
    const webEl = target.closest?.('.citation-web') as HTMLElement | null
    if (webEl) {
      e.preventDefault()
      e.stopPropagation()
      const url = webEl.getAttribute('data-url') || ''
      if (openDrawerForCitation({ url })) return
      openWeb(webEl)
      return
    }

    const kbEl = target.closest?.('.citation-kb') as HTMLElement | null
    if (kbEl) {
      e.preventDefault()
      e.stopPropagation()
      const rawChunkId = kbEl.getAttribute('data-chunk-id') || ''
      const title = kbEl.getAttribute('data-doc') || ''
      const kbId = kbEl.getAttribute('data-kb-id') || ''
      const chunkId =
        resolveCitationChunkId(
          rawChunkId,
          { doc: title, kbId },
          options?.getKnowledgeReferences?.(),
        ) || rawChunkId
      if (openDrawerForCitation({ chunkId })) return
      void openKb(kbEl)
    }
  }

  const onViewportChange = () => {
    if (float.value.visible) scheduleClose()
  }

  const bind = () => {
    const root = rootRef.value
    if (!root) return
    root.addEventListener('mouseover', onMouseOver, true)
    root.addEventListener('mouseout', onMouseOut, true)
    root.addEventListener('click', onClick, true)
    window.addEventListener('scroll', onViewportChange, true)
    window.addEventListener('resize', onViewportChange, true)
  }

  const unbind = () => {
    const root = rootRef.value
    if (root) {
      root.removeEventListener('mouseover', onMouseOver, true)
      root.removeEventListener('mouseout', onMouseOut, true)
      root.removeEventListener('click', onClick, true)
    }
    window.removeEventListener('scroll', onViewportChange, true)
    window.removeEventListener('resize', onViewportChange, true)
  }

  watch(rootRef, () => {
    unbind()
    bind()
  }, { flush: 'post' })

  onMounted(() => {
    bind()
  })

  onBeforeUnmount(() => {
    unbind()
    if (hoverTimer) window.clearTimeout(hoverTimer)
    if (closeTimer) window.clearTimeout(closeTimer)
  })

  return { float, rebind: () => { unbind(); bind() }, cancelClose, scheduleClose }
}
