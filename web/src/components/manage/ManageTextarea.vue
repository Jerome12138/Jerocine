<script setup lang="ts">
interface Props {
  modelValue: string
  placeholder?: string
  rows?: number
  /** 原生 maxlength（如 255, RegisterApplyListView 备注场景） */
  maxlength?: number | string
  disabled?: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onInput(e: Event): void {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
}
</script>

<template>
  <textarea
    :value="props.modelValue"
    :placeholder="props.placeholder"
    :rows="props.rows ?? 4"
    :maxlength="props.maxlength"
    :disabled="props.disabled"
    class="w-full min-h-[88px] bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition resize-y disabled:opacity-50"
    data-focusable="true"
    @input="onInput"
  />
</template>
