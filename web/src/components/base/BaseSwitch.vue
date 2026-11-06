<script setup lang="ts">
/**
 * 开关视觉件(W1a): 只渲染轨道+滑块, 交互语义(role=switch/aria-checked)由外层按钮承担
 * (BaseSwitchRow / 调用方自建按钮), 本件纯视觉、aria-hidden。
 *
 * size 双档 = 两处既有视觉逐字收敛(两档圆角/底色/滑块尺寸/位移/过渡均不同, 严禁互相"统一"):
 *   - lg = --jc-switch-* 三档自适应(scale=1; SettingsView 行内开关原款类 jc-settings-page__switch/__knob);
 *   - sm = --jc-switch-* 三档自适应(scale=0.85; PlayView 跳过弹窗原款类 jc-skip-switch__track/__thumb;
 *     is-on 状态类由原「按钮上」移到轨道自身, 计算样式等价)。
 */
withDefaults(
  defineProps<{
    /** 开关状态(视觉, 不产生交互) */
    modelValue?: boolean
    /** lg=--jc-switch-* 三档自适应(scale=1, 默认) / sm=同令牌 scale=0.85 */
    size?: 'lg' | 'sm'
  }>(),
  { modelValue: false, size: 'lg' }
)
</script>

<template>
  <span
    v-if="size === 'lg'"
    class="jc-settings-page__switch"
    :class="{ 'is-on': modelValue }"
    aria-hidden="true"
  >
    <span class="jc-settings-page__knob" />
  </span>
  <span
    v-else
    class="jc-skip-switch__track"
    :class="{ 'is-on': modelValue }"
    aria-hidden="true"
  >
    <span class="jc-skip-switch__thumb" />
  </span>
</template>

<style scoped>
/* —— lg: 绑 --jc-switch-*(scale=1) —— */
.jc-settings-page__switch {
  --jc-sw-scale: 1;
  flex: 0 0 auto;
  width: calc(var(--jc-switch-w) * var(--jc-sw-scale));
  height: calc(var(--jc-switch-h) * var(--jc-sw-scale));
  border-radius: calc(var(--jc-switch-h) * var(--jc-sw-scale) / 2);
  background: var(--jc-bg-surface);
  position: relative;
  transition: background 0.15s ease;
}
.jc-settings-page__switch.is-on {
  background: var(--jc-brand-primary);
}
.jc-settings-page__knob {
  position: absolute;
  top: calc(3px * var(--jc-sw-scale));
  left: calc(3px * var(--jc-sw-scale));
  width: calc(var(--jc-switch-knob) * var(--jc-sw-scale));
  height: calc(var(--jc-switch-knob) * var(--jc-sw-scale));
  border-radius: 50%;
  background: #fff;
  transition: transform 0.15s ease;
}
.jc-settings-page__switch.is-on .jc-settings-page__knob {
  transform: translateX(calc((var(--jc-switch-w) - var(--jc-switch-knob) - 6px) * var(--jc-sw-scale)));
}

/* —— sm: 绑 --jc-switch-*(scale=0.85, 兼容原 36×20 档) —— */
.jc-skip-switch__track {
  --jc-sw-scale: 0.85;
  position: relative;
  flex: none;
  width: calc(var(--jc-switch-w) * var(--jc-sw-scale));
  height: calc(var(--jc-switch-h) * var(--jc-sw-scale));
  border-radius: 999px;
  background-color: var(--jc-text-muted);
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch__track.is-on {
  background-color: var(--jc-brand-primary);
}
.jc-skip-switch__thumb {
  position: absolute;
  top: calc(2px * var(--jc-sw-scale));
  left: calc(2px * var(--jc-sw-scale));
  width: calc(var(--jc-switch-knob) * var(--jc-sw-scale));
  height: calc(var(--jc-switch-knob) * var(--jc-sw-scale));
  border-radius: 50%;
  background-color: #fff;
  transition: transform var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch__track.is-on .jc-skip-switch__thumb {
  transform: translateX(calc((var(--jc-switch-w) - var(--jc-switch-knob) - 4px) * var(--jc-sw-scale)));
}
</style>
