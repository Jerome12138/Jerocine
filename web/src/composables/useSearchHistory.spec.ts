import { describe, it, expect, beforeEach } from 'vitest'
import { useSearchHistory } from './useSearchHistory'

describe('useSearchHistory', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('初始为空数组', () => {
    const h = useSearchHistory()
    expect(h.history.value).toEqual([])
  })

  it('add 后插入到最前; 重复 add 同一关键词不会重复入列, 但会移到最前', () => {
    const h = useSearchHistory()
    h.add('流浪地球')
    h.add('庆余年')
    h.add('狂飙')
    expect(h.history.value).toEqual(['狂飙', '庆余年', '流浪地球'])
    // 再 add 已存在的 → 去重 + 移到最前
    h.add('流浪地球')
    expect(h.history.value).toEqual(['流浪地球', '狂飙', '庆余年'])
  })

  it('大小写不敏感去重, 保留新输入的大小写', () => {
    const h = useSearchHistory()
    h.add('Avengers')
    h.add('avengers')
    expect(h.history.value).toEqual(['avengers'])
  })

  it('add 自动 trim, 空串/全空白被忽略', () => {
    const h = useSearchHistory()
    h.add('  无间道  ')
    expect(h.history.value).toEqual(['无间道'])
    h.add('   ')
    h.add('')
    expect(h.history.value).toEqual(['无间道'])
  })

  it('超过 limit 时丢弃最旧的', () => {
    const h = useSearchHistory(3)
    h.add('a')
    h.add('b')
    h.add('c')
    h.add('d')
    expect(h.history.value).toEqual(['d', 'c', 'b'])
  })

  it('remove 大小写不敏感地删除', () => {
    const h = useSearchHistory()
    h.add('Foo')
    h.add('Bar')
    h.remove('FOO')
    expect(h.history.value).toEqual(['Bar'])
    // 不存在的关键词不报错也不改变
    h.remove('xxx')
    expect(h.history.value).toEqual(['Bar'])
  })

  it('clear 清空', () => {
    const h = useSearchHistory()
    h.add('a')
    h.add('b')
    h.clear()
    expect(h.history.value).toEqual([])
  })

  it('持久化到 localStorage; 新实例能恢复', () => {
    const h1 = useSearchHistory()
    h1.add('persist-1')
    h1.add('persist-2')
    const h2 = useSearchHistory()
    expect(h2.history.value).toEqual(['persist-2', 'persist-1'])
  })

  it('localStorage 故障 (隐私模式) 不抛错, 内存态仍可用', () => {
    const orig = Storage.prototype.setItem
    Storage.prototype.setItem = () => {
      throw new Error('QuotaExceededError')
    }
    try {
      const h = useSearchHistory()
      expect(() => h.add('x')).not.toThrow()
      expect(h.history.value).toEqual(['x'])
    } finally {
      Storage.prototype.setItem = orig
    }
  })
})
