<script setup lang="ts">
import { computed } from 'vue'
import BaseIcon from './BaseIcon.vue'
import { usePwaUpdate } from '@/composables/usePwaUpdate'

/**
 * 新版本提示 (方案 §4.3, 2026-10-27 增强).
 *
 * 两个状态(见 usePwaUpdate):
 *  - updateFound: 检测到新版本开始下载 —— 徽标立即出现(不等预取完成);
 *    点击 = 清旧缓存 + 刷新, 直接走网络拿新版(不等漫长预取)。
 *  - updateReady: 新 SW 已接管当前页面 —— 点击直接刷新切换。
 * 不点击也不打扰, 下次导航/启动本就是新版本。
 *
 * 放置: 顶栏 brand(标题)右侧(web/TV 共用) + 移动端抽屉菜单项(见 PublicHeader)。
 * 遥控器: `<button>` + data-focusable/tabindex ⇒ D-pad 可聚焦, OK 键触发 click。
 */

const { updateFound, updateReady, applyUpdate } = usePwaUpdate()
const visible = computed(() => updateFound.value || updateReady.value)
const text = computed(() => (updateReady.value ? '新版本' : '发现更新'))
const tip = computed(() =>
  updateReady.value ? '新版本已就绪，点击刷新' : '发现新版本，点击立即更新'
)
</script>

<template>
  <Transition name="jc-pwa-badge">
    <button
      v-if="visible"
      type="button"
      class="jc-pwa-badge"
      :class="{ 'is-found': !updateReady }"
      data-focusable="true"
      tabindex="0"
      :aria-label="tip"
      :title="tip"
      @click="applyUpdate"
    >
      <BaseIcon name="refresh" size="16px" />
      <span class="jc-pwa-badge__text">{{ text }}</span>
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

/* 发现更新(预取未完成)态: 琥珀色区分 —— 点击会清缓存强制切新版 */
.jc-pwa-badge.is-found {
  background-color: rgba(245, 158, 11, 0.14);
  border-color: rgba(245, 158, 11, 0.5);
  color: #f59e0b;
}
.jc-pwa-badge.is-found:hover,
.jc-pwa-badge.is-found:focus-visible {
  background-color: rgba(245, 158, 11, 0.24);
  border-color: rgba(245, 158, 11, 0.8);
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
