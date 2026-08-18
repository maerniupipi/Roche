<template>
  <div class="feedback-bar-h">
    <div class="feedback-bar-h__header">
      <div class="feedback-bar-h__title">{{ title }}</div>
      <button type="button" class="feedback-bar-h__more">
        {{ $t('dashboard.feedback.more') }}
      </button>
    </div>
    <div class="feedback-bar-h__list">
      <div
        v-for="(item, idx) in items"
        :key="idx"
        class="feedback-bar-h__row"
      >
        <div class="feedback-bar-h__label">{{ item.label }}</div>
        <div class="feedback-bar-h__track">
          <div
            class="feedback-bar-h__fill"
            :style="{
              width: barWidth(item.count) + '%',
              background: item.highlight,
            }"
          />
        </div>
        <div class="feedback-bar-h__count">{{ item.count }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { FeedbackReason } from '@/mock/dashboard'

defineProps<{
  title: string
  items: FeedbackReason[]
}>()

const barWidth = (count: number) => {
  const max = 10
  return Math.min(100, Math.round((count / max) * 100))
}
</script>

<style scoped lang="less">
.feedback-bar-h {
  background: #ffffff;
  border-radius: 4px;
  padding: 16px;
  width: 365px;
  height: 252px;
  display: flex;
  flex-direction: column;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 14px;
  }

  &__title {
    font-family: 'Noto Sans SC', sans-serif;
    font-weight: 600;
    font-size: 14px;
    color: #1f2329;
  }

  &__more {
    background: transparent;
    border: none;
    color: #586380;
    font-size: 12px;
    cursor: pointer;
    padding: 0;
  }

  &__list {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  &__row {
    display: grid;
    grid-template-columns: 72px 1fr 28px;
    align-items: center;
    gap: 8px;
  }

  &__label {
    color: #1f2329;
    font-size: 12px;
  }

  &__track {
    height: 8px;
    background: #f0f1f5;
    border-radius: 4px;
    overflow: hidden;
  }

  &__fill {
    height: 100%;
    border-radius: 4px;
    transition: width 0.3s ease;
  }

  &__count {
    color: #1f2329;
    font-size: 12px;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
}
</style>
