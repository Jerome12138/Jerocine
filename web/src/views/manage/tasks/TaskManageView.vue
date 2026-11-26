<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as collectApi from '@/api/manage/collect'
import * as tasksApi from '@/api/manage/tasks'
import type { TaskCenterItem } from '@/api/manage/tasks'
import type { SpiderJob } from '@/api/manage/collect'
import type { CollectFailure, TaskOverview, TaskRun } from '@/types/manage'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import ManageTable, { type Column } from '@/components/manage/ManageTable.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import type { TabItem } from '@/components/manage/ManageTabs.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import { usePagedList } from '@/composables/usePagedList'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { toast } from '@/api/http'
import { openManageDialog } from '@/composables/useManageDialog'
import TaskSettingDialog from './TaskSettingDialog.vue'
import { manageApi } from '@/api'
import { formatDateTime } from '@/utils/format'

/**
 * 任务管理: 统一展示全部定时/手动任务。
 * - 统计卡: 运行中 / 今日成功 / 今日失败 / 待补采
 * - 运行中: 采集 job 实时进度(Redis), 失败显示原因, 支持暂停/继续/取消/重跑
 * - 历史记录: 任务运行台账(task_run), 失败原因可查, 失败任务一键重跑
 *   (重跑是后台受理: 榜单刷新实测 1-2 分钟, 服务端登记台账后即返回 202, 结果看历史)
 * - 失败记录: 页级失败台账(待补采, 分页) + 补采全部/单条
 *
 * 提示口径: 写操作经 useAsyncAction 单点提示 + { silent: true }(拦截器的通用提示关闭,
 * 页面差异化文案不与拦截器叠两条); 后台轮询用的读接口也 silent, 失败只降级页内状态不刷 toast。
 */
type TabKey = 'running' | 'schedule' | 'history' | 'failures'

const activeTab = ref<TabKey>('running')

// 状态词汇表 → StatusTag(替代 jobStateVariant/Text 等自写映射)
const JOB_STATE_MAP: StatusMap = {
  running: { label: '采集中', variant: 'brand' },
  paused: { label: '已暂停', variant: 'warning' },
  done: { label: '已完成', variant: 'success' },
  error: { label: '失败', variant: 'danger' },
  canceled: { label: '已取消', variant: 'danger' }
}
/** 计划任务启停状态 */
const CRON_STATE_MAP: StatusMap = {
  on: { label: '启用', variant: 'success' },
  off: { label: '停用', variant: 'default' }
}

const RUN_STATUS_MAP: StatusMap = {
  running: { label: '运行中', variant: 'brand' },
  success: { label: '成功', variant: 'success' },
  failed: { label: '失败', variant: 'danger' },
  canceled: { label: '已取消', variant: 'warning' }
}

// ---- 统计卡 ----
const ov = ref<TaskOverview>({ running: 0, todaySuccess: 0, todayFailed: 0, pendingFailures: 0 })

// ---- 运行中 job ----
const jobs = ref<SpiderJob[]>([])
const jobsLoading = ref(false)
/** 统一任务中心：内置 + 采集，一份列表 */
const tcList = ref<TaskCenterItem[]>([])
const tcLoading = ref(false)
const tcBusy = ref<Record<string, boolean>>({})
const collapsed = ref<Record<string, boolean>>(loadCollapsed())

const TC_STATE_MAP: StatusMap = {
  running: { label: '运行中', variant: 'brand' },
  paused: { label: '已暂停', variant: 'warning' },
  succeeded: { label: '已完成', variant: 'success' },
  failed: { label: '失败', variant: 'danger' },
  canceled: { label: '已取消', variant: 'warning' },
  idle: { label: '空闲', variant: 'default' }
}

const COLLAPSE_KEY = 'jc.taskcenter.collapsed'

function loadCollapsed(): Record<string, boolean> {
  try {
    return JSON.parse(localStorage.getItem(COLLAPSE_KEY) ?? '{}') as Record<string, boolean>
  } catch {
    return {}
  }
}

function toggleGroup(key: string): void {
  collapsed.value = { ...collapsed.value, [key]: !collapsed.value[key] }
  localStorage.setItem(COLLAPSE_KEY, JSON.stringify(collapsed.value))
}

/** 分组：内置类 / 采集类（空组不显示） */
const tcGroups = computed(() => {
  const groups = [
    { key: 'builtin', label: '内置任务', items: tcList.value.filter((t) => t.kind === 'builtin') },
    { key: 'collect', label: '采集定时任务', items: tcList.value.filter((t) => t.kind !== 'builtin') }
  ]
  return groups.filter((g) => g.items.length > 0)
})

async function loadTc(): Promise<void> {
  tcLoading.value = true
  try {
    const r = await tasksApi.taskCenterTasks()
    tcList.value = r.list ?? []
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '任务列表加载失败')
  } finally {
    tcLoading.value = false
  }
}

async function runTask(t: TaskCenterItem): Promise<void> {
  tcBusy.value = { ...tcBusy.value, [t.key]: true }
  try {
    await tasksApi.taskCenterRun(t.key)
    toast('success', `已提交执行：${t.name}`)
    setTimeout(() => void loadTc(), 1500)
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '触发失败')
  } finally {
    tcBusy.value = { ...tcBusy.value, [t.key]: false }
  }
}

async function editTask(t: TaskCenterItem): Promise<void> {
  const done = await openManageDialog<boolean>({
    title: `编辑任务 · ${t.name}`,
    component: TaskSettingDialog,
    props: { task: t },
    width: '560px'
  })
  if (done) await loadTc()
}

const cronRunning = ref<Record<string, boolean>>({})

const jobsError = ref('') // 列表加载失败: 页内错误态+重试, 不伪装成"暂无任务"

// ---- 历史台账(usePagedList: 筛选 runStatus/runType 闭包捕获, elapsedMs 派生在 fetch 映射) ----
const runStatus = ref('')
const runType = ref('')
const RUN_SIZE = 20

interface RunRow extends TaskRun {
  elapsedMs: number
}

const {
  rows: runRows,
  total: runTotal,
  current: runPage,
  pageSize: runPageSize,
  loading: runsLoading,
  search: searchRuns,
  load: loadRuns
} = usePagedList<RunRow>({
  fetch: async (page, size) => {
    const resp = await tasksApi.runs({
      status: runStatus.value || undefined,
      type: runType.value || undefined,
      page,
      size
    })
    return {
      list: (resp?.list ?? []).map((r) => ({ ...r, elapsedMs: Math.max(0, r.endedAt - r.startedAt) })),
      total: resp?.page?.total ?? 0
    }
  },
  immediate: false
})

// ---- 失败台账(待补采, usePagedList; 本页只看待补采 status=0) ----
const FAIL_SIZE = 20 // 与补采中心页(/manage/collect/failures)对齐
const {
  rows: failures,
  total: failTotal,
  current: failPage,
  pageSize: failPageSize,
  load: loadFailures
} = usePagedList<CollectFailure>({
  fetch: async (page, size) => {
    const resp = await collectApi.failures({ status: 0, page, size }, { silent: true })
    return { list: resp?.list ?? [], total: resp?.page?.total ?? 0 }
  },
  pageSize: FAIL_SIZE,
  immediate: false
})

const sourceNames = ref<Record<string, string>>({})

const POLL_ACTIVE_MS = 5000
const POLL_IDLE_MS = 30_000
let timer: number | undefined

const hasActive = (): boolean =>
  jobs.value.some((j) => j.state === 'running' || j.state === 'paused') ||
  runRows.value.some((r) => r.status === 'running')

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
  if (activeTab.value === 'schedule') void loadTc()
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

// ---- 写操作: useAsyncAction 单点提示(busy 防连点, silent 关拦截器提示) ----
const { busy, run } = useAsyncAction()

async function pause(id: string): Promise<void> {
  await run({
    action: () => collectApi.spiderJobPause(id, { silent: true }),
    successMsg: '已暂停',
    after: loadJobs
  })
}

async function resume(id: string): Promise<void> {
  await run({
    action: () => collectApi.spiderJobResume(id, { silent: true }),
    successMsg: '已继续',
    after: loadJobs
  })
}

async function cancelJob(j: SpiderJob): Promise<void> {
  await run({
    confirm: {
      title: '取消采集任务',
      desc: `确认取消 [${j.sourceName}] 的采集? 已完成的页面会保留, 未完成的页面会丢弃.`
    },
    action: () => collectApi.spiderJobCancel(j.sourceId, { silent: true }),
    successMsg: '已取消',
    after: loadJobs
  })
}

/** 重跑一个已失败的采集 job: 按原时长(hour)重新触发同源采集 */
async function rerunJob(j: SpiderJob): Promise<void> {
  await run({
    confirm: {
      title: '重跑采集任务',
      desc: `重新采集 [${j.sourceName}]? 时长 ${j.hour > 0 ? `近 ${j.hour}h` : '全量'}.`
    },
    action: () =>
      collectApi.startSpider(
        { id: j.sourceId, ids: [], time: j.hour > 0 ? j.hour : -1, batch: false },
        { silent: true }
      ),
    successMsg: '已重新触发',
    after: loadJobs
  })
}

// ---- 历史台账操作 ----
// 行级防连点: 重跑请求在途时禁用该行按钮。服务端是后台受理(202 立即返回),
// 等待窗口虽短, 双击仍会登记两条台账(榜单刷新第二条撞 busy 记 canceled 行)。
const rerunBusyId = ref(0)

async function rerunRun(r: TaskRun): Promise<void> {
  rerunBusyId.value = r.id
  await run({
    confirm: {
      title: '重跑任务',
      desc: `重新执行 [${r.name}]? 会登记一条新的任务记录.`
    },
    action: () => tasksApi.rerun(r.id, { silent: true }),
    // 后台受理: 服务端登记台账后即返回, 榜单刷新要跑 1-2 分钟, 不在这里等结果
    successMsg: '已提交, 结果见历史记录',
    after: () => {
      runPage.value = 1
      return Promise.all([loadRuns(), loadOverview()]).then(() => undefined)
    }
  })
  rerunBusyId.value = 0
}

// 筛选变更 → 回第 1 页重查(search 在第 1 页时直接 load)
function changeRunStatus(): void {
  searchRuns()
}

// ---- 失败记录操作 ----
async function recover(ids?: number[]): Promise<void> {
  await run({
    action: async () => {
      const r = await collectApi.recoverFailures(ids, { silent: true })
      // 动态成功文案(带服务端回填的 pending 数) → action 内自管, run 不再传静态 successMsg
      toast('success', `已提交补采 (待处理 ${r.pending} 条)`)
      void loadFailures()
      void loadOverview()
    }
  })
}

// ---- 展示格式化 ----
function fmtTime(ms: number): string {
  return ms ? formatDateTime(ms) : '—'
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

/**
 * 进度条配色：
 *  - 运行中 = 品牌渐变（与旧版一致）
 *  - 已完成 = 成功绿（之前误用了不存在的 --jc-accent，渲染出来是灰的）
 *  - 暂停 = 琥珀 / 失败 = 红
 */
function barClass(j: SpiderJob): string {
  if (j.state === 'paused') return 'bg-[var(--jc-warning)]'
  if (j.state === 'error') return 'bg-[var(--jc-danger)]'
  if (j.state === 'done') return 'bg-[var(--jc-success)]'
  return 'bg-[linear-gradient(90deg,var(--jc-brand-purple),var(--jc-brand-cyan))]'
}

/** 进度文案：总数未知时不给假进度（"共 ? 页 · 0%" 是上一版的观感问题） */
function progressText(j: SpiderJob): string {
  if (j.totalPages <= 0) {
    if (j.state === 'running') return '准备中…'
    if (j.state === 'done') return '已完成（无分页数据）'
    if (j.state === 'error') return '失败'
    return '—'
  }
  return `已完成 ${j.donePages} / 共 ${j.totalPages} 页 · ${fmtPercent(j)}%`
}

function fmtPercent(j: SpiderJob): number {
  if (j.totalPages <= 0) return 0
  return Math.min(100, Math.round((j.donePages / j.totalPages) * 100))
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

// 历史台账筛选下拉(value 保字符串类型)
const RUN_STATUS_OPTIONS = [
  { label: '全部', value: '' },
  { label: '运行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' }
]
const RUN_TYPE_OPTIONS = [
  { label: '全部', value: '' },
  { label: '采集', value: 'collect' },
  { label: '补采', value: 'recover' },
  { label: '分类覆盖', value: 'category_cover' },
  { label: '榜单刷新', value: 'hot_refresh' }
]

// 下划线页签(ManageTabs): count 徽标替代旧「运行中 (N)」内联文案
const tabs = computed<TabItem<TabKey>[]>(() => [
  {
    value: 'running',
    label: '近期任务',
    count: jobs.value.filter((j) => j.state === 'running' || j.state === 'paused').length
  },
  { value: 'schedule', label: '计划任务', count: tcList.value.length },
  { value: 'history', label: '历史记录', count: runTotal.value },
  { value: 'failures', label: '失败记录' }
])
</script>

<template>
  <div class="container-page flex flex-1 min-h-0 flex-col py-[var(--jc-space-5)]">
    <header class="flex items-center justify-between mb-[var(--jc-space-5)]">
      <div>
        <h1 class="text-2xl font-[var(--jc-fw-semibold)]">任务中心</h1>
        <p class="text-muted text-sm mt-[var(--jc-space-1)]">
          内置任务与采集任务的统一入口：计划、状态、手动执行与失败重跑
        </p>
      </div>
      <div class="flex items-center gap-[var(--jc-space-3)]">
        <span class="text-muted text-sm" v-if="jobsLoading || runsLoading">刷新中…</span>
        <ManageButton variant="outline" @click="loadAll">手动刷新</ManageButton>
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

    <!-- 页签(ManageTabs underline) -->
    <ManageTabs
      class="mb-[var(--jc-space-4)]"
      :model-value="activeTab"
      :tabs="tabs"
      variant="underline"
      @update:model-value="switchTab"
    />

    <!-- ===== 运行中 ===== -->
    <template v-if="activeTab === 'running'">
      <!-- 加载中: 占位, 不与空态混同 -->
      <div v-if="jobsLoading && jobs.length === 0" class="flex flex-col gap-[var(--jc-space-3)]">
        <BaseSkeleton v-for="i in 3" :key="i" shape="rect" height="72px" />
      </div>

      <!-- 首次加载失败: 明确报错 + 重试, 不落入"暂无任务"空态误导 -->
      <div v-else-if="jobsError && jobs.length === 0" class="jc-tm-empty">
        加载失败: {{ jobsError }}.
        <div class="mt-[var(--jc-space-3)]">
          <ManageButton variant="outline" @click="loadJobs">点击重试</ManageButton>
        </div>
      </div>

      <div v-else-if="jobs.length === 0" class="jc-tm-empty">
        暂无正在运行 / 最近 30 分钟内结束的采集任务. 去
        <RouterLink to="/manage/collect" class="text-link">采集源列表</RouterLink>
        启动一个.
      </div>

      <div v-else class="flex flex-col gap-[var(--jc-space-4)]">
        <article
          v-for="j in jobs"
          :key="j.sourceId"
          class="flex flex-col gap-[var(--jc-space-2)] rounded-card bg-surface p-[var(--jc-space-4)] shadow-card"
        >
          <!-- 一行放完：状态 / 名称 / 目标 / 启动 / 耗时 / 进度 / 操作 -->
          <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
            <StatusTag :value="j.state" :map="JOB_STATE_MAP" size="sm" />
            <span class="text-sm font-[var(--jc-fw-medium)]">{{ j.sourceName }}</span>
            <BaseTag variant="info" size="xs">采集</BaseTag>
            <span class="text-xs text-muted">目标 {{ j.hour > 0 ? `近 ${j.hour} 小时` : '全量' }}</span>
            <span class="text-xs text-muted">启动 {{ j.startedAt ? formatDateTime(j.startedAt * 1000) : '—' }}</span>
            <span class="text-xs text-muted">耗时 {{ j.elapsedMs > 0 ? fmtDuration(j.elapsedMs) : '—' }}</span>

            <span class="ml-auto text-xs tabular-nums" :class="j.totalPages > 0 ? 'text-secondary' : 'text-muted'">
              {{ progressText(j) }}
            </span>

            <div class="flex gap-[var(--jc-space-1)]">
              <ManageButton
                v-if="j.state === 'running'"
                variant="outline"
                size="sm"
                :disabled="busy"
                @click="pause(j.sourceId)"
              >暂停</ManageButton>
              <ManageButton
                v-if="j.state === 'paused'"
                variant="primary"
                size="sm"
                :disabled="busy"
                @click="resume(j.sourceId)"
              >继续</ManageButton>
              <ManageButton
                v-if="j.state === 'running' || j.state === 'paused'"
                variant="danger"
                size="sm"
                :disabled="busy"
                @click="cancelJob(j)"
              >取消</ManageButton>
              <ManageButton
                v-if="j.state === 'error' || j.state === 'canceled'"
                variant="primary"
                size="sm"
                :disabled="busy"
                @click="rerunJob(j)"
              >重跑</ManageButton>
            </div>
          </div>

          <!-- 细进度条：只有知道总页数时才画，避免"共 ? 页 · 0%"这种假进度 -->
          <div
            v-if="j.totalPages > 0"
            class="h-1 w-full overflow-hidden rounded-full bg-[var(--jc-border-default)]"
          >
            <div
              class="h-full rounded-full transition-all"
              :class="barClass(j)"
              :style="{ width: fmtPercent(j) + '%' }"
            />
          </div>

          <div class="flex flex-wrap items-center gap-[var(--jc-space-3)] text-xs">
            <span v-if="j.failedPages > 0" class="text-[var(--jc-danger)]">失败 {{ j.failedPages }} 页</span>
            <span
              v-if="j.state === 'error' && j.error"
              class="flex min-w-0 flex-1 items-start gap-1 text-[var(--jc-danger)]"
            >
              <BaseIcon name="info" size="14" />
              <span class="break-all">{{ j.error }}</span>
            </span>
          </div>
        </article>
      </div>
    </template>

    <!-- ===== 计划任务（统一任务中心：内置 + 采集）===== -->
    <template v-if="activeTab === 'schedule'">
      <div v-if="tcLoading && tcList.length === 0" class="flex flex-col gap-[var(--jc-space-3)]">
        <BaseSkeleton v-for="i in 3" :key="i" shape="rect" height="72px" />
      </div>

      <div v-else class="flex flex-col gap-[var(--jc-space-4)]">
        <section v-for="g in tcGroups" :key="g.key" class="flex flex-col gap-[var(--jc-space-2)]">
          <button
            type="button"
            class="jc-hotarea flex items-center gap-[var(--jc-space-2)] text-left text-sm font-[var(--jc-fw-medium)]"
            @click="toggleGroup(g.key)"
          >
            <span class="text-xs text-muted">{{ collapsed[g.key] ? '▸' : '▾' }}</span>
            <span>{{ g.label }}</span>
            <span class="text-xs text-muted">{{ g.items.length }} 项</span>
          </button>

          <div v-show="!collapsed[g.key]" class="flex flex-col gap-[var(--jc-space-2)]">
            <article
              v-for="t in g.items"
              :key="t.key"
              class="flex flex-wrap items-center gap-[var(--jc-space-2)] rounded-card bg-surface p-[var(--jc-space-3)] shadow-card"
            >
              <StatusTag :value="t.enabled ? 'on' : 'off'" :map="CRON_STATE_MAP" size="sm" />
              <BaseTag :variant="t.kind === 'builtin' ? 'purple' : 'info'" size="xs">
                {{ t.kind === 'builtin' ? '内置' : '采集' }}
              </BaseTag>
              <span class="text-sm font-[var(--jc-fw-medium)]">{{ t.name }}</span>
              <span class="text-xs text-muted">计划 {{ t.scheduleText }}</span>
              <span v-if="t.target" class="text-xs text-muted">目标 {{ t.target }}</span>
              <span v-if="t.intervalLabel" class="text-xs text-muted">间隔 {{ t.intervalLabel }}</span>

              <StatusTag v-if="t.state !== 'idle'" :value="t.state" :map="TC_STATE_MAP" size="xs" />

              <span class="ml-auto text-xs text-muted">
                {{ t.lastRunAt || t.startedAt ? formatDateTime(t.lastRunAt || t.startedAt) : '未运行' }}
              </span>
              <span v-if="t.message" class="text-xs text-muted" :title="t.message">{{ t.message }}</span>

              <ManageButton
                size="sm"
                variant="ghost"
                :loading="!!tcBusy[t.key]"
                @click="runTask(t)"
              >立即执行</ManageButton>
              <ManageButton
                v-if="t.editable && t.editable.length"
                size="sm"
                variant="ghost"
                @click="editTask(t)"
              >编辑</ManageButton>
            </article>
          </div>
        </section>

        <p class="text-xs text-muted">
          计划任务到点自动执行；这里的「立即执行」等同手动触发一次（不影响原计划）。
          暂停/继续/停止在「近期任务」里对正在运行的采集任务操作。
        </p>
      </div>
    </template>

    <!-- ===== 历史记录 ===== -->
    <template v-else-if="activeTab === 'history'">
      <ManageTable
        :columns="historyColumns"
        :rows="runRows"
        row-key="id"
        :loading="runsLoading"
        :total="runTotal"
        v-model:current="runPage"
        :page-size="runPageSize"
        :empty="'暂无任务记录'"
      >
        <template #toolbar>
          <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
            <label class="text-sm text-muted flex items-center gap-[var(--jc-space-1)]">
              状态
              <ManageSelect
                v-model="runStatus"
                :options="RUN_STATUS_OPTIONS"
                aria-label="状态"
                class="w-[120px]"
                @update:model-value="changeRunStatus"
              />
            </label>
            <label class="text-sm text-muted flex items-center gap-[var(--jc-space-1)]">
              类型
              <ManageSelect
                v-model="runType"
                :options="RUN_TYPE_OPTIONS"
                aria-label="类型"
                class="w-[120px]"
                @update:model-value="changeRunStatus"
              />
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
            <StatusTag :value="row.status" :map="RUN_STATUS_MAP" />
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
        </template>

        <template #actions="{ row }">
          <div class="flex gap-[var(--jc-space-1)] justify-end">
            <ManageButton
              v-if="row.status === 'failed'"
              variant="primary"
              size="sm"
              :disabled="rerunBusyId === row.id || busy"
              @click="rerunRun(row)"
            >
              {{ rerunBusyId === row.id ? '提交中…' : '重跑' }}
            </ManageButton>
            <span v-else-if="row.status === 'running'" class="text-muted text-xs flex items-center">执行中…</span>
            <span v-else class="text-muted text-xs">—</span>
          </div>
        </template>
      </ManageTable>
    </template>

    <!-- ===== 失败记录 ===== -->
    <template v-else>
      <ManageTable
        :columns="failureColumns"
        :rows="failures"
        row-key="id"
        :total="failTotal"
        v-model:current="failPage"
        :page-size="failPageSize"
        :empty="'暂无待补采的失败页'"
      >
        <template #toolbar>
          <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
            <span class="text-sm text-muted">
              待补采 <span class="font-[var(--jc-fw-semibold)] text-primary">{{ ov.pendingFailures }}</span> 页
              · 共 {{ failTotal }} 条
            </span>
            <ManageButton
              variant="primary"
              :disabled="!canRecoverAll || busy"
              @click="recover()"
            >{{ busy ? '提交中…' : '补采全部' }}</ManageButton>
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
            {{ row[col.key] ?? '—' }}
          </template>
        </template>

        <template #actions="{ row }">
          <div class="flex justify-end">
            <ManageButton
              variant="outline"
              size="sm"
              :disabled="busy"
              @click="recover([row.id])"
            >补采</ManageButton>
          </div>
        </template>
      </ManageTable>
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
</style>
