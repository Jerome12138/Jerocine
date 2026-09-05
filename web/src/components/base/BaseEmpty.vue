<script setup lang="ts">
interface Props {
  title?: string
  description?: string
  /** 强制隐藏默认图标（用 slot:icon 替换） */
  hideIcon?: boolean
}

withDefaults(defineProps<Props>(), {
  title: '暂无内容',
  description: '',
  hideIcon: false
})
</script>

<template>
  <div
    class="gf-empty flex flex-col items-center justify-center text-center py-[var(--gf-space-12)] px-[var(--gf-space-4)] gap-[var(--gf-space-3)]"
    role="status"
    aria-live="polite"
  >
    <slot name="icon">
      <BaseIcon
        v-if="!hideIcon"
        name="search"
        size="80px"
        class="text-muted opacity-70"
      />
    </slot>
    <slot name="title">
      <h3
        class="text-[var(--gf-fs-lg)] font-[var(--gf-fw-semibold)] text-primary"
      >
        {{ title }}
      </h3>
    </slot>
    <slot name="description">
      <p
        v-if="description"
        class="text-[var(--gf-fs-md)] text-muted max-w-md"
      >
        {{ description }}
      </p>
    </slot>
    <div v-if="$slots.action" class="mt-[var(--gf-space-4)]">
      <slot name="action" />
    </div>
  </div>
</template>
