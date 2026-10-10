<script setup lang="ts">
import { reloadToLatest } from '@/utils/chunkReload'
import { useTvSettingsEntry } from '@/composables/useTvSettingsEntry'
import BaseIcon from '@/components/base/BaseIcon.vue'

/**
 * 顶栏最右侧的两个动作按钮（方案 §6；2026-10-10 实测反馈修订）。
 *   刷新 → 「拿最新版本」（不是裸 reload: SW 接管后裸 reload 只会吃 precache 的旧 index.html）
 *   设置 → 打开原生设置抽屉（给没有 MENU 键的遥控器 / 触屏留入口）
 *
 * 反馈修订点：
 *   1. **只画图标**，不再带「刷新 / 设置」文字 —— TV 视距下文字既占宽又冗余，
 *      可访问性交给 aria-label / title；
 *   2. **钉在最右**：由 PublicHeader 把它放在 header inner 的**最后一个 flex 子项** +
 *      两侧 flex-1 spacer 顶到容器右缘（原先它挂在居中的胶囊导航里，位置随胶囊走）；
 *   3/4. **显隐交给父级** `shouldShowHeaderActions()` —— TV 模式恒显示；非 TV 模式只有
 *      「在原生壳里且能调起设置抽屉」才显示。桌面布局下首页金刚区的「设置」卡不渲染
 *      （v-if="isTV"），顶栏是用户唯一的回退入口，藏掉它用户切到桌面模式后就切不回 TV 了。
 */

/** 设置入口语义(原生壳开抽屉 / 纯网页走 SPA)与首页金刚区共用同一份判断 */
const { open: openSettings } = useTvSettingsEntry()

function track(action: string): void {
  void import('@/utils/telemetry')
    .then(({ telemetry }) => {
      telemetry.track('action', { category: 'tv-capsule', action })
    })
    .catch(() => {})
}

/** 刷新到最新版本：清闸门 → SW update（拿新 manifest）→ reload */
function onRefresh(): void {
  track('refresh-latest')
  reloadToLatest({
    guardStore: sessionStorage,
    reload: () => window.location.reload(),
    getRegistration: () => navigator.serviceWorker?.getRegistration(),
    setTimer: (fn, ms) => window.setTimeout(fn, ms)
  })
}

function onOpenSettings(): void {
  track('open-settings')
  openSettings()
}
</script>

<template>
  <div class="jc-tv-actions">
    <button
      type="button"
      class="jc-tv-actions__btn"
      aria-label="刷新到最新版本"
      title="刷新到最新版本"
      data-focusable="true"
      tabindex="0"
      @click="onRefresh"
    >
      <BaseIcon name="refresh" size="20px" />
    </button>
    <button
      type="button"
      class="jc-tv-actions__btn"
      aria-label="打开设置"
      title="打开设置"
      data-focusable="true"
      tabindex="0"
      @click="onOpenSettings"
    >
      <BaseIcon name="settings" size="20px" />
    </button>
  </div>
</template>

<!-- 非 scoped: 按钮要参与 [data-mode='tv'] 的全局顶栏排布, 与 PublicHeader 里的 TV 覆盖同表更直观 -->
<style>
/* 顶栏最右的两个动作按钮 —— 只图标。定位靠 PublicHeader 把它放在 inner 的最后一个
   flex 子项(前面有两个 flex-1 spacer 把它顶到右缘), **不用 absolute** ——
   absolute 在桌面布局下会直接压到右侧的观看历史/用户头像上。 */
.jc-tv-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
}

.jc-tv-actions__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  padding: 0;
  border: none;
  border-radius: var(--jc-radius-full);
  background: transparent;
  color: var(--jc-text-secondary);
  cursor: pointer;
  outline: none;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-tv-actions__btn:hover,
.jc-tv-actions__btn:focus-visible {
  background: rgba(255, 255, 255, 0.18);
  color: var(--jc-text-primary);
}

/* TV: 与居中胶囊导航(PublicHeader [data-mode='tv'] .jc-header__nav)同款玻璃药丸,
   两枚图标按钮排成右侧小胶囊 —— 视觉上与导航呼应, 又明确是"钉住"的独立控件。 */
[data-mode='tv'] .jc-tv-actions {
  gap: 4px;
  padding: 5px;
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid var(--jc-border-subtle);
  border-radius: 999px;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}
[data-mode='tv'] .jc-tv-actions__btn {
  width: 48px;
  height: 48px;
  color: var(--jc-text-secondary);
}
/* 焦点环与 .jc-header__icon-btn 一致: 用 outline(不被 header 上下边界裁切), 
   并压掉全局 TV focus 的 scale(1.08) —— 右缘按钮放大后会顶出容器。 */
[data-mode='tv'] .jc-tv-actions__btn:focus,
[data-mode='tv'] .jc-tv-actions__btn:focus-visible {
  outline: 2px solid var(--jc-brand-cyan);
  outline-offset: 1px;
  box-shadow: 0 0 12px rgba(74, 209, 229, 0.4);
  background-color: rgba(255, 255, 255, 0.12);
  color: var(--jc-text-primary);
  transform: none;
}
</style>
