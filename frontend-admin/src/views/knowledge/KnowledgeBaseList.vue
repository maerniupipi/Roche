<template>
  <div class="kb-list-page">
    <!-- 顶部：面包屑 + 操作按钮 -->
    <div class="kb-list-header">
      <KnowledgeBreadcrumb :segments="breadcrumbSegments" @navigate="onBreadcrumbNavigate" />
      <div class="kb-list-header__actions">
        <t-button v-if="authStore.canManageKnowledge" variant="outline" theme="default" data-guide="kb-create-domain"
          @click="showCreateDomainDialog = true">
          <template #icon><t-icon name="add" /></template>
          {{ $t('knowledgeList.createDomain') }}
        </t-button>
        <t-button theme="primary" variant="outline" data-guide="kb-create-button" @click="handleCreateKnowledgeBase">
          <template #icon><t-icon name="folder-add" size="16px" /></template>
          {{ $t('knowledgeList.create') }}
        </t-button>
      </div>
    </div>

    <div class="kb-list-content">
      <!-- 加载骨架屏（与 AgentList 一致的"卡片套骨架"三段式） -->
      <div v-if="loading" class="kb-list-grid">
        <div v-for="i in 6" :key="i" class="kb-card kb-card-skeleton">
          <div class="kb-card__header">
            <t-skeleton animation="gradient"
              :row-col="[[{ width: '32px', height: '32px', type: 'circle' }, { width: '50%', height: '18px' }]]" />
          </div>
          <div class="kb-card__description-skel">
            <t-skeleton animation="gradient"
              :row-col="[{ width: '100%', height: '12px' }, { width: '70%', height: '12px' }]" />
          </div>
          <div class="kb-card__divider" aria-hidden="true"></div>
          <div class="kb-card__actions">
            <t-skeleton animation="gradient"
              :row-col="[[{ width: '50%', height: '28px', type: 'rect' }, { width: '50%', height: '28px', type: 'rect' }]]" />
          </div>
        </div>
      </div>

      <!-- 空状态 -->
      <div v-else-if="visibleKnowledgeBases.length === 0" class="kb-list-empty">
        <div class="kb-list-empty__inner">
          <img class="kb-list-empty__icon" src="@/assets/img/upload.svg" alt="">
          <h3 class="kb-list-empty__title">{{ $t('knowledgeList.empty.title') }}</h3>
          <p class="kb-list-empty__description">{{ $t('knowledgeList.empty.description') }}</p>
          <t-button theme="primary" size="medium" @click="handleCreateKnowledgeBase">
            <template #icon><t-icon name="folder-add" size="16px" /></template>
            {{ $t('knowledgeList.create') }}
          </t-button>
        </div>
      </div>

      <!-- 卡片网格（所有知识库统一展示，去掉分组与我的子视图） -->
      <div v-else class="kb-list-grid">
        <div v-for="kb in visibleKnowledgeBases" :key="kb.id" ref="kbCardRefs"
          :class="['kb-card', { 'kb-card--highlight': highlightedKbId === kb.id }]" :data-kb-id="kb.id">
          <div class="kb-card__header">
            <h3 class="kb-card__title">
              <t-tooltip v-if="kb.name && kb.name.length > 16" :content="kb.name" placement="top">
                <span>{{ kb.name }}</span>
              </t-tooltip>
              <span v-else>{{ kb.name }}</span>
            </h3>
            <p class="kb-card__description">
              {{ kb.description || $t('knowledgeList.card.noDescription') }}
            </p>
          </div>

          <div class="kb-card__divider" aria-hidden="true"></div>

          <div class="kb-card__actions">
            <t-button variant="outline" theme="default" class="kb-card__action-btn" @click="handleAccess(kb)">
              {{ $t('knowledgeList.actions.access') }}
            </t-button>
            <t-button variant="outline" theme="default" class="kb-card__action-btn" @click="handleManage(kb)">
              {{ $t('knowledgeList.actions.manage') }}
            </t-button>
          </div>
        </div>
      </div>

      <!-- 用户授权弹窗 -->
      <KnowledgeBaseAccessDialog v-if="accessVisible" v-model:visible="accessVisible"
        :knowledge-base="currentAccessKB" />

      <!-- 知识域创建弹窗 -->
      <KnowledgeDomainCreateDialog v-if="showCreateDomainDialog" v-model:visible="showCreateDomainDialog"
        @success="handleKnowledgeDomainCreated" />

      <!-- 简化版新建知识库弹窗 -->
      <KnowledgeBaseCreateSimpleModal v-if="simpleCreateVisible && currentDomainId != null"
        v-model:visible="simpleCreateVisible" :knowledge-domain-id="currentDomainId ?? undefined"
        @success="handleKBCreateSuccess" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import KnowledgeBreadcrumb from '@/components/KnowledgeBreadcrumb.vue'
import KnowledgeBaseAccessDialog from '@/views/knowledge/components/KnowledgeBaseAccessDialog.vue'
import KnowledgeDomainCreateDialog from '@/views/knowledge/components/KnowledgeDomainCreateDialog.vue'
import KnowledgeBaseCreateSimpleModal from '@/views/knowledge/components/KnowledgeBaseCreateSimpleModal.vue'

import { useChatResourcesStore } from '@/stores/chatResources'
import { useAuthStore } from '@/stores/auth'
import { useKnowledgeDomains } from '@/composables/useKnowledgeDomains'
import { useResourcePins } from '@/composables/useResourcePins'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const chatResources = useChatResourcesStore()
const { domains, load: loadDomains } = useKnowledgeDomains()
const pins = useResourcePins()

// 路由 props 注入的 domainId（由 /platform/knowledge-bases/domain/:domainId 提供）
const props = defineProps<{ domainId?: string | number }>()

const currentDomainId = computed<number | null>(() => {
  if (props.domainId == null) return null
  const n = Number(props.domainId)
  return Number.isFinite(n) ? n : null
})
const currentDomainName = computed(() => {
  const id = currentDomainId.value
  if (id == null) return ''
  return domains.value.find((d) => d.id === id)?.name || ''
})

// 面包屑 segments
// 列表子路由（/platform/knowledge-bases/domain/:domainId）：root + domain 段。
// domain 段在当前域名尚未从缓存加载完时显示 loading 占位，避免刷新瞬间整段消失。
const breadcrumbSegments = computed(() => {
  const id = currentDomainId.value
  if (id == null) {
    return [{ key: 'root', label: t('knowledgeList.root') || '知识库', disabled: true }]
  }
  const name = currentDomainName.value
  return [
    { key: 'root', label: t('knowledgeList.root') || '知识库', disabled: false },
    { key: `domain-${id}`, label: name || '', loading: !name, disabled: true },
  ]
})

// ============ 加载与列表 ============
const knowledgeBases = ref<any[]>([])
const loading = ref(false)
const highlightedKbId = ref<string | null>(null)

const visibleKnowledgeBases = computed(() => {
  if (currentDomainId.value == null) return knowledgeBases.value
  return knowledgeBases.value.filter((kb: any) => {
    const kbDomainId = kb.knowledge_domain_id ?? kb.domain_id ?? null
    return kbDomainId == null ? true : String(kbDomainId) === String(currentDomainId.value)
  })
})

async function fetchList(force = false) {
  loading.value = true
  try {
    const data = await chatResources.fetchKnowledgeBasesForList({ creator: 'all' }, force)
    knowledgeBases.value = Array.isArray(data) ? data : []
  } finally {
    loading.value = false
  }
}

// ============ 生命周期 ============
onMounted(async () => {
  // 先确保知识域列表已就绪，再请求 KB 列表：
  // - 面包屑依赖 currentDomainName（来自缓存），不 await 会导致刷新瞬间 domain 段消失。
  // - fetchList 返回后要做 knowledge_domain_id 过滤，缓存就绪可保证过滤生效。
  await loadDomains()
  await fetchList()
  // 处理从路由进入时的高亮 ID
  if (route.query.highlightKbId) {
    const kbId = String(route.query.highlightKbId)
    nextTick(() => triggerHighlightFlash(kbId))
  }
})

const onBreadcrumbNavigate = (seg: { key: string }) => {
  if (seg.key === 'root') {
    router.push('/platform/knowledge-bases')
  }
}

watch(
  () => currentDomainId.value,
  () => {
    // 知识域切换时列表会因 visibleKnowledgeBases 重新计算；缓存命中，无需强制刷新
  },
)

// ============ 卡片交互 ============
const kbCardRefs = ref<HTMLElement[]>([])
const accessVisible = ref(false)
const currentAccessKB = ref<any | null>(null)
const showCreateDomainDialog = ref(false)
const simpleCreateVisible = ref(false)
const highlightedCardRef = ref<HTMLElement | null>(null)

const handleAccess = (kb: any) => {
  currentAccessKB.value = kb
  accessVisible.value = true
}

const handleManage = (kb: any) => {
  const kbId = String(kb.id)
  pins.touchRecent('kb', kbId)
  router.push(`/platform/knowledge-bases/${kbId}`)
}

// ============ 创建 / 编辑 / 高亮 ============
const handleCreateKnowledgeBase = () => {
  if (currentDomainId.value == null) {
    MessagePlugin.warning(t('knowledgeEditor.messages.domainRequired'))
    return
  }
  simpleCreateVisible.value = true
}

const handleKnowledgeDomainCreated = async (domain: { id: number }) => {
  window.dispatchEvent(new CustomEvent('knowledge-domain-changed'))
  await router.push({
    name: 'knowledgeBaseListByDomain',
    params: { domainId: String(domain.id) },
  })
}

const handleKBCreateSuccess = (kbId: string) => {
  if (!kbId) return
  chatResources.invalidateKnowledgeBaseDetail(kbId)
  fetchList(true).then(() => {
    if (route.query.highlightKbId === kbId) {
      triggerHighlightFlash(kbId)
      const { highlightKbId: _drop, ...rest } = route.query
      router.replace({ query: rest })
    } else {
      // 默认行为：跳到新创建 KB 的详情页，便于继续上传文档
      router.push(`/platform/knowledge-bases/${kbId}`)
    }
  })
}

const triggerHighlightFlash = (kbId: string) => {
  highlightedKbId.value = kbId
  nextTick(() => {
    const target = kbCardRefs.value.find((el) => el?.dataset?.kbId === kbId)
    if (target) {
      target.scrollIntoView({ behavior: 'smooth', block: 'center' })
      highlightedCardRef.value = target
    }
    setTimeout(() => {
      highlightedKbId.value = null
    }, 3000)
  })
}

</script>

<style scoped lang="less">
.kb-list-page {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.kb-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 24px 6px;
}

.kb-list-header__actions {
  display: flex;
  gap: 8px;
}

.kb-list-content {
  flex: 1;
  overflow-y: auto;
  padding: 10px 24px 20px;
}

.kb-list-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
}

.kb-list-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 360px;
}

.kb-list-empty__inner {
  display: flex;
  flex-flow: column;
  justify-content: center;
  align-items: center;
}

.kb-list-empty__icon {
  width: 162px;
  height: 162px;
}

.kb-list-empty__title {
  margin: 12px 0 8px 0;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 16px;
  font-weight: 600;
  line-height: 26px;
}

.kb-list-empty__description {
  margin: 0 0 20px 0;
  color: var(--td-text-color-placeholder);
  font-family: var(--app-font-family);
  font-size: 13px;
  font-weight: 400;
  line-height: 20px;
  text-align: center;
}

.kb-card {
  position: relative;
  display: flex;
  flex-direction: column;
  // background: linear-gradient(115.91deg, #ffffff 6.44%, rgba(215, 235, 251, 0.3) 82.93%);
  background: url('@/assets/img/card-bg.png') no-repeat center center / cover;
  border: 1px solid #e7e6e4;
  border-radius: 8px;
  padding: 14px 14px 12px;
  overflow: hidden;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;

  &>* {
    position: relative;
    z-index: 1;
  }

  &:hover {
    border-color: #c8d4ff;
    box-shadow: 0 8px 24px rgba(11, 65, 205, 0.12);
    transform: translateY(-2px);
  }

  &--highlight {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 2px var(--td-brand-color-light);
    transition: box-shadow 0.3s ease;
  }
}

.kb-card__header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.kb-card__title {
  margin: 0;
  font-family: var(--app-font-family);
  font-size: 16px;
  font-weight: 700;
  line-height: 24px;
  color: #21201f;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;

  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-block;
    max-width: 100%;
  }
}

.kb-card__description {
  margin: 0;
  font-family: var(--app-font-family);
  font-size: 12px;
  line-height: 18px;
  color: #706b69;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.kb-card__divider {
  height: 1px;
  margin: 10px 0;
  background-image: linear-gradient(to right,
      rgba(231, 230, 228, 1) 0,
      rgba(231, 230, 228, 1) 2px,
      transparent 2px,
      transparent 4px);
  background-size: 4px 1px;
  background-repeat: repeat-x;
}

.kb-card__actions {
  display: flex;
  gap: 8px;
}

.kb-card-skeleton {
  // 骨架态：去掉装饰背景与 hover 交互，与真实卡片保持同尺寸同边框
  cursor: default;
  background: var(--td-bg-color-container);
  pointer-events: none;

  &:hover {
    border-color: #e7e6e4;
    box-shadow: none;
    transform: none;
  }

  // 关闭真实卡片的装饰圆背景
  &::after {
    display: none;
  }

  .kb-card__description-skel {
    margin-top: 8px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
}

.kb-card__action-btn {
  flex: 1;
  border-radius: 9px;
  background: transparent;
  font-size: 12px;
}

@media (max-width: 720px) {
  .kb-list-grid {
    grid-template-columns: 1fr;
  }
}
</style>