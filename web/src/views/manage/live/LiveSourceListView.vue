<script setup lang="ts">
/**
 * 管理端·直播源维护：多直播源增删改/启停/排序 + 频道数预览。
 * 后端：GET/POST /manage/live-sources，PUT/DELETE /manage/live-sources/:id
 * （改/删服务端自动清该源频道缓存，公开端点立即生效）。
 */
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { LiveSourceManage } from '@/types/live'
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
            {{ channelCounts[s.id] === undefined ? '频道数加载中…' : channelCounts[s.id] === null ? '频道数获取失败' : `${channelCounts[s.id]} 个频道` }}
          </span>
          <span class="jc-livemgr__sort">排序 {{ s.sort }}</span>
        </div>
        <div class="jc-livemgr__actions">
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
</style>
