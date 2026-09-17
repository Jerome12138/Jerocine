<script setup lang="ts">
interface Props {
  label: string
  required?: boolean
  hint?: string
  error?: string
}
defineProps<Props>()
</script>

<template>
  <label class="flex flex-col gap-[var(--jc-space-2)]">
    <span class="text-sm text-secondary">
      <span v-if="required" class="text-[var(--jc-danger)] mr-1">*</span>
      {{ label }}
    </span>
    <slot />
    <span v-if="error" class="text-xs text-[var(--jc-danger)]">{{ error }}</span>
    <span v-else-if="hint" class="text-xs text-muted">{{ hint }}</span>
  </label>
</template>
