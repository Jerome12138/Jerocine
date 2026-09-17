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
    class="jc-empty flex flex-col items-center justify-center text-center py-[var(--jc-space-12)] px-[var(--jc-space-4)] gap-[var(--jc-space-3)]"
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
        class="text-[var(--jc-fs-lg)] font-[var(--jc-fw-semibold)] text-primary"
      >
        {{ title }}
      </h3>
    </slot>
    <slot name="description">
      <p
        v-if="description"
        class="text-[var(--jc-fs-md)] text-muted max-w-md"
      >
        {{ description }}
      </p>
    </slot>
    <div v-if="$slots.action" class="mt-[var(--jc-space-4)]">
      <slot name="action" />
    </div>
  </div>
</template>
