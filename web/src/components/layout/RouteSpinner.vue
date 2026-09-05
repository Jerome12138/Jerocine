<script setup lang="ts">
/**
 * 路由切换中央 iOS 风格 spinner overlay
 *
 * - router.beforeEach 时延迟 250ms 显示 (避免短任务闪烁), afterEach 立即关
 * - 12 根辐条 + opacity 渐变, 纯 CSS animation, 不依赖外部 lib
 * - 半透明遮罩, 不阻塞点击 (pointer-events: none)
 * - 全局挂在 App.vue, 覆盖所有 layout
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const visible = ref(false)
let showTimer: number | undefined
let removeBefore: (() => void) | null = null
let removeAfter: (() => void) | null = null
let removeError: (() => void) | null = null

function start(): void {
  if (showTimer !== undefined) {
    window.clearTimeout(showTimer)
  }
  showTimer = window.setTimeout(() => {
    visible.value = true
    showTimer = undefined
  }, 250)
}

function stop(): void {
  if (showTimer !== undefined) {
    window.clearTimeout(showTimer)
    showTimer = undefined
  }
  visible.value = false
}

onMounted(() => {
  removeBefore = router.beforeEach((_to, _from, next) => {
    start()
    next()
  })
  removeAfter = router.afterEach(() => {
    stop()
  })
  removeError = router.onError(() => {
    stop()
  })
})

onBeforeUnmount(() => {
  removeBefore?.()
  removeAfter?.()
  removeError?.()
  if (showTimer !== undefined) {
    window.clearTimeout(showTimer)
  }
})
</script>

<template>
  <Teleport to="body">
    <Transition name="gf-spinner-fade">
      <div
        v-if="visible"
        class="gf-route-spinner"
        role="status"
        aria-live="polite"
        aria-label="加载中"
      >
        <div class="gf-route-spinner__box">
          <span
            v-for="i in 12"
            :key="i"
            class="gf-route-spinner__bar"
            :style="{
              transform: `rotate(${(i - 1) * 30}deg) translate(0, -10px)`,
              animationDelay: `${-((12 - i) + 1) / 12}s`
            }"
          />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.gf-route-spinner {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
  z-index: 950;
}

.gf-route-spinner__box {
  position: relative;
  width: 60px;
  height: 60px;
  background-color: rgba(20, 20, 24, 0.78);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.35);
}

/* 12 根辐条围一圈, 每根 opacity 按 delay 错峰渐变. iOS 标准 spinner 样式 */
.gf-route-spinner__bar {
  position: absolute;
  left: calc(50% - 1px);
  top: calc(50% - 9px);
  width: 2px;
  height: 6px;
  background-color: rgba(255, 255, 255, 0.95);
  border-radius: 1px;
  transform-origin: 1px 10px;
  opacity: 0.12;
  animation: gf-ios-spin 1s linear infinite;
}

@keyframes gf-ios-spin {
  0% { opacity: 1; }
  100% { opacity: 0.12; }
}

.gf-spinner-fade-enter-active,
.gf-spinner-fade-leave-active {
  transition: opacity var(--gf-dur-fast, 180ms) var(--gf-ease-standard, ease);
}
.gf-spinner-fade-enter-from,
.gf-spinner-fade-leave-to {
  opacity: 0;
}
</style>
