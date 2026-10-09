<script setup lang="ts">
import { reloadToLatest } from '@/utils/chunkReload'
import { useTvSettingsEntry } from '@/composables/useTvSettingsEntry'
import BaseIcon from '@/components/base/BaseIcon.vue'

/**
 * TV 顶栏胶囊行最右侧的两个动作按钮（方案 §6）:
 *   刷新 → 「拿最新版本」（不是裸 reload: SW 接管后裸 reload 只会吃 precache 的旧 index.html）
 *   设置 → 打开原生设置抽屉（给没有 MENU 键的遥控器 / 触屏留入口）
 *
 * 只在 TV 模式渲染（由 PublicHeader 用 v-if="isTV" 控制），与频道导航同处一个焦点区，
 * 遥控器左右键即可到达。
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
      <BaseIcon name="refresh" size="18px" />
      <span>刷新</span>
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
      <BaseIcon name="settings" size="18px" />
      <span>设置</span>
    </button>
  </div>
</template>

<!-- 非 scoped: 按钮要参与 [data-mode='tv'] .jc-header__nav 的胶囊排布,
     与 PublicHeader 里的 TV 覆盖规则同表更直观 -->
<style>
.jc-tv-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  /* 胶囊行内的分隔: 短竖线(不是整列全高), 与药丸的圆角协调 */
  align-self: center;
  height: 22px;
  margin-left: 4px;
  padding-left: 10px;
  border-left: 1px solid var(--jc-border-subtle);
}

.jc-tv-actions__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: auto;
  padding: 8px 16px;
  border: none;
  border-radius: 999px;
  background: transparent;
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-base);
  font-family: inherit;
  cursor: pointer;
  white-space: nowrap;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
  outline: none;
}

.jc-tv-actions__btn:hover,
.jc-tv-actions__btn:focus-visible {
  background: rgba(255, 255, 255, 0.18);
  color: var(--jc-text-primary);
}
</style>
