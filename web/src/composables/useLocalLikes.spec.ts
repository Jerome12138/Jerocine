import { describe, it, expect, beforeEach } from 'vitest'
import { useLocalLikes } from './useLocalLikes'

describe('useLocalLikes', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('未点赞过的 id 应返回 false', () => {
    const likes = useLocalLikes()
    expect(likes.isLiked('film-101').value).toBe(false)
  })

  it('toggle 切换 true/false; isLiked 响应式更新', () => {
    const likes = useLocalLikes()
    const ref = likes.isLiked('film-101')
    expect(ref.value).toBe(false)
    likes.toggle('film-101')
    expect(ref.value).toBe(true)
    likes.toggle('film-101')
    expect(ref.value).toBe(false)
  })

  it('点赞状态持久化到 localStorage', () => {
    const likes = useLocalLikes()
    likes.toggle('film-101')
    likes.toggle('film-202')
    const raw = localStorage.getItem('gf-likes')
    expect(raw).toBeTruthy()
    const parsed = JSON.parse(raw!)
    expect(parsed['film-101']).toBe(true)
    expect(parsed['film-202']).toBe(true)
    // 再 toggle film-101 → 应只剩 film-202
    likes.toggle('film-101')
    const after = JSON.parse(localStorage.getItem('gf-likes')!)
    expect(after['film-101']).toBeUndefined()
    expect(after['film-202']).toBe(true)
  })

  it('刷新页面 (重新 useLocalLikes) 仍记得点赞状态', () => {
    const likes1 = useLocalLikes()
    likes1.toggle('film-303')
    // 模拟新会话: 重新调用 hook
    const likes2 = useLocalLikes()
    expect(likes2.isLiked('film-303').value).toBe(true)
  })

  it('id 类型兼容 string / number, 统一按 string 存储', () => {
    const likes = useLocalLikes()
    likes.toggle(42)
    expect(likes.isLiked(42).value).toBe(true)
    expect(likes.isLiked('42').value).toBe(true)
  })

  it('localStorage 故障 (隐私模式) 不抛错, 内存态仍可用', () => {
    const origSetItem = Storage.prototype.setItem
    Storage.prototype.setItem = () => {
      throw new Error('QuotaExceededError')
    }
    try {
      const likes = useLocalLikes()
      expect(() => likes.toggle('film-999')).not.toThrow()
      expect(likes.isLiked('film-999').value).toBe(true)
    } finally {
      Storage.prototype.setItem = origSetItem
    }
  })
})
