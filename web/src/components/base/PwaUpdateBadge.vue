<script setup lang="ts">
import BaseIcon from './BaseIcon.vue'
import { usePwaUpdate } from '@/composables/usePwaUpdate'

/**
 * 新版本就绪提示 (方案 §4.3).
 *
 * 由 SW 的 `controllerchange`(见 usePwaUpdate) 驱动: 新版本已接管当前页面时出现,
 * 点击立即刷新切换; 不点击也不打扰, 下次导航/启动本就是新版本。
 *
 * 放置: 顶栏 brand(标题)右侧 —— TV 模式下顶栏 logo 被隐藏, 这里自然落在左上角。
 * 遥控器: `<button>` + data-focusable/tabindex ⇒ D-pad 可聚焦, OK 键触发 click。
 */

const { updateReady, applyUpdate } = usePwaUpdate()
</script>

<template>
  <Transition name="jc-pwa-badge">
    <button
      v-if="updateReady"
      type="button"
      class="jc-pwa-badge"
      data-focusable="true"
      tabindex="0"
      aria-label="新版本已就绪，点击刷新"
      title="新版本已就绪，点击刷新"
      @click="applyUpdate"
    >
      <BaseIcon name="refresh" size="16px" />
      <span class="jc-pwa-badge__text">新版本</span>
    </button>
  </Transition>
</template>

<style scoped>
.jc-pwa-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
  height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  background-color: rgba(74, 209, 229, 0.14);
  border: 1px solid rgba(74, 209, 229, 0.5);
  color: #4ad1e5;
  font-size: var(--jc-fs-xs);
  font-weight: var(--jc-fw-medium);
  white-space: nowrap;
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    border-color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-pwa-badge:hover,
.jc-pwa-badge:focus-visible {
  background-color: rgba(74, 209, 229, 0.24);
  border-color: rgba(74, 209, 229, 0.8);
  outline: none;
}

/* TV: 字号随主题放大, 行高加大便于远观与聚焦 */
[data-mode='tv'] .jc-pwa-badge {
  height: 40px;
  padding: 0 16px;
  font-size: var(--jc-fs-sm);
}

.jc-pwa-badge-enter-active,
.jc-pwa-badge-leave-active {
  transition:
    opacity var(--jc-dur-fast) var(--jc-ease-standard),
    transform var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-pwa-badge-enter-from,
.jc-pwa-badge-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
