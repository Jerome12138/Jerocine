import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TvSectionHeader from './TvSectionHeader.vue'

/**
 * TvSectionHeader(§C8#5) — TV 模式区块标题(jc-tv-sec 三段式)原子。
 * 收编 8 文件 14 处手拼 div.jc-tv-sec(样式自 tv-cards.css 整类迁入本件)。
 * 契约:
 *   - props title/sub → .t/.s 两 span(缺 sub 不渲染 .s)
 *   - #more 槽 → 右侧「更多/查看全部」入口(data-focusable/tabindex 由宿主逐项保留)
 *   - 默认槽 → 异形头(ClassifySearch/HotBoard 计数头、ClassifyView 渐变标题)整段自绘
 *   - 类名 jc-tv-sec/.t/.s 保持原样, 供宿主页 scoped 覆盖(如 jc-classify-tv__title 对齐)与
 *     全局 jc-tv-more 胶囊协作
 */
describe('TvSectionHeader', () => {
  it('title/sub: 容器 jc-tv-sec + .t/.s 两 span, 文案透传', () => {
    const w = mount(TvSectionHeader, { props: { title: '🔥 热门榜单', sub: '最热抢先看' } })
    expect(w.find('.jc-tv-sec').exists()).toBe(true)
    expect(w.find('.t').text()).toBe('🔥 热门榜单')
    expect(w.find('.s').text()).toBe('最热抢先看')
  })

  it('sub 省略: 只渲染 .t(FilmDetailView「相关推荐」原款)', () => {
    const w = mount(TvSectionHeader, { props: { title: '相关推荐' } })
    expect(w.find('.t').exists()).toBe(true)
    expect(w.find('.s').exists()).toBe(false)
  })

  it('#more 槽: 链接内容在标题之后, data-focusable 逐项保留(宿主传入)', () => {
    const w = mount(TvSectionHeader, {
      props: { title: '⏱ 近期历史' },
      slots: {
        // 宿主 RouterLink 原样传入(此处以等构 a 元素代表, RouterLink 渲染即 a)
        more: '<a href="#/history" class="jc-tv-more" data-focusable="true" tabindex="0">全部</a>'
      }
    })
    const more = w.find('.jc-tv-more')
    expect(more.exists()).toBe(true)
    expect(more.attributes('data-focusable')).toBe('true')
    expect(more.attributes('tabindex')).toBe('0')
    // 槽内容排在 .t 之后(视觉顺序: 标题 → 副标题 → 右侧链接)
    const children = w.find('.jc-tv-sec').element.children
    expect(children[children.length - 1]).toBe(more.element)
  })

  it('默认槽: 异形头整段自绘(容器仍带 jc-tv-sec, 原样迁入的宿主异形 span 不受影响)', () => {
    const w = mount(TvSectionHeader, {
      slots: {
        default:
          '<span class="jc-hb-tv__title">热榜 · 周放映榜</span><span class="jc-cs-tv__count">共 12 部</span>'
      }
    })
    expect(w.find('.jc-tv-sec').exists()).toBe(true)
    expect(w.find('.jc-hb-tv__title').text()).toBe('热榜 · 周放映榜')
    expect(w.find('.jc-cs-tv__count').text()).toBe('共 12 部')
    expect(w.find('.t').exists()).toBe(false)
  })

  it('title/sub 与 #more 槽可同用(HomeView 专区面板原款三段式)', () => {
    const w = mount(TvSectionHeader, {
      props: { title: '🆕 最新上架', sub: '每日更新' },
      slots: { more: '<a class="jc-tv-more" data-focusable="true">查看全部</a>' }
    })
    expect(w.find('.t').exists()).toBe(true)
    expect(w.find('.s').exists()).toBe(true)
    expect(w.find('.jc-tv-more').exists()).toBe(true)
  })
})
