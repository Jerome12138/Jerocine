<script setup lang="ts">
import { computed } from 'vue'

/**
 * 管理端 Switch（Element Plus 风格封装，自研、零依赖）
 *
 * props: v-model(modelValue) / disabled / loading / size(large|default|small) / activeText / inactiveText
 * 尺寸: 由 --jc-switch-* 令牌随视口宽度分档(移动/常规/宽屏/TV), 不再按设备二元切换。
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

/** 尺寸档透传给 data-size; 实际像素由 --jc-switch-* 令牌随视口分档 */
const normSize = computed(() => props.size)

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
  /* 尺寸由 --jc-switch-* 令牌随视口宽度分档; scale 供 size 档整体缩放 */
  --jc-switch-scale: 1;
  width: calc(var(--jc-switch-w) * var(--jc-switch-scale));
  height: calc(var(--jc-switch-h) * var(--jc-switch-scale));
  /* 作为 flex 子项时不被压垮 */
  flex-shrink: 0;
  /* 触控命中区: ::after 向四周扩 10px */
}
.jc-switch[data-size='small'] {
  --jc-switch-scale: 0.85;
}
.jc-switch[data-size='large'] {
  --jc-switch-scale: 1.25;
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
  width: calc(var(--jc-switch-knob) * var(--jc-switch-scale));
  height: calc(var(--jc-switch-knob) * var(--jc-switch-scale));
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
}
.jc-switch__knob--off {
  transform: translateX(calc(2px * var(--jc-switch-scale)));
}
.jc-switch__knob--on {
  transform: translateX(calc((var(--jc-switch-w) - var(--jc-switch-knob) - 2px) * var(--jc-switch-scale)));
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
/* TV 模式: 尺寸已由 --jc-switch-* 令牌在 theme.css 的 [data-mode='tv'] 中统一放大, 这里只补 focus 环 */
[data-mode='tv'] .jc-switch:focus,
[data-mode='tv'] .jc-switch:focus-visible {
  box-shadow: var(--jc-tv-focus-ring);
}
</style>