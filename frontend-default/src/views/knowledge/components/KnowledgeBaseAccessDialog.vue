<template>
  <t-dialog
    :visible="visible"
    :header="$t('knowledgeAccess.title', { name: knowledgeBase?.name || '' })"
    width="860px"
    :footer="false"
    destroy-on-close
    @update:visible="emit('update:visible', $event)"
  >
    <div class="access-dialog-body">
      <p class="access-description">{{ $t('knowledgeAccess.description') }}</p>

      <t-radio-group v-model="resourceType" variant="default-filled">
        <t-radio-button value="knowledge_base">
          {{ $t('knowledgeAccess.wholeKnowledgeBase') }}
        </t-radio-button>
        <t-radio-button v-if="knowledgeBase?.type !== 'faq'" value="folder">
          {{ $t('knowledgeAccess.folder') }}
        </t-radio-button>
        <t-radio-button v-if="knowledgeBase?.type !== 'faq'" value="knowledge">
          {{ $t('knowledgeAccess.singleDocument') }}
        </t-radio-button>
      </t-radio-group>

      <div class="grant-editor">
        <div v-if="resourceType !== 'knowledge_base'" class="form-field resource-field">
          <label>{{ resourceType === 'folder'
            ? $t('knowledgeAccess.folder')
            : $t('knowledgeAccess.document') }}</label>
          <div class="resource-control">
            <t-select
              v-model="selectedResourceId"
              :options="resourceOptions"
              :loading="resourcesLoading"
              :placeholder="resourceType === 'folder'
                ? $t('knowledgeAccess.selectFolder')
                : $t('knowledgeAccess.selectDocument')"
              filterable
            />
            <t-popconfirm
              theme="warning"
              :content="resourceDeleteConfirm"
              :confirm-btn="{ content: $t('knowledgeAccess.deleteResource'), theme: 'danger' }"
              :cancel-btn="{ content: $t('common.cancel') }"
              @confirm="deleteSelectedResource"
            >
              <t-button
                theme="danger"
                variant="outline"
                :disabled="!selectedResourceId"
                :loading="deletingResource"
              >
                <template #icon><t-icon name="delete" /></template>
                {{ $t('knowledgeAccess.deleteResource') }}
              </t-button>
            </t-popconfirm>
          </div>
        </div>

        <div class="form-field">
          <label>{{ $t('knowledgeAccess.subjectType') }}</label>
          <t-radio-group v-model="subjectType">
            <t-radio value="org_unit">{{ $t('knowledgeAccess.organization') }}</t-radio>
            <t-radio value="user">{{ $t('knowledgeAccess.user') }}</t-radio>
          </t-radio-group>
        </div>

        <div class="form-field">
          <label>{{ $t('knowledgeAccess.permission') }}</label>
          <t-radio-group v-model="permission">
            <t-radio value="read">{{ $t('knowledgeAccess.read') }}</t-radio>
            <t-radio value="manage">{{ $t('knowledgeAccess.manage') }}</t-radio>
          </t-radio-group>
        </div>

        <div class="form-field">
          <label>{{ $t('knowledgeAccess.effect') }}</label>
          <t-radio-group v-model="effect">
            <t-radio value="allow">{{ $t('knowledgeAccess.allow') }}</t-radio>
            <t-radio value="deny">{{ $t('knowledgeAccess.deny') }}</t-radio>
          </t-radio-group>
        </div>

        <div v-if="resourceType === 'folder'" class="form-field">
          <label>{{ $t('knowledgeAccess.inherit') }}</label>
          <t-checkbox v-model="inheritToChildren">
            {{ $t('knowledgeAccess.inheritHint') }}
          </t-checkbox>
        </div>

        <div class="form-field target-field">
          <label>{{ $t('knowledgeAccess.subject') }}</label>
          <t-select
            v-if="subjectType === 'org_unit'"
            v-model="selectedSubjectId"
            :options="orgUnitOptions"
            :loading="subjectsLoading"
            :placeholder="$t('knowledgeAccess.selectOrganization')"
            filterable
          />
          <t-select
            v-else
            v-model="selectedSubjectId"
            :options="userOptions"
            :loading="subjectsLoading"
            :placeholder="$t('knowledgeAccess.selectUser')"
            filterable
          />
          <t-button :loading="saving" :disabled="!canGrant" @click="grantAccess">
            <template #icon><t-icon name="add" /></template>
            {{ effect === 'deny' ? $t('knowledgeAccess.addDeny') : $t('knowledgeAccess.grant') }}
          </t-button>
        </div>
      </div>

      <t-alert
        v-if="effect === 'deny'"
        theme="warning"
        :message="$t('knowledgeAccess.denyHint')"
      />
      <t-alert
        v-else-if="resourceType === 'knowledge'"
        theme="info"
        :message="$t('knowledgeAccess.documentGrantHint')"
      />
      <t-alert v-if="error" theme="error" :message="error" />

      <div class="grant-list">
        <div class="grant-list-header">
          <div>
            <strong>{{ $t('knowledgeAccess.directRules') }}</strong>
            <span v-if="selectedResourceName">{{ selectedResourceName }}</span>
          </div>
          <span>{{ grants.length }}</span>
        </div>

        <t-loading v-if="loading" size="small" />
        <template v-else-if="grants.length">
          <div v-for="grant in grants" :key="grant.id" class="grant-row">
            <div class="grant-copy">
              <div class="grant-title-line">
                <t-tag size="small" variant="light">
                  {{ grant.subject_type === 'org_unit'
                    ? $t('knowledgeAccess.organization')
                    : $t('knowledgeAccess.user') }}
                </t-tag>
                <strong>{{ grant.subject_name || grant.subject_id }}</strong>
                <t-tag
                  size="small"
                  :theme="grant.effect === 'deny' ? 'danger' : 'success'"
                  variant="light"
                >
                  {{ grant.effect === 'deny'
                    ? $t('knowledgeAccess.deny')
                    : $t('knowledgeAccess.allow') }}
                </t-tag>
                <t-tag size="small" variant="outline">
                  {{ grant.permission === 'manage'
                    ? $t('knowledgeAccess.manage')
                    : $t('knowledgeAccess.read') }}
                </t-tag>
              </div>
              <span v-if="grant.subject_detail">{{ grant.subject_detail }}</span>
              <span v-if="grant.inherit_to_children">
                {{ $t('knowledgeAccess.inheritedRule') }}
              </span>
            </div>

            <t-popconfirm
              theme="warning"
              :content="$t('knowledgeAccess.revokeConfirm')"
              @confirm="revokeAccess(grant)"
            >
              <t-button
                theme="danger"
                variant="text"
                :loading="revokingIds.has(grant.id)"
              >
                {{ $t('knowledgeAccess.revoke') }}
              </t-button>
            </t-popconfirm>
          </div>
        </template>
        <t-empty v-else :description="$t('knowledgeAccess.noGrants')" />
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  batchDeleteKnowledge,
  deleteKnowledgeFolder,
  listKnowledgeFiles,
  listKnowledgeFolders,
  type KnowledgeFolder,
} from '@/api/knowledge-base'
import {
  listKnowledgeResourceGrants,
  listKnowledgeResourceGrantSubjects,
  revokeKnowledgeResourceGrant,
  upsertKnowledgeResourceGrant,
  type GrantEffect,
  type GrantSubjectType,
  type GrantUser,
  type KnowledgePermission,
  type KnowledgeResourceGrant,
  type KnowledgeResourceType,
  type OrgUnit,
} from '@/api/enterprise-access'

interface KnowledgeBaseSummary {
  id: string
  name: string
  knowledge_domain_id: number
  type?: string
}

interface KnowledgeOption {
  id: string
  title?: string
  file_name?: string
  original_file_name?: string
  folder_id?: string
}

const props = defineProps<{
  visible: boolean
  knowledgeBase: KnowledgeBaseSummary | null
}>()
const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'resources-changed'): void
}>()

const { t } = useI18n()
const resourceType = ref<KnowledgeResourceType>('knowledge_base')
const selectedResourceId = ref('')
const subjectType = ref<GrantSubjectType>('org_unit')
const selectedSubjectId = ref('')
const permission = ref<KnowledgePermission>('read')
const effect = ref<GrantEffect>('allow')
const inheritToChildren = ref(true)
const orgUnits = ref<OrgUnit[]>([])
const users = ref<GrantUser[]>([])
const documents = ref<KnowledgeOption[]>([])
const folders = ref<KnowledgeFolder[]>([])
const grants = ref<KnowledgeResourceGrant[]>([])
const loading = ref(false)
const subjectsLoading = ref(false)
const resourcesLoading = ref(false)
const saving = ref(false)
const deletingResource = ref(false)
const error = ref('')
const revokingIds = ref(new Set<number>())
let grantLoadSequence = 0
let subjectLoadSequence = 0

const activeResourceId = computed(() => (
  resourceType.value === 'knowledge_base'
    ? props.knowledgeBase?.id || ''
    : selectedResourceId.value
))
const activeOrgUnits = computed(() => orgUnits.value.filter((unit) => unit.status === 'active'))
const orgUnitOptions = computed(() => activeOrgUnits.value.map((unit) => ({
  label: formatOrgUnitLabel(unit),
  value: unit.id,
})))
const userOptions = computed(() => users.value.map((user) => ({
  label: `${user.username?.trim() || user.email} (${user.email})`,
  value: user.id,
})))
const folderOptions = computed(() => folders.value.map((folder) => ({
  label: folder.relative_path || folder.name,
  value: folder.id,
})))
const knowledgeOptions = computed(() => documents.value.map((knowledge) => ({
  label: knowledge.title || knowledge.original_file_name || knowledge.file_name || knowledge.id,
  value: knowledge.id,
})))
const resourceOptions = computed(() => (
  resourceType.value === 'folder' ? folderOptions.value : knowledgeOptions.value
))
const selectedResourceName = computed(() => {
  if (resourceType.value === 'knowledge_base') return props.knowledgeBase?.name || ''
  return resourceOptions.value.find((option) => option.value === selectedResourceId.value)?.label || ''
})
const resourceDeleteConfirm = computed(() => (
  resourceType.value === 'folder'
    ? t('knowledgeAccess.deleteFolderConfirm', { name: selectedResourceName.value })
    : t('knowledgeAccess.deleteDocumentConfirm', { name: selectedResourceName.value })
))
const canGrant = computed(() => (
  Boolean(props.knowledgeBase?.id)
  && Boolean(activeResourceId.value)
  && Boolean(selectedSubjectId.value)
))

function formatOrgUnitLabel(unit: OrgUnit): string {
  const path: string[] = [unit.name]
  const seen = new Set<string>([unit.id])
  let parentId = unit.parent_id
  while (parentId) {
    const parent = orgUnits.value.find((candidate) => candidate.id === parentId)
    if (!parent || seen.has(parent.id)) break
    path.unshift(parent.name)
    seen.add(parent.id)
    parentId = parent.parent_id
  }
  return `${path.join(' / ')} (${unit.code})`
}

async function loadResources(): Promise<void> {
  if (!props.knowledgeBase || props.knowledgeBase.type === 'faq') return
  resourcesLoading.value = true
  try {
    const collected: KnowledgeOption[] = []
    let page = 1
    let total = 0
    do {
      const response: any = await listKnowledgeFiles(props.knowledgeBase.id, {
        page,
        page_size: 100,
      })
      const items = Array.isArray(response?.data) ? response.data : []
      collected.push(...items)
      total = Number(response?.total || collected.length)
      page += 1
    } while (collected.length < total && page <= 100)
    documents.value = collected

    const folderResponse: any = await listKnowledgeFolders(props.knowledgeBase.id)
    folders.value = Array.isArray(folderResponse?.data) ? folderResponse.data : []
  } catch (err: any) {
    error.value = err?.message || t('knowledgeAccess.loadFailed')
  } finally {
    resourcesLoading.value = false
  }
}

async function loadSubjects(): Promise<void> {
  if (!props.knowledgeBase || !activeResourceId.value) {
    subjectLoadSequence += 1
    orgUnits.value = []
    users.value = []
    return
  }
  const sequence = ++subjectLoadSequence
  subjectsLoading.value = true
  try {
    const response = await listKnowledgeResourceGrantSubjects(
      props.knowledgeBase.id,
      resourceType.value,
      activeResourceId.value,
    )
    if (!response.success) throw new Error(response.message)
    if (sequence === subjectLoadSequence) {
      orgUnits.value = response.data?.org_units || []
      users.value = response.data?.users || []
    }
  } catch (err: any) {
    if (sequence === subjectLoadSequence) {
      orgUnits.value = []
      users.value = []
      error.value = err?.message || t('knowledgeAccess.loadFailed')
    }
  } finally {
    if (sequence === subjectLoadSequence) subjectsLoading.value = false
  }
}

async function loadGrants(): Promise<void> {
  if (!props.knowledgeBase || !activeResourceId.value) {
    grants.value = []
    return
  }
  const sequence = ++grantLoadSequence
  loading.value = true
  try {
    const response = await listKnowledgeResourceGrants(
      props.knowledgeBase.id,
      resourceType.value,
      activeResourceId.value,
    )
    if (!response.success) throw new Error(response.message)
    if (sequence === grantLoadSequence) grants.value = response.data || []
  } catch (err: any) {
    if (sequence === grantLoadSequence) {
      grants.value = []
      error.value = err?.message || t('knowledgeAccess.loadFailed')
    }
  } finally {
    if (sequence === grantLoadSequence) loading.value = false
  }
}

function resetResourceSelection(): void {
  resourceType.value = 'knowledge_base'
  selectedResourceId.value = ''
}

async function initialize(): Promise<void> {
  if (!props.knowledgeBase) return
  subjectType.value = 'org_unit'
  selectedSubjectId.value = ''
  permission.value = 'read'
  effect.value = 'allow'
  inheritToChildren.value = true
  error.value = ''
  grants.value = []
  resetResourceSelection()
  await Promise.all([loadSubjects(), loadResources()])
  await loadGrants()
}

async function deleteSelectedResource(): Promise<void> {
  if (!props.knowledgeBase || !selectedResourceId.value || deletingResource.value) return
  const deletingID = selectedResourceId.value
  deletingResource.value = true
  try {
    const response: any = resourceType.value === 'folder'
      ? await deleteKnowledgeFolder(props.knowledgeBase.id, deletingID)
      : await batchDeleteKnowledge(props.knowledgeBase.id, [deletingID])
    if (response?.success === false) {
      throw new Error(response?.message || t('knowledgeAccess.deleteFailed'))
    }

    if (resourceType.value === 'knowledge') {
      documents.value = documents.value.filter((document) => document.id !== deletingID)
    } else {
      await loadResources()
    }
    selectedResourceId.value = ''
    grants.value = []
    emit('resources-changed')
    MessagePlugin.success(t('knowledgeAccess.resourceDeleted'))
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeAccess.deleteFailed'))
  } finally {
    deletingResource.value = false
  }
}

async function grantAccess(): Promise<void> {
  if (!props.knowledgeBase || !canGrant.value || saving.value) return
  saving.value = true
  try {
    const response = await upsertKnowledgeResourceGrant(
      props.knowledgeBase.id,
      resourceType.value,
      activeResourceId.value,
      {
        subject_type: subjectType.value,
        subject_id: selectedSubjectId.value,
        permission: permission.value,
        effect: effect.value,
        inherit_to_children: resourceType.value === 'knowledge_base'
          ? true
          : resourceType.value === 'folder' && inheritToChildren.value,
      },
    )
    if (!response.success) throw new Error(response.message)
    await loadGrants()
    selectedSubjectId.value = ''
    MessagePlugin.success(t('knowledgeAccess.granted'))
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeAccess.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function revokeAccess(grant: KnowledgeResourceGrant): Promise<void> {
  if (!props.knowledgeBase || revokingIds.value.has(grant.id)) return
  revokingIds.value = new Set(revokingIds.value).add(grant.id)
  try {
    const response = await revokeKnowledgeResourceGrant(props.knowledgeBase.id, grant.id)
    if (!response.success) throw new Error(response.message)
    await loadGrants()
    MessagePlugin.success(t('knowledgeAccess.revoked'))
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeAccess.saveFailed'))
  } finally {
    const next = new Set(revokingIds.value)
    next.delete(grant.id)
    revokingIds.value = next
  }
}

watch(subjectType, () => {
  selectedSubjectId.value = ''
})

watch(resourceType, () => {
  selectedResourceId.value = ''
  inheritToChildren.value = resourceType.value !== 'knowledge'
  void loadGrants()
  void loadSubjects()
})

watch(selectedResourceId, () => {
  void loadGrants()
  void loadSubjects()
})

watch(() => props.visible, (visible) => {
  if (visible) void initialize()
})
</script>

<style scoped lang="less">
.access-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.access-description {
  margin: 0;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
}

.grant-editor {
  display: grid;
  gap: 14px;
  padding: 16px;
  background: var(--td-bg-color-container-hover);
  border-radius: 8px;
}

.form-field {
  display: grid;
  grid-template-columns: 108px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
}

.form-field label {
  color: var(--td-text-color-secondary);
}

.resource-control {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
}

.target-field {
  grid-template-columns: 108px minmax(0, 1fr) auto;
}

.grant-list {
  min-height: 220px;
  max-height: 380px;
  overflow-y: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
}

.grant-list-header {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: var(--td-bg-color-container);
  border-bottom: 1px solid var(--td-component-stroke);
}

.grant-list-header > div {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.grant-list-header span {
  overflow: hidden;
  color: var(--td-text-color-placeholder);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.grant-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.grant-row:last-child {
  border-bottom: 0;
}

.grant-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
}

.grant-title-line {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.grant-title-line strong,
.grant-copy span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.grant-copy > span {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

:deep(.t-empty) {
  padding: 36px 0;
}

@media (max-width: 720px) {
  .form-field,
  .target-field {
    grid-template-columns: 1fr;
  }

  .resource-control {
    grid-template-columns: 1fr;
  }

  .grant-row {
    align-items: flex-start;
  }
}
</style>
