<script setup lang="ts">
interface Props {
  modelValue: string
  placeholder?: string
  rows?: number
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
    class="w-full min-h-[88px] bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-4)] py-[var(--gf-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition resize-y"
    data-focusable="true"
    @input="onInput"
  />
</template>
