import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SiteConfigView from './SiteConfigView.vue'

const h = vi.hoisted(() => ({
  getBasic: vi.fn(),
  updateBasic: vi.fn(),
  getTMDBKey: vi.fn(),
  setTMDBKey: vi.fn(),
  clearTMDBKey: vi.fn(),
  getRuntimeSettings: vi.fn(),
  updateRuntimeSettings: vi.fn(),
  getMailConfig: vi.fn(),
  saveMailConfig: vi.fn(),
  testMail: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    system: {
      getBasic: h.getBasic,
      updateBasic: h.updateBasic,
      getTMDBKey: h.getTMDBKey,
      setTMDBKey: h.setTMDBKey,
      clearTMDBKey: h.clearTMDBKey,
      getRuntimeSettings: h.getRuntimeSettings,
      updateRuntimeSettings: h.updateRuntimeSettings
    },
    mail: {
      getMailConfig: h.getMailConfig,
      saveMailConfig: h.saveMailConfig,
      testMail: h.testMail
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))

async function mountView() {
  const w = mount(SiteConfigView)
  await new Promise((r) => setTimeout(r, 20))
  return w
}

describe('SiteConfigView 系统设置（卡片式）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.getBasic.mockResolvedValue({
      siteName: 'Jerocine',
      logo: '',
      keyword: '',
      describe: '',
      domain: 'https://jerocine.art',
      state: true,
      hint: ''
    })
    h.getTMDBKey.mockResolvedValue({ masked: '', set: false })
    h.getRuntimeSettings.mockResolvedValue({
      authTokenTtlHours: 720,
      maxDeviceLogin: 10,
      spiderMaxGoroutine: 16
    })
    h.getMailConfig.mockResolvedValue({
      registerMode: 'apply',
      smtpEnabled: 1,
      smtpHost: 'smtp.qq.com',
      smtpPort: 465,
      smtpSecure: 'ssl',
      smtpUser: 'bot@qq.com',
      smtpFromName: 'Jerocine',
      mailDailyQuota: 50,
      smtpPassMasked: '••••',
      smtpPassSet: true,
      presets: []
    })
  })

  it('一个页面内展示全部卡片（站点 / 注册与邮件 / TMDB / 账号与安全 / 采集性能）', async () => {
    const w = await mountView()
    const text = w.text()
    for (const title of ['站点信息', '注册与邮件通道', 'TMDB 横图', '账号与安全', '采集性能']) {
      expect(text, title).toContain(title)
    }
  })

  it('运行参数从 /manage/settings 读入并回填', async () => {
    const w = await mountView()
    expect(h.getRuntimeSettings).toHaveBeenCalled()
    const inputs = w.findAll('input[type="number"]')
    const values = inputs.map((i) => (i.element as HTMLInputElement).value)
    expect(values).toContain('720')
    expect(values).toContain('10')
    expect(values).toContain('16')
  })

  it('邮件卡: 原独立页的注册模式与 SMTP 字段都在设置页内', async () => {
    const w = await mountView()
    expect(h.getMailConfig).toHaveBeenCalled()
    expect(w.text()).toContain('申请审核')
    const values = w.findAll('input').map((i) => (i.element as HTMLInputElement).value)
    expect(values).toContain('smtp.qq.com')
    expect(values).toContain('bot@qq.com')
  })
})
