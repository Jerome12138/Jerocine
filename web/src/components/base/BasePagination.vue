<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  current: number
  pageSize: number
  total: number
  /** 桌面态显示页码数（奇数），默认 7 */
  pagerCount?: number
}

const props = withDefaults(defineProps<Props>(), {
  pagerCount: 7
})

const emit = defineEmits<{
  (e: 'change', page: number): void
  (e: 'update:current', page: number): void
}>()

const totalPages = computed(() =>
  Math.max(1, Math.ceil(props.total / Math.max(1, props.pageSize)))
)

const pages = computed<Array<number | '...'>>(() => {
  const total = totalPages.value
  const cur = clamp(props.current, 1, total)
  const max = Math.max(5, props.pagerCount)
  if (total <= max) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }
  const half = Math.floor((max - 2) / 2) // 减去首尾
  const list: Array<number | '...'> = [1]
  let left = Math.max(2, cur - half)
  let right = Math.min(total - 1, cur + half)
  if (cur - 1 <= half) {
    right = max - 2
  }
  if (total - cur <= half) {
    left = total - (max - 3)
  }
  if (left > 2) {
    list.push('...')
  }
  for (let i = left; i <= right; i++) {
    list.push(i)
  }
  if (right < total - 1) {
    list.push('...')
  }
  list.push(total)
  return list
})

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max)
}

function go(page: number): void {
  const next = clamp(page, 1, totalPages.value)
  if (next === props.current) return
  emit('update:current', next)
  emit('change', next)
}
</script>

<template>
  <nav
    class="gf-pagination flex items-center justify-center flex-wrap gap-[var(--gf-space-2)]"
    role="navigation"
    aria-label="pagination"
  >
    <!-- prev -->
    <button
      class="gf-page-chip flex items-center justify-center"
      :class="current <= 1 ? 'opacity-30 pointer-events-none' : ''"
      :disabled="current <= 1"
      :data-focusable="current > 1 ? 'true' : undefined"
      :tabindex="current > 1 ? 0 : -1"
      aria-label="prev page"
      @click="go(current - 1)"
    >
      <BaseIcon name="chevron-left" size="18px" />
    </button>

    <!-- 移动端简化：仅显示当前/总 -->
    <span class="gf-page-current md:hidden text-secondary text-sm">
      {{ current }} / {{ totalPages }}
    </span>

    <!-- 桌面端页码 -->
    <template v-for="(p, idx) in pages" :key="idx + '-' + p">
      <span
        v-if="p === '...'"
        class="gf-page-ellipsis hidden md:inline-flex items-center justify-center text-muted"
        >…</span
      >
      <button
        v-else
        class="gf-page-chip hidden md:inline-flex items-center justify-center"
        :class="
          p === current
            ? 'gf-page-chip--active'
            : 'text-secondary hover:bg-[rgba(255,255,255,0.1)] hover:text-primary'
        "
        :data-focusable="p === current ? undefined : 'true'"
        :tabindex="p === current ? -1 : 0"
        :aria-current="p === current ? 'page' : undefined"
        @click="go(p as number)"
      >
        {{ p }}
      </button>
    </template>

    <!-- next -->
    <button
      class="gf-page-chip flex items-center justify-center"
      :class="current >= totalPages ? 'opacity-30 pointer-events-none' : ''"
      :disabled="current >= totalPages"
      :data-focusable="current < totalPages ? 'true' : undefined"
      :tabindex="current < totalPages ? 0 : -1"
      aria-label="next page"
      @click="go(current + 1)"
    >
      <BaseIcon name="chevron-right" size="18px" />
    </button>
  </nav>
</template>

<style scoped>
.gf-page-chip {
  min-width: 44px;
  height: 44px;
  padding: 0 var(--gf-space-3);
  border-radius: var(--gf-radius-full);
  background-color: var(--gf-bg-elevated);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-semibold);
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard),
    transform var(--gf-dur-fast) var(--gf-ease-standard);
  border: none;
  color: var(--gf-text-secondary);
}

.gf-page-chip--active {
  background-image: var(--gf-brand-gradient);
  color: #fff;
  box-shadow: var(--gf-shadow-purple-glow);
}

.gf-page-ellipsis {
  min-width: 24px;
  height: 44px;
  font-size: var(--gf-fs-sm);
}
</style>

<style>
/* TV 模式：放大页码 chip + 焦点环 */
[data-mode='tv'] .gf-page-chip {
  min-width: 56px;
  height: 56px;
  padding: 0 var(--gf-space-4);
  font-size: var(--gf-fs-base);
}
[data-mode='tv'] .gf-page-chip:focus,
[data-mode='tv'] .gf-page-chip:focus-visible {
  outline: none;
  box-shadow: var(--gf-tv-focus-ring);
  background-color: rgba(255, 255, 255, 0.12);
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-page-ellipsis {
  height: 56px;
  font-size: var(--gf-fs-base);
}
</style>
