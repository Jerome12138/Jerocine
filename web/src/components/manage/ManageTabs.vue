<script setup lang="ts" generic="V extends string | number = string">
import BaseButton from '@/components/base/BaseButton.vue'

/**
 * 统一页签形态（复核 #13）:
 *  - pill（默认）= 按钮组（BaseButton: active gradient / 其余 ghost，现状同构）
 *  - underline = 下划线 tab（底边框高亮）
 * 无障碍: role=tablist/tab + aria-selected; 一切项恒 data-focusable（TV 遥控）。
 */
export interface TabItem<V2 extends string | number = string> {
  value: V2
  label: string
  /** 可选计数徽标 */
  count?: number
}

// generic V 经 defineProps 内联对象类型引用(独立 interface 引用泛型参数 vue-tsc 解析不到);
// TabItem 供页面 import type 复用(value 类型收窄用)
const props = withDefaults(
  defineProps<{
    modelValue: V
    tabs: TabItem<V>[]
    /** 'pill' = 按钮组（默认）; 'underline' = 下划线 tab */
    variant?: 'pill' | 'underline'
  }>(),
  { variant: 'pill' }
)
const emit = defineEmits<{ 'update:modelValue': [value: V] }>()

function onSelect(value: V): void {
  emit('update:modelValue', value)
}
</script>

<template>
  <div
    v-if="props.variant === 'underline'"
    class="jc-tabs--underline flex items-center gap-[var(--jc-space-2)] border-b border-subtle"
    role="tablist"
  >
    <button
      v-for="t in props.tabs"
      :key="String(t.value)"
      type="button"
      role="tab"
      class="jc-tab px-[var(--jc-space-4)] py-[var(--jc-space-2)] text-sm transition-colors border-b-2 -mb-px inline-flex items-center gap-[var(--jc-space-1)]"
      :class="
        props.modelValue === t.value
          ? 'jc-tab--active border-[var(--jc-brand-purple)] text-[var(--jc-brand-purple)] font-[var(--jc-fw-medium)]'
          : 'border-transparent text-muted hover:text-primary'
      "
      :aria-selected="props.modelValue === t.value"
      data-focusable="true"
      @click="onSelect(t.value)"
    >
      {{ t.label }}
      <span v-if="t.count !== undefined && t.count > 0" class="jc-tabs__count text-xs text-muted">
        {{ t.count }}
      </span>
    </button>
  </div>

  <div v-else class="flex gap-[var(--jc-space-1)] flex-wrap" role="tablist">
    <BaseButton
      v-for="t in props.tabs"
      :key="String(t.value)"
      :variant="props.modelValue === t.value ? 'gradient' : 'ghost'"
      size="sm"
      role="tab"
      :aria-selected="props.modelValue === t.value"
      data-focusable="true"
      @click="onSelect(t.value)"
    >
      {{ t.label }}
      <span v-if="t.count !== undefined && t.count > 0" class="jc-tabs__count">（{{ t.count }}）</span>
    </BaseButton>
  </div>
</template>
