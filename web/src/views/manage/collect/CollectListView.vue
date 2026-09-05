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
import { toast } from '@/api/http'

const rows = ref<CollectSource[]>([])
const loading = ref(true)

// ---- 健康度 / 测速 (持久化, 含自动停采) ----
const health = reactive<Record<string, SourceHealthRow>>({})
const testingId = ref('')
const testingAll = ref(false)
const sortByLatency = ref(false)

// ---- 浏览器端测速 (clientOnly 源: 服务端不可达, 走浏览器探测) ----
interface ClientHealthRow {
  ok: boolean
  latencyMs: number
  checkedAt: number
}
const clientHealth = reactive<Record<string, ClientHealthRow>>({})

const hasAnyHealth = computed(() => Object.keys(health).length > 0)

type Badge = { variant: 'success' | 'warning' | 'danger' | 'default'; label: string; title: string }
const badges = computed<Record<string, Badge>>(() => {
  const m: Record<string, Badge> = {}
  for (const id in health) {
    const h = health[id]
    if (!h) continue
    const when = h.checkedAt ? new Date(h.checkedAt).toLocaleString() : '未检测'
    const play = h.playLatencyMs > 0 ? `${h.playLatencyMs}ms` : '—'
    // 已采集 / 总片数: collected 实时(每次采集完成后即反映), total 为目录总量(测速时更新)
    const totalTxt = h.total > 0 ? String(h.total) : (h.films > 0 ? `≈${h.films}` : '?')
    const cnt = `采${h.collected}/${totalTxt}片`
    const detail = `${h.message} · 已采集 ${h.collected} / 目录 ${h.total} 片 · 采集 ${h.latencyMs}ms · 播放 ${play} · 成功 ${h.okCount}/${h.probes} · ${when}`
    if (h.suppressed || h.status === 'down') {
      m[id] = { variant: 'danger', label: `已停采 · ${cnt}`, title: detail }
    } else if (h.status === 'degraded') {
      m[id] = { variant: 'warning', label: `降级${h.consecutiveFails}× · ${cnt}`, title: detail }
    } else if (h.status === 'healthy') {
      // 直接展示用户关心的: 已采/总 + 播放延时
      m[id] = { variant: 'success', label: `${cnt} · ▶${play}`, title: detail }
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

// 按播放延时排序: 已测出播放延时的源升序, 其余排末尾
const displayRows = computed<CollectSource[]>(() => {
  if (!sortByLatency.value) return rows.value
  const rank = (id: string): number => {
    const h = health[id]
    return h && h.playLatencyMs > 0 ? h.playLatencyMs : Number.MAX_SAFE_INTEGER
  }
  return [...rows.value].sort((a, b) => rank(a.id) - rank(b.id))
})

async function loadHealth(): Promise<void> {
  try {
    const list = await manageApi.collect.health()
    for (const r of list) health[r.id] = r
  } catch {
    /* 忽略, 面板降级为无数据 */
  }
}

// 浏览器端探测 clientOnly 源可达性 + 耗时 (best-effort, no-cors 拿不到响应体, 仅计时/捕错)
async function runClientTest(row: CollectSource): Promise<void> {
  const uri = row.uri?.trim()
  if (!uri) {
    toast('error', '该源未配置采集 URI')
    return
  }
  testingId.value = row.id
  const t0 = performance.now()
  try {
    await fetch(uri, { mode: 'no-cors', cache: 'no-store' })
    const ms = Math.round(performance.now() - t0)
    clientHealth[row.id] = { ok: true, latencyMs: ms, checkedAt: Date.now() }
    toast('success', `「${row.name}」端侧可达 · ${ms}ms`)
  } catch {
    const ms = Math.round(performance.now() - t0)
    clientHealth[row.id] = { ok: false, latencyMs: ms, checkedAt: Date.now() }
    toast('error', `「${row.name}」端侧不可达`)
  } finally {
    testingId.value = ''
  }
}

async function runTest(row: CollectSource): Promise<void> {
  // clientOnly 源服务端测速无意义, 改走浏览器端探测
  if (row.clientOnly) {
    await runClientTest(row)
    return
  }
  testingId.value = row.id
  try {
    await manageApi.collect.test(row.id)
    await loadHealth()
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '测速失败')
  } finally {
    testingId.value = ''
  }
}

async function runTestAll(): Promise<void> {
  testingAll.value = true
  try {
    const list = await manageApi.collect.testAll()
    await loadHealth()
    sortByLatency.value = true
    toast('success', `测速完成: ${list.filter((r) => r.ok).length}/${list.length} 可用`)
  } catch (e: unknown) {
    toast('error', e instanceof Error ? e.message : '批量测速失败')
  } finally {
    testingAll.value = false
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

const form = reactive<CollectSource>({
  id: '',
  name: '',
  uri: '',
  resultModel: 0,
  grade: 0,
  syncPictures: false,
  collectType: 0,
  state: true,
  interval: 0,
  clientOnly: false
})

// 新增源默认采集间隔 (ms)
const DEFAULT_INTERVAL = 1000

// clientOnly 在 CollectSource 上为可选(boolean|undefined), ManageSwitch 需 boolean, 用 computed 归一
const formClientOnly = computed<boolean>({
  get: () => form.clientOnly === true,
  set: (v) => { form.clientOnly = v }
})

// 采集片数 / 延迟 为健康度派生列, 无对应 CollectSource 字段, 用 sentinel key (#cell 内按字符串匹配)
// 单条断言为 Column, 避免整数组断言触发 TS "类型不充分重叠" 报错
const derivedCol = (key: string, label: string, width: string): Column<CollectSource> =>
  ({ key, label, width }) as Column<CollectSource>

// 模板里按 sentinel key 匹配派生列; col.key 类型为 keyof CollectSource, 与 'collected'/'latency'
// 字面量无重叠会被 vue-tsc 判 no-overlap, 故经 string 拓宽再比较
const isColKey = (col: Column<CollectSource>, key: string): boolean => (col.key as string) === key

const columns: Column<CollectSource>[] = [
  { key: 'name', label: '名称' },
  { key: 'uri', label: '采集 URI' },
  { key: 'resultModel', label: '类型', width: '90px' },
  { key: 'grade', label: '等级', width: '80px' },
  derivedCol('collected', '采集片数', '120px'),
  derivedCol('latency', '延迟', '120px'),
  { key: 'state', label: '状态', width: '80px', align: 'center' }
]

// 采集片数文案: 已采 / 总(目录)
function collectedText(id: string): string {
  const h = health[id]
  if (!h) return '—'
  const totalTxt = h.total > 0 ? String(h.total) : (h.films > 0 ? `≈${h.films}` : '?')
  return `${h.collected}/${totalTxt}`
}

// 延迟文案 + 颜色: 优先播放延时, 否则采集 API 延时
function latencyInfo(id: string): { text: string; variant: 'success' | 'warning' | 'danger' | 'default' } {
  // clientOnly 浏览器测速结果优先
  const c = clientHealth[id]
  if (c) {
    if (!c.ok) return { text: '不可达', variant: 'danger' }
    return { text: `${c.latencyMs}ms`, variant: c.latencyMs < 800 ? 'success' : 'warning' }
  }
  const h = health[id]
  if (!h) return { text: '—', variant: 'default' }
  const play = h.playLatencyMs > 0 ? h.playLatencyMs : 0
  const api = h.latencyMs > 0 ? h.latencyMs : 0
  if (h.suppressed || h.status === 'down') return { text: '不可达', variant: 'danger' }
  if (play > 0) return { text: `▶${play}ms`, variant: play < 1500 ? 'success' : 'warning' }
  if (api > 0) return { text: `${api}ms`, variant: h.status === 'degraded' ? 'warning' : 'success' }
  return { text: h.status === 'degraded' ? '降级' : '—', variant: h.status === 'degraded' ? 'warning' : 'default' }
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
  form.resultModel = 0
  form.grade = 0
  form.syncPictures = false
  form.collectType = 0
  form.state = true
  form.interval = 0
  form.clientOnly = false
}

function openAdd(): void {
  editing.value = null
  resetForm()
  // 新增源默认采集间隔 1000ms (编辑时仍用源自身值)
  form.interval = DEFAULT_INTERVAL
  dialogOpen.value = true
}

function openEdit(row: CollectSource): void {
  editing.value = row
  // 深拷贝，防止表单编辑直接污染列表 row
  Object.assign(form, { ...row })
  // 旧源可能无 clientOnly 字段, 显式归一为布尔, 防上次表单值残留
  form.clientOnly = row.clientOnly === true
  dialogOpen.value = true
}

async function submit(): Promise<void> {
  submitting.value = true
  try {
    if (editing.value) await manageApi.collect.update({ ...form })
    else await manageApi.collect.add({ ...form })
    dialogOpen.value = false
    await load()
  } finally {
    submitting.value = false
  }
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
        <BaseButton variant="ghost" size="sm" :loading="testingAll" @click="runTestAll">
          <BaseIcon name="refresh" size="16px" /> 全部测速
        </BaseButton>
        <BaseButton
          v-if="hasAnyHealth"
          variant="ghost"
          size="sm"
          @click="sortByLatency = !sortByLatency"
        >
          {{ sortByLatency ? '默认排序' : '按播放延时排序' }}
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
      <BaseTag v-else-if="col.key === 'resultModel'" variant="purple" size="xs">
        {{ row.resultModel === 0 ? 'JSON' : 'XML' }}
      </BaseTag>
      <BaseTag v-else-if="col.key === 'grade'" variant="default" size="xs">
        {{ row.grade === 0 ? '主站' : '附属' }}
      </BaseTag>
      <!-- 名称列: 名称 + 可达性/角色徽标 -->
      <template v-else-if="col.key === 'name'">
        <div class="flex items-center gap-[var(--gf-space-2)] flex-wrap">
          <span class="font-[var(--gf-fw-medium)] text-primary">{{ row.name }}</span>
          <BaseTag v-if="row.clientOnly" variant="info" size="xs" title="服务器不可达, 测速走浏览器端">
            仅端侧
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
      </template>
      <span
        v-else-if="col.key === 'uri'"
        class="text-muted text-xs font-[var(--gf-font-mono)] break-all"
      >
        {{ row.uri }}
      </span>
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
      <!-- 延迟列: 播放/采集延时 或 端侧探测延时 -->
      <template v-else-if="isColKey(col, 'latency')">
        <BaseTag
          :variant="latencyInfo(row.id).variant"
          size="xs"
          :title="badges[row.id]?.title"
        >
          {{ latencyInfo(row.id).text }}
        </BaseTag>
      </template>
      <span v-else>{{ row[col.key] ?? '—' }}</span>
    </template>

    <template #actions="{ row }">
      <div class="flex gap-[var(--gf-space-1)] justify-end items-center flex-wrap">
        <BaseButton
          variant="ghost"
          size="sm"
          :loading="testingId === row.id"
          @click="runTest(row)"
        >
          测速
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
      <ManageFormField v-if="!editing" label="资源站标识" required hint="唯一英文 id, 如 lzi / fs (保存后不可改)">
        <ManageInput v-model="form.id" placeholder="例如：lzi" />
      </ManageFormField>
      <ManageFormField label="名称" required>
        <ManageInput v-model="form.name" placeholder="例如：飞速影视" />
      </ManageFormField>
      <ManageFormField label="采集 URI" required>
        <ManageInput v-model="form.uri" placeholder="https://..." />
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
      <ManageFormField
        label="仅端侧可达"
        hint="服务器无法访问该源(如 bf): 服务端不测速/不自动停采, 测速改走浏览器端"
      >
        <ManageSwitch v-model="formClientOnly" />
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
