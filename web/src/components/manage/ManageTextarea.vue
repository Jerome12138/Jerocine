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
    class="w-full min-h-[calc(var(--jc-ctrl-h)*2.4)] bg-elevated text-primary border border-default rounded-[var(--jc-ctrl-radius)] px-[var(--jc-ctrl-px)] py-[var(--jc-space-2)] text-[length:var(--jc-ctrl-fs)] outline-none focus:border-strong focus:shadow-focus transition resize-y disabled:opacity-50"
    data-focusable="true"
    @input="onInput"
  />
</template>
