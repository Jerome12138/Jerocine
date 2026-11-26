import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SchedulePicker from './SchedulePicker.vue'

/** 打开某个下拉并按文案选项（ManageSelect 自绘浮层） */
async function pick(w: ReturnType<typeof mount>, ariaLabel: string, label: string): Promise<void> {
  const trig = w.findAll('button[aria-haspopup="listbox"]').find((b) => b.attributes('aria-label') === ariaLabel)
  if (!trig) throw new Error(`未找到下拉: ${ariaLabel}`)
  await trig.trigger('click')
  const opt = w.findAll('[role="option"]').find((o) => o.text().includes(label))
  if (!opt) throw new Error(`未找到选项: ${label}（${ariaLabel}）`)
  await opt.trigger('click')
}

function lastEmitted(w: ReturnType<typeof mount>): string {
  const ev = w.emitted('update:modelValue')
  return ev && ev.length ? String(ev[ev.length - 1][0]) : ''
}

describe('SchedulePicker 执行周期选择', () => {
  it('默认（空值、不允许空）落成每天 04:00，不产生非法值', () => {
    const w = mount(SchedulePicker, { props: { modelValue: '' } })
    expect(lastEmitted(w)).toBe('daily:04:00')
  })

  it('每天：切小时/分钟即 emit（分钟补零）', async () => {
    const w = mount(SchedulePicker, { props: { modelValue: 'daily:04:00' } })
    await pick(w, '小时', '03')
    expect(lastEmitted(w)).toBe('daily:03:00')
    await pick(w, '分钟', '07')
    expect(lastEmitted(w)).toBe('daily:03:07')
  })

  it('每隔：数值 + 单位；分钟超 59 自动收敛', async () => {
    const w = mount(SchedulePicker, { props: { modelValue: 'daily:04:00' } })
    await pick(w, '周期类型', '每隔一段时间')
    expect(lastEmitted(w)).toBe('every:30m') // 默认 30 分钟
    await pick(w, '间隔单位', '小时')
    expect(lastEmitted(w)).toBe('every:30h')
    await pick(w, '间隔数值', '6')
    expect(lastEmitted(w)).toBe('every:6h')
    // 回到分钟：6 分钟合法；把数值调到 59 后再切单位不应越界
    await pick(w, '间隔单位', '分钟')
    expect(lastEmitted(w)).toBe('every:6m')
  })

  it('每周 / 每月：星期与日号 + 时分', async () => {
    const w = mount(SchedulePicker, { props: { modelValue: 'weekly:1 03:00' } })
    expect(w.text()).toContain('每周一 03:00')
    await pick(w, '星期', '周日')
    expect(lastEmitted(w)).toBe('weekly:7 03:00')

    await pick(w, '周期类型', '每月')
    expect(lastEmitted(w)).toBe('monthly:1 03:00')
    await pick(w, '每月几号', '28 日')
    expect(lastEmitted(w)).toBe('monthly:28 03:00')
  })

  it('允许空时可选"不定时（仅手动）"，emit 空串', async () => {
    const w = mount(SchedulePicker, { props: { modelValue: 'daily:04:00', allowEmpty: true } })
    await pick(w, '周期类型', '不定时')
    expect(lastEmitted(w)).toBe('')
    expect(w.text()).toContain('只在手动触发时执行')
  })

  it('解析不了的历史值：进"自定义"保留原文，不静默改写', () => {
    const w = mount(SchedulePicker, { props: { modelValue: '0 0 3 * *' } })
    const input = w.findAll('input').find((i) => (i.attributes('value') || '') === '0 0 3 * *')
      ?? w.findAll('input')[0]
    expect((input.element as HTMLInputElement).value).toBe('0 0 3 * *')
    expect(w.emitted('update:modelValue')).toBeFalsy()
  })

  it('外部改值会同步进控件（编辑回填）', async () => {
    const w = mount(SchedulePicker, { props: { modelValue: 'daily:04:00' } })
    await w.setProps({ modelValue: 'weekly:5 21:30' })
    expect(w.text()).toContain('每周五 21:30')
  })
})
