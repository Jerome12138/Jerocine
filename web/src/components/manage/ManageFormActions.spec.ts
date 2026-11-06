import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageFormActions from './ManageFormActions.vue'

/**
 * ManageFormActions(§C8#2) — 弹窗/卡片表单 footer 原子(ghost 左钮 + 渐变提交钮)。
 * 用例按 11 处原款视觉锁外观:
 *   - 默认 = 弹窗表单四款原款(justify-end gap-3 + ghost 取消 + gradient submit 保存)
 *   - 短弹窗三款(UserResetPassword/UserForm/RegisterReject) = gap-2 无 pt
 *   - TelemetryResolve = variant primary(非 gradient, 原款差异保留)
 *   - LiveSource/Danmu/TMDB 保存 Key = confirmType button(点击驱动, 非 form @submit)
 *   - DanmuConfig/SiteConfig 卡三处 = sticky(mt-auto 吸底) + confirmDisabled/cancelDisabled
 *   - SiteConfig TMDB 无 Key = cancelText='' 不渲染左钮
 */
describe('ManageFormActions', () => {
  it('默认: justify-end gap-3 + ghost 取消(emit cancel) + gradient submit 提交(emit confirm)', async () => {
    const w = mount(ManageFormActions, { props: { confirmText: '保存' } })
    expect(w.attributes('class')).toContain('justify-end')
    expect(w.attributes('class')).toContain('gap-[var(--jc-space-3)]')
    expect(w.attributes('class')).not.toContain('mt-auto')
    expect(w.attributes('class')).not.toContain('pt-[var(--jc-space-2)]')

    const [cancel, confirm] = w.findAll('button')
    expect(cancel.text()).toBe('取消')
    expect(cancel.classes()).not.toContain('jc-btn-gradient')
    expect(cancel.attributes('type')).toBe('button')
    await cancel.trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)

    expect(confirm.text()).toContain('保存')
    expect(confirm.classes()).toContain('jc-btn-gradient')
    expect(confirm.attributes('type')).toBe('submit')
    expect(confirm.attributes('disabled')).toBeUndefined()
    await confirm.trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
  })

  it('cancelText 定制(重置/清除同走 cancel 事件); 空串=不渲染左钮', async () => {
    const w = mount(ManageFormActions, {
      props: { confirmText: '保存', cancelText: '重置' }
    })
    const [cancel] = w.findAll('button')
    expect(cancel.text()).toBe('重置')
    await cancel.trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)

    const noCancel = mount(ManageFormActions, {
      props: { confirmText: '保存 Key', cancelText: '' }
    })
    expect(noCancel.findAll('button')).toHaveLength(1)
    expect(noCancel.text()).toContain('保存 Key')
  })

  it('gap=2 短弹窗原款: gap-[var(--jc-space-2)]', () => {
    const w = mount(ManageFormActions, { props: { confirmText: '保存', gap: 2 } })
    expect(w.attributes('class')).toContain('gap-[var(--jc-space-2)]')
    expect(w.attributes('class')).not.toContain('gap-[var(--jc-space-3)]')
  })

  it('pt 上内距原款(弹窗表单四款): pt-[var(--jc-space-2)]', () => {
    const w = mount(ManageFormActions, { props: { confirmText: '保存', pt: true } })
    expect(w.attributes('class')).toContain('pt-[var(--jc-space-2)]')
  })

  it('sticky 吸底原款(卡片长表单三处): mt-auto', () => {
    const w = mount(ManageFormActions, { props: { confirmText: '保存', sticky: true } })
    expect(w.attributes('class')).toContain('mt-auto')
    expect(w.attributes('class')).toContain('justify-end')
  })

  it('busy: 提交钮进 loading(disabled) 且点击不 emit confirm', async () => {
    const w = mount(ManageFormActions, { props: { confirmText: '保存', busy: true } })
    const confirm = w.findAll('button')[1]
    expect(confirm.attributes('disabled')).toBeDefined()
    expect(confirm.find('.jc-btn-spinner').exists()).toBe(true)
    await confirm.trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
  })

  it('confirmVariant=primary(TelemetryResolve 原款): 非渐变类, primary 实色类', () => {
    const w = mount(ManageFormActions, {
      props: { confirmText: '保存', confirmVariant: 'primary' }
    })
    const confirm = w.findAll('button')[1]
    expect(confirm.classes()).not.toContain('jc-btn-gradient')
    expect(confirm.attributes('data-type')).toBe('primary')
  })

  it('confirmType=button(LiveSource/Danmu/TMDB 三款): 原生 type=button, 点击 emit confirm', async () => {
    const w = mount(ManageFormActions, {
      props: { confirmText: '保存', confirmType: 'button' }
    })
    const confirm = w.findAll('button')[1]
    expect(confirm.attributes('type')).toBe('button')
    await confirm.trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
  })

  it('confirmDisabled(SiteConfig loadError 原款)与 cancelDisabled(TMDB 清除原款)', async () => {
    const w = mount(ManageFormActions, {
      props: { confirmText: '保存 Key', cancelText: '清除', confirmDisabled: true, cancelDisabled: true }
    })
    const [cancel, confirm] = w.findAll('button')
    expect(cancel.attributes('disabled')).toBeDefined()
    expect(confirm.attributes('disabled')).toBeDefined()
    await cancel.trigger('click')
    await confirm.trigger('click')
    expect(w.emitted('cancel')).toBeUndefined()
    expect(w.emitted('confirm')).toBeUndefined()
  })
})
