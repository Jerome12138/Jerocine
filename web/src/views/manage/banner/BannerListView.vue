<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { Banner, EffectiveSlide } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageImageUpload from '@/components/manage/ManageImageUpload.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'
import { toast } from '@/api/http'

/**
 * 首页轮播管理。
 * 横图 = 宽幅主视觉(桌面/大屏那张大图); 竖图 = 窄屏兜底(只给竖图时走"模糊铺底 + 侧栏竖海报")。
 * 跳转二选一: 关联影片(mid) 或 自定义链接(站内路径/外链, 优先)。
 * 顶部「当前生效」区实时展示首页此刻真正显示的轮播位(配置轮播 + 无配置时的热门兜底),
 * 兜底位可一键转为配置; 横图 worker 只对这里出现的影片回填 TMDB 横图。
 */

const rows = ref<Banner[]>([])
const loading = ref(true)
const sheetOpen = ref(false)
const submitting = ref(false)
const editing = ref<Banner | null>(null)

/** 当前生效列表(30s 自动刷新) */
const effectiveRows = ref<EffectiveSlide[]>([])
let effectiveTimer: number | undefined

interface Form {
  id: number
  title: string
  subtitle: string
  image: string
  poster: string
  mid: string
  link: string
  sort: string
  state: number
  startStr: string
  endStr: string
}

const form = reactive<Form>(blank())

function blank(): Form {
  return {
    id: 0, title: '', subtitle: '', image: '', poster: '',
    mid: '', link: '', sort: '0', state: 0, startStr: '', endStr: ''
  }
}

// 列 key 必须是 Banner 的真实字段（ManageTable 的列类型按行类型约束）：
// 「跳转」用 link、「生效期」用 startAt，渲染内容在 #cell 里按 key 定制。
const columns = [
  { key: 'image' as const, label: '横图', width: '150px' },
  { key: 'title' as const, label: '标题' },
  { key: 'link' as const, label: '跳转', width: '200px' },
  { key: 'sort' as const, label: '排序', width: '70px', align: 'center' as const },
  { key: 'startAt' as const, label: '生效期', width: '190px' },
  { key: 'state' as const, label: '状态', width: '80px', align: 'center' as const }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    rows.value = (await manageApi.banner.list()) ?? []
  } finally {
    loading.value = false
  }
  await loadEffective(true)
}

/** 拉取当前生效列表; silent=true 时失败不打断页面(轮询路径) */
async function loadEffective(silent = false): Promise<void> {
  try {
    effectiveRows.value = (await manageApi.banner.effective()) ?? []
  } catch (e) {
    if (!silent) console.error('load effective banners failed', e)
  }
}

/** 兜底片一键转配置: 预填片名/封面/mid, 管理员补横图后保存 */
function convertFromFallback(row: EffectiveSlide): void {
  editing.value = null
  Object.assign(form, blank())
  form.title = row.name
  form.subtitle = row.subtitle ?? ''
  form.poster = row.poster ?? ''
  form.mid = row.mid && row.mid > 0 ? String(row.mid) : ''
  sheetOpen.value = true
}

/** 从生效区跳编辑对应配置(列表里找 bannerId 对应行) */
function editEffective(row: EffectiveSlide): void {
  const target = row.bannerId ? rows.value.find((b) => b.id === row.bannerId) : undefined
  if (target) {
    openEdit(target)
  } else {
    toast('error', '该配置不在当前列表中，请刷新后重试')
  }
}

function effGoLabel(row: EffectiveSlide): string {
  if (row.link) return row.link
  if (row.mid && row.mid > 0) return `影片 #${row.mid}`
  return '未配置'
}

function openAdd(): void {
  editing.value = null
  Object.assign(form, blank())
  sheetOpen.value = true
}

function openEdit(row: Banner): void {
  editing.value = row
  Object.assign(form, {
    id: row.id,
    title: row.title,
    subtitle: row.subtitle,
    image: row.image,
    poster: row.poster,
    mid: row.mid > 0 ? String(row.mid) : '',
    link: row.link,
    sort: String(row.sort ?? 0),
    state: row.state,
    startStr: msToLocal(row.startAt),
    endStr: msToLocal(row.endAt)
  })
  sheetOpen.value = true
}

async function submit(): Promise<void> {
  if (!form.image && !form.poster) {
    window.alert('至少需要上传横图或竖图')
    return
  }
  if (!form.link.trim() && !(Number(form.mid) > 0)) {
    window.alert('需要配置跳转：关联影片 ID 或自定义链接')
    return
  }
  const startAt = localToMs(form.startStr)
  const endAt = localToMs(form.endStr)
  if (startAt > 0 && endAt > 0 && endAt < startAt) {
    window.alert('结束时间不能早于开始时间')
    return
  }
  submitting.value = true
  try {
    await manageApi.banner.save({
      id: form.id,
      title: form.title,
      subtitle: form.subtitle,
      image: form.image,
      poster: form.poster,
      mid: Number(form.mid) > 0 ? Number(form.mid) : 0,
      link: form.link.trim(),
      sort: Number(form.sort) || 0,
      state: form.state,
      startAt,
      endAt
    })
    sheetOpen.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

async function toggleState(row: Banner): Promise<void> {
  await manageApi.banner.save({ ...row, state: row.state === 0 ? 1 : 0 })
  await load()
}

async function remove(row: Banner): Promise<void> {
  const ok = await confirm({
    title: '确认删除这张轮播？',
    desc: `「${row.title || `#${row.id}`}」删除后不可恢复`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  await manageApi.banner.remove(row.id)
  await load()
}

/* ---- 时间与展示辅助 ---- */

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

/** ms → datetime-local 需要的 'YYYY-MM-DDTHH:mm' */
function msToLocal(ms: number): string {
  if (!ms) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function localToMs(s: string): number {
  if (!s) return 0
  const t = new Date(s).getTime()
  return Number.isNaN(t) ? 0 : t
}

function fmtWindow(row: Banner): string {
  if (!row.startAt && !row.endAt) return '长期'
  const f = (ms: number): string => {
    const d = new Date(ms)
    return `${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  return `${row.startAt ? f(row.startAt) : '即日'} ~ ${row.endAt ? f(row.endAt) : '不限'}`
}

function goLabel(row: Banner): string {
  if (row.link) return row.link
  if (row.mid > 0) return `影片 #${row.mid}`
  return '未配置'
}

/** 生效中 = 启用 + 落在窗口内 */
function isLive(row: Banner): boolean {
  if (row.state !== 0) return false
  const now = Date.now()
  if (row.startAt && row.startAt > now) return false
  if (row.endAt && row.endAt < now) return false
  return true
}

onMounted(() => {
  load()
  loadEffective(true)
  effectiveTimer = window.setInterval(() => loadEffective(true), 30_000)
})

onUnmounted(() => {
  if (effectiveTimer !== undefined) window.clearInterval(effectiveTimer)
})
</script>

<template>
  <!-- ===================== 当前生效：首页此刻真正显示的轮播位（实时） ===================== -->
  <section
    class="mb-[var(--gf-space-5)] rounded-[var(--gf-radius-md)] border border-default bg-elevated p-[var(--gf-space-4)]"
  >
    <div class="flex items-center justify-between mb-[var(--gf-space-2)]">
      <div class="flex items-center gap-[var(--gf-space-2)] flex-wrap">
        <h3 class="font-[var(--gf-fw-semibold)]">当前生效</h3>
        <span class="text-muted text-xs">首页此刻实际展示的轮播位 · 每 30 秒自动刷新</span>
      </div>
      <BaseButton variant="ghost" size="sm" @click="loadEffective()">
        <BaseIcon name="refresh" size="16px" /> 刷新
      </BaseButton>
    </div>

    <div v-if="!effectiveRows.length" class="text-muted text-sm py-[var(--gf-space-4)] text-center">
      暂无生效轮播 —— 无可用配置且片库为空
    </div>
    <ul v-else class="flex flex-col">
      <li
        v-for="(s, i) in effectiveRows"
        :key="`${s.source}-${s.bannerId ?? 0}-${s.mid ?? 0}-${i}`"
        class="flex items-center gap-[var(--gf-space-3)] py-[var(--gf-space-2)] border-b border-default last:border-b-0"
      >
        <img
          v-if="s.image || s.poster"
          :src="s.image || s.poster"
          alt=""
          class="w-[96px] h-[54px] object-cover rounded-[var(--gf-radius-sm)] bg-elevated shrink-0"
        />
        <span
          v-else
          class="w-[96px] h-[54px] grid place-items-center text-muted text-xs rounded-[var(--gf-radius-sm)] bg-elevated shrink-0"
        >无图</span>
        <div class="flex flex-col min-w-0 flex-1">
          <span class="truncate">{{ s.name || '为你推荐' }}</span>
          <span v-if="s.subtitle" class="text-muted text-xs truncate">{{ s.subtitle }}</span>
        </div>
        <BaseTag :variant="s.source === 'banner' ? 'success' : 'default'" size="sm" class="shrink-0">
          {{ s.source === 'banner' ? '配置' : '兜底' }}
        </BaseTag>
        <span class="text-xs text-link w-[130px] truncate hidden lg:block">{{ effGoLabel(s) }}</span>
        <BaseTag
          v-if="s.source === 'fallback'"
          :variant="s.image ? 'success' : 'warning'"
          size="sm"
          class="hidden md:inline-flex shrink-0"
        >
          {{ s.image ? '横图已就绪' : '横图待回填' }}
        </BaseTag>
        <div class="flex gap-[var(--gf-space-1)] shrink-0">
          <BaseButton v-if="s.source === 'banner'" variant="ghost" size="sm" @click="editEffective(s)">
            编辑
          </BaseButton>
          <BaseButton v-else variant="ghost" size="sm" @click="convertFromFallback(s)">
            转为轮播
          </BaseButton>
        </div>
      </li>
    </ul>
  </section>

  <ManageTable
    :columns="columns"
    :rows="rows"
    row-key="id"
    :loading="loading"
    empty="暂无轮播配置 —— 首页会自动回退用热门影片拼大图"
    actions-width="200px"
  >
    <template #toolbar>
      <div class="flex items-center gap-[var(--gf-space-3)] flex-wrap">
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">首页轮播</h2>
        <span class="text-muted text-xs">横图为宽幅主视觉，竖图仅作窄屏兜底；都不配则回退热门影片</span>
      </div>
      <div class="flex gap-[var(--gf-space-2)]">
        <BaseButton variant="ghost" size="sm" @click="load">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
        <BaseButton variant="gradient" size="sm" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </BaseButton>
      </div>
    </template>

    <template #cell="{ row, col }">
      <div v-if="col.key === 'image'" class="w-[132px]">
        <img
          v-if="row.image || row.poster"
          :src="row.image || row.poster"
          alt=""
          class="w-[132px] h-[42px] object-cover rounded-[var(--gf-radius-sm)] bg-elevated"
        />
        <span v-else class="text-muted text-xs">无图</span>
      </div>
      <div v-else-if="col.key === 'title'" class="flex flex-col min-w-0">
        <span class="truncate">{{ row.title || '—' }}</span>
        <span v-if="row.subtitle" class="text-muted text-xs truncate">{{ row.subtitle }}</span>
      </div>
      <span v-else-if="col.key === 'link'" class="text-xs break-all text-link">{{ goLabel(row) }}</span>
      <span v-else-if="col.key === 'startAt'" class="text-xs">{{ fmtWindow(row) }}</span>
      <BaseTag
        v-else-if="col.key === 'state'"
        :variant="isLive(row) ? 'success' : 'default'"
        size="sm"
      >
        {{ row.state === 0 ? (isLive(row) ? '生效中' : '未生效') : '停用' }}
      </BaseTag>
      <span v-else>{{ row[col.key] ?? '—' }}</span>
    </template>

    <template #actions="{ row }">
      <div class="flex gap-[var(--gf-space-1)] justify-end">
        <BaseButton variant="ghost" size="sm" @click="toggleState(row)">
          {{ row.state === 0 ? '停用' : '启用' }}
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="openEdit(row)">编辑</BaseButton>
        <BaseButton variant="danger" size="sm" @click="remove(row)">删除</BaseButton>
      </div>
    </template>
  </ManageTable>

  <ManageSheet v-model="sheetOpen" :title="editing ? '编辑轮播' : '新增轮播'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--gf-space-4)]">
      <ManageFormField label="标题" hint="留空则前台显示「为你推荐」">
        <ManageInput
          :model-value="form.title"
          placeholder="如：本周强推"
          @update:model-value="(v) => (form.title = String(v))"
        />
      </ManageFormField>

      <ManageFormField label="副标题" hint="展示在大标题下方的一行简介">
        <ManageInput
          :model-value="form.subtitle"
          placeholder="可选"
          @update:model-value="(v) => (form.subtitle = String(v))"
        />
      </ManageFormField>

      <ManageFormField label="横图（宽幅主视觉）" required hint="建议 21:9 或 16:9，桌面/大屏首屏主图">
        <ManageImageUpload v-model="form.image" ratio="21/9" />
      </ManageFormField>

      <ManageFormField label="竖图（窄屏兜底）" hint="建议 2:3；只给竖图时会用模糊铺底 + 侧栏竖海报">
        <ManageImageUpload v-model="form.poster" ratio="2/3" />
      </ManageFormField>

      <ManageFormField label="关联影片 ID" hint="填 mid 则点击进入该片详情；与自定义链接二选一">
        <ManageInput
          :model-value="form.mid"
          type="number"
          placeholder="如 12345"
          @update:model-value="(v) => (form.mid = String(v))"
        />
      </ManageFormField>

      <ManageFormField label="自定义链接" hint="站内路径（/filmClassify?Pid=1）或外链（https://…）；优先于关联影片">
        <ManageInput
          :model-value="form.link"
          placeholder="/filmClassify?Pid=1 或 https://…"
          @update:model-value="(v) => (form.link = String(v))"
        />
      </ManageFormField>

      <ManageFormField label="排序" hint="数字越小越靠前">
        <ManageInput
          :model-value="form.sort"
          type="number"
          placeholder="0"
          @update:model-value="(v) => (form.sort = String(v))"
        />
      </ManageFormField>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-[var(--gf-space-4)]">
        <ManageFormField label="生效开始" hint="留空 = 立即">
          <ManageInput
            :model-value="form.startStr"
            type="datetime-local"
            @update:model-value="(v) => (form.startStr = String(v))"
          />
        </ManageFormField>
        <ManageFormField label="生效结束" hint="留空 = 长期">
          <ManageInput
            :model-value="form.endStr"
            type="datetime-local"
            @update:model-value="(v) => (form.endStr = String(v))"
          />
        </ManageFormField>
      </div>

      <ManageFormField label="启用">
        <ManageSwitch
          :model-value="form.state === 0"
          @update:model-value="(v) => (form.state = v ? 0 : 1)"
        />
      </ManageFormField>
    </div>

    <template #footer>
      <BaseButton variant="ghost" @click="sheetOpen = false">取消</BaseButton>
      <BaseButton variant="gradient" :loading="submitting" @click="submit">保存</BaseButton>
    </template>
  </ManageSheet>
</template>
