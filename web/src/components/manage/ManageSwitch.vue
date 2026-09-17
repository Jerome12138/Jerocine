<script setup lang="ts">
interface Props {
  modelValue: boolean
  disabled?: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

function toggle(): void {
  if (!props.disabled) emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="props.modelValue"
    :disabled="props.disabled"
    class="jc-switch relative inline-flex items-center rounded-full transition-colors disabled:opacity-50"
    :class="props.modelValue ? 'bg-[var(--jc-brand-purple)]' : 'bg-[var(--jc-border-default)]'"
    data-focusable="true"
    @click="toggle"
  >
    <span class="jc-switch__hit" aria-hidden="true" />
    <span
      class="jc-switch__knob inline-block rounded-full bg-white transition-transform"
      :class="props.modelValue ? 'jc-switch__knob--on' : 'jc-switch__knob--off'"
    />
  </button>
</template>

<style scoped>
.jc-switch {
  /* 视觉宽高 */
  width: 44px;
  height: 28px;
  /* 作为 flex 子项时不被压缩 (否则窄屏下 track 塌缩, 只剩圆形 knob) */
  flex-shrink: 0;
  /* 触控目标至少 44x44，通过 ::before 拓展但不影响布局 */
  position: relative;
}
.jc-switch__hit {
  position: absolute;
  inset: -8px;
  /* 透明扩展点击区到 60x44，符合 44px 触控目标 */
  pointer-events: none;
}
.jc-switch__knob {
  width: 22px;
  height: 22px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
}
.jc-switch__knob--off {
  transform: translateX(3px);
}
.jc-switch__knob--on {
  transform: translateX(19px);
}

/* TV 模式 + focus 环 */
.jc-switch:focus,
.jc-switch:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}
</style>

<style>
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
