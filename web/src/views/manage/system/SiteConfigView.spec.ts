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
      spiderMaxGoroutine: 16,
      spiderMasterMaxWorkers: 9,
      spiderSlaveMaxWorkers: 6
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

  /** 卡片 = 顶部 5 个 section; 按标题定位, 返回该卡内的保存按钮 */
  function cardOf(w: Awaited<ReturnType<typeof mountView>>, title: string) {
    const card = w.findAll('section').find((s) => s.text().includes(title) && s.classes().includes('bg-surface'))
    if (!card) throw new Error(`未找到卡片: ${title}`)
    return card
  }

  it('每张卡恒占一列: 网格固定 1/2 列, 且没有跨列卡片', async () => {
    const w = await mountView()
    const grid = w.find('.grid')
    expect(grid.classes()).toContain('grid-cols-1')
    expect(grid.classes()).toContain('xl:grid-cols-2')
    expect(grid.classes()).not.toContain('2xl:grid-cols-3')
    for (const cls of w.findAll('section').flatMap((s) => s.classes())) {
      expect(cls).not.toContain('col-span')
    }
  })

  it('账号与安全、采集性能各自独立保存: 只提交本卡字段, 不带另一张卡的未保存草稿', async () => {
    h.updateRuntimeSettings.mockImplementation(async (d) => d)
    const w = await mountView()
    // 在「账号与安全」里改 TTL 但不保存（注意: 全页第一个 number 输入是邮件卡的端口, 不能按序号取）
    const ttl = cardOf(w, '账号与安全').find('input[type="number"]')
    await ttl.setValue('900')
    // 保存「采集性能」: 提交的 TTL 必须是上次落库的 720
    await cardOf(w, '采集性能').findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).toHaveBeenCalledWith({
      authTokenTtlHours: 720,
      maxDeviceLogin: 10,
      spiderMaxGoroutine: 16,
      spiderMasterMaxWorkers: 9,
      spiderSlaveMaxWorkers: 6
    })
  })

  it('反过来亦然: 采集并发草稿不影响「账号与安全」的保存', async () => {
    h.updateRuntimeSettings.mockImplementation(async (d) => d)
    const w = await mountView()
    const nums = w.findAll('input[type="number"]')
    await nums[nums.length - 1].setValue('99') // 采集并发草稿
    await cardOf(w, '账号与安全').findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).toHaveBeenCalledWith({
      authTokenTtlHours: 720,
      maxDeviceLogin: 10,
      spiderMaxGoroutine: 16,
      spiderMasterMaxWorkers: 9,
      spiderSlaveMaxWorkers: 6
    })
  })

  it('TMDB 未配置时提示"后台是唯一权威, env 配过的需重存一次"', async () => {
    const w = await mountView()
    const text = cardOf(w, 'TMDB 横图').text()
    expect(text).toContain('未配置')
    expect(text).toContain('唯一权威')
  })
})

describe('SiteConfigView 采集性能卡（三个上限可配）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.getBasic.mockResolvedValue({
      siteName: 'Jerocine', logo: '', keyword: '', describe: '',
      domain: 'https://jerocine.art', state: true, hint: ''
    })
    h.getTMDBKey.mockResolvedValue({ masked: '', set: false })
    h.getMailConfig.mockResolvedValue({
      registerMode: 'off', smtpEnabled: 0, smtpHost: '', smtpPort: 465, smtpSecure: 'ssl',
      smtpUser: '', smtpFromName: '', mailDailyQuota: 50,
      smtpPassMasked: '', smtpPassSet: false, presets: []
    })
    h.getRuntimeSettings.mockResolvedValue({
      authTokenTtlHours: 168,
      maxDeviceLogin: 5,
      spiderMaxGoroutine: 32,
      spiderMasterMaxWorkers: 12,
      spiderSlaveMaxWorkers: 8
    })
  })

  it('三个上限都回填, 保存时一并提交', async () => {
    h.updateRuntimeSettings.mockImplementation(async (d) => d)
    const w = await mountView()
    const card = w.findAll('section').find((s) => s.text().includes('采集性能') && s.classes().includes('bg-surface'))!
    const nums = card.findAll('input[type="number"]')
    expect(nums.map((i) => (i.element as HTMLInputElement).value)).toEqual(['32', '12', '8'])
    await nums[1].setValue('10')
    await card.findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).toHaveBeenCalledWith({
      authTokenTtlHours: 168,
      maxDeviceLogin: 5,
      spiderMaxGoroutine: 32,
      spiderMasterMaxWorkers: 10,
      spiderSlaveMaxWorkers: 8
    })
  })

  it('任一项越界都在前端拦下', async () => {
    const w = await mountView()
    const card = w.findAll('section').find((s) => s.text().includes('采集性能') && s.classes().includes('bg-surface'))!
    const nums = card.findAll('input[type="number"]')
    await nums[2].setValue('300')
    await card.findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('从站并发'))
  })
})
