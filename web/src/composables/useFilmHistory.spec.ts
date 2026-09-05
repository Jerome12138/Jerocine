import { describe, it, expect } from 'vitest'
import { buildPlayLink } from './useFilmHistory'

describe('buildPlayLink', () => {
  it('基础: id + source + episodeIndex', () => {
    const url = buildPlayLink({ id: 100, source: 'jerocine', episodeIndex: 2 })
    expect(url).toBe('/play?id=100&source=jerocine&episode=2')
  })

  it('字符串 id 同样支持', () => {
    const url = buildPlayLink({ id: 'abc', source: 'mock', episodeIndex: 0 })
    expect(url).toBe('/play?id=abc&source=mock&episode=0')
  })

  it('currentTime>0 → 加 currentTime 参数 (向下取整)', () => {
    const url = buildPlayLink({ id: 1, source: 's', episodeIndex: 0, currentTime: 123.7 })
    expect(url).toContain('currentTime=123')
    expect(url).not.toContain('123.7')
  })

  it('currentTime=0 (默认) → 不加 currentTime 参数', () => {
    const url = buildPlayLink({ id: 1, source: 's', episodeIndex: 0 })
    expect(url).not.toContain('currentTime')
  })

  it('currentTime<0 不加 (只 >0 才加)', () => {
    const url = buildPlayLink({ id: 1, source: 's', episodeIndex: 0, currentTime: -5 })
    expect(url).not.toContain('currentTime')
  })

  it('返回值以 /play? 开头', () => {
    const url = buildPlayLink({ id: 1, source: 's', episodeIndex: 0 })
    expect(url.startsWith('/play?')).toBe(true)
  })
})
