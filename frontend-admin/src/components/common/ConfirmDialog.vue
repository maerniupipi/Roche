<!--
  通用二次确认弹窗。

  适用于：封禁 / 取消封禁、设为 / 取消运维员、批量操作等需要二次确认的场景。
  加载状态由父组件管理（本组件不内置 loading），父组件在收到 confirm 后自行 disable 按钮 / 调接口。
-->
<template>
  <t-dialog :visible="visible" :header="title" :confirm-btn="confirmBtnConfig" :cancel-btn="cancelBtnText" :width="width"
    @update:visible="emit('update:visible', $event)" @confirm="handleConfirm" @close="handleCancel">
    <p class="confirm-content">{{ content }}</p>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

type Theme = 'default' | 'warning' | 'danger'

const props = withDefaults(
  defineProps<{
    visible: boolean
    title: string
    content: string
    confirmText?: string
    cancelText?: string
    theme?: Theme
    width?: string
  }>(),
  {
    confirmText: '',
    cancelText: '',
    theme: 'default',
    width: '420px',
  },
)

const { t } = useI18n()

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'confirm'): void
  (event: 'cancel'): void
}>()

// 父组件显式传 confirmText / cancelText 时优先使用；未传时按当前语言回落（common.confirm / common.cancel）。
const confirmBtnConfig = computed(() => ({
  content: props.confirmText || t('common.confirm'),
  theme: props.theme === 'default' ? 'primary' : props.theme,
}))

const cancelBtnText = computed(() => props.cancelText || t('common.cancel'))

function handleConfirm(): void {
  emit('confirm')
}

function handleCancel(): void {
  emit('cancel')
}
</script>

<style scoped lang="less">
.confirm-content {
  margin: 0;
  color: var(--td-text-color-primary);
  line-height: 1.6;
}
</style>
