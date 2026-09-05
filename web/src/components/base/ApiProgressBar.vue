<script setup lang="ts">
/**
 * 顶部 API 进度条 (与 RouteProgress 互补)
 * 由 uiStore.loading 驱动, 显示一个滑动的 2px 顶部条.
 *
 * 与 RouteProgress 区别:
 *  - RouteProgress 监听路由切换 (页面级)
 *  - ApiProgressBar 监听 uiStore.loading (任何 API 请求)
 *
 * 80ms 内完成的请求跳过显示 (避免闪烁).
 */
import { onBeforeUnmount, ref, watch } from 'vue'
import { useUIStore } from '@/stores/ui'

const uiStore = useUIStore()

const visible = ref(false)
const widthPct = ref(0)
let showTimer: number | undefined
let growTimer: number | undefined
let hideTimer: number | undefined

function clearTimers(): void {
  if (showTimer !== undefined) { window.clearTimeout(showTimer); showTimer = undefined }
  if (growTimer !== undefined) { window.clearTimeout(growTimer); growTimer = undefined }
  if (hideTimer !== undefined) { window.clearTimeout(hideTimer); hideTimer = undefined }
}

function scheduleGrow(): void {
  // 阶段式增长: 30 → 60 → 80 → 90, 防止假死
  const steps = [30, 60, 80, 90]
  let i = 0
  const tick = (): void => {
    if (i >= steps.length) return
    widthPct.value = steps[i] ?? 90
    i++
    growTimer = window.setTimeout(tick, 400 + i * 200)
  }
  tick()
}

watch(
  () => uiStore.loading,
  (loading) => {
    clearTimers()
    if (loading) {
      // 80ms 内完成 → 不显示
      showTimer = window.setTimeout(() => {
        visible.value = true
        widthPct.value = 10
        scheduleGrow()
      }, 80)
    } else {
      if (visible.value) {
        widthPct.value = 100
        hideTimer = window.setTimeout(() => {
          visible.value = false
          widthPct.value = 0
        }, 280)
      } else {
        visible.value = false
        widthPct.value = 0
      }
    }
  }
)

onBeforeUnmount(clearTimers)
</script>

<template>
  <div
    v-show="visible"
    class="gf-api-progress"
    :style="{ width: widthPct + '%' }"
    aria-hidden="true"
  />
</template>

<style scoped>
.gf-api-progress {
  position: fixed;
  top: 0;
  left: 0;
  height: 2px;
  z-index: 200;
  background: linear-gradient(90deg, #9b49e7, #4ad1e5);
  box-shadow: 0 0 4px rgba(155, 73, 231, 0.5);
  transition: width 300ms ease-out, opacity 280ms ease-out;
  pointer-events: none;
}
</style>
