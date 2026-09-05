<script setup lang="ts">
import { computed } from 'vue'

type Variant = 'primary' | 'ghost' | 'gradient' | 'danger' | 'outline'
type Size = 'sm' | 'md' | 'lg' | 'xl'

interface Props {
  variant?: Variant
  size?: Size
  disabled?: boolean
  loading?: boolean
  block?: boolean
  /** 按钮整体作为 router-link 时的目标 */
  type?: 'button' | 'submit' | 'reset'
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  disabled: false,
  loading: false,
  block: false,
  type: 'button'
})

const emit = defineEmits<{
  (e: 'click', evt: MouseEvent): void
}>()

const sizeClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-8 px-3 text-[var(--gf-fs-sm)] gap-[var(--gf-space-1)]'
    case 'lg':
      return 'h-12 px-6 text-[var(--gf-fs-md)] gap-[var(--gf-space-2)]'
    case 'xl':
      return 'h-14 px-8 text-[var(--gf-fs-lg)] gap-[var(--gf-space-2)] rounded-[var(--gf-radius-xl)]'
    case 'md':
    default:
      return 'h-10 px-4 text-[var(--gf-fs-md)] gap-[var(--gf-space-2)]'
  }
})

const variantClass = computed(() => {
  switch (props.variant) {
    case 'gradient':
      return 'gf-btn-gradient text-white'
    case 'ghost':
      return 'bg-transparent text-secondary hover:bg-[rgba(255,255,255,0.06)] hover:text-primary active:bg-[rgba(255,255,255,0.1)]'
    case 'outline':
      return 'bg-transparent border border-strong text-primary hover:bg-[rgba(255,255,255,0.08)] active:bg-[rgba(255,255,255,0.12)]'
    case 'danger':
      return 'bg-[var(--gf-danger)] text-white hover:brightness-110 active:brightness-90'
    case 'primary':
    default:
      return 'bg-[var(--gf-brand-primary)] text-white hover:bg-[var(--gf-brand-primary-hover)] active:bg-[var(--gf-brand-primary-active)] hover:shadow-brand-glow'
  }
})

function handleClick(e: MouseEvent): void {
  if (props.disabled || props.loading) {
    e.preventDefault()
    e.stopPropagation()
    return
  }
  emit('click', e)
}
</script>

<template>
  <button
    :type="type"
    :data-size="size"
    :disabled="disabled || loading"
    :data-focusable="disabled || loading ? undefined : 'true'"
    :tabindex="disabled || loading ? -1 : 0"
    class="gf-btn inline-flex items-center justify-center font-[var(--gf-fw-semibold)] rounded-[var(--gf-radius-md)] gf-btn-transition select-none whitespace-nowrap"
    :class="[
      sizeClass,
      variantClass,
      block ? 'w-full flex' : '',
      (disabled || loading) ? 'opacity-40 cursor-not-allowed' : 'cursor-pointer'
    ]"
    @click="handleClick"
  >
    <span v-if="loading" class="gf-btn-spinner" aria-hidden="true" />
    <slot v-else name="icon" />
    <slot />
  </button>
</template>

<style scoped>
.gf-btn {
  /* 触控命中区不小于 44 */
  min-height: 44px;
  /* sm 尺寸通过外层填充补足 */
}

.gf-btn-transition {
  transition:
    background-color var(--gf-dur-base) var(--gf-ease-standard),
    box-shadow var(--gf-dur-base) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-standard),
    opacity var(--gf-dur-base) var(--gf-ease-standard);
}
.gf-btn[data-size='sm'] {
  min-height: 32px;
}

.gf-btn-gradient {
  background-image: var(--gf-brand-gradient);
}
.gf-btn-gradient:hover {
  background-image: var(--gf-brand-gradient-hover);
  box-shadow: var(--gf-shadow-purple-glow);
}
.gf-btn-gradient:active {
  transform: scale(0.98);
}

.gf-btn-spinner {
  width: 1em;
  height: 1em;
  border: 2px solid rgba(255, 255, 255, 0.45);
  border-top-color: #fff;
  border-radius: 9999px;
  animation: gf-btn-spin 0.8s linear infinite;
}

@keyframes gf-btn-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
