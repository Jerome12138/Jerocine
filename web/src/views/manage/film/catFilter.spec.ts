import { describe, it, expect } from 'vitest'
import { normalizeCatIds } from './catFilter'
import type { FilmClass } from '@/types/manage'

/**
 * D⑩ 级联祖先归一纯函数用例。
 *
 * 服务端契约(核查结论, server/internal/domain/repository/query.go CatIDs +
 * repository/mysql/search_repo.go): 非空时按 (pid IN ids OR cid IN ids) 精确成员匹配,
 * 服务端不做祖先展开。影片归属两态(spider/engine.go backfillPid):
 *   - 子分类影片: pid=顶级祖先 id, cid=子分类 id
 *   - 直挂影片:   pid=0, cid=分类自身 id
 * 因此请求集合缺顶级 id 时, 直挂该顶级的影片必漏 —— 归一必须在安全前提下补齐。
 */

/** 固定两级树: 电影(1) → [科幻(2), 动作(3)]; 剧集(10) → [国产(11)] */
function tree(): FilmClass[] {
  return [
    {
      id: 1, pid: 0, name: '电影', show: true, sidebarShow: true, filmVisible: true,
      children: [
        { id: 2, pid: 1, name: '科幻', show: true, sidebarShow: true, filmVisible: true },
        { id: 3, pid: 1, name: '动作', show: true, sidebarShow: true, filmVisible: true }
      ]
    },
    {
      id: 10, pid: 0, name: '剧集', show: true, sidebarShow: true, filmVisible: true,
      children: [{ id: 11, pid: 10, name: '国产', show: true, sidebarShow: true, filmVisible: true }]
    }
  ]
}

/** 深层树: 顶级(1) → [中层(2) → [叶子A(4), 叶子B(5)], 直挂子级(3)] */
function deepTree(): FilmClass[] {
  return [
    {
      id: 1, pid: 0, name: '顶级', show: true, sidebarShow: true, filmVisible: true,
      children: [
        {
          id: 2, pid: 1, name: '中层', show: true, sidebarShow: true, filmVisible: true,
          children: [
            { id: 4, pid: 1, name: '叶子A', show: true, sidebarShow: true, filmVisible: true },
            { id: 5, pid: 1, name: '叶子B', show: true, sidebarShow: true, filmVisible: true }
          ]
        },
        { id: 3, pid: 1, name: '直挂子级', show: true, sidebarShow: true, filmVisible: true }
      ]
    }
  ]
}

describe('normalizeCatIds 分类级联祖先归一（规格 D⑩）', () => {
  it('勾顶级: 顶级 id 原样进生效集且全部子孙并入(子孙展开是纯函数自身的防御语义)', () => {
    // 勾顶级传顶级 id: 服务端 pid IN (1) 命中全部子分类影片, cid IN (1) 命中直挂影片
    expect([...normalizeCatIds(new Set([1]), tree())]).toEqual([1, 2, 3])
  })

  it('仅勾部分子级: 祖先不反推补入(pid IN 父id 会把未勾兄弟子分类影片一并过匹配)', () => {
    // 只勾科幻(2), 动作(3)未勾 → 电影(1)绝不补入; 直挂"电影"的影片随本次筛选漏掉是预期取舍
    expect([...normalizeCatIds(new Set([2]), tree())]).toEqual([2])
  })

  it('子级全勾: 祖先补入生效集 —— 直挂该祖先的影片(pid=0,cid=父id)不再漏', () => {
    // 科幻+动作全勾 = 电影的整棵子树被覆盖 → 补齐电影(1), 补入只新增直挂影片, 无过匹配
    expect([...normalizeCatIds(new Set([2, 3]), tree())]).toEqual([2, 3, 1])
  })

  it('多个顶级各自独立补齐: 谁的子孙全勾才补谁', () => {
    expect([...normalizeCatIds(new Set([2, 3, 11]), tree())]).toEqual([2, 3, 11, 10, 1])
  })

  it('深层树: 补齐按「全部子孙已覆盖」判定 —— 中层全勾补中层, 顶级仍有未勾分支不补', () => {
    // 叶子A(4)+叶子B(5)全勾 → 中层(2)补齐; 直挂子级(3)未勾 → 顶级(1)不补
    expect([...normalizeCatIds(new Set([4, 5]), deepTree())]).toEqual([4, 5, 2])
  })

  it('深层树传递闭包: 中层补齐后顶级若因此全覆盖, 也随之补齐', () => {
    // 4+5 全勾补中层(2), 加上直挂子级(3)后顶级的整棵子树被覆盖 → 顶级(1)随之补齐
    expect([...normalizeCatIds(new Set([4, 5, 3]), deepTree())]).toEqual([4, 5, 3, 2, 1])
  })

  it('树上不存在的勾选 id 原样保留(树加载失败/陈旧勾选不丢用户筛选)', () => {
    expect([...normalizeCatIds(new Set([9]), tree())]).toEqual([9])
  })

  it('空勾选 → 空集(调用方据 size=0 不传 catIds)', () => {
    expect(normalizeCatIds(new Set(), tree()).size).toBe(0)
  })
})
