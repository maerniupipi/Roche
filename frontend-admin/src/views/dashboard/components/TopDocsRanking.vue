<template>
  <div class="top-docs">
    <div class="top-docs__header">
      <div class="top-docs__title">{{ title }}</div>
      <button type="button" class="top-docs__more">
        {{ $t('dashboard.topDocs.more') }}
      </button>
    </div>
    <ul class="top-docs__list">
      <li
        v-for="item in items"
        :key="item.rank"
        class="top-docs__item"
      >
        <div class="top-docs__rank" :class="{ 'top-docs__rank--first': item.isFirst }">
          {{ item.rank }}
        </div>
        <div class="top-docs__name">{{ item.name }}</div>
        <div class="top-docs__bar-wrap">
          <div
            class="top-docs__bar"
            :style="{
              width: barPercent(item.barValue) + '%',
              background: item.isFirst ? '#ED4A0D' : '#3067EB',
            }"
          />
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import type { TopEntry } from '@/mock/dashboard'

const props = defineProps<{
  title: string
  items: TopEntry[]
}>()

const max = Math.max(...props.items.map((i) => i.barValue), 1)

const barPercent = (v: number) => Math.round((v / max) * 100)
</script>

<style scoped lang="less">
.top-docs {
  background: #ffffff;
  border-radius: 4px;
  padding: 16px;
  width: 100%;
  min-height: 480px;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;

  @media (max-width: 767px) {
    min-height: 400px;
  }

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
    overflow-y: auto;
  }

  &__item {
    display: grid;
    grid-template-columns: 22px 1fr 100px;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
  }

  &__rank {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: #f0f1f5;
    color: #586380;
    font-size: 11px;
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: center;

    &--first {
      background: #fff1eb;
      color: #ed4a0d;
    }
  }

  &__name {
    color: #1f2329;
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__bar-wrap {
    height: 6px;
    background: #f0f1f5;
    border-radius: 3px;
    overflow: hidden;
  }

  &__bar {
    height: 100%;
    border-radius: 3px;
    transition: width 0.3s ease;
  }
}
</style>
