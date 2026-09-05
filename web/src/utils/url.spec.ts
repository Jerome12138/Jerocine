import { describe, it, expect } from 'vitest'
import { stringifyQuery, parseQuery } from './url'

describe('stringifyQuery', () => {
  it('基础对象 → query string', () => {
    const s = stringifyQuery({ a: '1', b: '2' })
    expect(s).toBe('a=1&b=2')
  })

  it('skip undefined / null / 空串', () => {
    const s = stringifyQuery({ a: '1', b: undefined, c: null, d: '' })
    expect(s).toBe('a=1')
  })

  it('数字 / 布尔自动转 string', () => {
    const s = stringifyQuery({ n: 42, b: true })
    expect(s).toBe('n=42&b=true')
  })

  it('特殊字符 URL encode', () => {
    const s = stringifyQuery({ q: 'a b&c' })
    expect(s).toBe('q=a%20b%26c')
  })

  it('空对象 → 空串', () => {
    expect(stringifyQuery({})).toBe('')
  })
})

describe('parseQuery', () => {
  it('基础 → 对象', () => {
    expect(parseQuery('a=1&b=2')).toEqual({ a: '1', b: '2' })
  })

  it('带前缀 ?', () => {
    expect(parseQuery('?a=1')).toEqual({ a: '1' })
  })

  it('空串 → 空对象', () => {
    expect(parseQuery('')).toEqual({})
    expect(parseQuery('?')).toEqual({})
  })

  it('URL decode', () => {
    expect(parseQuery('q=a%20b')).toEqual({ q: 'a b' })
  })

  it('无值的 key → 空串', () => {
    expect(parseQuery('a')).toEqual({ a: '' })
  })

  it('=后无值 → 空串 value', () => {
    expect(parseQuery('a=')).toEqual({ a: '' })
  })
})
