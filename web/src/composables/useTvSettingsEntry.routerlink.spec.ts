import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, type Component } from 'vue'
import { createRouter, createMemoryHistory, RouterLink, type Router } from 'vue-router'

/**
 * 「设置」入口守卫与 RouterLink 的集成契约(评审实测后固化)。
 *
 * 背景: HomeView 的「设置」功能卡是 <RouterLink to="/settings">, 原生壳里要拦掉默认
 * 导航、改开原生抽屉。仅靠 `@click`(冒泡阶段) + e.preventDefault() **拦不住** ——
 * RouterLink 自己也在同一个 <a> 上绑了 navigate, Vue 合并同名 handler 后的执行顺序
 * 让 navigate 先跑完。只有捕获阶段(`@click.capture`)先于它, preventDefault 才有效。
 *
 * 这组用例把这个行为钉住:
 *  - 正例: capture ⇒ 不导航(我们依赖的行为);
 *  - 反例: 冒泡 ⇒ 仍导航(记录 vue-router 现状; 若哪天它变了, 反例会红, 提示重新评估)。
 */

const Blank = { template: '<div>blank</div>' }
const Settings = { template: '<div>settings</div>' }

async function makeRouter(): Promise<Router> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: Blank },
      { path: '/settings', component: Settings }
    ]
  })
  await router.push('/')
  await router.isReady()
  return router
}

/** 挂一个只含 RouterLink 的最小宿主, 便于精确观察导航是否发生 */
function mountLink(
  router: Router,
  props: Record<string, unknown>
): { wrapper: ReturnType<typeof mount> } {
  const Comp = defineComponent({
    setup() {
      return () => h(RouterLink as unknown as Component, props)
    }
  })
  const wrapper = mount(Comp, { global: { plugins: [router] } })
  return { wrapper }
}

/** 模拟一次左键点击, 并让 router 的异步导航落定 */
async function clickLink(wrapper: ReturnType<typeof mount>): Promise<void> {
  await wrapper.find('a').trigger('click')
  await new Promise((r) => setTimeout(r, 0))
}

describe('useTvSettingsEntry.guard × RouterLink 契约', () => {
  let router: Router

  beforeEach(async () => {
    router = await makeRouter()
  })

  it('捕获阶段(@click.capture) preventDefault 能拦下导航 — HomeView 依赖此行为', async () => {
    const pushSpy = vi.spyOn(router, 'push')
    const guard = vi.fn((e?: { preventDefault: () => void }) => {
      // 纯网页宿主时 guard 会直接 return(不 preventDefault); 这里模拟原生壳分支
      e?.preventDefault()
    })

    const { wrapper } = mountLink(router, { to: '/settings', onClickCapture: guard })
    await clickLink(wrapper)

    expect(guard).toHaveBeenCalledTimes(1)
    expect(pushSpy).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('冒泡阶段(@click) 拦不住 — 记录 vue-router 现状, 故必须用 capture', async () => {
    const pushSpy = vi.spyOn(router, 'push')
    const guard = vi.fn((e?: { preventDefault: () => void }) => {
      e?.preventDefault()
    })

    const { wrapper } = mountLink(router, { to: '/settings', onClick: guard })
    await clickLink(wrapper)

    // guard 确实被调用了, 但 navigate 已经先跑 ⇒ 导航仍然发生。
    // 这个断言若变红, 说明 vue-router 行为变了 —— 可以重新评估是否还需要捕获阶段。
    expect(guard).toHaveBeenCalledTimes(1)
    expect(pushSpy).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.path).toBe('/settings')
  })

  it('对照: 不绑守卫 ⇒ 正常导航(确认宿主本身没有干扰导航)', async () => {
    const { wrapper } = mountLink(router, { to: '/settings' })
    await clickLink(wrapper)

    expect(router.currentRoute.value.path).toBe('/settings')
  })
})
