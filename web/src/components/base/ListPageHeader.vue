<script setup lang="ts">
import BaseTag from './BaseTag.vue'

/**
 * 列表页头(W2c): FavoritesView / HistoryView 桌面页头的整块收编 ——
 * 标题 + 来源 Tag(云端 purple / 本地 default) + 来源说明 + 计数 + 同步中 + 登录链接。
 * .jc-link-btn 视觉(原两页逐字副本)随本件收敛。
 *
 * 右侧差异由 #actions 插槽承载(收藏页 = 裸登录链接; 历史页 = flex 包装层内
 * 登录链接 + 清空按钮, 包装层恒渲染与原模板一致):
 * - 未提供插槽: 未登录 → header 直接子级裸 <a class="jc-link-btn">;
 * - 提供插槽:   恒渲染 <div class="flex items-center gap-…"> 包 登录链接(未登录时) + 插槽。
 * DOM 类名与结构与两页原模板逐字一致; TV 变体(两页结构互异, 无公共 DOM 词汇)
 * 不在本件范围, 原样留在视图。
 */
defineProps<{
  title: string
  sourceLabel: string
  /** 云端 → purple「云端」; 本地 → default「本地」 */
  remoteMode: boolean
  count: number
  /** 计数量词: 收藏页「部」/ 历史页「条」 */
  unit: string
  remoteLoading: boolean
  /** 已登录时不渲染登录链接 */
  loggedIn: boolean
}>()
</script>

<template>
  <header class="flex items-center justify-between mb-[var(--jc-space-5)] flex-wrap gap-[var(--jc-space-3)]">
    <div>
      <h1 class="text-[length:var(--jc-fs-2xl)] font-[var(--jc-fw-bold)]">{{ title }}</h1>
      <p class="text-sm text-muted mt-[var(--jc-space-1)] flex items-center gap-[var(--jc-space-2)] flex-wrap">
        <BaseTag :variant="remoteMode ? 'purple' : 'default'" size="xs">
          {{ remoteMode ? '云端' : '本地' }}
        </BaseTag>
        <span>{{ sourceLabel }}</span>
        <span>·</span>
        <span>共 {{ count }} {{ unit }}</span>
        <span v-if="remoteLoading" class="text-link">同步中…</span>
      </p>
    </div>
    <div v-if="$slots.actions" class="flex items-center gap-[var(--jc-space-2)]">
      <RouterLink
        v-if="!loggedIn"
        to="/login"
        class="jc-link-btn"
      >
        登录以云端同步
      </RouterLink>
      <slot name="actions" />
    </div>
    <RouterLink
      v-else-if="!loggedIn"
      to="/login"
      class="jc-link-btn"
    >
      登录以云端同步
    </RouterLink>
  </header>
</template>

<style scoped>
/* 自 FavoritesView.vue 逐字迁入(与 HistoryView 的副本逐字相同, diff 已核对) */
.jc-link-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: var(--jc-ctrl-h); /* 2026-11-05 三档统一: 原 6px/12px 内距撑出的 ≈34 作废 */
  padding: 0 var(--jc-ctrl-px);
  border-radius: var(--jc-radius-sm);
  background-color: rgba(155, 73, 231, 0.16);
  color: var(--jc-text-link);
  font-size: var(--jc-ctrl-fs);
  text-decoration: none;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-link-btn:hover,
.jc-link-btn:focus-visible {
  background-color: rgba(155, 73, 231, 0.28);
  outline: none;
}
</style>
