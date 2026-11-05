<script setup lang="ts" generic="V extends string | number = string">
/**
 * 统一页签形态（复核 #13 / 规格 §3.3）:
 *  - segment（默认）= 胶囊组，逐像素照抄仪表盘在线明细手写胶囊（D①: pill 渐变按钮组已删除）
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
    /** 'segment' = 胶囊组（默认）; 'underline' = 下划线 tab */
    variant?: 'segment' | 'underline'
  }>(),
  { variant: 'segment' }
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
      class="jc-tab px-[var(--jc-space-4)] py-[var(--jc-ctrl-py)] text-[length:var(--jc-ctrl-fs)] transition-colors border-b-2 -mb-px inline-flex items-center gap-[var(--jc-space-1)]"
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

  <!-- segment（默认）: 胶囊页签 —— 2026-11-05 起**全绑令牌**（高 sm/标准/lg、字号、内外距随三档自适应）；
       激活 bg-surface+shadow / 未激活 text-secondary 的视觉规则不变；容器 p-0.5 保持 -->
  <div v-else class="flex rounded-full bg-elevated p-0.5" role="tablist">
    <button
      v-for="t in props.tabs"
      :key="String(t.value)"
      type="button"
      role="tab"
      class="px-[var(--jc-ctrl-px)] py-[var(--jc-ctrl-py)] rounded-full min-h-[var(--jc-ctrl-h)] text-[length:var(--jc-ctrl-fs)] transition-colors"
      :class="props.modelValue === t.value ? 'bg-surface shadow text-primary' : 'text-secondary'"
      :aria-selected="props.modelValue === t.value"
      data-focusable="true"
      @click="onSelect(t.value)"
    >
      {{ t.label }}
      <span v-if="t.count !== undefined && t.count > 0" class="jc-tabs__count">（{{ t.count }}）</span>
    </button>
  </div>
</template>
