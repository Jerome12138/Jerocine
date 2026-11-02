<script setup lang="ts">
interface Props {
  /** 数字字段空值约定（复核 #10）: type=number 清空后为 undefined, 页面提交前 Number() 收口 */
  modelValue: string | number | undefined
  placeholder?: string
  type?: string
  disabled?: boolean
}
const props = defineProps<Props>()
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
</script>

<template>
  <input
    :value="props.modelValue ?? ''"
    :type="props.type ?? 'text'"
    :placeholder="props.placeholder"
    :disabled="props.disabled"
    class="w-full min-h-[44px] md:min-h-[36px] bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition disabled:opacity-50"
    data-focusable="true"
    @input="onInput"
  />
</template>
