import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

/**
 * 全局 UI 状态：sidebar、主题、loading 计数
 * 由网络层 / 布局组件读写。
 *
 * sidebar 折叠态持久化到 localStorage 'jc-sidebar-collapsed', 跨刷新保留用户偏好.
 */
const SIDEBAR_LS_KEY = 'jc-sidebar-collapsed'

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

  // 顶栏"实底"滚动门槛(px): 首页沉浸式轮播会把它设为轮播背景(含下探)的文档底部,
  // >0 即"轮播大图场景" —— 页头不出边框线(2026-10-10)。
  // 注: 玻璃淡出走 PublicHeader 固定值常量(HERO_FADE_START/SPAN, 2026-10-11 定稿),
  // 本字段仅剩"是否轮播场景"(>0)的语义, 不再参与淡出/实底判定。
  const headerSolidAt = ref(0)

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
    headerSolidAt,
    pushLoading,
    popLoading,
    toggleSidebar,
    setSidebarCollapsed,
    setTheme
  }
})
