<template>
  <t-dialog :visible="visible" :header="$t('roles.messages.addUser.dialogTitle')" :confirm-btn="{
    content: $t('roles.messages.addUser.submit'),
    loading: saving,
  }" :cancel-btn="$t('roles.messages.addUser.cancel')" :on-confirm="handleConfirm" :on-close="handleCloseAttempt"
    :on-cancel="handleCloseAttempt" width="480px" @update:visible="emit('update:visible', $event)">
    <p class="dialog-subtitle">
      {{ $t('roles.messages.addUser.dialogSubtitle') }}
    </p>
    <div class="add-user-form">
      <label for="add-user-input">
        {{ $t('roles.messages.addUser.inputLabel') }}
        <span class="required">*</span>
      </label>
      <t-input id="add-user-input" v-model="input" :maxlength="128"
        :placeholder="$t('roles.messages.addUser.inputPlaceholder')" @enter="handleConfirm" />
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { createUser } from '@/api/user-roles'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'created'): void
}>()

const { t } = useI18n()

const input = ref('')
const saving = ref(false)

// 仅在 false → true 时重置；true → false 不重置以避免动画闪烁。
watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      input.value = ''
      saving.value = false
    }
  },
)

async function handleConfirm(): Promise<void> {
  if (saving.value) return
  const value = input.value.trim()
  if (!value) {
    MessagePlugin.warning(t('roles.messages.addUser.emptyInput'))
    return
  }

  // 前端识别：含 '@' 走 email 分支，否则走 user_id 分支。
  const payload = value.includes('@') ? { email: value } : { user_id: value }

  saving.value = true
  try {
    await createUser(payload)
    MessagePlugin.success(t('roles.messages.addUser.userCreated'))
    emit('created')
    emit('update:visible', false)
  } catch (error: any) {
    const message =
      error?.message || t('roles.messages.addUser.createFailed')
    MessagePlugin.error(message)
  } finally {
    saving.value = false
  }
}

// 提交中拦截关闭，避免请求未完成时弹窗消失导致状态丢失。
function handleCloseAttempt(): void {
  if (saving.value) return
  emit('update:visible', false)
}
</script>

<style scoped lang="less">
.dialog-subtitle {
  margin: 0 0 20px;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
}

.add-user-form {
  display: grid;
  gap: 10px;

  label {
    margin-top: 6px;
    color: var(--td-text-color-primary);
    font-weight: 500;
  }
}

.required {
  color: var(--td-error-color);
}
</style>
