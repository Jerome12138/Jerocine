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
    class="jc-filter-bar bg-surface rounded-[var(--jc-radius-lg)] p-[var(--jc-space-2)] md:p-[var(--jc-space-3)] flex flex-col gap-[var(--jc-space-1)] md:gap-[var(--jc-space-2)]"
  >
    <div
      v-for="group in groups"
      :key="group.key"
      class="jc-filter-row flex items-start gap-[var(--jc-space-2)]"
    >
      <!-- 标题与"首行选项"垂直居中: min-height 对齐胶囊高度, 选项换行时标题保持钉在首行 -->
      <div
        class="jc-filter-row__title shrink-0 text-secondary text-[var(--jc-fs-sm)] font-[var(--jc-fw-medium)] flex items-center min-h-[36px]"
      >
        {{ group.title }}
      </div>
      <div
        class="jc-filter-row__chips flex flex-wrap gap-[var(--jc-space-2)] flex-1"
      >
        <button
          v-for="opt in group.options"
          :key="String(opt.value)"
          class="jc-filter-chip"
          :class="
            String(opt.value) === String(group.current)
              ? 'jc-filter-chip--active'
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
/* 样式对齐播放页片源胶囊(jc-source-tab): 无边框 / 透明底 / 选中渐变 / outline 焦点环。
   旧版 transparent 1px border 在部分屏上渲染出"被胶囊截断的内边框"痕迹, 故去掉 border。 */
.jc-filter-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 36px;
  padding: 0 var(--jc-space-3);
  border-radius: var(--jc-chip-radius, 9999px);
  background-color: transparent;
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-medium);
  border: none;
  cursor: pointer;
  white-space: nowrap;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-filter-chip:hover {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--jc-text-primary);
}

.jc-filter-chip--active {
  background-image: var(--jc-brand-gradient);
  color: #fff;
  font-weight: var(--jc-fw-semibold);
}
.jc-filter-chip--active:hover {
  /* 覆盖 hover 半透明白底, 保持渐变(与片源胶囊一致) */
  background-color: transparent;
}

/* 焦点环: 跟随圆角的 outline, 不被祖先 overflow 裁切(与片源胶囊一致)。
   只用 :focus-visible —— 触屏点击/按压不会留下边框环(用户反馈移动端点完胶囊还挂着框),
   键盘(Tab)与 TV 遥控焦点仍可见。 */
.jc-filter-chip:focus-visible {
  outline: 2px solid var(--jc-brand-cyan);
  outline-offset: 2px;
}

@media (max-width: 767px) {
  .jc-filter-row {
    flex-direction: column;
    align-items: stretch;
  }
  .jc-filter-row__title {
    min-height: 0;
  }
  /* 移动端: 胶囊与四周留白整体收一档, 一屏能多放几个选项 */
  .jc-filter-chip {
    height: 30px;
    padding: 0 var(--jc-space-2);
    font-size: var(--jc-fs-xs);
  }
  .jc-filter-row__chips {
    gap: var(--jc-space-1) var(--jc-space-2);
  }
  /* 移动端不保留聚焦边框(触屏无需键盘焦点提示), 选中态靠渐变胶囊表达 */
  .jc-filter-chip:focus,
  .jc-filter-chip:focus-visible {
    outline: none;
  }
}
</style>

<style>
/* TV 模式：放大 chip 高度 + 字号 + 焦点环 */
[data-mode='tv'] .jc-filter-chip {
  height: 44px;
  padding: 0 var(--jc-space-4);
  font-size: var(--jc-fs-base);
}
[data-mode='tv'] .jc-filter-chip:focus,
[data-mode='tv'] .jc-filter-chip:focus-visible {
  outline: 3px solid var(--jc-brand-cyan);
  outline-offset: 2px;
  box-shadow: 0 0 14px rgba(74, 209, 229, 0.4);
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--jc-text-primary);
}
[data-mode='tv'] .jc-filter-chip--active {
  background-color: transparent;
}
[data-mode='tv'] .jc-filter-row__title {
  font-size: var(--jc-fs-base);
}
</style>
