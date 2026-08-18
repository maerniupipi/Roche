<template>
  <div class="summary-card" :style="{ background: background }">
    <div class="summary-card__title">{{ title }}</div>
    <div class="summary-card__big" v-if="bigValue">
      <span class="summary-card__num">{{ bigValue }}</span>
      <span v-if="unit" class="summary-card__unit">{{ unit }}</span>
    </div>
    <div class="summary-card__rows">
      <div
        v-for="(row, idx) in rows"
        :key="idx"
        class="summary-card__row"
      >
        <span class="summary-card__row-label">{{ row.label }}</span>
        <span class="summary-card__row-value">{{ row.value }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  bigValue?: string | number
  unit?: string
  rows: Array<{ label: string; value: string | number }>
  background?: string
}>()
</script>

<style scoped lang="less">
.summary-card {
  width: 100%;
  min-height: 180px;
  border-radius: 4px;
  padding: 20px;
  background: linear-gradient(135deg, #0b41cd 0%, #3067eb 100%);
  color: #ffffff;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;

  @media (max-width: 767px) {
    min-height: 160px;
    padding: 16px;
  }

  &__title {
    font-family: 'Noto Sans SC', sans-serif;
    font-weight: 600;
    font-size: 14px;
    color: rgba(255, 255, 255, 0.9);
    margin-bottom: 12px;
  }

  &__big {
    display: flex;
    align-items: baseline;
    gap: 6px;
    margin-bottom: 16px;
  }

  &__num {
    font-family: 'Fjalla One', sans-serif;
    font-size: 36px;
    line-height: 1;
    color: #ffffff;

    @media (max-width: 767px) {
      font-size: 28px;
    }
  }

  &__unit {
    font-size: 14px;
    color: rgba(255, 255, 255, 0.85);
  }

  &__rows {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 24px;
    margin-top: auto;
  }

  &__row {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
  }

  &__row-label {
    color: rgba(255, 255, 255, 0.85);
  }

  &__row-value {
    color: #ffffff;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
}
</style>