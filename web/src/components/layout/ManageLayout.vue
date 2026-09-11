<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useUserStore } from '@/stores/user'
import { useSiteStore } from '@/stores/site'
import { useViewMode } from '@/composables/useViewMode'
import ManageHeader from './ManageHeader.vue'
import ManageSidebar from './ManageSidebar.vue'

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
    class="gf-manage min-h-screen flex bg-base text-primary"
    :data-mode="mode"
  >
    <ManageSidebar
      :variant="isMobile ? 'drawer' : isTablet ? 'mini' : 'full'"
      :open="drawerOpen"
      @close="closeDrawer"
    />
    <div
      class="flex-1 flex flex-col min-w-0 transition-[margin] duration-[var(--gf-dur-base)]"
      :style="{ marginLeft: rightMarginLeft }"
    >
      <ManageHeader
        :show-hamburger="isMobile"
        @toggle-drawer="drawerOpen = !drawerOpen"
      />
      <main
        class="flex-1 overflow-x-auto min-w-0 p-[var(--gf-space-3)] md:p-[var(--gf-space-4)] lg:p-[var(--gf-space-6)]"
      >
        <slot />
      </main>
    </div>
  </div>
</template>
