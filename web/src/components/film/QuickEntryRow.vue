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
  <nav class="gf-quick container-page" aria-label="快捷入口">
    <div class="gf-quick__row" data-focus-zone="quick">
      <RouterLink
        v-for="e in entries"
        :key="e.key"
        :to="e.to"
        class="gf-quick__item"
        data-focusable="true"
        tabindex="0"
        :aria-label="e.label"
      >
        <span class="gf-quick__icon">
          <BaseIcon :name="e.icon" size="42%" />
        </span>
        <span class="gf-quick__label">{{ e.label }}</span>
      </RouterLink>
    </div>
  </nav>
</template>

<style scoped>
.gf-quick {
  padding-block: var(--gf-space-4);
}
.gf-quick__row {
  display: flex;
  gap: clamp(16px, 3vw, 48px);
  justify-content: flex-start;
  padding-block: 12px;
}
.gf-quick__item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--gf-space-2);
  text-decoration: none;
  color: var(--gf-text-secondary);
  outline: none;
}
.gf-quick__icon {
  width: var(--gf-tv-quick-size, 96px);
  height: var(--gf-tv-quick-size, 96px);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--gf-radius-full);
  background-color: var(--gf-tv-glass-soft, rgba(30, 30, 40, 0.7));
  border: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
  color: var(--gf-brand-cyan);
  transition:
    transform var(--gf-dur-base) var(--gf-ease-spring),
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    box-shadow var(--gf-dur-base) var(--gf-ease-standard);
}
.gf-quick__label {
  font-size: var(--gf-fs-base);
  color: var(--gf-text-secondary);
}
</style>

<style>
/* TV 焦点: 仅放大圆形图标 + 青光晕, 文字静止变亮(整项不放大) */
[data-mode='tv'] .gf-quick__item[data-focusable='true']:focus,
[data-mode='tv'] .gf-quick__item[data-focusable='true']:focus-visible {
  outline: none;
  transform: none;
  box-shadow: none;
}
[data-mode='tv'] .gf-quick__item[data-focusable='true']:focus .gf-quick__icon,
[data-mode='tv'] .gf-quick__item[data-focusable='true']:focus-visible .gf-quick__icon {
  transform: scale(var(--gf-tv-focus-scale-sm, 1.1));
  box-shadow: var(--gf-tv-focus-ring);
  background-color: var(--gf-tv-selected-bg, rgba(74, 209, 229, 0.14));
  z-index: 5;
}
[data-mode='tv'] .gf-quick__item[data-focusable='true']:focus .gf-quick__label,
[data-mode='tv'] .gf-quick__item[data-focusable='true']:focus-visible .gf-quick__label {
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-quick.container-page {
  padding-inline: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-quick__label {
  font-size: var(--gf-fs-md);
}
</style>
