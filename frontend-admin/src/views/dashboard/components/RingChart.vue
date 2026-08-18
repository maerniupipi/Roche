<template>
  <div class="ring-chart">
    <v-chart
      class="ring-chart__canvas"
      :option="option"
      autoresize
    />
    <div class="ring-chart__center">
      <div class="ring-chart__total">{{ data.total }}</div>
      <div class="ring-chart__label">{{ $t('dashboard.ring.total') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PieChart } from 'echarts/charts'
import VChart from 'vue-echarts'
import type { EffectiveAnswerRing } from '@/mock/dashboard'

use([CanvasRenderer, PieChart])

const props = defineProps<{
  data: EffectiveAnswerRing
}>()

const option = computed(() => ({
  tooltip: {
    trigger: 'item',
    formatter: (params: { name: string; value: number }) =>
      `${params.name}: ${params.value}%`,
  },
  series: [
    {
      type: 'pie',
      radius: ['64%', '84%'],
      avoidLabelOverlap: false,
      itemStyle: {
        borderColor: '#ffffff',
        borderWidth: 2,
      },
      label: { show: false },
      labelLine: { show: false },
      data: [
        {
          name: '有效回答',
          value: props.data.effectivePercent,
          itemStyle: { color: props.data.effectiveColor },
        },
        {
          name: '触发兜底话术',
          value: props.data.fallbackPercent,
          itemStyle: { color: props.data.fallbackColor },
        },
      ],
    },
  ],
}))
</script>

<style scoped lang="less">
.ring-chart {
  position: relative;
  width: 100%;
  height: 180px;

  &__canvas {
    width: 100%;
    height: 100%;
  }

  &__center {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    text-align: center;
    pointer-events: none;
  }

  &__total {
    font-family: 'Fjalla One', sans-serif;
    font-size: 24px;
    color: #1f2329;
    line-height: 1;
  }

  &__label {
    font-size: 11px;
    color: #586380;
    margin-top: 4px;
  }
}
</style>
