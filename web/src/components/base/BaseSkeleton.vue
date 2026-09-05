<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  shape?: 'rect' | 'circle' | 'text'
  width?: string
  height?: string
  /** 重复几个 */
  count?: number
  /** 圆角类，shape rect 默认 12px */
  rounded?: string
  ratio?: string
}

const props = withDefaults(defineProps<Props>(), {
  shape: 'rect',
  width: '100%',
  height: '',
  count: 1,
  rounded: '',
  ratio: ''
})

const itemStyle = computed(() => {
  const style: Record<string, string> = {
    width: props.width
  }
  if (props.shape === 'circle') {
    // 圆形保证宽=高
    style.height = props.height || props.width
    style.borderRadius = '9999px'
  } else if (props.shape === 'text') {
    style.height = props.height || '0.85em'
    style.borderRadius = 'var(--gf-radius-sm)'
  } else {
    if (props.height) style.height = props.height
    if (props.ratio) style.aspectRatio = props.ratio
    style.borderRadius = 'var(--gf-radius-lg)'
  }
  return style
})

const items = computed(() => Array.from({ length: Math.max(1, props.count) }))
</script>

<template>
  <div class="gf-skeleton-wrap flex flex-col gap-[var(--gf-space-2)]">
    <div
      v-for="(_, i) in items"
      :key="i"
      class="gf-skeleton"
      :class="rounded"
      :style="itemStyle"
      aria-hidden="true"
    />
  </div>
</template>

<style scoped>
.gf-skeleton {
  display: block;
  background: linear-gradient(
    90deg,
    rgba(255, 255, 255, 0.04) 0%,
    rgba(255, 255, 255, 0.08) 50%,
    rgba(255, 255, 255, 0.04) 100%
  );
  background-size: 200% 100%;
  animation: gf-skeleton-shimmer var(--gf-dur-page) linear infinite;
}

@keyframes gf-skeleton-shimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .gf-skeleton {
    animation: none;
    opacity: 0.6;
  }
}
</style>
