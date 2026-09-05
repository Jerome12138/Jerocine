<script setup lang="ts">
interface Props {
  modelValue: string | number
  placeholder?: string
  type?: string
  disabled?: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'update:modelValue': [value: string | number] }>()

function onInput(e: Event): void {
  const v = (e.target as HTMLInputElement).value
  if (props.type === 'number') {
    // 空字符串保持空，不强转 0；其它走 Number；NaN 时透传原值避免误转
    if (v === '') {
      emit('update:modelValue', '')
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
    :value="props.modelValue"
    :type="props.type ?? 'text'"
    :placeholder="props.placeholder"
    :disabled="props.disabled"
    class="w-full min-h-[44px] md:min-h-[36px] bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-4)] py-[var(--gf-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition disabled:opacity-50"
    data-focusable="true"
    @input="onInput"
  />
</template>
