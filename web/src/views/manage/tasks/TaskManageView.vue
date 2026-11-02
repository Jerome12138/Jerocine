<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as collectApi from '@/api/manage/collect'
import * as tasksApi from '@/api/manage/tasks'
import type { SpiderJob } from '@/api/manage/collect'
import type { CollectFailure, TaskOverview, TaskRun } from '@/types/manage'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import ManageTable, { type Column } from '@/components/manage/ManageTable.vue'
import { confirm } from '@/composables/useConfirm'
import { toast } from '@/api/http'

/**
 * 任务管理: 统一展示全部定时/手动任务。
 * - 统计卡: 运行中 / 今日成功 / 今日失败 / 待补采
 * - 运行中: 采集 job 实时进度(Redis), 失败显示原因, 支持暂停/继续/取消/重跑
 * - 历史记录: 任务运行台账(task_run), 失败原因可查, 失败任务一键重跑
 *   (重跑是后台受理: 榜单刷新实测 1-2 分钟, 服务端登记台账后即返回 202, 结果看历史)
 * - 失败记录: 页级失败台账(待补采, 分页) + 补采全部/单条
 *
 * 提示口径: 写操作传 { silent: true } 自管提示(拦截器的通用"操作成功"/错误 toast 会与
 * 页面差异化文案叠两条); 后台轮询用的读接口也 silent, 失败只降级页内状态不刷 toast。
 */
type TabKey = 'running' | 'history' | 'failures'

const activeTab = ref<TabKey>('running')

// ---- 统计卡 ----
const ov = ref<TaskOverview>({ running: 0, todaySuccess: 0, todayFailed: 0, pendingFailures: 0 })

// ---- 运行中 job ----
const jobs = ref<SpiderJob[]>([])
const jobsLoading = ref(false)
const jobsError = ref('') // 列表加载失败: 页内错误态+重试, 不伪装成"暂无任务"

// ---- 历史台账 ----
const runs = ref<TaskRun[]>([])
const runsLoading = ref(false)
const runStatus = ref('')
const runType = ref('')
const runPage = ref(1)
const runTotal = ref(0)
const RUN_SIZE = 20

interface RunRow extends TaskRun {
  elapsedMs: number
}
const runRows = computed<RunRow[]>(() =>
  runs.value.map((r) => ({ ...r, elapsedMs: Math.max(0, r.endedAt - r.startedAt) }))
)

// ---- 失败台账(待补采) ----
const failures = ref<CollectFailure[]>([])
const failTotal = ref(0)
const failPage = ref(1)
const FAIL_SIZE = 20 // 与补采中心页(/manage/collect/failures)对齐
const failBusy = ref(false)
const sourceNames = ref<Record<string, string>>({})

const POLL_ACTIVE_MS = 5000
const POLL_IDLE_MS = 30_000
let timer: number | undefined

const hasActive = (): boolean =>
  jobs.value.some((j) => j.state === 'running' || j.state === 'paused') ||
  runs.value.some((r) => r.status === 'running')

async function loadOverview(): Promise<void> {
  try {
    ov.value = await tasksApi.overview({ silent: true })
  } catch {
    /* 统计卡失败不阻塞页面 */
  }
}

async function loadJobs(): Promise<void> {
  jobsLoading.value = true
  try {
    jobs.value = (await collectApi.spiderJobs({ silent: true })) ?? []
    jobsError.value = ''
  } catch (e) {
    // 轮询失败不弹全局 toast(后台静默重试), 页内给错误态+重试
    jobsError.value = (e as Error)?.message ?? '加载运行中任务失败'
  } finally {
    jobsLoading.value = false
  }
}

async function loadRuns(): Promise<void> {
  runsLoading.value = true
  try {
    const resp = await tasksApi.runs({
      status: runStatus.value || undefined,
      type: runType.value || undefined,
      page: runPage.value,
      size: RUN_SIZE
    })
    runs.value = resp?.list ?? []
    runTotal.value = resp?.page?.total ?? 0
  } catch {
    /* 错误提示由 http 拦截器全局弹, 这里不重复 */
  } finally {
    runsLoading.value = false
  }
}

async function loadFailures(): Promise<void> {
  try {
    const resp = await collectApi.failures(
      { status: 0, page: failPage.value, size: FAIL_SIZE },
      { silent: true }
    )
    failures.value = resp?.list ?? []
    failTotal.value = resp?.page?.total ?? 0
  } catch {
    /* 失败台账加载失败不阻塞 */
  }
}

async function loadSources(): Promise<void> {
  try {
    const list = (await collectApi.list({ silent: true })) ?? []
    const map: Record<string, string> = {}
    for (const s of list) map[s.id] = s.name || s.id
    sourceNames.value = map
  } catch {
    /* 源名缺失时直接显示 id */
  }
}

async function loadAll(): Promise<void> {
  void loadOverview()
  if (activeTab.value === 'running') void loadJobs()
  if (activeTab.value === 'history') void loadRuns()
  if (activeTab.value === 'failures') void loadFailures()
}

function schedulePoll(): void {
  if (timer) window.clearTimeout(timer)
  const delay = hasActive() ? POLL_ACTIVE_MS : POLL_IDLE_MS
  timer = window.setTimeout(() => {
    void loadAll()
    schedulePoll()
  }, delay)
}

function stopPoll(): void {
  if (timer) {
    window.clearTimeout(timer)
    timer = undefined
  }
}

function switchTab(tab: TabKey): void {
  activeTab.value = tab
  void loadAll()
}

onMounted(() => {
  void loadSources()
  void loadAll()
  schedulePoll()
})

onBeforeUnmount(stopPoll)

// ---- 运行中 job 操作 ----
// 卡片级防连点: 操作请求在途时禁用该卡片全部按钮(双击重跑会登记两条台账,
// 第二条被服务端判同源冲突记成 canceled 垃圾行)
const busyId = ref('')

async function pause(id: string): Promise<void> {
  if (busyId.value) return
  busyId.value = id
  try {
    await collectApi.spiderJobPause(id, { silent: true })
    toast('success', '已暂停')
    void loadJobs()
  } catch (e) {
    toast('error', (e as Error).message ?? '暂停失败')
  } finally {
    busyId.value = ''
  }
}

async function resume(id: string): Promise<void> {
  if (busyId.value) return
  busyId.value = id
  try {
    await collectApi.spiderJobResume(id, { silent: true })
    toast('success', '已继续')
    void loadJobs()
  } catch (e) {
    toast('error', (e as Error).message ?? '继续失败')
  } finally {
    busyId.value = ''
  }
}

async function cancelJob(j: SpiderJob): Promise<void> {
  const ok = await confirm({
    title: '取消采集任务',
    desc: `确认取消 [${j.sourceName}] 的采集? 已完成的页面会保留, 未完成的页面会丢弃.`
  })
  if (!ok) return
  if (busyId.value) return
  busyId.value = j.sourceId
  try {
    await collectApi.spiderJobCancel(j.sourceId, { silent: true })
    toast('success', '已取消')
    void loadJobs()
  } catch (e) {
    toast('error', (e as Error).message ?? '取消失败')
  } finally {
    busyId.value = ''
  }
}

/** 重跑一个已失败的采集 job: 按原时长(hour)重新触发同源采集 */
async function rerunJob(j: SpiderJob): Promise<void> {
  const ok = await confirm({
    title: '重跑采集任务',
    desc: `重新采集 [${j.sourceName}]? 时长 ${j.hour > 0 ? `近 ${j.hour}h` : '全量'}.`
  })
  if (!ok) return
  if (busyId.value) return
  busyId.value = j.sourceId
  try {
    await collectApi.startSpider(
      { id: j.sourceId, ids: [], time: j.hour > 0 ? j.hour : -1, batch: false },
      { silent: true }
    )
    toast('success', '已重新触发')
    void loadJobs()
  } catch (e) {
    toast('error', (e as Error).message ?? '重跑失败')
  } finally {
    busyId.value = ''
  }
}

// ---- 历史台账操作 ----
// 行级防连点: 重跑请求在途时禁用该行按钮。服务端是后台受理(202 立即返回),
// 等待窗口虽短, 双击仍会登记两条台账(榜单刷新第二条撞 busy 记 canceled 行)。
const rerunBusyId = ref(0)

async function rerunRun(r: TaskRun): Promise<void> {
  const ok = await confirm({
    title: '重跑任务',
    desc: `重新执行 [${r.name}]? 会登记一条新的任务记录.`
  })
  if (!ok) return
  if (rerunBusyId.value) return
  rerunBusyId.value = r.id
  try {
    await tasksApi.rerun(r.id, { silent: true })
    // 后台受理: 服务端登记台账后即返回, 榜单刷新要跑 1-2 分钟, 不在这里等结果
    toast('success', '已提交, 结果见历史记录')
    runPage.value = 1
    void loadRuns()
    void loadOverview()
  } catch (e) {
    toast('error', (e as Error).message ?? '重跑提交失败')
  } finally {
    rerunBusyId.value = 0
  }
}

function changeRunStatus(): void {
  runPage.value = 1
  void loadRuns()
}

function changeRunPage(p: number): void {
  runPage.value = p
  void loadRuns()
}

function changeFailPage(p: number): void {
  failPage.value = p
  void loadFailures()
}

// ---- 失败记录操作 ----
async function recover(ids?: number[]): Promise<void> {
  failBusy.value = true
  try {
    const r = await collectApi.recoverFailures(ids, { silent: true })
    toast('success', `已提交补采 (待处理 ${r.pending} 条)`)
    void loadFailures()
    void loadOverview()
  } catch (e) {
    toast('error', (e as Error).message ?? '补采提交失败')
  } finally {
    failBusy.value = false
  }
}

// ---- 展示格式化 ----
function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function fmtTime(ms: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function fmtDuration(ms: number): string {
  if (ms <= 0) return '—'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ${s % 60}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

function fmtPercent(j: SpiderJob): number {
  if (j.totalPages <= 0) return 0
  return Math.min(100, Math.round((j.donePages / j.totalPages) * 100))
}

function jobStateVariant(s: SpiderJob['state']): 'default' | 'brand' | 'warning' | 'success' | 'danger' {
  switch (s) {
    case 'running':
      return 'brand'
    case 'paused':
      return 'warning'
    case 'done':
      return 'success'
    case 'error':
      return 'danger'
    default:
      return 'default'
  }
}

function jobStateText(s: SpiderJob['state']): string {
  switch (s) {
    case 'running':
      return '采集中'
    case 'paused':
      return '已暂停'
    case 'done':
      return '已完成'
    case 'error':
      return '失败'
    case 'canceled':
      return '已取消'
    default:
      return '等待中'
  }
}

function runStatusVariant(s: TaskRun['status']): 'default' | 'brand' | 'success' | 'danger' | 'warning' {
  switch (s) {
    case 'running':
      return 'brand'
    case 'success':
      return 'success'
    case 'failed':
      return 'danger'
    case 'canceled':
      return 'warning'
    default:
      return 'default'
  }
}

function runStatusText(s: TaskRun['status']): string {
  switch (s) {
    case 'running':
      return '运行中'
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    case 'canceled':
      return '已取消'
    default:
      return s
  }
}

function runKindText(kind: TaskRun['kind']): string {
  return kind === 'cron' ? '定时' : '手动'
}

function runTypeText(t: TaskRun['type']): string {
  switch (t) {
    case 'collect':
      return '采集'
    case 'recover':
      return '补采'
    case 'category_cover':
      return '分类覆盖'
    case 'hot_refresh':
      return '榜单刷新'
    default:
      return t
  }
}

const historyColumns: Column<RunRow>[] = [
  { key: 'startedAt', label: '开始时间', width: '150px' },
  { key: 'name', label: '任务', width: '200px' },
  { key: 'kind', label: '触发', width: '70px', align: 'center' },
  { key: 'type', label: '类型', width: '90px', align: 'center' },
  { key: 'status', label: '状态', width: '90px', align: 'center' },
  { key: 'message', label: '结果', width: '240px' },
  { key: 'error', label: '失败原因' },
  { key: 'elapsedMs', label: '耗时', width: '90px', align: 'center' }
]

const failureColumns: Column<CollectFailure>[] = [
  { key: 'createdAt', label: '失败时间', width: '150px' },
  { key: 'sourceId', label: '采集源', width: '120px' },
  { key: 'pageNo', label: '页码', width: '80px', align: 'center' },
  { key: 'cause', label: '失败原因' },
  { key: 'attempts', label: '重复', width: '70px', align: 'center' },
  { key: 'status', label: '状态', width: '90px', align: 'center' }
]

const canRecoverAll = computed(() => ov.value.pendingFailures > 0)

const statCard = (_label: string, _value: number | string, danger = false): string =>
  `stat-card${danger ? ' stat-card--danger' : ''}`

interface TabItem {
  key: TabKey
  label: string
}
const tabs = computed<TabItem[]>(() => [
  {
    key: 'running',
    label: `运行中 (${jobs.value.filter((j) => j.state === 'running' || j.state === 'paused').length})`
  },
  { key: 'history', label: `历史记录 (${runTotal.value})` },
  { key: 'failures', label: '失败记录' }
])
</script>

<template>
  <div class="container-page py-[var(--jc-space-5)]">
    <header class="flex items-center justify-between mb-[var(--jc-space-5)]">
      <div>
        <h1 class="text-2xl font-[var(--jc-fw-semibold)]">任务管理</h1>
        <p class="text-muted text-sm mt-[var(--jc-space-1)]">
          全部定时 / 手动任务的实时状态、失败原因与重跑入口
        </p>
      </div>
      <div class="flex items-center gap-[var(--jc-space-3)]">
        <span class="text-muted text-sm" v-if="jobsLoading || runsLoading">刷新中…</span>
        <BaseButton variant="outline" size="sm" @click="loadAll">手动刷新</BaseButton>
      </div>
    </header>

    <!-- 统计卡 -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-[var(--jc-space-3)] mb-[var(--jc-space-5)]">
      <div :class="statCard('running', ov.running)">
        <div class="stat-card__value">{{ ov.running }}</div>
        <div class="stat-card__label">运行中任务</div>
      </div>
      <div :class="statCard('todaySuccess', ov.todaySuccess)">
        <div class="stat-card__value">{{ ov.todaySuccess }}</div>
        <div class="stat-card__label">今日成功</div>
      </div>
      <div :class="statCard('todayFailed', ov.todayFailed, ov.todayFailed > 0)">
        <div class="stat-card__value">{{ ov.todayFailed }}</div>
        <div class="stat-card__label">今日失败</div>
      </div>
      <div :class="statCard('pendingFailures', ov.pendingFailures, ov.pendingFailures > 0)">
        <div class="stat-card__value">{{ ov.pendingFailures }}</div>
        <div class="stat-card__label">待补采失败页</div>
      </div>
    </div>

    <!-- 页签 -->
    <div class="flex items-center gap-[var(--jc-space-2)] border-b border-subtle mb-[var(--jc-space-4)]">
      <button
        v-for="t in tabs"
        :key="t.key"
        class="px-[var(--jc-space-4)] py-[var(--jc-space-2)] text-sm transition-colors border-b-2 -mb-px"
        :class="
          activeTab === t.key
            ? 'border-[var(--jc-brand-purple)] text-[var(--jc-brand-purple)] font-[var(--jc-fw-medium)]'
            : 'border-transparent text-muted hover:text-primary'
        "
        @click="switchTab(t.key)"
      >
        {{ t.label }}
      </button>
    </div>

    <!-- ===== 运行中 ===== -->
    <template v-if="activeTab === 'running'">
      <!-- 加载中: 占位, 不与空态混同 -->
      <div v-if="jobsLoading && jobs.length === 0" class="jc-tm-empty">加载中…</div>

      <!-- 首次加载失败: 明确报错 + 重试, 不落入"暂无任务"空态误导 -->
      <div v-else-if="jobsError && jobs.length === 0" class="jc-tm-empty">
        加载失败: {{ jobsError }}.
        <div class="mt-[var(--jc-space-3)]">
          <BaseButton variant="outline" size="sm" @click="loadJobs">点击重试</BaseButton>
        </div>
      </div>

      <div v-else-if="jobs.length === 0" class="jc-tm-empty">
        暂无正在运行 / 最近 30 分钟内结束的采集任务. 去
        <RouterLink to="/manage/collect" class="text-link">采集源列表</RouterLink>
        启动一个.
      </div>

      <div v-else class="flex flex-col gap-[var(--jc-space-4)]">
        <article v-for="j in jobs" :key="j.sourceId" class="jc-job">
          <header class="jc-job__head">
            <span class="jc-job__name">{{ j.sourceName }}</span>
            <BaseTag :variant="jobStateVariant(j.state)" size="sm">{{ jobStateText(j.state) }}</BaseTag>
            <span class="text-muted text-xs ml-auto">
              {{ j.hour > 0 ? `近 ${j.hour}h` : '全量' }} · 用时 {{ fmtDuration(j.elapsedMs) }}
            </span>
          </header>

          <div class="jc-job__bar">
            <div
              class="jc-job__bar-fill"
              :class="{ 'jc-job__bar-fill--paused': j.state === 'paused' }"
              :style="{ width: fmtPercent(j) + '%' }"
            />
            <span class="jc-job__bar-text">
              已完成 {{ j.donePages }} 页 / 共 {{ j.totalPages || '?' }} 页
              <template v-if="j.failedPages > 0">
                · <span class="text-danger">失败 {{ j.failedPages }} 页</span>
              </template>
              · 进度 {{ fmtPercent(j) }}%
            </span>
          </div>

          <!-- 失败原因: 红色高亮 + 展开提示 -->
          <div v-if="j.state === 'error' && j.error" class="jc-job__error">
            <BaseIcon name="info" size="14" />
            <span class="flex-1 break-all">{{ j.error }}</span>
          </div>

          <div class="jc-job__actions">
            <BaseButton
              v-if="j.state === 'running'"
              variant="outline"
              size="sm"
              :disabled="!!busyId"
              @click="pause(j.sourceId)"
            >暂停</BaseButton>
            <BaseButton
              v-if="j.state === 'paused'"
              variant="primary"
              size="sm"
              :disabled="!!busyId"
              @click="resume(j.sourceId)"
            >继续</BaseButton>
            <BaseButton
              v-if="j.state === 'running' || j.state === 'paused'"
              variant="danger"
              size="sm"
              :disabled="!!busyId"
              @click="cancelJob(j)"
            >取消</BaseButton>
            <BaseButton
              v-if="j.state === 'error' || j.state === 'canceled'"
              variant="primary"
              size="sm"
              :disabled="!!busyId"
              @click="rerunJob(j)"
            >重跑</BaseButton>
          </div>
        </article>
      </div>
    </template>

    <!-- ===== 历史记录 ===== -->
    <template v-else-if="activeTab === 'history'">
      <ManageTable :columns="historyColumns" :rows="runRows" row-key="id" :loading="runsLoading" :empty="'暂无任务记录'">
        <template #toolbar>
          <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
            <label class="text-sm text-muted flex items-center gap-[var(--jc-space-1)]">
              状态
              <select v-model="runStatus" class="jc-select" @change="changeRunStatus">
                <option value="">全部</option>
                <option value="running">运行中</option>
                <option value="success">成功</option>
                <option value="failed">失败</option>
                <option value="canceled">已取消</option>
              </select>
            </label>
            <label class="text-sm text-muted flex items-center gap-[var(--jc-space-1)]">
              类型
              <select v-model="runType" class="jc-select" @change="changeRunStatus">
                <option value="">全部</option>
                <option value="collect">采集</option>
                <option value="recover">补采</option>
                <option value="category_cover">分类覆盖</option>
                <option value="hot_refresh">榜单刷新</option>
              </select>
            </label>
          </div>
        </template>

        <template #cell="{ row, col }">
          <template v-if="col.key === 'startedAt'">
            <span class="text-xs text-muted whitespace-nowrap">{{ fmtTime(row.startedAt) }}</span>
          </template>
          <template v-else-if="col.key === 'name'">
            <span class="text-sm font-[var(--jc-fw-medium)]">{{ row.name }}</span>
            <span v-if="row.sourceId && row.sourceId !== row.name" class="text-xs text-muted ml-[var(--jc-space-1)]">
              ({{ sourceNames[row.sourceId] || row.sourceId }})
            </span>
          </template>
          <template v-else-if="col.key === 'kind'">
            <BaseTag :variant="row.kind === 'cron' ? 'default' : 'brand'" size="sm">
              {{ runKindText(row.kind) }}
            </BaseTag>
          </template>
          <template v-else-if="col.key === 'type'">
            <span class="text-xs text-muted">{{ runTypeText(row.type) }}</span>
          </template>
          <template v-else-if="col.key === 'status'">
            <BaseTag :variant="runStatusVariant(row.status)" size="sm">{{ runStatusText(row.status) }}</BaseTag>
          </template>
          <template v-else-if="col.key === 'message'">
            <span class="text-xs text-muted break-all" :title="row.message">{{ row.message || '—' }}</span>
          </template>
          <template v-else-if="col.key === 'error'">
            <span v-if="row.error" class="text-xs text-danger break-all" :title="row.error">{{ row.error }}</span>
            <span v-else class="text-muted text-xs">—</span>
          </template>
          <template v-else-if="col.key === 'elapsedMs'">
            <span class="text-xs text-muted">{{ fmtDuration(row.elapsedMs) }}</span>
          </template>
          <template v-else>
            {{ (row as unknown as Record<string, unknown>)[col.key] ?? '—' }}
          </template>
        </template>

        <template #actions="{ row }">
          <div class="flex gap-[var(--jc-space-1)] justify-end">
            <BaseButton
              v-if="row.status === 'failed'"
              variant="primary"
              size="sm"
              :disabled="rerunBusyId === row.id"
              @click="rerunRun(row)"
            >
              {{ rerunBusyId === row.id ? '提交中…' : '重跑' }}
            </BaseButton>
            <span v-else-if="row.status === 'running'" class="text-muted text-xs flex items-center">执行中…</span>
            <span v-else class="text-muted text-xs">—</span>
          </div>
        </template>
      </ManageTable>

      <div class="flex justify-end mt-[var(--jc-space-3)]">
        <BasePagination
          :total="runTotal"
          :current="runPage"
          :page-size="RUN_SIZE"
          @change="changeRunPage"
        />
      </div>
    </template>

    <!-- ===== 失败记录 ===== -->
    <template v-else>
      <ManageTable :columns="failureColumns" :rows="failures" row-key="id" :empty="'暂无待补采的失败页'">
        <template #toolbar>
          <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
            <span class="text-sm text-muted">
              待补采 <span class="font-[var(--jc-fw-semibold)] text-primary">{{ ov.pendingFailures }}</span> 页
              · 共 {{ failTotal }} 条
            </span>
            <BaseButton
              variant="primary"
              size="sm"
              :disabled="!canRecoverAll || failBusy"
              @click="recover()"
            >{{ failBusy ? '提交中…' : '补采全部' }}</BaseButton>
            <RouterLink to="/manage/collect/failures" class="text-link text-sm">查看完整补采中心 →</RouterLink>
          </div>
        </template>

        <template #cell="{ row, col }">
          <template v-if="col.key === 'createdAt'">
            <span class="text-xs text-muted whitespace-nowrap">{{ fmtTime(row.createdAt) }}</span>
          </template>
          <template v-else-if="col.key === 'sourceId'">
            <span class="text-xs">{{ sourceNames[row.sourceId] || row.sourceId }}</span>
          </template>
          <template v-else-if="col.key === 'cause'">
            <span class="text-xs text-danger break-all" :title="row.cause">{{ row.cause }}</span>
          </template>
          <template v-else-if="col.key === 'status'">
            <BaseTag variant="warning" size="sm">待补采</BaseTag>
          </template>
          <template v-else>
            {{ (row as unknown as Record<string, unknown>)[col.key] ?? '—' }}
          </template>
        </template>

        <template #actions="{ row }">
          <div class="flex justify-end">
            <BaseButton
              variant="outline"
              size="sm"
              :disabled="failBusy"
              @click="recover([row.id])"
            >补采</BaseButton>
          </div>
        </template>
      </ManageTable>

      <div class="flex justify-end mt-[var(--jc-space-3)]">
        <BasePagination
          :total="failTotal"
          :current="failPage"
          :page-size="FAIL_SIZE"
          @change="changeFailPage"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.stat-card {
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
  padding: var(--jc-space-4);
}
.stat-card__value {
  font-size: 28px;
  font-weight: var(--jc-fw-semibold);
  line-height: 1.2;
}
.stat-card__label {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  margin-top: var(--jc-space-1);
}
.stat-card--danger .stat-card__value {
  color: var(--jc-color-danger, #ef4444);
}

.jc-job {
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
  padding: var(--jc-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-3);
}
.jc-job__head {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  flex-wrap: wrap;
}
.jc-job__name {
  font-weight: var(--jc-fw-semibold);
  font-size: var(--jc-fs-md);
}
.jc-job__bar {
  position: relative;
  height: 22px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: var(--jc-radius-sm);
  overflow: hidden;
}
.jc-job__bar-fill {
  position: absolute;
  inset: 0 auto 0 0;
  background: linear-gradient(90deg, var(--jc-brand-purple), var(--jc-brand-cyan));
  transition: width var(--jc-dur-base) var(--jc-ease-standard);
}
.jc-job__bar-fill--paused {
  background: linear-gradient(90deg, #d97706, #f59e0b);
  opacity: 0.7;
}
.jc-job__bar-text {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-primary);
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.7);
}
.jc-job__error {
  display: flex;
  align-items: flex-start;
  gap: var(--jc-space-2);
  font-size: var(--jc-fs-xs);
  color: var(--jc-color-danger, #ef4444);
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--jc-radius-sm);
  padding: var(--jc-space-2) var(--jc-space-3);
}
.jc-job__actions {
  display: flex;
  gap: var(--jc-space-2);
  align-items: center;
}
.jc-tm-empty {
  padding: var(--jc-space-12) var(--jc-space-4);
  text-align: center;
  color: var(--jc-text-muted);
  background: var(--jc-bg-elevated);
  border: 1px dashed var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
}
.jc-select {
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-sm);
  color: var(--jc-text-primary);
  font-size: var(--jc-fs-sm);
  padding: var(--jc-space-1) var(--jc-space-2);
}
</style>
