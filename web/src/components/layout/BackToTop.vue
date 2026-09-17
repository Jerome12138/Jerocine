<script setup lang="ts">
/**
 * 回顶悬浮按钮 (bilibili / 腾讯视频右下角 FAB)
 *
 * - 滚动 > 600px 时浮出, 否则隐藏
 * - 点击 → window.scrollTo({ top: 0, behavior: 'smooth' })
 * - 移动端 tabbar fixed 在底部, 自动加 56px + safe-area 间距
 * - 桌面 hover 上浮 + 阴影加深
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'

const visible = ref(false)
const THRESHOLD = 600

function onScroll(): void {
  visible.value = (window.scrollY ?? 0) > THRESHOLD
}

function backToTop(): void {
  const prefersReduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  window.scrollTo({ top: 0, behavior: prefersReduced ? 'auto' : 'smooth' })
}

onMounted(() => {
  onScroll()
  window.addEventListener('scroll', onScroll, { passive: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
})
</script>

<template>
  <Transition name="jc-fab-fade">
    <button
      v-if="visible"
      type="button"
      class="jc-back-top"
      aria-label="回到顶部"
      data-focusable="true"
      tabindex="0"
      @click="backToTop"
    >
      <svg
        viewBox="0 0 24 24"
        fill="currentColor"
        width="22"
        height="22"
        aria-hidden="true"
      >
        <path d="M12 4l-8 8h5v8h6v-8h5z" />
      </svg>
    </button>
  </Transition>
</template>

<style scoped>
.jc-back-top {
  position: fixed;
  right: var(--jc-space-5);
  /* 移动端 tabbar 高度 + safe-area, > md 失去 tabbar 由 media query 重置 */
  bottom: calc(var(--jc-tabbar-height, 56px) + env(safe-area-inset-bottom, 0px) + var(--jc-space-4));
  width: 44px;
  height: 44px;
  border: 1px solid var(--jc-border-subtle);
  border-radius: 9999px;
  background-color: rgba(20, 20, 24, 0.85);
  backdrop-filter: blur(8px);
  color: var(--jc-text-primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  transition:
    transform var(--jc-dur-fast) var(--jc-ease-spring),
    box-shadow var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
  z-index: 40;
}

@media (min-width: 768px) {
  .jc-back-top {
    bottom: var(--jc-space-6);
    right: var(--jc-space-6);
    width: 48px;
    height: 48px;
  }
}

.jc-back-top:hover {
  background-color: rgba(155, 73, 231, 0.92);
  transform: translateY(-2px);
  box-shadow: 0 12px 28px rgba(155, 73, 231, 0.35), 0 0 0 1px rgba(155, 73, 231, 0.5);
}

.jc-back-top:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring), 0 8px 24px rgba(0, 0, 0, 0.4);
}

.jc-back-top:active {
  transform: translateY(0);
}

/* 渐显渐隐 */
.jc-fab-fade-enter-from,
.jc-fab-fade-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.9);
}
.jc-fab-fade-enter-active,
.jc-fab-fade-leave-active {
  transition:
    opacity var(--jc-dur-base) var(--jc-ease-standard),
    transform var(--jc-dur-base) var(--jc-ease-spring);
}
</style>

<style>
/* TV 模式: 用 D-pad 不需要 FAB, 隐藏 */
[data-mode='tv'] .jc-back-top {
  display: none;
}
</style>
