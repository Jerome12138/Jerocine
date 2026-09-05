import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

/**
 * 全局 UI 状态：sidebar、主题、loading 计数
 * 由网络层 / 布局组件读写。
 *
 * sidebar 折叠态持久化到 localStorage 'gf-sidebar-collapsed', 跨刷新保留用户偏好.
 */
const SIDEBAR_LS_KEY = 'gf-sidebar-collapsed'

function readSidebar(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_LS_KEY) === '1'
  } catch {
    return false
  }
}

export const useUIStore = defineStore('ui', () => {
  const sidebarCollapsed = ref<boolean>(readSidebar())
  const theme = ref<'dark' | 'light'>('dark')

  // 跨刷新保留偏好
  watch(sidebarCollapsed, (v) => {
    try {
      if (v) localStorage.setItem(SIDEBAR_LS_KEY, '1')
      else localStorage.removeItem(SIDEBAR_LS_KEY)
    } catch {
      /* 隐私模式忽略 */
    }
  })

  const loadingCount = ref(0)
  const loading = ref(false)

  function pushLoading(): void {
    loadingCount.value++
    loading.value = true
  }

  function popLoading(): void {
    loadingCount.value = Math.max(0, loadingCount.value - 1)
    if (loadingCount.value === 0) {
      loading.value = false
    }
  }

  function toggleSidebar(): void {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  function setSidebarCollapsed(v: boolean): void {
    sidebarCollapsed.value = v
  }

  function setTheme(t: 'dark' | 'light'): void {
    theme.value = t
  }

  return {
    sidebarCollapsed,
    theme,
    loading,
    loadingCount,
    pushLoading,
    popLoading,
    toggleSidebar,
    setSidebarCollapsed,
    setTheme
  }
})
