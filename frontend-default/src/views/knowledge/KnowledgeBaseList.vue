<template>
  <div class="kb-list-container">
    <ListSpaceSidebar v-if="!authStore.isLiteMode" v-model="spaceSelection" :count-all="allKnowledgeBases"
      :count-mine="kbs.length" :count-favorites="kbFavoritesCount"
      :count-recents="kbRecentsCount" />
    <div class="kb-list-content">
      <div class="header">
        <div class="header-title">
          <div class="title-row">
            <h2>{{ $t('knowledgeBase.title') }}</h2>
            <t-tooltip v-if="authStore.canManageKnowledge" :content="$t('knowledgeList.create')" placement="bottom">
              <t-button variant="text" theme="default" size="small" class="header-action-btn"
                data-guide="kb-list-create" @click="handleCreateKnowledgeBase">
                <template #icon><t-icon name="folder-add" size="16px" /></template>
              </t-button>
            </t-tooltip>
          </div>
          <p class="header-subtitle">{{ $t('knowledgeList.subtitle') }}</p>
        </div>
        <div v-if="authStore.canManageKnowledge" class="knowledge-domain-actions">
          <KnowledgeDomainSelector
            :key="knowledgeDomainSelectorVersion"
            v-model="selectedKnowledgeDomainId"
            variant="toolbar"
          />
          <t-button
            v-if="authStore.isSystemAdmin"
            variant="outline"
            theme="primary"
            @click="knowledgeDomainCreateVisible = true"
          >
            <template #icon><t-icon name="add" /></template>
            {{ $t('knowledgeDomain.create.action') }}
          </t-button>
        </div>
      </div>
      <div class="kb-list-main">
        <!-- 未初始化知识库提示 -->
        <div v-if="hasUninitializedKbs" class="warning-banner">
          <t-icon name="info-circle" size="16px" />
          <span>{{ $t('knowledgeList.uninitializedBanner') }}</span>
        </div>

        <!-- 上传进度提示 -->
        <div v-if="uploadSummaries.length" class="upload-progress-panel">
          <div v-for="summary in uploadSummaries" :key="summary.kbId" class="upload-progress-item">
            <div class="upload-progress-icon">
              <t-icon :name="summary.completed === summary.total ? 'check-circle-filled' : 'upload'" size="20px" />
            </div>
            <div class="upload-progress-content">
              <div class="progress-title">
                {{
                  summary.completed === summary.total
                    ? $t('knowledgeList.uploadProgress.completedTitle', { name: summary.kbName })
                    : $t('knowledgeList.uploadProgress.uploadingTitle', { name: summary.kbName })
                }}
              </div>
              <div class="progress-subtitle">
                {{
                  summary.completed === summary.total
                    ? $t('knowledgeList.uploadProgress.completedDetail', { total: summary.total })
                    : $t('knowledgeList.uploadProgress.detail', { completed: summary.completed, total: summary.total })
                }}
              </div>
              <div class="progress-subtitle secondary">
                {{
                  summary.completed === summary.total
                    ? $t('knowledgeList.uploadProgress.refreshing')
                    : $t('knowledgeList.uploadProgress.keepPageOpen')
                }}
              </div>
              <div v-if="summary.hasError" class="progress-subtitle error">
                {{ $t('knowledgeList.uploadProgress.errorTip') }}
              </div>
              <div class="progress-bar">
                <div class="progress-bar-inner" :style="{ width: summary.progress + '%' }"></div>
              </div>
            </div>
          </div>
        </div>

        <!-- 骨架屏占位 -->
        <div v-if="loading && kbs.length === 0" class="kb-card-wrap">
          <div v-for="n in 6" :key="'skel-' + n" class="kb-card kb-card-skeleton">
            <div class="card-header">
              <t-skeleton animation="gradient" :row-col="[{ width: '60%', height: '20px' }]" />
            </div>
            <div class="card-content">
              <t-skeleton animation="gradient"
                :row-col="[{ width: '100%', height: '14px' }, { width: '80%', height: '14px' }]" />
            </div>
            <div class="card-bottom">
              <t-skeleton animation="gradient"
                :row-col="[[{ width: '28px', height: '28px', type: 'rect' }, { width: '28px', height: '28px', type: 'rect' }]]" />
            </div>
          </div>
        </div>

        <!-- 卡片网格：全部 / 收藏 / 最近 — 共用同一份卡片模板，
             仅依赖 filteredKnowledgeBases 切片即可切换视图 -->
        <div
          v-if="(spaceSelection === 'all' || spaceSelection === 'favorites' || spaceSelection === 'recents') && filteredKnowledgeBases.length > 0"
          class="kb-card-wrap">
          <!-- 置顶分组标题 -->
          <div
            v-if="filteredKnowledgeBases[0] && filteredKnowledgeBases[0].is_pinned"
            class="kb-section-header kb-section-header-pinned" role="button" tabindex="0"
            @click="toggleKbSection('pinned')"
            @keydown.enter.prevent="toggleKbSection('pinned')"
            @keydown.space.prevent="toggleKbSection('pinned')">
            <t-icon name="pin-filled" size="14px" />
            <span>{{ $t('knowledgeList.sections.pinned') }}</span>
            <span class="kb-section-count">{{ filteredKbSectionCounts.pinned }}</span>
            <t-icon class="kb-section-toggle" :name="isKbSectionCollapsed('pinned') ? 'chevron-right' : 'chevron-down'"
              size="14px" />
          </div>
          <!-- 全部：当前用户可访问的知识库。「已置顶」分组由顶部 header
               接管，其余按我创建和其他成员创建分段。 -->
          <template v-for="(kb, index) in filteredKnowledgeBases" :key="kb.id">
            <!-- 我创建的：第一张「我创建」非置顶卡片前打标题，统一展示，
                 不管上方是否存在「已置顶」段，并与其他成员分组保持一致。 -->
            <div v-if="showGroupHeaders
              && isMyKb(kb as KB)
              && !kb.is_pinned
              && (index === 0
                || (filteredKnowledgeBases[index - 1] as any).is_pinned)" class="kb-section-header" role="button"
              tabindex="0" @click="toggleKbSection('mine')"
              @keydown.enter.prevent="toggleKbSection('mine')"
              @keydown.space.prevent="toggleKbSection('mine')">
              <t-icon name="user" size="14px" />
              <span>{{ $t('knowledgeList.sections.mine') }}</span>
              <span class="kb-section-count">{{ filteredKbSectionCounts.mine }}</span>
              <t-icon class="kb-section-toggle" :name="isKbSectionCollapsed('mine') ? 'chevron-right' : 'chevron-down'"
                size="14px" />
            </div>
            <!-- 本部门 · 仅查看：本部门里其他成员创建、对当前 viewer
                 不可编辑。置顶卡片由「已置顶」分组接管。 -->
            <div v-if="showGroupHeaders
              && !isMyKb(kb as KB)
              && !kb.is_pinned
              && (index === 0
                || isMyKb(filteredKnowledgeBases[index - 1] as KB)
                || (filteredKnowledgeBases[index - 1] as any).is_pinned)" class="kb-section-header" role="button"
              tabindex="0" @click="toggleKbSection('knowledgeDomainOthers')"
              @keydown.enter.prevent="toggleKbSection('knowledgeDomainOthers')"
              @keydown.space.prevent="toggleKbSection('knowledgeDomainOthers')">
              <t-icon :name="knowledgeDomainSectionIconName" size="14px" />
              <span>{{ $t(knowledgeDomainSectionLabelKey) }}</span>
              <span class="kb-section-count">{{ filteredKbSectionCounts.knowledgeDomainOthers }}</span>
              <t-icon class="kb-section-toggle"
                :name="isKbSectionCollapsed('knowledgeDomainOthers') ? 'chevron-right' : 'chevron-down'" size="14px" />
            </div>
            <!-- 知识库卡片 -->
            <div v-show="!isKbSectionCollapsed(kbSectionOf(kb))" class="kb-card" :class="{
              'uninitialized': !isInitialized(kb),
              'kb-type-document': (kb.type || 'document') === 'document',
              'kb-type-faq': kb.type === 'faq',
              'highlight-flash': highlightedKbId !== null && highlightedKbId === kb.id
            }"
              :ref="el => { if (highlightedKbId !== null && highlightedKbId === kb.id && el) highlightedCardRef = el as HTMLElement }"
              @click="handleCardClick(kb)">
              <!-- 收藏按钮：右上角浮动；通过 .card-header 的 padding-right
                   给「更多」按钮腾出空间，避免两个按钮叠在一起。 -->
              <button type="button" class="kb-favorite-star" :class="{ 'is-favorited': isKbFavorited(kb.id) }"
                @click.stop="toggleFavoriteKb(kb.id, $event)">
                <t-icon :name="isKbFavorited(kb.id) ? 'star-filled' : 'star'" size="14px" />
              </button>
              <!-- 卡片头部 -->
              <div class="card-header">
                <span class="card-title" :title="kb.name">
                  <span class="card-title-text">{{ kb.name }}</span>
                </span>
                <!-- The card menu always exists when the card is visible: pin
                     is now per-user and available to anyone who can see the KB
                     (backend route only requires KB read access). Settings /
                     Delete are mutations, so they stay behind canManageKBCard. -->
                <t-popup overlayClassName="card-more-popup" trigger="click" destroy-on-close
                  placement="bottom-right">
                  <div class="more-wrap" @click.stop>
                    <img class="more-icon" src="@/assets/img/more.png" alt="" />
                  </div>
                  <template #content>
                    <div class="popup-menu" @click.stop>
                      <div class="popup-menu-item" @click.stop="handleTogglePinById(kb.id)">
                        <t-icon class="menu-icon" :name="kb.is_pinned ? 'pin-filled' : 'pin'" />
                        <span>{{ kb.is_pinned ? $t('knowledgeList.pin.unpin') : $t('knowledgeList.pin.pin') }}</span>
                      </div>
                      <template v-if="canManageKBCard(kb)">
                        <div class="popup-menu-item" @click.stop="handleAccessById(kb.id)">
                          <t-icon class="menu-icon" name="usergroup" />
                          <span>{{ $t('knowledgeAccess.action') }}</span>
                        </div>
                        <div class="popup-menu-item" @click.stop="handleSettingsById(kb.id)">
                          <t-icon class="menu-icon" name="setting" />
                          <span>{{ $t('knowledgeBase.settings') }}</span>
                        </div>
                        <div class="popup-menu-item delete" @click.stop="handleDeleteById(kb.id)">
                          <t-icon class="menu-icon" name="delete" />
                          <span>{{ $t('common.delete') }}</span>
                        </div>
                      </template>
                    </div>
                  </template>
                </t-popup>
              </div>

              <!-- 卡片内容 -->
              <div class="card-content">
                <div class="card-description">
                  {{ kb.description || $t('knowledgeBase.noDescription') }}
                </div>
              </div>

              <!-- 卡片底部 -->
              <div class="card-bottom">
                <div class="bottom-left">
                  <div class="feature-badges">
                    <t-tooltip
                      :content="kb.type === 'faq' ? $t('knowledgeEditor.basic.typeFAQ') : $t('knowledgeEditor.basic.typeDocument')"
                      placement="top">
                      <div class="feature-badge"
                        :class="{ 'type-document': (kb.type || 'document') === 'document', 'type-faq': kb.type === 'faq' }">
                        <t-icon :name="kb.type === 'faq' ? 'chat-bubble-help' : 'folder'" size="14px" />
                        <span class="badge-count">{{ kb.type === 'faq' ? (kb.chunk_count || 0) : (kb.knowledge_count ||
                          0) }}</span>
                        <t-icon v-if="kb.isProcessing" name="loading" size="12px" class="processing-icon" />
                      </div>
                    </t-tooltip>
                    <t-tooltip v-if="kb.extract_config?.enabled" :content="$t('knowledgeList.features.knowledgeGraph')"
                      placement="top">
                      <div class="feature-badge kg">
                        <t-icon name="relation" size="14px" />
                      </div>
                    </t-tooltip>
                    <t-tooltip v-if="kb.vlm_config?.enabled" :content="$t('knowledgeList.features.multimodal')"
                      placement="top">
                      <div class="feature-badge multimodal">
                        <t-icon name="image" size="14px" />
                      </div>
                    </t-tooltip>
                    <t-tooltip v-if="kb.question_generation_config?.enabled"
                      :content="$t('knowledgeList.features.questionGeneration')" placement="top">
                      <div class="feature-badge question">
                        <t-icon name="help-circle" size="14px" />
                      </div>
                    </t-tooltip>
                  </div>
                </div>
              </div>
            </div>

          </template>
        </div>

        <div v-if="spaceSelection === 'mine' && sortedMineKbs.length > 0" class="kb-card-wrap">
          <!-- 置顶分组标题 -->
          <div v-if="sortedMineKbs[0] && sortedMineKbs[0].is_pinned" class="kb-section-header kb-section-header-pinned"
            role="button" tabindex="0" @click="toggleKbSection('pinned')"
            @keydown.enter.prevent="toggleKbSection('pinned')"
            @keydown.space.prevent="toggleKbSection('pinned')">
            <t-icon name="pin-filled" size="14px" />
            <span>{{ $t('knowledgeList.sections.pinned') }}</span>
            <span class="kb-section-count">{{ mineKbSectionCounts.pinned }}</span>
            <t-icon class="kb-section-toggle" :name="isKbSectionCollapsed('pinned') ? 'chevron-right' : 'chevron-down'"
              size="14px" />
          </div>
          <!-- 我的知识库。「已置顶」由顶部 header 接管；其余各分段各打各的
               标题——见「全部」tab 同处注释。 -->
          <template v-for="(kb, index) in sortedMineKbs" :key="kb.id">
            <!-- 我创建的：第一张非置顶的我创建卡片前打标题，无论上方是否
                 有「已置顶」段都要显示，和「本知识域 · 仅查看」对齐——见
                 「全部」tab 同处注释。 -->
            <div v-if="showGroupHeaders
              && isMyKb(kb)
              && !kb.is_pinned
              && (index === 0 || sortedMineKbs[index - 1].is_pinned)" class="kb-section-header" role="button"
              tabindex="0" @click="toggleKbSection('mine')"
              @keydown.enter.prevent="toggleKbSection('mine')"
              @keydown.space.prevent="toggleKbSection('mine')">
              <t-icon name="user" size="14px" />
              <span>{{ $t('knowledgeList.sections.mine') }}</span>
              <span class="kb-section-count">{{ mineKbSectionCounts.mine }}</span>
              <t-icon class="kb-section-toggle" :name="isKbSectionCollapsed('mine') ? 'chevron-right' : 'chevron-down'"
                size="14px" />
            </div>
            <!-- 本知识域 · 仅查看：当前非置顶的同事 KB，且前一张要么不存在、
                 要么是我创建、要么是置顶卡片（置顶→非置顶过渡）。 -->
            <div v-if="showGroupHeaders
              && !isMyKb(kb)
              && !kb.is_pinned
              && (index === 0
                || isMyKb(sortedMineKbs[index - 1])
                || sortedMineKbs[index - 1].is_pinned)" class="kb-section-header" role="button" tabindex="0"
              @click="toggleKbSection('knowledgeDomainOthers')"
              @keydown.enter.prevent="toggleKbSection('knowledgeDomainOthers')"
              @keydown.space.prevent="toggleKbSection('knowledgeDomainOthers')">
              <t-icon :name="knowledgeDomainSectionIconName" size="14px" />
              <span>{{ $t(knowledgeDomainSectionLabelKey) }}</span>
              <span class="kb-section-count">{{ mineKbSectionCounts.knowledgeDomainOthers }}</span>
              <t-icon class="kb-section-toggle"
                :name="isKbSectionCollapsed('knowledgeDomainOthers') ? 'chevron-right' : 'chevron-down'" size="14px" />
            </div>
            <div v-show="!isKbSectionCollapsed(kbSectionOf(kb))" class="kb-card" :class="{
              'uninitialized': !isInitialized(kb),
              'kb-type-document': (kb.type || 'document') === 'document',
              'kb-type-faq': kb.type === 'faq',
              'highlight-flash': highlightedKbId !== null && highlightedKbId === kb.id
            }"
              :ref="el => { if (highlightedKbId !== null && highlightedKbId === kb.id && el) highlightedCardRef = el as HTMLElement }"
              @click="handleCardClick(kb)">
              <button type="button" class="kb-favorite-star" :class="{ 'is-favorited': isKbFavorited(kb.id) }"
                @click.stop="toggleFavoriteKb(kb.id, $event)">
                <t-icon :name="isKbFavorited(kb.id) ? 'star-filled' : 'star'" size="14px" />
              </button>
              <!-- 卡片头部 -->
              <div class="card-header">
                <span class="card-title" :title="kb.name">
                  <span class="card-title-text">{{ kb.name }}</span>
                </span>
                <!-- See the matching block in the "all" tab template for why
                     this is no longer gated by canManageKBCard. -->
                <t-popup v-model="kb.showMore" overlayClassName="card-more-popup"
                  :on-visible-change="onVisibleChange" trigger="click" destroy-on-close placement="bottom-right">
                  <div variant="outline" class="more-wrap" @click.stop="openMore(index)"
                    :class="{ 'active-more': currentMoreIndex === index }">
                    <img class="more-icon" src="@/assets/img/more.png" alt="" />
                  </div>
                  <template #content>
                    <div class="popup-menu" @click.stop>
                      <div class="popup-menu-item" @click.stop="handleTogglePin(kb)">
                        <t-icon class="menu-icon" :name="kb.is_pinned ? 'pin-filled' : 'pin'" />
                        <span>{{ kb.is_pinned ? $t('knowledgeList.pin.unpin') : $t('knowledgeList.pin.pin') }}</span>
                      </div>
                      <template v-if="canManageKBCard(kb)">
                        <div class="popup-menu-item" @click.stop="handleAccess(kb)">
                          <t-icon class="menu-icon" name="usergroup" />
                          <span>{{ $t('knowledgeAccess.action') }}</span>
                        </div>
                        <div class="popup-menu-item" @click.stop="handleSettings(kb)">
                          <t-icon class="menu-icon" name="setting" />
                          <span>{{ $t('knowledgeBase.settings') }}</span>
                        </div>
                        <div class="popup-menu-item delete" @click.stop="handleDelete(kb)">
                          <t-icon class="menu-icon" name="delete" />
                          <span>{{ $t('common.delete') }}</span>
                        </div>
                      </template>
                    </div>
                  </template>
                </t-popup>
              </div>

              <!-- 卡片内容 -->
              <div class="card-content">
                <div class="card-description">
                  {{ kb.description || $t('knowledgeBase.noDescription') }}
                </div>
              </div>

              <!-- 卡片底部 -->
              <div class="card-bottom">
                <div class="bottom-left">
                  <div class="feature-badges">
                    <t-tooltip
                      :content="kb.type === 'faq' ? $t('knowledgeEditor.basic.typeFAQ') : $t('knowledgeEditor.basic.typeDocument')"
                      placement="top">
                      <div class="feature-badge"
                        :class="{ 'type-document': (kb.type || 'document') === 'document', 'type-faq': kb.type === 'faq' }">
                        <t-icon :name="kb.type === 'faq' ? 'chat-bubble-help' : 'folder'" size="14px" />
                        <span class="badge-count">{{ kb.type === 'faq' ? (kb.chunk_count || 0) : (kb.knowledge_count ||
                          0) }}</span>
                        <t-icon v-if="kb.isProcessing" name="loading" size="12px" class="processing-icon" />
                      </div>
                    </t-tooltip>
                    <t-tooltip v-if="kb.extract_config?.enabled" :content="$t('knowledgeList.features.knowledgeGraph')"
                      placement="top">
                      <div class="feature-badge kg">
                        <t-icon name="relation" size="14px" />
                      </div>
                    </t-tooltip>
                    <t-tooltip
                      v-if="kb.vlm_config?.enabled || (kb.storage_provider_config?.provider && kb.storage_provider_config.provider !== 'local')"
                      :content="$t('knowledgeList.features.multimodal')" placement="top">
                      <div class="feature-badge multimodal">
                        <t-icon name="image" size="14px" />
                      </div>
                    </t-tooltip>
                    <t-tooltip v-if="kb.question_generation_config?.enabled"
                      :content="$t('knowledgeList.features.questionGeneration')" placement="top">
                      <div class="feature-badge question">
                        <t-icon name="help-circle" size="14px" />
                      </div>
                    </t-tooltip>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>

        <!-- 全部空状态：保留「新建知识库」CTA。 -->
        <div v-if="spaceSelection === 'all' && filteredKnowledgeBases.length === 0 && !loading" class="empty-state">
          <img class="empty-img" src="@/assets/img/upload.svg" alt="">
          <span class="empty-txt">{{ $t('knowledgeList.empty.title') }}</span>
          <span class="empty-desc">{{ $t('knowledgeList.empty.description') }}</span>
          <t-button v-if="authStore.canManageKnowledge" class="kb-create-btn empty-state-btn"
            data-guide="kb-list-create" @click="handleCreateKnowledgeBase">
            <template #icon><t-icon name="folder-add" /></template>
            {{ $t('knowledgeList.create') }}
          </t-button>
        </div>

        <!-- 收藏空状态：不放创建按钮——「没有收藏」 ≠ 「没有知识库」，
             正确引导是「去星标一下」，不是「再建一个」。 -->
        <div v-if="spaceSelection === 'favorites' && filteredKnowledgeBases.length === 0 && !loading"
          class="empty-state">
          <t-icon name="star" size="48px" class="empty-icon" />
          <span class="empty-txt">{{ $t('knowledgeList.empty.favoritesTitle') }}</span>
          <span class="empty-desc">{{ $t('knowledgeList.empty.favoritesDescription') }}</span>
        </div>

        <!-- 最近空状态：同理，引导是「去打开一个」。 -->
        <div v-if="spaceSelection === 'recents' && filteredKnowledgeBases.length === 0 && !loading" class="empty-state">
          <t-icon name="history" size="48px" class="empty-icon" />
          <span class="empty-txt">{{ $t('knowledgeList.empty.recentsTitle') }}</span>
          <span class="empty-desc">{{ $t('knowledgeList.empty.recentsDescription') }}</span>
        </div>

        <!-- 我的知识库空状态 -->
        <div v-if="spaceSelection === 'mine' && kbs.length === 0 && !loading" class="empty-state">
          <img class="empty-img" src="@/assets/img/upload.svg" alt="">
          <span class="empty-txt">{{ $t('knowledgeList.empty.title') }}</span>
          <span class="empty-desc">{{ $t('knowledgeList.empty.description') }}</span>
          <t-button v-if="authStore.canManageKnowledge" class="kb-create-btn empty-state-btn"
            data-guide="kb-list-create" @click="handleCreateKnowledgeBase">
            <template #icon><t-icon name="folder-add" /></template>
            {{ $t('knowledgeList.create') }}
          </t-button>
        </div>

      </div>
    </div>

    <!-- 删除确认对话框 -->
    <t-dialog v-model:visible="deleteVisible" dialogClassName="del-knowledge-dialog" :closeBtn="false" :cancelBtn="null"
      :confirmBtn="null">
      <div class="circle-wrap">
        <div class="dialog-header">
          <img class="circle-img" src="@/assets/img/circle.png" alt="">
          <span class="circle-title">{{ $t('knowledgeList.delete.confirmTitle') }}</span>
        </div>
        <span class="del-circle-txt">
          {{ $t('knowledgeList.delete.confirmMessage', { name: deletingKb?.name ?? '' }) }}
        </span>
        <div class="circle-btn">
          <span class="circle-btn-txt" @click="deleteVisible = false">{{ $t('common.cancel') }}</span>
          <span class="circle-btn-txt confirm" @click="confirmDelete">{{ $t('knowledgeList.delete.confirmButton')
          }}</span>
        </div>
      </div>
    </t-dialog>

    <!-- 知识库编辑器（创建/编辑统一组件） -->
    <KnowledgeBaseEditorModal :visible="uiStore.showKBEditorModal" :mode="uiStore.kbEditorMode"
      :kb-id="uiStore.currentKBId || undefined" :initial-type="uiStore.kbEditorType"
      :knowledge-domain-id="selectedKnowledgeDomainId || undefined"
      @update:visible="(val) => val ? null : uiStore.closeKBEditor()" @success="handleKBEditorSuccess" />

    <KnowledgeBaseAccessDialog
      v-model:visible="accessDialogVisible"
      :knowledge-base="accessKnowledgeBase"
      @resources-changed="fetchList(true)"
    />

    <KnowledgeDomainCreateDialog
      v-model:visible="knowledgeDomainCreateVisible"
      @created="handleKnowledgeDomainCreated"
    />

    <ContextualGuide tour="kbList" :when="showKbListContextualGuide" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MessagePlugin, Icon as TIcon } from 'tdesign-vue-next'
import { deleteKnowledgeBase, togglePinKnowledgeBase } from '@/api/knowledge-base'
import { useChatResourcesStore } from '@/stores/chatResources'
import { formatStringDate } from '@/utils/index'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import KnowledgeBaseEditorModal from './KnowledgeBaseEditorModal.vue'
import ListSpaceSidebar from '@/components/ListSpaceSidebar.vue'
import ContextualGuide from '@/components/ContextualGuide.vue'
import { isContextualGuideDone, markContextualGuideDone } from '@/config/contextualGuides'
import { usePlatformModelReadiness } from '@/composables/usePlatformModelReadiness'
import { useI18n } from 'vue-i18n'
import { useListUrlState } from '@/composables/useListUrlState'
import { useResourcePins } from '@/composables/useResourcePins'
import KnowledgeDomainSelector from '@/components/KnowledgeDomainSelector.vue'
import KnowledgeBaseAccessDialog from './components/KnowledgeBaseAccessDialog.vue'
import KnowledgeDomainCreateDialog from './components/KnowledgeDomainCreateDialog.vue'
import type { KnowledgeDomainInfo } from '@/api/knowledge-domain'

const router = useRouter()
const route = useRoute()
const uiStore = useUIStore()
const authStore = useAuthStore()
const selectedKnowledgeDomainId = ref<number | null>(null)
const knowledgeDomainCreateVisible = ref(false)
const knowledgeDomainSelectorVersion = ref(0)
const { loaded: modelsReadyLoaded, isReadyForDocumentKb } = usePlatformModelReadiness()
const chatResources = useChatResourcesStore()
const { t } = useI18n()

// 左侧范围选择默认展示当前用户全部有权读取的知识库。
//
// State lives in `?scope=` so links are shareable/bookmarkable; the
// composable handles two-way sync with the URL. "mine" is the stable query
// value; ListSpaceSidebar supplies its localized display label.
const defaultScope: 'all' | 'mine' = 'all'
const { scope: spaceSelection, creator: creatorFilter } = useListUrlState({
  defaultScope,
  defaultCreator: 'all',
})

// Per-user favorites + recents (localStorage-backed). isFavorite & touchRecent
// are wired into card render and click handlers below.
const pins = useResourcePins()
const kbFavoritesCount = computed(
  () => pins.favorites.value.filter((e) => e.type === 'kb').length
)
const kbRecentsCount = computed(
  () => pins.recents.value.filter((e) => e.type === 'kb').length
)

interface KB {
  id: string;
  knowledge_domain_id: number;
  name: string;
  description?: string;
  updated_at?: string;
  created_at?: string;
  pinned_at?: string;
  embedding_model_id?: string;
  summary_model_id?: string;
  type?: 'document' | 'faq';
  showMore?: boolean;
  vlm_config?: { enabled?: boolean; model_id?: string };
  extract_config?: { enabled?: boolean };
  storage_provider_config?: { provider?: string };
  storage_config?: { provider?: string; bucket_name?: string }; // legacy
  question_generation_config?: { enabled?: boolean; question_count?: number };
  knowledge_count?: number;
  chunk_count?: number;
  isProcessing?: boolean;
  processing_count?: number;
  my_permission?: 'read' | 'manage';
  is_pinned?: boolean;
  // creator_id is audit/display metadata used for creator grouping and badges.
  // It does not grant management permission.
  creator_id?: string;
  // creator_name 由后端 list 接口回填，仅用于卡片右下角来源徽章的 tooltip。
  creator_name?: string;
}

const kbs = ref<KB[]>([])
const loading = ref(false)
const deleteVisible = ref(false)
const deletingKb = ref<KB | null>(null)
const accessKnowledgeBase = ref<KB | null>(null)
const accessDialogVisible = ref(false)
const currentMoreIndex = ref<number>(-1)
const highlightedKbId = ref<string | null>(null)
const highlightedCardRef = ref<HTMLElement | null>(null)
const uploadTasks = ref<UploadTaskState[]>([])
const uploadCleanupTimers = new Map<string, ReturnType<typeof setTimeout>>()
let uploadRefreshTimer: ReturnType<typeof setTimeout> | null = null
const UPLOAD_CLEANUP_DELAY = 10000

const allKnowledgeBases = computed(() => kbs.value.length)

// 「本部门」视图下的稳定排序：本部门内「我创建」在前、「同事创建」在后；
// 子段内保留服务端的置顶优先顺序，并在两组之间插入只读分组标题。
// Ordering for the department tab:
//   1. pinned KBs (mine or teammate), newest pin first
//   2. my non-pinned KBs
//   3. teammate non-pinned KBs (rendered under the read-only header)
//
// Pin is per-user as of migration 000050, so a teammate-created KB that
// the caller has personally pinned must float into the pinned section
// even though it would otherwise live in the teammate sub-group. The
// previous version only bucketed by isMyKb and silently demoted these
// pinned-but-teammate KBs.
const sortedMineKbs = computed<KB[]>(() => {
  return [...kbs.value].sort((a, b) => {
    const ap = a.is_pinned ? 0 : 1
    const bp = b.is_pinned ? 0 : 1
    if (ap !== bp) return ap - bp
    if (a.is_pinned && b.is_pinned) {
      const at = a.pinned_at ? Date.parse(a.pinned_at as string) : 0
      const bt = b.pinned_at ? Date.parse(b.pinned_at as string) : 0
      if (at !== bt) return bt - at
    }
    const am = isMyKb(a) ? 0 : 1
    const bm = isMyKb(b) ? 0 : 1
    if (am !== bm) return am - bm
    const ac = a.created_at ? Date.parse(a.created_at as string) : 0
    const bc = b.created_at ? Date.parse(b.created_at as string) : 0
    return bc - ac
  })
})

// Favorites and recents are hydrated against the resources visible in this knowledgeDomain.
const kbResourceIndex = computed(() => {
  const map = new Map<string, KB>()
  for (const kb of kbs.value) map.set(kb.id, kb)
  return map
})

const favoritesList = computed(() => pins.favorites.value
  .filter((entry) => entry.type === 'kb')
  .map((entry) => {
    const kb = kbResourceIndex.value.get(entry.id)
    return kb ? { ...kb, _pinTs: entry.ts } : null
  })
  .filter((kb): kb is NonNullable<typeof kb> => kb !== null))

const recentsList = computed(() => pins.recents.value
  .filter((entry) => entry.type === 'kb')
  .map((entry) => {
    const kb = kbResourceIndex.value.get(entry.id)
    return kb ? { ...kb, _pinTs: entry.ts } : null
  })
  .filter((kb): kb is NonNullable<typeof kb> => kb !== null))
const showGroupHeaders = computed(() => true)

// 同部门、非当前用户创建的 KB 分组标题。
// 普通用户显示“仅查看”，知识域管理员显示“其他成员”。
const knowledgeDomainSectionLabelKey = computed(() =>
  authStore.canManageKnowledge
    ? 'knowledgeList.sections.knowledgeDomainOthers'
    : 'knowledgeList.sections.knowledgeDomainReadonly'
)

// 图标与文案对齐：admin 使用 usergroup，viewer 使用 browse。
const knowledgeDomainSectionIconName = computed(() =>
  authStore.canManageKnowledge ? 'usergroup' : 'browse'
)

// 分组折叠：ephemeral，只在当前会话里生效，不落 localStorage/服务器。
// 之所以走"折叠集合"而不是"展开集合"，是因为默认全展开——空 Set
// 即表示初始的全展开状态，避免每次新加分段还得回头维护默认值。
type KbSectionKey = 'pinned' | 'mine' | 'knowledgeDomainOthers'
const collapsedKbSections = ref<Set<KbSectionKey>>(new Set())
const isKbSectionCollapsed = (key: KbSectionKey) => collapsedKbSections.value.has(key)
const toggleKbSection = (key: KbSectionKey) => {
  // 重新赋一个新的 Set 是为了让 ref 的 .value 身份变化触发模板重渲染；
  // 直接 .add/.delete 在 Vue 3 的 reactive Set 里也能 work，但 ref(Set) 的
  // 内层代理行为在不同版本上略有差异，整体替换最稳。
  const next = new Set(collapsedKbSections.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsedKbSections.value = next
}
const kbSectionOf = (kb: any): KbSectionKey => {
  if (kb?.is_pinned) return 'pinned'
  return isMyKb(kb) ? 'mine' : 'knowledgeDomainOthers'
}

// 每个分组里实际有多少张卡片——直接把分组判定函数复用一遍。组标题上展示
// "(N)" 让用户一眼知道折叠后会藏掉多少，也方便核对筛选结果。
const emptyKbCounts = (): Record<KbSectionKey, number> => ({
  pinned: 0, mine: 0, knowledgeDomainOthers: 0,
})
const filteredKbSectionCounts = computed<Record<KbSectionKey, number>>(() => {
  const c = emptyKbCounts()
  filteredKnowledgeBases.value.forEach(kb => { c[kbSectionOf(kb)]++ })
  return c
})
const mineKbSectionCounts = computed<Record<KbSectionKey, number>>(() => {
  const c = emptyKbCounts()
  sortedMineKbs.value.forEach(kb => { c[kbSectionOf(kb)]++ })
  return c
})
// Filtered knowledge bases stay within the current knowledgeDomain and permission scope.
const filteredKnowledgeBases = computed(() => {
  const visibleKnowledgeBases =
    authStore.canManageKnowledge && selectedKnowledgeDomainId.value
      ? kbs.value.filter(kb => kb.knowledge_domain_id === selectedKnowledgeDomainId.value)
      : kbs.value
  if (spaceSelection.value === 'favorites') {
    return favoritesList.value.filter(kb => visibleKnowledgeBases.some(item => item.id === kb.id))
  }
  if (spaceSelection.value === 'recents') {
    return recentsList.value.filter(kb => visibleKnowledgeBases.some(item => item.id === kb.id))
  }
  if (spaceSelection.value === 'mine') {
    return visibleKnowledgeBases
  }
  if (spaceSelection.value !== 'all') {
    return []
  }
  return visibleKnowledgeBases
})

const showKbListEmpty = computed(() => {
  if (loading.value) return false
  if (!authStore.canManageKnowledge) return false
  if (spaceSelection.value === 'all' && filteredKnowledgeBases.value.length === 0) return true
  if (spaceSelection.value === 'mine' && kbs.value.length === 0) return true
  return false
})

const showKbListContextualGuide = computed(
  () => showKbListEmpty.value && !uiStore.showKBEditorModal,
)

interface UploadTaskState {
  uploadId: string
  kbId: string
  fileName?: string
  progress: number
  status: 'uploading' | 'success' | 'error'
  error?: string
}

interface UploadSummary {
  kbId: string
  kbName: string
  total: number
  completed: number
  progress: number
  hasError: boolean
}

const applyKbListData = (data: any[]) => {
  kbs.value = data.map((kb: any) => ({
    ...kb,
    updated_at: kb.updated_at ? formatStringDate(new Date(kb.updated_at)) : '',
    showMore: false,
    isProcessing: kb.is_processing || false,
    processing_count: kb.processing_count || 0
  }))
}

const fetchList = (force = false) => {
  loading.value = true
  return chatResources.fetchKnowledgeBasesForList({ creator: creatorFilter.value }, force)
    .then(applyKbListData)
    .finally(() => { loading.value = false })
}

watch(spaceSelection, (val) => {
  if (!['all', 'mine', 'favorites', 'recents'].includes(val)) {
    spaceSelection.value = 'all'
  }
}, { immediate: true })

// Refetch when the creator filter flips. We re-pull the whole list rather
// than filtering in-memory so the server stays the single source of truth
watch(creatorFilter, () => {
  fetchList(true)
})

onMounted(() => {
  fetchList().then(() => {
    // 检查路由参数中是否有需要高亮的知识库ID
    const highlightKbId = route.query.highlightKbId as string
    if (highlightKbId) {
      triggerHighlightFlash(highlightKbId)
      // Drop the transient highlight param but preserve other state
      // (scope / creator / q) so refreshing doesn't reset the user's view.
      const { highlightKbId: _drop, ...rest } = route.query
      router.replace({ query: rest })
    }
  })

  window.addEventListener('knowledgeFileUploadStart', handleUploadStartEvent as EventListener)
  window.addEventListener('knowledgeFileUploadProgress', handleUploadProgressEvent as EventListener)
  window.addEventListener('knowledgeFileUploadComplete', handleUploadCompleteEvent as EventListener)
  window.addEventListener('knowledgeFileUploaded', handleUploadFinishedEvent as EventListener)
})

onUnmounted(() => {
  window.removeEventListener('knowledgeFileUploadStart', handleUploadStartEvent as EventListener)
  window.removeEventListener('knowledgeFileUploadProgress', handleUploadProgressEvent as EventListener)
  window.removeEventListener('knowledgeFileUploadComplete', handleUploadCompleteEvent as EventListener)
  window.removeEventListener('knowledgeFileUploaded', handleUploadFinishedEvent as EventListener)

  uploadCleanupTimers.forEach(timer => clearTimeout(timer))
  uploadCleanupTimers.clear()
  if (uploadRefreshTimer) {
    clearTimeout(uploadRefreshTimer)
    uploadRefreshTimer = null
  }
})

// 监听路由变化，处理从其他页面跳转过来的高亮需求
watch(() => route.query.highlightKbId, (newKbId) => {
  if (newKbId && typeof newKbId === 'string' && kbs.value.length > 0) {
    triggerHighlightFlash(newKbId)
    const { highlightKbId: _drop, ...rest } = route.query
    router.replace({ query: rest })
  }
})

const openMore = (index: number) => {
  // 只记录当前打开的索引，用于显示激活样式
  // 弹窗的开关由 v-model 自动管理
  currentMoreIndex.value = index
}

const onVisibleChange = (visible: boolean) => {
  // 弹窗关闭时重置索引
  if (!visible) {
    currentMoreIndex.value = -1
  }
}

const handleSettings = (kb: KB) => {
  // 手动关闭弹窗
  kb.showMore = false
  goSettings(kb.id)
}

// canManageKBCard mirrors the server's effective management permission and
// hides destructive menu items from read-only users. creator_id is display
// and audit metadata only.
//
// The pin item is intentionally NOT gated by this predicate any more:
// pin state is per (user, kb) as of migration 000050 and the backend
// route only requires KB read access, so anyone who can see the card
// should be able to pin it for themselves.
//
function canManageKBCard(kb: KB): boolean {
  return authStore.canManageKnowledge || kb.my_permission === 'manage'
}

const handleAccess = (kb: KB) => {
  kb.showMore = false
  accessKnowledgeBase.value = kb
  accessDialogVisible.value = true
}

// isMyKb 仅用于卡片右下角徽章在「我创建」与「本知识域其他成员创建」之间切换。
// 与 canManageKBCard 不同：徽章纯粹按创建者匹配，不代表管理权限。
// creator_id 为空时按知识域公共知识库处理，避免错误标成「我创建」。
function isMyKb(kb: { creator_id?: string }): boolean {
  const userId = authStore.user?.id || ''
  return !!(kb.creator_id && userId && kb.creator_id === userId)
}

// 通过 ID 处理设置（用于全部 Tab 下的知识库）
const handleSettingsById = (id: string) => {
  goSettings(id)
}

const handleAccessById = (id: string) => {
  const kb = kbs.value.find(item => item.id === id)
  if (kb) handleAccess(kb)
}

// 通过 ID 处理删除（用于全部 Tab 下的知识库）
const handleDeleteById = (id: string) => {
  const kb = kbs.value.find(k => k.id === id)
  if (kb) {
    deletingKb.value = kb
    deleteVisible.value = true
  }
}

const handleTogglePin = async (kb: KB) => {
  kb.showMore = false
  try {
    const res: any = await togglePinKnowledgeBase(kb.id)
    if (res.success) {
      MessagePlugin.success(
        res.data.is_pinned ? t('knowledgeList.pin.pinSuccess') : t('knowledgeList.pin.unpinSuccess')
      )
      fetchList(true)
    }
  } catch {
    MessagePlugin.error(t('knowledgeList.pin.failed'))
  }
}

const handleTogglePinById = async (id: string) => {
  try {
    const res: any = await togglePinKnowledgeBase(id)
    if (res.success) {
      MessagePlugin.success(
        res.data.is_pinned ? t('knowledgeList.pin.pinSuccess') : t('knowledgeList.pin.unpinSuccess')
      )
      fetchList(true)
    }
  } catch {
    MessagePlugin.error(t('knowledgeList.pin.failed'))
  }
}

const handleDelete = (kb: KB) => {
  // 手动关闭弹窗
  kb.showMore = false
  deletingKb.value = kb
  deleteVisible.value = true
}

const confirmDelete = () => {
  if (!deletingKb.value) return

  deleteKnowledgeBase(deletingKb.value.id).then((res: any) => {
    if (res.success) {
      MessagePlugin.success(t('knowledgeList.messages.deleted'))
      deleteVisible.value = false
      deletingKb.value = null
      fetchList(true)
    } else {
      MessagePlugin.error(res.message || t('knowledgeList.messages.deleteFailed'))
    }
  }).catch((e: any) => {
    MessagePlugin.error(e?.message || t('knowledgeList.messages.deleteFailed'))
  })
}

const isInitialized = (kb: KB) => {
  // LLM (summary) model is always required
  if (!kb.summary_model_id || kb.summary_model_id === '') return false
  // Embedding model only required when RAG indexing is enabled (vector or keyword)
  const strategy = (kb as any).indexing_strategy
  const needsEmbedding = !strategy || strategy.vector_enabled || strategy.keyword_enabled
  if (needsEmbedding && (!kb.embedding_model_id || kb.embedding_model_id === '')) return false
  return true
}

// 计算是否有未初始化的知识库
const hasUninitializedKbs = computed(() => {
  return kbs.value.some(kb => !isInitialized(kb))
})

const getKbDisplayName = (kbId: string) => {
  const target = kbs.value.find(kb => kb.id === kbId)
  if (target?.name) return target.name
  return t('knowledgeList.uploadProgress.unknownKb', { id: kbId }) as string
}

const uploadSummaries = computed<UploadSummary[]>(() => {
  if (!uploadTasks.value.length) return []
  const grouped: Record<string, UploadTaskState[]> = {}
  uploadTasks.value.forEach(task => {
    const kbKey = String(task.kbId)
    if (!grouped[kbKey]) grouped[kbKey] = []
    grouped[kbKey].push(task)
  })
  return Object.entries(grouped).map(([kbId, tasks]) => {
    const total = tasks.length
    const completed = tasks.filter(task => task.status !== 'uploading').length
    const progressSum = tasks.reduce((sum, task) => sum + (task.progress ?? 0), 0)
    const avgProgress = total === 0 ? 0 : Math.min(100, Math.max(0, Math.round(progressSum / total)))
    const hasError = tasks.some(task => task.status === 'error')
    return {
      kbId,
      kbName: getKbDisplayName(kbId),
      total,
      completed,
      progress: avgProgress,
      hasError
    }
  }).sort((a, b) => a.kbName.localeCompare(b.kbName))
})

const clampProgress = (value: number) => Math.min(100, Math.max(0, Math.round(value)))

const addUploadTask = (task: UploadTaskState) => {
  uploadTasks.value = [
    ...uploadTasks.value.filter(item => item.uploadId !== task.uploadId),
    task,
  ]
}

const patchUploadTask = (uploadId: string, patch: Partial<UploadTaskState>) => {
  const index = uploadTasks.value.findIndex(task => task.uploadId === uploadId)
  if (index === -1) return
  const nextTasks = [...uploadTasks.value]
  nextTasks[index] = { ...nextTasks[index], ...patch }
  uploadTasks.value = nextTasks
}

const removeUploadTask = (uploadId: string) => {
  uploadTasks.value = uploadTasks.value.filter(task => task.uploadId !== uploadId)
  const timer = uploadCleanupTimers.get(uploadId)
  if (timer) {
    clearTimeout(timer)
    uploadCleanupTimers.delete(uploadId)
  }
}

const scheduleUploadTaskCleanup = (uploadId: string) => {
  const existing = uploadCleanupTimers.get(uploadId)
  if (existing) {
    clearTimeout(existing)
  }
  const timer = setTimeout(() => {
    removeUploadTask(uploadId)
  }, UPLOAD_CLEANUP_DELAY)
  uploadCleanupTimers.set(uploadId, timer)
}

type UploadEventDetail = {
  uploadId: string
  kbId?: string | number
  fileName?: string
  progress?: number
  status?: UploadTaskState['status']
  error?: string
}

const ensureUploadTaskEntry = (detail?: UploadEventDetail) => {
  if (!detail?.uploadId) return null
  const existing = uploadTasks.value.find(task => task.uploadId === detail.uploadId)
  if (existing) return existing
  if (!detail.kbId) return null
  const initialProgress = typeof detail.progress === 'number' ? clampProgress(detail.progress) : 0
  const newTask: UploadTaskState = {
    uploadId: detail.uploadId,
    kbId: String(detail.kbId),
    fileName: detail.fileName,
    progress: initialProgress,
    status: detail.status || 'uploading',
    error: detail.error
  }
  addUploadTask(newTask)
  return newTask
}

const handleCardClick = (kb: KB) => {
  // Track this open in the per-user "recent" list before navigating —
  // matches the user mental model "this is what I last worked on".
  pins.touchRecent('kb', kb.id)
  if (isInitialized(kb)) {
    goDetail(kb.id)
  } else {
    goSettings(kb.id)
  }
}

// toggleFavoriteKb is the click handler for the star icon rendered on
// each card. Stops propagation so it doesn't bubble into the card's
// own @click which would open the KB.
const toggleFavoriteKb = (kbId: string, evt?: Event) => {
  evt?.stopPropagation()
  pins.toggleFavorite('kb', kbId)
}
const isKbFavorited = (kbId: string) => pins.isFavorite('kb', kbId)

const goDetail = (id: string) => {
  router.push(`/platform/knowledge-bases/${id}`)
}

const goSettings = (id: string) => {
  // 使用模态框打开设置
  uiStore.openKBSettings(id)
}

// 创建知识库
const handleCreateKnowledgeBase = () => {
  if (!selectedKnowledgeDomainId.value) {
    MessagePlugin.warning(t('knowledgeEditor.messages.domainRequired'))
    return
  }
  markContextualGuideDone('kbList')
  // 无模型时仍打开创建向导，并定位到模型配置页；用户可在向导内添加模型，无需先跳转系统设置
  const initialSection =
    modelsReadyLoaded.value && !isReadyForDocumentKb.value ? 'models' : undefined
  uiStore.openCreateKB('document', initialSection)
}

const handleKnowledgeDomainCreated = (domain: KnowledgeDomainInfo) => {
  selectedKnowledgeDomainId.value = domain.id
  knowledgeDomainSelectorVersion.value += 1
}

// 知识库编辑器成功回调（创建或编辑成功）
const handleKBEditorSuccess = (kbId: string) => {
  console.log('[KnowledgeBaseList] knowledge operation success:', kbId)
  const shouldOpenDetailForUploadGuide = !isContextualGuideDone('kbDetail')
  // 列表页编辑同样要让单 KB 详情缓存失效，否则侧栏 / 详情页 60s 内仍显示旧信息
  chatResources.invalidateKnowledgeBaseDetail(kbId)
  fetchList(true).then(() => {
    if (shouldOpenDetailForUploadGuide && kbId) {
      goDetail(kbId)
    }
    // 如果是从路由参数中获取的高亮ID，触发闪烁效果
    if (route.query.highlightKbId === kbId) {
      triggerHighlightFlash(kbId)
      const { highlightKbId: _drop, ...rest } = route.query
      router.replace({ query: rest })
    }
  })
}

// 触发高亮闪烁效果
const triggerHighlightFlash = (kbId: string) => {
  highlightedKbId.value = kbId
  nextTick(() => {
    if (highlightedCardRef.value) {
      // 滚动到高亮的卡片
      highlightedCardRef.value.scrollIntoView({
        behavior: 'smooth',
        block: 'center'
      })
    }
    // 3秒后清除高亮
    setTimeout(() => {
      highlightedKbId.value = null
    }, 3000)
  })
}

const handleUploadStartEvent = (event: Event) => {
  const detail = (event as CustomEvent<UploadEventDetail>).detail
  if (!detail?.uploadId || !detail?.kbId) return
  addUploadTask({
    uploadId: detail.uploadId,
    kbId: String(detail.kbId),
    fileName: detail.fileName,
    progress: typeof detail.progress === 'number' ? clampProgress(detail.progress) : 0,
    status: 'uploading'
  })
}

const handleUploadProgressEvent = (event: Event) => {
  const detail = (event as CustomEvent<UploadEventDetail>).detail
  if (!detail?.uploadId || typeof detail.progress !== 'number') return
  if (!ensureUploadTaskEntry(detail)) return
  patchUploadTask(detail.uploadId, {
    progress: clampProgress(detail.progress)
  })
}

const handleUploadCompleteEvent = (event: Event) => {
  const detail = (event as CustomEvent<UploadEventDetail>).detail
  if (!detail?.uploadId) return
  const progress = typeof detail.progress === 'number'
    ? clampProgress(detail.progress)
    : 100
  if (!ensureUploadTaskEntry({ ...detail, progress })) return
  patchUploadTask(detail.uploadId, {
    status: detail.status || 'success',
    progress,
    error: detail.error
  })
  scheduleUploadTaskCleanup(detail.uploadId)
}

const handleUploadFinishedEvent = (event: Event) => {
  const detail = (event as CustomEvent<{ kbId?: string | number }>).detail
  if (!detail?.kbId) return
  if (uploadRefreshTimer) {
    clearTimeout(uploadRefreshTimer)
  }
  uploadRefreshTimer = setTimeout(() => {
    fetchList(true)
    uploadRefreshTimer = null
  }, 800)
}
</script>

<style scoped lang="less">
.kb-list-container {
  margin: 0;
  height: 100%;
  box-sizing: border-box;
  flex: 1;
  display: flex;
  position: relative;
  min-height: 0;
}

.kb-list-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 20px 0 0 28px;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  padding-right: 28px;

  .header-title {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .title-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  h2 {
    margin: 0;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
  }

}

.kb-create-btn {
  background: linear-gradient(135deg, var(--td-brand-color) 0%, #00a67e 100%);
  border: none;
  color: var(--td-text-color-anti);

  &:hover {
    background: linear-gradient(135deg, var(--td-brand-color) 0%, var(--td-brand-color-active) 100%);
  }
}

.kb-list-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  overflow-x: hidden;
  // 顶部不留 padding，sticky 的分组标题 (top: 0) 才能贴到容器最顶；
  // 底部 padding 保留，避免最后一行卡片紧贴边。
  padding: 0 28px 8px 0;
  scrollbar-width: auto;
  scrollbar-color: auto;
}

.kb-list-main-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  padding: 12px;
  background: var(--td-bg-color-container);
}

.header-subtitle {
  margin: 0;
  color: var(--td-text-color-placeholder);
  font-family: var(--app-font-family);
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;
}

.knowledge-domain-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 10px;
}

.header-action-btn {
  padding: 0 !important;
  min-width: 28px !important;
  width: 28px !important;
  height: 28px !important;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  background: var(--td-bg-color-secondarycontainer) !important;
  border: 1px solid var(--td-component-stroke) !important;
  border-radius: 6px !important;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: background 0.2s, border-color 0.2s, color 0.2s;

  &:hover {
    background: var(--td-bg-color-secondarycontainer) !important;
    border-color: var(--td-component-stroke) !important;
    color: var(--td-text-color-primary);
  }

  :deep(.t-icon),
  :deep(.btn-icon-wrapper) {
    color: var(--td-brand-color);
  }
}

// Tab 切换样式（已由左侧菜单替代，保留以备兼容）
.kb-tabs {
  display: flex;
  align-items: center;
  gap: 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  margin-bottom: 20px;

  .tab-item {
    padding: 12px 0;
    cursor: pointer;
    color: var(--td-text-color-secondary);
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 400;
    user-select: none;
    position: relative;
    transition: color 0.2s ease;

    &:hover {
      color: var(--td-text-color-primary);
    }

    &.active {
      color: var(--td-brand-color);
      font-weight: 500;

      &::after {
        content: '';
        position: absolute;
        bottom: -1px;
        left: 0;
        right: 0;
        height: 2px;
        background: var(--td-brand-color);
        border-radius: 1px;
      }
    }
  }
}


.warning-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  margin-bottom: 20px;
  background: var(--td-warning-color-light);
  border: 1px solid var(--td-warning-color-focus);
  border-radius: 6px;
  color: var(--td-warning-color);
  font-family: var(--app-font-family);
  font-size: 14px;

  .t-icon {
    color: var(--td-warning-color);
    flex-shrink: 0;
  }
}

.upload-progress-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 20px;
}

.upload-progress-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
}

.upload-progress-icon {
  color: var(--td-brand-color);
  display: flex;
  align-items: center;
  justify-content: center;
}

.upload-progress-content {
  flex: 1;
}

.progress-title {
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
  margin-bottom: 2px;
}

.progress-subtitle {
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 12px;
  line-height: 18px;
}

.progress-subtitle.secondary {
  color: var(--td-text-color-placeholder);
  margin-top: 2px;
}

.progress-subtitle.error {
  color: var(--td-error-color);
  margin-top: 4px;
}

.progress-bar {
  width: 100%;
  height: 6px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
  margin-top: 10px;
  overflow: hidden;
}

.progress-bar-inner {
  height: 100%;
  background: linear-gradient(90deg, var(--td-brand-color-active) 0%, var(--td-brand-color) 100%);
  transition: width 0.2s ease;
}

@keyframes contentFadeIn {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.kb-card-wrap {
  display: grid;
  gap: 12px;
  grid-template-columns: 1fr;
  animation: contentFadeIn 0.32s ease-out;
}

.kb-section-header {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  gap: 6px;
  // 整行只用来铺背景实现 sticky；点击事件靠子元素冒泡触发，避免点到
  // 标题右侧大片空白时误折叠。键盘 tab/enter 不受 pointer-events 影响。
  pointer-events: none;

  & > * {
    pointer-events: auto;
  }
  // 下滑时吸顶到滚动容器（.kb-list-main）顶部。z-index 要高于卡片自身的
  // hover 阴影 / 装饰层；背景必须不透明，否则卡片会从下方透出来。
  position: sticky;
  top: 0;
  z-index: 5;
  background: var(--td-bg-color-container);
  // 用 box-shadow 把背景再往上"延伸"8px，封掉 sticky 与容器顶之间任何
  // subpixel 残缝（border-radius 的圆角三角、滚动时浏览器子像素渲染等
  // 都会让卡片从这里漏出 1-2px）。第二条 shadow 在下方也再补一点，避免
  // grid-gap 区域里卡片穿插过来。
  box-shadow: 0 -8px 0 0 var(--td-bg-color-container),
    0 4px 0 0 var(--td-bg-color-container);
  padding: 6px 4px 6px 0;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 13px;
  font-weight: 600;
  line-height: 20px;
  cursor: pointer;
  user-select: none;
  outline: none;

  &:hover {
    color: var(--td-text-color-primary);
  }

  &:focus-visible {
    box-shadow: 0 0 0 2px var(--td-brand-color-focus, rgba(0, 82, 217, 0.2));
  }

  // Icons inherit the section header's text color so the whole row
  // (icon + label) reads as one muted secondary tone. The pinned
  // modifier no longer overrides this either — uniform appearance
  // is intentional; the icon shape alone is enough to flag which
  // section the user is looking at.
  .t-icon {
    color: inherit;
  }

  .kb-section-toggle {
    margin-left: 4px;
    opacity: 0.7;
    transition: opacity 0.15s ease;
  }

  // 组里实际有多少张卡片。用 13px 主字号同色降透明度，避免抢标题视觉，
  // 同时给个轻底色保证在浅色容器上仍可读。
  .kb-section-count {
    margin-left: 2px;
    padding: 0 6px;
    border-radius: 8px;
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-secondary);
    font-size: 11px;
    line-height: 16px;
    font-weight: 500;
  }

  &:hover .kb-section-toggle {
    opacity: 1;
  }
}

.kb-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  background: var(--td-bg-color-container);
  position: relative;
  cursor: pointer;
  transition: all 0.25s ease;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  height: 136px;
  min-height: 136px;

  &.kb-card-skeleton {
    cursor: default;

    .card-header {
      margin-bottom: 12px;
    }

    .card-content {
      flex: 1;
    }

    .card-bottom {
      margin-top: auto;
    }
  }

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 4px 12px rgba(7, 192, 95, 0.12);
  }

  &.uninitialized {
    opacity: 0.9;
  }

  // 文档类型样式
  &.kb-type-document {
    background: linear-gradient(135deg, var(--td-bg-color-container) 0%, rgba(7, 192, 95, 0.04) 100%);

    &:hover {
      border-color: var(--td-brand-color);
      background: linear-gradient(135deg, var(--td-bg-color-container) 0%, rgba(7, 192, 95, 0.08) 100%);
    }

    // 右上角装饰
    &::after {
      content: '';
      position: absolute;
      top: 0;
      right: 0;
      width: 60px;
      height: 60px;
      background: linear-gradient(135deg, rgba(7, 192, 95, 0.08) 0%, transparent 100%);
      border-radius: 0 12px 0 100%;
      pointer-events: none;
      z-index: 0;
    }
  }

  // 问答类型样式
  &.kb-type-faq {
    background: linear-gradient(135deg, var(--td-bg-color-container) 0%, rgba(0, 82, 217, 0.04) 100%);

    &:hover {
      border-color: var(--td-brand-color);
      box-shadow: 0 4px 12px rgba(0, 82, 217, 0.12);
      background: linear-gradient(135deg, var(--td-bg-color-container) 0%, rgba(0, 82, 217, 0.08) 100%);
    }

    // 右上角装饰
    &::after {
      content: '';
      position: absolute;
      top: 0;
      right: 0;
      width: 60px;
      height: 60px;
      background: linear-gradient(135deg, rgba(0, 82, 217, 0.08) 0%, transparent 100%);
      border-radius: 0 12px 0 100%;
      pointer-events: none;
      z-index: 0;
    }
  }

  .kb-favorite-star {
    // 浮在卡片右上角顶角。卡片自身有 padding，"更多"按钮在 header flex 末端
    // 自然落在 padding 内部，与零位的 star 错开一段距离。
    position: absolute;
    top: 0;
    right: 0;
    z-index: 3;
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-radius: 6px;
    color: var(--td-text-color-secondary);
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s ease, background 0.15s ease, color 0.15s ease;

    &:hover {
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-warning-color, #e37318);
    }

    &.is-favorited {
      opacity: 1;
      color: var(--td-warning-color, #e37318);
    }
  }

  // Reveal the star on card hover; favorited state forces it visible.
  &:hover .kb-favorite-star {
    opacity: 1;
  }

  // 确保内容在装饰之上
  .card-header,
  .card-content,
  .card-bottom {
    position: relative;
    z-index: 1;
  }

  .card-header {
    margin-bottom: 6px;
  }

  .card-title {
    font-size: 15px;
    line-height: 22px;
  }

  .card-content {
    margin-bottom: 6px;
  }

  .card-description {
    font-size: 12px;
    line-height: 17px;
  }

  .card-bottom {
    padding-top: 6px;
  }

  .more-wrap {
    width: 28px;
    height: 28px;

    .more-icon {
      width: 16px;
      height: 16px;
    }
  }

  .card-more-btn {
    width: 28px;
    height: 28px;
  }
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 4px;
  margin-bottom: 6px;

  .card-title {
    flex: 1;
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    letter-spacing: 0.01em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .card-title-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-more-btn {
    flex-shrink: 0;
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 6px;
    color: var(--td-text-color-placeholder);
    cursor: pointer;
    transition: all 0.2s;

    &:hover {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-secondary);
    }
  }

  .permission-tag {
    flex-shrink: 0;
  }
}

.card-title {
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 15px;
  font-weight: 600;
  line-height: 22px;
  letter-spacing: 0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

.more-wrap {
  display: flex;
  width: 24px;
  height: 24px;
  justify-content: center;
  align-items: center;
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.2s ease;
  opacity: 0;

  .kb-card:hover & {
    opacity: 0.6;
  }

  &:hover {
    background: var(--td-bg-color-container-hover);
    opacity: 1 !important;
  }

  &.active-more {
    background: var(--td-bg-color-container-hover);
    opacity: 1 !important;
  }

  .more-icon {
    width: 14px;
    height: 14px;
  }
}

.card-content {
  flex: 1;
  min-height: 0;
  margin-bottom: 8px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 三个列表卡片统一：描述字体 */
.card-description {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  overflow: hidden;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 12px;
  font-weight: 400;
  line-height: 18px;
}

.card-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: auto;
  padding-top: 8px;
  border-top: .5px solid var(--td-component-stroke);
}

.bottom-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.bottom-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;

  .card-time {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }
}

.feature-badges {
  display: flex;
  align-items: center;
  gap: 4px;
}

.feature-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 5px;
  cursor: default;
  transition: background 0.2s ease;

  &.type-document {
    background: rgba(7, 192, 95, 0.08);
    color: var(--td-brand-color-active);
    width: auto;
    padding: 0 6px;
    gap: 3px;

    &:hover {
      background: rgba(7, 192, 95, 0.12);
    }

    .badge-count {
      font-size: 11px;
      font-weight: 500;
    }

    .processing-icon {
      animation: spin 1s linear infinite;
    }
  }

  &.type-faq {
    background: rgba(0, 82, 217, 0.08);
    color: var(--td-brand-color);
    width: auto;
    padding: 0 6px;
    gap: 3px;

    &:hover {
      background: rgba(0, 82, 217, 0.12);
    }

    .badge-count {
      font-size: 11px;
      font-weight: 500;
    }

    .processing-icon {
      animation: spin 1s linear infinite;
    }
  }

  &.kg {
    background: rgba(124, 77, 255, 0.08);
    color: var(--td-brand-color);

    &:hover {
      background: rgba(124, 77, 255, 0.12);
    }
  }

  &.multimodal {
    background: rgba(255, 152, 0, 0.08);
    color: var(--td-warning-color);

    &:hover {
      background: rgba(255, 152, 0, 0.12);
    }
  }

  &.question {
    background: rgba(0, 150, 136, 0.08);
    color: var(--td-success-color);

    &:hover {
      background: rgba(0, 150, 136, 0.12);
    }
  }

}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

@keyframes highlightFlash {
  0% {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 0 rgba(7, 192, 95, 0.4);
    transform: scale(1);
  }

  50% {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 8px rgba(7, 192, 95, 0);
    transform: scale(1.02);
  }

  100% {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 0 rgba(7, 192, 95, 0);
    transform: scale(1);
  }
}

.kb-card.highlight-flash {
  animation: highlightFlash 0.6s ease-in-out 3;
  border-color: var(--td-brand-color) !important;
  box-shadow: 0 0 12px rgba(7, 192, 95, 0.3) !important;
}

.card-time {
  color: var(--td-text-color-placeholder);
  font-family: var(--app-font-family);
  font-size: 12px;
  font-weight: 400;
}


.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  padding: 60px 20px;

  .empty-img {
    width: 162px;
    height: 162px;
    margin-bottom: 20px;
  }

  .empty-txt {
    color: var(--td-text-color-placeholder);
    font-family: var(--app-font-family);
    font-size: 16px;
    font-weight: 600;
    line-height: 26px;
    margin-bottom: 8px;
  }

  .empty-desc {
    color: var(--td-text-color-disabled);
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
    margin-bottom: 0;
  }

  .empty-state-btn {
    margin-top: 20px;
  }
}

// 响应式布局
@media (min-width: 900px) {
  .kb-card-wrap {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1250px) {
  .kb-card-wrap {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1600px) {
  .kb-card-wrap {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (min-width: 1900px) {
  .kb-card-wrap {
    grid-template-columns: repeat(5, 1fr);
  }
}

@media (min-width: 2200px) {
  .kb-card-wrap {
    grid-template-columns: repeat(6, 1fr);
  }
}

// 删除确认对话框样式
:deep(.del-knowledge-dialog) {
  padding: 0px !important;
  border-radius: 6px !important;

  .t-dialog__header {
    display: none;
  }

  .t-dialog__body {
    padding: 16px;
  }

  .t-dialog__footer {
    padding: 0;
  }
}

@media (max-width: 900px) {
  .header {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
  }

  .knowledge-domain-actions {
    width: 100%;
    flex-wrap: wrap;

    :deep(.knowledge-domain-selector) {
      width: min(100%, 320px);
    }
  }
}

:deep(.t-dialog__position.t-dialog--top) {
  padding-top: 40vh !important;
}

.circle-wrap {
  .dialog-header {
    display: flex;
    align-items: center;
    margin-bottom: 8px;
  }

  .circle-img {
    width: 20px;
    height: 20px;
    margin-right: 8px;
  }

  .circle-title {
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 16px;
    font-weight: 600;
    line-height: 24px;
  }

  .del-circle-txt {
    color: var(--td-text-color-placeholder);
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
    display: inline-block;
    margin-left: 29px;
    margin-bottom: 21px;
  }

  .circle-btn {
    height: 22px;
    width: 100%;
    display: flex;
    justify-content: flex-end;
  }

  .circle-btn-txt {
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
    cursor: pointer;

    &:hover {
      opacity: 0.8;
    }
  }

  .confirm {
    color: var(--td-error-color);
    margin-left: 40px;

    &:hover {
      opacity: 0.8;
    }
  }
}
</style>
<style lang="less">
/* 下拉菜单样式已统一至 @/assets/dropdown-menu.less */

// 创建对话框样式优化
.create-kb-dialog {
  .t-form-item__label {
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }

  .t-input,
  .t-textarea {
    font-family: var(--app-font-family);
  }

}
</style>
