<script setup lang="ts">
import { computed } from 'vue'

type Variant =
  | 'default'
  | 'brand'
  | 'purple'
  | 'success'
  | 'warning'
  | 'danger'
  | 'info'
type Size = 'xs' | 'sm' | 'md'

interface Props {
  variant?: Variant
  size?: Size
  /** 实心 / 透明描边 */
  outlined?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'sm',
  outlined: false
})

const sizeClass = computed(() => {
  switch (props.size) {
    case 'xs':
      return 'h-5 px-[6px] text-[var(--jc-fs-xs)]'
    case 'md':
      return 'h-7 px-[10px] text-[var(--jc-fs-sm)]'
    case 'sm':
    default:
      return 'h-6 px-2 text-[var(--jc-fs-xs)]'
  }
})

const variantClass = computed(() => {
  if (props.outlined) {
    switch (props.variant) {
      case 'brand':
        return 'border border-[var(--jc-brand-primary)] text-[var(--jc-brand-primary)]'
      case 'purple':
        return 'border border-[var(--jc-brand-purple)] text-[var(--jc-brand-purple)]'
      case 'success':
        return 'border border-[var(--jc-success)] text-[var(--jc-success)]'
      case 'warning':
        return 'border border-[var(--jc-warning)] text-[var(--jc-warning)]'
      case 'danger':
        return 'border border-[var(--jc-danger)] text-[var(--jc-danger)]'
      case 'info':
        return 'border border-[var(--jc-info)] text-[var(--jc-info)]'
      case 'default':
      default:
        return 'border border-default text-secondary'
    }
  }
  switch (props.variant) {
    case 'brand':
      return 'bg-[var(--jc-brand-primary)] text-white'
    case 'purple':
      return 'jc-tag--purple text-[var(--jc-brand-purple)]'
    case 'success':
      return 'bg-[var(--jc-success-soft)] text-[var(--jc-success)]'
    case 'warning':
      return 'bg-[var(--jc-warning-soft)] text-[var(--jc-warning)]'
    case 'danger':
      return 'bg-[var(--jc-danger-soft)] text-[var(--jc-danger)]'
    case 'info':
      return 'bg-[var(--jc-info-soft)] text-[var(--jc-info)]'
    case 'default':
    default:
      return 'jc-tag--default text-[var(--jc-text-primary)]'
  }
})
</script>

<template>
  <span
    class="jc-tag inline-flex items-center justify-center font-[var(--jc-fw-medium)] rounded-[var(--jc-radius-sm)] whitespace-nowrap leading-none"
    :class="[sizeClass, variantClass]"
  >
    <slot />
  </span>
</template>

<style scoped>
.jc-tag--purple {
  background-color: rgba(155, 73, 231, 0.2);
}
.jc-tag--default {
  background-color: rgba(0, 0, 0, 0.6);
}
</style>
