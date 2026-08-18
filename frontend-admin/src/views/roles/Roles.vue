<!-- 用户角色配置页（/platform/roles）。
     复用 PageSearchFilters / PageTable 公共组件。 -->
<template>
  <div class="roles-page page-shell page-shell--no-scroll">
    <!-- 筛选区 -->
    <PageSearchFilters v-model:search="state.filters.query" :search-placeholder="t('roles.filters.placeholder')">
      <template #default>
        <t-input v-model="state.filters.account" :placeholder="t('roles.filters.account')" clearable />
        <t-input v-model="state.filters.employee_name" :placeholder="t('roles.filters.employeeName')" clearable />
        <t-select v-model="state.filters.is_knowledge_officer" :placeholder="t('roles.filters.isKnowledgeOfficer')"
          clearable>
          <t-option :value="'true'" :label="t('common.yes')" />
          <t-option :value="'false'" :label="t('common.no')" />
        </t-select>
        <t-select v-model="state.filters.is_operator" :placeholder="t('roles.filters.isOperationsAdmin')" clearable>
          <t-option :value="'true'" :label="t('common.yes')" />
          <t-option :value="'false'" :label="t('common.no')" />
        </t-select>
        <t-select v-model="state.filters.department" :placeholder="t('roles.filters.department')" clearable filterable>
          <t-option v-for="opt in state.departmentOptions" :key="opt.id" :value="opt.id" :label="opt.name" />
        </t-select>
      </template>
      <template #trailing>
        <t-button theme="primary" @click="onApply">
          {{ t('common.search') }}
        </t-button>
        <t-button theme="default" variant="base" @click="onReset">
          {{ t('common.reset') }}
        </t-button>
      </template>
    </PageSearchFilters>

    <!-- 表格区（占满剩余空间，body 滚动） -->
    <div class="roles-page__table">
      <PageTable :data="state.items" :columns="columns" :loading="state.loading" :selected-ids="state.selectedIds"
        :pagination="state.pagination" @update:selected-ids="onUpdateSelectedIds" @page-change="onPageChange">
        <template #status="{ row }">
          <t-tag :theme="statusTheme((row as UserRole).status)" size="small">
            {{ t(`roles.status.${(row as UserRole).status}`) }}
          </t-tag>
        </template>
        <template #action="{ row }">
          <t-popup v-model:visible="rowPopupVisible[(row as UserRole).id]" placement="bottom-right" trigger="click"
            destroy-on-close overlay-class-name="card-more"
            :on-visible-change="(v: boolean) => onMoreVisible((row as UserRole).id, v)">
            <button class="row-more-btn" :class="{ active: moreOpen === (row as UserRole).id }" type="button"
              :aria-label="t('roles.columns.action')">
              <t-icon name="more" size="16px" />
            </button>
            <template #content>
              <ul class="card-menu">
                <li v-for="opt in buildActionOptions(row as UserRole)" :key="opt.value" class="card-menu-item"
                  @click.stop="handleActionSelect(opt.value, row as UserRole)">
                  {{ opt.label }}
                </li>
              </ul>
            </template>
          </t-popup>
        </template>
      </PageTable>
    </div>

    <!-- 底部操作栏 -->
    <footer class="roles-page__footer">
      <!-- <span class="roles-page__selection-info">
        {{ t('roles.footer.selected', { count: state.selectedIds.length, total: state.pagination.total, }) }}
      </span> -->
      <div class="roles-page__actions">
        <t-button theme="default" variant="outline" :disabled="!state.selectionStats.ko && !state.selectionStats.nonKo"
          @click="onBatchKnowledgeOfficer(true)">
          {{ t('roles.actions.setKnowledgeOfficer') }}
        </t-button>
        <t-button theme="default" variant="outline" :disabled="!state.selectionStats.ko"
          @click="onBatchKnowledgeOfficer(false)">
          {{ t('roles.actions.unsetKnowledgeOfficer') }}
        </t-button>
        <t-button theme="default" variant="outline" :disabled="!state.selectionStats.oa && !state.selectionStats.nonOa"
          @click="onBatchOpsAdmin(true)">
          {{ t('roles.actions.setOpsAdmin') }}
        </t-button>
        <t-button theme="default" variant="outline" :disabled="!state.selectionStats.oa"
          @click="onBatchOpsAdmin(false)">
          {{ t('roles.actions.unsetOpsAdmin') }}
        </t-button>
        <t-button theme="primary" @click="onAddUser">
          {{ t('roles.actions.addUser') }}
        </t-button>
      </div>
    </footer>

    <UserRoleDetailDrawer v-model:visible="detailVisible" :row="detailRow" />

    <AddUserDialog v-model:visible="addUserVisible" @created="state.load" />

    <ConfirmDialog v-model:visible="opsConfirmVisible" :title="opsConfirmTitle" :content="opsConfirmContent"
      @confirm="onOpsConfirm" @cancel="onOpsCancel" />

    <KnowledgeScopeDialog v-model:visible="scopeDialogVisible" :user-name="scopeDialogUserName"
      :default-domain-ids="scopeDialogDefaultDomainIds" :default-base-ids="scopeDialogDefaultBaseIds"
      @confirm="onScopeDialogConfirm" @cancel="onScopeDialogCancel" />

    <ConfirmDialog v-model:visible="knowledgeOfficerConfirmVisible" :title="knowledgeOfficerConfirmTitle"
      :content="knowledgeOfficerConfirmContent" theme="warning" @confirm="onKnowledgeOfficerConfirm"
      @cancel="onKnowledgeOfficerCancel" />

    <!-- 批量知识官：复用 KnowledgeScopeDialog，标题/副标题切到 batch 文案 -->
    <KnowledgeScopeDialog v-model:visible="koBatch.scopeDialogVisible.value" mode="batch"
      :user-count="koBatch.userCount.value" @confirm="koBatch.onScopeConfirm" @cancel="koBatch.onScopeCancel" />

    <ConfirmDialog v-model:visible="koBatch.confirmVisible.value" :title="koBatch.confirmTitle.value"
      :content="koBatch.confirmContent.value" theme="warning" @confirm="koBatch.onConfirm" @cancel="koBatch.onCancel" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { MessagePlugin } from 'tdesign-vue-next'
import PageSearchFilters from '@/components/common/PageSearchFilters.vue'
import PageTable from '@/components/common/PageTable.vue'
import type { PageTableColumn } from '@/components/common/PageTable.vue'
import UserRoleDetailDrawer from './components/UserRoleDetailDrawer.vue'
import AddUserDialog from './components/AddUserDialog.vue'
import KnowledgeScopeDialog from './components/KnowledgeScopeDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useUserRoles } from '@/composables/useUserRoles'
import { useKnowledgeOfficerBatchDialog } from '@/composables/useKnowledgeOfficerBatchDialog'
import type { UserRole } from '@/types/userRole'
import type { KnowledgeOfficerScope } from '@/composables/useUserRoles'

const { t } = useI18n()
const authStore = useAuthStore()

const currentUserId = authStore.currentUserId

const state = reactive(useUserRoles())

// 批量知识官流程：设走"范围选 → 确认"两阶段；取消走"确认"单阶段。
// 文案占位用的是"已选"用户数；如选到自己，batchSetKnowledgeOfficer 内部 filterOutSelf 会兜底跳过。
const koBatch = useKnowledgeOfficerBatchDialog({
  onConfirm: state.batchSetKnowledgeOfficer,
  getUserCount: () => state.selectedIds.length,
  t,
})

/** columns 包成 computed 让 title / tags.label 跟随 locale 自动重算。 */
const columns = computed<PageTableColumn<UserRole>[]>(() => [
  { key: 'selection', title: '', width: 50, },
  { key: 'account', title: t('roles.columns.account'), width: 120, ellipsis: true },
  { key: 'chineseName', title: t('roles.columns.chineseName'), width: 100, ellipsis: true },
  { key: 'email', title: t('roles.columns.email'), minWidth: 200, ellipsis: true },
  { key: 'departmentName', title: t('roles.columns.department'), minWidth: 160, ellipsis: true },
  { key: 'status', title: t('roles.columns.status'), width: 100 },
  {
    key: 'isKnowledgeOfficer',
    title: t('roles.columns.isKnowledgeOfficer'),
    width: 120,
    tags: [
      { value: true, label: t('roles.status.isOfficer') },
      { value: false, label: t('roles.status.isOfficerNo') },
    ],
  },
  {
    key: 'isOperationsAdmin',
    title: t('roles.columns.isOperationsAdmin'),
    width: 120,
    tags: [
      { value: true, label: t('roles.status.isAdmin') },
      { value: false, label: t('roles.status.isAdminNo') },
    ],
  },
  { key: 'action', title: '', width: 40, fixed: 'right' },
])

function statusTheme(s: UserRole['status']): 'success' | 'warning' | 'danger' | 'default' {
  if (s === 'active') return 'success'
  if (s === 'inactive') return 'warning'
  if (s === 'blacklisted') return 'danger'
  return 'default'
}

function statusIsBlacklisted(row: UserRole): boolean {
  return row.status === 'blacklisted'
}

/** 已选行中是否包含当前用户自己：用于批量按钮统一禁用，避免误把自己一起提交。 */
const hasSelfInSelection = computed(
  () => currentUserId !== '' && state.selectedIds.includes(currentUserId),
)

/** 拼装操作列 popup 菜单项；label 实时根据当前行状态反转文案；自己是当前用户时只保留查看详情。 */
function buildActionOptions(row: UserRole): Array<{ value: string; label: string }> {
  if (row.id === currentUserId) {
    return [{ value: 'viewDetail', label: t('roles.actions.viewDetail') }]
  }
  return [
    { value: 'viewDetail', label: t('roles.actions.viewDetail') },
    {
      value: 'toggleBlacklist',
      label: statusIsBlacklisted(row) ? t('roles.actions.unblacklist') : t('roles.actions.blacklist'),
    },
    {
      value: 'toggleOpsAdmin',
      label: row.isOperationsAdmin ? t('roles.actions.unsetOpsAdmin') : t('roles.actions.setOpsAdmin'),
    },
    {
      value: 'toggleKnowledgeOfficer',
      label: row.isKnowledgeOfficer
        ? t('roles.actions.unsetKnowledgeOfficer')
        : t('roles.actions.setKnowledgeOfficer'),
    },
  ]
}

function handleActionSelect(action: string, row: UserRole): void {
  // 先关掉 popup，避免菜单面板残留盖住下方表格。
  closeRowPopup(row.id)
  switch (action) {
    case 'viewDetail':
      openDetail(row)
      break
    case 'toggleBlacklist':
      onToggleBlacklist(row)
      break
    case 'toggleOpsAdmin':
      onToggleOpsAdmin(row)
      break
    case 'toggleKnowledgeOfficer':
      onToggleKnowledgeOfficer(row)
      break
  }
}

function onApply(): void {
  void state.applyFilter()
}

function onReset(): void {
  void state.resetFilter()
}

function onUpdateSelectedIds(ids: string[]): void {
  state.selectedIds = ids
}

function onPageChange(p: { page: number; pageSize: number }): void {
  state.pagination.page = p.page
  state.pagination.pageSize = p.pageSize
}

function onToggleBlacklist(row: UserRole): void {
  if (row.id === currentUserId) {
    MessagePlugin.warning(t('roles.messages.cannotOperateSelf'))
    return
  }
  void state.toggleBlacklist(row)
}

function onToggleOpsAdmin(row: UserRole): void {
  if (row.id === currentUserId) {
    MessagePlugin.warning(t('roles.messages.cannotOperateSelf'))
    return
  }
  const willSet = !row.isOperationsAdmin
  const name = row.chineseName || row.account
  opsConfirmRow.value = row
  opsConfirmTitle.value = t('roles.operator.confirmTitle')
  opsConfirmContent.value = willSet
    ? t('roles.operator.setMessage', { name })
    : t('roles.operator.unsetMessage', { name })
  opsConfirmVisible.value = true
}

function onOpsConfirm(): void {
  const row = opsConfirmRow.value
  opsConfirmRow.value = null
  if (row) {
    void state.toggleOpsAdmin(row)
  }
}

function onOpsCancel(): void {
  opsConfirmRow.value = null
}

function onToggleKnowledgeOfficer(row: UserRole): void {
  if (row.id === currentUserId) {
    MessagePlugin.warning(t('roles.messages.cannotOperateSelf'))
    return
  }
  const willSet = !row.isKnowledgeOfficer
  if (willSet) {
    // 设为知识官：先选范围，再二次确认。
    pendingScopeRow.value = row
    scopeDialogUserName.value = row.chineseName || row.account
    scopeDialogDefaultDomainIds.value = [...(row.knowledgeDomainIds ?? [])]
    scopeDialogDefaultBaseIds.value = [...(row.knowledgeBaseIds ?? [])]
    scopeDialogVisible.value = true
    return
  }
  // 取消知识官：跳过范围选择，直接走确认。
  knowledgeOfficerConfirmRow.value = row
  knowledgeOfficerConfirmTitle.value = t('roles.knowledgeOfficer.confirmTitle')
  knowledgeOfficerConfirmContent.value = t('roles.knowledgeOfficer.unsetMessage', {
    name: row.chineseName || row.account,
  })
  knowledgeOfficerConfirmVisible.value = true
}

function onScopeDialogConfirm(payload: { domainIds: string[]; baseIds: string[] }): void {
  const row = pendingScopeRow.value
  scopeDialogVisible.value = false
  pendingScopeRow.value = null
  if (!row) return
  // 把用户选的范围暂存到 row 引用上，确认通过后再由 composable 透传给后端。
  pendingScope.value = {
    knowledgeDomainIds: payload.domainIds,
    knowledgeBaseIds: payload.baseIds,
  }
  knowledgeOfficerConfirmRow.value = row
  knowledgeOfficerConfirmTitle.value = t('roles.knowledgeOfficer.confirmTitle')
  knowledgeOfficerConfirmContent.value = t('roles.knowledgeOfficer.setMessage', {
    name: row.chineseName || row.account,
  })
  knowledgeOfficerConfirmVisible.value = true
}

function onScopeDialogCancel(): void {
  scopeDialogVisible.value = false
  pendingScopeRow.value = null
  pendingScope.value = null
}

function onKnowledgeOfficerConfirm(): void {
  const row = knowledgeOfficerConfirmRow.value
  const scope = pendingScope.value
  knowledgeOfficerConfirmRow.value = null
  pendingScope.value = null
  if (row) {
    void state.toggleKnowledgeOfficer(row, scope ?? undefined)
  }
}

function onKnowledgeOfficerCancel(): void {
  knowledgeOfficerConfirmRow.value = null
  pendingScope.value = null
}

function onBatchKnowledgeOfficer(isOfficer: boolean): void {
  // 走批量知识官的两阶段状态机：设必选范围，取消跳过范围直接确认。
  if (isOfficer) {
    koBatch.openSet()
  } else {
    koBatch.openUnset()
  }
}

function onBatchOpsAdmin(isAdmin: boolean): void {
  void state.batchSetOperationsAdmin(isAdmin)
}

function onBatchBlacklist(isBlacklisted: boolean): void {
  void state.batchSetBlacklisted(isBlacklisted)
}

function onAddUser(): void {
  addUserVisible.value = true
}

/* 表格行"更多"按钮 popup 状态：保证 popup 打开时按钮本身保持可见（避免悬停行移开后按钮消失）。
 * rowPopupVisible 用 v-model:visible 双向绑定控制 popup 自身的开关（菜单项点击后手动置 false 关闭）。
 * moreOpen 单独追踪当前激活的 row id，仅用于按钮的 .active 高亮。 */
const moreOpen = ref<string | null>(null)
const rowPopupVisible = reactive<Record<string, boolean>>({})

function onMoreVisible(rowId: string, visible: boolean): void {
  moreOpen.value = visible ? rowId : null
}

function closeRowPopup(rowId: string): void {
  rowPopupVisible[rowId] = false
  // 同步清掉 icon 激活态：菜单项点击后 popup 通过 v-model 被外部置 false，但 TDesign 不保证触发 on-visible-change，
  // 导致 moreOpen 残留；这里直接同步，避免 icon 持续 .active 高亮。
  if (moreOpen.value === rowId) {
    moreOpen.value = null
  }
}

const detailVisible = ref(false)
const detailRow = ref<UserRole | null>(null)
const addUserVisible = ref(false)
const opsConfirmVisible = ref(false)
const opsConfirmTitle = ref('')
const opsConfirmContent = ref('')
const opsConfirmRow = ref<UserRole | null>(null)

// 知识官范围选择弹窗状态（两阶段流程第一步）。
const scopeDialogVisible = ref(false)
const scopeDialogUserName = ref('')
const scopeDialogDefaultDomainIds = ref<string[]>([])
const scopeDialogDefaultBaseIds = ref<string[]>([])
const pendingScopeRow = ref<UserRole | null>(null)
const pendingScope = ref<KnowledgeOfficerScope | null>(null)

// 知识官二次确认弹窗状态（两阶段流程第二步；设/取消共用）。
const knowledgeOfficerConfirmVisible = ref(false)
const knowledgeOfficerConfirmTitle = ref('')
const knowledgeOfficerConfirmContent = ref('')
const knowledgeOfficerConfirmRow = ref<UserRole | null>(null)

function openDetail(row: UserRole): void {
  detailRow.value = row
  detailVisible.value = true
}

onMounted(() => {
  void state.load()
})
</script>

<style lang="less" scoped>
@import '@/assets/styles/page-shared.less';

// 角色页面：覆盖 page-shell 让其不滚动，由内部表格 body 滚动
.page-shell--no-scroll {
  overflow: hidden;
  padding-bottom: 0;
}

.roles-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 16px;
}

.roles-page__table {
  flex: 1 1 auto;
  min-height: 0;
}

.roles-page__footer {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 16px;
  padding: 12px 0;
  background: var(--td-bg-color-container, #fff);
  border-top: 1px solid var(--td-component-stroke, #e7e7e7);
}

.roles-page__selection-info {
  color: var(--td-text-color-secondary, #666);
  font-size: 13px;
}

.roles-page__actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>