import { describe, it, expect } from 'vitest'
import type { DanmuComment } from '@/types/film'
import { toDanmuInput, toDanmuInputs } from './danmu'

function cmt(time: number, extra: Partial<DanmuComment> = {}): DanmuComment {
  return { text: '弹幕', time, color: 0xffffff, mode: 1, ...extra }
}

describe('toDanmuInput(s)：毫秒 → 秒映射 + 过滤', () => {
  it('浮点毫秒原样除以 1000（不做二次取整，库内部处理）；形状为 DanmakuItem', () => {
    expect(toDanmuInput(cmt(49953))).toEqual({
      id: '49953:弹幕:0',
      text: '弹幕',
      time: 49.953,
      mode: 1,
      color: 0xffffff
    })
    expect(toDanmuInput(cmt(2000, { mode: 4, color: 0xff0000, text: '底部' }))).toEqual({
      id: '2000:底部:0',
      text: '底部',
      time: 2,
      mode: 6,
      color: 0xff0000
    })
  })

  it('批量映射保持顺序；空列表 → 空数组', () => {
    expect(toDanmuInputs([cmt(1250), cmt(3750)])).toHaveLength(2)
    expect(toDanmuInputs([cmt(1250)])[0]!.time).toBe(1.25) // 非整秒不打散
    expect(toDanmuInputs([])).toEqual([])
  })

  it('整秒弹幕随机打散：±0.5s 确定性偏移、重新升序、消除齐发列队', () => {
    const items = toDanmuInputs([cmt(10000), cmt(10000), cmt(10000), cmt(10000)])
    expect(items).toHaveLength(4)
    for (const it of items) {
      expect(it.time).toBeGreaterThanOrEqual(10 - 0.5)
      expect(it.time).toBeLessThanOrEqual(10 + 0.5)
    }
    // 确实散开了(不再全部等于 10)
    expect(new Set(items.map((i) => i.time)).size).toBeGreaterThan(1)
    // 重新按 time 升序(引擎 load 要求有序)
    for (let i = 1; i < items.length; i += 1) {
      expect(items[i]!.time).toBeGreaterThanOrEqual(items[i - 1]!.time)
    }
    // 确定性: 两次映射结果一致(重看/重拉不漂移)
    const again = toDanmuInputs([cmt(10000), cmt(10000), cmt(10000), cmt(10000)])
    expect(again.map((i) => i.time)).toEqual(items.map((i) => i.time))
    const ids = new Set(items.map((i) => i.id))
    expect(ids.size).toBe(4) // id 唯一
  })

  it('非整秒弹幕不打散（已有毫秒级分散）', () => {
    expect(toDanmuInputs([cmt(10500)])[0]!.time).toBe(10.5)
  })

  it('模式归一: 源 1/2/6（滚动类）-> 1（danmaku-lite 无逆向滚动，共用同一轨道分配器）', () => {
    expect(toDanmuInput(cmt(1000, { mode: 2 })).mode).toBe(1)
    expect(toDanmuInput(cmt(1000, { mode: 6 })).mode).toBe(1)
    expect(
      toDanmuInputs([cmt(1000, { mode: 6 }), cmt(2000, { mode: 2 }), cmt(3000, { mode: 1 })]).map((i) => i.mode)
    ).toEqual([1, 1, 1])
  })

  it('模式归一: 源 4（底部固定）-> 6，源 5（顶部固定）-> 5', () => {
    expect(toDanmuInput(cmt(1000, { mode: 4 })).mode).toBe(6)
    expect(toDanmuInput(cmt(1000, { mode: 5 })).mode).toBe(5)
  })

  it('模式兜底: 其余非法 mode（3/7/8/17 等）-> 1（避免非法值直达引擎）', () => {
    expect(toDanmuInput(cmt(1000, { mode: 3 })).mode).toBe(1)
    expect(toDanmuInput(cmt(1000, { mode: 7 })).mode).toBe(1)
    expect(toDanmuInput(cmt(1000, { mode: 8 })).mode).toBe(1)
    expect(toDanmuInput(cmt(1000, { mode: 17 })).mode).toBe(1)
  })

  it('过滤: 关闭底部弹幕（mode 6）返回 null', () => {
    expect(toDanmuInput(cmt(1000, { mode: 4 }), 0, { bottomMode: false })).toBeNull()
    // 顶部 / 滚动不受 bottomMode 影响
    expect(toDanmuInput(cmt(1000, { mode: 5 }), 0, { bottomMode: false })).not.toBeNull()
    expect(toDanmuInput(cmt(1000, { mode: 1 }), 0, { bottomMode: false })).not.toBeNull()
  })

  it('过滤: 关闭顶部弹幕（mode 5）返回 null', () => {
    expect(toDanmuInput(cmt(1000, { mode: 5 }), 0, { topMode: false })).toBeNull()
    expect(toDanmuInput(cmt(1000, { mode: 1 }), 0, { topMode: false })).not.toBeNull()
  })

  it('过滤: 关闭彩色 -> 颜色改写为 0xffffff', () => {
    const it = toDanmuInput(cmt(1000, { color: 0xff0000 }), 0, { colored: false })
    expect(it).not.toBeNull()
    expect(it!.color).toBe(0xffffff)
  })
})
