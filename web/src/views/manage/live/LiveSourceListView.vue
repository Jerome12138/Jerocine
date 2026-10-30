<script setup lang="ts">
/**
 * 管理端·直播源维护：多直播源增删改/启停/排序 + 频道管理（开关/改名/增删/探源）。
 * 后端：GET/POST /manage/live-sources，PUT/DELETE /manage/live-sources/:id
 *   GET/PUT/DELETE /manage/live-channels，POST /manage/live-channels/probe
 * （改/删服务端自动清该源频道缓存，公开端点立即生效）。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { LiveChannelManage, LiveSourceManage } from '@/types/live'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseDialog from '@/components/base/BaseDialog.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'

const loading = ref(true)
const sources = ref<LiveSourceManage[]>([])
/** 频道数预览（id → 数量；加载中/失败为 null） */
const channelCounts = ref<Record<number, number | null>>({})

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await manageApi.live.listLiveSources()
    sources.value = resp.sources
    void loadCounts()
  } catch {
    /* 拦截器已 toast */
  } finally {
    loading.value = false
  }
}

/** 逐源预取频道数（编辑/启停后刷新）—— 并发 1 逐个拉，避免小机器一次性打满 */
async function loadCounts(): Promise<void> {
  for (const s of sources.value) {
    try {
      const resp = await manageApi.live.previewChannels(s.id)
      channelCounts.value = { ...channelCounts.value, [s.id]: resp.channels.length }
    } catch {
      channelCounts.value = { ...channelCounts.value, [s.id]: null }
    }
  }
}

/* ---------- 编辑弹窗 ---------- */
const dialogOpen = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null) // null = 新增
const form = reactive({
  name: '',
  url: '',
  remark: '',
  sort: 0,
  state: 0
})

function openCreate(): void {
  editingId.value = null
  form.name = ''
  form.url = ''
  form.remark = ''
  form.sort = 0
  form.state = 0
  dialogOpen.value = true
}

function openEdit(s: LiveSourceManage): void {
  editingId.value = s.id
  form.name = s.name
  form.url = s.url
  form.remark = s.remark
  form.sort = s.sort
  form.state = s.state
  dialogOpen.value = true
}

async function save(): Promise<void> {
  if (!form.name.trim() || !form.url.trim()) {
    toast('warning', '名称与地址必填')
    return
  }
  const lu = form.url.trim().toLowerCase()
  if (!lu.startsWith('http://') && !lu.startsWith('https://')) {
    toast('warning', '地址需以 http(s):// 开头')
    return
  }
  saving.value = true
  try {
    const payload = { ...form, name: form.name.trim(), url: form.url.trim() }
    if (editingId.value === null) {
      await manageApi.live.createLiveSource(payload)
      toast('success', '直播源已添加')
    } else {
      await manageApi.live.updateLiveSource(editingId.value, payload)
      toast('success', '直播源已保存')
    }
    dialogOpen.value = false
    await load()
  } catch {
    /* 拦截器已 toast */
  } finally {
    saving.value = false
  }
}

async function toggleState(s: LiveSourceManage): Promise<void> {
  const next = s.state === 0 ? 1 : 0
  try {
    await manageApi.live.updateLiveSource(s.id, {
      name: s.name,
      url: s.url,
      remark: s.remark,
      sort: s.sort,
      state: next
    })
    toast('success', next === 0 ? '已启用' : '已停用')
    await load()
  } catch {
    /* 拦截器已 toast */
  }
}

async function remove(s: LiveSourceManage): Promise<void> {
  if (!window.confirm(`确认删除直播源「${s.name}」？频道列表将一并失效。`)) return
  try {
    await manageApi.live.deleteLiveSource(s.id)
    toast('success', '已删除')
    await load()
  } catch {
    /* 拦截器已 toast */
  }
}

/* ---------- 频道管理面板（按源打开） ---------- */
const chOpen = ref(false)
const chSource = ref<LiveSourceManage | null>(null)
const chLoading = ref(false)
const chList = ref<LiveChannelManage[]>([])
const chSearch = ref('')
const chSaving = ref(false)
/** 探源进行中（批量最坏 ≈50s） */
const probing = ref(false)
/** 自定义频道新增弹窗 */
const customOpen = ref(false)
const customForm = reactive({ name: '', groupName: '', url: '' })
/** 频道改名弹窗（解析频道 display_name 覆盖） */
const renameOpen = ref(false)
const renameTarget = ref<LiveChannelManage | null>(null)
const renameForm = reactive({ displayName: '' })

const chFiltered = computed(() => {
  const kw = chSearch.value.trim().toLowerCase()
  if (!kw) return chList.value
  return chList.value.filter(
    (c) => c.name.toLowerCase().includes(kw) || c.group.toLowerCase().includes(kw) || c.url.toLowerCase().includes(kw)
  )
})
/** 探源统计（最近一次结果从列表标记聚合） */
const chProbeStats = computed(() => {
  const probed = chList.value.filter((c) => c.alive !== undefined && c.alive !== null)
  const ok = probed.filter((c) => c.alive && c.cors).length
  return { total: chList.value.length, probed: probed.length, ok }
})

async function openChannels(s: LiveSourceManage): Promise<void> {
  chSource.value = s
  chOpen.value = true
  chSearch.value = ''
  await loadChannels(s.id)
}

async function loadChannels(id: number): Promise<void> {
  chLoading.value = true
  try {
    const resp = await manageApi.live.listLiveChannels(id)
    chList.value = resp.channels
  } catch {
    /* 拦截器已 toast */
  } finally {
    chLoading.value = false
  }
}

/** 开关：解析频道与自定义频道都走 upsert（hidden 翻转，其余字段保留） */
async function toggleChannel(c: LiveChannelManage): Promise<void> {
  if (!chSource.value) return
  chSaving.value = true
  try {
    await manageApi.live.upsertLiveChannel({
      sourceId: chSource.value.id,
      url: c.url,
      name: c.isCustom ? c.name : undefined,
      groupName: c.isCustom ? c.group : undefined,
      hidden: !c.hidden,
      displayName: c.isCustom ? undefined : c.displayName
    })
    c.hidden = !c.hidden
    toast('success', c.hidden ? '已隐藏' : '已显示')
  } catch {
    /* 拦截器已 toast */
  } finally {
    chSaving.value = false
  }
}

/** 改名（解析频道：display_name 覆盖） */
function openRename(c: LiveChannelManage): void {
  renameTarget.value = c
  renameForm.displayName = c.displayName || (c.isCustom ? '' : c.name)
  renameOpen.value = true
}

async function saveRename(): Promise<void> {
  if (!chSource.value || !renameTarget.value) return
  chSaving.value = true
  try {
    await manageApi.live.upsertLiveChannel({
      sourceId: chSource.value.id,
      url: renameTarget.value.url,
      displayName: renameForm.displayName.trim()
    })
    toast('success', '显示名已保存')
    renameOpen.value = false
    await loadChannels(chSource.value.id)
  } catch {
    /* 拦截器已 toast */
  } finally {
    chSaving.value = false
  }
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
  chSaving.value = true
  try {
    await manageApi.live.upsertLiveChannel({
      sourceId: chSource.value.id,
      url: customForm.url.trim(),
      name: customForm.name.trim(),
      groupName: customForm.groupName.trim()
    })
    toast('success', '自定义频道已添加')
    customOpen.value = false
    await loadChannels(chSource.value.id)
  } catch {
    /* 拦截器已 toast */
  } finally {
    chSaving.value = false
  }
}

/** 删除频道：自定义物理删 / 解析频道置 hidden（服务端自动分流） */
async function removeChannel(c: LiveChannelManage): Promise<void> {
  if (!chSource.value) return
  const tip = c.isCustom ? '确认删除自定义频道' : '确认隐藏频道（源重解析后仍保持隐藏）'
  if (!window.confirm(`${tip}「${c.name}」？`)) return
  try {
    await manageApi.live.deleteLiveChannel(chSource.value.id, c.url)
    toast('success', '已删除')
    await loadChannels(chSource.value.id)
  } catch {
    /* 拦截器已 toast */
  }
}

/** 批量探源（CORS/活性）：结果写服务端缓存 → 前台频道列表带置灰标记 */
async function probe(): Promise<void> {
  if (!chSource.value || probing.value) return
  probing.value = true
  try {
    toast('info', `开始探测 ${chList.value.length} 个频道，约需 1 分钟…`)
    const resp = await manageApi.live.probeLiveChannels(chSource.value.id)
    const okCors = resp.results.filter((r) => r.cors).length
    const okAlive = resp.results.filter((r) => r.alive).length
    toast('success', `探测完成：${okAlive} 可达 / ${okCors} 可跨域（共 ${resp.results.length}）`)
    await loadChannels(chSource.value.id)
  } catch {
    /* 拦截器已 toast（含超时） */
  } finally {
    probing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="jc-livemgr">
    <header class="jc-livemgr__head">
      <div>
        <h1 class="jc-livemgr__title">直播源</h1>
        <p class="jc-livemgr__desc">维护直播页的 m3u 播放列表源；保存/停用后公开频道列表立即刷新。</p>
      </div>
      <BaseButton variant="gradient" @click="openCreate">
        <BaseIcon name="plus" size="16px" />
        新增直播源
      </BaseButton>
    </header>

    <BaseSkeleton v-if="loading" variant="rect" class="jc-livemgr__skeleton" />

    <div v-else-if="sources.length === 0" class="jc-livemgr__empty">还没有直播源，点击右上角「新增直播源」添加第一个。</div>

    <div v-else class="jc-livemgr__list">
      <div v-for="s in sources" :key="s.id" class="jc-livemgr__item">
        <div class="jc-livemgr__info">
          <div class="jc-livemgr__name">
            {{ s.name }}
            <span class="jc-livemgr__badge" :class="s.state === 0 ? 'is-on' : 'is-off'">
              {{ s.state === 0 ? '启用中' : '已停用' }}
            </span>
          </div>
          <div class="jc-livemgr__url">{{ s.url }}</div>
          <div v-if="s.remark" class="jc-livemgr__remark">{{ s.remark }}</div>
        </div>
        <div class="jc-livemgr__meta">
          <span class="jc-livemgr__count">
            <!-- 停用源公开端点 404 是预期, 显示 — 而非"获取失败" -->
            <template v-if="s.state !== 0">—</template>
            <template v-else-if="channelCounts[s.id] === undefined">频道数加载中…</template>
            <template v-else-if="channelCounts[s.id] === null">频道数获取失败</template>
            <template v-else>{{ channelCounts[s.id] }} 个频道</template>
          </span>
          <span class="jc-livemgr__sort">排序 {{ s.sort }}</span>
        </div>
        <div class="jc-livemgr__actions">
          <BaseButton variant="ghost" size="sm" @click="openChannels(s)">频道管理</BaseButton>
          <BaseButton variant="ghost" size="sm" @click="openEdit(s)">编辑</BaseButton>
          <BaseButton variant="ghost" size="sm" @click="toggleState(s)">{{ s.state === 0 ? '停用' : '启用' }}</BaseButton>
          <BaseButton variant="ghost" size="sm" class="jc-livemgr__danger" @click="remove(s)">删除</BaseButton>
        </div>
      </div>
    </div>

    <BaseDialog v-model:visible="dialogOpen" :title="editingId === null ? '新增直播源' : '编辑直播源'" width="480px">
      <div class="jc-livemgr__form">
        <ManageFormField label="名称" required>
          <ManageInput v-model="form.name" placeholder="如： IPTV 主源" :maxlength="64" />
        </ManageFormField>
        <ManageFormField label="m3u 播放列表地址" required>
          <ManageInput v-model="form.url" placeholder="http(s):// 开头的 .m3u / .m3u8 地址" :maxlength="1024" />
        </ManageFormField>
        <ManageFormField label="备注">
          <ManageTextarea v-model="form.remark" placeholder="可选；记录来源/更新方式等" :maxlength="255" />
        </ManageFormField>
        <ManageFormField label="排序" hint="越小越靠前">
          <ManageInput v-model.number="form.sort" type="number" />
        </ManageFormField>
        <ManageFormField label="状态">
          <label class="jc-livemgr__switch">
            <input v-model.number="form.state" type="checkbox" :true-value="0" :false-value="1" />
            <span>{{ form.state === 0 ? '启用' : '停用' }}</span>
          </label>
        </ManageFormField>
      </div>
      <template #footer>
        <BaseButton variant="ghost" @click="dialogOpen = false">取消</BaseButton>
        <BaseButton variant="gradient" :loading="saving" @click="save">保存</BaseButton>
      </template>
    </BaseDialog>

    <!-- 频道管理面板 -->
    <BaseDialog v-model:visible="chOpen" :title="`频道管理 · ${chSource?.name ?? ''}`" width="720px">
      <div class="jc-chmgr">
        <div class="jc-chmgr__toolbar">
          <ManageInput v-model="chSearch" placeholder="搜索频道名/分组/地址" class="jc-chmgr__search" />
          <BaseButton variant="outline" size="sm" @click="openCustom">
            <BaseIcon name="plus" size="14px" />
            添加自定义频道
          </BaseButton>
          <BaseButton variant="gradient" size="sm" :loading="probing" @click="probe">
            {{ probing ? '探测中…' : 'CORS 探源' }}
          </BaseButton>
        </div>
        <p class="jc-chmgr__hint">
          共 {{ chList.length }} 个频道，可跨域 {{ chProbeStats.ok }} / 已探测 {{ chProbeStats.probed }}。
          探源结果供前台频道置灰标记（跨域受限/离线）。
        </p>
        <BaseSkeleton v-if="chLoading" variant="rect" class="jc-chmgr__skeleton" />
        <div v-else class="jc-chmgr__list">
          <div v-for="c in chFiltered" :key="c.url" class="jc-chmgr__item" :class="{ 'is-hidden': c.hidden }">
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
              <BaseButton variant="ghost" size="sm" @click="toggleChannel(c)">{{ c.hidden ? '显示' : '隐藏' }}</BaseButton>
              <BaseButton variant="ghost" size="sm" @click="openRename(c)">改名</BaseButton>
              <BaseButton variant="ghost" size="sm" class="jc-livemgr__danger" @click="removeChannel(c)">
                {{ c.isCustom ? '删除' : '隐藏并移除' }}
              </BaseButton>
            </div>
          </div>
          <p v-if="chFiltered.length === 0" class="jc-chmgr__empty">无匹配频道</p>
        </div>
      </div>
      <template #footer>
        <BaseButton variant="ghost" @click="chOpen = false">关闭</BaseButton>
      </template>
    </BaseDialog>

    <!-- 自定义频道新增 -->
    <BaseDialog v-model:visible="customOpen" title="添加自定义频道" width="480px">
      <div class="jc-livemgr__form">
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
        <BaseButton variant="ghost" @click="customOpen = false">取消</BaseButton>
        <BaseButton variant="gradient" :loading="chSaving" @click="saveCustom">添加</BaseButton>
      </template>
    </BaseDialog>

    <!-- 频道改名（解析频道 display_name 覆盖） -->
    <BaseDialog v-model:visible="renameOpen" :title="`显示名 · ${renameTarget?.name ?? ''}`" width="420px">
      <div class="jc-livemgr__form">
        <ManageFormField label="显示名" hint="留空 = 恢复解析名">
          <ManageInput v-model="renameForm.displayName" :maxlength="255" />
        </ManageFormField>
      </div>
      <template #footer>
        <BaseButton variant="ghost" @click="renameOpen = false">取消</BaseButton>
        <BaseButton variant="gradient" :loading="chSaving" @click="saveRename">保存</BaseButton>
      </template>
    </BaseDialog>
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
.jc-livemgr__form {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-3);
}
.jc-livemgr__switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
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
</style>
