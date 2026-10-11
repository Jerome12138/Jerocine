<script setup lang="ts">
import { onMounted } from 'vue'
import PublicHeader from './PublicHeader.vue'
import PublicFooter from './PublicFooter.vue'
import MobileTabbar from './MobileTabbar.vue'
import BackToTop from './BackToTop.vue'
import RouteProgress from './RouteProgress.vue'
import { useViewMode } from '@/composables/useViewMode'
import { useUserStore } from '@/stores/user'

/**
 * 用户端布局壳
 * mobile / desktop / tv 三档共享同一壳
 * 字号 / 间距 / 焦点环差异由 theme.css 内 [data-mode] 段处理
 */
const { mode } = useViewMode()
const userStore = useUserStore()

// 有 token 但 info 未拉 (刷新页 / 直接访问 / 管理员从 /manage 跳回 /index) → fetchInfo
// 否则 isAdmin/displayName 等计算属性会取 stale 值, admin 错显示成"普通用户"
onMounted(async () => {
  if (userStore.isLoggedIn && !userStore.info) {
    try {
      await userStore.fetchInfo()
    } catch {
      // 拦截器会处理 401 (清 token + 跳 /login)
    }
  }
})
</script>

<template>
  <div
    class="jc-public-layout"
    :data-mode="mode"
  >
    <RouteProgress />
    <PublicHeader />
    <main class="jc-public-layout__main">
      <slot />
    </main>
    <!-- TV 模式不显示页脚(遥控器无交互价值, 且占据焦点行) -->
    <PublicFooter v-if="mode !== 'tv'" />
    <!-- 移动端底部 tabbar (>= md 自身 hidden) -->
    <MobileTabbar />
    <!-- 滚动 > 600px 后浮出的回顶 FAB -->
    <BackToTop />
  </div>
</template>

<style scoped>
.jc-public-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: var(--jc-bg-base);
  color: var(--jc-text-primary);
}

.jc-public-layout__main {
  flex: 1 0 auto;
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - 56px - 220px);
  /* 移动端 tabbar fixed 在底部, 留出 padding 不让最后一屏内容被压住 */
  padding-bottom: calc(var(--jc-tabbar-height, 56px) + env(safe-area-inset-bottom, 0));
}

@media (min-width: 768px) {
  .jc-public-layout__main {
    min-height: calc(100vh - 64px - 200px);
    padding-bottom: 0; /* md+ tabbar 隐藏, 不需要留位 */
  }
}
</style>

<style>
/* ============ 宽屏左侧导航栏联动(2026-10-10 用户定稿) ============
 * 侧栏(PublicHeader 的 jc-mnav--side, 260px 栏 + 16px 间距 = 276px)出现时,
 * 内容不再保持"两侧对称大留白": 布局整体右移 --jc-side-pad, 右侧自适应
 * 填满剩余空间(容器 1600 封顶后右侧才重新出现自然留白)。
 * 断点取 1600 = 视口到达容器最大宽度量级; 再低内容区会被压得过窄。
 * header/footer 用"负 margin + 等值 padding"把背景拉回全幅, 其内部
 * container-page 与 main 的内容区保持同一中心线。
 * 轮播 breakout 图在 HeroCarousel 里按 --jc-side-pad/2 反向平移,
 * 保证 100vw 图层仍正对视口居中。 */
/* 变量必须定义在 html 层: 侧栏(teleport 到 body, 非 .jc-public-layout 后代)
 * 也要取到 --jc-side-pad —— 侧栏宽度=右侧留白, 不再 260+16 错位出空白条 */
html {
  --jc-side-pad: 0px;
}
@media (min-width: 1600px) {
  html:not([data-mode='tv']) {
    --jc-side-pad: 276px;
  }
  html:not([data-mode='tv']) .jc-public-layout {
    padding-left: var(--jc-side-pad);
  }
  /* footer: 左右各负 margin 拉回全幅背景, padding-left 让内容跟随内容区中心线 */
  html:not([data-mode='tv']) .jc-public-layout .jc-footer {
    margin-left: calc(-1 * var(--jc-side-pad));
    margin-right: calc(-1 * var(--jc-side-pad));
    padding-left: var(--jc-side-pad);
  }
  /* 顶栏(2026-10-11 用户定稿): 按布局内容区(侧栏右侧)宽度, 不覆盖侧栏区域
   * (全幅玻璃方案已否); 标题与主导航在侧栏出现时隐藏(导航职责移交侧栏) */
  html:not([data-mode='tv']) .jc-public-layout .jc-header .jc-header__brand,
  html:not([data-mode='tv']) .jc-public-layout .jc-header .jc-header__nav {
    display: none;
  }
  /* 顶栏内容铺满右侧区域(压掉 container-page 的 max-width, 保留 gutter) */
  html:not([data-mode='tv']) .jc-public-layout .jc-header .jc-header__inner {
    max-width: none;
  }
}

/* TV 安全区 + 居中容器 */
[data-mode='tv'] .jc-public-layout {
  /* layout 自身保持流式（背景需要全屏），仅 main 居中 */
}
[data-mode='tv'] .jc-public-layout__main {
  min-height: calc(100vh - 96px - 200px);
}
/* TV 模式下 .container-page 走 1600 居中（已由 theme.css 内 --jc-container-max-2xl 控制） */
[data-mode='tv'] .container-page {
  max-width: var(--jc-container-max-2xl);
  margin-inline: auto;
  padding-inline: var(--jc-tv-safe);
}
</style>
