<script setup lang="ts">
import { computed } from 'vue'

/**
 * 管理端 Button（Element Plus 风格封装，自研、零依赖）
 *
 * EP 风格 props: type / plain / round / circle / text / link / size(large|default|small) / nativeType
 * 兼容旧 BaseButton props: variant(primary|ghost|gradient|danger|outline) / size(sm|md|lg|xl) / type(button|submit|reset)
 *
 * 尺寸: 移动端(<768) 一律小尺寸; 传原生 type(button/submit/reset) 时等价 native-type, 样式按 default。
 */
type BtnType = 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
type NativeType = 'button' | 'submit' | 'reset'
type LegacyVariant = 'primary' | 'ghost' | 'gradient' | 'danger' | 'outline'
type BtnSize = 'large' | 'default' | 'small'
type SizeInput = BtnSize | 'sm' | 'md' | 'lg' | 'xl'

interface Props {
  /** EP 风格类型; 若传原生类型(button/submit/reset) 则等价 nativeType, 样式按 default */
  type?: BtnType | NativeType
  /** 兼容旧 BaseButton 的 variant */
  variant?: LegacyVariant
  size?: SizeInput
  plain?: boolean
  round?: boolean
  circle?: boolean
  text?: boolean
  link?: boolean
  loading?: boolean
  disabled?: boolean
  block?: boolean
  nativeType?: NativeType
}

const props = withDefaults(defineProps<Props>(), {
  type: 'default',
  size: 'default',
  plain: false,
  round: false,
  circle: false,
  text: false,
  link: false,
  loading: false,
  disabled: false,
  block: false
})

const emit = defineEmits<{ (e: 'click', evt: MouseEvent): void }>()

const NATIVE_TYPES: readonly NativeType[] = ['button', 'submit', 'reset']
const isNativeType = computed(() => NATIVE_TYPES.includes(props.type as NativeType))

/** 原生 button type 属性 */
const nativeType = computed<NativeType>(() =>
  props.nativeType ?? (isNativeType.value ? (props.type as NativeType) : 'button')
)

/** EP 风格语义 type (variant 优先, 兼容旧 API) */
const styleType = computed<BtnType>(() => {
  if (props.variant) {
    switch (props.variant) {
      case 'gradient':
      case 'primary':
      case 'outline':
        return 'primary'
      case 'danger':
        return 'danger'
      case 'ghost':
      default:
        return 'default'
    }
  }
  return isNativeType.value ? 'default' : (props.type as BtnType)
})

/** 归一化尺寸: 兼容旧 sm/md/lg/xl; 实际像素由 --jc-ctrl-* 令牌随视口宽度分档 */
const normSize = computed<BtnSize>(() => {
  switch (props.size) {
    case 'sm':
    case 'small':
      return 'small'
    case 'lg':
    case 'xl':
    case 'large':
      return 'large'
    default:
      return 'default'
  }
})

const sizeClass = computed(() => {
  if (props.circle) {
    switch (normSize.value) {
      case 'small':
        return 'w-[var(--jc-ctrl-h-sm)] h-[var(--jc-ctrl-h-sm)] text-[length:var(--jc-ctrl-fs-sm)]'
      case 'large':
        return 'w-[var(--jc-ctrl-h-lg)] h-[var(--jc-ctrl-h-lg)] text-[length:var(--jc-ctrl-fs-lg)]'
      default:
        return 'w-[var(--jc-ctrl-h)] h-[var(--jc-ctrl-h)] text-[length:var(--jc-ctrl-fs)]'
    }
  }
  switch (normSize.value) {
    case 'small':
      return 'h-[var(--jc-ctrl-h-sm)] px-[var(--jc-ctrl-px-sm)] text-[length:var(--jc-ctrl-fs-sm)] gap-[var(--jc-space-1)]'
    case 'large':
      return 'h-[var(--jc-ctrl-h-lg)] px-[var(--jc-ctrl-px-lg)] text-[length:var(--jc-ctrl-fs-lg)] gap-[var(--jc-space-2)]'
    default:
      return 'h-[var(--jc-ctrl-h)] px-[var(--jc-ctrl-px)] text-[length:var(--jc-ctrl-fs)] gap-[var(--jc-space-2)]'
  }
})

const roundClass = computed(() =>
      props.round || props.circle ? 'rounded-full' : 'rounded-[var(--jc-ctrl-radius)]'
)

/** 旧 variant 样式(与 BaseButton 一致, 保证既有外观/jc-btn-gradient 断言不变) */
const legacyClass = computed(() => {
  switch (props.variant) {
    case 'gradient':
      return 'jc-btn-gradient text-white'
    case 'ghost':
      return 'bg-transparent text-secondary hover:bg-[rgba(255,255,255,0.06)] hover:text-primary active:bg-[rgba(255,255,255,0.1)]'
    case 'outline':
      return 'bg-transparent border border-strong text-primary hover:bg-[rgba(255,255,255,0.08)] active:bg-[rgba(255,255,255,0.12)]'
    case 'danger':
      return 'bg-[var(--jc-danger)] text-white hover:brightness-110 active:brightness-90'
    case 'primary':
      return 'bg-[var(--jc-brand-primary)] text-white hover:bg-[var(--jc-brand-primary-hover)] active:bg-[var(--jc-brand-primary-active)] hover:shadow-brand-glow'
    default:
      return ''
  }
})

/** EP 风格样式(未传 variant 时生效) */
const epClass = computed(() => {
  if (props.variant) return ''
  if (props.link) return 'bg-transparent text-link hover:underline'
  if (props.text) return 'bg-transparent text-primary hover:bg-[rgba(255,255,255,0.06)]'
  const t = styleType.value
  if (props.plain) {
    const map = {
      primary: 'text-[var(--jc-brand-primary)] border border-[var(--jc-brand-primary)] hover:bg-[rgba(229,9,20,0.12)]',
      success: 'text-[var(--jc-success)] border border-[var(--jc-success)] hover:bg-[rgba(34,197,94,0.12)]',
      warning: 'text-[var(--jc-warning)] border border-[var(--jc-warning)] hover:bg-[rgba(245,158,11,0.12)]',
      danger: 'text-[var(--jc-danger)] border border-[var(--jc-danger)] hover:bg-[rgba(239,68,68,0.12)]',
      info: 'text-[var(--jc-info)] border border-[var(--jc-info)] hover:bg-[rgba(59,130,246,0.12)]',
      default: 'text-primary border border-strong hover:bg-[rgba(255,255,255,0.08)]'
    } as const
    return map[t]
  }
  const solid = {
    primary: 'bg-[var(--jc-brand-primary)] text-white hover:bg-[var(--jc-brand-primary-hover)]',
    success: 'bg-[var(--jc-success)] text-white hover:brightness-110',
    warning: 'bg-[var(--jc-warning)] text-black hover:brightness-110',
    danger: 'bg-[var(--jc-danger)] text-white hover:brightness-110',
    info: 'bg-[var(--jc-info)] text-white hover:brightness-110',
    default: 'bg-elevated border border-default text-primary hover:bg-[rgba(255,255,255,0.08)]'
  } as const
  return solid[t]
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
    :type="nativeType"
    :data-size="normSize"
    :data-type="styleType"
    :disabled="disabled || loading"
    :data-focusable="disabled || loading ? undefined : 'true'"
    :tabindex="disabled || loading ? -1 : 0"
    class="jc-btn inline-flex items-center justify-center font-[var(--jc-fw-semibold)] jc-btn-transition select-none whitespace-nowrap"
    :class="[
      sizeClass,
      roundClass,
      legacyClass,
      epClass,
      block ? 'w-full flex' : '',
      disabled || loading ? 'opacity-40 cursor-not-allowed' : 'cursor-pointer'
    ]"
    @click="handleClick"
  >
    <span v-if="loading" class="jc-btn-spinner" aria-hidden="true" />
    <slot v-else name="icon" />
    <slot />
  </button>
</template>

<style scoped>
.jc-btn {
  /* 尺寸由 --jc-ctrl-* 令牌随视口宽度分档; min-height 兜底触控命中区 */
  min-height: var(--jc-ctrl-h);
}

.jc-btn-transition {
  transition:
    background-color var(--jc-dur-base) var(--jc-ease-standard),
    box-shadow var(--jc-dur-base) var(--jc-ease-standard),
    transform var(--jc-dur-base) var(--jc-ease-standard),
    opacity var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-btn[data-size='small'] {
  min-height: var(--jc-ctrl-h-sm);
}
.jc-btn[data-size='large'] {
  min-height: var(--jc-ctrl-h-lg);
}

.jc-btn-gradient {
  background-image: var(--jc-brand-gradient);
}
.jc-btn-gradient:hover {
  background-image: var(--jc-brand-gradient-hover);
  box-shadow: var(--jc-shadow-purple-glow);
}
.jc-btn-gradient:active {
  transform: scale(0.98);
}

.jc-btn-spinner {
  width: 1em;
  height: 1em;
  border: 2px solid rgba(255, 255, 255, 0.45);
  border-top-color: #fff;
  border-radius: 9999px;
  animation: jc-btn-spin 0.8s linear infinite;
}

@keyframes jc-btn-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>