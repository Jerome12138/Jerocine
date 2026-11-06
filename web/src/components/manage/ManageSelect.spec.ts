import { describe, it, expect, afterEach, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import ManageSelect from './ManageSelect.vue'
import type { SelectOption } from './ManageSelect.vue'

/**
 * ManageSelect v2(自绘浮层, 规格 §3.5/D④/D⑥/D⑧)用例:
 *   API 兼容(9 个使用页零改动): options/placeholder/disabled/ariaLabel/原类型回传;
 *   新能力: 浮层开关、useDismiss 外点/ESC、useListNavigation 键盘(↑↓ 循环/Home/End/Enter)、
 *   active/选中态、data-focusable、option/trigger 槽、size 档类、teleport 预留。
 * jsdom 无 PointerEvent 构造器, 用 MouseEvent 冒充 pointerdown(同 useDismiss.spec 口径)。
 */

const OPTIONS: SelectOption[] = [
  { label: 'JSON', value: 0 },
  { label: 'XML', value: 1 },
  { label: '混合', value: 2 }
]
const STR_OPTIONS: SelectOption[] = [
  { label: '全部', value: '' },
  { label: '失败', value: 'failed' }
]

/** 键盘导航 active 项高亮类(规格 §3.5: active 项加 bg-[rgba(255,255,255,0.06)]) */
const ACTIVE_CLS = 'bg-[rgba(255,255,255,0.06)]'

function firePointerDown(el: Element): void {
  el.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
}

function findTrigger(w: VueWrapper) {
  return w.find('button[aria-haspopup="listbox"]')
}
function findOptions(w: VueWrapper) {
  return w.findAll('[role="option"]')
}
function activeIndex(w: VueWrapper): number {
  return findOptions(w).findIndex((o) => o.classes().includes(ACTIVE_CLS))
}
async function openPanel(w: VueWrapper): Promise<void> {
  await findTrigger(w).trigger('click')
  await nextTick()
}

let wrapper: VueWrapper | null = null

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('ManageSelect v2: API 兼容(9 个使用页零改动)', () => {
  it('触发钮回显选中 label + aria-haspopup/aria-expanded/aria-label/data-focusable', () => {
    wrapper = mount(ManageSelect, {
      props: { modelValue: 1, options: OPTIONS, ariaLabel: '返回类型' }
    })
    const t = findTrigger(wrapper)
    expect(t.text()).toBe('XML')
    expect(t.attributes('aria-haspopup')).toBe('listbox')
    expect(t.attributes('aria-expanded')).toBe('false')
    expect(t.attributes('aria-label')).toBe('返回类型')
    expect(t.attributes('data-focusable')).toBe('true')
  })

  it('modelValue=undefined + placeholder → 触发钮显示占位文案', () => {
    wrapper = mount(ManageSelect, {
      props: { modelValue: undefined, options: OPTIONS, placeholder: '请选择' }
    })
    expect(findTrigger(wrapper).text()).toBe('请选择')
  })

  it('disabled → 触发钮禁用且无 data-focusable, 点击不开浮层', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, disabled: true } })
    const t = findTrigger(wrapper)
    expect((t.element as HTMLButtonElement).disabled).toBe(true)
    expect(t.attributes('data-focusable')).toBeUndefined()
    await t.trigger('click')
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('数字 value 原类型回传(选后关浮层)', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    await openPanel(wrapper)
    await findOptions(wrapper)[1]!.trigger('click')
    await nextTick()
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([1])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('字符串 value 同样按原类型回传', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: '', options: STR_OPTIONS } })
    await openPanel(wrapper)
    await findOptions(wrapper)[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual(['failed'])
  })

  it('同族外观: bg-elevated/border-ctrl/rounded/focus 态与 ManageInput 同族', () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    const cls = findTrigger(wrapper).classes()
    expect(cls).toContain('bg-elevated')
    expect(cls).toContain('border-ctrl')
    expect(cls).toContain('rounded-[var(--jc-ctrl-radius)]')
    expect(cls).toContain('focus:border-ctrl-focus')
    expect(cls).toContain('focus:shadow-focus')
  })
})

describe('ManageSelect v2: 浮层开关与 dismiss', () => {
  it('默认关闭 → 点击展开(N 个 role=option + aria-selected 选中态) → 再点收起', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 1, options: OPTIONS } })
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    await openPanel(wrapper)
    const listbox = wrapper.find('[role="listbox"]')
    expect(listbox.exists()).toBe(true)
    expect(findTrigger(wrapper).attributes('aria-expanded')).toBe('true')
    const opts = findOptions(wrapper)
    expect(opts.map((o) => o.text())).toEqual(['JSON', 'XML', '混合'])
    expect(opts[1]!.attributes('aria-selected')).toBe('true')
    expect(opts[0]!.attributes('aria-selected')).toBe('false')
    await findTrigger(wrapper).trigger('click')
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('外点关闭: pointerdown 落在组件根外 → 浮层收起(useDismiss §3.1)', async () => {
    wrapper = mount(ManageSelect, {
      attachTo: document.body,
      props: { modelValue: 0, options: OPTIONS }
    })
    const outside = document.createElement('div')
    document.body.appendChild(outside)
    await openPanel(wrapper)
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)
    firePointerDown(outside)
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('外点不误伤: pointerdown 在触发钮自身(根内)不关闭, toggle 语义完整', async () => {
    wrapper = mount(ManageSelect, {
      attachTo: document.body,
      props: { modelValue: 0, options: OPTIONS }
    })
    await openPanel(wrapper)
    firePointerDown(findTrigger(wrapper).element)
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)
  })

  it('ESC 关闭并归还焦点到触发钮', async () => {
    wrapper = mount(ManageSelect, {
      attachTo: document.body,
      props: { modelValue: 0, options: OPTIONS }
    })
    await openPanel(wrapper)
    // 模拟焦点落在浮层选项上(点击选项后浮层 DOM 将被移除的场景)
    ;(findOptions(wrapper)[0]!.element as HTMLElement).focus()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(document.activeElement).toBe(findTrigger(wrapper).element)
  })

  it('弹窗内 ESC 只关浮层不关弹窗(ManageSheet/BaseDialog 组合场景, R1 评审 ESC 穿透)', async () => {
    // 模拟外层弹窗: 开窗时先注册 window bubble keydown(BaseDialog:73 / ManageSheet:75 同款时序)
    const dialogEsc = vi.fn()
    window.addEventListener('keydown', dialogEsc)
    wrapper = mount(ManageSelect, {
      attachTo: document.body,
      props: { modelValue: 0, options: OPTIONS }
    })
    await openPanel(wrapper)
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)

    // 焦点在触发钮(弹窗内焦点保持的现状)上按 ESC: useDismiss 捕获段消费并阻断传播
    findTrigger(wrapper).element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(dialogEsc).not.toHaveBeenCalled()

    window.removeEventListener('keydown', dialogEsc)
  })
})

describe('ManageSelect v2: 键盘导航(useListNavigation §3.2)', () => {
  it('open 时 active=选中项索引; ↓ 循环移动(末→首); active 项带高亮类', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 2, options: OPTIONS } })
    await openPanel(wrapper)
    expect(activeIndex(wrapper)).toBe(2)
    await findTrigger(wrapper).trigger('keydown', { key: 'ArrowDown' })
    expect(activeIndex(wrapper)).toBe(0)
    await findTrigger(wrapper).trigger('keydown', { key: 'ArrowUp' })
    expect(activeIndex(wrapper)).toBe(2)
  })

  it('Enter 确认 active 项: 原类型 emit + 收起浮层', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    await openPanel(wrapper)
    await findTrigger(wrapper).trigger('keydown', { key: 'ArrowDown' })
    await findTrigger(wrapper).trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([1])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('Home/End 跳首尾', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    await openPanel(wrapper)
    await findTrigger(wrapper).trigger('keydown', { key: 'End' })
    expect(activeIndex(wrapper)).toBe(2)
    await findTrigger(wrapper).trigger('keydown', { key: 'Home' })
    expect(activeIndex(wrapper)).toBe(0)
  })

  it('a11y: 触发钮 role=combobox 承载 aria-activedescendant(绑定 option id), 关闭态不挂', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 2, options: OPTIONS } })
    const t = findTrigger(wrapper)
    // R2 评审: aria-activedescendant 须挂组合部件角色(button 角色不承载), 补 role=combobox
    expect(t.attributes('role')).toBe('combobox')
    // 关闭态: 不挂 aria-activedescendant
    expect(t.attributes('aria-activedescendant')).toBeUndefined()
    await openPanel(wrapper)
    // 打开态初始 active=选中项(索引 2): 指向该 option 的 id
    expect(t.attributes('aria-activedescendant')).toBe(findOptions(wrapper)[2]!.attributes('id'))
    // ↓ 移动后跟随 active 项
    await findTrigger(wrapper).trigger('keydown', { key: 'ArrowDown' })
    expect(t.attributes('aria-activedescendant')).toBe(findOptions(wrapper)[0]!.attributes('id'))
  })

  it('空列表: 浮层可开但无选项, 方向键/Enter 不动作不抛错', async () => {
    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: [] } })
    await openPanel(wrapper)
    expect(findOptions(wrapper)).toHaveLength(0)
    await findTrigger(wrapper).trigger('keydown', { key: 'ArrowDown' })
    await findTrigger(wrapper).trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})

describe('ManageSelect v2: 槽位与尺寸档', () => {
  it('option 槽接管选项渲染, 作用域含 option/active/selected', async () => {
    wrapper = mount(ManageSelect, {
      props: { modelValue: 1, options: OPTIONS },
      slots: {
        option: `<template #option="{ option, active, selected }">
          <span class="opt-scope">{{ option.label }}|{{ String(active) }}|{{ String(selected) }}</span>
        </template>`
      }
    })
    await openPanel(wrapper)
    const texts = findOptions(wrapper).map((o) => o.find('.opt-scope').text())
    // 初始 active=选中项(modelValue=1 → 索引 1)
    expect(texts).toEqual(['JSON|false|false', 'XML|true|true', '混合|false|false'])
  })

  it('trigger 槽接管触发钮内容, 作用域含 selected; 默认 chevron 不再渲染', () => {
    wrapper = mount(ManageSelect, {
      props: { modelValue: 1, options: OPTIONS },
      slots: {
        trigger: `<template #trigger="{ selected }">
          <span class="trg-scope">{{ selected ? selected.label : 'none' }}</span>
        </template>`
      }
    })
    const t = findTrigger(wrapper)
    expect(t.find('.trg-scope').text()).toBe('XML')
    expect(t.find('svg').exists()).toBe(false)
  })

  it('size 档(§3.6/D⑥): sm/default/lg 分别挂 --jc-ctrl-* 三档令牌类', () => {
    const sm = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, size: 'sm' } })
    const smCls = findTrigger(sm).classes()
    expect(smCls).toContain('h-[var(--jc-ctrl-h-sm)]')
    expect(smCls).toContain('px-[var(--jc-ctrl-px-sm)]')
    expect(smCls).toContain('text-[length:var(--jc-ctrl-fs-sm)]')
    sm.unmount()

    const lg = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, size: 'lg' } })
    const lgCls = findTrigger(lg).classes()
    expect(lgCls).toContain('h-[var(--jc-ctrl-h-lg)]')
    expect(lgCls).toContain('px-[var(--jc-ctrl-px-lg)]')
    expect(lgCls).toContain('text-[length:var(--jc-ctrl-fs-lg)]')
    lg.unmount()

    wrapper = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    const cls = findTrigger(wrapper).classes()
    expect(cls).toContain('h-[var(--jc-ctrl-h)]')
    expect(cls).toContain('px-[var(--jc-ctrl-px)]')
    expect(cls).toContain('text-[length:var(--jc-ctrl-fs)]')
  })

  it('teleport(D⑧ 预留): 浮层挂 body + fixed 定位; 外点/ESC 仍可关闭, 选项点击不误关', async () => {
    wrapper = mount(ManageSelect, {
      attachTo: document.body,
      props: { modelValue: 0, options: OPTIONS, teleport: true }
    })
    await openPanel(wrapper)
    // teleport 后 wrapper 内查不到, 直接查 body
    const panel = document.body.querySelector('[role="listbox"]') as HTMLElement | null
    expect(panel, 'teleport 浮层应挂到 body').not.toBeNull()
    expect(panel!.className).toContain('fixed')
    // jsdom getBoundingClientRect 全 0 → top = rect.bottom + 4(mt-1 对齐)
    expect(panel!.style.top).toBe('4px')
    // 全屏遮罩存在(拦截触发钮再点的 toggle, 防"点外关闭→click 重开")
    expect(document.body.querySelector('[data-select-backdrop]')).not.toBeNull()

    // 外点关闭(useDismiss 锚切到浮层自身分支): pointerdown 落在锚外(body) → 收起
    firePointerDown(document.body)
    await nextTick()
    expect(document.body.querySelector('[role="listbox"]')).toBeNull()

    // 重开 → 选项点击(anchor 内)不关闭
    await openPanel(wrapper)
    const opt = document.body.querySelector('[role="option"]') as HTMLElement
    opt.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(document.body.querySelector('[role="listbox"]')).toBeNull()

    // 再重开 → ESC 关闭(useDismiss 锚在浮层自身, 照常接管)
    await openPanel(wrapper)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(document.body.querySelector('[role="listbox"]')).toBeNull()
  })
})
