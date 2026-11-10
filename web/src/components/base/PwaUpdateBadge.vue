<script setup lang="ts">
import { computed } from 'vue'
import BaseIcon from './BaseIcon.vue'
import { usePwaUpdate } from '@/composables/usePwaUpdate'

/**
 * 新版本提示 (方案 §4.3, 2026-10-27 增强, 2026-11-09 改图标化, 2026-11-10 改向上箭头).
 *
 * 形态: 标题旁一个圆形按钮, 里面是**向上箭头**(↑, 一般网站/APP「有新版本」的样子);
 * 水印动效做在箭头内部 —— 底色是半透明箭头, 实色箭头按进度从下往上露出(液面上升);
 * 不带文字, 语义都放进 aria-label/title。
 *
 * 三种态(2026-11-10 定稿):
 *  - 下载中(amber): **不可点**。此时旧 SW 还控制着页面, 刷新只会拿回旧 index.html
 *    (导航命中 navigateFallback)，点了纯属误导。悬停/标题给出百分比。
 *  - 已就绪(cyan): 水位满格, 可点 ⇒ 点了一下子就换到新版。
 *  - 切换中(applying): 满格 + 呼吸, 禁重复点击(实际是一次 reload, 转瞬即逝)。
 *
 * 放置: 顶栏 brand(标题)右侧(web/TV 共用) + 移动端抽屉菜单项(见 PublicHeader)。
 * 遥控器: `<button>` + data-focusable/tabindex ⇒ D-pad 可聚焦, OK 键触发 click。
 */

const { updateFound, updateReady, applying, progress, applyUpdate } = usePwaUpdate()

const visible = computed(() => updateFound.value || updateReady.value || applying.value)

/** 可用 = 点了一下子就能换新版; 下载中一律不可点 */
const actionable = computed(() => updateReady.value && !applying.value)

const pct = computed(() => Math.max(1, Math.min(99, Math.round(progress.value * 100))))

/** 文案只用于读屏/悬停提示 —— 视觉上不显示文字 */
const tip = computed(() => {
  if (applying.value) return '正在切换到新版本，请稍候'
  if (updateReady.value) return '新版本已就绪，点击刷新'
  return `正在下载新版本 ${pct.value}%，完成后即可刷新`
})

/** 填充百分比: 就绪/切换中才是 100%(满格 = 真的能点了);
 * 下载中最大 99% —— 满格要留给"可以点"这个信号, 别在还装不完的时候骗人。
 * 下限 6%, 让"刚刚开始"也有可见的一小格。
 */
const fill = computed(() => {
  if (updateReady.value || applying.value) return '100%'
  return `${Math.max(6, pct.value)}%`
})

/**
 * 点击入口再守一道: 下载中点下去什么都不该发生(applyUpdate 自身也会拦,
 * 这里让"不可点"在 UI 层就成立, 也便于测试)。
 */
function onClick(): void {
  if (!actionable.value) return
  applyUpdate()
}
</script>

<template>
  <Transition name="jc-pwa-badge">
    <button
      v-if="visible"
      type="button"
      class="jc-pwa-badge"
      :class="{ 'is-found': !updateReady && !applying, 'is-ready': updateReady && !applying }"
      data-focusable="true"
      tabindex="0"
      :aria-disabled="actionable ? undefined : 'true'"
      :aria-busy="applying ? 'true' : undefined"
      :aria-label="tip"
      :title="tip"
      @click="onClick"
    >
      <span class="jc-pwa-badge__glyph" :style="{ '--jc-pwa-fill': fill }">
        <!-- 底: 半透明箭头(轮廓) -->
        <BaseIcon name="arrow-up" class="jc-pwa-badge__ghost" size="1em" />
        <!-- 上: 实色箭头, 用 clip-path 从下往上露出 = 按进度填满(液面上升) -->
        <BaseIcon name="arrow-up" class="jc-pwa-badge__liquid" size="1em" />
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

/* 只有"能点"时才给 hover 加深 —— 下载中悬停仅靠 title 提示,
   外观不能暗示可点(用户点了个刷不出新版的按钮, 比没入口更糟) */
.jc-pwa-badge:not([aria-disabled='true']):hover,
.jc-pwa-badge:focus-visible {
  background-color: rgba(245, 158, 11, 0.24);
  border-color: rgba(245, 158, 11, 0.8);
  outline: none;
}

/* 下载中: 可聚焦(遥控器能停在上面看提示), 但光标不给"手型" */
.jc-pwa-badge[aria-disabled='true'] {
  cursor: default;
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
