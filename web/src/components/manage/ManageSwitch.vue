<script setup lang="ts">
import { computed } from 'vue'
import { useViewMode } from '@/composables/useViewMode'

/**
 * 管理端 Switch（Element Plus 风格封装，自研、零依赖）
 *
 * props: v-model(modelValue) / disabled / loading / size(large|default|small) / activeText / inactiveText
 * 尺寸: 移动端(<768) 自动小尺寸。
 */
interface Props {
  modelValue: boolean
  disabled?: boolean
  loading?: boolean
  size?: 'large' | 'default' | 'small'
  /** 开关内联文字(EP inline-prompt 风格), 不传则仅显示圆点 */
  activeText?: string
  inactiveText?: string
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  loading: false,
  size: 'default'
})

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const { isMobile } = useViewMode()

/** 移动端一律小尺寸 */
const normSize = computed(() => (isMobile.value ? 'small' : props.size))

const hasText = computed(() => !!(props.activeText || props.inactiveText))

function toggle(): void {
  if (props.disabled || props.loading) return
  emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="modelValue"
    :disabled="disabled || loading"
    :data-size="normSize"
    class="jc-switch relative inline-flex items-center rounded-full transition-colors disabled:opacity-50"
    :class="[
      modelValue ? 'jc-switch--on' : 'jc-switch--off',
      hasText ? 'jc-switch--with-text' : ''
    ]"
    data-focusable="true"
    @click="toggle"
  >
    <span v-if="hasText" class="jc-switch__text">
      {{ modelValue ? (activeText ?? '') : (inactiveText ?? '') }}
    </span>
    <span
      class="jc-switch__knob inline-block rounded-full bg-white transition-transform"
      :class="modelValue ? 'jc-switch__knob--on' : 'jc-switch__knob--off'"
    >
      <span v-if="loading" class="jc-switch__spinner" aria-hidden="true" />
    </span>
  </button>
</template>

<style scoped>
.jc-switch {
  /* 默认(medium): 40 x 20 */
  width: 40px;
  height: 20px;
  /* 作为 flex 子项时不被压垮 */
  flex-shrink: 0;
  /* 触控命中区: ::after 向四周扩 10px, 实际可点 ≥ 40x40 */
}
.jc-switch::after {
  content: '';
  position: absolute;
  inset: -10px;
}
.jc-switch--on {
  background-color: var(--jc-brand-purple);
}
.jc-switch--off {
  background-color: var(--jc-border-default);
}

.jc-switch__knob {
  position: relative;
  width: 16px;
  height: 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
}
.jc-switch__knob--off {
  transform: translateX(2px);
}
.jc-switch__knob--on {
  transform: translateX(22px);
}

/* small: 32 x 16 */
.jc-switch[data-size='small'] {
  width: 32px;
  height: 16px;
}
.jc-switch[data-size='small'] .jc-switch__knob {
  width: 12px;
  height: 12px;
}
.jc-switch[data-size='small'] .jc-switch__knob--off {
  transform: translateX(2px);
}
.jc-switch[data-size='small'] .jc-switch__knob--on {
  transform: translateX(18px);
}

/* large: 50 x 24 */
.jc-switch[data-size='large'] {
  width: 50px;
  height: 24px;
}
.jc-switch[data-size='large'] .jc-switch__knob {
  width: 20px;
  height: 20px;
}
.jc-switch[data-size='large'] .jc-switch__knob--off {
  transform: translateX(2px);
}
.jc-switch[data-size='large'] .jc-switch__knob--on {
  transform: translateX(28px);
}

/* 内联文字: 圆点两侧留出文字空间 */
.jc-switch--with-text {
  width: auto;
  padding: 0 4px;
  gap: 4px;
}
.jc-switch__text {
  font-size: var(--jc-fs-xs);
  line-height: 1;
  color: #fff;
  user-select: none;
  white-space: nowrap;
}
.jc-switch--with-text[data-size='small'] .jc-switch__text {
  font-size: var(--jc-fs-xs);
}
/* 内联文字态的圆点改为绝对定位到底色轨道内, 避免挤压 */
.jc-switch--with-text .jc-switch__knob {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
}
.jc-switch--with-text .jc-switch__knob--off {
  left: 2px;
}
.jc-switch--with-text .jc-switch__knob--on {
  left: auto;
  right: 2px;
}

.jc-switch__spinner {
  position: absolute;
  inset: 0;
  margin: auto;
  width: 10px;
  height: 10px;
  border: 2px solid rgba(0, 0, 0, 0.2);
  border-top-color: var(--jc-brand-purple);
  border-radius: 9999px;
  animation: jc-switch-spin 0.8s linear infinite;
}

@keyframes jc-switch-spin {
  to {
    transform: rotate(360deg);
  }
}

/* TV 模式 + focus 环 */
.jc-switch:focus,
.jc-switch:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}
</style>

<style>
/* TV 模式放大(全局, 与既有约定一致) */
[data-mode='tv'] .jc-switch {
  width: 64px;
  height: 36px;
}
[data-mode='tv'] .jc-switch__knob {
  width: 30px;
  height: 30px;
}
[data-mode='tv'] .jc-switch__knob--off {
  transform: translateX(3px);
}
[data-mode='tv'] .jc-switch__knob--on {
  transform: translateX(31px);
}
[data-mode='tv'] .jc-switch:focus,
[data-mode='tv'] .jc-switch:focus-visible {
  box-shadow: var(--jc-tv-focus-ring);
}
</style>