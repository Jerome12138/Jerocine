<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { EpidOverrideImportItem, EpidOverrideListResp, EpidReport } from '@/api/manage/epid'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 集身份（epid）派生卡片（后台运维）。
 *
 * 一键把全库"集身份"重新派生：`fill` 补缺/更新（不动已有），`rebuild` 清空重建（规则升级后用）。
 * 派生表 `movie_episode_epid` 是**单向派生**物（真相源是 `movie_play_source.episodes_json`）
 * ⇒ 清空/重建都不影响站点可用：出参没有 epid 时，端侧自动回退到实时解析（方案 §4.6）。
 *
 * 参数（本卡片可调）：
 *   `batch`      每批源行数（默认 500；调小 ⇒ 单批更短、对线上读的抖动更小，代价是总时长更长）
 *   `timeoutMin` 单次任务时长上限（默认 60，上限 240）—— 到点自动停；**没跑完请再点「补缺」续跑**
 *                （补缺会从表头快速跳过已完成的，只补差；**不要**再点「清空重建」）
 *
 * ---- 进度口径（2026-11-11 线上事故后重做）----
 * 旧版把「精确 COUNT」当进度读数，派生表涨到 590 万行后 `COUNT(*)` 要 19.2s、
 * `COUNT(DISTINCT src_row_id)` 要 8.6s ⇒ 撞穿接口的每项超时、被降级成 **0**
 * ⇒ 管理员看到 `0 / 0` 以为数据丢了 ✗。
 * 现在：
 *   · **影片完整度**（= 该片全部源行都已派生的影片数 / 影片总数）是**主口径**——不碰 `episodes_json`
 *     巨列，且与 §4.8 门槛的"成熟"判据同一个定义 ⇒ 看它就能预判门槛能不能过；代价 10~14s，走后台 60s 快照。
 *   · 全库计数（派生行数 / 已派生源行 / 源行总数）走**写侧计数器 + 缓存**，O(1)。
 *   · 本轮扫过/重建/速率/预计剩余是**内存计数**，0 成本。
 * ⚠️ **游标不要当进度看**：源行 id 有大量空洞，且"长尾趟"会快速跳过已完成的区间
 * （实测游标 57% 时真实覆盖已 91%）⇒ 只作技术细节展示。
 * ⚠️ 指标**不随页面自动加载**（重型查询慢）⇒ 由用户点「刷新指标」按需拉取；触发派生后也会自动刷新一次。
 * 写操作口径：useAsyncAction + { silent: true }，成功/失败文案由 run 单点负责。
 */
const loading = ref(false)
const loadError = ref(false)
const report = ref<EpidReport | null>(null)
const batch = ref(500)
const timeoutMin = ref(60)
const { busy, run } = useAsyncAction()

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    report.value = await manageApi.epid.report()
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}

function pct(v: number | null | undefined): string {
  return typeof v === 'number' && Number.isFinite(v) ? `${v.toFixed(1)}%` : '—'
}

function num(v: number | null | undefined): string {
  return typeof v === 'number' && Number.isFinite(v) ? v.toLocaleString('zh-CN') : '—'
}

/** 秒 → 人类可读（"1h2m" / "3m20s" / "45s"）。 */
function dur(sec: number | null | undefined): string {
  if (typeof sec !== 'number' || !Number.isFinite(sec) || sec <= 0) return '—'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.round(sec % 60)
  if (h > 0) return `${h}h${m}m`
  if (m > 0) return `${m}m${s}s`
  return `${s}s`
}

/** 毫秒时间戳 → "N 秒前 / N 分钟前"（0 或非数 ⇒ 空串）。 */
function ago(ms: number | null | undefined): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms <= 0) return ''
  const d = Math.max(0, Math.round((Date.now() - ms) / 1000))
  if (d < 60) return `${d} 秒前`
  if (d < 3600) return `${Math.round(d / 60)} 分钟前`
  return `${Math.round(d / 3600)} 小时前`
}

function clampInt(v: number, lo: number, hi: number, dflt: number): number {
  const n = Math.floor(Number(v))
  if (!Number.isFinite(n) || n <= 0) return dflt
  return Math.min(Math.max(n, lo), hi)
}

const prog = computed(() => report.value?.progress ?? null)

/** 影片完整度百分比（null = 还没有快照 ⇒ 显示"计算中"）。 */
const moviePct = computed<number | null>(() => {
  const p = prog.value
  if (!p || !p.movieTotal) return null
  return (p.movieComplete * 100) / p.movieTotal
})

/** 进度条宽度（内联 style；UnoCSS 不能静态提取动态 class）。 */
const barWidth = computed(() => {
  const v = moviePct.value
  if (v === null) return '0%'
  return `${Math.min(100, Math.max(0, v)).toFixed(1)}%`
})

/**
 * 重型指标本轮**是否算过**。
 * `onlyProgress=true`（任务进行中）时，对齐率/分层/成熟度/形态/覆盖率都不是"算出来是 0"而是**没算**
 * ⇒ 必须显示为"—/进行中"，不能渲染成 0.0% 误导（这正是上一版让人以为数据丢了的原因之一）。
 */
const heavy = computed(() => !!report.value && !report.value.onlyProgress)

/** 本轮速率文案。 */
const rateTxt = computed(() => {
  const st = report.value?.status
  if (!st || st.elapsedSec <= 0) return '—'
  const r = (v: number) => (v > 0 ? `${v.toFixed(1)}/s` : '0/s')
  return `扫 ${r(st.scanRate)} · 重建 ${r(st.rebuildRate)}`
})

function derive(mode: 'fill' | 'rebuild'): void {
  const b = clampInt(batch.value, 100, 5000, 500)
  const t = clampInt(timeoutMin.value, 1, 240, 60)
  batch.value = b
  timeoutMin.value = t
  void run({
    confirm:
      mode === 'rebuild'
        ? { title: '清空重建派生表？', okText: '清空重建', danger: true }
        : { title: '开始补缺/更新派生？', okText: '开始' },
    action: () => manageApi.epid.deriveAll(mode, { batch: b, timeoutMin: t }),
    successMsg: `已排队（${mode}，batch=${b}，上限 ${t} 分钟）`,
    after: load
  })
}

function refresh(): void {
  void load()
}

// ===== 源头校正（裁定台账 movie_epid_override · T4, 2026-11-12）=====
//
// 「源头校正」面板：把人工核对过的裁定（corrected=要改 / kept=判定不改）录入台账 →
// 打开生效开关 → 应用（或等下轮派生在写库前自动套用）。
// 流程口径（方案 §4 状态机）：import（pending）→ 人工核对 → 打开开关（D3，默认关、重启回关）→ apply。
// 台账是小表（12 条量级）⇒ 列表/统计挂载即拉取、拉取失败只给行内提示；导入/应用/开关走
// useAsyncAction（独立 busy 实例，不与重型派生动作互抢）。
const ov = ref<EpidOverrideListResp | null>(null)
const ovLoading = ref(false)
const ovError = ref(false)
const ovImportText = ref('')
const ovNote = ref('')
const { busy: ovBusy, run: runOv } = useAsyncAction()

async function loadOverrides(): Promise<void> {
  ovLoading.value = true
  ovError.value = false
  try {
    ov.value = await manageApi.epid.overridesList()
  } catch {
    ovError.value = true
  } finally {
    ovLoading.value = false
  }
}

/** 导入候选：textarea 粘贴 JSON 数组；前端只做形状校验，逐行校验/去重/key_epid 对账都在后端。 */
function importCandidates(): void {
  let items: EpidOverrideImportItem[]
  try {
    const parsed: unknown = JSON.parse(ovImportText.value)
    if (!Array.isArray(parsed) || parsed.length === 0) throw new Error('候选文件必须是 JSON 数组且非空')
    items = parsed as EpidOverrideImportItem[]
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '候选文件不是合法 JSON')
    return
  }
  let note = ''
  void runOv({
    confirm: { title: `导入 ${items.length} 条候选到台账？（status=pending，待人工核对确认后才生效）`, okText: '导入' },
    action: async () => {
      const res = await manageApi.epid.overridesImport(items, { silent: true })
      const rej = res.rejected.length ? ` · 拒收 ${res.rejected.length}（原因见后端响应）` : ''
      note = `导入结果：新增 ${res.inserted} · 去重 ${res.deduped}${rej}`
    },
    successMsg: '导入完成',
    after: () => {
      ovNote.value = note
      void loadOverrides()
    }
  })
}

/** 应用全部 active（B 快速应用：只点改命中行的 epid 一列，幂等可重复；开关关着时按钮禁用）。 */
function applyOverrides(): void {
  void runOv({
    confirm: { title: '应用全部 active 裁定？（只点改命中行的 epid 列，幂等；三元组对不上的会标 stale 不改值）', okText: '应用' },
    action: async () => {
      const res = await manageApi.epid.overridesApply(undefined, { silent: true })
      ovNote.value =
        `应用结果：considered=${res.considered} · applied=${res.applied} · inEffect=${res.inEffect}` +
        ` · kept=${res.kept} · stale=${res.stale} · missing=${res.missing}`
    },
    successMsg: '应用完成',
    after: () => void loadOverrides()
  })
}

/** 切换 D3 生效开关（内存态：重启回默认关 = fail-safe；已落库的校正值不动）。 */
function toggleOverrideEnabled(): void {
  const next = !(ov.value?.enabled ?? false)
  void runOv({
    confirm: next
      ? {
          title: '打开生效开关？打开后：派生写库前自动套用 active 裁定，「应用」放行；重启后回到默认关。',
          okText: '打开'
        }
      : {
          title: '关闭生效开关？关闭后：派生不再套用校正、「应用」拒绝；已落库的校正值不动，重新打开可再应用。',
          okText: '关闭',
          danger: true
        },
    action: () => manageApi.epid.overridesSetEnabled(next, { silent: true }),
    successMsg: next ? '已打开（重启后回默认关）' : '已关闭',
    after: () => void loadOverrides()
  })
}

onMounted(() => void loadOverrides())
</script>

<template>
  <section
    class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-5)]"
  >
    <header>
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">集身份（epid）派生</h2>
      <p class="mt-1 text-sm text-muted">
        把"整片所有源的集名"归一出集身份（自带期号 → 他源桥接 → 锚点），落派生表供切源/续播定位。
        任务**异步**执行、与采集收尾的自动触发互斥；期间可随时刷新看进度。
        到点自动停；**没跑完请再点「补缺」续跑**（不要再点「清空重建」）。
      </p>
    </header>

    <div class="flex flex-wrap items-end gap-[var(--jc-space-4)] text-sm">
      <label class="flex flex-col gap-1">
        <span class="text-muted">每批源行数（100~5000）</span>
        <input
          v-model.number="batch"
          type="number"
          min="100"
          max="5000"
          step="100"
          class="w-[9rem] rounded-[var(--jc-radius-sm)] border border-[var(--jc-border)] bg-transparent px-[var(--jc-space-2)] py-1"
        />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-muted">单次时长上限（分钟，1~240）</span>
        <input
          v-model.number="timeoutMin"
          type="number"
          min="1"
          max="240"
          step="10"
          class="w-[9rem] rounded-[var(--jc-radius-sm)] border border-[var(--jc-border)] bg-transparent px-[var(--jc-space-2)] py-1"
        />
      </label>
    </div>

    <BaseSkeleton v-if="loading" shape="rect" height="160px" />
    <p v-else-if="loadError" class="text-sm text-muted">指标拉取失败（点「刷新指标」重试）</p>
    <p v-else-if="!report" class="text-sm text-muted">
      指标未加载（重型查询慢，故按需拉取）—— 点「刷新指标」查看。
    </p>

    <div v-else class="flex flex-col gap-[var(--jc-space-4)] text-sm">
      <!-- ===== 主口径：影片完整度（任务进行中也给，来自后台快照）===== -->
      <div>
        <div class="flex flex-wrap items-baseline justify-between gap-2">
          <span class="text-muted">影片完整度（该片全部源行都已派生 / 影片总数）</span>
          <span class="font-[var(--jc-fw-semibold)]">
            {{ moviePct === null ? '计算中…' : pct(moviePct) }}
          </span>
        </div>
        <div
          class="mt-1 h-2 w-full overflow-hidden rounded-full bg-[var(--jc-border)]"
          role="progressbar"
          :aria-valuenow="moviePct === null ? undefined : Math.round(moviePct)"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <div class="h-full rounded-full bg-[var(--jc-info)]" :style="{ width: barWidth }" />
        </div>
        <div class="mt-1 text-muted">
          <template v-if="prog && prog.movieTotal > 0">
            完整 {{ num(prog.movieComplete) }} / {{ num(prog.movieTotal) }} 部；还缺
            <span class="font-[var(--jc-fw-semibold)]">{{ num(prog.movieTotal - prog.movieComplete) }}</span>
            部影片 ·
            <span class="font-[var(--jc-fw-semibold)]">{{ num(prog.missingSrcRows) }}</span> 个源行
            <span v-if="prog.movieStatStale">（展示 {{ ago(prog.movieStatAt) }}的快照，后台刷新中）</span>
            <span v-else-if="prog.movieStatAt > 0">（{{ ago(prog.movieStatAt) }}采样）</span>
          </template>
          <template v-else>
            计算中…（这条要按影片聚合，一次约 10s ⇒ 走后台刷新，稍后再点「刷新指标」）
          </template>
        </div>
      </div>

      <!-- ===== 本轮任务（内存计数，0 成本）—— 仅派生中渲染进度类读数（空闲时它们全是陈旧 0）===== -->
      <dl class="grid grid-cols-2 gap-[var(--jc-space-3)]">
        <div>
          <dt class="text-muted">任务状态</dt>
          <dd>
            {{ report.deriving ? '派生中' : report.status.running ? '在跑' : '空闲' }}
            <span v-if="report.deriving">
              （{{ report.status.mode }} / {{ report.status.hotPhase ? '热门趟' : '长尾趟' }}）
            </span>
          </dd>
        </div>
        <template v-if="report.deriving">
          <div>
            <dt class="text-muted">本轮扫过 / 重建</dt>
            <dd>
              {{ num(report.status.scanned) }} / {{ num(report.status.rebuilt) }} 源行（{{ num(report.status.batches) }} 批）
            </dd>
          </div>
          <div>
            <dt class="text-muted">本轮速率 / 已跑</dt>
            <dd>{{ rateTxt }} · {{ dur(report.status.elapsedSec) }}</dd>
          </div>
          <div>
            <dt class="text-muted">预计还需（按重建速率，下界）</dt>
            <dd>{{ prog && prog.etaSec > 0 ? dur(prog.etaSec) : '—' }}</dd>
          </div>
        </template>
        <div v-else>
          <dt class="text-muted">进度类指标</dt>
          <dd class="text-muted">空闲态 —— 本轮扫过 / 速率 / 预计还需只在派生中有意义</dd>
        </div>
      </dl>

      <!-- ===== 全库计数（写侧计数器 O(1) + 缓存）===== -->
      <dl class="grid grid-cols-2 gap-[var(--jc-space-3)]">
        <div>
          <dt class="text-muted">派生行数 / 空 epid</dt>
          <dd>{{ num(prog?.derivedRows ?? report.derivedRows) }} / {{ heavy ? num(report.emptyEpidRows) : '—' }}</dd>
        </div>
        <div>
          <dt class="text-muted">源行覆盖（已派生 / 总数）</dt>
          <dd>
            {{ num(prog?.derivedSrcRows) }} / {{ num(prog?.srcRows) }}
            <span v-if="prog && prog.srcRows > 0" class="text-muted">（还缺 {{ num(prog.remainSrcRows) }}）</span>
          </dd>
        </div>
      </dl>
      <p v-if="prog && !prog.countingAvailable" class="text-muted">
        ⚠️ 全库计数器尚未初始化（首次会上报后再刷新一次即可看到确切值）。
      </p>

      <p class="text-muted">
        游标 {{ num(report.status.cursor) }} —— <span class="font-[var(--jc-fw-semibold)]">不要拿它当进度</span>：
        源行 id 有大量空洞，且长尾趟会快速跳过已完成的区间（实测游标 57% 时真实覆盖已 91%）。
      </p>

      <!-- ===== 重型指标：任务进行中一律"未算"，不渲染成 0 ===== -->
      <p v-if="report.onlyProgress" class="text-muted">
        对齐率 / 分类对齐 / 空形态 / 覆盖率：<span class="font-[var(--jc-fw-semibold)]">任务进行中不计算</span>
        （分母里混着"还没跑到"的片，读数无判定意义）—— 跑完再点「刷新指标」。
      </p>
      <template v-else>
        <p v-if="report.metricsAt > 0" class="text-muted">
          重型指标采样：{{ ago(report.metricsAt) }}
          <span v-if="report.metricsStale">（后台刷新中，当前展示的是上一份快照）</span>
        </p>
        <p v-else class="text-muted">
          重型指标正在后台计算（每条 10~33s，共 5 条）—— 稍后点「刷新指标」即可。
        </p>
        <dl class="grid grid-cols-2 gap-[var(--jc-space-3)]">
          <div>
            <dt class="text-muted">对齐率（双粒度；片级判定单位是片：单源默认对齐，多源片有跨源共享组即对齐）</dt>
            <dd>
              片级 {{ pct(report.classAlignmentAll?.pct ?? 0) }}
              <span class="text-muted">
                （{{ num(report.classAlignmentAll?.alignedMks) }}/{{ num(report.classAlignmentAll?.totalMks) }} 片）
              </span>
              · 集级 {{ pct(report.classAlignmentAll?.rowsPct ?? 0) }}
              <span class="text-muted">
                （{{ num(report.classAlignmentAll?.alignedRows) }}/{{ num(report.classAlignmentAll?.totalRows) }} 集行跨源共享）
              </span>
              <span class="text-muted">组级对照 {{ pct(report.alignmentPct) }}</span>
            </dd>
          </div>
          <div>
            <dt class="text-muted">覆盖率（§6.1）</dt>
            <dd v-if="report.coverage">{{ pct(report.coverage.pct) }}</dd>
            <dd v-else class="text-muted">{{ report.coverageError || '—' }}</dd>
          </div>
          <div class="col-span-2">
            <dt class="text-muted">期号来源分布</dt>
            <dd>
              <!-- 空值守卫：后端已保证返回 `{}` 而非 null，这里再兜一层（历史事故：`Object.keys(null)` 抛错
                   ⇒ Vue 渲染该组件失败 ⇒ 整张卡片消失，且 production 构建不打 console ⇒ 看着像"没报错"） -->
              <span v-for="(n, k) in report.byFromSrc" :key="k" class="mr-3">{{ k }}={{ n }}</span>
              <span v-if="!report.byFromSrc || !Object.keys(report.byFromSrc).length" class="text-muted">—</span>
            </dd>
          </div>
          <!-- 分类跨源对齐（双粒度，2026-11-15）：取代旧的"有身份率/成熟度"两块 -->
          <div v-if="report.classAlignment?.length" class="col-span-2">
            <dt class="text-muted">按分类跨源对齐（片级 = epid 只有一种；集级 = site≥2 的集行占比）</dt>
            <dd class="flex flex-col">
              <span v-for="c in report.classAlignment" :key="c.pid">
                {{ c.category }}：片级 {{ pct(c.pct) }}（{{ num(c.alignedMks) }}/{{ num(c.totalMks) }}）
                · 集级 {{ pct(c.rowsPct) }}（{{ num(c.alignedRows) }}/{{ num(c.totalRows) }}）
              </span>
            </dd>
          </div>
          <!-- 空 epid 的形态分档：回答"为什么空" -->
          <div v-if="report.emptyByShape?.length" class="col-span-2">
            <dt class="text-muted">空 epid 的集名形态</dt>
            <dd>
              <span v-for="s in report.emptyByShape" :key="s.shape" class="mr-3">
                {{ s.shape }}={{ s.rowCnt }}
              </span>
            </dd>
          </div>
        </dl>
      </template>

      <p v-if="report.metricsError" class="text-muted">⚠️ {{ report.metricsError }}</p>
    </div>

    <div class="flex flex-wrap gap-[var(--jc-space-3)]">
      <ManageButton variant="primary" :loading="busy" @click="derive('fill')">
        补缺（不动已有）
      </ManageButton>
      <ManageButton variant="danger" :loading="busy" @click="derive('rebuild')">
        清空重建
      </ManageButton>
      <ManageButton variant="ghost" :disabled="busy" @click="refresh">刷新指标</ManageButton>
    </div>

    <!-- ===== 源头校正（裁定台账 · T4）：导入候选 → 核对 → 打开开关 → 应用 ===== -->
    <div
      class="flex flex-col gap-[var(--jc-space-4)] border-t border-[var(--jc-border)] pt-[var(--jc-space-5)] text-sm"
    >
      <div>
        <h3 class="font-[var(--jc-fw-semibold)]">源头校正（裁定台账）</h3>
        <p class="mt-1 text-muted">
          规则修不动的个例由人工裁定（corrected=要改 / kept=判定不改）落在台账；打开开关后派生/重建
          会在写库前自动套用、清空重建也不丢。流程：导入候选 → 核对 → 打开生效开关 → 应用。
        </p>
      </div>

      <!-- 开关状态可见（D3） -->
      <div class="flex flex-wrap items-center gap-[var(--jc-space-3)]">
        <span class="text-muted">生效开关（默认关 · 重启回关）：</span>
        <span v-if="ov" class="font-[var(--jc-fw-semibold)]">{{ ov.enabled ? '已打开' : '已关闭' }}</span>
        <span v-else class="text-muted">—</span>
        <ManageButton size="small" variant="ghost" :loading="ovBusy" :disabled="ovLoading" @click="toggleOverrideEnabled">
          {{ ov?.enabled ? '关闭' : '打开' }}
        </ManageButton>
        <span v-if="ov && !ov.enabled" class="text-muted">
          关着时：派生不套用校正、「应用」拒绝（导入与列表不受影响）。
        </span>
      </div>

      <!-- 统计（三维计数；后端保证对象非 null，这里仍按卡片惯例兜空数组） -->
      <div v-if="ov" class="flex flex-wrap gap-x-6 gap-y-1">
        <div>
          <span class="text-muted">按状态：</span>
          <span v-for="s in ov.stats.byStatus" :key="s.name" class="mr-3">{{ s.name }}={{ s.n }}</span>
          <span v-if="!ov.stats.byStatus.length" class="text-muted">—</span>
        </div>
        <div>
          <span class="text-muted">按裁定：</span>
          <span v-for="s in ov.stats.byVerdict" :key="s.name" class="mr-3">{{ s.name }}={{ s.n }}</span>
          <span v-if="!ov.stats.byVerdict.length" class="text-muted">—</span>
        </div>
        <div>
          <span class="text-muted">按来源：</span>
          <span v-for="s in ov.stats.byProvenance" :key="s.name" class="mr-3">{{ s.name }}={{ s.n }}</span>
          <span v-if="!ov.stats.byProvenance.length" class="text-muted">—</span>
        </div>
      </div>

      <!-- 导入候选（粘贴 JSON 数组） -->
      <div class="flex flex-col gap-2">
        <span class="text-muted">
          导入候选（JSON 数组，逐项：srcRowId, epIndex, keyRawName, keyEpid, verdict, newEpid(corrected 必填),
          provenance, confidence, reason?; status/id 等由后端强制）
        </span>
        <textarea
          v-model="ovImportText"
          rows="4"
          spellcheck="false"
          placeholder='[{"srcRowId":9,"epIndex":2,"keyRawName":"第3期上","keyEpid":"s=-|v=-|p=上|n=3|d=20260101","verdict":"corrected","newEpid":"s=-|v=-|p=上|n=10|d=20260101","provenance":"agent","confidence":"verified"}]'
          class="w-full rounded-[var(--jc-radius-sm)] border border-[var(--jc-border)] bg-transparent px-[var(--jc-space-2)] py-1 font-mono text-xs"
        ></textarea>
        <div class="flex flex-wrap items-center gap-[var(--jc-space-3)]">
          <ManageButton size="small" variant="primary" :loading="ovBusy" @click="importCandidates">
            导入候选
          </ManageButton>
          <ManageButton
            size="small"
            variant="primary"
            :loading="ovBusy"
            :disabled="!ov?.enabled"
            @click="applyOverrides"
          >
            应用全部 active
          </ManageButton>
          <ManageButton size="small" variant="ghost" :disabled="ovBusy || ovLoading" @click="loadOverrides">
            刷新台账
          </ManageButton>
        </div>
      </div>

      <BaseSkeleton v-if="ovLoading" shape="rect" height="80px" />
      <p v-else-if="ovError" class="text-muted">台账拉取失败（点「刷新台账」重试）</p>
      <p v-if="ovNote" class="text-muted">{{ ovNote }}</p>

      <!-- 台账列表（后端白名单出参：无 evidence/审计列 —— 红线③） -->
      <div v-if="ov && ov.list.length" class="overflow-x-auto">
        <table class="w-full min-w-[760px] text-left text-xs">
          <thead class="text-muted">
            <tr>
              <th class="py-1 pr-3 font-normal">id</th>
              <th class="py-1 pr-3 font-normal">状态</th>
              <th class="py-1 pr-3 font-normal">裁定</th>
              <th class="py-1 pr-3 font-normal">集名</th>
              <th class="py-1 pr-3 font-normal">key_epid → new_epid</th>
              <th class="py-1 pr-3 font-normal">src_row_id / ep_index</th>
              <th class="py-1 pr-3 font-normal">来源/置信</th>
              <th class="py-1 font-normal">原因</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in ov.list" :key="r.id" class="border-t border-[var(--jc-border)]">
              <td class="py-1 pr-3">{{ r.id }}</td>
              <td class="py-1 pr-3">{{ r.status }}</td>
              <td class="py-1 pr-3">{{ r.verdict }}</td>
              <td class="py-1 pr-3">{{ r.keyRawName }}</td>
              <td class="py-1 pr-3 font-mono">
                {{ r.keyEpid }} → {{ r.verdict === 'kept' ? '（不变）' : r.newEpid }}
              </td>
              <td class="py-1 pr-3">{{ r.srcRowId }} / {{ r.epIndex }}</td>
              <td class="py-1 pr-3">{{ r.provenance }}/{{ r.confidence }}</td>
              <td class="py-1">{{ r.reason }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else-if="ov && !ov.list.length" class="text-muted">台账为空（先导入候选）。</p>
    </div>
  </section>
</template>
