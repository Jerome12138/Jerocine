import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import BasePagePlaceholder from './BasePagePlaceholder.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

async function mountIt(props: { title: string; description?: string }, path = '/foo?q=1') {
  await router.push(path)
  await router.isReady()
  return mount(BasePagePlaceholder, { props, global: { plugins: [router] } })
}

describe('BasePagePlaceholder', () => {
  it('显示 title', async () => {
    const w = await mountIt({ title: '待开发' })
    expect(w.find('h1').text()).toBe('待开发')
  })

  it('description 显示', async () => {
    const w = await mountIt({ title: 'X', description: '占位说明' })
    expect(w.text()).toContain('占位说明')
  })

  it('显示当前路径 fullPath', async () => {
    const w = await mountIt({ title: 'X' }, '/manage/foo?q=1')
    expect(w.text()).toContain('/manage/foo')
  })

  it('当 query 有内容时显示 Query JSON', async () => {
    const w = await mountIt({ title: 'X' }, '/x?k=v&a=b')
    expect(w.text()).toContain('Query')
    expect(w.text()).toContain('k')
  })

  it('无 description 时不渲染 p 描述行', async () => {
    const w = await mountIt({ title: 'X' })
    expect(w.findAll('p').length).toBe(0)
  })
})
