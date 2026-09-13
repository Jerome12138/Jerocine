<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
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

// ---- 健康度 / 测速 (持久化, 含自动停采) ----
// 测速拆两类: 测采集(服务端打采集 API, 顺带判定广告过滤可达性) / 测播放(浏览器端直连 CDN 计时)。
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
      `广告过滤 ${h.adFilterOk === null || h.adFilterOk === undefined ? '未测' : h.adFilterOk ? '可用' : '不可达'}`,
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
    for (const r of list) health[r.id] = r
  } catch {
    /* 忽略, 面板降级为无数据 */
  }
}

// 「测采集」: 服务端打采集 API(3 次探测取中位), 顺带产出广告过滤可达性, 写健康度
async function runApiTest(row: CollectSource): Promise<void> {
  testingId.value = row.id
  try {
    const res = await manageApi.collect.test(row.id)
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
    await manageApi.collect.recordPlayLatency(row.id, ms)
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
    const list = await manageApi.collect.testAll()
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
        await manageApi.collect.recordPlayLatency(row.id, ms)
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

// 「广告过滤」列: 服务端 m3u8 可达性 → 服务端代理过滤链路是否可用
function adFilterInfo(id: string): { text: string; variant: LatVariant; title: string } {
  const h = health[id]
  if (!h || h.adFilterOk === null || h.adFilterOk === undefined) {
    return { text: '—', variant: 'default', title: '尚未判定(测采集时自动判定)' }
  }
  const when = h.adFilterAt ? new Date(h.adFilterAt).toLocaleString() : ''
  if (!h.adFilterOk) {
    return { text: '✗ 不可达', variant: 'danger', title: `服务端拉不到 m3u8, 代理过滤不可用(播放时自动走直链) · ${when}` }
  }
  return {
    text: '✓ 可用',
    variant: 'success',
    title: `服务端 m3u8 ${h.playLatencyMs > 0 ? `${h.playLatencyMs}ms` : ''} 可代理过滤 · ${when}`
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await manageApi.collect.list()
    rows.value = Array.isArray(resp) ? resp : []
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
  dialogOpen.value = true
}

function openEdit(row: CollectSource): void {
  editing.value = row
  // 深拷贝，防止表单编辑直接污染列表 row
  Object.assign(form, { ...row })
  // 旧数据可能无 siteUrl 字段, 归一为字符串
  form.siteUrl = row.siteUrl ?? ''
  dialogOpen.value = true
}

// 资源站 id 规则 —— 与后端 manage_service.collectSourceIDRe 保持一致。
// id 是主键且被播放源/失败台账/健康表以字符串引用(无外键), 落库后不可改。
const SOURCE_ID_RE = /^[a-z][a-z0-9_]{1,31}$/
const SITE_URL_RE = /^https?:\/\/\S+$/

// 校验 + 保存; 返回是否成功(失败已 toast)
async function saveForm(): Promise<boolean> {
  if (!SOURCE_ID_RE.test(form.id.trim())) {
    toast('error', '资源站标识不合法：小写字母开头，仅小写字母/数字/下划线，2~32 字符')
    return false
  }
  form.id = form.id.trim()
  const siteUrl = (form.siteUrl ?? '').trim()
  if (siteUrl && !SITE_URL_RE.test(siteUrl)) {
    toast('error', '站点网址需为 http(s):// 开头的完整链接, 或留空')
    return false
  }
  form.siteUrl = siteUrl
  submitting.value = true
  try {
    if (editing.value) await manageApi.collect.update({ ...form })
    else await manageApi.collect.add({ ...form })
    dialogOpen.value = false
    await load()
    return true
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

async function toggleState(row: CollectSource): Promise<void> {
  // 列表接口可能未返回完整字段，先用 find 拉全量再翻转 state
  let full: CollectSource = row
  try {
    full = (await manageApi.collect.find(row.id)) ?? row
  } catch {
    /* 忽略，回退用 row */
  }
  await manageApi.collect.change({ ...full, state: !full.state })
  await load()
}

async function remove(row: CollectSource): Promise<void> {
  const ok = await confirm({
    title: '确认删除采集源？',
    desc: `「${row.name}」删除后不可恢复`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  await manageApi.collect.remove(row.id)
  await load()
}

async function startSpider(row: CollectSource): Promise<void> {
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
    await manageApi.collect.resetSpider(resetKey.value)
    toast('success', '影视数据已重置, 全量采集已开始, 请耐心等待')
    resetDialogOpen.value = false
  } catch (e: unknown) {
    resetError.value = e instanceof Error ? e.message : '重置失败'
  } finally {
    resetSubmitting.value = false
  }
}

onMounted(() => {
  void load()
  void loadHealth()
})
</script>

<template>
  <ManageTable
    :columns="columns"
    :rows="displayRows"
    row-key="id"
    :loading="loading"
    actions-width="300px"
    empty="暂无采集源，点击右上角新增"
  >
    <template #toolbar>
      <div class="min-w-0">
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">采集源管理</h2>
        <p v-if="hasAnyHealth" class="text-xs text-muted mt-[var(--gf-space-1)] flex flex-wrap gap-x-[var(--gf-space-2)]">
          <span class="text-[var(--gf-success)]">健康 {{ healthSummary.healthy }}</span>
          <span v-if="healthSummary.degraded" class="text-[var(--gf-warning)]">降级 {{ healthSummary.degraded }}</span>
          <span v-if="healthSummary.down" class="text-[var(--gf-danger)]">已停采 {{ healthSummary.down }}</span>
          <span v-if="healthSummary.untested">未测 {{ healthSummary.untested }}</span>
        </p>
      </div>
      <div class="flex gap-[var(--gf-space-2)] items-center flex-wrap">
        <label class="text-sm text-secondary">采集时长:</label>
        <select
          v-model.number="spiderHours"
          class="bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-2)] text-sm min-h-[44px] md:min-h-[36px]"
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
        <div class="flex flex-col gap-[var(--gf-space-1)] min-w-0">
          <div class="flex items-center gap-[var(--gf-space-2)] flex-wrap">
            <a
              v-if="row.siteUrl"
              :href="row.siteUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="font-[var(--gf-fw-medium)] text-primary underline decoration-dotted underline-offset-4 hover:text-[var(--gf-brand-primary)]"
              :title="`打开站点网址: ${row.siteUrl}`"
            >{{ row.name }}</a>
            <span v-else class="font-[var(--gf-fw-medium)] text-primary">{{ row.name }}</span>
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
            class="text-muted text-xs font-[var(--gf-font-mono)] truncate max-w-[280px]"
            :title="row.uri"
          >
            {{ row.uri }}
          </span>
        </div>
      </template>
      <!-- 采集片数列: 已采 / 目录总片数 (+ 停采/降级状态徽标) -->
      <template v-else-if="isColKey(col, 'collected')">
        <div class="flex items-center gap-[var(--gf-space-1)] flex-wrap">
          <span
            class="font-[var(--gf-font-mono)] text-sm"
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
        <div class="flex items-center gap-[var(--gf-space-1)] flex-wrap">
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
          <BaseTag
            :variant="adFilterInfo(row.id).variant"
            size="xs"
            :title="`广告过滤 · ${adFilterInfo(row.id).title}`"
          >
            {{ adFilterInfo(row.id).text }}
          </BaseTag>
        </div>
      </template>
      <span v-else>{{ row[col.key] ?? '—' }}</span>
    </template>

    <template #actions="{ row }">
      <div class="flex gap-[var(--gf-space-1)] justify-end items-center flex-wrap">
        <BaseButton
          variant="ghost"
          size="sm"
          :loading="testingId === row.id"
          title="服务端: 采集 API 延时 + m3u8 可达性(广告过滤)"
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
        <BaseButton variant="ghost" size="sm" @click="startSpider(row)">采集</BaseButton>
        <BaseButton variant="ghost" size="sm" @click="toggleState(row)">
          {{ row.state ? '停用' : '启用' }}
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="openEdit(row)">编辑</BaseButton>
        <BaseButton variant="danger" size="sm" @click="remove(row)">删除</BaseButton>
      </div>
    </template>
  </ManageTable>

  <ManageSheet
    v-model="dialogOpen"
    :title="editing ? '编辑采集源' : '新增采集源'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--gf-space-4)]">
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
          class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-3)]"
          data-focusable="true"
        >
          <option :value="0">JSON</option>
          <option :value="1">XML</option>
        </select>
      </ManageFormField>
      <ManageFormField label="站点等级">
        <select
          v-model.number="form.grade"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-3)]"
          data-focusable="true"
        >
          <option :value="0">主站</option>
          <option :value="1">附属</option>
        </select>
      </ManageFormField>
      <ManageFormField label="资源类型">
        <select
          v-model.number="form.collectType"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-3)]"
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
      </ManageFormField>
      <ManageFormField label="同步图片">
        <ManageSwitch v-model="form.syncPictures" />
      </ManageFormField>
      <ManageFormField label="启用">
        <ManageSwitch v-model="form.state" />
      </ManageFormField>
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
    <div class="flex flex-col gap-[var(--gf-space-4)]">
      <p class="text-sm text-secondary">
        <BaseIcon name="info" size="14px" class="inline align-middle mr-1 text-[var(--gf-danger)]" />
        此操作将<strong class="text-[var(--gf-danger)]">清空全部影片数据</strong>并对所有已启用的采集源触发全量重新采集, <strong>不可撤销</strong>。
      </p>
      <ManageFormField label="重置密钥" required hint="联系系统管理员获取">
        <ManageInput v-model="resetKey" type="password" placeholder="请输入密钥" />
      </ManageFormField>
      <p v-if="resetError" class="text-xs text-[var(--gf-danger)]">{{ resetError }}</p>
    </div>
    <template #footer>
      <BaseButton variant="ghost" @click="resetDialogOpen = false">取消</BaseButton>
      <BaseButton variant="danger" :loading="resetSubmitting" @click="submitReset">
        确认重置
      </BaseButton>
    </template>
  </ManageSheet>
</template>
