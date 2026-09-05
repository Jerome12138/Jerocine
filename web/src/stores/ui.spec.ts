import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUIStore } from './ui'

describe('useUIStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('sidebarCollapsed 默认 false', () => {
    const s = useUIStore()
    expect(s.sidebarCollapsed).toBe(false)
  })

  it('localStorage gf-sidebar-collapsed=1 → 初始 true', () => {
    localStorage.setItem('gf-sidebar-collapsed', '1')
    const s = useUIStore()
    expect(s.sidebarCollapsed).toBe(true)
  })

  it('toggleSidebar 翻转 + 写 localStorage', async () => {
    const s = useUIStore()
    s.toggleSidebar()
    expect(s.sidebarCollapsed).toBe(true)
    // watch 是 async, 等一拍
    await new Promise((r) => setTimeout(r, 0))
    expect(localStorage.getItem('gf-sidebar-collapsed')).toBe('1')
    s.toggleSidebar()
    await new Promise((r) => setTimeout(r, 0))
    expect(localStorage.getItem('gf-sidebar-collapsed')).toBeNull()
  })

  it('setSidebarCollapsed(true) 显式设置', () => {
    const s = useUIStore()
    s.setSidebarCollapsed(true)
    expect(s.sidebarCollapsed).toBe(true)
  })

  it('theme 默认 dark', () => {
    expect(useUIStore().theme).toBe('dark')
  })

  it('setTheme("light") 切换', () => {
    const s = useUIStore()
    s.setTheme('light')
    expect(s.theme).toBe('light')
  })

  it('pushLoading 单次 → loading=true, count=1', () => {
    const s = useUIStore()
    s.pushLoading()
    expect(s.loading).toBe(true)
    expect(s.loadingCount).toBe(1)
  })

  it('push/pop 多次 → count 归零时 loading=false', () => {
    const s = useUIStore()
    s.pushLoading()
    s.pushLoading()
    expect(s.loadingCount).toBe(2)
    s.popLoading()
    expect(s.loading).toBe(true)
    s.popLoading()
    expect(s.loading).toBe(false)
    expect(s.loadingCount).toBe(0)
  })

  it('popLoading 过头 → count 不会变负', () => {
    const s = useUIStore()
    s.popLoading()
    s.popLoading()
    expect(s.loadingCount).toBe(0)
  })
})
