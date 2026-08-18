<template>
  <div class="fallback-questions">
    <div class="fallback-questions__header">
      <div class="fallback-questions__title">{{ title }}</div>
      <button type="button" class="fallback-questions__more">
        {{ $t('dashboard.fallback.more') }}
      </button>
    </div>
    <ul class="fallback-questions__list">
      <li
        v-for="item in items"
        :key="item.id"
        class="fallback-questions__item"
      >
        <div class="fallback-questions__q">
          <span class="fallback-questions__no">{{ item.id.replace('fbq-', '') }}.</span>
          <span class="fallback-questions__text">{{ item.question }}</span>
        </div>
        <div v-if="item.department" class="fallback-questions__dept">
          {{ item.department }}
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import type { FallbackQuestion } from '@/mock/dashboard'

defineProps<{
  title: string
  items: FallbackQuestion[]
}>()
</script>

<style scoped lang="less">
.fallback-questions {
  background: #ffffff;
  border-radius: 4px;
  padding: 16px;
  width: 365px;
  height: 293px;
  display: flex;
  flex-direction: column;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
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
    list-style: none;
    margin: 0;
    padding: 0;
    flex: 1;
    overflow: hidden;
  }

  &__item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 0;
    border-bottom: 1px solid #f0f1f5;

    &:last-child {
      border-bottom: none;
    }
  }

  &__q {
    flex: 1;
    display: flex;
    gap: 6px;
    min-width: 0;
  }

  &__no {
    color: #1f2329;
    font-weight: 500;
    font-size: 12px;
    flex-shrink: 0;
  }

  &__text {
    color: #1f2329;
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__dept {
    color: #586380;
    font-size: 12px;
    flex-shrink: 0;
    margin-left: 12px;
  }
}
</style>
