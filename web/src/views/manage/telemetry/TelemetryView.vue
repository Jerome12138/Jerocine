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
  ApiPerfItem
} from '@/api/manage/telemetry'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseDialog from '@/components/base/BaseDialog.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import { confirm } from '@/composables/useConfirm'

const days = ref<number>(7)
const overview = ref<OverviewResp | null>(null)
const errors = ref<TelemetryEventRow[]>([])
const hotFilms = ref<FilmStat[]>([])
const mostFavorited = ref<FilmStat[]>([])
const apiPerf = ref<ApiPerfItem[]>([])
const loading = ref(false)
const expanded = ref<Record<number, boolean>>({})

const DAYS_OPTIONS = [
  { value: 1, label: '近 1 天' },
  { value: 7, label: '近 7 天' },
  { value: 30, label: '近 30 天' }
]

async function loadAll(): Promise<void> {
  loading.value = true
  try {
    const [ov, errs, hf, mf, ap] = await Promise.all([
      telemetryApi.overview(days.value),
      telemetryApi.listEvents({ type: 'error', days: days.value, limit: 100, status: statusFilter.value }),
      telemetryApi.hotFilms(days.value, 10),
      telemetryApi.mostFavorited(10),
      telemetryApi.apiPerf(days.value, 10)
    ])
    overview.value = ov
    errors.value = errs.rows ?? []
    hotFilms.value = hf ?? []
    mostFavorited.value = mf ?? []
    apiPerf.value = ap ?? []
  } finally {
    loading.value = false
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

function fmtTime(s: string): string {
  if (!s) return ''
  try {
    const d = new Date(s)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch {
    return s
  }
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
const statusFilter = ref<'active' | 'all' | 'resolved' | 'reopened'>('active')
const STATUS_OPTIONS = [
  { value: 'active' as const, label: '活跃 (未解决 + 重启)' },
  { value: 'reopened' as const, label: '已重启 (解决后 24h 仍出现)' },
  { value: 'resolved' as const, label: '已解决 (≤30天)' },
  { value: 'expired' as const, label: '过期 (>30天)' },
  { value: 'all' as const, label: '全部' }
]

const resolveDialog = ref<{
  open: boolean
  row: TelemetryEventRow | null
  commitId: string
  note: string
}>({ open: false, row: null, commitId: '', note: '' })

function openResolve(row: TelemetryEventRow): void {
  resolveDialog.value = {
    open: true,
    row,
    commitId: row.resolution?.resolvedCommitId ?? '',
    note: row.resolution?.note ?? ''
  }
}

async function submitResolve(): Promise<void> {
  const r = resolveDialog.value.row
  if (!r) return
  try {
    await telemetryApi.resolveIssue({
      category: r.category,
      path: r.path,
      label: r.label,
      commitId: resolveDialog.value.commitId.trim(),
      note: resolveDialog.value.note.trim()
    })
    resolveDialog.value.open = false
    void loadAll()
  } catch {
    // 拦截器已 toast
  }
}

async function undoResolve(row: TelemetryEventRow): Promise<void> {
  const ok = await confirm({
    title: '撤销已解决', desc: '撤销后此问题重回未解决列表, 继续累计计数.'
  })
  if (!ok) return
  await telemetryApi.unresolveIssue({
    category: row.category, path: row.path, label: row.label
  })
  void loadAll()
}

function statusVariant(s: TelemetryEventRow['issueStatus']): 'default' | 'success' | 'warning' | 'danger' {
  switch (s) {
    case 'unresolved': return 'danger'
    case 'reopened': return 'warning'
    case 'resolved': return 'success'
    default: return 'default'
  }
}
function statusText(s: TelemetryEventRow['issueStatus']): string {
  return ({
    'unresolved': '未处理',
    'reopened': '已重启',
    'resolved': '已解决',
    'expired': '已过期'
  } as Record<string, string>)[s] ?? s
}

onMounted(loadAll)
watch(days, loadAll)
watch(statusFilter, loadAll)
</script>

<template>
  <div class="container-page py-[var(--gf-space-5)]">
    <header class="flex items-center justify-between mb-[var(--gf-space-5)]">
      <h1 class="text-2xl font-[var(--gf-fw-semibold)]">埋点监控</h1>
      <div class="flex items-center gap-[var(--gf-space-3)]">
        <select
          v-model.number="days"
          class="bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-2)]"
          data-focusable="true"
        >
          <option v-for="o in DAYS_OPTIONS" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <BaseButton variant="ghost" size="sm" :loading="loading" @click="loadAll">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
      </div>
    </header>

    <!-- 顶部数字卡 -->
    <section class="grid grid-cols-2 md:grid-cols-4 gap-[var(--gf-space-4)] mb-[var(--gf-space-6)]">
      <div class="gf-tm-card">
        <span class="gf-tm-card__label">PV</span>
        <span class="gf-tm-card__value">{{ overview?.pv ?? '-' }}</span>
      </div>
      <div class="gf-tm-card">
        <span class="gf-tm-card__label">UV (sessions)</span>
        <span class="gf-tm-card__value">{{ overview?.uv ?? '-' }}</span>
      </div>
      <div class="gf-tm-card gf-tm-card--err">
        <span class="gf-tm-card__label">错误数</span>
        <span class="gf-tm-card__value">{{ overview?.errorCount ?? '-' }}</span>
      </div>
      <div class="gf-tm-card">
        <span class="gf-tm-card__label">平均 API 响应</span>
        <span class="gf-tm-card__value">{{ overview ? fmtMs(overview.avgApiMs) : '-' }}</span>
      </div>
    </section>

    <!-- 错误列表 -->
    <section class="mb-[var(--gf-space-6)]">
      <header class="flex items-center justify-between mb-[var(--gf-space-3)] gap-[var(--gf-space-3)] flex-wrap">
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">
          错误 <span class="text-muted text-sm">({{ errors.length }})</span>
        </h2>
        <select
          v-model="statusFilter"
          class="bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-2)] text-sm"
          data-focusable="true"
        >
          <option v-for="o in STATUS_OPTIONS" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
      </header>
      <div class="gf-tm-table">
        <div class="gf-tm-table__head">
          <span class="w-[150px]">时间</span>
          <span class="w-[100px]">状态</span>
          <span class="w-[90px]">分类</span>
          <span class="w-[170px]">路径</span>
          <span class="w-[50px] text-right">次数</span>
          <span class="flex-1">描述</span>
          <span class="w-[80px]"></span>
        </div>
        <div v-if="errors.length === 0" class="gf-tm-empty">暂无错误</div>
        <div v-for="row in errors" :key="row.id" class="gf-tm-row" @click="toggleExpand(row.id)">
          <span class="w-[150px] text-xs text-muted font-mono">{{ fmtTime(row.serverTs) }}</span>
          <span class="w-[100px]">
            <BaseTag :variant="statusVariant(row.issueStatus)" size="xs">{{ statusText(row.issueStatus) }}</BaseTag>
          </span>
          <span class="w-[90px]"><BaseTag variant="default" size="xs">{{ row.category || row.type }}</BaseTag></span>
          <span class="w-[170px] text-xs font-mono text-secondary truncate">{{ row.path || '-' }}</span>
          <span class="w-[50px] text-right text-xs font-mono" :class="(row.value ?? 1) > 1 ? 'text-warning font-bold' : 'text-muted'">
            x{{ row.value ?? 1 }}
          </span>
          <span class="flex-1 min-w-0 text-sm">
            <span class="block" :class="expanded[row.id] ? 'whitespace-pre-wrap break-all' : 'truncate'">{{ row.label }}</span>
            <div v-if="expanded[row.id]" class="gf-tm-extra">
              <div v-if="row.resolution" class="gf-tm-extra__item">
                <div class="text-muted text-xs">resolution:</div>
                <pre class="gf-tm-stack">{{ JSON.stringify(row.resolution, null, 2) }}</pre>
              </div>
              <div v-if="row.userId">user: {{ row.userId }}</div>
              <div v-if="row.platform">platform: {{ row.platform }}</div>
              <template v-if="row.extra">
                <div v-for="[k, v] in parsedExtraEntries(row.extra)" :key="k" class="gf-tm-extra__item">
                  <div class="text-muted text-xs">{{ k }}:</div>
                  <pre class="gf-tm-stack">{{ v }}</pre>
                </div>
              </template>
            </div>
          </span>
          <span class="w-[80px]" @click.stop>
            <BaseButton
              v-if="row.issueStatus === 'unresolved' || row.issueStatus === 'reopened'"
              variant="primary" size="sm"
              @click="openResolve(row)"
            >{{ row.issueStatus === 'reopened' ? '重标解决' : '标记解决' }}</BaseButton>
            <BaseButton
              v-else
              variant="ghost" size="sm"
              @click="undoResolve(row)"
            >撤销</BaseButton>
          </span>
        </div>
      </div>
    </section>

    <!-- 标记已解决 Dialog -->
    <BaseDialog v-model:visible="resolveDialog.open" title="标记问题已解决">
      <div v-if="resolveDialog.row" class="flex flex-col gap-[var(--gf-space-3)]">
        <div class="text-sm text-secondary">
          <div><b>分类:</b> {{ resolveDialog.row.category }}</div>
          <div><b>路径:</b> {{ resolveDialog.row.path || '-' }}</div>
          <div><b>描述:</b> {{ resolveDialog.row.label }}</div>
        </div>
        <ManageFormField label="修复 commit ID" required hint="git log 拿首 7-12 位">
          <ManageInput v-model="resolveDialog.commitId" placeholder="e.g. c68eff0" />
        </ManageFormField>
        <ManageFormField label="备注 (可选)">
          <ManageInput v-model="resolveDialog.note" placeholder="改了什么 / 为什么" />
        </ManageFormField>
        <p class="text-xs text-muted">
          标记后立即从活跃列表移除. 24 小时内的重复触发视为旧缓存/旧 deploy 遗留, 不算
          重开. 24 小时后到 30 天内若再次出现 → 自动变 "已重启" 重回活跃列表; 30 天都没
          再出现则状态变 "过期", 仍不显示.
        </p>
      </div>
      <template #footer>
        <BaseButton variant="ghost" @click="resolveDialog.open = false">取消</BaseButton>
        <BaseButton variant="primary" @click="submitResolve">保存</BaseButton>
      </template>
    </BaseDialog>

    <div class="grid md:grid-cols-2 gap-[var(--gf-space-5)] mb-[var(--gf-space-6)]">
      <!-- 热点视频 -->
      <section>
        <h2 class="text-lg font-[var(--gf-fw-semibold)] mb-[var(--gf-space-3)]">热点视频 Top 10</h2>
        <div class="gf-tm-table">
          <div v-if="hotFilms.length === 0" class="gf-tm-empty">暂无数据</div>
          <RouterLink
            v-for="(f, i) in hotFilms"
            :key="f.mid"
            :to="{ path: '/filmDetail', query: { link: String(f.mid) } }"
            class="gf-tm-row gf-tm-film"
          >
            <span class="w-[24px] text-muted shrink-0">{{ i + 1 }}</span>
            <BaseImage
              :src="f.cover"
              :alt="f.name"
              ratio="40/54"
              rounded="rounded-[var(--gf-radius-sm)]"
              class="w-[40px] shrink-0"
            />
            <span class="flex-1 truncate text-sm">{{ f.name }}</span>
            <span class="text-secondary text-sm shrink-0">{{ fmtCount(f.count) }} 次</span>
          </RouterLink>
        </div>
      </section>

      <!-- 收藏最多 -->
      <section>
        <h2 class="text-lg font-[var(--gf-fw-semibold)] mb-[var(--gf-space-3)]">收藏最多 Top 10</h2>
        <div class="gf-tm-table">
          <div v-if="mostFavorited.length === 0" class="gf-tm-empty">暂无数据</div>
          <RouterLink
            v-for="(f, i) in mostFavorited"
            :key="f.mid"
            :to="{ path: '/filmDetail', query: { link: String(f.mid) } }"
            class="gf-tm-row gf-tm-film"
          >
            <span class="w-[24px] text-muted shrink-0">{{ i + 1 }}</span>
            <BaseImage
              :src="f.cover"
              :alt="f.name"
              ratio="40/54"
              rounded="rounded-[var(--gf-radius-sm)]"
              class="w-[40px] shrink-0"
            />
            <span class="flex-1 truncate text-sm">{{ f.name }}</span>
            <span class="text-secondary text-sm shrink-0">{{ fmtCount(f.count) }} 收藏</span>
          </RouterLink>
        </div>
      </section>
    </div>

    <div class="grid md:grid-cols-2 gap-[var(--gf-space-5)]">
      <!-- API 性能 -->
      <section>
        <h2 class="text-lg font-[var(--gf-fw-semibold)] mb-[var(--gf-space-3)]">API 性能 Top 10</h2>
        <div class="gf-tm-table">
          <div class="gf-tm-table__head">
            <span class="flex-1">接口</span>
            <span class="w-[50px] text-right">P50</span>
            <span class="w-[50px] text-right">P95</span>
            <span class="w-[50px] text-right">P99</span>
          </div>
          <div v-if="apiPerf.length === 0" class="gf-tm-empty">暂无</div>
          <div v-for="a in apiPerf" :key="a.action" class="gf-tm-row">
            <span class="flex-1 truncate font-mono text-xs">{{ a.action }}</span>
            <span class="w-[50px] text-right text-xs">{{ fmtMs(a.p50) }}</span>
            <span class="w-[50px] text-right text-xs">{{ fmtMs(a.p95) }}</span>
            <span class="w-[50px] text-right text-xs">{{ fmtMs(a.p99) }}</span>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.gf-tm-card {
  background: var(--gf-bg-elevated);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-md);
  padding: var(--gf-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-1);
}
.gf-tm-card--err .gf-tm-card__value {
  color: var(--gf-danger);
}
.gf-tm-card__label {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-secondary);
}
.gf-tm-card__value {
  font-size: 1.75rem;
  font-weight: var(--gf-fw-semibold);
}
.gf-tm-table {
  background: var(--gf-bg-elevated);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-md);
  overflow: hidden;
}
.gf-tm-table__head {
  display: flex;
  gap: var(--gf-space-3);
  padding: var(--gf-space-3) var(--gf-space-4);
  background: rgba(255, 255, 255, 0.03);
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-secondary);
  font-weight: var(--gf-fw-semibold);
  border-bottom: 1px solid var(--gf-border-subtle);
}
.gf-tm-row {
  display: flex;
  gap: var(--gf-space-3);
  padding: var(--gf-space-3) var(--gf-space-4);
  border-bottom: 1px solid var(--gf-border-subtle);
  align-items: flex-start;
  cursor: pointer;
}
.gf-tm-row:last-child {
  border-bottom: none;
}
.gf-tm-row:hover {
  background: rgba(255, 255, 255, 0.02);
}
.gf-tm-empty {
  padding: var(--gf-space-6);
  text-align: center;
  color: var(--gf-text-muted);
}
.gf-tm-film {
  align-items: center;
  text-decoration: none;
  color: inherit;
}
.gf-tm-film:hover .flex-1 {
  color: var(--gf-text-primary);
}
.gf-tm-extra {
  margin-top: var(--gf-space-2);
  padding: var(--gf-space-2);
  background: var(--gf-bg-base);
  border-radius: var(--gf-radius-sm);
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-secondary);
  font-family: var(--gf-font-mono);
}
.gf-tm-extra pre {
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
.gf-tm-extra__item {
  margin-top: var(--gf-space-2);
}
.gf-tm-extra__item:first-child {
  margin-top: 0;
}
.gf-tm-stack {
  background: rgba(0, 0, 0, 0.35);
  padding: var(--gf-space-2);
  border-radius: var(--gf-radius-sm);
  max-height: 360px;
  overflow: auto;
}
</style>
