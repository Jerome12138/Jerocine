import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FileGalleryView from './FileGalleryView.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import type { FileItem } from '@/types/manage'

const h = vi.hoisted(() => ({
  list: vi.fn(),
  remove: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    file: {
      list: h.list,
      remove: h.remove,
      upload: vi.fn()
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: vi.fn().mockResolvedValue(true) }))

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 10))

function row(id: number, name: string, ext: string): FileItem {
  return { id, name, url: `http://x/${id}.${ext}`, type: ext }
}

async function mountView() {
  const w = mount(FileGalleryView)
  await tick()
  return w
}

describe('FileGalleryView 文件库', () => {
  beforeEach(() => {
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({
      list: [row(1, 'a.jpg', 'jpg'), row(2, 'b.png', 'png'), row(3, 'c.mp4', 'mp4')],
      page: { total: 3, size: 39, current: 1, pageCount: 1 }
    })
  })

  it('渲染: 卡片显示名称与扩展名角标, 图片走 BaseImage(ratio=1/1), 视频走 <video>', async () => {
    const w = await mountView()
    expect(w.text()).toContain('a.jpg')
    expect(w.text()).toContain('c.mp4')
    // 扩展名角标(CSS uppercase)代替死掉的 KB 尺寸分支: api 从不映射 size
    const badges = w.findAll('span.uppercase').map((s) => s.text())
    expect(badges).toEqual(['jpg', 'png', 'mp4'])
    expect(w.text()).not.toContain('KB')

    const images = w.findAllComponents(BaseImage)
    expect(images.length).toBe(2)
    expect(images[0]!.props('ratio')).toBe('1/1') // 方形网格名副其实
    const videos = w.findAll('video')
    expect(videos.length).toBe(1)
    expect(videos[0]!.attributes('src')).toBe('http://x/3.mp4')
    expect(videos[0]!.attributes('preload')).toBe('metadata')
  })

  it('加载失败: 呈现错误态+重试, 不冒充「尚无文件」; 重试成功恢复列表', async () => {
    h.list.mockRejectedValueOnce(new Error('网络断了'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('网络断了')
    expect(w.text()).not.toContain('尚无文件')

    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await tick()
    expect(w.text()).toContain('a.jpg')
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('翻页: 页码随请求提交', async () => {
    const w = await mountView()
    w.findComponent(BasePagination).vm.$emit('change', 2)
    await tick()
    expect(h.list).toHaveBeenLastCalledWith({ current: 2 })
  })

  it('类型筛选: 客户端过滤图片/视频, 空类型有独立文案', async () => {
    const w = await mountView()
    const pick = (label: string) =>
      w.findAll('button').find((b) => b.text() === label)!

    await pick('视频').trigger('click')
    expect(w.text()).toContain('c.mp4')
    expect(w.text()).not.toContain('a.jpg')

    await pick('图片').trigger('click')
    expect(w.text()).toContain('a.jpg')
    expect(w.text()).not.toContain('c.mp4')

    // 该类型下无文件 ≠ 库是空的
    h.list.mockResolvedValue({
      list: [row(1, 'a.jpg', 'jpg')],
      page: { total: 1, size: 39, current: 1, pageCount: 1 }
    })
    const w2 = await mountView()
    await w2.findAll('button').find((b) => b.text() === '视频')!.trigger('click')
    expect(w2.text()).toContain('该类型下暂无文件')
    expect(w2.text()).not.toContain('尚无文件')

    await pick('全部').trigger('click')
    expect(w.text()).toContain('a.jpg')
    expect(w.text()).toContain('c.mp4')
  })

  it('删除: confirm 弹危险文案, 确认后 remove(id) 并刷新; 取消则不发请求', async () => {
    const { confirm } = await import('@/composables/useConfirm')
    const w = await mountView()
    const del = w.findAll('button').find((b) => b.text() === '删除')!
    await del.trigger('click')
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({ title: '确认删除？', danger: true, okText: '删除' })
    )
    await tick()
    expect(h.remove).toHaveBeenCalledWith(1)
    expect(h.list).toHaveBeenCalledTimes(2) // 初始 + 删除后刷新
    expect(h.toast).toHaveBeenCalledWith('success', '已删除')

    vi.mocked(confirm).mockResolvedValueOnce(false)
    await w.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await tick()
    expect(h.remove).toHaveBeenCalledTimes(1)
  })

  it('删除失败: toast error, 不误报成功, 也不留未接住的 rejection', async () => {
    h.remove.mockRejectedValueOnce(new Error('服务器繁忙'))
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await tick()
    expect(h.toast).toHaveBeenCalledWith('error', '服务器繁忙')
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
  })

  it('在途防护: 删除请求挂起期间全部删除按钮禁用, 完成后恢复', async () => {
    let resolveRemove!: () => void
    h.remove.mockImplementation(() => new Promise<void>((res) => (resolveRemove = res)))
    const w = await mountView()
    const delButtons = () => w.findAll('button').filter((b) => b.text() === '删除')
    expect(delButtons().length).toBe(3)

    await delButtons()[0]!.trigger('click')
    await tick()
    expect(delButtons().every((b) => (b.element as HTMLButtonElement).disabled)).toBe(true)

    resolveRemove()
    await tick()
    expect(delButtons().every((b) => (b.element as HTMLButtonElement).disabled)).toBe(false)
  })

  it('末页删空回缩: 删掉末页最后一条后自动回缩到最后一页重拉', async () => {
    let total = 40
    let page2 = [row(9, 'tail.jpg', 'jpg')]
    h.list.mockImplementation(async (p: { current?: number }) => ({
      list: p.current === 2 ? page2 : [row(1, 'a.jpg', 'jpg')],
      page: { total, size: 39, current: p.current ?? 1, pageCount: Math.max(1, Math.ceil(total / 39)) }
    }))
    h.remove.mockImplementation(async () => {
      total = 39 // 删除后只剩 1 页, 原第 2 页的行消失
      page2 = []
    })
    const w = await mountView()
    w.findComponent(BasePagination).vm.$emit('change', 2)
    await tick()
    expect(h.list).toHaveBeenLastCalledWith({ current: 2 })

    await w.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await tick()
    await tick()
    expect(h.remove).toHaveBeenCalledWith(9)
    // 越界的第 2 页被自动回缩回第 1 页重拉
    expect(h.list).toHaveBeenLastCalledWith({ current: 1 })
  })
})
