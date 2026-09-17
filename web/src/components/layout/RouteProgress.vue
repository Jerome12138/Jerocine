<script setup lang="ts">
/**
 * 路由切换顶部进度条 (bilibili / NProgress 风格)
 *
 * - router.beforeEach 时 start (300ms 内增长到 70%)
 * - router.afterEach 时 finish (跳到 100% → 200ms 后淡出)
 * - 不依赖 NProgress 第三方包, 纯 CSS animation
 * - z-index 高于 header
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const phase = ref<'idle' | 'loading' | 'done'>('idle')
let doneTimer: number | undefined
let removeBefore: (() => void) | null = null
let removeAfter: (() => void) | null = null
let removeError: (() => void) | null = null

function start(): void {
  if (doneTimer !== undefined) {
    window.clearTimeout(doneTimer)
    doneTimer = undefined
  }
  // 用两帧确保从 idle/done → loading 重置动画
  phase.value = 'idle'
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      phase.value = 'loading'
    })
  })
}

function finish(): void {
  phase.value = 'done'
  doneTimer = window.setTimeout(() => {
    phase.value = 'idle'
    doneTimer = undefined
  }, 380)
}

onMounted(() => {
  removeBefore = router.beforeEach((_to, _from, next) => {
    start()
    next()
  })
  removeAfter = router.afterEach(() => {
    finish()
  })
  removeError = router.onError(() => {
    finish()
  })
})

onBeforeUnmount(() => {
  removeBefore?.()
  removeAfter?.()
  removeError?.()
  if (doneTimer !== undefined) {
    window.clearTimeout(doneTimer)
  }
})
</script>

<template>
  <div
    class="jc-route-progress"
    :data-phase="phase"
    aria-hidden="true"
  >
    <span class="jc-route-progress__bar" />
  </div>
</template>

<style scoped>
.jc-route-progress {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  z-index: 100;
  pointer-events: none;
  opacity: 0;
  transition: opacity var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-route-progress[data-phase='loading'],
.jc-route-progress[data-phase='done'] {
  opacity: 1;
}
.jc-route-progress__bar {
  display: block;
  height: 100%;
  width: 0%;
  background-image: var(--jc-brand-gradient);
  box-shadow: 0 0 8px rgba(155, 73, 231, 0.5);
  transform-origin: left center;
  transition: width 0.6s var(--jc-ease-out);
}
.jc-route-progress[data-phase='loading'] .jc-route-progress__bar {
  /* 300ms 内增长到 70%, 模拟 NProgress 加速曲线 */
  width: 70%;
  transition-duration: 0.7s;
}
.jc-route-progress[data-phase='done'] .jc-route-progress__bar {
  /* 完成: 100% 后由父 opacity 淡出 */
  width: 100%;
  transition-duration: 0.18s;
}
</style>

<style>
/* TV 模式: 加粗一点 + 更亮, 大屏才看得清 */
[data-mode='tv'] .jc-route-progress {
  height: 4px;
}
</style>