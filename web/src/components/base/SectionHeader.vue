<script setup lang="ts">
import BaseIcon from './BaseIcon.vue'

/**
 * 桌面区块标题(W1e): h2 + 右上「更多 ›」。TV 版 jc-tv-sec 不在本件范围(规格后置)。
 * - 默认类 = ClassifyView 桌面同款(FilmRow 的 header/h2/链接与它逐字相同):
 *   header flex items-end justify-between gap-4 mb-3 / h2 fs-lg 粗体 primary snug /
 *   更多链接 text-link fs-sm + data-focusable + tabindex=0 + chevron-right 16px。
 * - headerClass / titleClass: 提供 = **整组替换**默认类 —— 调用方自带完整视觉词汇
 *   (FilmRow/继续观看行/猜你喜欢 的页面钩子类必须逐字保留, TV 字号/安全区覆盖依赖
 *   全局选择器命中这些类); 缺省 = 组件默认类。
 * - 默认插槽: 右侧内容整体替换「更多›」链接(继续观看行带 hover 的链接 / 猜你喜欢 tip
 *   等差异形态); 插槽内容在调用方作用域编译, 页面 scoped 样式(如 hover)继续生效。
 * - bare: 只渲染裸 h2(RelatedList 形态, 无 header 包装与右侧区)。
 */
withDefaults(
  defineProps<{
    title?: string
    /** 「更多 ›」链接目标(RouterLink to, 字符串或位置对象, null=不渲染); 有值且未提供插槽时渲染默认链接 */
    moreTo?: string | { path: string; query?: Record<string, string | number> } | null
    /** header 整组替换类 */
    headerClass?: string
    /** h2 整组替换类 */
    titleClass?: string
    /** 裸 h2 形态 */
    bare?: boolean
  }>(),
  { title: '', moreTo: '', headerClass: '', titleClass: '', bare: false }
)
</script>

<template>
  <h2
    v-if="bare"
    class="text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)] text-primary leading-[var(--jc-lh-snug)]"
    :class="titleClass"
  >{{ title }}</h2>
  <header
    v-else
    :class="headerClass || 'flex items-end justify-between gap-[var(--jc-space-4)] mb-[var(--jc-space-3)]'"
  >
    <h2
      v-if="title"
      :class="titleClass || 'text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)] text-primary leading-[var(--jc-lh-snug)]'"
    >{{ title }}</h2>
    <slot>
      <RouterLink
        v-if="moreTo"
        :to="moreTo"
        class="text-link text-[length:var(--jc-fs-sm)] inline-flex items-center gap-[var(--jc-space-1)] shrink-0"
        data-focusable="true"
        tabindex="0"
      >
        更多
        <BaseIcon name="chevron-right" size="16px" />
      </RouterLink>
    </slot>
  </header>
</template>
