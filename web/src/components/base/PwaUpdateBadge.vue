<script setup lang="ts">
import { computed } from 'vue'
import BaseIcon from './BaseIcon.vue'
import { usePwaUpdate } from '@/composables/usePwaUpdate'

/**
 * 新版本提示 (方案 §4.3, 2026-10-27 增强).
 *
 * 三个状态(见 usePwaUpdate):
 *  - updateFound: 检测到新版本(构建号确实不同) —— 徽标出现, 点击开始切换;
 *  - applying: 已点击, 正在等新 SW 接管 —— 转"更新中"并禁止重复点击;
 *  - updateReady: 新 SW 已接管当前页面 —— 点击直接刷新切换。
 * 不点击也不打扰, 下次导航/启动本就是新版本。
 * 同一构建号一个会话只提示一次(去重在 composable 里), 不会反复弹。
 *
 * 放置: 顶栏 brand(标题)右侧(web/TV 共用) + 移动端抽屉菜单项(见 PublicHeader)。
 * 遥控器: `<button>` + data-focusable/tabindex ⇒ D-pad 可聚焦, OK 键触发 click。
 */

const { updateFound, updateReady, applying, applyUpdate } = usePwaUpdate()
const visible = computed(() => updateFound.value || updateReady.value || applying.value)
const text = computed(() =>
  applying.value ? '更新中' : updateReady.value ? '新版本' : '发现更新'
)
const tip = computed(() => {
  if (applying.value) return '正在切换到新版本，请稍候'
  return updateReady.value ? '新版本已就绪，点击刷新' : '发现新版本，点击立即更新'
})
</script>

<template>
  <Transition name="jc-pwa-badge">
    <button
      v-if="visible"
      type="button"
      class="jc-pwa-badge"
      :class="{ 'is-found': !updateReady && !applying }"
      data-focusable="true"
      tabindex="0"
      :disabled="applying"
      :aria-busy="applying ? 'true' : undefined"
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

.jc-pwa-badge:disabled {
  cursor: default;
  opacity: 0.75;
}

/* 发现更新(预取未完成)态: 琥珀色区分 —— 点击会等待新 SW 接管后切版 */
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
