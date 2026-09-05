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
  <label class="flex flex-col gap-[var(--gf-space-2)]">
    <span class="text-sm text-secondary">
      <span v-if="required" class="text-[var(--gf-danger)] mr-1">*</span>
      {{ label }}
    </span>
    <slot />
    <span v-if="error" class="text-xs text-[var(--gf-danger)]">{{ error }}</span>
    <span v-else-if="hint" class="text-xs text-muted">{{ hint }}</span>
  </label>
</template>
