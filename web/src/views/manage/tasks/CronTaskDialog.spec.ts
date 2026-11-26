import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CronTaskDialog from './CronTaskDialog.vue'
import type { CronTask } from '@/types/manage'

// 采集任务建模面弹窗（原「定时任务页」的表单搬到这里）。
// 重点：周期是选择式（SchedulePicker），"采集时长 ≤1 / 指定源必须勾选"这类守卫仍在。

const h = vi.hoisted(() => ({
  add: vi.fn(),
  update: vi.fn(),
  sources: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    cron: { add: h.add, update: h.update },
    collect: { list: h.sources }
  }
}))

function task(over: Partial<CronTask> = {}): CronTask {
  return {
    id: '7', ids: ['src_a'], time: 24, schedule: 'daily:03:00', model: 0, state: true, remark: '',
    ...over
  }
}

async function mountOpen(t: CronTask | null = null) {
  // ManageSheet 内容 Teleport 到 body，stub 掉让弹窗内联渲染可查（与其它弹窗单测同口径）
  const w = mount(CronTaskDialog, { props: { modelValue: false, task: t }, global: { stubs: { teleport: true } } })
  await w.setProps({ modelValue: true })
  await flushPromises()
  return w
}

function btn(w: ReturnType<typeof mount>, text: string) {
  return w.findAll('button').find((b) => b.text().trim() === text)
}

async function pick(w: ReturnType<typeof mount>, ariaLabel: string, label: string) {
  const trig = w.findAll('button[aria-haspopup="listbox"]').find((b) => b.attributes('aria-label') === ariaLabel)
  if (!trig) throw new Error(`未找到下拉: ${ariaLabel}`)
  await trig.trigger('click')
  const opt = w.findAll('[role="option"]').find((o) => o.text().includes(label))
  if (!opt) throw new Error(`未找到选项: ${label}`)
  await opt.trigger('click')
}

describe('CronTaskDialog 采集任务新建/编辑', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.sources.mockResolvedValue([{ id: 'src_a', name: '源A' }, { id: 'src_b', name: '源B' }])
  })

  it('新建：默认周期（每天 04:00）+ 时长 24 直接可保存', async () => {
    const w = await mountOpen(null)
    expect(w.text()).toContain('新建采集任务')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.add).toHaveBeenCalledWith(expect.objectContaining({ schedule: 'daily:04:00', time: 24, model: 0 }))
    expect(h.update).not.toHaveBeenCalled()
    expect(w.emitted('saved')).toBeTruthy()
  })

  it('编辑：回填行数据（周期/时长/资源站），保存走 update', async () => {
    const w = await mountOpen(task())
    expect(w.text()).toContain('编辑采集任务')
    expect(w.text()).toContain('每天 03:00')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.update).toHaveBeenCalledWith(expect.objectContaining({ id: '7', schedule: 'daily:03:00', ids: ['src_a'] }))
    expect(h.add).not.toHaveBeenCalled()
  })

  it('周期改选择器后提交的是新值（不再手敲）', async () => {
    const w = await mountOpen(null)
    await pick(w, '周期类型', '每周')
    await pick(w, '星期', '周日')
    await pick(w, '小时', '21')
    await pick(w, '分钟', '30')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.add).toHaveBeenCalledWith(expect.objectContaining({ schedule: 'weekly:7 21:30' }))
  })

  it('采集时长清空/0/非整数不提交（防服务端反转为全量采集）', async () => {
    const w = await mountOpen(null)
    const timeInput = w.findAll('input').find((i) => i.attributes('aria-label') === '采集时长')!
    for (const bad of ['', '0', '1.5']) {
      await timeInput.setValue(bad)
      await btn(w, '保存')!.trigger('click')
      await flushPromises()
      expect(w.text()).toContain('采集时长需为不小于 1 的整数')
      expect(h.add).not.toHaveBeenCalled()
    }
  })

  it('任务类型=仅更新所选资源站 且未勾选 → 拦住；勾选后放行', async () => {
    const w = await mountOpen(null)
    await pick(w, '任务类型', '仅更新所选资源站')
    await flushPromises()
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('请至少勾选一个资源站')
    expect(h.add).not.toHaveBeenCalled()

    await w.findAll('[role="button"]').find((t) => t.text() === '源A')!.trigger('click')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.add).toHaveBeenCalledWith(expect.objectContaining({ model: 1, ids: ['src_a'] }))
  })

  it('ids 深拷贝：勾选/取消不污染传入的行对象', async () => {
    const row = task({ model: 1, ids: ['src_a'] })
    const w = await mountOpen(row)
    await w.findAll('[role="button"]').find((t) => t.text() === '源A')!.trigger('click')
    await w.findAll('[role="button"]').find((t) => t.text() === '源A')!.trigger('click')
    expect(row.ids).toEqual(['src_a'])
  })

  it('保存失败：弹窗保持打开并内联展示错误', async () => {
    h.add.mockRejectedValue(new Error('服务器开小差'))
    const w = await mountOpen(null)
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('服务器开小差')
    expect(w.emitted('saved')).toBeFalsy()
  })
})

  it('启用开关：新建默认开；编辑回填原状态；提交带上 state（P2 单条启停能力恢复）', async () => {
    // 新建：默认启用
    const w = await mountOpen(null)
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.add).toHaveBeenCalledWith(expect.objectContaining({ state: true }))
    w.unmount()

    // 编辑一个已停用的任务：开关回填 false，原样提交 false（不会偷偷打开）
    h.add.mockClear()
    const off = await mountOpen(task({ state: false }))
    await btn(off, '保存')!.trigger('click')
    await flushPromises()
    expect(h.update).toHaveBeenCalledWith(expect.objectContaining({ id: '7', state: false }))
    off.unmount()
  })
