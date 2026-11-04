import { describe, it, expect } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import SectionHeader from './SectionHeader.vue'

/**
 * W1e 桌面区块标题 — h2 + 右上「更多 ›」(规格 §1 W1e, TV 版 jc-tv-sec 不做):
 * - 默认类 = ClassifyView/FilmRow 桌面同款(header: flex items-end justify-between gap-4 mb-3;
 *   h2: fs-lg 粗体 primary snug; 链接: text-link fs-sm + data-focusable + tabindex=0);
 * - headerClass/titleClass 提供 = 整组替换默认类(FilmRow/Home/继续观看的页面钩子类逐字保留,
 *   TV 字号/安全区覆盖依赖这些类被全局选择器命中);
 * - 默认插槽 = 右侧内容整体替换「更多›」(继续观看行 hover 链接 / 猜你喜欢 tip 形态);
 * - bare = 只渲染裸 h2(RelatedList 形态)。
 */
const stubs = { RouterLink: RouterLinkStub }

describe('SectionHeader', () => {
  it('默认: header 类 + h2 默认类(ClassifyView 原款)', () => {
    const w = mount(SectionHeader, { props: { title: '最新上映' }, global: { stubs } })
    const header = w.find('header')
    expect(header.classes()).toContain('flex')
    expect(header.classes()).toContain('items-end')
    expect(header.classes()).toContain('justify-between')
    expect(header.classes()).toContain('gap-[var(--jc-space-4)]')
    expect(header.classes()).toContain('mb-[var(--jc-space-3)]')

    const h2 = w.find('h2')
    expect(h2.classes()).toContain('text-[length:var(--jc-fs-lg)]')
    expect(h2.classes()).toContain('font-[var(--jc-fw-bold)]')
    expect(h2.classes()).toContain('text-primary')
    expect(h2.classes()).toContain('leading-[var(--jc-lh-snug)]')
    expect(h2.text()).toBe('最新上映')
  })

  it('moreTo → 默认「更多›」RouterLink(data-focusable + tabindex=0)', () => {
    const w = mount(SectionHeader, {
      props: { title: 'x', moreTo: '/classify?sort=new' },
      global: { stubs }
    })
    const link = w.find('a')
    expect(link.exists()).toBe(true)
    expect(link.attributes('data-focusable')).toBe('true')
    expect(link.attributes('tabindex')).toBe('0')
    expect(link.classes()).toContain('text-link')
    expect(link.classes()).toContain('text-[length:var(--jc-fs-sm)]')
    expect(link.text()).toContain('更多')
  })

  it('无 moreTo 且无插槽 → 不渲染右侧链接', () => {
    const w = mount(SectionHeader, { props: { title: 'x' }, global: { stubs } })
    expect(w.find('a').exists()).toBe(false)
  })

  it('默认插槽整体替换右侧(不渲染默认更多链接)', () => {
    const w = mount(SectionHeader, {
      props: { title: '猜你喜欢', moreTo: '/classify' },
      global: { stubs },
      slots: { default: '<span class="tip">基于浏览数据混合推荐</span>' }
    })
    expect(w.find('a').exists()).toBe(false)
    expect(w.find('.tip').exists()).toBe(true)
  })

  it('headerClass/titleClass 提供 = 整组替换默认类(FilmRow 钩子类逐字保留)', () => {
    const w = mount(SectionHeader, {
      props: {
        title: '热播榜',
        moreTo: '/classify',
        headerClass:
          'container-page flex items-end justify-between gap-[var(--jc-space-4)] mb-[var(--jc-space-3)]',
        titleClass:
          'jc-film-row__title text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)] text-primary leading-[var(--jc-lh-snug)]'
      },
      global: { stubs }
    })
    expect(w.find('header').classes()).toContain('container-page')
    expect(w.find('header').classes()).not.toContain('flex-first-placeholder')
    expect(w.find('h2').classes()).toContain('jc-film-row__title')
  })

  it('bare: 只渲染裸 h2(RelatedList 形态, 无 header 包装/右侧区)', () => {
    const w = mount(SectionHeader, {
      props: { title: '相关推荐', bare: true, moreTo: '/classify' },
      global: { stubs }
    })
    expect(w.find('header').exists()).toBe(false)
    expect(w.find('a').exists()).toBe(false)
    expect(w.find('h2').text()).toBe('相关推荐')
    expect(w.find('h2').classes()).toContain('text-[length:var(--jc-fs-lg)]')
  })

  it('无 title → 不渲染 h2(header 仍渲染)', () => {
    const w = mount(SectionHeader, { props: { moreTo: '/classify' }, global: { stubs } })
    expect(w.find('h2').exists()).toBe(false)
    expect(w.find('header').exists()).toBe(true)
  })
})
