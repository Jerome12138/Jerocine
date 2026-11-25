<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { openManageDialog } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import { formatDateTime } from '@/utils/format'
import DomainApplyProgressDialog from './DomainApplyProgressDialog.vue'
import DomainFetchDialog from './DomainFetchDialog.vue'
import DomainRuleFormDialog from './DomainRuleFormDialog.vue'
import DomainRulePreviewDialog from './DomainRulePreviewDialog.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import type { DomainRuleView } from '@/api/manage/system'

/**
 * 域名替换规则（macrep）—— 列表页。
 *
 * 与「影视列表」同款：ManageTable（操作列固定）+ ManageTabs + toolbar。
 * 口径：
 *  · 拉取只增不改（一致跳过、目标变了新增并保留历史）；
 *  · 封面 → movie.cover、播放 → episodes_json 可执行（带进度）；下载本库不存；
 *  · 封面只可能来自主站；播放/下载可选主站或某个从站。
 */
const rows = ref<DomainRuleView[]>([])
const loading = ref(false)
const filter = ref('')
const keyword = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const counts = ref<Record<string, number>>({})
const stale = ref(0)
const sourceNames = ref<Record<string, string>>({})

const sourceMeta: Record<string, { label: string; url: string }> = {
  lz: { label: 'lz 量子资源', url: 'https://www.lzizy.com/index.php/macrep.html' },
  manual: { label: '手工录入', url: '' }
}

const tabs = computed(() => {
  const sum = Object.values(counts.value).reduce((a, b) => a + b, 0)
  return [
    { value: '', label: '全部', count: sum },
    { value: 'pending', label: '待处理', count: counts.value.pending ?? 0 },
    { value: 'applied', label: '已执行', count: counts.value.applied ?? 0 },
    { value: 'skipped', label: '无需替换', count: counts.value.skipped ?? 0 },
    { value: 'merged', label: '已被取代', count: counts.value.merged ?? 0 },
    { value: 'rejected', label: '已拒绝', count: counts.value.rejected ?? 0 }
  ]
})

/** 列定义（派生列：类型/站点/域名/来源都是渲染列） */
const columns: Column<DomainRuleView>[] = [
  { key: 'type', label: '类型', width: '150px' },
  { key: 'site', label: '站点', width: '110px' },
  { key: 'domain', label: '域名' },
  { key: 'announcedAt', label: '公告时间', width: '140px' },
  { key: 'source', label: '来源', width: '120px' },
  { key: 'state', label: '状态', width: '96px' }
]

/** 状态词表：交给公共 StatusTag 渲染（label + variant 一处定义） */
const STATE_MAP = {
  pending: { label: '待处理', variant: 'warning' as const },
  applied: { label: '已执行', variant: 'success' as const },
  skipped: { label: '无需替换', variant: 'default' as const },
  merged: { label: '已被取代', variant: 'info' as const },
  rejected: { label: '已拒绝', variant: 'danger' as const }
}

/** 列 key 归一成 string：ManageTable 的 #cell 插槽把 key 收窄成 keyof T，直接比较派生列会报错 */
function colKey(col: { key: string }): string {
  return col.key
}

function fieldVariant(field: string): 'brand' | 'purple' | 'default' {
  if (field === 'vod_pic') return 'brand'
  if (field === 'vod_play_url') return 'purple'
  return 'default'
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

async function load(): Promise<void> {
  loading.value = true
  try {
    const r = await manageApi.system.listDomainRules({
      state: filter.value,
      keyword: keyword.value,
      page: page.value,
      size: pageSize
    })
    rows.value = r.list ?? []
    total.value = r.total ?? 0
    counts.value = r.counts ?? {}
    stale.value = r.stale ?? 0
    // 当前页被删空 → 回退一页（改 page 会触发 watch 重新加载，这里直接 return 避免并发两次）
    if (!rows.value.length && page.value > 1) {
      page.value -= 1
      return
    }
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function onTab(v: string | number): void {
  filter.value = String(v)
  if (page.value !== 1) {
    page.value = 1 // watch(page) 会触发加载
    return
  }
  void load()
}

let kwTimer: number | undefined
watch(keyword, () => {
  if (kwTimer) window.clearTimeout(kwTimer)
  kwTimer = window.setTimeout(() => {
    if (page.value !== 1) {
      page.value = 1
      return
    }
    void load()
  }, 300)
})
watch(page, () => void load())

async function openFetch(): Promise<void> {
  const ok = await openManageDialog<boolean>({
    title: '立即拉取公告',
    component: DomainFetchDialog,
    width: '520px'
  })
  if (ok) {
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
  const act = await openManageDialog<{ action: 'apply' | 'skip' | 'reject' | 'reset' }>({
    title: '影响面预览',
    component: DomainRulePreviewDialog,
    props: { rule: r },
    width: '620px'
  })
  if (!act) return
  // 弹窗里点"执行/编辑/无需替换…"都回到页面走同一套流程，做完自动刷新列表
  switch (act.action) {
    case 'apply':
      await applyRule(r)
      break
    case 'skip':
      await setState(r, 'skipped')
      break
    case 'reject':
      await setState(r, 'rejected')
      break
    case 'reset':
      await setState(r, 'pending')
      break
  }
}

const { busy, run } = useAsyncAction()

async function applyRule(r: DomainRuleView): Promise<void> {
  await run({
    confirm: {
      title: '执行替换',
      desc:
        r.field === 'vod_pic'
          ? `将把库里封面域名 ${r.fromHost} 换成 ${r.toHost}，会改动影片数据并记审计。`
          : `将把 ${scopeText(r)} 的播放地址域名 ${r.fromHost} 换成 ${r.toHost}，会改动播放源数据并记审计。`,
      okText: '开始执行',
      danger: true,
      small: true
    },
    action: () => manageApi.system.applyDomainRule(r.id, r.fromHost, r.toHost),
    successMsg: '已开始执行',
    after: async () => {
      await openManageDialog({
        title: `执行进度 · ${r.fieldLabel || r.field}`,
        component: DomainApplyProgressDialog,
        props: { rule: r },
        width: '480px'
      })
      await load()
    }
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
            okText: '确定',
            small: true
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
      /* 拿不到名字不影响使用 */
    })
})
</script>

<template>
  <ManageTable
    :columns="columns"
    :rows="rows"
    row-key="id"
    :loading="loading"
    :total="total"
    v-model:current="page"
    :page-size="pageSize"
    empty="暂无规则，点右上角「立即拉取公告」或「手工添加」"
    actions-width="268px"
    sticky-actions
  >
    <template #toolbar>
      <div class="flex flex-wrap items-center gap-[var(--jc-space-3)]">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">域名替换</h2>
        <ManageTabs :model-value="filter" :tabs="tabs" @update:model-value="onTab" />
        <BaseTag v-if="stale > 0" variant="danger" size="xs">{{ stale }} 条公告超 7 天未处理</BaseTag>
      </div>
      <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
        <div class="w-[200px]">
          <ManageInput v-model="keyword" size="sm" placeholder="搜域名 / 备注" />
        </div>
        <ManageButton variant="ghost" @click="addRule">手工添加</ManageButton>
        <ManageButton variant="gradient" @click="openFetch">立即拉取公告</ManageButton>
      </div>
    </template>

    <template #cell="{ row: r, col }">
      <template v-if="colKey(col) === 'type'">
        <div class="flex flex-col items-start gap-1">
          <BaseTag :variant="fieldVariant(r.field)" size="sm">{{ r.fieldLabel || r.field }}</BaseTag>
          <span class="text-xs text-muted">{{ r.targetColumn || '本库不存储' }}</span>
        </div>
      </template>

      <span v-else-if="colKey(col) === 'site'" class="text-xs text-muted">{{ scopeText(r) }}</span>

      <div v-else-if="colKey(col) === 'domain'">
        <p class="break-all font-[var(--jc-fw-medium)]">
          {{ r.fromHost }} <span class="text-muted">→</span> {{ r.toHost }}
        </p>
        <p v-if="r.sourceNote" class="mt-1 line-clamp-1 text-xs text-muted" :title="r.sourceNote">
          {{ r.sourceNote }}
        </p>
        <p v-if="r.note" class="mt-1 line-clamp-1 text-xs text-muted" :title="r.note">{{ r.note }}</p>
      </div>

      <span v-else-if="colKey(col) === 'announcedAt'" class="text-xs text-muted">
        {{ r.announcedAt ? formatDateTime(r.announcedAt) : '—' }}
      </span>

      <template v-else-if="colKey(col) === 'source'">
        <a
          v-if="sourceLink(r)"
          :href="sourceLink(r)"
          target="_blank"
          rel="noopener"
          class="jc-hotarea text-[var(--jc-accent)] underline"
          title="打开公告原文"
        >{{ sourceLabel(r) }}</a>
        <span v-else class="text-muted">{{ sourceLabel(r) }}</span>
      </template>

      <StatusTag v-else-if="colKey(col) === 'state'" :value="r.state" :map="STATE_MAP" />
    </template>

    <template #actions="{ row }">
      <div class="flex flex-wrap justify-end gap-[var(--jc-space-1)]">
        <ManageButton variant="ghost" size="sm" @click="previewRule(row)">预览</ManageButton>
        <ManageButton
          v-if="row.supported && row.state !== 'applied'"
          variant="ghost"
          size="sm"
          class="text-[var(--jc-accent)]"
          @click="applyRule(row)"
        >执行</ManageButton>
        <ManageButton variant="ghost" size="sm" @click="editRule(row)">编辑</ManageButton>
        <ManageButton
          v-if="row.state !== 'skipped'"
          variant="ghost"
          size="sm"
          @click="setState(row, 'skipped')"
        >无需替换</ManageButton>
        <ManageButton
          v-if="row.state !== 'rejected'"
          variant="ghost"
          size="sm"
          class="text-[var(--jc-danger)]"
          @click="setState(row, 'rejected')"
        >拒绝</ManageButton>
        <ManageButton
          v-if="row.state !== 'pending' && row.state !== 'applied'"
          variant="ghost"
          size="sm"
          @click="setState(row, 'pending')"
        >重置</ManageButton>
      </div>
    </template>
  </ManageTable>
</template>