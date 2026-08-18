<template>
  <t-select :value="modelValue" :placeholder="placeholder" :options="options" borderless class="kb-selector"
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
.kb-selector {
  width: 160px;
  border-radius: 4px;
  background: #ffffff;
}
</style>