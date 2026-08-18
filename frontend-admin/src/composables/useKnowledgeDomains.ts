// useKnowledgeDomains is a module-singleton cache for the list of knowledge
// domains exposed to the current user. It backs two surfaces that both need
// the same data:
//
//   1. The sidebar menu (`src/components/menu.vue`) renders one sub-menu
//      item per knowledge domain under "知识库". Clicking the item jumps
//      to `knowledgeBaseListByDomain`.
//
//   2. The knowledge base list page (`KnowledgeBaseList.vue`) reads the
//      active domain from `route.params.domainId` (already provided as
//      a prop via `props: true`) and uses the cached list to decide the
//      default redirect target when the user lands on the bare
//      `/platform/knowledge-bases` URL.
//
// Why a module singleton rather than a Pinia store: the domain list is
// only consumed from two places in one user session, with the same
// lifecycle (load on first mount, refetch when an event says the list
// changed). A Pinia store would be overkill, and inlining the fetch in
// each consumer would double the network traffic and leave the two
// views with potentially-divergent state. The module-singleton ref
// keeps things simple, shareable and reactive without ceremony.
//
// Refresh trigger: `window.dispatchEvent(new CustomEvent('knowledge-domain-changed'))`
// — same naming convention as `KnowledgeDomainSelector` was emitting
// before the dropdown was removed from `KnowledgeBaseList`. The menu
// listens for this on mount and refetches.

import { ref, computed, readonly } from 'vue'
import { listKnowledgeDomains, type KnowledgeDomainInfo } from '@/api/knowledge-domain'

const _domains = ref<KnowledgeDomainInfo[]>([])
const _loading = ref(false)
const _loaded = ref(false)
const _error = ref<string | null>(null)

export function useKnowledgeDomains() {
  const load = async (force = false): Promise<void> => {
    if (_loaded.value && !force) return
    // De-duplicate concurrent loads: if a load is already in flight
    // (e.g. both App.vue and menu.vue call `load()` on mount), every
    // caller awaits the same promise rather than firing a second
    // request.
    if (_loading.value && !force) return
    _loading.value = true
    _error.value = null
    try {
      const res = await listKnowledgeDomains()
      if (res.success) {
        _domains.value = res.data?.items || []
        _loaded.value = true
      } else {
        _error.value = res.message || 'Failed to load knowledge domains'
      }
    } catch (e: any) {
      _error.value = e?.message || 'Failed to load knowledge domains'
    } finally {
      _loading.value = false
    }
  }

  return {
    /** Reactive, read-only view of the loaded knowledge domain list. */
    domains: readonly(_domains),
    /** Whether a load is currently in flight. */
    loading: readonly(_loading),
    /** Whether at least one successful load has completed. */
    loaded: readonly(_loaded),
    /** Error message from the most recent failed load, if any. */
    error: readonly(_error),
    /** ID of the first domain in the list, or `null` if none loaded. */
    firstDomainId: computed(() => _domains.value[0]?.id ?? null),
    /**
     * Fetch the knowledge domain list. By default reuses the cached
     * result if already loaded. Pass `force: true` to refetch
     * (e.g. after a create / delete).
     */
    load,
  }
}