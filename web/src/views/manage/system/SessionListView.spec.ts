import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SessionListView from './SessionListView.vue'

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
})
