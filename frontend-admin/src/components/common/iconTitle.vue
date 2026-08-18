<template>
  <div class="title-wrap">
    <img src="@/assets/img/titleIcon.svg" />
    <h1 class="title">{{ title }}</h1>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
defineProps<{
  title: string
}>()

</script>

<style lang="less" scoped>
.title-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
}

.title {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  color: #191919;
  line-height: 1.5;
}
</style>
