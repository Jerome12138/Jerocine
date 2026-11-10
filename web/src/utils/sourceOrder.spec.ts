import { describe, expect, it } from 'vitest'
import { sortSourcesBySpeed } from './sourceOrder'

const src = (id: string) => ({ id, name: id })

describe('sortSourcesBySpeed', () => {
  it('无测速结果时保持原顺序', () => {
    const list = [src('a'), src('b'), src('c')]
    expect(sortSourcesBySpeed(list, {}).map((s) => s.id)).toEqual(['a', 'b', 'c'])
    expect(sortSourcesBySpeed(list, undefined).map((s) => s.id)).toEqual(['a', 'b', 'c'])
  })

  it('测速后按由快到慢排序', () => {
    const list = [src('a'), src('b'), src('c')]
    const speeds = { a: 900, b: 200, c: 500 }
    expect(sortSourcesBySpeed(list, speeds).map((s) => s.id)).toEqual(['b', 'c', 'a'])
  })

  it('未测/失败(-1)排在最后, 且保持它们之间原有相对顺序', () => {
    const list = [src('a'), src('b'), src('c'), src('d')]
    const speeds = { a: 300, c: -1 } // b/d 未测, c 失败
    // 有效(a=300)在前; 无效(b 未测, c 失败, d 未测)按原序 b,c,d
    expect(sortSourcesBySpeed(list, speeds).map((s) => s.id)).toEqual(['a', 'b', 'c', 'd'])
  })

  it('全部失败/未测 → 原顺序', () => {
    const list = [src('a'), src('b')]
    expect(sortSourcesBySpeed(list, { a: -1, b: -1 }).map((s) => s.id)).toEqual(['a', 'b'])
  })

  it('同延时用原下标稳定 tie-break', () => {
    const list = [src('a'), src('b'), src('c')]
    const speeds = { a: 100, b: 100, c: 100 }
    expect(sortSourcesBySpeed(list, speeds).map((s) => s.id)).toEqual(['a', 'b', 'c'])
  })

  it('不修改入参数组', () => {
    const list = [src('a'), src('b')]
    sortSourcesBySpeed(list, { a: 500, b: 100 })
    expect(list.map((s) => s.id)).toEqual(['a', 'b'])
  })
})
