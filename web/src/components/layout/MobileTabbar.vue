<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { storeToRefs } from 'pinia'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { useUserStore } from '@/stores/user'

/**
 * 移动端底部 tabbar (bilibili App 风格).
 *
 * - 仅 < md 渲染, 由 CSS media query 控制显隐
 * - 4 tab: 首页 / 分类 / 历史 / 我的
 * - "我的" 未登录 → /login; 登录 → /favorites (我的收藏); 原先误指 /history 与"历史"撞车
 * - active 态用品牌渐变图标 + 文字
 * - tabbar 高度 = --gf-tabbar-height; PublicLayout 给 main 加同高 padding-bottom
 *   避免内容被遮挡
 */

const route = useRoute()
const userStore = useUserStore()
const { isLoggedIn } = storeToRefs(userStore)

interface TabItem {
  key: string
  to: string
  label: string
  icon: string
  /** 哪些 route name / path 视为本 tab active */
  matches: (path: string, name?: string) => boolean
}

const meRoute = computed(() => (isLoggedIn.value ? '/favorites' : '/login'))

const tabs = computed<TabItem[]>(() => [
  {
    key: 'home',
    to: '/index',
    label: '首页',
    icon: 'home',
    matches: (p, n) => p === '/' || p === '/index' || n === 'home'
  },
  {
    key: 'classify',
    to: '/filmClassify?Pid=1',
    label: '分类',
    icon: 'menu',
    matches: (p) => p.startsWith('/filmClassify') || p.startsWith('/search')
  },
  {
    key: 'history',
    to: '/history',
    label: '历史',
    icon: 'history',
    matches: (p) => p.startsWith('/history')
  },
  {
    key: 'me',
    to: meRoute.value,
    label: '我的',
    icon: 'user',
    matches: (p) =>
      p.startsWith('/favorites') ||
      p.startsWith('/login') ||
      p.startsWith('/manage')
  }
])

function isActive(t: TabItem): boolean {
  return t.matches(route.path, route.name as string | undefined)
}
</script>

<template>
  <nav class="gf-tabbar md:hidden" aria-label="底部导航">
    <RouterLink
      v-for="t in tabs"
      :key="t.key"
      :to="t.to"
      class="gf-tabbar__item"
      :class="isActive(t) ? 'gf-tabbar__item--active' : ''"
      data-focusable="true"
      tabindex="0"
    >
      <BaseIcon :name="t.icon" size="22px" class="gf-tabbar__icon" />
      <span class="gf-tabbar__label">{{ t.label }}</span>
    </RouterLink>
  </nav>
</template>

<style scoped>
.gf-tabbar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  height: var(--gf-tabbar-height, 56px);
  background-color: rgba(11, 11, 15, 0.96);
  backdrop-filter: blur(12px);
  border-top: 1px solid var(--gf-border-subtle);
  z-index: 80;
  padding-bottom: env(safe-area-inset-bottom, 0);
}

/* PC / 平板 (≥768px) 强制隐藏底部 tabbar, 兜底 unocss md:hidden */
@media (min-width: 768px) {
  .gf-tabbar {
    display: none !important;
  }
}

.gf-tabbar__item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  color: var(--gf-text-muted);
  text-decoration: none;
  font-size: var(--gf-fs-xs);
  outline: none;
  transition: color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-tabbar__item:focus-visible {
  background-color: rgba(255, 255, 255, 0.05);
}

.gf-tabbar__item--active {
  color: var(--gf-brand-cyan);
}
.gf-tabbar__item--active .gf-tabbar__icon {
  filter: drop-shadow(0 0 4px rgba(74, 209, 229, 0.5));
}

.gf-tabbar__icon {
  display: block;
}
.gf-tabbar__label {
  line-height: 1;
}
</style>
