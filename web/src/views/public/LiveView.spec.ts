import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

/**
 * LiveView 组件测试：源/频道加载、分组渲染、切源清空、频道选择。
 * usePlayer（ArtPlayer+hls.js）以 vi.mock 桩替 —— 组件测试只关心数据流与交互。
 */

vi.mock('@/api', () => ({
  liveApi: {
    getLiveSources: vi.fn(),
    getLiveChannels: vi.fn()
  }
}))

vi.mock('@/composables/usePlayer', () => ({
  usePlayer: () => ({
    init: vi.fn(),
    dispose: vi.fn(),
    play: vi.fn().mockResolvedValue(undefined),
    pause: vi.fn(),
    on: vi.fn(() => () => undefined),
    errored: { value: false },
    player: { value: null },
    ready: { value: false },
    paused: { value: true },
    buffering: { value: false },
    videoSize: { value: null },
    currentTime: { value: 0 },
    duration: { value: 0 }
  })
}))

vi.mock('@/composables/useViewMode', () => ({
  useViewMode: () => ({ isTV: { value: false }, isDesktop: { value: true }, isMobile: { value: false } })
}))

import { liveApi } from '@/api'

const m = liveApi as unknown as {
  getLiveSources: ReturnType<typeof vi.fn>
  getLiveChannels: ReturnType<typeof vi.fn>
}

import LiveView from './LiveView.vue'

function mountView(): ReturnType<typeof mount> {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div/>' } }] })
  return mount(LiveView, { global: { plugins: [router] } })
}

const sources = {
  sources: [
    { id: 1, name: '主源' },
    { id: 2, name: '备用源' }
  ]
}
const chans1 = {
  id: 1,
  name: '主源',
  channels: [
    { name: 'CCTV-1', group: '央视', logo: 'http://l/1.png', url: 'http://s/cctv1' },
    { name: 'CCTV-5', group: '央视', logo: '', url: 'http://s/cctv5' },
    { name: '湖南卫视', group: '卫视', logo: '', url: 'http://s/hntv' }
  ]
}
const chans2 = { id: 2, name: '备用源', channels: [{ name: '测试台', group: '其他', logo: '', url: 'http://s/test' }] }

describe('LiveView', () => {
  beforeEach(() => {
    localStorage.clear()
    m.getLiveSources.mockReset()
    m.getLiveChannels.mockReset()
    m.getLiveSources.mockResolvedValue(sources)
    m.getLiveChannels.mockImplementation((id: number) =>
      Promise.resolve(id === 2 ? chans2 : chans1)
    )
  })

  it('加载源与频道, 按分组渲染(保序), 无分组归默认组', async () => {
    const w = mountView()
    await flushPromises()
    const groups = w.findAll('.jc-live-group__title')
    expect(groups.map((g) => g.text().replace(/\d+$/, ''))).toEqual(['央视', '卫视'])
    expect(w.findAll('.jc-live-channel')).toHaveLength(3)
    // 频道数徽标
    expect(groups[0]!.text()).toContain('2')
  })

  it('点击频道: 高亮 + 记忆写入 localStorage', async () => {
    const w = mountView()
    await flushPromises()
    await w.findAll('.jc-live-channel')[0]!.trigger('click')
    expect(w.findAll('.jc-live-channel')[0]!.classes()).toContain('is-active')
    const saved = JSON.parse(localStorage.getItem('jc-live-last-channel') ?? '{}')
    expect(saved.url).toBe('http://s/cctv1')
    expect(saved.sourceId).toBe(1)
  })

  it('切换源: 重新拉取频道列表并清空旧选择', async () => {
    const w = mountView()
    await flushPromises()
    await w.findAll('.jc-live-channel')[0]!.trigger('click')
    await w.findAll('.jc-live-source-chip')[1]!.trigger('click')
    await flushPromises()
    expect(m.getLiveChannels).toHaveBeenLastCalledWith(2)
    expect(w.findAll('.jc-live-channel')).toHaveLength(1)
    expect(w.findAll('.jc-live-channel')[0]!.text()).toContain('测试台')
    expect(w.findAll('.jc-live-channel.is-active')).toHaveLength(0) // 切源清空选中
  })

  it('记忆恢复: 上次源+频道在刷新后回显(不自动播放)', async () => {
    localStorage.setItem('jc-live-last-channel', JSON.stringify({ sourceId: 2, url: 'http://s/test' }))
    const w = mountView()
    await flushPromises()
    expect(m.getLiveChannels).toHaveBeenLastCalledWith(2) // 直接拉记忆源
    expect(w.findAll('.jc-live-channel.is-active')).toHaveLength(1)
    expect(w.text()).toContain('测试台')
  })

  it('源列表为空: 显示空态而非报错', async () => {
    m.getLiveSources.mockResolvedValue({ sources: [] })
    const w = mountView()
    await flushPromises()
    expect(w.text()).toContain('暂无直播源')
  })

  it('源加载失败: 显示错误态', async () => {
    m.getLiveSources.mockRejectedValue(new Error('network'))
    const w = mountView()
    await flushPromises()
    expect(w.text()).toContain('直播源加载失败')
  })

  /* ---- 覆盖率补强(2026-10-30) ---- */

  it('频道列表加载失败: 显示错误态而非空白', async () => {
    m.getLiveChannels.mockRejectedValue(new Error('network'))
    const w = mountView()
    await flushPromises()
    expect(w.text()).toContain('频道列表加载失败，请稍后重试')
    expect(w.findAll('.jc-live-channel')).toHaveLength(0)
  })

  it('频道列表为空: 提示该源暂无可用频道', async () => {
    m.getLiveChannels.mockResolvedValue({ id: 1, name: '主源', channels: [] })
    const w = mountView()
    await flushPromises()
    expect(w.text()).toContain('该直播源暂无可用频道')
    expect(w.findAll('.jc-live-group')).toHaveLength(0)
  })

  it('记忆频道不在列表中: 源按记忆恢复, 但频道不高亮', async () => {
    localStorage.setItem('jc-live-last-channel', JSON.stringify({ sourceId: 2, url: 'http://s/gone' }))
    const w = mountView()
    await flushPromises()
    expect(m.getLiveChannels).toHaveBeenLastCalledWith(2) // 记忆源仍生效
    expect(w.findAll('.jc-live-channel.is-active')).toHaveLength(0) // url 不在列表, 不回显
    expect(w.find('.jc-live-loading').exists()).toBe(true) // 播放区停留在"选择频道"提示, 无僵尸画面
  })

  it('源列表为空时播放器区不渲染(仅空态)', async () => {
    m.getLiveSources.mockResolvedValue({ sources: [] })
    const w = mountView()
    await flushPromises()
    expect(w.find('.jc-live-grid').exists()).toBe(false)
    expect(w.find('.jc-live-source-chip').exists()).toBe(false)
  })
})
