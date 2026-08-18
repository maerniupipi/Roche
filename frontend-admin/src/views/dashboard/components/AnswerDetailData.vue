<template>
  <div class="answer-detail">
    <div class="answer-detail__header">
      <div class="answer-detail__title">{{ title }}</div>
      <div class="answer-detail__legend">
        <span class="answer-detail__legend-item">
          <i class="answer-detail__dot" :style="{ background: '#0B41CD' }" />
          {{ leftLegend }}
        </span>
        <span class="answer-detail__legend-item">
          <i class="answer-detail__dot" :style="{ background: '#3D5BFF' }" />
          {{ rightLegend }}
        </span>
      </div>
    </div>
    <div class="answer-detail__chart">
      <v-chart
        class="answer-detail__echart"
        :option="option"
        autoresize
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TrendSeries } from '@/mock/dashboard'

const props = defineProps<{
  title: string
  leftLegend: string
  rightLegend: string
  leftSeries: TrendSeries
  rightSeries: TrendSeries
}>()

const option = computed(() => {
  const dates = props.leftSeries.points.map(p => p.date)
  const leftData = props.leftSeries.points.map(p => p.value)
  const rightData = props.rightSeries.points.map(p => p.value)

  return {
    grid: {
      left: 0,
      right: 0,
      top: 16,
      bottom: 24,
      containLabel: true,
    },
    legend: { show: false },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
    },
    xAxis: [
      {
        type: 'category',
        data: dates,
        axisLine: { lineStyle: { color: '#E0E2EA' } },
        axisLabel: {
          color: '#586380',
          fontSize: 10,
          fontFamily: 'Noto Sans SC, sans-serif',
        },
        axisTick: { show: false },
      },
    ],
    yAxis: [
      {
        type: 'value',
        position: 'left',
        minInterval: 1000,
        splitLine: { lineStyle: { color: '#F0F1F5', type: 'dashed' } },
        axisLabel: {
          color: '#586380',
          fontSize: 10,
        },
        axisLine: { show: false },
        axisTick: { show: false },
      },
      {
        type: 'value',
        position: 'right',
        min: 0,
        max: 1,
        interval: 0.25,
        axisLabel: {
          color: '#586380',
          fontSize: 10,
          formatter: (v: number) => `${Math.round(v * 100)}%`,
        },
        splitLine: { show: false },
        axisLine: { show: false },
        axisTick: { show: false },
      },
    ],
    series: [
      {
        name: props.leftLegend,
        type: 'bar',
        yAxisIndex: 0,
        data: leftData,
        barWidth: 22,
        itemStyle: { color: '#0B41CD', borderRadius: [2, 2, 0, 0] },
        emphasis: { focus: 'series' },
      },
      {
        name: props.rightLegend,
        type: 'bar',
        yAxisIndex: 1,
        data: rightData,
        barWidth: 22,
        itemStyle: { color: '#3D5BFF', borderRadius: [2, 2, 0, 0] },
        emphasis: { focus: 'series' },
      },
    ],
  }
})
</script>

<style scoped lang="less">
.answer-detail {
  width: 100%;

  &__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  &__title {
    font-family: 'Noto Sans SC', sans-serif;
    font-size: 14px;
    font-weight: 600;
    color: #1f2329;
  }

  &__legend {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: #586380;
  }

  &__legend-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  &__dot {
    width: 8px;
    height: 8px;
    border-radius: 2px;
    display: inline-block;
  }

  &__chart {
    height: 280px;
    width: 100%;
  }

  &__echart {
    width: 100%;
    height: 100%;
  }
}

@media (max-width: 767px) {
  .answer-detail__chart {
    height: 220px;
  }
  .answer-detail__legend {
    gap: 8px;
    font-size: 11px;
  }
}
</style>