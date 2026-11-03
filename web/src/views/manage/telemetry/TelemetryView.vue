<script setup lang="ts">
/**
 * 埋点监控 / 异常诊断 看板
 *
 * 数据源:
 *   GET /manage/telemetry/overview        概览
 *   GET /manage/telemetry/events          错误列表 (type=error)
 *   GET /manage/telemetry/hot-films       热点视频 (按访问量)
 *   GET /manage/telemetry/most-favorited  收藏最多
 *   GET /manage/telemetry/api-perf        API 性能 (type=api, P50/95/99)
 *
 * 操作:
 *   - 时间窗口切换 (1d / 7d / 30d) → 重拉所有指标
 *   - 刷新按钮
 *   - 错误行可展开 stack / extra
 */
import { onMounted, ref, watch } from 'vue'
import * as telemetryApi from '@/api/manage/telemetry'
import type {
  OverviewResp,
  TelemetryEventRow,
  FilmStat,
  ApiPerfItem,
  IssueStatusFilter
} from '@/api/manage/telemetry'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import { openManageDialog } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { formatDateTime } from '@/utils/format'
import TelemetryResolveDialog from './TelemetryResolveDialog.vue'

const PAGE_SIZE = 100
const days = ref<number>(7)
const overview = ref<OverviewResp | null>(null)
const errors = ref<TelemetryEventRow[]>([])
/** 服务端状态过滤后的总数(供计数徽标与「加载更多」), 与已加载行数解耦 */
const total = ref(0)
const hotFilms = ref<FilmStat[]>([])
const mostFavorited = ref<FilmStat[]>([])
const apiPerf = ref<ApiPerfItem[]>([])
const loading = ref(false)
const error = ref('')
const expanded = ref<Record<number, boolean>>({})

const DAYS_OPTIONS = [
  { value: 1, label: '近 1 天' },
  { value: 7, label: '近 7 天' },
  { value: 30, label: '近 30 天' }
]

/** issueStatus → StatusTag 词汇表(状态值→文案/variant 统一走 StatusTag) */
const ISSUE_STATUS_MAP: StatusMap = {
  unresolved: { label: '未处理', variant: 'danger' },
  reopened: { label: '已重启', variant: 'warning' },
  resolved: { label: '已解决', variant: 'success' },
  expired: { label: '已过期', variant: 'default' }
}

/** 请求序号: 快速切换时间窗/筛选时只采纳最后一次请求的结果, 旧慢响应丢弃 */
let reqSeq = 0

async function loadAll(append = false): Promise<void> {
  const seq = ++reqSeq
  if (!append) error.value = ''
  loading.value = true
  const offset = append ? errors.value.length : 0
  try {
    const [ov, errs, hf, mf, ap] = await Promise.all([
      telemetryApi.overview(days.value),
      telemetryApi.listEvents({ type: 'error', days: days.value, limit: PAGE_SIZE, offset, status: statusFilter.value }),
      telemetryApi.hotFilms(days.value, 10),
      telemetryApi.mostFavorited(10),
      telemetryApi.apiPerf(days.value, 10)
    ])
    if (seq !== reqSeq) return // 已被更新的请求取代
    overview.value = ov
    const rows = errs.rows ?? []
    errors.value = append ? [...errors.value, ...rows] : rows
    total.value = errs.total ?? errors.value.length
    hotFilms.value = hf ?? []
    mostFavorited.value = mf ?? []
    apiPerf.value = ap ?? []
  } catch (e) {
    if (seq !== reqSeq) return
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

function fmtMs(v: number): string {
  if (v >= 1000) return (v / 1000).toFixed(2) + 's'
  return Math.round(v) + 'ms'
}

/** 1234 → "1.2k", < 1000 原样 */
function fmtCount(v: number): string {
  if (v >= 1000) return (v / 1000).toFixed(1).replace(/\.0$/, '') + 'k'
  return String(v)
}

function toggleExpand(id: number): void {
  expanded.value[id] = !expanded.value[id]
}

/** extra 是 JSON 字符串. 解析成 (key, prettyValue) 数组, stack 这种含 \n 的渲成多行. */
function parsedExtraEntries(raw: string): Array<[string, string]> {
  if (!raw) return []
  try {
    const obj = JSON.parse(raw) as Record<string, unknown>
    return Object.entries(obj).map(([k, v]) => {
      let s: string
      if (typeof v === 'string') {
        s = v
      } else {
        try {
          s = JSON.stringify(v, null, 2)
        } catch {
          s = String(v)
        }
      }
      return [k, s] as [string, string]
    })
  } catch {
    // 不是合法 JSON 就当一整行 raw 展示
    return [['raw', raw]]
  }
}

/** ============ 问题闭环 ============ */
const statusFilter = ref<IssueStatusFilter>('active')
const STATUS_OPTIONS: Array<{ value: IssueStatusFilter; label: string }> = [
  { value: 'active', label: '活跃 (未解决 + 重启)' },
  { value: 'reopened', label: '已重启 (解决后 24h 仍出现)' },
  { value: 'resolved', label: '已解决 (≤30天)' },
  { value: 'expired', label: '过期 (>30天)' },
  { value: 'all', label: '全部' }
]

// 标记解决: 函数化弹窗(表单含独立校验/提交 → openManageDialog, 内容组件自治), 成功后重拉列表
async function openResolve(row: TelemetryEventRow): Promise<void> {
  const done = await openManageDialog<boolean>({
    title: '标记问题已解决',
    component: TelemetryResolveDialog,
    props: { row },
    mobileMode: 'sheet'
  })
  if (!done) return
  void loadAll()
}

// 撤销解决: confirm + silent 请求 + 单点提示统一走 useAsyncAction
const { busy, run } = useAsyncAction()
const undoingId = ref<number | null>(null)

async function undoResolve(row: TelemetryEventRow): Promise<void> {
  undoingId.value = row.id
  try {
    await run({
      confirm: {
        title: '撤销已解决',
        desc: '撤销后此问题重回未解决列表, 继续累计计数.'
      },
      action: () =>
        telemetryApi.unresolveIssue(
          { category: row.category, path: row.path, label: row.label },
          { silent: true }
        ),
      successMsg: '已撤销，问题重回未解决列表',
      after: () => loadAll()
    })
  } finally {
    undoingId.value = null
  }
}

/** 未知影片兜底: 影片已从 search 索引清除时服务端只有 mid+count, 避免渲染空白行 */
function filmName(f: FilmStat): string {
  return f.name || `未知影片 (mid:${f.mid})`
}

onMounted(() => void loadAll())
// 注意不能把 loadAll 直接作为 watch 回调: 新值会被当成 append 参数
watch(days, () => void loadAll())
watch(statusFilter, () => void loadAll())
</script>

<template>
  <div class="container-page py-[var(--jc-space-5)]">
    <header class="flex items-center justify-between mb-[var(--jc-space-5)]">
      <h1 class="text-2xl font-[var(--jc-fw-semibold)]">埋点监控</h1>
      <div class="flex items-center gap-[var(--jc-space-3)]">
        <div class="w-[120px] shrink-0">
          <ManageSelect v-model="days" :options="DAYS_OPTIONS" aria-label="时间范围" />
        </div>
        <ManageButton variant="ghost" size="sm" :loading="loading" @click="loadAll()">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </ManageButton>
      </div>
    </header>

    <!-- 初次加载骨架: 加载中不渲染空态文案, 与「确实没数据」区分 -->
    <template v-if="!overview && !error">
      <div class="grid grid-cols-2 md:grid-cols-4 gap-[var(--jc-space-4)] mb-[var(--jc-space-6)]">
        <BaseSkeleton v-for="i in 4" :key="i" shape="rect" height="84px" />
      </div>
      <BaseSkeleton shape="rect" height="260px" />
    </template>

    <!-- 加载失败: 与空态区分, 提供重试 -->
    <BaseEmpty
      v-else-if="error"
      :title="`加载失败：${error}`"
      description="网络或服务端异常, 请稍后重试"
    >
      <template #action>
        <ManageButton size="sm" variant="outline" @click="loadAll()">重试</ManageButton>
      </template>
    </BaseEmpty>

    <template v-else>
    <!-- 顶部数字卡 -->
    <section class="grid grid-cols-2 md:grid-cols-4 gap-[var(--jc-space-4)] mb-[var(--jc-space-6)]">
      <div class="jc-tm-card">
        <span class="jc-tm-card__label">PV</span>
        <span class="jc-tm-card__value">{{ overview?.pv ?? '-' }}</span>
      </div>
      <div class="jc-tm-card">
        <span class="jc-tm-card__label">UV (sessions)</span>
        <span class="jc-tm-card__value">{{ overview?.uv ?? '-' }}</span>
      </div>
      <div class="jc-tm-card jc-tm-card--err">
        <span class="jc-tm-card__label">错误数</span>
        <span class="jc-tm-card__value">{{ overview?.errorCount ?? '-' }}</span>
      </div>
      <div class="jc-tm-card">
        <span class="jc-tm-card__label">平均 API 响应</span>
        <span class="jc-tm-card__value">{{ overview ? fmtMs(overview.avgApiMs) : '-' }}</span>
      </div>
    </section>

    <!-- 错误列表 -->
    <section class="mb-[var(--jc-space-6)]">
      <header class="flex items-center justify-between mb-[var(--jc-space-3)] gap-[var(--jc-space-3)] flex-wrap">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">
          错误 <span class="text-muted text-sm">({{ total || errors.length }})</span>
        </h2>
        <div class="w-[190px] shrink-0">
          <ManageSelect v-model="statusFilter" :options="STATUS_OPTIONS" aria-label="状态筛选" />
        </div>
      </header>
      <div class="jc-tm-table">
        <div class="jc-tm-table__head">
          <span class="w-[150px]">时间</span>
          <span class="w-[100px]">状态</span>
          <span class="w-[90px]">分类</span>
          <span class="w-[170px]">路径</span>
          <span class="w-[70px] text-right">累计次数</span>
          <span class="flex-1">描述</span>
          <span class="w-[80px]"></span>
        </div>
        <div v-if="errors.length === 0" class="jc-tm-empty">暂无错误</div>
        <div v-for="row in errors" :key="row.id" class="jc-tm-row" @click="toggleExpand(row.id)">
          <span class="w-[150px] text-xs text-muted font-mono">{{ formatDateTime(row.serverTs) }}</span>
          <span class="w-[100px]">
            <StatusTag :value="row.issueStatus" :map="ISSUE_STATUS_MAP" size="xs" />
          </span>
          <span class="w-[90px]"><BaseTag variant="default" size="xs">{{ row.category || row.type }}</BaseTag></span>
          <span class="w-[170px] text-xs font-mono text-secondary truncate">{{ row.path || '-' }}</span>
          <span class="w-[70px] text-right text-xs font-mono" :class="(row.value ?? 1) > 1 ? 'text-warning font-bold' : 'text-muted'">
            {{ row.value ?? 1 }}
          </span>
          <span class="flex-1 min-w-0 text-sm">
            <span class="block" :class="expanded[row.id] ? 'whitespace-pre-wrap break-all' : 'truncate'">{{ row.label }}</span>
            <div v-if="expanded[row.id]" class="jc-tm-extra">
              <div v-if="row.resolution" class="jc-tm-extra__item">
                <div class="text-muted text-xs">resolution:</div>
                <pre class="jc-tm-stack">{{ JSON.stringify(row.resolution, null, 2) }}</pre>
              </div>
              <div v-if="row.userId">user: {{ row.userId }}</div>
              <div v-if="row.platform">platform: {{ row.platform }}</div>
              <template v-if="row.extra">
                <div v-for="[k, v] in parsedExtraEntries(row.extra)" :key="k" class="jc-tm-extra__item">
                  <div class="text-muted text-xs">{{ k }}:</div>
                  <pre class="jc-tm-stack">{{ v }}</pre>
                </div>
              </template>
            </div>
          </span>
          <span class="w-[80px]" @click.stop>
            <ManageButton
              v-if="row.issueStatus === 'unresolved' || row.issueStatus === 'reopened'"
              variant="primary" size="sm"
              @click="openResolve(row)"
            >{{ row.issueStatus === 'reopened' ? '重标解决' : '标记解决' }}</ManageButton>
            <ManageButton
              v-else
              variant="ghost" size="sm"
              :loading="undoingId === row.id"
              :disabled="undoingId !== null && undoingId !== row.id"
              @click="undoResolve(row)"
            >撤销</ManageButton>
          </span>
        </div>
        <!-- 服务端状态过滤后 total 大于已加载行数时提供加载更多(每页 100 条) -->
        <div v-if="errors.length < total" class="flex justify-center py-[var(--jc-space-3)]">
          <ManageButton variant="ghost" size="sm" :loading="loading" @click="loadAll(true)">
            加载更多 (还剩 {{ total - errors.length }})
          </ManageButton>
        </div>
      </div>
    </section>

    <!-- 标记已解决弹窗已函数化: openManageDialog + TelemetryResolveDialog(见 script) -->

    <div class="grid md:grid-cols-2 gap-[var(--jc-space-5)] mb-[var(--jc-space-6)]">
      <!-- 热点视频 -->
      <section>
        <h2 class="text-lg font-[var(--jc-fw-semibold)] mb-[var(--jc-space-3)]">热点视频 Top 10</h2>
        <div class="jc-tm-table">
          <div v-if="hotFilms.length === 0" class="jc-tm-empty">暂无数据</div>
          <RouterLink
            v-for="(f, i) in hotFilms"
            :key="f.mid"
            :to="{ path: '/filmDetail', query: { link: String(f.mid) } }"
            class="jc-tm-row jc-tm-film"
          >
            <span class="w-[24px] text-muted shrink-0">{{ i + 1 }}</span>
            <div class="w-[40px] shrink-0">
              <BaseImage
                :src="f.cover"
                :alt="filmName(f)"
                ratio="40/54"
                rounded="rounded-[var(--jc-radius-sm)]"
              />
            </div>
            <span class="flex-1 truncate text-sm">{{ filmName(f) }}</span>
            <span class="text-secondary text-sm shrink-0">{{ fmtCount(f.count) }} 次</span>
          </RouterLink>
        </div>
      </section>

      <!-- 收藏最多 -->
      <section>
        <h2 class="text-lg font-[var(--jc-fw-semibold)] mb-[var(--jc-space-3)]">收藏最多 Top 10</h2>
        <div class="jc-tm-table">
          <div v-if="mostFavorited.length === 0" class="jc-tm-empty">暂无数据</div>
          <RouterLink
            v-for="(f, i) in mostFavorited"
            :key="f.mid"
            :to="{ path: '/filmDetail', query: { link: String(f.mid) } }"
            class="jc-tm-row jc-tm-film"
          >
            <span class="w-[24px] text-muted shrink-0">{{ i + 1 }}</span>
            <div class="w-[40px] shrink-0">
              <BaseImage
                :src="f.cover"
                :alt="filmName(f)"
                ratio="40/54"
                rounded="rounded-[var(--jc-radius-sm)]"
              />
            </div>
            <span class="flex-1 truncate text-sm">{{ filmName(f) }}</span>
            <span class="text-secondary text-sm shrink-0">{{ fmtCount(f.count) }} 收藏</span>
          </RouterLink>
        </div>
      </section>
    </div>

    <div class="grid md:grid-cols-2 gap-[var(--jc-space-5)]">
      <!-- API 性能 -->
      <section>
        <h2 class="text-lg font-[var(--jc-fw-semibold)] mb-[var(--jc-space-3)]">API 性能 Top 10</h2>
        <div class="jc-tm-table">
          <div class="jc-tm-table__head">
            <span class="flex-1">接口</span>
            <span class="w-[50px] text-right">P50</span>
            <span class="w-[50px] text-right">P95</span>
            <span class="w-[50px] text-right">P99</span>
          </div>
          <div v-if="apiPerf.length === 0" class="jc-tm-empty">暂无</div>
          <div v-for="a in apiPerf" :key="a.action" class="jc-tm-row">
            <span class="flex-1 truncate font-mono text-xs">{{ a.action }}</span>
            <span class="w-[50px] text-right text-xs">{{ fmtMs(a.p50) }}</span>
            <span class="w-[50px] text-right text-xs">{{ fmtMs(a.p95) }}</span>
            <span class="w-[50px] text-right text-xs">{{ fmtMs(a.p99) }}</span>
          </div>
        </div>
      </section>
    </div>
    </template>
  </div>
</template>

<style scoped>
.jc-tm-card {
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
  padding: var(--jc-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-1);
}
.jc-tm-card--err .jc-tm-card__value {
  color: var(--jc-danger);
}
.jc-tm-card__label {
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-secondary);
}
.jc-tm-card__value {
  font-size: 1.75rem;
  font-weight: var(--jc-fw-semibold);
}
.jc-tm-table {
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
  overflow: hidden;
}
.jc-tm-table__head {
  display: flex;
  gap: var(--jc-space-3);
  padding: var(--jc-space-3) var(--jc-space-4);
  background: rgba(255, 255, 255, 0.03);
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
  font-weight: var(--jc-fw-semibold);
  border-bottom: 1px solid var(--jc-border-subtle);
}
.jc-tm-row {
  display: flex;
  gap: var(--jc-space-3);
  padding: var(--jc-space-3) var(--jc-space-4);
  border-bottom: 1px solid var(--jc-border-subtle);
  align-items: flex-start;
  cursor: pointer;
}
.jc-tm-row:last-child {
  border-bottom: none;
}
.jc-tm-row:hover {
  background: rgba(255, 255, 255, 0.02);
}
.jc-tm-empty {
  padding: var(--jc-space-6);
  text-align: center;
  color: var(--jc-text-muted);
}
.jc-tm-film {
  align-items: center;
  text-decoration: none;
  color: inherit;
}
.jc-tm-film:hover .flex-1 {
  color: var(--jc-text-primary);
}
.jc-tm-extra {
  margin-top: var(--jc-space-2);
  padding: var(--jc-space-2);
  background: var(--jc-bg-base);
  border-radius: var(--jc-radius-sm);
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
  font-family: var(--jc-font-mono);
}
.jc-tm-extra pre {
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
.jc-tm-extra__item {
  margin-top: var(--jc-space-2);
}
.jc-tm-extra__item:first-child {
  margin-top: 0;
}
.jc-tm-stack {
  background: rgba(0, 0, 0, 0.35);
  padding: var(--jc-space-2);
  border-radius: var(--jc-radius-sm);
  max-height: 360px;
  overflow: auto;
}
</style>
