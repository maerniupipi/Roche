<!-- 公共筛选容器。
     样式借鉴 KnowledgeBase 的 .doc-filter-bar：小屏 grid 三区，宽屏 (>= 1280px)
     横向 flex。
     使用：
       - 搜索框：内置（带 search icon / clearable / placeholder），通过 v-model:search 双向绑定
       - 筛选字段：放在默认 slot，父组件完全控制类型（select / input / 自定义）
       - 操作按钮：放在 #trailing slot（查询 / 重置 / 批量） -->
<template>
  <div v-if="!empty" class="page-search-filters">
    <t-input
      v-if="searchable"
      :model-value="search"
      :placeholder="searchPlaceholder || t('common.search')"
      clearable
      class="page-search-filters__search"
      @update:model-value="onSearchInput"
      @enter="onSearchEnter"
      @clear="onSearchClear"
    >
      <template #prefix-icon>
        <t-icon name="search" size="16px" />
      </template>
    </t-input>

    <div class="page-search-filters__filters">
      <slot />
    </div>

    <div class="page-search-filters__trailing">
      <slot name="trailing" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    /** 当前搜索关键词，配合 v-model:search 使用 */
    search?: string
    /** 搜索框 placeholder */
    searchPlaceholder?: string
    /** 是否显示搜索框 */
    searchable?: boolean
    /** 整块筛选区是否显示 */
    empty?: boolean
  }>(),
  {
    search: '',
    searchPlaceholder: '',
    searchable: true,
    empty: false,
  },
)

const emit = defineEmits<{
  (e: 'update:search', value: string): void
}>()

function onSearchInput(val: string | number): void {
  emit('update:search', String(val ?? ''))
}

function onSearchEnter(): void {
  emit('update:search', props.search)
}

function onSearchClear(): void {
  emit('update:search', '')
}
</script>

<style lang="less" scoped>
.page-search-filters {
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-areas:
    'search trailing'
    'filters filters';
  gap: 8px 12px;
  align-items: center;
  // padding: 12px 24px;
  background: var(--td-bg-color-container, #fff);
  border-radius: 6px;
  // border: 1px solid var(--td-component-stroke, #e7e7e7);
}

.page-search-filters__search {
  grid-area: search;
  min-width: 0;
  width: 100%;
}

.page-search-filters__filters {
  grid-area: filters;
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  overflow-x: auto;
  flex-wrap: nowrap;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: thin;
  scrollbar-color: rgba(0, 0, 0, 0.15) transparent;

  &::-webkit-scrollbar {
    height: 4px;
  }

  &::-webkit-scrollbar-thumb {
    background-color: rgba(0, 0, 0, 0.15);
    border-radius: 2px;
  }
}

.page-search-filters__trailing {
  grid-area: trailing;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

@media (min-width: 1280px) {
  .page-search-filters {
    display: flex;
    flex-direction: row;
    flex-wrap: nowrap;
    gap: 12px;
  }

  .page-search-filters__filters {
    flex: 0 1 auto;
    overflow-x: visible;
  }

  .page-search-filters__search {
    flex: 1 1 220px;
    min-width: 220px;
  }
}
</style>