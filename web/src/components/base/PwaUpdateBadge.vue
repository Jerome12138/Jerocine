<script setup lang="ts">
import { computed } from 'vue'
import BaseIcon from './BaseIcon.vue'
import { usePwaUpdate } from '@/composables/usePwaUpdate'

/**
 * 新版本提示 (方案 §4.3, 2026-10-27 增强, 2026-11-09 改图标化).
 *
 * 形态: 标题旁一个圆形按钮, 里面是更新箭头; **不带文字**, 语义都放进 aria-label/title。
 *  - 发现更新/下载中(amber): 箭头用"液面上升"的方式按落地进度填满(见 usePwaUpdate 的
 *    progress) —— 底色是半透明箭头, 实色箭头按进度从下往上露出, 一眼能看出"在动";
 *  - 更新中(applying): 同上, 并加轻微呼吸, 且禁用重复点击;
 *  - 新版本就绪(cyan): 箭头填满 100%, 外圈呼吸提示可点。
 * 不点击也不打扰, 下次导航/启动本就是新版本。
 *
 * 放置: 顶栏 brand(标题)右侧(web/TV 共用) + 移动端抽屉菜单项(见 PublicHeader)。
 * 遥控器: `<button>` + data-focusable/tabindex ⇒ D-pad 可聚焦, OK 键触发 click。
 */

const { updateFound, updateReady, applying, progress, applyUpdate } = usePwaUpdate()

const visible = computed(() => updateFound.value || updateReady.value || applying.value)

/** 文案只用于读屏/悬停提示 —— 视觉上不再显示文字 */
const tip = computed(() => {
  if (applying.value) return '正在切换到新版本，请稍候'
  if (updateReady.value) return '新版本已就绪，点击刷新'
  return '发现新版本，点击立即更新'
})

/** 填充百分比: 就绪即 100%; 下载中最小 6%, 让"刚刚开始"也有可见的一小格 */
const fill = computed(() => {
  if (updateReady.value) return '100%'
  const pct = Math.max(6, Math.min(95, Math.round(progress.value * 100)))
  return `${pct}%`
})
</script>

<template>
  <Transition name="jc-pwa-badge">
    <button
      v-if="visible"
      type="button"
      class="jc-pwa-badge"
      :class="{ 'is-found': !updateReady && !applying, 'is-ready': updateReady }"
      data-focusable="true"
      tabindex="0"
      :disabled="applying"
      :aria-busy="applying ? 'true' : undefined"
      :aria-label="tip"
      :title="tip"
      @click="applyUpdate"
    >
      <span class="jc-pwa-badge__glyph" :style="{ '--jc-pwa-fill': fill }">
        <!-- 底: 半透明箭头(轮廓) -->
        <BaseIcon name="refresh" class="jc-pwa-badge__ghost" size="1em" />
        <!-- 上: 实色箭头, 用 clip-path 从下往上露出 = 按进度填满 -->
        <BaseIcon name="refresh" class="jc-pwa-badge__liquid" size="1em" />
      </span>
    </button>
  </Transition>
</template>

<style scoped>
.jc-pwa-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 32px;
  height: 32px;
  padding: 0;
  border-radius: 999px;
  background-color: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.45);
  color: #f59e0b;
  font-size: 16px;
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    border-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-pwa-badge:hover,
.jc-pwa-badge:focus-visible {
  background-color: rgba(245, 158, 11, 0.24);
  border-color: rgba(245, 158, 11, 0.8);
  outline: none;
}

.jc-pwa-badge:disabled {
  cursor: default;
  opacity: 0.85;
}

/* 图标容器: 尺寸跟着 font-size 走, 箭头本体绝对叠两层 */
.jc-pwa-badge__glyph {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1em;
  height: 1em;
  line-height: 1;
}

.jc-pwa-badge__ghost {
  opacity: 0.32;
}

.jc-pwa-badge__liquid {
  position: absolute;
  inset: 0;
  /* top 裁掉未完成的部分 ⇒ 可见高度 = 进度(液面从下往上涨) */
  clip-path: inset(calc(100% - var(--jc-pwa-fill, 0%)) 0 0 0);
  transition: clip-path 0.45s linear;
}

/* 下载中: 轻微呼吸, 表示"正在做事"(不做旋转 —— 旋转会把液面也转歪) */
.jc-pwa-badge.is-found .jc-pwa-badge__glyph {
  animation: jc-pwa-breathe 1.6s ease-in-out infinite;
}

/* 就绪: 转青色, 箭头填满, 外圈呼吸提示可点 */
.jc-pwa-badge.is-ready {
  color: #4ad1e5;
  background-color: rgba(74, 209, 229, 0.14);
  border-color: rgba(74, 209, 229, 0.5);
  animation: jc-pwa-glow 2s ease-in-out infinite;
}
.jc-pwa-badge.is-ready:hover,
.jc-pwa-badge.is-ready:focus-visible {
  background-color: rgba(74, 209, 229, 0.24);
  border-color: rgba(74, 209, 229, 0.8);
}

/* TV: 按钮与图标一起放大, 远看也能认出是个可聚焦的圆点 */
[data-mode='tv'] .jc-pwa-badge {
  width: 40px;
  height: 40px;
  font-size: 20px;
}

@keyframes jc-pwa-breathe {
  0%,
  100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(0.9);
    opacity: 0.75;
  }
}

@keyframes jc-pwa-glow {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(74, 209, 229, 0.35);
  }
  50% {
    box-shadow: 0 0 0 4px rgba(74, 209, 229, 0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .jc-pwa-badge,
  .jc-pwa-badge__glyph,
  .jc-pwa-badge__liquid {
    animation: none !important;
    transition: none !important;
  }
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
