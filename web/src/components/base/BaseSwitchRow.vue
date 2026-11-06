<script setup lang="ts">
import BaseSwitch from './BaseSwitch.vue'

/**
 * 开关行(W1a): 标题+描述+开关的整行按钮 —— role=switch / aria-checked 跟随值 /
 * data-focusable 保留 / 点击 emit update:modelValue(翻转值)。
 * 数据驱动沿用 SettingsView 的 toggles 数组(键名即 localStorage 契约的语义名)。
 * 行类名 jc-settings-page__row* 逐字保留(SettingsView 视觉契约); __row 系样式在本件
 * 自带一份(值不变), SettingsView 同值规则留存供页内数字行(非开关行)继续使用。
 */
withDefaults(
  defineProps<{
    modelValue?: boolean
    title: string
    desc?: string
    /** 开关尺寸(默认 lg=--jc-switch-* 三档自适应, SettingsView 现用档) */
    size?: 'lg' | 'sm'
  }>(),
  { modelValue: false, size: 'lg' }
)

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <button
    type="button"
    class="jc-settings-page__row"
    role="switch"
    :aria-checked="modelValue"
    data-focusable="true"
    @click="emit('update:modelValue', !modelValue)"
  >
    <span class="jc-settings-page__row-main">
      <span class="jc-settings-page__row-title">{{ title }}</span>
      <span v-if="desc" class="jc-settings-page__row-desc">{{ desc }}</span>
    </span>
    <BaseSwitch :model-value="modelValue" :size="size" />
  </button>
</template>

<style scoped>
/* 值原样迁移自 SettingsView(开关行部分); __row 在 SettingsView 另有同值规则(数字行用) */
.jc-settings-page__row {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  width: 100%;
  text-align: left;
  padding: var(--jc-space-3) 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.jc-settings-page__row-main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.jc-settings-page__row-title {
  font-size: var(--jc-fs-base);
  color: var(--jc-text-primary);
}
.jc-settings-page__row-desc {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
  line-height: var(--jc-lh-snug);
}
</style>
