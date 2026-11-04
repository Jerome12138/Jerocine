<script setup lang="ts">
/**
 * 开关视觉件(W1a): 只渲染轨道+滑块, 交互语义(role=switch/aria-checked)由外层按钮承担
 * (BaseSwitchRow / 调用方自建按钮), 本件纯视觉、aria-hidden。
 *
 * size 双档 = 两处既有视觉逐字收敛(两档圆角/底色/滑块尺寸/位移/过渡均不同, 严禁互相"统一"):
 *   - lg(44×24): SettingsView 行内开关原样式(jc-settings-page__switch/__knob);
 *   - sm(36×20): PlayView 跳过弹窗开关原样式(jc-skip-switch__track/__thumb;
 *     is-on 状态类由原「按钮上」移到轨道自身, 计算样式等价)。
 */
withDefaults(
  defineProps<{
    /** 开关状态(视觉, 不产生交互) */
    modelValue?: boolean
    /** lg=44×24(默认) / sm=36×20 */
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
/* —— lg(44×24): 原样迁移自 SettingsView —— */
.jc-settings-page__switch {
  flex: 0 0 auto;
  width: 44px;
  height: 24px;
  border-radius: 12px;
  background: var(--jc-bg-surface);
  position: relative;
  transition: background 0.15s ease;
}
.jc-settings-page__switch.is-on {
  background: var(--jc-brand-primary);
}
.jc-settings-page__knob {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.15s ease;
}
.jc-settings-page__switch.is-on .jc-settings-page__knob {
  transform: translateX(20px);
}

/* —— sm(36×20): 原样迁移自 PlayView 跳过弹窗 —— */
.jc-skip-switch__track {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  border-radius: 999px;
  background-color: var(--jc-text-muted);
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch__track.is-on {
  background-color: var(--jc-brand-primary);
}
.jc-skip-switch__thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background-color: #fff;
  transition: transform var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch__track.is-on .jc-skip-switch__thumb {
  transform: translateX(16px);
}
</style>
