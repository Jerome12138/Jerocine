import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSiteStore } from './site'

vi.mock('@/api/config', () => ({
  getSiteConfig: vi.fn()
}))

import { getSiteConfig } from '@/api/config'

describe('useSiteStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('初始 basic=null, loaded=false', () => {
    const s = useSiteStore()
    expect(s.basic).toBeNull()
    expect(s.loaded).toBe(false)
    expect(s.loading).toBe(false)
  })

  it('ensureLoaded 调 api 并填充 basic', async () => {
    ;(getSiteConfig as ReturnType<typeof vi.fn>).mockResolvedValue({
      siteName: 'Jerocine',
      domain: 'http://x'
    })
    const s = useSiteStore()
    await s.ensureLoaded()
    expect(s.basic?.siteName).toBe('Jerocine')
    expect(s.loaded).toBe(true)
  })

  it('已 loaded → ensureLoaded 不再调 api', async () => {
    ;(getSiteConfig as ReturnType<typeof vi.fn>).mockResolvedValue({ siteName: 'X' })
    const s = useSiteStore()
    await s.ensureLoaded()
    await s.ensureLoaded()
    expect(getSiteConfig).toHaveBeenCalledTimes(1)
  })

  it('reset 清空状态', async () => {
    ;(getSiteConfig as ReturnType<typeof vi.fn>).mockResolvedValue({ siteName: 'X' })
    const s = useSiteStore()
    await s.ensureLoaded()
    s.reset()
    expect(s.basic).toBeNull()
    expect(s.loaded).toBe(false)
  })
})
