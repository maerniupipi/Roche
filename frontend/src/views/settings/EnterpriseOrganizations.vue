<template>
  <div class="enterprise-organizations">
    <header class="section-header">
      <div>
        <h2>{{ $t('enterpriseOrg.title') }}</h2>
        <p>{{ $t('enterpriseOrg.description') }}</p>
      </div>
      <div class="section-actions">
        <t-popconfirm
          theme="warning"
          :content="$t('enterpriseOrg.workdaySyncConfirm')"
          @confirm="syncWorkday"
        >
          <t-button size="small" variant="outline" :loading="workdaySyncing">
            <template #icon><t-icon name="refresh" /></template>
            {{ $t('enterpriseOrg.workdaySync') }}
          </t-button>
        </t-popconfirm>
        <t-button @click="openCreateDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('enterpriseOrg.create') }}
        </t-button>
      </div>
    </header>

    <t-alert v-if="error" theme="error" :message="error" class="error-alert" />

    <div class="organization-layout">
      <aside class="org-panel">
        <div class="panel-title">
          <strong>{{ $t('enterpriseOrg.organizationTree') }}</strong>
          <t-button variant="text" shape="square" :loading="loading" @click="loadOrganizations">
            <template #icon><t-icon name="refresh" /></template>
          </t-button>
        </div>
        <div v-if="loading" class="center-state"><t-loading size="small" /></div>
        <button
          v-for="item in flattenedOrgUnits"
          v-else
          :key="item.unit.id"
          type="button"
          class="org-row"
          :class="{ active: selectedOrgUnitId === item.unit.id }"
          :style="{ paddingLeft: `${14 + item.depth * 18}px` }"
          @click="selectOrgUnit(item.unit.id)"
        >
          <t-icon name="organization" />
          <span>
            <strong>{{ item.unit.name }}</strong>
            <small>{{ item.unit.code }}</small>
          </span>
          <t-tag v-if="item.unit.source === 'workday'" size="small" variant="light">
            Workday
          </t-tag>
        </button>
      </aside>

      <main class="member-panel">
        <template v-if="selectedOrgUnit">
          <div class="member-header">
            <div>
              <div class="member-title-line">
                <h3>{{ selectedOrgUnit.name }}</h3>
                <t-tag size="small" variant="light">{{ selectedOrgUnit.code }}</t-tag>
              </div>
              <p>{{ $t('enterpriseOrg.memberDescription') }}</p>
            </div>
            <div v-if="selectedOrgUnit.source !== 'bootstrap'" class="header-actions">
              <t-button variant="outline" @click="openEditDialog">
                {{ $t('common.edit') }}
              </t-button>
              <t-popconfirm
                theme="warning"
                :content="$t('enterpriseOrg.deleteConfirm')"
                @confirm="removeOrgUnit"
              >
                <t-button theme="danger" variant="outline">
                  {{ $t('common.delete') }}
                </t-button>
              </t-popconfirm>
            </div>
          </div>

          <div class="member-adder">
            <t-select
              v-model="selectedUserId"
              :options="availableUserOptions"
              :loading="usersLoading"
              :placeholder="$t('enterpriseOrg.selectUser')"
              filterable
            />
            <t-button :disabled="!selectedUserId" :loading="memberSaving" @click="addMember">
              <template #icon><t-icon name="user-add" /></template>
              {{ $t('enterpriseOrg.addMember') }}
            </t-button>
          </div>

          <div class="member-list">
            <t-loading v-if="membersLoading" size="small" />
            <template v-else-if="members.length">
              <div v-for="member in members" :key="member.membership_id" class="member-row">
                <div class="member-profile">
                  <t-avatar size="36px" :image="member.avatar">
                    {{ memberName(member).slice(0, 1).toUpperCase() }}
                  </t-avatar>
                  <div>
                    <strong>{{ memberName(member) }}</strong>
                    <span>{{ member.email }}</span>
                  </div>
                  <t-tag v-if="member.is_primary" size="small" variant="light">
                    {{ $t('enterpriseOrg.primary') }}
                  </t-tag>
                  <t-tag v-if="member.source === 'workday'" size="small" variant="light">
                    Workday
                  </t-tag>
                </div>
                <t-popconfirm
                  v-if="member.source !== 'workday'"
                  theme="warning"
                  :content="$t('enterpriseOrg.removeMemberConfirm')"
                  @confirm="removeMember(member)"
                >
                  <t-button theme="danger" variant="text" shape="square">
                    <template #icon><t-icon name="delete" /></template>
                  </t-button>
                </t-popconfirm>
                <span v-else class="managed-label">{{ $t('enterpriseOrg.managedByWorkday') }}</span>
              </div>
            </template>
            <t-empty v-else :description="$t('enterpriseOrg.noMembers')" />
          </div>
        </template>
        <t-empty v-else :description="$t('enterpriseOrg.selectOrganization')" />
      </main>
    </div>

    <t-dialog
      v-model:visible="editorVisible"
      :header="editingOrgUnit ? $t('enterpriseOrg.edit') : $t('enterpriseOrg.create')"
      :confirm-btn="{ content: $t('common.confirm'), loading: editorSaving }"
      :cancel-btn="$t('common.cancel')"
      @confirm="saveOrgUnit"
    >
      <div class="org-form">
        <label>{{ $t('enterpriseOrg.name') }}</label>
        <t-input v-model="orgForm.name" />
        <label>{{ $t('enterpriseOrg.code') }}</label>
        <t-input v-model="orgForm.code" />
        <label>{{ $t('enterpriseOrg.parent') }}</label>
        <t-select
          v-model="orgForm.parent_id"
          :options="parentOptions"
          clearable
          filterable
          :placeholder="$t('enterpriseOrg.noParent')"
        />
        <label>{{ $t('enterpriseOrg.status') }}</label>
        <t-radio-group v-model="orgForm.status">
          <t-radio value="active">{{ $t('enterpriseOrg.active') }}</t-radio>
          <t-radio value="inactive">{{ $t('enterpriseOrg.inactive') }}</t-radio>
        </t-radio-group>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  createOrgUnit,
  deleteOrgUnit,
  listOrgUnitMembers,
  listOrgUnits,
  listUserOrgMemberships,
  replaceUserOrgMemberships,
  searchGrantUsers,
  updateOrgUnit,
  type GrantUser,
  type OrgUnit,
  type OrgUnitMember,
  type UserOrgMembership,
} from '@/api/enterprise-access'
import {
  getWorkdaySyncRun,
  triggerWorkdayFullSync,
} from '@/api/enterprise-integration'

const ENTERPRISE_ROOT_ID = '00000000-0000-0000-0000-000000000001'

const { t } = useI18n()
const orgUnits = ref<OrgUnit[]>([])
const users = ref<GrantUser[]>([])
const members = ref<OrgUnitMember[]>([])
const selectedOrgUnitId = ref('')
const selectedUserId = ref('')
const loading = ref(false)
const usersLoading = ref(false)
const membersLoading = ref(false)
const memberSaving = ref(false)
const workdaySyncing = ref(false)
const error = ref('')
const editorVisible = ref(false)
const editorSaving = ref(false)
const editingOrgUnit = ref<OrgUnit | null>(null)
const orgForm = ref({
  name: '',
  code: '',
  parent_id: '',
  status: 'active' as 'active' | 'inactive',
})

const selectedOrgUnit = computed(() => (
  orgUnits.value.find((unit) => unit.id === selectedOrgUnitId.value) || null
))

const flattenedOrgUnits = computed(() => {
  const result: Array<{ unit: OrgUnit; depth: number }> = []
  const byParent = new Map<string, OrgUnit[]>()
  for (const unit of orgUnits.value) {
    const parentKey = unit.parent_id || ''
    const children = byParent.get(parentKey) || []
    children.push(unit)
    byParent.set(parentKey, children)
  }
  for (const children of byParent.values()) {
    children.sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name))
  }
  const visited = new Set<string>()
  const append = (parentId: string, depth: number) => {
    for (const unit of byParent.get(parentId) || []) {
      if (visited.has(unit.id)) continue
      visited.add(unit.id)
      result.push({ unit, depth })
      append(unit.id, depth + 1)
    }
  }
  append('', 0)
  for (const unit of orgUnits.value) {
    if (!visited.has(unit.id)) result.push({ unit, depth: 0 })
  }
  return result
})

const memberUserIds = computed(() => new Set(members.value.map((member) => member.user_id)))
const availableUserOptions = computed(() => users.value
  .filter((user) => !memberUserIds.value.has(user.id))
  .map((user) => ({
    label: `${user.username?.trim() || user.email} (${user.email})`,
    value: user.id,
  })))

const excludedParentIds = computed(() => {
  if (!editingOrgUnit.value) return new Set<string>()
  const excluded = new Set<string>([editingOrgUnit.value.id])
  let changed = true
  while (changed) {
    changed = false
    for (const unit of orgUnits.value) {
      if (unit.parent_id && excluded.has(unit.parent_id) && !excluded.has(unit.id)) {
        excluded.add(unit.id)
        changed = true
      }
    }
  }
  return excluded
})

const parentOptions = computed(() => orgUnits.value
  .filter((unit) => unit.status === 'active' && !excludedParentIds.value.has(unit.id))
  .map((unit) => ({ label: `${unit.name} (${unit.code})`, value: unit.id })))

function memberName(member: OrgUnitMember): string {
  return member.username?.trim() || member.email?.trim() || '-'
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}

async function syncWorkday(): Promise<void> {
  if (workdaySyncing.value) return
  workdaySyncing.value = true
  try {
    const response = await triggerWorkdayFullSync()
    if (!response.success || !response.data?.id) {
      throw new Error(response.message || t('enterpriseOrg.workdaySyncFailed'))
    }

    const runId = response.data.id
    for (let attempt = 0; attempt < 90; attempt += 1) {
      const runResponse = await getWorkdaySyncRun(runId)
      if (!runResponse.success || !runResponse.data) {
        throw new Error(runResponse.message || t('enterpriseOrg.workdaySyncFailed'))
      }
      if (runResponse.data.status === 'succeeded') {
        await Promise.all([loadOrganizations(), loadUsers()])
        MessagePlugin.success(t('enterpriseOrg.workdaySyncSucceeded'))
        return
      }
      if (runResponse.data.status === 'failed') {
        throw new Error(
          runResponse.data.error_summary || t('enterpriseOrg.workdaySyncFailed'),
        )
      }
      await wait(1000)
    }
    MessagePlugin.warning(t('enterpriseOrg.workdaySyncPending'))
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('enterpriseOrg.workdaySyncFailed'))
  } finally {
    workdaySyncing.value = false
  }
}

async function loadOrganizations(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const response = await listOrgUnits()
    if (!response.success) throw new Error(response.message)
    orgUnits.value = response.data || []
    if (!selectedOrgUnitId.value || !orgUnits.value.some((unit) => unit.id === selectedOrgUnitId.value)) {
      selectedOrgUnitId.value = orgUnits.value[0]?.id || ''
    }
    if (selectedOrgUnitId.value) await loadMembers()
  } catch (err: any) {
    error.value = err?.message || t('enterpriseOrg.loadFailed')
  } finally {
    loading.value = false
  }
}

async function loadUsers(): Promise<void> {
  usersLoading.value = true
  try {
    const response = await searchGrantUsers('', 100)
    if (!response.success) throw new Error(response.message)
    users.value = response.data || []
  } catch (err: any) {
    error.value = err?.message || t('enterpriseOrg.loadFailed')
  } finally {
    usersLoading.value = false
  }
}

async function loadMembers(): Promise<void> {
  if (!selectedOrgUnitId.value) return
  membersLoading.value = true
  try {
    const response = await listOrgUnitMembers(selectedOrgUnitId.value)
    if (!response.success) throw new Error(response.message)
    members.value = response.data || []
  } catch (err: any) {
    error.value = err?.message || t('enterpriseOrg.loadFailed')
  } finally {
    membersLoading.value = false
  }
}

function selectOrgUnit(id: string): void {
  selectedOrgUnitId.value = id
  selectedUserId.value = ''
  void loadMembers()
}

function openCreateDialog(): void {
  editingOrgUnit.value = null
  orgForm.value = {
    name: '',
    code: '',
    parent_id: selectedOrgUnitId.value || '',
    status: 'active',
  }
  editorVisible.value = true
}

function openEditDialog(): void {
  if (!selectedOrgUnit.value) return
  editingOrgUnit.value = selectedOrgUnit.value
  orgForm.value = {
    name: selectedOrgUnit.value.name,
    code: selectedOrgUnit.value.code,
    parent_id: selectedOrgUnit.value.parent_id || '',
    status: selectedOrgUnit.value.status,
  }
  editorVisible.value = true
}

async function saveOrgUnit(): Promise<void> {
  if (!orgForm.value.name.trim() || !orgForm.value.code.trim()) {
    MessagePlugin.warning(t('enterpriseOrg.requiredFields'))
    return
  }
  editorSaving.value = true
  try {
    const payload = {
      name: orgForm.value.name.trim(),
      code: orgForm.value.code.trim(),
      parent_id: orgForm.value.parent_id || undefined,
      status: orgForm.value.status,
    }
    const response = editingOrgUnit.value
      ? await updateOrgUnit(editingOrgUnit.value.id, payload)
      : await createOrgUnit(payload)
    if (!response.success) throw new Error(response.message)
    editorVisible.value = false
    MessagePlugin.success(t('common.success'))
    await loadOrganizations()
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('common.failed'))
  } finally {
    editorSaving.value = false
  }
}

async function removeOrgUnit(): Promise<void> {
  if (!selectedOrgUnit.value) return
  try {
    const response = await deleteOrgUnit(selectedOrgUnit.value.id)
    if (!response.success) throw new Error(response.message)
    selectedOrgUnitId.value = ''
    MessagePlugin.success(t('common.success'))
    await loadOrganizations()
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('common.failed'))
  }
}

function normalizedMemberships(memberships: UserOrgMembership[]) {
  return memberships.map((membership) => ({
    org_unit_id: membership.org_unit_id,
    is_primary: membership.is_primary,
  }))
}

async function addMember(): Promise<void> {
  if (!selectedUserId.value || !selectedOrgUnit.value) return
  memberSaving.value = true
  try {
    const response = await listUserOrgMemberships(selectedUserId.value)
    if (!response.success) throw new Error(response.message)
    let memberships = response.data || []
    if (memberships.some((membership) => membership.org_unit_id === selectedOrgUnit.value?.id)) return

    const onlyBootstrap = memberships.length === 1 && memberships[0].org_unit_id === ENTERPRISE_ROOT_ID
    if (onlyBootstrap && selectedOrgUnit.value.id !== ENTERPRISE_ROOT_ID) {
      memberships = []
    }
    memberships.push({
      org_unit_id: selectedOrgUnit.value.id,
      is_primary: memberships.length === 0,
    })
    const saveResponse = await replaceUserOrgMemberships(
      selectedUserId.value,
      normalizedMemberships(memberships),
    )
    if (!saveResponse.success) throw new Error(saveResponse.message)
    selectedUserId.value = ''
    MessagePlugin.success(t('enterpriseOrg.memberAdded'))
    await loadMembers()
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('common.failed'))
  } finally {
    memberSaving.value = false
  }
}

async function removeMember(member: OrgUnitMember): Promise<void> {
  try {
    const response = await listUserOrgMemberships(member.user_id)
    if (!response.success) throw new Error(response.message)
    let memberships = (response.data || []).filter(
      (membership) => membership.org_unit_id !== selectedOrgUnitId.value,
    )
    if (memberships.length === 0) {
      memberships = [{ org_unit_id: ENTERPRISE_ROOT_ID, is_primary: true }]
    } else if (!memberships.some((membership) => membership.is_primary)) {
      memberships[0].is_primary = true
    }
    const saveResponse = await replaceUserOrgMemberships(
      member.user_id,
      normalizedMemberships(memberships),
    )
    if (!saveResponse.success) throw new Error(saveResponse.message)
    MessagePlugin.success(t('enterpriseOrg.memberRemoved'))
    await loadMembers()
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('common.failed'))
  }
}

void Promise.all([loadOrganizations(), loadUsers()])
</script>

<style scoped lang="less">
.enterprise-organizations {
  padding: 24px 32px;
}

.section-header,
.member-header,
.member-title-line,
.section-actions,
.member-adder,
.panel-title,
.member-row,
.member-profile {
  display: flex;
  align-items: center;
}

.section-header,
.member-header,
.member-row,
.panel-title {
  justify-content: space-between;
}

.section-header {
  gap: 24px;
  margin-bottom: 20px;
}

.section-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.section-header h2,
.member-header h3 {
  margin: 0;
}

.section-header p,
.member-header p {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
}

.error-alert {
  margin-bottom: 16px;
}

.organization-layout {
  display: grid;
  min-height: 520px;
  grid-template-columns: 300px minmax(0, 1fr);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
}

.org-panel {
  border-right: 1px solid var(--td-component-stroke);
  overflow-y: auto;
}

.panel-title {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 12px 14px;
  background: var(--td-bg-color-container);
  border-bottom: 1px solid var(--td-component-stroke);
}

.org-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 9px;
  padding-top: 10px;
  padding-right: 12px;
  padding-bottom: 10px;
  color: inherit;
  text-align: left;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
}

.org-row:hover,
.org-row.active {
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

.org-row span {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.org-row strong,
.org-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.org-row small {
  color: var(--td-text-color-secondary);
}

.member-panel {
  min-width: 0;
  padding: 20px;
}

.member-header {
  align-items: flex-start;
  gap: 16px;
}

.member-title-line,
.header-actions,
.member-adder,
.member-profile {
  gap: 10px;
}

.member-adder {
  margin: 20px 0 14px;
}

.member-adder :deep(.t-select__wrap) {
  flex: 1;
}

.member-list {
  min-height: 350px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
}

.member-row {
  gap: 16px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.member-row:last-child {
  border-bottom: 0;
}

.member-profile {
  min-width: 0;
}

.member-profile > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.member-profile span,
.managed-label {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.center-state {
  display: flex;
  min-height: 160px;
  align-items: center;
  justify-content: center;
}

.org-form {
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr);
  align-items: center;
  gap: 16px 12px;
}

@media (max-width: 900px) {
  .organization-layout {
    grid-template-columns: 1fr;
  }

  .org-panel {
    max-height: 260px;
    border-right: 0;
    border-bottom: 1px solid var(--td-component-stroke);
  }
}
</style>
