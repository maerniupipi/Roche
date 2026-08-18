<template>
  <div class="date-range-picker-wrap">
    <t-select
      v-model="preset"
      :options="presetOptions"
      size="small"
      class="preset-select"
    />
    <t-date-range-picker
      :value="internalRange"
      :placeholder="placeholder"
      borderless
      class="date-range-picker"
      @change="handleDateChange"
    />
  </div>
</template>

<script setup lang="ts">
// 日期区间筛选器：下拉选择预设（自定义 / 近 7 天 / 近 30 天）+ 实际日期范围。
// 双向联动：
//   - 下拉变预设 → 日期范围同步成今天往前的 N 天
//   - 日期范围被改成非 7/30 天的区间 → 下拉变「自定义」
//   - 日期范围被改回恰好 7 天或 30 天 → 下拉跟随到对应预设
//   - 选「自定义」时 → 日期范围不变，仅作为 UI 状态

import { computed, ref, watch, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'

type DateRange = [string, string]
type Preset = 'custom' | '7d' | '30d'

const props = withDefaults(defineProps<{
  modelValue?: DateRange
  placeholder?: string
}>(), {
  modelValue: undefined,
  placeholder: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: DateRange]
}>()

const { t } = useI18n()

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function formatDate(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// 含今天往前共 N 天
function computeRange(preset: Exclude<Preset, 'custom'>): DateRange {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const start = new Date(today)
  start.setDate(today.getDate() - (preset === '7d' ? 6 : 29))
  return [formatDate(start), formatDate(today)]
}

function isValidRange(range: DateRange | undefined): range is DateRange {
  return !!range && !!range[0] && !!range[1]
}

// 把任意日期区间反推成预设：必须是 end=今天，且跨度恰好为 7 或 30 才算匹配。
function matchPreset(range: DateRange): Preset {
  if (!isValidRange(range)) return 'custom'
  const startDate = new Date(range[0])
  const endDate = new Date(range[1])
  if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) return 'custom'
  // YYYY-MM-DD 字符串被解析为 UTC 00:00，需归一到本地 00:00 才能跟 today 比对。
  startDate.setHours(0, 0, 0, 0)
  endDate.setHours(0, 0, 0, 0)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  if (endDate.getTime() !== today.getTime()) return 'custom'
  const span = Math.round((endDate.getTime() - startDate.getTime()) / 86400000) + 1
  if (span === 7) return '7d'
  if (span === 30) return '30d'
  return 'custom'
}

const internalRange = ref<DateRange>(
  isValidRange(props.modelValue) ? [...props.modelValue] : computeRange('30d'),
)

const presetOptions = computed(() => [
  { label: t('time.custom'), value: 'custom' },
  { label: t('time.last7Days'), value: '7d' },
  { label: t('time.last30Days'), value: '30d' },
])

const preset = ref<Preset>(matchPreset(internalRange.value))

// 防止「下拉 → 日期 → 下拉」形成循环触发。
let skipNextRangeSync = false

watch(preset, (newPreset) => {
  if (newPreset === 'custom') return
  const nextRange = computeRange(newPreset)
  if (
    nextRange[0] === internalRange.value[0] &&
    nextRange[1] === internalRange.value[1]
  ) {
    return
  }
  skipNextRangeSync = true
  internalRange.value = nextRange
  emit('update:modelValue', nextRange)
  nextTick(() => { skipNextRangeSync = false })
})

watch(internalRange, (newRange) => {
  if (skipNextRangeSync) {
    skipNextRangeSync = false
    return
  }
  emit('update:modelValue', newRange)
  const matched = matchPreset(newRange)
  if (matched !== preset.value) preset.value = matched
})

// 父级 modelValue 变化时（外部重置场景）同步到内部。
watch(() => props.modelValue, (newVal) => {
  if (!isValidRange(newVal)) return
  if (newVal[0] === internalRange.value[0] && newVal[1] === internalRange.value[1]) return
  internalRange.value = [...newVal]
})

function handleDateChange(value: DateRange) {
  internalRange.value = value
}

onMounted(() => {
  // 首次挂载时若父级没有传 v-model 值，主动 emit 一次默认值让父级拿到初始范围。
  if (!isValidRange(props.modelValue)) {
    emit('update:modelValue', internalRange.value)
  }
})
</script>

<style scoped lang="less">
.date-range-picker-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.preset-select {
  width: 110px;
}
.date-range-picker {
  min-width: 240px;
  border-radius: 4px;
  background: #ffffff;
}
</style>