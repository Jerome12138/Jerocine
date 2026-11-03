import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SiteConfigView from './SiteConfigView.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'

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
  getDanmuConfig: vi.fn(),
  saveDanmuConfig: vi.fn(),
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
    },
    danmu: {
      getDanmuConfig: h.getDanmuConfig,
      saveDanmuConfig: h.saveDanmuConfig
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))

vi.mock('@/composables/useConfirm', () => ({ confirm: vi.fn().mockResolvedValue(true) }))

import { confirm } from '@/composables/useConfirm'

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
    h.getDanmuConfig.mockResolvedValue({
      enabled: 0,
      providers: [],
      all: ['bilibili', 'tencent', 'iqiyi', 'youku']
    })
  })

  it('一个页面内展示全部卡片（站点 / 注册与邮件 / TMDB / 账号与安全 / 采集性能 / 弹幕服务）', async () => {
    const w = await mountView()
    const text = w.text()
    for (const title of ['站点信息', '注册与邮件通道', 'TMDB 横图', '账号与安全', '采集性能', '弹幕服务']) {
      expect(text, title).toContain(title)
    }
  })

  it('弹幕卡: 默认不启用, 开启且勾选平台后保存', async () => {
    h.saveDanmuConfig.mockImplementation(async (d) => ({ enabled: d.enabled, providers: d.providers, all: ['bilibili', 'tencent', 'iqiyi', 'youku'] }))
    const w = await mountView()
    expect(h.getDanmuConfig).toHaveBeenCalled()
    const card = cardOf(w, '弹幕服务')
    expect(card.text()).toContain('未启用（默认）')
    // 开启总开关(ManageSwitch 是 button[role=switch])
    await card.find('button[role="switch"]').trigger('click')
    // 勾选两个平台(全页卡片里 4 个 checkbox 都在弹幕卡; 但保险起见按卡片内查)
    const boxes = card.findAll('input[type="checkbox"]')
    await boxes[0].setValue(true) // bilibili
    await boxes[2].setValue(true) // iqiyi
    await card.findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveDanmuConfig).toHaveBeenCalledWith({ enabled: 1, providers: ['bilibili', 'iqiyi'] }, { silent: true })
  })

  it('弹幕卡: 开启但未勾选任何平台时前端拦下', async () => {
    const w = await mountView()
    const card = cardOf(w, '弹幕服务')
    await card.find('button[role="switch"]').trigger('click')
    await card.findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveDanmuConfig).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('至少勾选'))
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

  it('邮件卡: 原独立页的注册模式与 SMTP 字段都在设置页内, 三个手写 select 换 ManageSelect', async () => {
    const w = await mountView()
    expect(h.getMailConfig).toHaveBeenCalled()
    expect(w.text()).toContain('申请审核')
    const values = w.findAll('input').map((i) => (i.element as HTMLInputElement).value)
    expect(values).toContain('smtp.qq.com')
    expect(values).toContain('bot@qq.com')
    // 注册模式 / 服务商预设 / 加密方式 → ManageSelect(复核#3: 8 页 3 种手写写法统一)
    const card = cardOf(w, '注册与邮件通道')
    expect(card.findAllComponents(ManageSelect).length).toBeGreaterThanOrEqual(3)
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
    expect(h.updateRuntimeSettings).toHaveBeenCalledWith(
      {
        authTokenTtlHours: 720,
        maxDeviceLogin: 10,
        spiderMaxGoroutine: 16,
        spiderMasterMaxWorkers: 9,
        spiderSlaveMaxWorkers: 6
      },
      { silent: true }
    )
  })

  it('反过来亦然: 采集并发草稿不影响「账号与安全」的保存', async () => {
    h.updateRuntimeSettings.mockImplementation(async (d) => d)
    const w = await mountView()
    const nums = w.findAll('input[type="number"]')
    await nums[nums.length - 1].setValue('99') // 采集并发草稿
    await cardOf(w, '账号与安全').findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).toHaveBeenCalledWith(
      {
        authTokenTtlHours: 720,
        maxDeviceLogin: 10,
        spiderMaxGoroutine: 16,
        spiderMasterMaxWorkers: 9,
        spiderSlaveMaxWorkers: 6
      },
      { silent: true }
    )
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
    h.getDanmuConfig.mockResolvedValue({
      enabled: 0,
      providers: [],
      all: ['bilibili', 'tencent', 'iqiyi', 'youku']
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
    expect(h.updateRuntimeSettings).toHaveBeenCalledWith(
      {
        authTokenTtlHours: 168,
        maxDeviceLogin: 5,
        spiderMaxGoroutine: 32,
        spiderMasterMaxWorkers: 10,
        spiderSlaveMaxWorkers: 8
      },
      { silent: true }
    )
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

// 保存一张卡不得冲掉另一张卡的未保存草稿(响应整体 assign 回草稿的回归)
it('保存「采集性能」后, 「账号与安全」里未保存的草稿原样保留', async () => {
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
  h.getDanmuConfig.mockResolvedValue({
    enabled: 0,
    providers: [],
    all: ['bilibili', 'tencent', 'iqiyi', 'youku']
  })
  h.getRuntimeSettings.mockResolvedValue({
    authTokenTtlHours: 168,
    maxDeviceLogin: 5,
    spiderMaxGoroutine: 32,
    spiderMasterMaxWorkers: 12,
    spiderSlaveMaxWorkers: 8
  })
  h.updateRuntimeSettings.mockImplementation(async (d) => d)
  const w = await mountView()
  const account = w
    .findAll('section')
    .find((s) => s.text().includes('账号与安全') && s.classes().includes('bg-surface'))!
  await account.find('input[type="number"]').setValue('900')
  const spider = w
    .findAll('section')
    .find((s) => s.text().includes('采集性能') && s.classes().includes('bg-surface'))!
  await spider.findAll('button').at(-1)!.trigger('click')
  await new Promise((r) => setTimeout(r, 10))
  expect((account.find('input[type="number"]').element as HTMLInputElement).value).toBe('900')
})

// ---- 站点信息保存链路 / TMDB / load 失败态 / 邮件卡不变量 ----

function cardOf(w: Awaited<ReturnType<typeof mountView>>, title: string) {
  const card = w
    .findAll('section')
    .find((s) => s.text().includes(title) && s.classes().includes('bg-surface'))
  if (!card) throw new Error(`未找到卡片: ${title}`)
  return card
}

function baseBasic(over: Record<string, unknown> = {}) {
  return {
    siteName: 'Jerocine', logo: '', keyword: '', describe: '',
    domain: 'https://jerocine.art', state: true, hint: '',
    masked: '', set: false, // getBasic 同响应附带 TMDB 掩码状态(修复后不再二次 GET)
    ...over
  }
}

function baseMail(over: Record<string, unknown> = {}) {
  return {
    registerMode: 'apply', smtpEnabled: 1, smtpHost: 'smtp.qq.com', smtpPort: 465,
    smtpSecure: 'ssl', smtpUser: 'bot@qq.com', smtpFromName: 'Jerocine',
    mailDailyQuota: 50, smtpPassMasked: '••••', smtpPassSet: true, presets: [],
    ...over
  }
}

describe('SiteConfigView 站点信息 / TMDB / load 失败态', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    vi.mocked(confirm).mockResolvedValue(true)
    h.getBasic.mockResolvedValue(baseBasic())
    h.getTMDBKey.mockResolvedValue({ masked: '0c06…676b', set: true })
    h.getRuntimeSettings.mockResolvedValue({
      authTokenTtlHours: 168, maxDeviceLogin: 5,
      spiderMaxGoroutine: 32, spiderMasterMaxWorkers: 12, spiderSlaveMaxWorkers: 8
    })
    h.getMailConfig.mockResolvedValue(baseMail())
    h.getDanmuConfig.mockResolvedValue({
      enabled: 0, providers: [], all: ['bilibili', 'tencent', 'iqiyi', 'youku']
    })
  })

  it('站点信息保存: 提交全字段 payload + 更新共享 store + success toast', async () => {
    h.updateBasic.mockResolvedValue(undefined)
    const w = await mountView()
    const site = cardOf(w, '站点信息')
    await site.find('input').setValue('新站名')
    await site.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateBasic).toHaveBeenCalledWith(
      expect.objectContaining({ siteName: '新站名' }),
      { silent: true }
    )
    expect(h.toast).toHaveBeenCalledWith('success', '站点信息已保存')
  })

  it('站点名称为空(或纯空格)时前端拦下, 不发请求', async () => {
    const w = await mountView()
    const site = cardOf(w, '站点信息')
    await site.find('input').setValue('   ')
    await site.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateBasic).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请填写站点名称')
  })

  it('TMDB 保存: 空 key 拦截; 有效 key 走 setTMDBKey 并清空输入', async () => {
    h.setTMDBKey.mockResolvedValue(undefined)
    const w = await mountView()
    const tmdb = cardOf(w, 'TMDB 横图')
    const keyInput = tmdb.find('input')
    // 空 key 拦截
    await tmdb.findAll('button').find((b) => b.text() === '保存 Key')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.setTMDBKey).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('请输入'))
    // 有效 key
    await keyInput.setValue('my-tmdb-key')
    await tmdb.findAll('button').find((b) => b.text() === '保存 Key')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.setTMDBKey).toHaveBeenCalledWith('my-tmdb-key', { silent: true })
    expect((keyInput.element as HTMLInputElement).value).toBe('')
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已保存'))
  })

  it('TMDB 清除(破坏性): confirm 取消不动手; 确认后调 clearTMDBKey', async () => {
    h.getBasic.mockResolvedValue(baseBasic({ masked: '0c06…676b', set: true }))
    h.clearTMDBKey.mockResolvedValue(undefined)
    const w = await mountView()
    const tmdb = cardOf(w, 'TMDB 横图')
    expect(tmdb.text()).toContain('已配置')
    const clear = tmdb.findAll('button').find((b) => b.text() === '清除')!
    // 取消: 不调清除
    vi.mocked(confirm).mockResolvedValueOnce(false)
    await clear.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.clearTMDBKey).not.toHaveBeenCalled()
    // 确认: 清除 + 重拉掩码 + success toast
    vi.mocked(confirm).mockResolvedValueOnce(true)
    await clear.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ danger: true }))
    expect(h.clearTMDBKey).toHaveBeenCalledWith({ silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已清除'))
  })

  it('账号与安全范围校验: TTL 1~8760、设备数 1~50 越界拦截', async () => {
    const w = await mountView()
    const card = cardOf(w, '账号与安全')
    const nums = card.findAll('input[type="number"]')
    await nums[0].setValue('9000')
    await card.findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('1~8760'))

    await nums[0].setValue('720')
    await nums[1].setValue('51')
    await card.findAll('button').at(-1)!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateRuntimeSettings).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('1~50'))
  })

  it('load 失败: 错误横幅 + 各卡保存禁用; 重试成功后恢复', async () => {
    h.getBasic.mockRejectedValueOnce(new Error('boom'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败')
    // 站点表单渲染了空字段, 但保存禁用
    const siteSave = cardOf(w, '站点信息').findAll('button').find((b) => b.text() === '保存')!
    expect(siteSave.attributes('disabled')).toBeDefined()
    const accountSave = cardOf(w, '账号与安全').findAll('button').find((b) => b.text() === '保存')!
    expect(accountSave.attributes('disabled')).toBeDefined()
    // 保存被拦: 点了也不发请求
    await siteSave.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.updateBasic).not.toHaveBeenCalled()
    // 重试恢复
    await w.findAll('button').find((b) => b.text() === '重试')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).not.toContain('加载失败')
    expect(
      cardOf(w, '账号与安全').findAll('button').find((b) => b.text() === '保存')!
        .attributes('disabled')
    ).toBeUndefined()
  })

  it('站点卡「重置」只重取基础信息, 不重拉运行参数/TMDB(保住别卡草稿)', async () => {
    const w = await mountView()
    expect(h.getBasic).toHaveBeenCalledTimes(1)
    const site = cardOf(w, '站点信息')
    await site.find('input').setValue('未保存草稿')
    await site.findAll('button').find((b) => b.text() === '重置')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.getBasic).toHaveBeenCalledTimes(2)
    // 运行参数没有重新拉取; 草稿被库内值覆盖回 'Jerocine'
    expect(h.getRuntimeSettings).toHaveBeenCalledTimes(1)
    expect((site.find('input').element as HTMLInputElement).value).toBe('Jerocine')
  })
})

describe('SiteConfigView 邮件卡（注册与邮件通道不变量）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.getBasic.mockResolvedValue(baseBasic())
    h.getTMDBKey.mockResolvedValue({ masked: '', set: false })
    h.getRuntimeSettings.mockResolvedValue({
      authTokenTtlHours: 168, maxDeviceLogin: 5,
      spiderMaxGoroutine: 32, spiderMasterMaxWorkers: 12, spiderSlaveMaxWorkers: 8
    })
    h.getMailConfig.mockResolvedValue(baseMail())
    h.getDanmuConfig.mockResolvedValue({
      enabled: 0, providers: [], all: ['bilibili', 'tencent', 'iqiyi', 'youku']
    })
  })

  /** 邮件卡内输入顺序: host / 端口(number) / 账号 / 授权码(password) / 显示名 / 配额(number) / 测试邮箱 */
  function mailCard(w: Awaited<ReturnType<typeof mountView>>) {
    return cardOf(w, '注册与邮件通道')
  }

  it('保存成功: 留空授权码 payload 不带 smtpPass; 填了才带上', async () => {
    h.saveMailConfig.mockResolvedValue({ smtpPassSet: true, smtpPassMasked: '••••' })
    const w = await mountView()
    const card = mailCard(w)
    await card.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveMailConfig).toHaveBeenCalledTimes(1)
    const p1 = h.saveMailConfig.mock.calls[0][0]
    expect(p1).not.toHaveProperty('smtpPass')

    await card.find('input[type="password"]').setValue('new-secret')
    await card.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    const p2 = h.saveMailConfig.mock.calls[1][0]
    expect(p2).toHaveProperty('smtpPass', 'new-secret')
  })

  it('清空日配额按 0=不修改放行(payload mailDailyQuota: 0)', async () => {
    h.saveMailConfig.mockResolvedValue({ smtpPassSet: true, smtpPassMasked: '••••' })
    const w = await mountView()
    const card = mailCard(w)
    const quota = card.findAll('input[type="number"]')[1]
    await quota.setValue('')
    await card.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveMailConfig).toHaveBeenCalledWith(
      expect.objectContaining({ mailDailyQuota: 0 }),
      { silent: true }
    )
    expect(h.toast).not.toHaveBeenCalledWith('error', expect.stringContaining('1~1000'))
  })

  it('清空端口回落默认 465; 越界端口中文拦下', async () => {
    h.saveMailConfig.mockResolvedValue({ smtpPassSet: true, smtpPassMasked: '••••' })
    const w = await mountView()
    const card = mailCard(w)
    const port = card.findAll('input[type="number"]')[0]
    await port.setValue('')
    await card.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveMailConfig).toHaveBeenCalledWith(expect.objectContaining({ smtpPort: 465 }), { silent: true })

    await port.setValue('70000')
    await card.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('1~65535'))
    expect(h.saveMailConfig).toHaveBeenCalledTimes(1)
  })

  it('启用 SMTP 但配置不完整: 前端中文拦下, 不发 PUT', async () => {
    h.getMailConfig.mockResolvedValue(
      baseMail({ registerMode: 'off', smtpHost: '', smtpUser: '', smtpPassSet: false })
    )
    const w = await mountView()
    const card = mailCard(w)
    expect(card.text()).toContain('SMTP 配置不完整')
    await card.find('form').trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.saveMailConfig).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('启用 SMTP'))
  })

  it('注册模式 open/apply 而邮件通道不可用: 展示联动警告', async () => {
    h.getMailConfig.mockResolvedValue(baseMail({ registerMode: 'open', smtpEnabled: 0 }))
    const w = await mountView()
    expect(mailCard(w).text()).toContain('依赖邮件通道')
  })

  it('测试邮件: testMail 收到表单值 + 收件人; 非法邮箱格式中文拦下', async () => {
    h.testMail.mockResolvedValue({})
    const w = await mountView()
    const card = mailCard(w)
    const testTo = card.findAll('input').at(-1)!
    // 非法格式
    await testTo.setValue('not-an-email')
    await card.findAll('button').find((b) => b.text() === '发送测试邮件')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.testMail).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '测试收件邮箱格式不正确')
    // 合法格式
    await testTo.setValue('admin@example.com')
    await card.findAll('button').find((b) => b.text() === '发送测试邮件')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.testMail).toHaveBeenCalledWith(
      expect.objectContaining({ to: 'admin@example.com', smtpHost: 'smtp.qq.com', smtpPort: 465 }),
      { silent: true }
    )
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已发送'))
  })

  it('GET 失败: 错误态占位(无表单可编辑), 重试后恢复', async () => {
    h.getMailConfig.mockRejectedValueOnce(new Error('boom'))
    const w = await mountView()
    const card = mailCard(w)
    expect(card.text()).toContain('读取失败')
    expect(card.find('form').exists()).toBe(false)
    h.getMailConfig.mockResolvedValue(baseMail())
    await card.findAll('button').find((b) => b.text() === '重试')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(card.find('form').exists()).toBe(true)
  })
})
