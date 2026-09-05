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
      return 'h-5 px-[6px] text-[var(--gf-fs-xs)]'
    case 'md':
      return 'h-7 px-[10px] text-[var(--gf-fs-sm)]'
    case 'sm':
    default:
      return 'h-6 px-2 text-[var(--gf-fs-xs)]'
  }
})

const variantClass = computed(() => {
  if (props.outlined) {
    switch (props.variant) {
      case 'brand':
        return 'border border-[var(--gf-brand-primary)] text-[var(--gf-brand-primary)]'
      case 'purple':
        return 'border border-[var(--gf-brand-purple)] text-[var(--gf-brand-purple)]'
      case 'success':
        return 'border border-[var(--gf-success)] text-[var(--gf-success)]'
      case 'warning':
        return 'border border-[var(--gf-warning)] text-[var(--gf-warning)]'
      case 'danger':
        return 'border border-[var(--gf-danger)] text-[var(--gf-danger)]'
      case 'info':
        return 'border border-[var(--gf-info)] text-[var(--gf-info)]'
      case 'default':
      default:
        return 'border border-default text-secondary'
    }
  }
  switch (props.variant) {
    case 'brand':
      return 'bg-[var(--gf-brand-primary)] text-white'
    case 'purple':
      return 'gf-tag--purple text-[var(--gf-brand-purple)]'
    case 'success':
      return 'bg-[var(--gf-success-soft)] text-[var(--gf-success)]'
    case 'warning':
      return 'bg-[var(--gf-warning-soft)] text-[var(--gf-warning)]'
    case 'danger':
      return 'bg-[var(--gf-danger-soft)] text-[var(--gf-danger)]'
    case 'info':
      return 'bg-[var(--gf-info-soft)] text-[var(--gf-info)]'
    case 'default':
    default:
      return 'gf-tag--default text-[var(--gf-text-primary)]'
  }
})
</script>

<template>
  <span
    class="gf-tag inline-flex items-center justify-center font-[var(--gf-fw-medium)] rounded-[var(--gf-radius-sm)] whitespace-nowrap leading-none"
    :class="[sizeClass, variantClass]"
  >
    <slot />
  </span>
</template>

<style scoped>
.gf-tag--purple {
  background-color: rgba(155, 73, 231, 0.2);
}
.gf-tag--default {
  background-color: rgba(0, 0, 0, 0.6);
}
</style>
