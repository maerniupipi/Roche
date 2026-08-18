<template>
  <div class="basic-data">
    <div
      v-for="(m, idx) in items"
      :key="idx"
      class="basic-data__card"
    >
      <div class="basic-data__title">{{ m.label }}</div>
      <div class="basic-data__main">
        <span class="basic-data__main-num">{{ m.primary.value }}</span>
      </div>
      <div class="basic-data__sub">
        <span class="basic-data__sub-label">{{ m.primary.label }}</span>
        <span class="basic-data__sub-value">{{ m.secondary.value }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { BasicMetricItem } from '@/mock/dashboard'

defineProps<{
  items: BasicMetricItem[]
}>()
</script>

<style scoped lang="less">
.basic-data {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 12px;

  @media (max-width: 1199px) {
    grid-template-columns: repeat(3, 1fr);
  }
  @media (max-width: 767px) {
    grid-template-columns: repeat(2, 1fr);
  }

  &__card {
    width: 100%;
    min-height: 180px;
    background: #ffffff;
    border-radius: 4px;
    padding: 16px;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  &__title {
    font-family: 'Noto Sans SC', sans-serif;
    font-size: 13px;
    color: #1f2329;
    font-weight: 500;
  }

  &__main {
    display: flex;
    align-items: baseline;
    gap: 4px;
    margin-top: 6px;
  }

  &__main-num {
    font-family: 'Fjalla One', sans-serif;
    font-size: 28px;
    color: #1f2329;
    line-height: 1;
    font-variant-numeric: tabular-nums;
  }

  &__main-unit {
    font-size: 12px;
    color: #586380;
  }

  &__sub {
    margin-top: auto;
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-top: 12px;
    border-top: 1px solid #f0f1f5;
  }

  &__sub-label {
    font-size: 12px;
    color: #586380;
  }

  &__sub-value {
    font-size: 12px;
    color: #1f2329;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }
}
</style>