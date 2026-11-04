<script setup lang="ts">
import { ref, watchEffect } from 'vue'

/**
 * 管理端复选框（规格 §3.4 / D③）：label 包裹 + 视觉隐藏原生 input + 自绘方框。
 * 零依赖纯 CSS 实现 —— 原生 accent 跨端观感不一，且 indeterminate 横杠需要自绘。
 *
 * - 简单文案用 label prop；复杂内容（标签/说明/右对齐码）用默认 slot。
 * - input 用 :checked + @change 手控而非 v-model：change 时先把 DOM 勾选态
 *   还原为 props 值再上报，父级不回写时保持受控语义（更新经 :checked 写回）。
 * - indeterminate 是 DOM prop 而非 attribute，经 watchEffect 直写，
 *   不走 Vue 模板绑定，避免重渲染把它重置回 false。
 * - input 保留几何位置（absolute + 0 尺寸）供 TV 空间导航按 DOM 寻焦。
 */
interface Props {
  modelValue: boolean
  indeterminate?: boolean
  disabled?: boolean
  /** 兜底文案；复杂内容用默认 slot */
  label?: string
}

const props = withDefaults(defineProps<Props>(), {
  indeterminate: false,
  disabled: false,
  label: ''
})

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const inputRef = ref<HTMLInputElement | null>(null)

// flush:'sync' —— 挂载/换绑 ref 与 prop 翻转时立刻直写 DOM prop，
// 既有调用方(ManageCascader)在 mount 后同步断言 indeterminate，不能等微任务 flush
watchEffect(
  () => {
    if (inputRef.value) inputRef.value.indeterminate = props.indeterminate
  },
  { flush: 'sync' }
)

function onChange(e: Event): void {
  if (props.disabled) return
  const el = e.target as HTMLInputElement
  const next = el.checked
  // 受控回写：先还原为 props 值，是否翻转由父级根据 emit 决定（modelValue 更新后经 :checked 写回）
  el.checked = props.modelValue
  emit('update:modelValue', next)
}
</script>

<template>
  <label
    class="jc-checkbox inline-flex items-center gap-[var(--jc-space-2)]"
    :class="disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'"
  >
    <input
      ref="inputRef"
      type="checkbox"
      class="jc-checkbox__input absolute h-0 w-0 opacity-0"
      :checked="modelValue"
      :disabled="disabled"
      data-focusable="true"
      @change="onChange"
    />
    <!-- 自绘方框：选中/半选为品牌色块，未选中为描边空框 -->
    <span
      aria-hidden="true"
      class="jc-checkbox__box flex h-4 w-4 shrink-0 items-center justify-center rounded-[var(--jc-radius-sm)] border transition-colors"
      :class="
        modelValue || indeterminate
          ? 'jc-checkbox__box--on border-[var(--jc-brand-primary)] bg-[var(--jc-brand-primary)]'
          : 'border-strong bg-elevated'
      "
    >
      <svg v-if="indeterminate" viewBox="0 0 12 12" class="jc-checkbox__bar h-2 w-2" aria-hidden="true">
        <line x1="2.5" y1="6" x2="9.5" y2="6" stroke="#fff" stroke-width="2" stroke-linecap="round" />
      </svg>
      <svg v-else-if="modelValue" viewBox="0 0 12 12" class="h-2.5 w-2.5" aria-hidden="true">
        <polyline
          points="2.5,6.5 5,9 9.5,3.5"
          fill="none"
          stroke="#fff"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </span>
    <span v-if="label" class="text-sm">{{ label }}</span>
    <slot />
  </label>
</template>

<style scoped>
/* hover 提亮：仅未选中态（border-strong → 更亮一档），选中色块不随 hover 变化 */
.jc-checkbox:hover .jc-checkbox__box:not(.jc-checkbox__box--on) {
  border-color: var(--jc-text-muted);
}
/* 键盘焦点环画在方框上（input 本体 0 尺寸不可见），与全局 focus-ring 令牌同源 */
.jc-checkbox__input:focus-visible + .jc-checkbox__box {
  box-shadow: var(--jc-shadow-focus-ring);
}
</style>
