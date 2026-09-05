import { describe, it, expect, beforeEach } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ContinueWatchingRow from './ContinueWatchingRow.vue'
import { useHistoryStore } from '@/stores'

/**
 * 首页「继续观看」横滚区单测:
 *  - 无历史 → 整块不渲染
 *  - 有历史 → 渲染标题 + 卡片(链到 record.link 续播)
 *  - 最多取 12 条
 *
 * 用本地模式(未登录)的 history store: store.record() 即写内存 + localStorage, list 立即可读。
 */
const mountOpts = {
  global: {
    stubs: { RouterLink: RouterLinkStub, BaseIcon: true, BaseImage: true }
  }
}

describe('ContinueWatchingRow', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('无观看历史时整块不渲染', () => {
    const w = mount(ContinueWatchingRow, mountOpts)
    expect(w.find('.gf-continue').exists()).toBe(false)
  })

  it('有历史时渲染"继续观看"标题与卡片', () => {
    const store = useHistoryStore()
    store.record({
      id: '1',
      name: '片A',
      link: '/play?id=1&source=x&episode=0',
      episode: '第1集',
      currentTime: 300,
      duration: 1000
    })
    store.record({
      id: '2',
      name: '片B',
      link: '/play?id=2&source=x&episode=0',
      episode: '第2集'
    })
    const w = mount(ContinueWatchingRow, mountOpts)
    expect(w.find('.gf-continue').exists()).toBe(true)
    expect(w.text()).toContain('继续观看')
    expect(w.text()).toContain('片A')
    expect(w.text()).toContain('片B')
    expect(w.findAll('.gf-continue__item').length).toBe(2)
  })

  it('最多取 12 条', () => {
    const store = useHistoryStore()
    for (let i = 0; i < 20; i++) {
      store.record({
        id: String(i),
        name: '片' + i,
        link: `/play?id=${i}&source=x&episode=0`,
        episode: '第1集'
      })
    }
    const w = mount(ContinueWatchingRow, mountOpts)
    expect(w.findAll('.gf-continue__item').length).toBe(12)
  })

  it('卡片链接指向 record.link(续播 /play)', () => {
    const store = useHistoryStore()
    store.record({
      id: '1',
      name: '片A',
      link: '/play?id=1&source=x&episode=0',
      episode: '第1集'
    })
    const w = mount(ContinueWatchingRow, mountOpts)
    const links = w.findAllComponents(RouterLinkStub)
    const playLink = links.find((l) => String(l.props('to')).includes('/play?id=1'))
    expect(playLink).toBeTruthy()
  })
})
