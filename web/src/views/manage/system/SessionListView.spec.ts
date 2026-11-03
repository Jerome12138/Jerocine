import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SessionListView from './SessionListView.vue'
import { ApiError } from '@/types/api'
import { useUserStore } from '@/stores/user'

const h = vi.hoisted(() => ({
  list: vi.fn(),
  revoke: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    system: {
      listSessions: h.list,
      revokeSession: h.revoke
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))

vi.mock('@/composables/useConfirm', () => ({ confirm: vi.fn().mockResolvedValue(true) }))

import { confirm } from '@/composables/useConfirm'

function row(over: Record<string, unknown> = {}) {
  return {
    id: 'abc123',
    userId: 7,
    userName: 'jerry',
    ip: '1.2.3.4',
    ua: 'Mozilla/5.0 (Windows NT 10.0)',
    loginAt: 1761000000,
    lastSeen: 1761003600,
    ...over
  }
}

async function mountView() {
  const w = mount(SessionListView)
  await new Promise((r) => setTimeout(r, 10))
  return w
}

describe('SessionListView 在线会话', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    h.list.mockReset()
    h.revoke.mockReset()
    h.toast.mockReset()
    vi.mocked(confirm).mockResolvedValue(true)
  })

  it('挂载即拉列表并展示用户/IP/登录时间', async () => {
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    const w = await mountView()
    expect(h.list).toHaveBeenCalledWith({ keyword: undefined, limit: 200 })
    const text = w.text()
    expect(text).toContain('jerry')
    expect(text).toContain('#7')
    expect(text).toContain('1.2.3.4')
    expect(text).toContain('共 1 条')
  })

  it('踢下线: 确认后调 revoke 并刷新列表', async () => {
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    h.revoke.mockResolvedValue({ id: 'abc123', revoked: true })
    const w = await mountView()

    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))

    expect(confirm).toHaveBeenCalled()
    expect(h.revoke).toHaveBeenCalledWith('abc123')
    expect(h.toast).toHaveBeenCalledWith('success', '已踢下线')
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('查询: 把关键字带给后端', async () => {
    h.list.mockResolvedValue({ list: [], total: 0 })
    const w = await mountView()
    await w.find('input').setValue('9.9.9.9')
    await w.findAll('button').find((b) => b.text() === '查询')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '9.9.9.9', limit: 200 })
  })

  it('查询: 回车触发, 且关键字 trim 后传后端(纯空格视为无关键字)', async () => {
    h.list.mockResolvedValue({ list: [], total: 0 })
    const w = await mountView()
    await w.find('input').setValue('  9.9.9.9  ')
    await w.find('input').trigger('keydown.enter')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '9.9.9.9', limit: 200 })

    await w.find('input').setValue('   ')
    await w.findAll('button').find((b) => b.text() === '查询')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: undefined, limit: 200 })
  })

  it('加载失败: 错误态+重试, 不冒充"暂无会话"空态', async () => {
    h.list.mockRejectedValueOnce(new Error('network down'))
    const w = await mountView()
    expect(h.toast).toHaveBeenCalledWith('error', 'network down')
    expect(w.text()).toContain('加载失败')
    expect(w.text()).not.toContain('暂无在线会话')
    // 重试成功恢复列表
    h.list.mockResolvedValueOnce({ list: [row()], total: 1 })
    await w.findAll('button').find((b) => b.text() === '重试')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('jerry')
  })

  it('空列表: 展示空态文案', async () => {
    h.list.mockResolvedValue({ list: [], total: 0 })
    const w = await mountView()
    expect(w.text()).toContain('暂无在线会话')
  })

  it('列表达到 200 条上限时提示"仅显示最近活跃的 200 条"', async () => {
    h.list.mockResolvedValue({ list: Array.from({ length: 200 }, (_, i) => row({ id: String(i) })), total: 200 })
    const w = await mountView()
    expect(w.text()).toContain('仅显示最近活跃的 200 条')
  })

  it('踢下线: confirm 取消则不调 revoke', async () => {
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    h.revoke.mockResolvedValue({ id: 'abc123', revoked: true })
    vi.mocked(confirm).mockResolvedValueOnce(false)
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(confirm).toHaveBeenCalled()
    expect(h.revoke).not.toHaveBeenCalled()
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
  })

  it('踢下线: revoke 请求期间行按钮禁用', async () => {
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    let resolveRevoke!: () => void
    h.revoke.mockImplementation(() => new Promise<void>((r) => (resolveRevoke = r)))
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    const btn = w.findAll('button').find((b) => b.text() === '踢下线')!
    expect(btn.attributes('disabled')).toBeDefined()
    resolveRevoke()
    await new Promise((r) => setTimeout(r, 10))
    expect(
      w.findAll('button').find((b) => b.text() === '踢下线')!.attributes('disabled')
    ).toBeUndefined()
  })

  it('写操作互斥收敛单点: 任一 revoke 在途, 全部踢下线按钮禁用(useAsyncAction 共享 busy)', async () => {
    h.list.mockResolvedValue({
      list: [row({ id: 'a', userName: 'alice' }), row({ id: 'b', userName: 'bob' })],
      total: 2
    })
    let resolveRevoke!: () => void
    h.revoke.mockImplementation(() => new Promise<void>((r) => (resolveRevoke = r)))
    const w = await mountView()
    // 踢第一行(挂起)
    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    const kickBtns = w.findAll('tbody button').filter((b) => b.text() === '踢下线')
    expect(kickBtns).toHaveLength(2)
    // 两行按钮都被禁用(此前 revoking===row.id 仅禁单行)
    for (const b of kickBtns) {
      expect(b.attributes('disabled')).toBeDefined()
    }
    resolveRevoke()
    await new Promise((r) => setTimeout(r, 10))
    // after 会重拉列表重渲染, 必须重新查询(旧 wrapper 指向已卸载节点)
    for (const b of w.findAll('tbody button').filter((b) => b.text() === '踢下线')) {
      expect(b.attributes('disabled')).toBeUndefined()
    }
  })

  it('踢下线失败(非 404): error toast 且不刷新列表', async () => {
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    h.revoke.mockRejectedValueOnce(new ApiError(500))
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.toast).toHaveBeenCalledWith('error', '请求失败')
    expect(h.list).toHaveBeenCalledTimes(1) // 未刷新
  })

  it('踢下线 404(会话已下线): 提示后刷新列表清掉死行', async () => {
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    h.revoke.mockRejectedValueOnce(new ApiError(404))
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.toast).toHaveBeenCalledWith('info', expect.stringContaining('已下线'))
    expect(h.list).toHaveBeenCalledTimes(2) // 已刷新
  })

  it('自己的会话: 行内"当前登录"标识 + 确认弹窗说明会登出自己', async () => {
    const store = useUserStore()
    store.info = { id: 7, userName: 'jerry' }
    h.list.mockResolvedValue({ list: [row()], total: 1 })
    h.revoke.mockResolvedValue({ id: 'abc123', revoked: true })
    const w = await mountView()
    expect(w.text()).toContain('当前登录')
    await w.findAll('button').find((b) => b.text() === '踢下线')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({ desc: expect.stringContaining('登出自己') })
    )
  })
})
