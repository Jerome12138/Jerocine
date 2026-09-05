<script setup lang="ts">
export interface FilterOption {
  value: string | number
  label: string
}

export interface FilterGroup {
  key: string
  title: string
  options: FilterOption[]
  current: string | number
}

interface Props {
  groups: FilterGroup[]
}

defineProps<Props>()

const emit = defineEmits<{
  (e: 'change', payload: { key: string; value: string | number }): void
}>()

function pick(key: string, value: string | number): void {
  emit('change', { key, value })
}
</script>

<template>
  <section
    class="gf-filter-bar bg-surface rounded-[var(--gf-radius-lg)] p-[var(--gf-space-4)] flex flex-col gap-[var(--gf-space-3)]"
  >
    <div
      v-for="group in groups"
      :key="group.key"
      class="gf-filter-row flex items-start gap-[var(--gf-space-3)]"
    >
      <div
        class="gf-filter-row__title shrink-0 text-secondary text-[var(--gf-fs-sm)] font-[var(--gf-fw-medium)] pt-[6px]"
      >
        {{ group.title }}
      </div>
      <div
        class="gf-filter-row__chips flex flex-wrap gap-[var(--gf-space-2)] flex-1"
      >
        <button
          v-for="opt in group.options"
          :key="String(opt.value)"
          class="gf-filter-chip"
          :class="
            String(opt.value) === String(group.current)
              ? 'gf-filter-chip--active'
              : ''
          "
          data-focusable="true"
          tabindex="0"
          :aria-pressed="String(opt.value) === String(group.current)"
          @click="pick(group.key, opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.gf-filter-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 44px;
  padding: 0 var(--gf-space-4);
  border-radius: var(--gf-radius-full);
  background-color: transparent;
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  border: 1px solid transparent;
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-filter-chip:hover {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--gf-text-primary);
}

.gf-filter-chip--active {
  background-image: var(--gf-brand-gradient);
  color: #fff;
  font-weight: var(--gf-fw-semibold);
  box-shadow: var(--gf-shadow-purple-glow);
}

/* 聚焦用跟随圆角的 outline 环, 替掉 box-shadow(易被父级 overflow 裁掉, 显得"边框被截断") */
.gf-filter-chip:focus,
.gf-filter-chip:focus-visible {
  outline: 2px solid var(--gf-brand-cyan);
  outline-offset: 2px;
}

@media (max-width: 767px) {
  .gf-filter-row {
    flex-direction: column;
    align-items: stretch;
  }
  .gf-filter-row__title {
    padding-top: 0;
  }
}
</style>

<style>
/* TV 模式：放大 chip 高度 + 字号 + 焦点环 */
[data-mode='tv'] .gf-filter-chip {
  height: 44px;
  padding: 0 var(--gf-space-4);
  font-size: var(--gf-fs-base);
}
[data-mode='tv'] .gf-filter-chip:focus,
[data-mode='tv'] .gf-filter-chip:focus-visible {
  outline: 3px solid var(--gf-brand-cyan);
  outline-offset: 2px;
  box-shadow: 0 0 14px rgba(74, 209, 229, 0.4);
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-filter-row__title {
  font-size: var(--gf-fs-base);
}
</style>
