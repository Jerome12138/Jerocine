import { describe, it, expect } from 'vitest'
import type { DanmuComment, DanmuProvider, DanmuTencentSegment } from '@/types/film'
import {
  nextSegmentIndex,
  prefetchWindow,
  resolveDanmuProvider,
  segmentIndexAt,
  shouldPrefetchNext,
  toDanmuInput,
  toDanmuInputs,
} from './danmu'

/** 造分段：按给定起始秒序列（默认 360s/段） */
function segs(...starts: number[]): DanmuTencentSegment[] {
  return starts.map((startTime, i) => ({ provider: 'tencent', startTime, segmentId: `s${i}` }))
}

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
    expect(toDanmuInputs([cmt(1000), cmt(3000)])).toHaveLength(2)
    expect(toDanmuInputs([cmt(1000)])[0]!.time).toBe(1)
    expect(toDanmuInputs([])).toEqual([])
  })

  it('同秒弹幕聚堆打散：相同 time 按序错开小步长，避免齐发列成整齐一列', () => {
    const items = toDanmuInputs([cmt(10000), cmt(10000), cmt(10000), cmt(10000)])
    expect(items).toHaveLength(4)
    expect(items[0]!.time).toBe(10)
    expect(items[1]!.time).toBeCloseTo(10 + 0.18, 3)
    expect(items[2]!.time).toBeCloseTo(10 + 0.36, 3)
    expect(items[3]!.time).toBeCloseTo(10 + 0.54, 3)
    const ids = new Set(items.map((i) => i.id))
    expect(ids.size).toBe(4) // id 唯一
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

describe('segmentIndexAt：t 所在段索引', () => {
  const s = segs(0, 360, 720, 1080)

  it.each([
    ['空列表 → 0（兜底，不让预拉从负索引开始）', [] as number[], 500, 0],
    ['0 段起始为 0，t=0 → 0', [0, 360], 0, 0],
    ['段间中点归前一段', [0, 360], 100, 0],
    ['边界归属后段（startTime <= t）', [0, 360], 360, 1],
    ['超过末段起始 → 末段', [0, 360, 720], 99999, 2],
    ['首段起始 > t（异常数据）→ 0', [360, 720], 0, 0],
  ])('%s', (_name, starts, t, want) => {
    expect(segmentIndexAt(segs(...starts), t)).toBe(want)
  })
})

describe('prefetchWindow：当前段 + 后续 span-1 段', () => {
  const s = segs(0, 360, 720, 1080)

  it('窗口末尾不越界（末段附近收敛到 length）', () => {
    expect(prefetchWindow(s, 1000, 3)).toEqual({ start: 2, end: 4 })
    expect(prefetchWindow(s, 0, 3)).toEqual({ start: 0, end: 3 })
  })

  it('空列表 → {0,0}（循环不执行）', () => {
    expect(prefetchWindow([], 100, 3)).toEqual({ start: 0, end: 0 })
  })
})

describe('nextSegmentIndex：下一段（startTime > t）', () => {
  const s = segs(0, 360, 720)

  it('t 在首段内 → 下一段是 1', () => {
    expect(nextSegmentIndex(s, 100)).toBe(1)
  })

  it('t 恰在边界（不含当前段）→ 下一段跳一格', () => {
    expect(nextSegmentIndex(s, 360)).toBe(2)
  })

  it('t 超过全部 → -1', () => {
    expect(nextSegmentIndex(s, 99999)).toBe(-1)
  })

  it('空列表 → -1', () => {
    expect(nextSegmentIndex([], 100)).toBe(-1)
  })
})

describe('shouldPrefetchNext：跨段预拉三条件', () => {
  const s = segs(0, 360, 720)
  const loaded = new Set(['s0'])

  it('未进入提前量窗口 → 不拉', () => {
    expect(shouldPrefetchNext(s, 100, 1, loaded)).toBe(false)
  })

  it('进入提前量窗口（距边界 < lead）→ 拉', () => {
    expect(shouldPrefetchNext(s, 359.5, 1, loaded)).toBe(true)
  })

  it('恰好在边界-lead 处（>= 语义）→ 拉', () => {
    expect(shouldPrefetchNext(s, 359, 1, loaded)).toBe(true)
  })

  it('下一段已装载 → 不拉（loadedSegs 去重）', () => {
    expect(shouldPrefetchNext(s, 359.5, 1, new Set(['s0', 's1']))).toBe(false)
  })

  it('没有下一段（播放到末尾）→ 不拉', () => {
    expect(shouldPrefetchNext(s, 9999, 1, loaded)).toBe(false)
  })
})

describe('resolveDanmuProvider：源选择回退链', () => {
  const list: DanmuProvider[] = [
    { provider: 'bilibili', resourceId: '44847' },
    { provider: 'tencent', resourceId: 'mzc00200' },
  ]

  it('候选为空 → null', () => {
    expect(resolveDanmuProvider([], 'tencent', 'bilibili')).toBeNull()
  })

  it('偏好命中 → 直接用，不回退标记', () => {
    const sel = resolveDanmuProvider(list, 'tencent', 'bilibili')
    expect(sel).toEqual({
      provider: 'tencent',
      entry: { provider: 'tencent', resourceId: 'mzc00200' },
      fellBack: false,
    })
  })

  it('偏好不在候选 → 回退默认平台', () => {
    const sel = resolveDanmuProvider(list, 'iqiyi', 'bilibili')
    expect(sel?.provider).toBe('bilibili')
    expect(sel?.fellBack).toBe(true)
  })

  it('偏好与默认都不在 → 取首个候选', () => {
    const onlyYouku: DanmuProvider[] = [{ provider: 'youku', resourceId: 'yk1' }]
    const sel = resolveDanmuProvider(onlyYouku, 'tencent', 'bilibili')
    expect(sel?.provider).toBe('youku')
    expect(sel?.fellBack).toBe(true)
  })

  it('空串偏好（localStorage 异常兜底）→ 按失效处理', () => {
    const sel = resolveDanmuProvider(list, '', 'bilibili')
    expect(sel?.provider).toBe('bilibili')
    expect(sel?.fellBack).toBe(true)
  })
})
