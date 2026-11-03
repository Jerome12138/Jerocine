<script setup lang="ts">
import { computed } from 'vue'
import BaseTag from '@/components/base/BaseTag.vue'

/** BaseTag 支持的 variant 集合（此处收窄为状态标签常用子集） */
export type BaseTagVariant = 'default' | 'brand' | 'purple' | 'success' | 'warning' | 'danger' | 'info'

export interface StatusMapEntry {
  label: string
  variant: BaseTagVariant
}

/** 键为 String(value) 后的状态词汇表 */
export type StatusMap = Record<string, StatusMapEntry>

interface Props {
  /** 任意原始值; 比较时 String(value) 后查 map */
  value: string | number | boolean | null | undefined
  map: StatusMap
  /** 未命中回退 */
  fallback?: StatusMapEntry
  /** 透传 BaseTag */
  size?: 'xs' | 'sm' | 'md'
}

const props = withDefaults(defineProps<Props>(), {
  fallback: () => ({ label: '—', variant: 'default' }),
  size: 'sm'
})

const entry = computed<StatusMapEntry>(() => {
  if (props.value === null || props.value === undefined) return props.fallback
  return props.map[String(props.value)] ?? props.fallback
})
</script>

<template>
  <BaseTag :variant="entry.variant" :size="props.size">{{ entry.label }}</BaseTag>
</template>
