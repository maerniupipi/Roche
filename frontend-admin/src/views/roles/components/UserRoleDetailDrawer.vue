<template>
  <t-drawer :visible="visible" :header="t('roles.detail.title')" :size="'520'" :footer="false"
    @close="emit('update:visible', false)">
    <div v-if="row" class="user-role-detail">
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.employeeId') }}</span>
        <span class="user-role-detail__value">{{ row.employeeId || '—' }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.account') }}</span>
        <span class="user-role-detail__value">{{ row.account || '—' }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.englishName') }}</span>
        <span class="user-role-detail__value">{{ row.englishName || '—' }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.chineseName') }}</span>
        <span class="user-role-detail__value">{{ row.chineseName || '—' }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.email') }}</span>
        <span class="user-role-detail__value">{{ row.email || '—' }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.department') }}</span>
        <span class="user-role-detail__value">{{ row.departmentName || '—' }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.status') }}</span>
        <span class="user-role-detail__value">{{ statusLabel(row.status) }}</span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.isKnowledgeOfficer') }}</span>
        <span class="user-role-detail__value">
          {{ row.isKnowledgeOfficer ? t('common.yes') : t('common.no') }}
        </span>
      </div>
      <div class="user-role-detail__row">
        <span class="user-role-detail__label">{{ t('roles.detail.isOperationsAdmin') }}</span>
        <span class="user-role-detail__value">
          {{ row.isOperationsAdmin ? t('common.yes') : t('common.no') }}
        </span>
      </div>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UserRole, UserRoleStatus } from '@/types/userRole'

const { t } = useI18n()

defineProps<{
  visible: boolean
  row: UserRole | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

function statusLabel(status: UserRoleStatus): string {
  switch (status) {
    case 'active':
      return t('roles.status.active')
    case 'inactive':
      return t('roles.status.inactive')
    case 'blacklisted':
      return t('roles.status.blacklisted')
    default:
      return status
  }
}
</script>

<style lang="less" scoped>
.user-role-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 8px 0;
}

.user-role-detail__row {
  display: flex;
  gap: 12px;
  align-items: baseline;
  padding: 8px 0;
  border-bottom: 1px solid var(--td-component-stroke, #f0f0f0);
}

.user-role-detail__label {
  flex: 0 0 120px;
  color: var(--td-text-color-secondary, #666);
  font-size: 13px;
}

.user-role-detail__value {
  flex: 1;
  color: var(--td-text-color-primary, #181818);
  font-size: 14px;
}
</style>