<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useUserStore } from '@/stores/user'
import { useSiteStore } from '@/stores/site'
import { useViewMode } from '@/composables/useViewMode'
import ManageHeader from './ManageHeader.vue'
import ManageSidebar from './ManageSidebar.vue'
import { useAutoHideScrollbar } from '@/composables/useAutoHideScrollbar'

/** 管理后台主区滚动条：滚动时显形、停止 700ms 后收起（用户 2026-11-06 要求） */
const mainScroll = useAutoHideScrollbar()

const userStore = useUserStore()
const siteStore = useSiteStore()
const { mode, isMobile, isTablet } = useViewMode()

const drawerOpen = ref(false)
function closeDrawer(): void { drawerOpen.value = false }

// 右侧列(头部+主区)左侧 margin = sidebar 实际宽度 (mobile 抽屉不占位, tablet 72 迷你栏, desktop 固定 220)
// 侧栏 fixed 通到视口顶部, 头部/页面标题从侧栏右缘开始排。
const rightMarginLeft = computed(() => {
  if (isMobile.value) return '0px'
  if (isTablet.value) return '72px'
  return '220px'
})

onMounted(async () => {
  await siteStore.ensureLoaded()
  if (userStore.isLoggedIn && !userStore.info) {
    try {
      await userStore.fetchInfo()
    } catch {
      // 拦截器会处理 401
    }
  }
})
</script>

<template>
  <div
    class="jc-manage h-screen overflow-hidden flex bg-base text-primary"
    :data-mode="mode"
  >
    <ManageSidebar
      :variant="isMobile ? 'drawer' : isTablet ? 'mini' : 'full'"
      :open="drawerOpen"
      @close="closeDrawer"
    />
    <div
      class="flex-1 flex flex-col min-w-0 min-h-0 transition-[margin] duration-[var(--jc-dur-base)]"
      :style="{ marginLeft: rightMarginLeft }"
    >
      <ManageHeader
        :show-hamburger="isMobile"
        @toggle-drawer="drawerOpen = !drawerOpen"
      />
      <main
        class="flex-1 flex flex-col min-h-0 overflow-hidden min-w-0 p-[var(--jc-space-3)] md:p-[var(--jc-space-4)] lg:p-[var(--jc-space-6)]"
      >
        <!-- 内容包裹层: 桌面封顶 1440px 居中(页面过空治理, D5) + 高度链贯通(表格 fit 依赖, D1) -->
        <div
          class="jc-manage-main jc-scrollauto mx-auto flex w-full min-h-0 flex-1 flex-col overflow-y-auto max-w-[var(--jc-content-max-w)]"
          @scroll="mainScroll.onScroll"
        >
          <slot />
        </div>
      </main>
    </div>
  </div>
</template>
