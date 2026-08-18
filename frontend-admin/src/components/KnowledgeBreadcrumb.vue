<script lang="ts">
// 单段面包屑描述。`label` 缺省时由 `#seg-${key}` 插槽接管，可用于
// FAQ 那种"中间段包 KBSwitcherDropdown"的富内容。
export type KBreadcrumbSegment = {
  /** 唯一 key；同时作为派生插槽名 `#seg-${key}` */
  key: string
  /** 文本内容；缺省时改用插槽渲染 */
  label?: string
  /** 当前页 / 无对应路由 —— 不可点击 */
  disabled?: boolean
  /** 加载中：渲染 <t-skeleton 120×20> 占位，覆盖 label */
  loading?: boolean
}
</script>

<script setup lang="ts">
// 知识库域通用面包屑：
//   - segments: 从根到当前的有序段
//   - 点击某段 → `navigate` 事件，由调用方决定路由（保持组件纯展示）
//   - 富内容段（FAQ 中间段包 KBSwitcherDropdown 等）可用 `#seg-${key}` 插槽覆盖
//
// 使用方：KnowledgeBase.vue（3 段）、KnowledgeBaseList.vue（1~2 段）；
// FAQEntryManager.vue（4 段）后续可接入。
withDefaults(
  defineProps<{
    segments: KBreadcrumbSegment[]
    /** t-icon 名称，作为分隔符 */
    separator?: string
  }>(),
  { separator: 'chevron-right' },
)

const emit = defineEmits<{
  (e: 'navigate', segment: KBreadcrumbSegment): void
}>()

const onClick = (seg: KBreadcrumbSegment) => {
  if (seg.disabled) return
  emit('navigate', seg)
}
</script>

<template>
  <h2 class="kb-breadcrumb">
    <template v-for="(seg, i) in segments" :key="seg.key">
      <t-icon v-if="i > 0" :name="separator" class="kb-breadcrumb__separator" />
      <slot :name="`seg-${seg.key}`" :segment="seg">
        <button type="button" class="kb-breadcrumb__link" :class="{ 'is-current': seg.disabled }"
          :disabled="seg.disabled || i == 0" @click="onClick(seg)">
          <t-skeleton v-if="seg.loading" animation="gradient" :row-col="[{ width: '120px', height: '20px' }]" />
          <template v-else>{{ seg.label }}</template>
        </button>
      </slot>
    </template>
  </h2>
</template>

<style scoped lang="less">
.kb-breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.kb-breadcrumb__link {
  border: none;
  background: transparent;
  padding: 4px 8px;
  margin: -4px -8px;
  font: inherit;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  font-size: 16px;
  gap: 4px;
  border-radius: 6px;
  transition: all 0.12s ease;

  &:hover:not(:disabled) {
    color: var(--td-brand-color-4);
  }

  &:disabled,
  &.is-current {
    cursor: auto;
  }

  &.is-current {
    color: var(--td-text-color-primary);
    font-weight: 600;
  }
}

.kb-breadcrumb__separator {
  font-size: 14px;
  color: var(--td-text-color-placeholder);
}
</style>