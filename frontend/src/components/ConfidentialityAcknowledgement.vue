<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import {
  getConfidentialityAck,
  acknowledgeConfidentiality,
} from '@/api/auth'

const { t } = useI18n()
const authStore = useAuthStore()

const props = defineProps<{
  modelValue?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

// v-model 双向绑定：外部可触发打开，内部关闭也会回写到父组件 ref
const visible = computed({
  get: () => props.modelValue ?? false,
  set: (v) => emit('update:modelValue', v),
})

// 当前用户的确认状态：由后端 GET 接口读取，组件内缓存
const acknowledged = ref(false)
// 本次登录会话是否已经查过接口，避免多个 Input-field 实例触发重复请求
const ackChecked = ref(false)
// 防止用户在 POST 接口未返回前重复点击确认按钮
const submitting = ref(false)

const confirmButtonText = computed(() =>
  acknowledged.value
    ? t('confidentialityAck.close')
    : t('confidentialityAck.confirm'),
)

watch(
  () => authStore.isLoggedIn,
  async (loggedIn) => {
    if (!loggedIn) {
      visible.value = false
      acknowledged.value = false
      ackChecked.value = false
      return
    }
    if (ackChecked.value) return
    try {
      const result = await getConfidentialityAck()
      acknowledged.value = result.acknowledged
      ackChecked.value = true
      if (!result.acknowledged) visible.value = true
    } catch (e) {
      // 查询失败时按未确认处理：让用户主动点击触发 POST 重试
      acknowledged.value = false
      ackChecked.value = true
      visible.value = true
    }
  },
  { immediate: true },
)

const onConfirm = async () => {
  // 已确认态：仅关闭弹窗，不调任何接口
  if (acknowledged.value) {
    visible.value = false
    return
  }
  if (submitting.value) return
  submitting.value = true
  try {
    const result = await acknowledgeConfidentiality()
    acknowledged.value = result.acknowledged
    visible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('confidentialityAck.ackFailed'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <Teleport to="body">
    <t-dialog v-model:visible="visible" placement="center" :width="500" :footer="null" :close-on-overlay-click="false"
      :close-on-escape-keydown="false" :close-btn="false" destroy-on-close class="confidentiality-ack-dialog"
      style="padding: 25px 34px;">
      <template #header>
        <div class="confidentiality-ack-header">
          {{ t('confidentialityAck.title') }}
        </div>
      </template>
      <div class="confidentiality-ack-body">
        {{ t('confidentialityAck.body') }}
      </div>
      <div class="confidentiality-ack-footer">
        <t-button theme="primary" size="large" block :loading="submitting" @click="onConfirm">
          {{ confirmButtonText }}
        </t-button>
      </div>
    </t-dialog>
  </Teleport>
</template>

<style scoped lang="less">
.confidentiality-ack-header {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-brand-color);
  padding-bottom: 20px;
  border-bottom: 1px solid #dcdcdc;
  width: 100%;
  margin-bottom: 20px;
}

.confidentiality-ack-body {
  font-size: 14px;
  line-height: 1.7;
  color: var(--td-text-color-secondary);
  white-space: pre-wrap;
}

.confidentiality-ack-footer {
  margin-top: 30px;

  button {
    padding: 12px;
    border-radius: 8px;
    font-size: 14px;
  }
}
</style>

<style lang="less">
.confidentiality-ack-dialog {
  .t-dialog {
    padding: 25px 34px !important;

  }
}
</style>