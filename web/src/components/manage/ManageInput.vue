<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  /** 数字字段空值约定（复核 #10）: type=number 清空后为 undefined, 页面提交前 Number() 收口 */
  modelValue: string | number | undefined
  placeholder?: string
  type?: string
  disabled?: boolean
  /** 尺寸档（规格 §3.6/D⑥）: 与 ManageButton/ManageSelect 同一 --jc-ctrl-* 令牌, 默认 default */
  size?: 'sm' | 'default' | 'lg'
}
const props = withDefaults(defineProps<Props>(), { type: 'text', disabled: false, size: 'default' })
const emit = defineEmits<{ 'update:modelValue': [value: string | number | undefined] }>()

function onInput(e: Event): void {
  const v = (e.target as HTMLInputElement).value
  if (props.type === 'number') {
    // 清空 → emit undefined（原 emit '' 造成 string 空串混入数字字段的隐性类型漂移）;
    // 其它走 Number; NaN 时透传原值避免误转
    if (v === '') {
      emit('update:modelValue', undefined)
      return
    }
    const n = Number(v)
    emit('update:modelValue', Number.isNaN(n) ? v : n)
    return
  }
  emit('update:modelValue', v)
}

/** size 三档 → --jc-ctrl-* 令牌类（§3.6 映射表, 与 ManageButton.sizeClass 同构）;
 *  min-h 保持（input 可增高的输入语义不变, 这里只切高度档） */
const sizeClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'min-h-[var(--jc-ctrl-h-sm)] px-[var(--jc-ctrl-px-sm)] text-[length:var(--jc-ctrl-fs-sm)]'
    case 'lg':
      return 'min-h-[var(--jc-ctrl-h-lg)] px-[var(--jc-ctrl-px-lg)] text-[length:var(--jc-ctrl-fs-lg)]'
    default:
      return 'min-h-[var(--jc-ctrl-h)] px-[var(--jc-ctrl-px)] text-[length:var(--jc-ctrl-fs)]'
  }
})
</script>

<template>
  <input
    :value="props.modelValue ?? ''"
    :type="props.type ?? 'text'"
    :placeholder="props.placeholder"
    :disabled="props.disabled"
      class="jc-field w-full bg-elevated text-primary border border-transparent rounded-[var(--jc-ctrl-radius)] py-[var(--jc-ctrl-py)] outline-none focus:border-ctrl-focus focus:shadow-focus transition disabled:opacity-50"
    :class="sizeClass"
    data-focusable="true"
    @input="onInput"
  />
</template>
