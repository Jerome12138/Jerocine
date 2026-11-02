import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import type { SourceSearchHit } from '@/types/manage'
import SourceCandidateDialog from './SourceCandidateDialog.vue'

function hit(over: Partial<SourceSearchHit> = {}): SourceSearchHit {
  return {
    sourceVodId: 101,
    name: '甄嬛传',
    year: 2011,
    typeName: '古装',
    remarks: 'HD',
    cover: 'http://x/c.jpg',
    episodes: 76,
    ...over
  }
}

function clearBody(): void {
  // 清测试间 Teleport 残留 (不操作生产用户输入, 仅测试隔离)
  while (document.body.firstChild) {
    document.body.removeChild(document.body.firstChild)
  }
}

function mountDialog(props: Record<string, unknown> = {}): ReturnType<typeof mount> {
  return mount(SourceCandidateDialog, {
    props: { visible: true, candidates: [], ...props },
    attachTo: document.body
  })
}

describe('SourceCandidateDialog', () => {
  beforeEach(() => clearBody())

  it('visible=true + 候选列表 → 渲染每项名称/年份/集数', async () => {
    const w = mountDialog({
      sourceName: '开关源',
      candidates: [hit(), hit({ sourceVodId: 202, year: 2024, episodes: 5, name: '甄嬛传2024' })]
    })
    await w.vm.$nextTick()
    const items = document.querySelectorAll('li[data-focusable="true"]')
    expect(items.length).toBe(2)
    expect(document.body.textContent).toContain('甄嬛传')
    expect(document.body.textContent).toContain('甄嬛传2024')
    expect(document.body.textContent).toContain('2011')
    expect(document.body.textContent).toContain('76 集')
    w.unmount()
  })

  it('标题含来源名; 点选候选 → emit pick(该候选)', async () => {
    const h = hit()
    const w = mountDialog({ sourceName: '飞飞', candidates: [h] })
    await w.vm.$nextTick()
    expect(document.body.textContent).toContain('选择版本 · 飞飞')

    ;(document.querySelector('li[data-focusable="true"]') as HTMLElement).click()
    await w.vm.$nextTick()
    expect(w.emitted('pick')?.length).toBe(1)
    expect(w.emitted('pick')?.[0]).toEqual([h])
    w.unmount()
  })

  it('Enter/Space 键盘选择 → emit pick (TV 遥控可达)', async () => {
    const w = mountDialog({ candidates: [hit()] })
    await w.vm.$nextTick()
    const item = document.querySelector('li[data-focusable="true"]') as HTMLElement
    item.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('pick')?.length).toBe(1)
    w.unmount()
  })

  it('picking=true → 候选锁定交互且点击不再 emit pick', async () => {
    const w = mountDialog({ candidates: [hit()], picking: true })
    await w.vm.$nextTick()
    const item = document.querySelector('li[data-focusable="true"]') as HTMLElement
    expect(item.className).toContain('pointer-events-none')
    item.click()
    await w.vm.$nextTick()
    expect(w.emitted('pick')).toBeUndefined()
    w.unmount()
  })

  it('空候选 → BaseEmpty 渲染「无候选版本」', async () => {
    const w = mountDialog({ candidates: [] })
    await w.vm.$nextTick()
    expect(document.querySelector('.jc-empty')).not.toBeNull()
    expect(document.body.textContent).toContain('无候选版本')
    w.unmount()
  })

  it('visible=false → 不渲染', async () => {
    const w = mount(SourceCandidateDialog, {
      props: { visible: false, candidates: [hit()] },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('li[data-focusable="true"]')).toBeNull()
    w.unmount()
  })
})
