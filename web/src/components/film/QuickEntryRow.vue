<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'

/**
 * 首页金刚区 —— 一排圆角快捷入口(国产电视常见). 仅 TV 模式使用(HomeView v-if isTV)。
 * 5 个入口: 分类 / 历史 / 收藏 / 排行 / 设置(用户拍板)。
 * 注: "排行" 暂无独立排行页, 先指向分类筛选(全部影片浏览), 后续有排行页再改。
 */
interface QuickEntry {
  key: string
  label: string
  icon: string
  to: RouteLocationRaw
}

const entries: QuickEntry[] = [
  { key: 'classify', label: '分类', icon: 'film', to: '/filmClassify' },
  { key: 'history', label: '历史', icon: 'history', to: '/history' },
  { key: 'favorites', label: '收藏', icon: 'heart', to: '/favorites' },
  { key: 'ranking', label: '排行', icon: 'fire', to: '/filmClassifySearch' },
  { key: 'settings', label: '设置', icon: 'settings', to: '/settings' }
]
</script>

<template>
  <nav class="jc-quick container-page" aria-label="快捷入口">
    <div class="jc-quick__row" data-focus-zone="quick">
      <RouterLink
        v-for="e in entries"
        :key="e.key"
        :to="e.to"
        class="jc-quick__item"
        data-focusable="true"
        tabindex="0"
        :aria-label="e.label"
      >
        <span class="jc-quick__icon">
          <BaseIcon :name="e.icon" size="42%" />
        </span>
        <span class="jc-quick__label">{{ e.label }}</span>
      </RouterLink>
    </div>
  </nav>
</template>

<style scoped>
.jc-quick {
  padding-block: var(--jc-space-4);
}
.jc-quick__row {
  display: flex;
  gap: clamp(16px, 3vw, 48px);
  justify-content: flex-start;
  padding-block: 12px;
}
.jc-quick__item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--jc-space-2);
  text-decoration: none;
  color: var(--jc-text-secondary);
  outline: none;
}
.jc-quick__icon {
  width: var(--jc-tv-quick-size, 96px);
  height: var(--jc-tv-quick-size, 96px);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--jc-radius-full);
  background-color: var(--jc-tv-glass-soft, rgba(30, 30, 40, 0.7));
  border: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
  color: var(--jc-brand-cyan);
  transition:
    transform var(--jc-dur-base) var(--jc-ease-spring),
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    box-shadow var(--jc-dur-base) var(--jc-ease-standard);
}
.jc-quick__label {
  font-size: var(--jc-fs-base);
  color: var(--jc-text-secondary);
}
</style>

<style>
/* TV 焦点: 仅放大圆形图标 + 青光晕, 文字静止变亮(整项不放大) */
[data-mode='tv'] .jc-quick__item[data-focusable='true']:focus,
[data-mode='tv'] .jc-quick__item[data-focusable='true']:focus-visible {
  outline: none;
  transform: none;
  box-shadow: none;
}
[data-mode='tv'] .jc-quick__item[data-focusable='true']:focus .jc-quick__icon,
[data-mode='tv'] .jc-quick__item[data-focusable='true']:focus-visible .jc-quick__icon {
  transform: scale(var(--jc-tv-focus-scale-sm, 1.1));
  box-shadow: var(--jc-tv-focus-ring);
  background-color: var(--jc-tv-selected-bg, rgba(74, 209, 229, 0.14));
  z-index: 5;
}
[data-mode='tv'] .jc-quick__item[data-focusable='true']:focus .jc-quick__label,
[data-mode='tv'] .jc-quick__item[data-focusable='true']:focus-visible .jc-quick__label {
  color: var(--jc-text-primary);
}
[data-mode='tv'] .jc-quick.container-page {
  padding-inline: var(--jc-tv-safe);
}
[data-mode='tv'] .jc-quick__label {
  font-size: var(--jc-fs-md);
}
</style>
