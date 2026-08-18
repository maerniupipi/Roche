<template>
  <div class="domain-pie">
    <v-chart
      class="domain-pie__canvas"
      :option="option"
      autoresize
    />
    <ul class="domain-pie__legend">
      <li
        v-for="(item, idx) in data"
        :key="idx"
        class="domain-pie__legend-item"
      >
        <span
          class="domain-pie__dot"
          :style="{ background: item.color }"
        />
        <span class="domain-pie__label">{{ item.name }}</span>
        <span class="domain-pie__value">{{ item.value }}%</span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PieChart } from 'echarts/charts'
import VChart from 'vue-echarts'
import type { PieDatum } from '@/mock/dashboard'

use([CanvasRenderer, PieChart])

const props = defineProps<{
  data: PieDatum[]
}>()

const option = computed(() => ({
  tooltip: {
    trigger: 'item',
    formatter: (params: { name: string; value: number }) =>
      `${params.name}: ${params.value}%`,
  },
  legend: { show: false },
  series: [
    {
      type: 'pie',
      radius: '78%',
      avoidLabelOverlap: false,
      itemStyle: {
        borderColor: '#ffffff',
        borderWidth: 2,
      },
      label: { show: false },
      labelLine: { show: false },
      data: props.data.map((d) => ({
        name: d.name,
        value: d.value,
        itemStyle: { color: d.color },
      })),
    },
  ],
}))
</script>

<style scoped lang="less">
.domain-pie {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  align-items: center;
  width: 100%;
  height: 180px;

  &__canvas {
    width: 100%;
    height: 100%;
    min-height: 160px;
  }

  &__legend {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  &__legend-item {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: #1f2329;
  }

  &__dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  &__label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__value {
    color: #586380;
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }
}
</style>