import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DanmuConfigCard from './DanmuConfigCard.vue'

const h = vi.hoisted(() => ({
  getDanmuConfig: vi.fn(),
  saveDanmuConfig: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    danmu: {
      getDanmuConfig: h.getDanmuConfig,
      saveDanmuConfig: h.saveDanmuConfig
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))

const ALL = ['bilibili', 'tencent', 'iqiyi', 'youku']

function resp(enabled: number, providers: string[]) {
  return { enabled, providers, all: ALL }
}

async function mountCard() {
  const w = mount(DanmuConfigCard)
  await new Promise((r) => setTimeout(r, 10))
  return w
}

describe('DanmuConfigCard 弹幕服务卡', () => {
  beforeEach(() => {
    h.getDanmuConfig.mockReset()
    h.saveDanmuConfig.mockReset()
    h.toast.mockReset()
    h.getDanmuConfig.mockResolvedValue(resp(0, []))
  })

  it('默认不启用: 总开关文案 + 平台勾选区不出现; 旧"分段点播"失实文案不再出现', async () => {
    const w = await mountCard()
    const text = w.text()
    expect(text).toContain('未启用（默认）')
    expect(text).not.toContain('分段点播')
    expect(w.findAll('input[type="checkbox"]').length).toBe(0)
  })

  it('GET 失败: 不渲染可编辑表单(防误存默认值全局关停), 重试成功后恢复', async () => {
    h.getDanmuConfig.mockRejectedValueOnce(new Error('boom'))
    const w = await mountCard()
    expect(w.text()).toContain('读取失败')
    // 错误态下无开关、无保存按钮
    expect(w.find('button[role="switch"]').exists()).toBe(false)
    expect(w.findAll('button').some((b) => b.text() === '保存')).toBe(false)
    // 重试恢复
    await w.findAll('button').find((b) => b.text() === '重试')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.find('button[role="switch"]').exists()).toBe(true)
  })

  it('保存成功: payload 带勾选平台, 响应回填 + success toast', async () => {
    h.getDanmuConfig.mockResolvedValue(resp(0, []))
    h.saveDanmuConfig.mockResolvedValue(resp(1, ['bilibili']))
    const w = await mountCard()
    await w.find('button[role="switch"]').trigger('click')
    await w.findAll('input[type="checkbox"]')[0].setValue(true) // bilibili
    await w.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveDanmuConfig).toHaveBeenCalledWith({ enabled: 1, providers: ['bilibili'] })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已保存'))
    // 响应回填: 勾选状态与开关一致
    expect(w.findAll('input[type="checkbox"]')[0].element as HTMLInputElement).toBeTruthy()
    expect((w.findAll('input[type="checkbox"]')[0].element as HTMLInputElement).checked).toBe(true)
  })

  it('保存失败: error toast 且 submitting 复位(按钮不再 loading)', async () => {
    h.getDanmuConfig.mockResolvedValue(resp(0, []))
    h.saveDanmuConfig.mockRejectedValueOnce(new Error('db down'))
    const w = await mountCard()
    await w.find('button[role="switch"]').trigger('click')
    await w.findAll('input[type="checkbox"]')[0].setValue(true)
    await w.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.toast).toHaveBeenCalledWith('error', 'db down')
    const save = w.findAll('button').find((b) => b.text() === '保存')!
    expect(save.attributes('disabled')).toBeUndefined()
  })

  it('总开关关闭时保存 enabled:0 直接放行(不需要勾选平台)', async () => {
    h.saveDanmuConfig.mockResolvedValue(resp(0, []))
    const w = await mountCard()
    await w.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveDanmuConfig).toHaveBeenCalledWith({ enabled: 0, providers: [] })
    expect(h.toast).not.toHaveBeenCalledWith('error', expect.stringContaining('至少勾选'))
  })

  it('反向 toggle: 已勾选平台可取消勾选, payload 保持 all 顺序', async () => {
    h.getDanmuConfig.mockResolvedValue(resp(1, ['bilibili', 'tencent']))
    h.saveDanmuConfig.mockResolvedValue(resp(1, ['bilibili']))
    const w = await mountCard()
    const boxes = w.findAll('input[type="checkbox"]')
    // bilibili / tencent 已勾选
    expect((boxes[0].element as HTMLInputElement).checked).toBe(true)
    expect((boxes[1].element as HTMLInputElement).checked).toBe(true)
    await boxes[1].setValue(false) // 取消 tencent
    await w.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveDanmuConfig).toHaveBeenCalledWith({ enabled: 1, providers: ['bilibili'] })
  })

  it('重置: 只重拉本卡配置(getDanmuConfig), 不发保存请求', async () => {
    const w = await mountCard()
    expect(h.getDanmuConfig).toHaveBeenCalledTimes(1)
    await w.findAll('button').find((b) => b.text() === '重置')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.getDanmuConfig).toHaveBeenCalledTimes(2)
    expect(h.saveDanmuConfig).not.toHaveBeenCalled()
  })
})
