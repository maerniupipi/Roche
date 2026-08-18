<template>
  <div class="kb-stats">
    <div v-for="(kpi, idx) in items" :key="idx" class="kb-stats__card" :style="{ background: kpi.bgColor }">
      <div class="kb-stats__head">
        <div class="kb-stats__icon" :style="{ background: kpi.iconBg, color: kpi.iconColor }">
          {{ kpi.iconText }}
        </div>
        <div class="kb-stats__title">{{ kpi.title }}</div>
      </div>
      <div class="kb-stats__value">
        <span class="kb-stats__num">{{ kpi.value }}</span>
        <span class="kb-stats__unit">{{ kpi.unit }}</span>
      </div>
      <div v-if="kpi.subtitle" class="kb-stats__sub">{{ kpi.subtitle }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { KbKpiItem } from '@/mock/dashboard'

defineProps<{
  items: KbKpiItem[]
}>()
</script>

<style scoped lang="less">
.kb-stats {
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
    // width: 176px;
    width: 100%;
    height: 78px;
    border-radius: 4px;
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    box-sizing: border-box;
  }

  &__head {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  &__icon {
    width: 18px;
    height: 18px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 10px;
    font-weight: 600;
    flex-shrink: 0;
  }

  &__title {
    font-family: 'Noto Sans SC', sans-serif;
    font-size: 12px;
    color: #1f2329;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__value {
    display: flex;
    align-items: baseline;
    gap: 4px;
  }

  &__num {
    font-family: 'Fjalla One', sans-serif;
    font-size: 22px;
    color: #1f2329;
    line-height: 1;
  }

  &__unit {
    font-size: 11px;
    color: #586380;
  }

  &__sub {
    font-size: 11px;
    color: #586380;
    margin-top: 2px;
  }
}
</style>