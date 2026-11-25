<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { openManageDialog } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import { formatDateTime } from '@/utils/format'
import DomainFetchDialog from './DomainFetchDialog.vue'
import DomainRuleFormDialog from './DomainRuleFormDialog.vue'
import DomainRulePreviewDialog from './DomainRulePreviewDialog.vue'
import type { DomainFetchResult, DomainRuleView } from '@/api/manage/system'

/**
 * 域名替换规则（macrep）—— 独立管理页。
 *
 * 设计要点：
 *  · 规则是长期台账（源站 2024 年的公告仍在生效），按公告时间倒序、分页展示；
 *  · 拉取：弹窗列出**后端登记的来源**（未来会有多个站点/类型），点哪个拉哪个；
 *  · 拉取只增不改：完全一致跳过、目标变了新增并保留历史；
 *  · 只有 vod_pic 能在这里一键执行；播放/下载在 JSON 大列里，只展示不执行；
 *  · 封面只可能来自主站；播放/下载可选主站或某个从站。
 */
const rows = ref<DomainRuleView[]>([])
const loading = ref(false)
const loadError = ref(false)
const filter = ref('')
const keyword = ref('')
const page = ref(1)
const size = 24
const total = ref(0)
const counts = ref<Record<string, number>>({})
const stale = ref(0)

/** 采集源 id → 名称（把规则里的 sourceCode 显示成人看得懂的名字） */
const sourceNames = ref<Record<string, string>>({})

/** 来源站：规则挂在哪个公告站上，点「原文」可跳过去核对 */
const sourceMeta: Record<string, { label: string; url: string }> = {
  lz: { label: 'lz 量子资源', url: 'https://www.lzizy.com/index.php/macrep.html' },
  manual: { label: '手工录入', url: '' }
}

const tabs = [
  { key: '', label: '全部' },
  { key: 'pending', label: '待处理' },
  { key: 'applied', label: '已执行' },
  { key: 'skipped', label: '无需替换' },
  { key: 'merged', label: '已被取代' },
  { key: 'rejected', label: '已拒绝' }
]

const stateText: Record<string, string> = {
  pending: '待处理',
  applied: '已执行',
  skipped: '无需替换',
  merged: '已被取代',
  rejected: '已拒绝'
}

const pages = computed(() => Math.max(1, Math.ceil(total.value / size)))

function tabCount(key: string): number {
  return key ? (counts.value[key] ?? 0) : total.value
}

function scopeText(r: DomainRuleView): string {
  if (r.scope !== 'slave') return '主站'
  if (!r.sourceCode) return '从站 · 全部'
  return `从站 · ${sourceNames.value[r.sourceCode] ?? r.sourceCode}`
}

function sourceLabel(r: DomainRuleView): string {
  if (!r.sourceKey) return '未知来源'
  return sourceMeta[r.sourceKey]?.label ?? r.sourceKey
}

function sourceLink(r: DomainRuleView): string {
  return r.sourceUrl || sourceMeta[r.sourceKey]?.url || ''
}

function stateClass(r: DomainRuleView): string {
  switch (r.state) {
    case 'pending':
      return 'text-[var(--jc-warning)]'
    case 'applied':
      return 'text-[var(--jc-success)]'
    case 'rejected':
      return 'text-[var(--jc-danger)]'
    default:
      return 'text-secondary'
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const r = await manageApi.system.listDomainRules({
      state: filter.value,
      keyword: keyword.value,
      page: page.value,
      size
    })
    rows.value = r.list ?? []
    total.value = r.total ?? 0
    counts.value = r.counts ?? {}
    stale.value = r.stale ?? 0
    loadError.value = false
    // 当前页被删空时回退一页（避免停在空白页）
    if (!rows.value.length && page.value > 1) {
      page.value -= 1
      await load()
    }
  } catch (e) {
    loadError.value = true
    toast('error', e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function setFilter(key: string): void {
  if (filter.value === key) return
  filter.value = key
  page.value = 1
  void load()
}

let kwTimer: number | undefined
watch(keyword, () => {
  if (kwTimer) window.clearTimeout(kwTimer)
  kwTimer = window.setTimeout(() => {
    page.value = 1
    void load()
  }, 300)
})

function goto(target: number): void {
  const p = Math.min(Math.max(1, target), pages.value)
  if (p === page.value) return
  page.value = p
  void load()
}

/** 立即拉取：弹窗选来源（来源清单由后端下发） */
async function openFetch(): Promise<void> {
  const r = await openManageDialog<DomainFetchResult>({
    title: '立即拉取公告',
    component: DomainFetchDialog,
    width: '560px'
  })
  if (r) {
    page.value = 1
    await load()
  }
}

async function addRule(): Promise<void> {
  const done = await openManageDialog<boolean>({
    title: '手工添加规则',
    component: DomainRuleFormDialog,
    width: '560px'
  })
  if (done) await load()
}

async function editRule(r: DomainRuleView): Promise<void> {
  const done = await openManageDialog<boolean>({
    title: `编辑规则 #${r.id}`,
    component: DomainRuleFormDialog,
    props: { initial: r },
    width: '560px'
  })
  if (done) await load()
}

async function previewRule(r: DomainRuleView): Promise<void> {
  await openManageDialog({
    title: '影响面预览',
    component: DomainRulePreviewDialog,
    props: { rule: r },
    width: '640px'
  })
}

const { busy, run } = useAsyncAction()

async function applyRule(r: DomainRuleView): Promise<void> {
  await run({
    confirm: {
      title: '执行替换',
      desc: `将把库里 ${r.fromHost} 换成 ${r.toHost}（仅封面字段），会改动影片数据并记审计。确定执行？`,
      okText: '确认执行',
      danger: true
    },
    action: () => manageApi.system.applyDomainRule(r.id, r.fromHost, r.toHost),
    successMsg: '替换完成',
    after: load
  })
}

async function setState(r: DomainRuleView, state: string): Promise<void> {
  const labels: Record<string, string> = { skipped: '无需替换', rejected: '已拒绝', pending: '待处理' }
  await run({
    confirm:
      state === 'pending'
        ? undefined
        : {
            title: `标记为${labels[state]}`,
            desc: `规则 #${r.id}（${r.fromHost} → ${r.toHost}）将标记为「${labels[state]}」，不修改业务数据。`,
            okText: '确定'
          },
    action: () => manageApi.system.setDomainRuleState(r.id, state),
    successMsg: `已标记为${labels[state]}`,
    after: load
  })
}

onMounted(() => {
  void load()
  void manageApi.collect
    .list()
    .then((list) => {
      const m: Record<string, string> = {}
      for (const s of list) m[s.id] = s.name || s.id
      sourceNames.value = m
    })
    .catch(() => {
      /* 拿不到名字不影响使用：退回显示 id */
    })
})
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-4)]">
    <header class="flex flex-wrap items-end justify-between gap-[var(--jc-space-3)]">
      <div>
        <h1 class="text-lg font-[var(--jc-fw-semibold)]">域名替换规则</h1>
        <p class="mt-1 text-sm text-muted">
          按公告时间倒序；拉取只增不改（一致跳过、目标变了新增并保留历史）。执行替换仅支持封面。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
        <ManageButton variant="outline" @click="addRule">手工添加</ManageButton>
        <ManageButton @click="openFetch">立即拉取公告</ManageButton>
      </div>
    </header>

    <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
      <button
        v-for="t in tabs"
        :key="t.key"
        type="button"
        class="jc-hotarea rounded-[var(--jc-radius-md)] border px-[var(--jc-space-3)] py-[var(--jc-space-1)] text-xs"
        :class="filter === t.key ? 'border-[var(--jc-accent)] text-[var(--jc-accent)]' : 'border-default text-secondary'"
        @click="setFilter(t.key)"
      >
        {{ t.label }}
        <span class="ml-1 opacity-70">{{ tabCount(t.key) }}</span>
      </button>
      <ManageInput v-model="keyword" class="ml-auto w-[220px]" size="sm" placeholder="搜域名 / 备注" />
    </div>

    <p v-if="stale > 0" class="text-xs text-[var(--jc-danger)]">
      有 {{ stale }} 条待处理规则公告已超过 7 天，建议尽快核对（旧域名可能已失效）。
    </p>

    <ManageErrorState v-if="loadError" title="规则加载失败" @retry="load" />

    <div
      v-else-if="loading && !rows.length"
      class="grid grid-cols-1 gap-[var(--jc-space-4)] md:grid-cols-2 xl:grid-cols-3"
    >
      <div v-for="i in 6" :key="i" class="h-[168px] animate-pulse rounded-card bg-[var(--jc-bg-subtle)]" />
    </div>

    <p v-else-if="!rows.length" class="rounded-card bg-surface p-[var(--jc-space-5)] text-sm text-secondary shadow-card">
      没有匹配的规则。{{ total ? '换个筛选或关键词试试。' : '点右上角「立即拉取公告」或「手工添加」。' }}
    </p>

    <div v-else class="grid grid-cols-1 gap-[var(--jc-space-4)] md:grid-cols-2 xl:grid-cols-3">
      <article
        v-for="r in rows"
        :key="r.id"
        class="flex flex-col gap-[var(--jc-space-2)] rounded-card bg-surface p-[var(--jc-space-4)] shadow-card"
      >
        <div class="flex flex-wrap items-center gap-[var(--jc-space-2)] text-xs">
          <span class="rounded-[var(--jc-radius-sm)] bg-[var(--jc-bg-subtle)] px-[var(--jc-space-2)] py-[2px]">
            {{ r.fieldLabel || r.field }}
          </span>
          <span
            class="rounded-[var(--jc-radius-sm)] bg-[var(--jc-bg-subtle)] px-[var(--jc-space-2)] py-[2px]"
            :class="r.scope === 'slave' ? 'text-secondary' : 'text-[var(--jc-accent)]'"
          >{{ scopeText(r) }}</span>
          <span class="ml-auto" :class="stateClass(r)">{{ stateText[r.state] || r.state }}</span>
        </div>

        <p class="break-all text-sm font-[var(--jc-fw-medium)]">
          {{ r.fromHost }}
          <span class="mx-1 text-secondary">→</span>
          {{ r.toHost }}
        </p>

        <p class="flex flex-wrap items-center gap-[var(--jc-space-2)] text-xs text-secondary">
          <span>{{ r.announcedAt ? formatDateTime(r.announcedAt) : '日期未知' }}</span>
          <span v-if="r.ruleType">{{ r.ruleType }}</span>
          <span>·</span>
          <span :title="r.targetNote">{{ r.targetColumn || '本库不存储' }}</span>
          <span>·</span>
          <span>{{ sourceLabel(r) }}</span>
          <a
            v-if="sourceLink(r)"
            :href="sourceLink(r)"
            target="_blank"
            rel="noopener"
            class="jc-hotarea underline text-[var(--jc-accent)]"
          >原文</a>
        </p>

        <p v-if="r.sourceNote" class="line-clamp-2 text-xs text-secondary">{{ r.sourceNote }}</p>
        <p v-if="r.note" class="text-xs text-secondary">{{ r.note }}</p>

        <div class="mt-auto flex flex-wrap gap-[var(--jc-space-2)] pt-[var(--jc-space-2)] text-xs">
          <button type="button" class="jc-hotarea underline" @click="previewRule(r)">预览</button>
          <button type="button" class="jc-hotarea underline" @click="editRule(r)">编辑</button>
          <button
            v-if="r.supported && r.state !== 'applied'"
            type="button"
            class="jc-hotarea underline text-[var(--jc-accent)]"
            @click="applyRule(r)"
          >执行替换</button>
          <button
            v-if="r.state !== 'skipped'"
            type="button"
            class="jc-hotarea underline"
            title="库里没有这个旧域名时用它收口"
            @click="setState(r, 'skipped')"
          >无需替换</button>
          <button
            v-if="r.state !== 'rejected'"
            type="button"
            class="jc-hotarea underline text-[var(--jc-danger)]"
            @click="setState(r, 'rejected')"
          >拒绝</button>
          <button
            v-if="r.state !== 'pending' && r.state !== 'applied'"
            type="button"
            class="jc-hotarea underline text-secondary"
            @click="setState(r, 'pending')"
          >重置</button>
        </div>
      </article>
    </div>

    <div v-if="total > size" class="flex items-center justify-center gap-[var(--jc-space-3)] text-xs text-secondary">
      <ManageButton variant="outline" size="sm" :disabled="page <= 1 || loading" @click="goto(page - 1)">上一页</ManageButton>
      <span>第 {{ page }} / {{ pages }} 页 · 共 {{ total }} 条</span>
      <ManageButton variant="outline" size="sm" :disabled="page >= pages || loading" @click="goto(page + 1)">下一页</ManageButton>
    </div>

    <p v-if="busy" class="text-xs text-secondary">处理中…</p>
  </div>
</template>