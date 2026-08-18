<template>
  <t-select :value="modelValue" :placeholder="placeholder" :options="options" borderless class="dept-selector"
    @change="$emit('update:modelValue', $event)" />
</template>

<script setup lang="ts">
defineProps<{
  modelValue: string | number
  placeholder: string
  options: Array<{ label: string; value: string | number }>
}>()

defineEmits<{
  'update:modelValue': [value: string | number]
}>()
</script>

<style scoped lang="less">
.dept-selector {
  min-width: 140px;
  border-radius: 4px;
  background: #ffffff;
}
</style>