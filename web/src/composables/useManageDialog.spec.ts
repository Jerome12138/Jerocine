import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import ManageDialogHost from '@/components/manage/ManageDialogHost.vue'
import {
  openManageDialog,
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from './useManageDialog'
import { useViewMode } from '@/composables/useViewMode'

/** 内容组件桩: 遵守契约 emits('confirm', payload) | emits('cancel') */
const DialogStub = defineComponent({
  props: { initial: { type: Object, default: null } },
  emits: ['confirm', 'cancel'],
  template: '<div class="dialog-stub">stub:{{ initial?.name }}</div>'
})

async function flush(): Promise<void> {
  await nextTick()
  await nextTick()
}

function forceMode(m: 'mobile' | 'desktop'): void {
  useViewMode().setMode(m)
}

describe('useManageDialog + ManageDialogHost', () => {
  let warnSpy: ReturnType<typeof vi.spyOn> | null = null

  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = '' // 清 Teleport 上轮残留
    forceMode('desktop')
    // 模块单例复位(测试间隔离)
    manageDialogHostMounted.value = false
    if (manageDialogState.resolve) {
      answerManageDialog({ type: 'close' })
    }
    manageDialogState.visible = false
    manageDialogState.title = ''
    manageDialogState.component = null
    manageDialogState.props = {}
    manageDialogState.mobileMode = 'sheet'
    manageDialogState.width = undefined
  })

  afterEach(() => {
    warnSpy?.mockRestore()
    warnSpy = null
  })

  async function mountHost(): Promise<VueWrapper> {
    const w = mount(ManageDialogHost, { attachTo: document.body })
    await flush()
    return w
  }

  it('openManageDialog → visible=true 且 Host 内渲染内容组件', async () => {
    const w = await mountHost()
    const p = openManageDialog({ title: '新增用户', component: DialogStub })
    await flush()
    expect(manageDialogState.visible).toBe(true)
    expect(manageDialogState.title).toBe('新增用户')
    expect(w.findComponent(DialogStub).exists()).toBe(true)
    answerManageDialog({ type: 'confirm', payload: null })
    await expect(p).resolves.toBeNull()
    w.unmount()
  })

  it('内容组件 emit confirm(payload) → promise resolve(payload) 且 visible=false', async () => {
    const w = await mountHost()
    const p = openManageDialog({ title: 'T', component: DialogStub })
    await flush()
    w.findComponent(DialogStub).vm.$emit('confirm', { a: 1 })
    await expect(p).resolves.toEqual({ a: 1 })
    expect(manageDialogState.visible).toBe(false)
    expect(manageDialogState.resolve).toBeNull()
    w.unmount()
  })

  it('内容组件 emit cancel → promise resolve(null)', async () => {
    const w = await mountHost()
    const p = openManageDialog({ title: 'T', component: DialogStub })
    await flush()
    w.findComponent(DialogStub).vm.$emit('cancel')
    await expect(p).resolves.toBeNull()
    w.unmount()
  })

  it('Host 上触发 Escape(ManageSheet document keydown 路径) → resolve(null)', async () => {
    const w = await mountHost()
    const p = openManageDialog({ title: 'T', component: DialogStub })
    await flush()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await expect(p).resolves.toBeNull()
    expect(manageDialogState.visible).toBe(false)
    w.unmount()
  })

  it('pending 未决时第二次 open → 旧 promise resolve(null), state 为新弹窗', async () => {
    const w = await mountHost()
    const p1 = openManageDialog({ title: '第一个', component: DialogStub })
    await flush()
    const p2 = openManageDialog({ title: '第二个', component: DialogStub })
    await expect(p1).resolves.toBeNull()
    expect(manageDialogState.title).toBe('第二个')
    answerManageDialog({ type: 'confirm', payload: 'ok' })
    await expect(p2).resolves.toBe('ok')
    w.unmount()
  })

  it('props: { initial } 透传给内容组件', async () => {
    const w = await mountHost()
    const row = { name: '张三' }
    const p = openManageDialog({ title: '编辑', component: DialogStub, props: { initial: row } })
    await flush()
    // 内容经 Teleport 渲染在 body 上, 用 document 查询而非 wrapper.find
    expect(document.querySelector('.dialog-stub')?.textContent).toContain('张三')
    answerManageDialog({ type: 'close' })
    await expect(p).resolves.toBeNull()
    w.unmount()
  })

  it('未挂载 Host 直接 open → resolve(null) + console.warn, 不置 visible', async () => {
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const p = openManageDialog({ title: 'T', component: DialogStub })
    await expect(p).resolves.toBeNull()
    expect(manageDialogState.visible).toBe(false)
    expect(warnSpy).toHaveBeenCalledWith('[useManageDialog] Host 未挂载')
  })

  it('Host 卸载时仍有待决 → resolve(null) 防悬挂', async () => {
    const w = await mountHost()
    const p = openManageDialog({ title: 'T', component: DialogStub })
    await flush()
    w.unmount()
    await expect(p).resolves.toBeNull()
    expect(manageDialogState.visible).toBe(false)
  })

  it('width="640px" → modal 容器 maxWidth=640px; mobileMode=fullsheet → 容器类 jc-sheet--fullsheet', async () => {
    const w = await mountHost()
    const p1 = openManageDialog({ title: 'T', component: DialogStub, width: '640px' })
    await flush()
    const modal = document.querySelector('.jc-sheet--modal') as HTMLElement
    expect(modal.style.maxWidth).toBe('640px')
    answerManageDialog({ type: 'close' })
    await expect(p1).resolves.toBeNull()
    w.unmount()

    // mobile + fullsheet
    forceMode('mobile')
    const w2 = await mountHost()
    const p2 = openManageDialog({
      title: 'T',
      component: DialogStub,
      mobileMode: 'fullsheet'
    })
    await flush()
    expect(document.querySelector('.jc-sheet--fullsheet')).not.toBeNull()
    answerManageDialog({ type: 'close' })
    await expect(p2).resolves.toBeNull()
    w2.unmount()
  })
})
