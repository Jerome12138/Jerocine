import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BannerListView from './BannerListView.vue'
import type { Banner, BannerBoard, EffectiveSlide, InactiveBanner } from '@/types/manage'

const h = vi.hoisted(() => ({
  board: vi.fn(),
  move: vi.fn(),
  save: vi.fn(),
  remove: vi.fn(),
  disableByMid: vi.fn(),
  refreshHotBoard: vi.fn(),
  fillBackdrop: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    banner: {
      board: h.board,
      move: h.move,
      save: h.save,
      remove: h.remove,
      disableByMid: h.disableByMid,
      refreshHotBoard: h.refreshHotBoard,
      fillBackdrop: h.fillBackdrop,
      // 豆瓣链接按真实实现拼 URL(行渲染时会调用, mock 缺失会让整棵组件树渲染中断)
      doubanSearchUrl: (name: string) =>
        `https://search.douban.com/movie/subject_search?search_text=${encodeURIComponent(name)}`
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

// ManageImageUpload stub: 点击即写入一张横图, 供新增表单通过「至少一图」校验
const ImgUploadStub = {
  props: ['modelValue'],
  emits: ['update:modelValue'],
  template: `<button type="button" class="img-upload-stub" @click="$emit('update:modelValue', 'http://x/a.jpg')">{{ modelValue }}</button>`
}

function slide(over: Partial<EffectiveSlide> = {}): EffectiveSlide {
  return {
    source: 'fallback',
    mid: 1,
    name: '影片',
    image: 'x.jpg',
    ...over
  }
}

function board(over: Partial<BannerBoard> = {}): BannerBoard {
  return {
    active: Array.from({ length: 10 }, (_, i) =>
      slide({ mid: i + 1, name: `片${i + 1}`, source: i < 2 ? 'banner' : 'fallback' })
    ),
    liveCount: 5,
    inactive: [],
    ...over
  }
}

/** 手动位配置行(id/state/sort 由用例覆盖) */
function bannerRow(over: Partial<Banner> = {}): Banner {
  return {
    id: 7, title: '手动A', subtitle: '', image: 'a.jpg', poster: '',
    mid: 1, link: '', sort: 3, state: 0, startAt: 0, endAt: 0,
    ...over
  }
}

async function mountView(): Promise<VueWrapper> {
  const w = mount(BannerListView, {
    global: {
      plugins: [createPinia()],
      // RouterLink: 测试环境无路由; teleport: ManageSheet 内容内联渲染可查;
      // ManageImageUpload: 点击写入横图的 stub, 供表单通过图片校验
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        teleport: true,
        ManageImageUpload: ImgUploadStub
      }
    }
  })
  await flushPromises()
  return w
}

/** 找文案含 text 的第一个按钮 */
function btn(w: VueWrapper, text: string) {
  const found = w.findAll('button').find((b) => b.text().includes(text))
  expect(found, `应渲染「${text}」按钮`).toBeTruthy()
  return found!
}

/** 定位某行的 li, 返回行内文案含 text 的按钮查找器 */
function rowOps(w: VueWrapper, rowText: string) {
  const li = w.findAll('li').find((l) => l.text().includes(rowText))
  expect(li, `应渲染「${rowText}」所在行`).toBeTruthy()
  return (text: string) => {
    const found = li!.findAll('button').find((b) => b.text().includes(text))
    expect(found, `行内应渲染「${text}」按钮`).toBeTruthy()
    return found!
  }
}

/** 展开未生效折叠区 */
async function expandInactive(w: VueWrapper): Promise<void> {
  await btn(w, '未生效（').trigger('click')
  await flushPromises()
}

describe('BannerListView 轮播管理(生效位 + 候选位)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.board.mockResolvedValue(board())
  })

  it('展示全部 10 位, 并在 liveCount 处分区(生效位/候选位提示)', async () => {
    const w = await mountView()
    expect(w.text()).toContain('以下为候选位')
    expect(w.text()).toContain('前 5 位对外生效')
    // 10 个数据行(分区提示行不算): 每行有编号或名称
    expect(w.text()).toContain('片10')
  })

  it('无候选位(全部手动且不足 liveCount)时不渲染分区提示', async () => {
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', name: 'A' }), slide({ source: 'banner', name: 'B' })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    expect(w.text()).not.toContain('以下为候选位')
    expect(w.text()).toContain('A')
  })

  it('liveCount 缺省(旧后端)按 5 处理', async () => {
    const b = board()
    delete (b as Partial<BannerBoard>).liveCount
    h.board.mockResolvedValue(b)
    const w = await mountView()
    expect(w.text()).toContain('前 5 位对外生效')
  })

  it('行内展示轮播元信息(评分/标签/榜位)且关联影片可跳详情', async () => {
    h.board.mockResolvedValue({
      active: [
        slide({
          source: 'fallback',
          mid: 160975,
          name: '好片',
          dbScore: 8.7,
          classTag: '动作,冒险',
          hotRank: 3,
          hotBoard: '热门电影'
        })
      ],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    expect(w.text()).toContain('8.7')
    expect(w.text()).toContain('动作,冒险')
    expect(w.text()).toContain('热门电影 No.3')
    // 跳转列: 关联影片渲染为链接(点击进详情页), 文案保持「影片 #mid」
    const link = w.findAll('a').find((a) => a.text().includes('影片 #160975'))
    expect(link, '关联影片应渲染为可点击跳转').toBeTruthy()
  })

  it('自定义链接行跳转列仅展示文本(不渲染跳详情链接)', async () => {
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', mid: 0, name: '活动位', link: '/filmClassify?Pid=1' })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    expect(w.text()).toContain('/filmClassify?Pid=1')
    expect(w.findAll('a').find((a) => a.text().includes('filmClassify'))).toBeUndefined()
  })

  it('「立即刷新榜单」确认后调 refreshHotBoard, 完成后 toast 本轮报告并重载', async () => {
    h.confirm.mockResolvedValue(true)
    h.refreshHotBoard.mockResolvedValue({ fetched: 627, matched: 465, applied: 505, elapsedMs: 97123 })
    const w = await mountView()
    await btn(w, '立即刷新榜单').trigger('click')
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '立即刷新榜单？' }))
    await flushPromises()
    expect(h.refreshHotBoard).toHaveBeenCalledTimes(1)
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('627'))
    expect(h.board).toHaveBeenCalledTimes(2) // 初载 + 完成后 load(true)
  })

  it('「刷新榜单」失败时 toast error(不再用阻塞式 alert)', async () => {
    h.confirm.mockResolvedValue(true)
    h.refreshHotBoard.mockRejectedValue(new Error('上一轮刷新未结束'))
    const w = await mountView()
    await btn(w, '立即刷新榜单').trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('上一轮刷新未结束'))
  })

  it('「横图」按钮: 调 fillBackdrop 并按 status toast(filled→success / already→info)', async () => {
    h.fillBackdrop.mockResolvedValue({ mid: 1, status: 'filled' })
    const w = await mountView()
    const b = btn(w, '回填横图')
    await b.trigger('click')
    await flushPromises()
    expect(h.fillBackdrop).toHaveBeenCalledWith(1)
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('横图已获取'))

    h.fillBackdrop.mockResolvedValue({ mid: 1, status: 'already' })
    await b.trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('info', expect.stringContaining('该片已有横图'))
  })

  it('初次加载失败时渲染错误空态(而非误导性的「暂无生效轮播」)', async () => {
    h.board.mockRejectedValue(new Error('boom'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('boom')
    expect(w.text()).toContain('点击右上角「刷新」重试')
    expect(w.text()).not.toContain('暂无生效轮播')
  })
})

describe('BannerListView 行内操作(move/toggle/disable/remove)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.board.mockResolvedValue(board())
    h.move.mockResolvedValue(board())
    h.disableByMid.mockResolvedValue(board())
  })

  it('↑↓ 调 move 并用返回的新 Board 回读', async () => {
    const w = await mountView()
    h.move.mockResolvedValue({
      ...board(),
      active: board().active.map((s, i) => ({ ...s, name: `新位${i + 1}` }))
    })
    await rowOps(w, '片2')('↑').trigger('click')
    await flushPromises()
    expect(h.move).toHaveBeenCalledWith(1, 'up')
    expect(w.text()).toContain('新位2')
  })

  it('move 进行中(inFlight)连点被守卫, 结束后可再次操作', async () => {
    const w = await mountView()
    let resolveMove!: (b: BannerBoard) => void
    h.move.mockImplementation(
      () => new Promise<BannerBoard>((res) => { resolveMove = res })
    )
    const up = rowOps(w, '片2')('↑')
    await up.trigger('click')
    await up.trigger('click') // inFlight 中: 守卫直接 return
    expect(h.move).toHaveBeenCalledTimes(1)
    resolveMove(board())
    await flushPromises()
    await up.trigger('click')
    expect(h.move).toHaveBeenCalledTimes(2)
  })

  it('手动位边界: 首行↑禁用、末位手动行↓禁用、自动位↓禁用', async () => {
    const w = await mountView()
    // 片1 = 手动位第 0 行: 上移边界
    expect(rowOps(w, '片1')('↑').attributes('disabled')).toBeDefined()
    // 片2 = 手动位最后一行: 下移边界
    expect(rowOps(w, '片2')('↓').attributes('disabled')).toBeDefined()
    // 片3 起为自动位: 恒在尾部, ↓ 禁用; ↑ 可用(采纳转手动)
    expect(rowOps(w, '片3')('↓').attributes('disabled')).toBeDefined()
    expect(rowOps(w, '片3')('↑').attributes('disabled')).toBeUndefined()
  })

  it('手动位「禁用」: save 携带整行 + state 翻转, 完成后回读', async () => {
    const row = bannerRow({ id: 7, state: 0 })
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', mid: 1, name: '片1', banner: row })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    await rowOps(w, '片1')('禁用').trigger('click')
    await flushPromises()
    // toggleState 只传 payload 一个实参
    expect(h.save).toHaveBeenCalledWith(
      expect.objectContaining({ id: 7, state: 1, sort: row.sort })
    )
    expect(h.board).toHaveBeenCalledTimes(2) // 初载 + load(true)
  })

  it('自动位「禁用」: 调 disableByMid 屏蔽(不再内联 promise)', async () => {
    const w = await mountView()
    await rowOps(w, '片3')('禁用').trigger('click')
    await flushPromises()
    expect(h.disableByMid).toHaveBeenCalledWith(3)
    expect(h.save).not.toHaveBeenCalled()
  })

  it('删除需确认: 确认后 remove + 回读, 取消则不动', async () => {
    const row = bannerRow({ id: 7 })
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', mid: 1, name: '片1', banner: row })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()

    h.confirm.mockResolvedValue(false)
    await rowOps(w, '片1')('删除').trigger('click')
    await flushPromises()
    expect(h.remove).not.toHaveBeenCalled()

    h.confirm.mockResolvedValue(true)
    await rowOps(w, '片1')('删除').trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '确认删除这张轮播？' }))
    expect(h.remove).toHaveBeenCalledWith(7)
    expect(h.board).toHaveBeenCalledTimes(2)
  })
})

describe('BannerListView 表单(新增/编辑/采纳)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.board.mockResolvedValue(board())
    h.save.mockResolvedValue(bannerRow())
  })

  it('三条校验分支: 缺图 / 缺跳转 / 结束早于开始 → toast error 且不提交', async () => {
    const w = await mountView()
    await btn(w, '新增').trigger('click')
    const saveBtn = btn(w, '保存')

    await saveBtn.trigger('click')
    expect(h.toast).toHaveBeenCalledWith('error', '至少需要上传横图或竖图')
    expect(h.save).not.toHaveBeenCalled()

    // stub 点击写入横图, 但仍无跳转
    await w.find('.img-upload-stub').trigger('click')
    await saveBtn.trigger('click')
    expect(h.toast).toHaveBeenCalledWith('error', '需要配置跳转：关联影片 ID 或自定义链接')
    expect(h.save).not.toHaveBeenCalled()

    // 填 mid + 起止时间(结束 < 开始)
    await w.find('input[placeholder="如 12345"]').setValue('123')
    const times = w.findAll('input[type="datetime-local"]')
    await times[0].setValue('2026-01-02T08:00')
    await times[1].setValue('2026-01-01T08:00')
    await saveBtn.trigger('click')
    expect(h.toast).toHaveBeenCalledWith('error', '结束时间不能早于开始时间')
    expect(h.save).not.toHaveBeenCalled()
  })

  it('新增保存: id 0 / sort 0(服务端自行 maxSort+1) / 不带 slot', async () => {
    const w = await mountView()
    await btn(w, '新增').trigger('click')
    await w.find('.img-upload-stub').trigger('click')
    await w.find('input[placeholder="如 12345"]').setValue('123')
    await btn(w, '保存').trigger('click')
    await flushPromises()
    expect(h.save).toHaveBeenCalledTimes(1)
    const [payload, slot] = h.save.mock.calls[0]
    expect(payload).toMatchObject({ id: 0, sort: 0, mid: 123, state: 0 })
    expect(slot).toBeUndefined()
    expect(h.board).toHaveBeenCalledTimes(2) // 初载 + 保存后 load(true)
  })

  it('编辑保存: 原样回传 row.sort(id>0 → save 走 PUT 路由), 不再固定 sort: 0', async () => {
    const row = bannerRow({ id: 7, sort: 3, title: '手动A' })
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', mid: 1, name: '片1', banner: row })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    await rowOps(w, '片1')('编辑').trigger('click')
    // 表单已预填, 直接保存(只改标题的场景也应保住排序)
    await btn(w, '保存').trigger('click')
    await flushPromises()
    expect(h.save).toHaveBeenCalledTimes(1)
    const [payload, slot] = h.save.mock.calls[0]
    expect(payload).toMatchObject({ id: 7, sort: 3, title: '手动A' })
    expect(slot).toBeUndefined()
  })

  it('编辑未生效行同样回传 sort(从折叠区进入)', async () => {
    const row = bannerRow({ id: 21, sort: 9, title: '过期位', state: 0 })
    h.board.mockResolvedValue({
      ...board(),
      inactive: [{ ...row, reason: 'expired' }]
    })
    const w = await mountView()
    await expandInactive(w)
    await rowOps(w, '过期位')('编辑').trigger('click')
    await btn(w, '保存').trigger('click')
    await flushPromises()
    expect(h.save).toHaveBeenCalledWith(
      expect.objectContaining({ id: 21, sort: 9 }),
      undefined
    )
  })

  it('采纳自动位: 预填生效内容, save 第二参透传钉入位置 slot', async () => {
    const w = await mountView()
    // board() 默认前 2 位手动, 片3 在下标 2(候选位第一行)
    await rowOps(w, '片3')('编辑').trigger('click')
    await btn(w, '保存').trigger('click')
    await flushPromises()
    expect(h.save).toHaveBeenCalledTimes(1)
    const [payload, slot] = h.save.mock.calls[0]
    expect(payload).toMatchObject({ id: 0, sort: 0, mid: 3 })
    expect(slot).toBe(2)
  })
})

describe('BannerListView 未生效折叠区', () => {
  /** 本地时区构造 ms, 保证 msToLocal 的显示断言与机器时区无关 */
  const startMs = new Date(2026, 0, 1, 8, 0).getTime()
  const endMs = new Date(2026, 1, 1, 8, 0).getTime()

  function inactiveBoard(rows: InactiveBanner[]): BannerBoard {
    return { ...board(), inactive: rows }
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.board.mockResolvedValue(
      inactiveBoard([
        {
          id: 11, title: '过期位', subtitle: '', image: 'i.jpg', poster: 'p.jpg',
          mid: 0, link: '', sort: 9, state: 0, startAt: startMs, endAt: endMs,
          reason: 'expired'
        },
        {
          id: 12, title: '', subtitle: '', image: '', poster: '',
          mid: 55, link: '', sort: 10, state: 1, startAt: 0, endAt: 0,
          reason: 'disabled'
        },
        {
          id: 13, title: '有图停用', subtitle: '', image: 'b.jpg', poster: '',
          mid: 0, link: '', sort: 11, state: 1, startAt: 0, endAt: 0,
          reason: 'disabled'
        }
      ])
    )
  })

  it('渲染原因标签 / 展示名兜底(影片 #mid)与排期时间', async () => {
    const w = await mountView()
    await expandInactive(w)
    expect(w.text()).toContain('已过期')
    expect(w.text()).toContain('已停用')
    // inactiveName: 配置标题 > meta.name > 影片 #mid
    expect(w.text()).toContain('过期位')
    expect(w.text()).toContain('影片 55')
    // 排期时间(需求⑦): 过期行展示起止, 无排期行不显示
    expect(w.text()).toContain('2026-01-01 08:00 ~ 2026-02-01 08:00')
  })

  it('纯屏蔽行(reason=disabled 且无图)不渲染必 422 的「启用」按钮; 有图停用行保留', async () => {
    const w = await mountView()
    await expandInactive(w)
    // 无图的纯屏蔽行(#12): 无启用按钮
    const blocked = w.findAll('li').find((l) => l.text().includes('影片 55'))
    expect(blocked, '应渲染纯屏蔽行').toBeTruthy()
    expect(blocked!.findAll('button').some((b) => b.text().includes('启用'))).toBe(false)
    // 有图停用行(#13)与已过期行(#11, state=0)分别可启用/不可启用
    const withImg = w.findAll('li').find((l) => l.text().includes('有图停用'))
    expect(withImg!.findAll('button').some((b) => b.text().includes('启用'))).toBe(true)
    const expired = w.findAll('li').find((l) => l.text().includes('过期位'))
    expect(expired!.findAll('button').some((b) => b.text().includes('启用'))).toBe(false)
  })

  it('无关联影片元信息的行显示兜底文案; 启用按钮调 toggleState', async () => {
    const w = await mountView()
    await expandInactive(w)
    expect(w.text()).toContain('无关联影片元信息')
    // 有图停用行点「启用」→ save(state 翻转为 0)
    const withImg = w.findAll('li').find((l) => l.text().includes('有图停用'))
    const enable = withImg!.findAll('button').find((b) => b.text().includes('启用'))
    await enable!.trigger('click')
    await flushPromises()
    expect(h.save).toHaveBeenCalledWith(
      expect.objectContaining({ id: 13, state: 0, sort: 11 })
    )
  })
})
