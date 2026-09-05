<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useUserStore } from '@/stores/user'
import { useSiteStore } from '@/stores/site'
import { useUIStore } from '@/stores/ui'
import { useViewMode } from '@/composables/useViewMode'
import ManageHeader from './ManageHeader.vue'
import ManageSidebar from './ManageSidebar.vue'

const userStore = useUserStore()
const siteStore = useSiteStore()
const uiStore = useUIStore()
const { sidebarCollapsed } = storeToRefs(uiStore)
const { mode, isMobile, isTablet } = useViewMode()

const drawerOpen = ref(false)
function closeDrawer(): void { drawerOpen.value = false }

// main 区左侧 margin = sidebar 实际宽度 (mobile 抽屉模式不占位, tablet 强制 64, desktop 看用户偏好)
const mainMarginLeft = computed(() => {
  if (isMobile.value) return '0px'
  if (isTablet.value) return '64px'
  return sidebarCollapsed.value ? '64px' : '220px'
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
    class="gf-manage min-h-screen flex flex-col bg-base text-primary"
    :data-mode="mode"
  >
    <ManageHeader
      :show-hamburger="isMobile"
      @toggle-drawer="drawerOpen = !drawerOpen"
    />
    <div class="flex-1 flex relative">
      <ManageSidebar
        :variant="isMobile ? 'drawer' : isTablet ? 'icon-rail' : 'full'"
        :open="drawerOpen"
        @close="closeDrawer"
      />
      <main
        class="flex-1 overflow-x-auto p-[var(--gf-space-3)] md:p-[var(--gf-space-4)] lg:p-[var(--gf-space-6)] transition-[margin] duration-[var(--gf-dur-base)]"
        :style="{ marginLeft: mainMarginLeft }"
      >
        <slot />
      </main>
    </div>
  </div>
</template>
