<script setup lang="ts">
import { useRoute } from 'vue-router'

interface Props {
  title: string
  /** 可选辅助说明 */
  description?: string
}

defineProps<Props>()

const route = useRoute()
</script>

<template>
  <section
    class="container-page py-[var(--jc-space-12)] flex flex-col gap-[var(--jc-space-4)]"
  >
    <h1 class="text-[var(--jc-fs-2xl)] font-[var(--jc-fw-bold)] text-brand-gradient">
      {{ title }}
    </h1>
    <p v-if="description" class="text-secondary">{{ description }}</p>
    <div
      class="card p-[var(--jc-space-5)] text-sm text-muted font-[var(--jc-font-mono)]"
    >
      <div>当前路径：{{ route.fullPath }}</div>
      <div v-if="Object.keys(route.query).length">
        Query：{{ JSON.stringify(route.query) }}
      </div>
    </div>
    <slot />
  </section>
</template>
