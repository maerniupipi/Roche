<!-- 公共分页表格。
     封装 <t-table> + <t-pagination> + 行选择 + 固定表头。
     行 / 列 cell 渲染完全交给父组件 #<key> slot（key 即 column.key；未传 slot 时回退到 row[key] 纯文本）。
     这是 TDesign t-table 的 slot 命名约定：内部用 slots[col.colKey](params) 查找，
     所以父组件传 #cell-xxx 不会被识别，必须传 #xxx。
     使用：
       <PageTable
         :data="rows"
         :columns="cols"
         :selected-ids="ids"
         :pagination="pg"
         @update:selected-ids="..."
         @page-change="...">
         <template #status="{ row }">...</template>
         <template #action="{ row }">...</template>
       </PageTable> -->
<template>
  <div class="page-table">
    <!-- 表格区（flex: 1 撑满父容器剩余高度），数据多时 body 内部滚动 -->
    <div class="page-table__body">
      <t-table :data="data" :columns="renderColumns" :loading="loading" :fixed-header="true" :height="tableHeight"
        :pagination="null" :row-key="rowKey" :row-class-name="rowClassName" :stripe="stripe" :hover="hover"
        :empty="emptyText || t('common.noData')" :selected-row-keys="selectedIds ?? []" @select-change="onSelectChange">
        <!-- TDesign 的 cell slot 名约定：父组件用 #<colKey>，内部 slots[col.colKey](params) 查找 -->
        <template v-for="col in renderableColumns" :key="String(col.key)" #[String(col.key)]="{ row }">
          <slot :name="String(col.key)" :row="row">
            <!-- 列配置了 tags：命中按 theme 是否存在决定 t-tag / 纯文本 -->
            <t-tag v-if="matchedTag(col, row)?.theme" size="small" :theme="matchedTag(col, row)!.theme">
              {{ matchedTag(col, row)!.label }}
            </t-tag>
            <span v-else-if="matchedTag(col, row)" class="page-table__cell">
              {{ matchedTag(col, row)!.label }}
            </span>
            <!-- 未配置 tags 或未命中：兜底显示 row[col.key] -->
            <span v-else class="page-table__cell">{{ row[col.key as keyof typeof row] || '—' }}</span>
          </slot>
        </template>
        <template #empty>
          <slot name="empty">{{ emptyText || t('common.noData') }}</slot>
        </template>
      </t-table>
    </div>

    <div v-if="pagination" class="page-table__pagination">
      <t-pagination :current="pagination.page" :page-size="pagination.pageSize" :total="pagination.total"
        :page-size-options="pageSizeOptions" :show-jumper="showJumper" @change="onPageChange" />
    </div>
  </div>
</template>

<script setup lang="ts" generic="T extends object">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

/** 单个 tag 映射配置；用于把枚举/布尔值展示成可读文案或带色 tag。 */
export interface PageTableColumnTag {
  /** 用来匹配 row[col.key]；string / number / boolean 都允许 */
  value: unknown
  /** 匹配后展示的文案（建议由父组件传 t('xxx') 已翻译好的字符串） */
  label: string
  /** TDesign <t-tag theme>；不传则按纯文本渲染（不包 t-tag） */
  theme?: 'success' | 'warning' | 'danger' | 'primary' | 'default'
}

export interface PageTableColumn<T = object> {
  /** 列 key；与行对象字段名对应（'selection' 时不渲染 cell，自动生成多选列） */
  key: keyof T | string
  /** 表头文本（不含 i18n 转换，由父组件传入） */
  title: string
  width?: number
  minWidth?: number
  fixed?: 'left' | 'right'
  align?: 'left' | 'center' | 'right'
  /** 内容超出列宽时省略显示；开启后默认联动 ellipsisTitle=true（hover 显示完整内容） */
  ellipsis?: boolean
  /** ellipsis 列 hover 时显示的 tooltip 内容。
   *  不传：ellipsis=true 时自动启用（显示单元格原文）；
   *  传 false：关闭 tooltip；
   *  传对象：作为 TDesign Tooltip props 透传。 */
  ellipsisTitle?: boolean | Record<string, unknown>
  /** 列配置 tags 时，按 row[col.key] 在数组中查找命中的项：
   *   - 命中且 item.theme 有值 → <t-tag :theme="..."> 渲染
   *   - 命中且 item.theme 缺省 → 纯文本 span 渲染
   *   - 未命中 / 未配置      → 兜底为纯文本 row[col.key] || '—'
   * 父组件同名 #<colKey> slot 仍然优先于此配置（slot 未传时才走这里）。 */
  tags?: PageTableColumnTag[]
}

export interface PageTablePagination {
  page: number
  pageSize: number
  total: number
}

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    data: T[]
    columns: PageTableColumn<T>[]
    loading?: boolean
    selectedIds?: string[]
    pagination?: PageTablePagination
    rowKey?: string
    /** 行 class 名；透传给 <t-table :row-class-name>。
     *  常用于：行 hover/选中态下展示行内操作（如 .row-more-btn）。 */
    rowClassName?: string | ((row: T, index: number) => string)
    tableMaxHeight?: number
    emptyText?: string
    pageSizeOptions?: number[]
    showJumper?: boolean
    stripe?: boolean
    hover?: boolean
  }>(),
  {
    loading: false,
    selectedIds: () => [] as string[],
    rowKey: 'id',
    rowClassName: undefined,
    pageSizeOptions: () => [10, 20, 50, 100],
    showJumper: true,
    stripe: false,
    hover: true,
  },
)

const emit = defineEmits<{
  (e: 'update:selectedIds', value: string[]): void
  (e: 'page-change', value: { page: number; pageSize: number }): void
}>()

/** 转成 <t-table> 期望的列结构（colKey / title / width / ...）。
     当 key === 'selection' 时，自动生成 type: 'multiple' 的多选列。 */
const renderColumns = computed(() =>
  props.columns.map((c) => {
    if (String(c.key) === 'selection') {
      return {
        colKey: 'row-select',
        type: 'multiple' as const,
        width: c.width,
        fixed: c.fixed,
        checkProps: { value: false },
      }
    }
    return {
      colKey: String(c.key),
      title: c.title,
      width: c.width,
      minWidth: c.minWidth,
      fixed: c.fixed,
      align: c.align ?? 'left',
      ellipsis: c.ellipsis,
      // ellipsis 列默认开启 tooltip（hover 显示完整内容）；父组件可用 ellipsisTitle: false 关闭
      ...(c.ellipsisTitle !== undefined
        ? { ellipsisTitle: c.ellipsisTitle }
        : c.ellipsis
          ? { ellipsisTitle: true }
          : {}),
    }
  }),
)

/** 需要注册 cell slot 的列；selection 列由 t-table 自身渲染多选框 */
const renderableColumns = computed(() => props.columns.filter((c) => String(c.key) !== 'selection'))

/** 按 row[col.key] 在 col.tags 中查找命中的项；未配置或未命中返回 undefined。 */
function matchedTag<T>(col: PageTableColumn<T>, row: T): PageTableColumnTag | undefined {
  if (!col.tags?.length) return undefined
  const cellVal = (row as Record<string, unknown>)[col.key as string]
  return col.tags.find((item) => item.value === cellVal)
}

function onSelectChange(selectedRowKeys: Array<string | number>): void {
  emit('update:selectedIds', selectedRowKeys as string[])
}

function onPageChange(pageInfo: { current: number; previous: number; pageSize: number }): void {
  emit('page-change', { page: pageInfo.current, pageSize: pageInfo.pageSize })
}

/** 表格高度：父组件显式指定时用固定值（向后兼容），否则撑满父容器剩余空间。
 * 配合 fixed-header，让数据少时空数据区居中、数据多时 body 内部滚动。 */
const tableHeight = computed(() => props.tableMaxHeight ?? '100%')
</script>

<style lang="less" scoped>
.page-table {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--td-bg-color-container, #fff);
  border-radius: 6px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  overflow: hidden;
}

/* 表格容器：撑满父容器剩余高度。
 * min-height: 0 是 flex 子项允许被压缩到 0 的关键，让 t-table 的 height: 100% 真正生效。 */
.page-table__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
  position: relative;
}

/* TDesign v2 t-table 接受 height="100%" prop，但只把 height: 100% 应用到内部
 * .t-table__content 上，自身 root 仍按内容撑开 → 内部 100% 永远解析为"自适应"，
 * 滚动条不出现。这里强制给 .t-table root 一个有限高度，内部 100% 才能真正生效。 */
.page-table__body :deep(.t-table) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-table__body :deep(th) {
  background-color: #f4f4f4;
}


.page-table__pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--td-component-stroke, #e7e7e7);
  background: var(--td-bg-color-container, #fff);
}
</style>