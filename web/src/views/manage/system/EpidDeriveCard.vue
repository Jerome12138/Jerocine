<script setup lang="ts">
import { ref } from 'vue'
import { manageApi } from '@/api'
import type { EpidReport } from '@/api/manage/epid'
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
 *   `batch`      每批源行数（默认 2000；调小 ⇒ 单批更短、对线上读的抖动更小，代价是总时长更长）
 *   `timeoutMin` 单次任务时长上限（默认 60，上限 240）—— 到点自动停；**没跑完请再点「补缺」续跑**
 *                （补缺会从表头快速跳过已完成的，只补差；**不要**再点「清空重建」）
 *
 * 指标口径：对齐率 §4.8 门槛 **≥85%**；覆盖率 §6.1 = 派生行数 / Σ 集数（**空 epid 也算行**）。
 * ⚠️ 指标**不随页面自动加载**（覆盖率那条 SQL 要扫 `Σ JSON_LENGTH(episodes_json)` 巨列，很慢）
 * ⇒ 由用户点「刷新指标」按需拉取；触发派生后也会自动刷新一次。
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

function pct(v: number | undefined): string {
  return typeof v === 'number' ? `${v.toFixed(1)}%` : '—'
}

function clampInt(v: number, lo: number, hi: number, dflt: number): number {
  const n = Math.floor(Number(v))
  if (!Number.isFinite(n) || n <= 0) return dflt
  return Math.min(Math.max(n, lo), hi)
}

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
</script>

<template>
  <section
    class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-5)]"
  >
    <header>
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">集身份（epid）派生</h2>
      <p class="mt-1 text-sm text-muted">
        把"整片所有源的集名"归一出集身份（自带期号 → 他源桥接 → 锚点），落派生表供切源/续播定位。
        任务**异步**执行、与采集收尾的自动触发互斥；期间可随时刷新看游标与阶段。
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

    <BaseSkeleton v-if="loading" shape="rect" height="120px" />
    <p v-else-if="loadError" class="text-sm text-muted">指标拉取失败（点「刷新指标」重试）</p>
    <p v-else-if="!report" class="text-sm text-muted">
      指标未加载（覆盖率要扫巨列，故按需拉取）—— 点「刷新指标」查看。
    </p>
    <dl v-else class="grid grid-cols-2 gap-[var(--jc-space-3)] text-sm">
      <div>
        <dt class="text-muted">任务状态</dt>
        <dd>
          {{ report.status.running ? '在跑' : '空闲' }}
          <span v-if="report.status.running">
            （{{ report.status.mode }} / {{ report.status.hotPhase ? '热门趟' : '长尾趟' }} / 游标
            {{ report.status.cursor }}）
          </span>
        </dd>
      </div>
      <div>
        <dt class="text-muted">派生行数 / 空 epid</dt>
        <dd>{{ report.derivedRows }} / {{ report.emptyEpidRows }}</dd>
      </div>
      <div>
        <dt class="text-muted">对齐率（门槛 ≥85%）</dt>
        <dd>
          {{ pct(report.alignmentPct) }}
          <span class="text-muted">（{{ report.alignedGroups }}/{{ report.totalGroups }} 组）</span>
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
          <span v-for="(n, k) in report.byFromSrc" :key="k" class="mr-3">{{ k }}={{ n }}</span>
          <span v-if="!Object.keys(report.byFromSrc).length" class="text-muted">—</span>
        </dd>
      </div>
    </dl>

    <div class="flex flex-wrap gap-[var(--jc-space-3)]">
      <ManageButton variant="primary" :loading="busy" @click="derive('fill')">
        补缺（不动已有）
      </ManageButton>
      <ManageButton variant="danger" :loading="busy" @click="derive('rebuild')">
        清空重建
      </ManageButton>
      <ManageButton variant="ghost" :disabled="busy" @click="refresh">刷新指标</ManageButton>
    </div>
  </section>
</template>
