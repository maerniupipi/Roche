<template>
  <div class="top-users">
    <div class="top-users__header">
      <div class="top-users__title">{{ title }}</div>
      <button type="button" class="top-users__more">
        {{ $t('dashboard.topUsers.more') }}
      </button>
    </div>
    <ul class="top-users__list">
      <li
        v-for="item in items"
        :key="item.rank"
        class="top-users__item"
      >
        <div class="top-users__avatar" :class="avatarClass(item.rank)">
          {{ initial(item.name) }}
        </div>
        <div class="top-users__name">{{ item.name }}</div>
        <div class="top-users__bar-wrap">
          <div
            class="top-users__bar"
            :style="{
              width: barPercent(item.barValue) + '%',
              background: '#3067EB',
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

const initial = (name: string) => name.charAt(0).toUpperCase()

const avatarClass = (rank: number) => {
  if (rank === 1) return 'top-users__avatar--gold'
  if (rank === 2) return 'top-users__avatar--silver'
  if (rank === 3) return 'top-users__avatar--bronze'
  return ''
}
</script>

<style scoped lang="less">
.top-users {
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
    grid-template-columns: 28px 1fr 100px;
    align-items: center;
    gap: 10px;
    padding: 8px 0;
  }

  &__avatar {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: #dde5fa;
    color: #0b41cd;
    font-size: 11px;
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;

    &--gold {
      background: #ffe7c2;
      color: #b36200;
    }

    &--silver {
      background: #e7eaef;
      color: #586380;
    }

    &--bronze {
      background: #ffe1d6;
      color: #cc0033;
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
