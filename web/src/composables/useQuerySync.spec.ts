import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { useQuerySync } from './useQuerySync'

let router: Router

function mountHost(initial: Record<string, unknown>) {
  const Host = defineComponent({
    setup() {
      const api = useQuerySync(initial as never)
      return { api }
    },
    render() { return h('div') }
  })
  return mount(Host, { global: { plugins: [router] } })
}

describe('useQuerySync', () => {
  beforeEach(async () => {
    router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
    })
    await router.push('/')
    await router.isReady()
  })

  it('初始 params = initial 默认值 (URL 空 query)', async () => {
    const w = mountHost({ search: '', current: 1 })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    expect(api.params.value).toEqual({ search: '', current: 1 })
  })

  it('URL 带 query → 初始 params 读取并按 initial 类型转换', async () => {
    await router.push('/?search=foo&current=3')
    const w = mountHost({ search: '', current: 1 })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    expect(api.params.value.search).toBe('foo')
    expect(api.params.value.current).toBe(3) // 转 number
  })

  it('push() 改 URL query (跳过 undefined/null/空值)', async () => {
    const w = mountHost({ search: '', current: 1 })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    await api.push({ search: 'abc', current: 5 } as never)
    expect(router.currentRoute.value.query.search).toBe('abc')
    expect(router.currentRoute.value.query.current).toBe('5')
  })

  it('push 空值 → 该字段从 URL 移除', async () => {
    await router.push('/?search=keep')
    const w = mountHost({ search: '', current: 1 })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    await api.push({ search: '' } as never)
    expect(router.currentRoute.value.query.search).toBeUndefined()
  })

  it('外部改 URL → params 自动跟随', async () => {
    const w = mountHost({ search: '', current: 1 })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    await router.push('/?search=ext&current=9')
    await w.vm.$nextTick()
    await w.vm.$nextTick()
    expect(api.params.value.search).toBe('ext')
    expect(api.params.value.current).toBe(9)
  })

  it('boolean 字段: query "true" → true, "false" → false', async () => {
    await router.push('/?flag=true')
    const w = mountHost({ flag: false })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    expect(api.params.value.flag).toBe(true)
  })

  it('数字字段无效值 → fallback 到 initial', async () => {
    await router.push('/?current=abc')
    const w = mountHost({ current: 1 })
    const api = (w.vm as never as { api: ReturnType<typeof useQuerySync> }).api
    expect(api.params.value.current).toBe(1)
  })
})
