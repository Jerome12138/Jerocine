<script setup lang="ts">
/**
 * 管理端·直播源维护：多直播源增删改/启停/排序 + 频道管理（开关/改名/增删）。
 * 后端：GET/POST /manage/live-sources，PUT/DELETE /manage/live-sources/:id
 *   GET/PUT/DELETE /manage/live-channels
 * 探源改为端侧执行——服务器无 IPv6 出口导致探测结果不可靠。
 *
 * 弹窗口径: 新增/编辑直播源(含独立校验+提交)经 openManageDialog 函数化发起;
 * 频道面板与一次性小弹窗(自定义频道/改名)保留组件式但迁 ManageSheet(移动端得 sheet
 * 形态 + ESC 弹层栈)。写操作: useAsyncAction 收敛 —— 源行级共用一实例(替代 rowBusyId
 * 互斥)、频道操作共用一实例(替代 chSaving 互斥, 保住「任一在途全行禁用」语义),
 * 请求传 {silent:true}, 文案由 run/业务分支单点。
 */
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { LiveChannelManage, LiveSourceManage } from '@/types/live'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageTable from '@/components/manage/ManageTable.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import { probeLiveChannelsLimited } from '@/utils/live'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import { openManageDialog } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import LiveSourceFormDialog from './LiveSourceFormDialog.vue'
import type { LiveSourceFormPayload } from './LiveSourceFormDialog.vue'

const loading = ref(true)
/** 加载失败信息（非空时显示错误态 + 重试，不显示空态文案） */
const error = ref('')
/** 直播顶栏入口开关 (000038): 挂载时一次读, 保存后页面不会自动刷新因此需用户返回重进;
 * 若希望刷新后同步, 可监听路由变化在此重新 load, 现阶段够用。 */
const liveNavOn = ref(true)
async function loadLiveNav(): Promise<void> {
  try {
    const v = await manageApi.live.getLiveNav()
    liveNavOn.value = v.liveNavEnabled !== false
  } catch {
    liveNavOn.value = true // 后端缺字段或异常兜底为开, 与迁移前行为一致
  }
}
// 顶栏开关独立实例: 与源行/频道操作互不阻塞
const { run: runNav } = useAsyncAction()
async function toggleLiveNav(on: boolean): Promise<void> {
  await runNav({
    action: async () => {
      await manageApi.live.updateLiveNav(on, { silent: true })
      liveNavOn.value = on
    }
  })
}
const sources = ref<LiveSourceManage[]>([])
/** 频道数预览（id → 数量；加载中/失败为 null） */
const channelCounts = ref<Record<number, number | null>>({})

/** 频道数展示文案: 停用源显示 — (公开端点 404 属预期), 加载中/失败分别提示 */
function countText(s: LiveSourceManage): string {
  if (s.state !== 0) return '—'
  const v = channelCounts.value[s.id]
  if (v === undefined) return '加载中…'
  if (v === null) return '获取失败'
  return `${v} 个频道`
}

/** 表格行: 源 + 派生「频道数」文案列 */
interface LiveSourceRow extends LiveSourceManage {
  channelCountText: string
}
const rows = computed<LiveSourceRow[]>(() =>
  sources.value.map((s) => ({ ...s, channelCountText: countText(s) }))
)

const columns: Column<LiveSourceRow>[] = [
  { key: 'name', label: '名称' },
  { key: 'url', label: '地址' },
  { key: 'remark', label: '备注', width: '160px' },
  { key: 'channelCountText', label: '频道数', width: '120px' },
  { key: 'sort', label: '排序', width: '80px' }
]

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const resp = await manageApi.live.listLiveSources()
    sources.value = resp.sources
    void loadCounts()
  } catch (e) {
    // 拦截器已 toast; 页面同时显示错误态(而非空态) + 重试
    error.value = e instanceof Error && e.message ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

/** 逐源预取频道数（编辑/启停后刷新）—— 并发 1 逐个拉，避免小机器一次性打满。
 * 停用源跳过：公开预览端点对停用源一律 404，白打请求还消耗 live 限流配额；
 * 端点语义限制——「拉取成功但 0 频道」与「拉取失败」在 200 空列表下无法区分，失败已单独显示。 */
async function loadCounts(): Promise<void> {
  for (const s of sources.value) {
    if (s.state !== 0) continue
    try {
      const resp = await manageApi.live.previewChannels(s.id)
      channelCounts.value = { ...channelCounts.value, [s.id]: resp.channels.length }
    } catch {
      channelCounts.value = { ...channelCounts.value, [s.id]: null }
    }
  }
}

/* ---------- 新增/编辑弹窗(函数化) ---------- */
async function openCreate(): Promise<void> {
  const payload = await openManageDialog<LiveSourceFormPayload>({
    title: '新增直播源',
    component: LiveSourceFormDialog,
    mobileMode: 'sheet'
  })
  if (!payload) return
  await load()
}

async function openEdit(s: LiveSourceManage): Promise<void> {
  const payload = await openManageDialog<LiveSourceFormPayload>({
    title: '编辑直播源',
    component: LiveSourceFormDialog,
    props: { initial: s },
    mobileMode: 'sheet'
  })
  if (!payload) return
  await load()
}

/** 源行级操作(停用/启用、删除)共享一实例: 互斥防重复提交 */
const { busy: rowBusy, run: runSource } = useAsyncAction()

async function toggleState(s: LiveSourceManage): Promise<void> {
  const next = s.state === 0 ? 1 : 0
  await runSource({
    action: () =>
      manageApi.live.updateLiveSource(
        s.id,
        {
          name: s.name,
          url: s.url,
          remark: s.remark,
          sort: s.sort,
          state: next
        },
        { silent: true }
      ),
    successMsg: next === 0 ? '已启用' : '已停用',
    after: () => load()
  })
}

async function remove(s: LiveSourceManage): Promise<void> {
  await runSource({
    confirm: {
      title: '确认删除直播源？',
      desc: `「${s.name}」删除后频道列表将一并失效。`,
      okText: '删除',
      danger: true
    },
    action: () => manageApi.live.deleteLiveSource(s.id, { silent: true }),
    successMsg: '已删除',
    after: () => load()
  })
}

/* ---------- 频道管理面板（按源打开） ---------- */
const chOpen = ref(false)
const chSource = ref<LiveSourceManage | null>(null)
const chLoading = ref(false)
const chList = ref<LiveChannelManage[]>([])
const chSearch = ref('')
/** 状态筛选: all=全部 / up=可用(alive≠false) / down=不可用(alive=false) / unprobed=未探测 */
const chFilterStatus = ref<'all' | 'up' | 'down' | 'unprobed'>('all')
const CH_STATUS_OPTIONS = [
  { value: 'all', label: '全部状态' },
  { value: 'up', label: '可用' },
  { value: 'down', label: '不可用' },
  { value: 'unprobed', label: '未探测' }
]
/** 自定义频道新增弹窗 */
const customOpen = ref(false)
const customForm = reactive({ name: '', groupName: '', url: '' })
/** 频道改名弹窗（解析频道 display_name 覆盖） */
const renameOpen = ref(false)
const renameTarget = ref<LiveChannelManage | null>(null)
const renameForm = reactive({ displayName: '' })

const chFiltered = computed(() => {
  const kw = chSearch.value.trim().toLowerCase()
  const status = chFilterStatus.value
  return chList.value.filter((c) => {
    if (status === 'up' && c.alive === false) return false
    if (status === 'down' && c.alive !== false) return false
    if (status === 'unprobed' && c.alive !== undefined) return false
    if (!kw) return true
    return c.name.toLowerCase().includes(kw) || c.group.toLowerCase().includes(kw) || c.url.toLowerCase().includes(kw)
  })
})

/** 频道列表前端分页：m3u 源动辄上千频道，整表渲染会卡死弹窗（先过滤再分页） */
const CH_PAGE_SIZE = 100
const chPage = ref(1)
const chPaged = computed(() => {
  const start = (chPage.value - 1) * CH_PAGE_SIZE
  return chFiltered.value.slice(start, start + CH_PAGE_SIZE)
})
watch([chSearch, chFilterStatus], () => {
  chPage.value = 1
})

async function openChannels(s: LiveSourceManage): Promise<void> {
  chSource.value = s
  chOpen.value = true
  chSearch.value = ''
  chPage.value = 1
  await loadChannels(s.id)
}

async function loadChannels(id: number): Promise<void> {
  chLoading.value = true
  try {
    const resp = await manageApi.live.listLiveChannels(id)
    chList.value = resp.channels
  } catch {
    /* silent 请求 + run 之外的手拉: 失败由拦截器统一提示 */
  } finally {
    chLoading.value = false
  }
}

/** 频道操作(开关/改名/自定义/删除)共享一实例: 任一在途全部行按钮禁用, 防重复提交 */
const { busy: chBusy, run: runCh } = useAsyncAction()

/* ---------- 频道可访问性探测(端侧) ----------
 * 用本机浏览器网络逐个探测当前(过滤后)频道; 低频限流(固定小并发 + 请求间隔),
 * 避免瞬间大量连接被 IPTV 源站限流/封禁。结果写回行对象, 供角标渐进点亮。 */
const chProbing = ref(false)
const chProbeDone = ref(0)
const PROBE_CONCURRENCY = 3
const PROBE_TIMEOUT_MS = 6000
const PROBE_DELAY_MS = 150

async function probeChannels(): Promise<void> {
  if (chProbing.value) return
  const targets = chFiltered.value.filter((c) => !!c.url)
  if (!targets.length) {
    toast('info', '没有可探测的频道')
    return
  }
  const byUrl = new Map(targets.map((c) => [c.url, c] as const))
  chProbing.value = true
  chProbeDone.value = 0
  try {
    const results = await probeLiveChannelsLimited(
      targets.map((c) => c.url),
      PROBE_CONCURRENCY,
      PROBE_TIMEOUT_MS,
      fetch,
      (r) => {
        const c = byUrl.get(r.url)
        if (c) {
          c.alive = r.alive
          c.cors = r.cors
        }
        chProbeDone.value++
      },
      PROBE_DELAY_MS
    )
    const ok = results.filter((r) => r.alive && r.cors).length
    toast('success', `探测完成：${results.length} 个频道，可用 ${ok} 个`)
  } finally {
    chProbing.value = false
  }
}

/* ---------- 一键隐藏不可用频道 ----------
 * 基于探测结果(alive===false)批量隐藏：**一次批量接口**提交 urls（不再逐条打接口）。
 * 语义：解析频道=覆盖层置 hidden（可逆）、自定义频道同样只隐藏不删除。 */
const chHideDone = ref(0)
const unavailableCount = computed(
  () => chFiltered.value.filter((c) => c.alive === false && !c.hidden).length
)

async function hideUnavailable(): Promise<void> {
  if (!chSource.value) return
  const targets = chFiltered.value.filter((c) => c.alive === false && !c.hidden)
  if (!targets.length) return
  chHideDone.value = 0
  await runCh({
    confirm: {
      title: `隐藏 ${targets.length} 个不可用频道？`,
      desc: '把探测判定为「离线」的频道从公开列表隐藏（可逆：列表里点「显示」还原；自定义频道同样只隐藏、不删除）。',
      okText: '一键隐藏'
    },
    action: async () => {
      const res = await manageApi.live.hideLiveChannelsBatch(
        chSource.value!.id,
        targets.map((c) => c.url),
        true,
        { silent: true }
      )
      targets.forEach((c) => {
        c.hidden = true
      })
      chHideDone.value = res.affected ?? targets.length
    },
    successMsg: `已隐藏 ${targets.length} 个不可用频道`,
    after: () => loadChannels(chSource.value!.id)
  })
}

/** 单频道探源：只探这一条（低频单请求），结果写回该行 */
async function probeOne(c: LiveChannelManage): Promise<void> {
  if (!c.url || chProbing.value) return
  const results = await probeLiveChannelsLimited([c.url], 1, PROBE_TIMEOUT_MS, fetch, undefined, 0)
  const r = results[0]
  if (!r) return
  c.alive = r.alive
  c.cors = r.cors
}

/** 按源重新拉取频道：服务端清缓存重抓 m3u；内容有更新的隐藏频道自动恢复显示 */
async function refreshChannels(): Promise<void> {
  if (!chSource.value) return
  await runCh({
    confirm: {
      title: '重新拉取该源的频道数据？',
      desc: '会重新抓取该源的 m3u 并解析：内容有更新的隐藏频道会自动恢复显示。',
      okText: '重新拉取'
    },
    action: async () => {
      const res = await manageApi.live.refreshLiveChannels(chSource.value!.id, { silent: true })
      toast('success', `已重新拉取：共 ${res.total} 个频道，恢复显示 ${res.restored} 个`)
    },
    after: () => loadChannels(chSource.value!.id)
  })
}

/** 开关：解析频道与自定义频道都走 upsert（hidden 翻转，其余字段保留） */
async function toggleChannel(c: LiveChannelManage): Promise<void> {
  if (!chSource.value) return
  await runCh({
    action: async () => {
      await manageApi.live.upsertLiveChannel(
        {
          sourceId: chSource.value!.id,
          url: c.url,
          name: c.isCustom ? c.name : undefined,
          groupName: c.isCustom ? c.group : undefined,
          hidden: !c.hidden,
          displayName: c.isCustom ? undefined : c.displayName
        },
        { silent: true }
      )
      c.hidden = !c.hidden
      toast('success', c.hidden ? '已隐藏' : '已显示')
    }
  })
}

/** 改名（解析频道：display_name 覆盖; 自定义频道：直接改 name） */
function openRename(c: LiveChannelManage): void {
  renameTarget.value = c
  renameForm.displayName = c.isCustom ? c.name : c.displayName || c.name
  renameOpen.value = true
}

async function saveRename(): Promise<void> {
  if (!chSource.value || !renameTarget.value) return
  const name = renameForm.displayName.trim()
  const target = renameTarget.value
  const isCustom = target.isCustom
  if (isCustom && !name) {
    toast('warning', '自定义频道名不能为空')
    return
  }
  await runCh({
    action: () =>
      manageApi.live.upsertLiveChannel(
        {
          sourceId: chSource.value!.id,
          url: target.url,
          // upsert 是整行覆盖写：必须带回当前 hidden，否则改名会把已隐藏频道重置为显示
          hidden: target.hidden,
          // 自定义频道没有 displayName 语义（服务端要求 name 必填），改名走 name；
          // groupName 一并带回，避免整行覆盖把分组清空。
          name: isCustom ? name : undefined,
          groupName: isCustom ? target.group : undefined,
          displayName: isCustom ? undefined : name
        },
        { silent: true }
      ),
    successMsg: isCustom ? '频道名已保存' : '显示名已保存',
    after: () => {
      renameOpen.value = false
      return loadChannels(chSource.value!.id)
    }
  })
}

/** 自定义频道新增 */
function openCustom(): void {
  customForm.name = ''
  customForm.groupName = ''
  customForm.url = ''
  customOpen.value = true
}

async function saveCustom(): Promise<void> {
  if (!chSource.value) return
  if (!customForm.name.trim() || !customForm.url.trim()) {
    toast('warning', '频道名与地址必填')
    return
  }
  const lu = customForm.url.trim().toLowerCase()
  if (!lu.startsWith('http://') && !lu.startsWith('https://')) {
    toast('warning', '地址需以 http(s):// 开头')
    return
  }
  await runCh({
    action: () =>
      manageApi.live.upsertLiveChannel(
        {
          sourceId: chSource.value!.id,
          url: customForm.url.trim(),
          name: customForm.name.trim(),
          groupName: customForm.groupName.trim()
        },
        { silent: true }
      ),
    successMsg: '自定义频道已添加',
    after: () => {
      customOpen.value = false
      return loadChannels(chSource.value!.id)
    }
  })
}

/** 删除频道：自定义物理删 / 解析频道置 hidden（服务端自动分流） */
async function removeChannel(c: LiveChannelManage): Promise<void> {
  if (!chSource.value) return
  await runCh({
    confirm: {
      title: c.isCustom ? '确认删除自定义频道？' : '确认移除该频道？',
      desc: c.isCustom ? `「${c.name}」将被物理删除。` : `「${c.name}」将从列表移除（解析频道=置为隐藏，源重解析后仍保持隐藏）。`,
      okText: c.isCustom ? '删除' : '移除',
      danger: true
    },
    action: () => manageApi.live.deleteLiveChannel(chSource.value!.id, c.url, { silent: true }),
    successMsg: '已删除',
    after: () => loadChannels(chSource.value!.id)
  })
}

onMounted(async () => {
  await Promise.all([load(), loadLiveNav()])
})
</script>

<template>
  <div class="jc-livemgr flex flex-1 min-h-0 flex-col">
    <header class="jc-livemgr__head shrink-0">
      <div>
        <h1 class="jc-livemgr__title">直播源</h1>
        <p class="jc-livemgr__desc">维护直播页的 m3u 播放列表源；保存/停用后公开频道列表立即刷新。</p>
      </div>
      <div class="flex items-center gap-[var(--jc-space-3)]">
        <label class="jc-livemgr__toggle-label" :title="`直播顶栏入口${liveNavOn ? '（已开）' : '（已关）'}: 仅控制 web/TV 顶部导航是否显示「直播」; 侧边栏固定显示, 不受此开关控制`">
          <span class="jc-livemgr__toggle-state" :class="{ 'jc-livemgr__toggle-state--on': liveNavOn }">
            {{ liveNavOn ? '顶栏显示' : '顶栏隐藏' }}
          </span>
          <ManageSwitch
            :model-value="liveNavOn"
            :disabled="loading"
            @update:model-value="toggleLiveNav"
          />
        </label>
        <ManageButton variant="gradient" @click="openCreate">
          <BaseIcon name="plus" size="16px" />
          新增直播源
        </ManageButton>
      </div>
    </header>

    <!-- 加载失败错误态: 收口 ManageErrorState plain 形态(§6 组合组件), 视觉沿用原 jc-livemgr__error -->
    <ManageErrorState
      v-if="error"
      variant="plain"
      title="直播源加载失败"
      :description="error"
      @retry="load"
    />

    <ManageTable
      v-else
      :columns="columns"
      :rows="rows"
      row-key="id"
      :loading="loading"
      empty="还没有直播源，点击右上角「新增直播源」添加第一个。"
      actions-width="230px"
    >
      <template #cell="{ row, col }">
        <div v-if="col.key === 'name'" class="flex items-center gap-[var(--jc-space-2)] min-w-0">
          <span class="truncate font-[var(--jc-fw-medium)]">{{ row.name }}</span>
          <span class="jc-livemgr__badge shrink-0" :class="row.state === 0 ? 'is-on' : 'is-off'">
            {{ row.state === 0 ? '启用中' : '已停用' }}
          </span>
        </div>
        <span
          v-else-if="col.key === 'url'"
          class="text-secondary text-xs truncate block max-w-[340px]"
          :title="row.url"
        >{{ row.url }}</span>
        <span
          v-else-if="col.key === 'remark'"
          class="text-secondary text-xs truncate block max-w-[160px]"
          :title="row.remark"
        >{{ row.remark || '—' }}</span>
        <span v-else-if="col.key === 'channelCountText'" class="text-secondary text-xs">{{ row.channelCountText }}</span>
        <span v-else>{{ row[col.key] }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end flex-wrap">
          <ManageButton variant="ghost" size="sm" @click="openChannels(row)">频道管理</ManageButton>
          <ManageButton variant="ghost" size="sm" @click="openEdit(row)">编辑</ManageButton>
          <ManageButton variant="ghost" size="sm" :disabled="rowBusy" @click="toggleState(row)">
            {{ row.state === 0 ? '停用' : '启用' }}
          </ManageButton>
          <ManageButton variant="ghost" size="sm" class="jc-livemgr__danger" :disabled="rowBusy" @click="remove(row)">
            删除
          </ManageButton>
        </div>
      </template>

      <template #mobile-card="{ row }">
        <div class="flex flex-col gap-[var(--jc-space-2)]">
          <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap">
            <span class="font-[var(--jc-fw-medium)]">{{ row.name }}</span>
            <span class="jc-livemgr__badge" :class="row.state === 0 ? 'is-on' : 'is-off'">
              {{ row.state === 0 ? '启用中' : '已停用' }}
            </span>
          </div>
          <span class="text-secondary text-xs break-all">{{ row.url }}</span>
          <span v-if="row.remark" class="text-secondary text-xs">{{ row.remark }}</span>
          <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap text-xs text-secondary">
            <span>{{ row.channelCountText }}</span>
            <span>排序 {{ row.sort }}</span>
          </div>
          <div class="flex gap-[var(--jc-space-2)] flex-wrap justify-end border-t border-subtle pt-[var(--jc-space-2)]">
            <ManageButton variant="ghost" size="sm" @click="openChannels(row)">频道管理</ManageButton>
            <ManageButton variant="ghost" size="sm" @click="openEdit(row)">编辑</ManageButton>
            <ManageButton variant="ghost" size="sm" :disabled="rowBusy" @click="toggleState(row)">
              {{ row.state === 0 ? '停用' : '启用' }}
            </ManageButton>
            <ManageButton variant="ghost" size="sm" class="jc-livemgr__danger" :disabled="rowBusy" @click="remove(row)">
              删除
            </ManageButton>
          </div>
        </div>
      </template>
    </ManageTable>

    <!-- 频道管理面板(组件式 ManageSheet: 复杂管理面, 非表单弹窗) -->
    <ManageSheet v-model="chOpen" :title="`频道管理 · ${chSource?.name ?? ''}`" width="1120px" mobile-mode="fullsheet">
      <div class="jc-chmgr">
        <div class="jc-chmgr__toolbar">
          <ManageInput v-model="chSearch" placeholder="搜索频道名/分组/地址" size="sm" class="jc-chmgr__search" />
          <div class="w-[150px] shrink-0">
            <!-- size=sm: 与同行一排 outline sm 按钮同高(规格 §3.6/D⑥) -->
            <ManageSelect v-model="chFilterStatus" :options="CH_STATUS_OPTIONS" aria-label="状态筛选" size="sm" />
          </div>
          <ManageButton variant="outline" size="sm" @click="openCustom">
            <BaseIcon name="plus" size="14px" />
            添加自定义频道
          </ManageButton>
          <ManageButton variant="outline" size="sm" :disabled="chBusy || chProbing" @click="refreshChannels">
            <BaseIcon name="refresh" size="14px" />
            重新拉取频道
          </ManageButton>
          <ManageButton variant="outline" size="sm" :loading="chProbing" :disabled="chProbing" @click="probeChannels">
            <BaseIcon name="magic" size="14px" />
            探测可访问性
          </ManageButton>
          <ManageButton
            variant="outline"
            size="sm"
            :disabled="chBusy || chProbing || unavailableCount === 0"
            @click="hideUnavailable"
          >
            <BaseIcon name="eye-off" size="14px" />
            隐藏不可用 ({{ unavailableCount }})
          </ManageButton>
        </div>
        <p class="jc-chmgr__hint">
          共 {{ chList.length }} 个频道。
          <template v-if="chProbing">
            正在用本机网络探测 {{ chProbeDone }}/{{ chFiltered.length }}（限流 {{ PROBE_CONCURRENCY }} 并发 · 每个间隔 {{ PROBE_DELAY_MS }}ms，避免被源站屏蔽）…
          </template>
          <template v-else-if="chBusy && chHideDone > 0">正在隐藏不可用频道 {{ chHideDone }} 个…</template>
          <template v-else>先点「探测可访问性」判活，再「隐藏不可用」一键隐藏离线频道（均可逆）。</template>
        </p>
        <BaseSkeleton v-if="chLoading" variant="rect" class="jc-chmgr__skeleton" />
        <template v-else>
          <div class="jc-chmgr__list">
            <div v-for="c in chPaged" :key="c.url" class="jc-chmgr__item" :class="{ 'is-hidden': c.hidden }">
              <div class="jc-chmgr__info">
                <div class="jc-chmgr__name">
                  {{ c.name }}
                  <span v-if="c.isCustom" class="jc-chmgr__tag">自定义</span>
                  <span v-if="c.hidden" class="jc-chmgr__tag is-off">已隐藏</span>
                  <span v-if="c.alive === false" class="jc-chmgr__tag is-off">离线</span>
                  <span v-if="c.cors === false" class="jc-chmgr__tag is-off">跨域受限</span>
                  <span v-if="c.cors === true && c.alive === true" class="jc-chmgr__tag is-ok">可用</span>
                </div>
                <div class="jc-chmgr__url">{{ c.url }}</div>
              </div>
              <div class="jc-chmgr__actions">
                <ManageButton variant="ghost" size="sm" :disabled="chProbing" @click="probeOne(c)">探测</ManageButton>
                <ManageButton variant="ghost" size="sm" :disabled="chBusy" @click="toggleChannel(c)">
                  {{ c.hidden ? '显示' : '隐藏' }}
                </ManageButton>
                <ManageButton variant="ghost" size="sm" :disabled="chBusy" @click="openRename(c)">改名</ManageButton>
                <ManageButton variant="ghost" size="sm" class="jc-livemgr__danger" :disabled="chBusy" @click="removeChannel(c)">
                  移除
                </ManageButton>
              </div>
            </div>
            <p v-if="chFiltered.length === 0" class="jc-chmgr__empty">无匹配频道</p>
          </div>
          <!-- 超过一页才出现分页; 搜索/翻页均前端内存过滤, 不打接口 -->
          <BasePagination
            v-if="chFiltered.length > CH_PAGE_SIZE"
            :current="chPage"
            :page-size="CH_PAGE_SIZE"
            :total="chFiltered.length"
            @update:current="chPage = $event"
          />
        </template>
      </div>
      <template #footer>
        <ManageButton variant="ghost" @click="chOpen = false">关闭</ManageButton>
      </template>
    </ManageSheet>

    <!-- 自定义频道新增(一次性内联小弹窗 → 组件式 ManageSheet) -->
    <ManageSheet v-model="customOpen" title="添加自定义频道" width="480px">
      <div class="flex flex-col gap-[var(--jc-space-3)]">
        <ManageFormField label="频道名" required>
          <ManageInput v-model="customForm.name" placeholder="如：CCTV-1 综合" :maxlength="255" />
        </ManageFormField>
        <ManageFormField label="分组" hint="留空归入「自定义」">
          <ManageInput v-model="customForm.groupName" placeholder="如：央视" :maxlength="128" />
        </ManageFormField>
        <ManageFormField label="播放地址" required>
          <ManageInput v-model="customForm.url" placeholder="http(s):// 的 m3u8 / ts 流地址" :maxlength="1024" />
        </ManageFormField>
      </div>
      <template #footer>
        <ManageButton variant="ghost" @click="customOpen = false">取消</ManageButton>
        <ManageButton variant="gradient" :loading="chBusy" @click="saveCustom">添加</ManageButton>
      </template>
    </ManageSheet>

    <!-- 频道改名（解析频道 display_name 覆盖; 自定义频道改 name） -->
    <ManageSheet
      v-model="renameOpen"
      :title="renameTarget?.isCustom ? `改名 · ${renameTarget?.name ?? ''}` : `显示名 · ${renameTarget?.name ?? ''}`"
      width="420px"
    >
      <div class="flex flex-col gap-[var(--jc-space-3)]">
        <ManageFormField
          :label="renameTarget?.isCustom ? '频道名' : '显示名'"
          :hint="renameTarget?.isCustom ? '' : '留空 = 恢复解析名'"
          :required="renameTarget?.isCustom"
        >
          <ManageInput v-model="renameForm.displayName" :maxlength="255" />
        </ManageFormField>
      </div>
      <template #footer>
        <ManageButton variant="ghost" @click="renameOpen = false">取消</ManageButton>
        <ManageButton variant="gradient" :loading="chBusy" @click="saveRename">保存</ManageButton>
      </template>
    </ManageSheet>
  </div>
</template>

<style scoped>
.jc-livemgr__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--jc-space-4);
  margin-bottom: var(--jc-space-5);
}
.jc-livemgr__title {
  font-size: var(--jc-fs-lg);
  font-weight: var(--jc-fw-bold);
}
.jc-livemgr__desc {
  margin-top: 4px;
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-secondary);
}
.jc-livemgr__skeleton {
  height: 240px;
}
.jc-livemgr__empty {
  padding: var(--jc-space-8) var(--jc-space-4);
  text-align: center;
  color: var(--jc-text-secondary);
  background-color: var(--jc-bg-surface);
  border-radius: var(--jc-radius-md);
}
/* 加载失败错误块样式已收口 ManageErrorState plain 形态(§6 组合组件) */
.jc-livemgr__list {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-3);
}
.jc-livemgr__item {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--jc-space-4);
  padding: var(--jc-space-4);
  background-color: var(--jc-bg-surface);
  border-radius: var(--jc-radius-md);
}
.jc-livemgr__info {
  flex: 1 1 260px;
  min-width: 0;
}
.jc-livemgr__name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: var(--jc-fw-semibold);
}
.jc-livemgr__badge {
  font-size: var(--jc-fs-xs);
  padding: 2px 8px;
  border-radius: 999px;
}
.jc-livemgr__badge.is-on {
  color: #10b981;
  background-color: rgba(16, 185, 129, 0.12);
}
.jc-livemgr__badge.is-off {
  color: var(--jc-text-secondary);
  background-color: rgba(148, 163, 184, 0.15);
}
.jc-livemgr__url {
  margin-top: 4px;
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.jc-livemgr__remark {
  margin-top: 2px;
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-tertiary, var(--jc-text-secondary));
}
.jc-livemgr__meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
}
.jc-livemgr__actions {
  display: flex;
  gap: var(--jc-space-2);
}
.jc-livemgr__danger {
  color: var(--jc-danger, #ef4444);
}

/* ---------- 频道管理面板 ---------- */
.jc-chmgr__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--jc-space-2);
  margin-bottom: var(--jc-space-2);
}
.jc-chmgr__search {
  flex: 1 1 200px;
}
.jc-chmgr__hint {
  margin-bottom: var(--jc-space-3);
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
}
.jc-chmgr__skeleton {
  height: 300px;
}
.jc-chmgr__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 46vh;
  overflow-y: auto;
  padding-right: 4px;
}
.jc-chmgr__item {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  padding: 8px 12px;
  background-color: var(--jc-bg-surface);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-sm);
}
.jc-chmgr__item.is-hidden {
  opacity: 0.55;
}
.jc-chmgr__info {
  flex: 1 1 auto;
  min-width: 0;
}
.jc-chmgr__name {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-semibold);
}
.jc-chmgr__tag {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 999px;
  font-weight: var(--jc-fw-normal);
}
.jc-chmgr__tag.is-off {
  color: var(--jc-text-secondary);
  background: rgba(148, 163, 184, 0.15);
}
.jc-chmgr__tag.is-ok {
  color: #10b981;
  background: rgba(16, 185, 129, 0.12);
}
.jc-chmgr__url {
  margin-top: 2px;
  font-size: 11px;
  color: var(--jc-text-tertiary, var(--jc-text-secondary));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.jc-chmgr__actions {
  display: flex;
  flex-shrink: 0;
  gap: 4px;
}
.jc-chmgr__empty {
  padding: var(--jc-space-6);
  text-align: center;
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
}

/* 直播顶栏开关(000038) */
.jc-livemgr__toggle-label {
  display: inline-flex;
  align-items: center;
  gap: var(--jc-space-2);
  cursor: pointer;
  font-size: var(--jc-fs-sm);
}
.jc-livemgr__toggle-state {
  color: var(--jc-text-muted);
  min-width: 3.6em;
  text-align: right;
  user-select: none;
}
.jc-livemgr__toggle-state--on {
  color: var(--jc-brand-purple);
  font-weight: var(--jc-fw-medium);
}
</style>
