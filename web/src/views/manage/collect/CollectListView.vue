<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { CollectSource } from '@/types/manage'
import type { SourceHealthRow } from '@/api/manage/collect'
import ManageTable from '@/components/manage/ManageTable.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'
import { measureLine } from '@/composables/usePlaySpeedTest'
import { toast } from '@/api/http'

const rows = ref<CollectSource[]>([])
const loading = ref(true)
const loadError = ref('') // 列表加载失败: 明确报错+重试, 不伪装成"暂无采集源"

// ---- 健康度 / 测速 (持久化, 含自动停采) ----
// 测速拆两类: 测采集(服务端打采集 API, 取中位延时与成功概率) / 测播放(浏览器端直连 CDN 计时)。
const health = reactive<Record<string, SourceHealthRow>>({})
const testingId = ref('') // 正在「测采集」的源 id
const testingPlayId = ref('') // 正在「测播放」的源 id
const testingAll = ref(false)
const testingPlayAll = ref(false)
const sortMode = ref<'default' | 'api' | 'play'>('default')

const hasAnyHealth = computed(() => Object.keys(health).length > 0)

type Badge = { variant: 'success' | 'warning' | 'danger' | 'default'; label: string; title: string }
const badges = computed<Record<string, Badge>>(() => {
  const m: Record<string, Badge> = {}
  for (const id in health) {
    const h = health[id]
    if (!h) continue
    const when = h.checkedAt ? new Date(h.checkedAt).toLocaleString() : '未检测'
    const totalTxt = h.total > 0 ? String(h.total) : (h.films > 0 ? `≈${h.films}` : '?')
    const cnt = `采${h.collected}/${totalTxt}片`
    const detail = [
      h.message,
      `已采集 ${h.collected} / 目录 ${h.total} 片`,
      `采集 ${h.latencyMs > 0 ? `${h.latencyMs}ms` : '未测'}`,
      `播放 ${h.playLatencyWeb > 0 ? `${h.playLatencyWeb}ms` : '未测'}`,
      `成功 ${h.okCount}/${h.probes}`,
      when
    ].join(' · ')
    if (h.suppressed || h.status === 'down') {
      m[id] = { variant: 'danger', label: `已停采 · ${cnt}`, title: detail }
    } else if (h.status === 'degraded') {
      m[id] = { variant: 'warning', label: `降级${h.consecutiveFails}× · ${cnt}`, title: detail }
    } else if (h.status === 'healthy') {
      m[id] = { variant: 'success', label: `${cnt}`, title: detail }
    } else {
      // 未测速也展示已采集数(collected 不依赖测速)
      m[id] = { variant: 'default', label: h.collected > 0 ? cnt : '未测', title: `已采集 ${h.collected} 片 · 尚未测速` }
    }
  }
  return m
})

// 健康度汇总
const healthSummary = computed(() => {
  let healthy = 0
  let degraded = 0
  let down = 0
  for (const id in health) {
    const h = health[id]
    if (!h) continue
    if (h.suppressed || h.status === 'down') down++
    else if (h.status === 'degraded') degraded++
    else if (h.status === 'healthy') healthy++
  }
  const untested = rows.value.length - (healthy + degraded + down)
  return { healthy, degraded, down, untested: untested > 0 ? untested : 0 }
})

// 建议主站: 健康源里目录最全的那个(资源最全)
const recommendedMasterId = computed(() => {
  let best = ''
  let bestTotal = -1
  for (const id in health) {
    const h = health[id]
    if (!h || h.status !== 'healthy') continue
    if (h.total > bestTotal) {
      bestTotal = h.total
      best = id
    }
  }
  return best
})

// 排序: 默认 / 按采集延时(服务端 API) / 按播放延时(浏览器端), 未测的排末尾
const displayRows = computed<CollectSource[]>(() => {
  if (sortMode.value === 'default') return rows.value
  const rank = (id: string): number => {
    const h = health[id]
    if (!h) return Number.MAX_SAFE_INTEGER
    if (sortMode.value === 'api') return h.latencyMs > 0 ? h.latencyMs : Number.MAX_SAFE_INTEGER
    return h.playLatencyWeb > 0 ? h.playLatencyWeb : Number.MAX_SAFE_INTEGER
  }
  return [...rows.value].sort((a, b) => rank(a.id) - rank(b.id))
})

const SORT_LABELS: Record<typeof sortMode.value, string> = {
  default: '默认排序',
  api: '按采集延时',
  play: '按播放延时'
}
function cycleSort(): void {
  sortMode.value = sortMode.value === 'default' ? 'api' : sortMode.value === 'api' ? 'play' : 'default'
}

async function loadHealth(): Promise<void> {
  try {
    const list = await manageApi.collect.health()
    // 按返回 list 整体重建: 此前只增不删, 已删源/被 GC 的健康条目会残留,
    // 汇总计数把幽灵条目算进去(untested 甚至被挤成负数)
    const next: Record<string, SourceHealthRow> = {}
    for (const r of list ?? []) next[r.id] = r
    Object.keys(health).forEach((k) => {
      if (!(k in next)) delete health[k]
    })
    Object.assign(health, next)
  } catch {
    /* 忽略, 面板降级为无数据 */
  }
}

// 「测采集」: 服务端打采集 API(3 次探测取中位), 写健康度
async function runApiTest(row: CollectSource): Promise<void> {
  testingId.value = row.id
  try {
    // silent: http 拦截器对非 silent 写操作自动弹"操作成功", 会与本页具体文案叠两条
    const res = await manageApi.collect.test(row.id, { silent: true })
    await loadHealth()
    toast('success', `「${row.name}」采集 ${res.latencyMs > 0 ? `${res.latencyMs}ms` : '失败'} · ${res.message}`)
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '测速失败')
  } finally {
    testingId.value = ''
  }
}

// 「测播放」: 浏览器端直连 CDN 计时(视频加载速度), 结果回传落库
async function runPlayTest(row: CollectSource): Promise<void> {
  testingPlayId.value = row.id
  try {
    // 后端每次实时探测取新鲜样本(存量样本可能已下线, 造成假阴性), 探测失败才回退存量
    const sample = await manageApi.collect.sampleM3u8(row.id)
    if (!sample) {
      toast('warning', `「${row.name}」拿不到样本 m3u8, 请先确认测采集通过`)
      return
    }
    const ms = await measureLine(sample)
    if (ms < 0) {
      toast('error', `「${row.name}」播放测速失败: 无可用分片`)
      return
    }
    await manageApi.collect.recordPlayLatency(row.id, ms, { silent: true })
    await loadHealth()
    toast('success', `「${row.name}」播放 ${ms}ms`)
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '播放测速失败')
  } finally {
    testingPlayId.value = ''
  }
}

// 「全部测采集」: 并发测全部源(含停用), 完成后按采集延时排序
async function runApiTestAll(): Promise<void> {
  testingAll.value = true
  try {
    const list = await manageApi.collect.testAll({ silent: true })
    await loadHealth()
    sortMode.value = 'api'
    toast('success', `采集测速完成: ${list.filter((r) => r.ok).length}/${list.length} 可用`)
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '批量测速失败')
  } finally {
    testingAll.value = false
  }
}

// 「全部测播放」: 浏览器端逐源测(限并发 3, 样本 m3u8 优先用已存, 缺失时现取)
async function runPlayTestAll(): Promise<void> {
  if (testingPlayAll.value || !rows.value.length) return
  testingPlayAll.value = true
  let okCount = 0
  const CONCURRENCY = 3
  const queue = [...rows.value]
  async function worker(): Promise<void> {
    while (queue.length) {
      const row = queue.shift()
      if (!row) return
      try {
        const sample = await manageApi.collect.sampleM3u8(row.id)
        if (!sample) continue
        const ms = await measureLine(sample)
        if (ms < 0) continue
        await manageApi.collect.recordPlayLatency(row.id, ms, { silent: true })
        okCount++
      } catch {
        /* 单源失败不影响其余 */
      }
    }
  }
  try {
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, queue.length) }, worker))
    await loadHealth()
    sortMode.value = 'play'
    toast('success', `播放测速完成: ${okCount}/${rows.value.length} 成功`)
  } finally {
    testingPlayAll.value = false
  }
}
const dialogOpen = ref(false)
const submitting = ref(false)
const editing = ref<CollectSource | null>(null)
// 表单保存失败的弹窗内联错误(与重置库存弹窗的 resetError 同款)
const formError = ref('')

// 重置库存 dialog
const resetDialogOpen = ref(false)
const resetKey = ref('')
const resetSubmitting = ref(false)
const resetError = ref('')

// 采集时长 (小时) 选择, 24h / 7d / 30d / 全量
const spiderHours = ref<number>(24)
const HOURS_OPTIONS = [
  { value: 24, label: '近 24 小时' },
  { value: 24 * 7, label: '近 7 天' },
  { value: 24 * 30, label: '近 30 天' },
  { value: -1, label: '全量' }
]

// siteUrl 在 CollectSource 上可选, 表单里恒有值 → 收窄为必有字符串(ManageInput 需要)
const form = reactive<CollectSource & { siteUrl: string }>({
  id: '',
  name: '',
  uri: '',
  siteUrl: '',
  resultModel: 0,
  grade: 1,
  syncPictures: false,
  collectType: 0,
  state: true,
  interval: 0
})

// 新增源默认值: 附属站 + 采集间隔 3000ms
const DEFAULT_INTERVAL = 3000

// 采集片数 / 延迟 为健康度派生列, 无对应 CollectSource 字段, 用 sentinel key (#cell 内按字符串匹配)
// 单条断言为 Column, 避免整数组断言触发 TS "类型不充分重叠" 报错
const derivedCol = (key: string, label: string, width: string): Column<CollectSource> =>
  ({ key, label, width }) as Column<CollectSource>

// 模板里按 sentinel key 匹配派生列; col.key 类型为 keyof CollectSource, 与 'collected'/'latency'
// 字面量无重叠会被 vue-tsc 判 no-overlap, 故经 string 拓宽再比较
const isColKey = (col: Column<CollectSource>, key: string): boolean => (col.key as string) === key

const columns: Column<CollectSource>[] = [
  { key: 'name', label: '名称' },
  derivedCol('collected', '采集片数', '120px'),
  derivedCol('health', '测速', '240px'),
  { key: 'state', label: '状态', width: '80px', align: 'center' }
]

// 采集片数文案: 已采 / 总(目录)
function collectedText(id: string): string {
  const h = health[id]
  if (!h) return '—'
  const totalTxt = h.total > 0 ? String(h.total) : (h.films > 0 ? `≈${h.films}` : '?')
  return `${h.collected}/${totalTxt}`
}

type LatVariant = 'success' | 'warning' | 'danger' | 'default'

// 「采集」列: 服务端打采集 API 的延时
function apiLatInfo(id: string): { text: string; variant: LatVariant; title: string } {
  const h = health[id]
  if (!h) return { text: '—', variant: 'default', title: '尚未测采集' }
  const api = h.latencyMs > 0 ? h.latencyMs : 0
  const when = h.apiCheckedAt ? new Date(h.apiCheckedAt).toLocaleString() : ''
  const title = `服务端采集 API 延时 · ${when}`
  if (h.suppressed || h.status === 'down') return { text: '采·不可达', variant: 'danger', title }
  if (api > 0) {
    return {
      text: `采${api}ms`,
      variant: h.status === 'degraded' ? 'warning' : 'success',
      title
    }
  }
  return { text: '—', variant: 'default', title }
}

// 「播放」列: 浏览器端直连 CDN 的视频加载延时(真实用户网络)
function playLatInfo(id: string): { text: string; variant: LatVariant; title: string } {
  const h = health[id]
  if (!h || !(h.playLatencyWeb > 0)) return { text: '—', variant: 'default', title: '尚未测播放' }
  const when = h.playCheckedAt ? new Date(h.playCheckedAt).toLocaleString() : ''
  return {
    text: `▶${h.playLatencyWeb}ms`,
    variant: h.playLatencyWeb < 1500 ? 'success' : 'warning',
    title: `浏览器端视频加载延时 · ${when}`
  }
}

// 「播放延时」列: 端侧过滤已下线, 保留采集/播放两列测速供源站质量评估
// 「广告过滤」列: 已随服务端代理/中转下线移除 (2026-10-28)

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const resp = await manageApi.collect.list()
    rows.value = Array.isArray(resp) ? resp : []
  } catch (e: unknown) {
    // 失败明确报错并可重试: 此前无 catch, 加载失败复位居 rows 为空,
    // ManageTable 空态"暂无采集源, 点击右上角新增"把故障伪装成空库
    loadError.value = e instanceof Error ? e.message : '加载采集源失败'
  } finally {
    loading.value = false
  }
}

function resetForm(): void {
  form.id = ''
  form.name = ''
  form.uri = ''
  form.siteUrl = ''
  form.resultModel = 0
  form.grade = 1
  form.syncPictures = false
  form.collectType = 0
  form.state = true
  form.interval = 0
}

function openAdd(): void {
  editing.value = null
  resetForm()
  // 新增源默认: 附属站 + 采集间隔 3000ms (编辑时仍用源自身值)
  form.interval = DEFAULT_INTERVAL
  formError.value = ''
  dialogOpen.value = true
}

function openEdit(row: CollectSource): void {
  editing.value = row
  // 深拷贝，防止表单编辑直接污染列表 row
  Object.assign(form, { ...row })
  // 旧数据可能无 siteUrl 字段, 归一为字符串
  form.siteUrl = row.siteUrl ?? ''
  formError.value = ''
  dialogOpen.value = true
}

// 资源站 id 规则 —— 与后端 manage_service.collectSourceIDRe 保持一致。
// id 是主键且被播放源/失败台账/健康表以字符串引用(无外键), 落库后不可改。
const SOURCE_ID_RE = /^[a-z][a-z0-9_]{1,31}$/
const SITE_URL_RE = /^https?:\/\/\S+$/
// 采集 URI 与站点网址同一规则: 必须 http(s):// 完整链接
const SOURCE_URI_RE = /^https?:\/\/\S+$/

// 校验 + 保存; 返回是否成功(失败已在弹窗内联/toast 提示)
async function saveForm(): Promise<boolean> {
  formError.value = ''
  if (!SOURCE_ID_RE.test(form.id.trim())) {
    formError.value = '资源站标识不合法：小写字母开头，仅小写字母/数字/下划线，2~32 字符'
    return false
  }
  form.id = form.id.trim()
  // 名称/采集 URI 表单标注 required, 服务端 UpsertSource 亦不查非空 —— 前置守住,
  // 否则空 uri 源落库后采集探测必失败(且第二个空 uri 会被 ExistsByUri 误判成冲突)
  if (!form.name.trim()) {
    formError.value = '请填写名称'
    return false
  }
  form.name = form.name.trim()
  const uri = form.uri.trim()
  if (!uri || !SOURCE_URI_RE.test(uri)) {
    formError.value = '采集 URI 需为 http(s):// 开头的完整链接'
    return false
  }
  form.uri = uri
  const siteUrl = (form.siteUrl ?? '').trim()
  if (siteUrl && !SITE_URL_RE.test(siteUrl)) {
    formError.value = '站点网址需为 http(s):// 开头的完整链接, 或留空'
    return false
  }
  form.siteUrl = siteUrl
  // 多主站防护: 服务端不校验 grade=0 唯一, 误设双主站后两个都因主站禁删而卡死
  if (form.grade === 0) {
    const currentMaster = rows.value.find((r) => r.grade === 0 && r.id !== form.id)
    if (currentMaster) {
      const ok = await confirm({
        title: '确认再设一个主站？',
        desc: `「${currentMaster.name}」已是主站。主站并非唯一，实际主站由排序内定，且主站不可删除 —— 请确认是否继续。`,
        okText: '仍然保存'
      })
      if (!ok) return false
    }
  }
  submitting.value = true
  try {
    if (editing.value) await manageApi.collect.update({ ...form })
    else await manageApi.collect.add({ ...form })
    dialogOpen.value = false
    await load()
    await loadHealth() // 新增/编辑后健康面板同步(新源出现/主站变更影响汇总)
    return true
  } catch (e: unknown) {
    // 弹窗内联展示服务端错误(如"该采集 URI 已被其他源使用"),
    // 此前无 catch, ApiError 上抛成未处理 rejection 且弹窗无法就地显示
    formError.value = e instanceof Error ? e.message : '保存失败'
    return false
  } finally {
    submitting.value = false
  }
}

async function submit(): Promise<void> {
  await saveForm()
}

// 保存并立即测采集(弹窗里的「立即测速」): 落库成功后马上跑服务端采集测速
async function submitAndTest(): Promise<void> {
  const saved = await saveForm()
  if (!saved) return
  const row = rows.value.find((r) => r.id === form.id)
  if (row) await runApiTest(row)
}

// 行级 in-flight 标记: 启停/采集请求期间禁用该行按钮, 防连点翻转状态或重复触发任务
const busyId = ref('')

async function toggleState(row: CollectSource): Promise<void> {
  if (busyId.value) return
  busyId.value = row.id
  try {
    // 列表接口可能未返回完整字段，先用 find 拉全量再翻转 state
    let full: CollectSource = row
    try {
      full = (await manageApi.collect.find(row.id)) ?? row
    } catch {
      /* 忽略，回退用 row */
    }
    await manageApi.collect.change({ ...full, state: !full.state })
    await load()
    await loadHealth()
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '切换状态失败')
  } finally {
    busyId.value = ''
  }
}

async function remove(row: CollectSource): Promise<void> {
  const ok = await confirm({
    title: '确认删除采集源？',
    desc: `「${row.name}」删除后不可恢复`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  try {
    await manageApi.collect.remove(row.id)
    // 同步清掉该源的健康条目: 此前只 load 不清 health, 幽灵条目残留进汇总计数
    delete health[row.id]
    await load()
    await loadHealth()
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '删除失败')
  }
}

async function startSpider(row: CollectSource): Promise<void> {
  if (busyId.value || !row.state) return // 停用源服务端必拒(400), 直接置灰禁点
  busyId.value = row.id
  const hours = spiderHours.value
  const label = HOURS_OPTIONS.find(o => o.value === hours)?.label ?? `${hours}h`
  try {
    await manageApi.collect.startSpider({
      id: row.id,
      ids: [],
      time: hours,
      batch: false
    }, { silent: true } as never)
    toast('success', `已触发「${row.name}」${label} 采集任务`)
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : '触发采集失败'
    toast('error', msg)
  } finally {
    busyId.value = ''
  }
}

function openReset(): void {
  resetKey.value = ''
  resetError.value = ''
  resetDialogOpen.value = true
}

async function submitReset(): Promise<void> {
  resetError.value = ''
  if (!resetKey.value) {
    resetError.value = '请输入重置密钥'
    return
  }
  resetSubmitting.value = true
  try {
    await manageApi.collect.resetSpider(resetKey.value, { silent: true })
    toast('success', '影视数据已重置, 全量采集已开始, 请耐心等待')
    resetDialogOpen.value = false
  } catch (e: unknown) {
    resetError.value = e instanceof Error ? e.message : '重置失败'
  } finally {
    resetSubmitting.value = false
  }
}

// ---- 健康面板低频轮询(30s): 健康调度器自动停采/恢复、后台采集完成后汇总与徽标自行更新,
// 不必手动点刷新; 弹窗打开时暂停, 避免表单填写中途行序/数据跳动。参照 BannerListView。
const HEALTH_POLL_MS = 30_000
let healthTimer: number | undefined
function scheduleHealthPoll(): void {
  stopHealthPoll()
  healthTimer = window.setInterval(() => {
    if (dialogOpen.value || resetDialogOpen.value || testingAll.value || testingPlayAll.value) return
    void loadHealth()
  }, HEALTH_POLL_MS)
}
function stopHealthPoll(): void {
  if (healthTimer !== undefined) {
    window.clearInterval(healthTimer)
    healthTimer = undefined
  }
}
onBeforeUnmount(stopHealthPoll)

onMounted(() => {
  void load()
  void loadHealth()
  scheduleHealthPoll()
})
</script>

<template>
  <!-- 加载失败: 明确报错 + 重试, 不落到"暂无采集源"空态 -->
  <div
    v-if="loadError && !loading"
    class="flex items-center gap-[var(--jc-space-2)] bg-elevated border border-[var(--jc-danger)] rounded-[var(--jc-radius-md)] px-[var(--jc-space-4)] py-[var(--jc-space-3)] mb-[var(--jc-space-3)]"
  >
    <BaseIcon name="info" size="16px" class="text-[var(--jc-danger)]" />
    <span class="text-sm text-[var(--jc-danger)]">加载失败: {{ loadError }}</span>
    <BaseButton variant="outline" size="sm" class="ml-auto" @click="load">点击重试</BaseButton>
  </div>

  <ManageTable
    :columns="columns"
    :rows="displayRows"
    row-key="id"
    :loading="loading"
    actions-width="300px"
    :empty="loadError ? undefined : '暂无采集源，点击右上角新增'"
  >
    <template #toolbar>
      <div class="min-w-0">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">采集源管理</h2>
        <p class="text-xs text-muted mt-[var(--jc-space-1)] flex flex-wrap gap-x-[var(--jc-space-2)]">
          <template v-if="hasAnyHealth">
            <span class="text-[var(--jc-success)]">健康 {{ healthSummary.healthy }}</span>
            <span v-if="healthSummary.degraded" class="text-[var(--jc-warning)]">降级 {{ healthSummary.degraded }}</span>
            <span v-if="healthSummary.down" class="text-[var(--jc-danger)]">已停采 {{ healthSummary.down }}</span>
            <span v-if="healthSummary.untested">未测 {{ healthSummary.untested }}</span>
          </template>
          <span v-else class="text-muted">健康面板尚未检测</span>
        </p>
      </div>
      <div class="flex gap-[var(--jc-space-2)] items-center flex-wrap">
        <label class="text-sm text-secondary">采集时长:</label>
        <select
          v-model.number="spiderHours"
          class="bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-sm min-h-[44px] md:min-h-[36px]"
          data-focusable="true"
        >
          <option v-for="opt in HOURS_OPTIONS" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>
        <BaseButton variant="ghost" size="sm" :loading="testingAll" @click="runApiTestAll">
          <BaseIcon name="refresh" size="16px" /> 全部测采集
        </BaseButton>
        <BaseButton variant="ghost" size="sm" :loading="testingPlayAll" @click="runPlayTestAll">
          <BaseIcon name="refresh" size="16px" /> 全部测播放
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="cycleSort">
          {{ SORT_LABELS[sortMode] }}
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="load">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
        <BaseButton variant="danger" size="sm" @click="openReset">
          <BaseIcon name="trash" size="16px" /> 重置库存
        </BaseButton>
        <BaseButton variant="gradient" size="sm" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </BaseButton>
      </div>
    </template>

    <template #cell="{ row, col }">
      <BaseTag v-if="col.key === 'state'" :variant="row.state ? 'success' : 'default'">
        {{ row.state ? '启用' : '停用' }}
      </BaseTag>
      <!-- 名称列: 名称(有站点网址时可点跳转) + 类型/角色徽标 + URI 副行 -->
      <template v-else-if="col.key === 'name'">
        <div class="flex flex-col gap-[var(--jc-space-1)] min-w-0">
          <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap">
            <a
              v-if="row.siteUrl"
              :href="row.siteUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="font-[var(--jc-fw-medium)] text-primary underline decoration-dotted underline-offset-4 hover:text-[var(--jc-brand-primary)]"
              :title="`打开站点网址: ${row.siteUrl}`"
            >{{ row.name }}</a>
            <span v-else class="font-[var(--jc-fw-medium)] text-primary">{{ row.name }}</span>
            <BaseTag variant="default" size="xs" title="接口返回类型">
              {{ row.resultModel === 0 ? 'JSON' : 'XML' }}
            </BaseTag>
            <BaseTag v-if="health[row.id]?.isMaster" variant="purple" size="xs" title="当前主站(决定影片目录)">
              主站
            </BaseTag>
            <BaseTag
              v-else-if="row.id === recommendedMasterId"
              variant="success"
              size="xs"
              title="健康源中目录最全, 建议设为主站(换主站会重排 mid, 需手动确认)"
            >
              荐主站
            </BaseTag>
          </div>
          <span
            class="text-muted text-xs font-[var(--jc-font-mono)] truncate max-w-[280px]"
            :title="row.uri"
          >
            {{ row.uri }}
          </span>
        </div>
      </template>
      <!-- 采集片数列: 已采 / 目录总片数 (+ 停采/降级状态徽标) -->
      <template v-else-if="isColKey(col, 'collected')">
        <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap">
          <span
            class="font-[var(--jc-font-mono)] text-sm"
            :title="badges[row.id]?.title"
          >
            {{ collectedText(row.id) }}
          </span>
          <BaseTag
            v-if="health[row.id]?.suppressed || health[row.id]?.status === 'down'"
            variant="danger"
            size="xs"
            :title="badges[row.id]?.title"
          >
            已停采
          </BaseTag>
          <BaseTag
            v-else-if="health[row.id]?.status === 'degraded'"
            variant="warning"
            size="xs"
            :title="badges[row.id]?.title"
          >
            降级{{ health[row.id]?.consecutiveFails }}×
          </BaseTag>
        </div>
      </template>
      <!-- 测速列: 采集延时 / 播放延时 / 广告过滤可达性 三合一, 各带完整 tooltip -->
      <template v-else-if="isColKey(col, 'health')">
        <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap">
          <BaseTag
            :variant="apiLatInfo(row.id).variant"
            size="xs"
            :title="apiLatInfo(row.id).title"
          >
            {{ apiLatInfo(row.id).text }}
          </BaseTag>
          <BaseTag
            :variant="playLatInfo(row.id).variant"
            size="xs"
            :title="playLatInfo(row.id).title"
          >
            {{ playLatInfo(row.id).text }}
          </BaseTag>
        </div>
      </template>
      <span v-else>{{ row[col.key] ?? '—' }}</span>
    </template>

    <template #actions="{ row }">
      <div class="flex gap-[var(--jc-space-1)] justify-end items-center flex-wrap">
        <BaseButton
          variant="ghost"
          size="sm"
          :loading="testingId === row.id"
          title="服务端: 采集 API 延时 + 成功率"
          @click="runApiTest(row)"
        >
          测采集
        </BaseButton>
        <BaseButton
          variant="ghost"
          size="sm"
          :loading="testingPlayId === row.id"
          title="浏览器端: 直连 CDN 的视频加载速度"
          @click="runPlayTest(row)"
        >
          测播放
        </BaseButton>
        <BaseButton
          variant="ghost"
          size="sm"
          :disabled="!row.state || busyId === row.id"
          :title="row.state ? '触发采集任务' : '源已停用, 请先启用'"
          @click="startSpider(row)"
        >采集</BaseButton>
        <BaseButton
          variant="ghost"
          size="sm"
          :loading="busyId === row.id"
          :disabled="!!busyId && busyId !== row.id"
          @click="toggleState(row)"
        >
          {{ row.state ? '停用' : '启用' }}
        </BaseButton>
        <BaseButton
          variant="ghost"
          size="sm"
          :disabled="!!busyId"
          @click="openEdit(row)"
        >编辑</BaseButton>
        <BaseButton
          variant="danger"
          size="sm"
          :disabled="!!busyId"
          @click="remove(row)"
        >删除</BaseButton>
      </div>
    </template>
  </ManageTable>

  <ManageSheet
    v-model="dialogOpen"
    :title="editing ? '编辑采集源' : '新增采集源'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--jc-space-4)]">
      <ManageFormField v-if="!editing" label="资源站标识" required
        hint="规则：小写字母开头，仅小写字母/数字/下划线，2~32 字符；保存后不可修改（例：src_lz）">
        <ManageInput v-model="form.id" placeholder="例如：src_lz" />
      </ManageFormField>
      <ManageFormField label="名称" required>
        <ManageInput v-model="form.name" placeholder="例如：飞速影视" />
      </ManageFormField>
      <ManageFormField label="采集 URI" required>
        <ManageInput v-model="form.uri" placeholder="https://..." />
      </ManageFormField>
      <ManageFormField
        label="站点网址"
        hint="可选, 源站官网链接; 填了之后列表里站点名可点击跳转"
      >
        <ManageInput v-model="form.siteUrl" placeholder="https://example.com" />
      </ManageFormField>
      <ManageFormField label="返回类型" required>
        <select
          v-model.number="form.resultModel"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-3)]"
          data-focusable="true"
        >
          <option :value="0">JSON</option>
          <option :value="1">XML</option>
        </select>
      </ManageFormField>
      <ManageFormField label="站点等级">
        <select
          v-model.number="form.grade"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-3)]"
          data-focusable="true"
        >
          <option :value="0">主站</option>
          <option :value="1">附属</option>
        </select>
      </ManageFormField>
      <ManageFormField label="资源类型">
        <select
          v-model.number="form.collectType"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-3)]"
          data-focusable="true"
        >
          <option :value="0">视频</option>
          <option :value="1">文章</option>
          <option :value="2">演员</option>
          <option :value="3">角色</option>
          <option :value="4">网站</option>
        </select>
      </ManageFormField>
      <ManageFormField label="采集间隔（毫秒）">
        <ManageInput v-model="form.interval" type="number" placeholder="0" />
        <p class="mt-[var(--jc-space-1)] text-[var(--jc-text-tertiary)] text-[length:var(--jc-fs-caption)] leading-snug">
          ≥300 时该源按单线程限速采集；默认仅全量或超过 200 页时并发（主站 ≤12、附属 ≤8），增量采集自动单线程
        </p>
      </ManageFormField>
      <ManageFormField label="同步图片">
        <ManageSwitch v-model="form.syncPictures" />
      </ManageFormField>
      <ManageFormField label="启用">
        <ManageSwitch v-model="form.state" />
      </ManageFormField>
      <p v-if="formError" class="text-xs text-[var(--jc-danger)]">{{ formError }}</p>
    </div>
    <template #footer>
      <BaseButton variant="ghost" @click="dialogOpen = false">取消</BaseButton>
      <BaseButton variant="gradient" :loading="submitting" @click="submit">
        保存
      </BaseButton>
      <BaseButton variant="gradient" :loading="submitting" @click="submitAndTest">
        保存并测速
      </BaseButton>
    </template>
  </ManageSheet>

  <!-- 重置库存 confirm dialog (密钥校验) -->
  <ManageSheet v-model="resetDialogOpen" title="重置影片库存" mobile-mode="sheet">
    <div class="flex flex-col gap-[var(--jc-space-4)]">
      <p class="text-sm text-secondary">
        <BaseIcon name="info" size="14px" class="inline align-middle mr-1 text-[var(--jc-danger)]" />
        此操作将<strong class="text-[var(--jc-danger)]">清空全部影片数据</strong>并对所有已启用的采集源触发全量重新采集, <strong>不可撤销</strong>。
      </p>
      <ManageFormField label="重置密钥" required hint="联系系统管理员获取">
        <ManageInput v-model="resetKey" type="password" placeholder="请输入密钥" />
      </ManageFormField>
      <p v-if="resetError" class="text-xs text-[var(--jc-danger)]">{{ resetError }}</p>
    </div>
    <template #footer>
      <BaseButton variant="ghost" @click="resetDialogOpen = false">取消</BaseButton>
      <BaseButton variant="danger" :loading="resetSubmitting" @click="submitReset">
        确认重置
      </BaseButton>
    </template>
  </ManageSheet>
</template>
