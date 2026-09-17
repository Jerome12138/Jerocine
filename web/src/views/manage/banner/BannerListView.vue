<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { Banner, BannerBoard, EffectiveSlide, InactiveBanner } from '@/types/manage'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageImageUpload from '@/components/manage/ManageImageUpload.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'

/**
 * 首页轮播管理 —— 单一列表 = 首页此刻实际生效的前 5 位, 与前台大图完全同源。
 * 手动位(banner): 完整增删改 + 启用/停用 + 上下移;
 * 自动位(热榜补位): 派生数据, 支持编辑采纳(转手动钉位)、禁用屏蔽、上移采纳;
 *   铁律: mid 出现在 banner 表(无论启用与否)就不再被自动补位选中。
 * 未生效区: 停用/缺图/未开始/已过期/超位的配置行, 可启用(转手动)/编辑/删除。
 * 横图 worker 只对生效位的影片回填 TMDB 横图; 轮播一有变动即触发补采。
 */

const board = ref<BannerBoard>({ active: [], inactive: [] })
const loading = ref(true)
const sheetOpen = ref(false)
const submitting = ref(false)
const editing = ref<Banner | null>(null)
/** 采纳自动位时的钉入位置(生效列表下标); undefined = 追加到手动末尾 */
const adoptSlot = ref<number | undefined>(undefined)
const showInactive = ref(false)
let pollTimer: number | undefined

const active = computed(() => board.value.active ?? [])
const inactive = computed(() => board.value.inactive ?? [])
/** 可见手动位数(上/下移边界用) */
const manualCount = computed(() => active.value.filter((s) => s.source === 'banner').length)

interface Form {
  id: number
  title: string
  subtitle: string
  image: string
  poster: string
  mid: string
  link: string
  state: number
  startStr: string
  endStr: string
}

const form = reactive<Form>(blank())

function blank(): Form {
  return {
    id: 0, title: '', subtitle: '', image: '', poster: '',
    mid: '', link: '', state: 0, startStr: '', endStr: ''
  }
}

async function load(silent = false): Promise<void> {
  if (!silent) loading.value = true
  try {
    board.value = (await manageApi.banner.board()) ?? { active: [], inactive: [] }
  } catch (e) {
    if (!silent) console.error('load banner board failed', e)
  } finally {
    if (!silent) loading.value = false
  }
}

function openAdd(): void {
  editing.value = null
  adoptSlot.value = undefined
  Object.assign(form, blank())
  sheetOpen.value = true
}

function openEdit(row: Banner): void {
  editing.value = row
  adoptSlot.value = undefined
  Object.assign(form, {
    id: row.id,
    title: row.title,
    subtitle: row.subtitle,
    image: row.image,
    poster: row.poster,
    mid: row.mid > 0 ? String(row.mid) : '',
    link: row.link,
    state: row.state,
    startStr: msToLocal(row.startAt),
    endStr: msToLocal(row.endAt)
  })
  sheetOpen.value = true
}

/** 采纳自动位: 预填当前生效内容(含已回填横图), 保存后转手动位并钉在原位置 */
function adoptSlide(s: EffectiveSlide, slot: number): void {
  editing.value = null
  adoptSlot.value = slot
  Object.assign(form, blank())
  form.title = s.name
  form.subtitle = s.subtitle ?? ''
  form.image = s.image ?? ''
  form.poster = s.poster ?? ''
  form.mid = s.mid && s.mid > 0 ? String(s.mid) : ''
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
      sort: 0,
      state: form.state,
      startAt,
      endAt
    }, adoptSlot.value)
    sheetOpen.value = false
    adoptSlot.value = undefined
    await load(true)
  } finally {
    submitting.value = false
  }
}

/** 生效位排序: 手动位换位 / 自动位上移采纳(后端返回新 Board) */
async function move(i: number, dir: 'up' | 'down'): Promise<void> {
  board.value = await manageApi.banner.move(i, dir)
}

/** 启用/停用配置行(停用自动补位里的 mid = 屏蔽) */
async function toggleState(row: Banner): Promise<void> {
  await manageApi.banner.save({ ...row, state: row.state === 0 ? 1 : 0 })
  await load(true)
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
  await load(true)
}

function canMoveUp(s: EffectiveSlide, i: number): boolean {
  return s.source === 'banner' ? i > 0 : true
}

function canMoveDown(s: EffectiveSlide, i: number): boolean {
  if (s.source !== 'banner') return false // 自动位恒在尾部
  return i < manualCount.value - 1
}

/* ---- 展示辅助 ---- */

function goLabelOf(s: EffectiveSlide): string {
  if (s.link) return s.link
  if (s.mid && s.mid > 0) return `影片 #${s.mid}`
  return '未配置'
}

const reasonStyle: Record<InactiveBanner['reason'], { text: string; variant: 'danger' | 'warning' | 'default' }> = {
  disabled: { text: '已停用', variant: 'danger' },
  noimage: { text: '缺横竖图', variant: 'warning' },
  pending: { text: '未开始', variant: 'default' },
  expired: { text: '已过期', variant: 'default' },
  overflow: { text: '超出前5位', variant: 'default' }
}

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

onMounted(() => {
  load()
  pollTimer = window.setInterval(() => {
    if (!sheetOpen.value) load(true) // 表单打开时暂停轮询, 避免编辑中被覆盖
  }, 30_000)
})

onUnmounted(() => {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
})
</script>

<template>
  <div
    class="rounded-[var(--jc-radius-md)] border border-default bg-elevated p-[var(--jc-space-4)]"
  >
    <div class="flex items-center justify-between mb-[var(--jc-space-2)] flex-wrap gap-[var(--jc-space-2)]">
      <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">首页轮播</h2>
        <span class="text-muted text-xs">
          与首页大图实时一致 · 手动位排前, 不足 5 位由热榜自动补位 · 每 30 秒刷新
        </span>
      </div>
      <div class="flex gap-[var(--jc-space-2)]">
        <BaseButton variant="ghost" size="sm" @click="load()">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
        <BaseButton variant="gradient" size="sm" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </BaseButton>
      </div>
    </div>

    <div v-if="loading" class="text-muted text-sm py-[var(--jc-space-6)] text-center">加载中…</div>
    <div
      v-else-if="!active.length"
      class="text-muted text-sm py-[var(--jc-space-6)] text-center"
    >
      暂无生效轮播 —— 无可用配置且片库为空
    </div>

    <!-- 生效位(前 5) -->
    <ul v-else class="flex flex-col">
      <li
        v-for="(s, i) in active"
        :key="`${s.source}-${s.bannerId ?? 0}-${s.mid ?? 0}-${i}`"
        class="flex items-center gap-[var(--jc-space-3)] py-[var(--jc-space-2)] border-b border-default last:border-b-0"
      >
        <span class="w-4 text-center text-muted text-xs shrink-0">{{ i + 1 }}</span>
        <img
          v-if="s.image || s.poster"
          :src="s.image || s.poster"
          alt=""
          class="w-[96px] h-[54px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated shrink-0"
        />
        <span
          v-else
          class="w-[96px] h-[54px] grid place-items-center text-muted text-xs rounded-[var(--jc-radius-sm)] bg-elevated shrink-0"
        >无图</span>
        <div class="flex flex-col min-w-0 flex-1">
          <span class="truncate">{{ s.name || '为你推荐' }}</span>
          <span class="text-muted text-xs truncate">
            {{ s.subtitle || (s.source === 'banner' ? goLabelOf(s) : '热榜自动补入') }}
          </span>
        </div>
        <BaseTag :variant="s.source === 'banner' ? 'success' : 'default'" size="sm" class="shrink-0">
          {{ s.source === 'banner' ? '手动' : '自动' }}
        </BaseTag>
        <BaseTag
          v-if="s.source === 'fallback'"
          :variant="s.image ? 'success' : 'warning'"
          size="sm"
          class="hidden md:inline-flex shrink-0"
        >
          {{ s.image ? '横图已就绪' : '横图待回填' }}
        </BaseTag>
        <span class="text-xs text-link w-[120px] truncate hidden lg:block shrink-0">{{ goLabelOf(s) }}</span>
        <div class="flex gap-[var(--jc-space-1)] shrink-0">
          <BaseButton
            variant="ghost" size="sm" :disabled="!canMoveUp(s, i)"
            :title="s.source === 'banner' ? '上移' : '上移（转为手动位）'"
            @click="move(i, 'up')"
          >↑</BaseButton>
          <BaseButton
            variant="ghost" size="sm" :disabled="!canMoveDown(s, i)"
            :title="s.source === 'banner' ? '下移' : '自动位恒在尾部'"
            @click="move(i, 'down')"
          >↓</BaseButton>
          <BaseButton
            v-if="s.source === 'banner'"
            variant="ghost" size="sm" @click="s.banner && openEdit(s.banner)"
          >编辑</BaseButton>
          <BaseButton v-else variant="ghost" size="sm" @click="adoptSlide(s, i)">编辑</BaseButton>
          <BaseButton
            variant="ghost" size="sm"
            :title="s.source === 'banner' ? '停用' : '停用（屏蔽该影片的自动补位）'"
            @click="s.source === 'banner' && s.banner ? toggleState(s.banner) : manageApi.banner.save({ id: 0, title: s.name, subtitle: '', image: '', poster: '', mid: s.mid ?? 0, link: '', sort: 0, state: 1, startAt: 0, endAt: 0 }).then(() => load(true))"
          >禁用</BaseButton>
          <BaseButton
            v-if="s.source === 'banner'"
            variant="danger" size="sm" @click="s.banner && remove(s.banner)"
          >删除</BaseButton>
        </div>
      </li>
    </ul>

    <!-- 未生效配置行(折叠区) -->
    <div v-if="!loading && inactive.length" class="border-t border-default mt-[var(--jc-space-1)]">
      <button
        type="button"
        class="w-full flex items-center gap-[var(--jc-space-1)] py-[var(--jc-space-2)] text-muted text-xs"
        @click="showInactive = !showInactive"
      >
        <span>{{ showInactive ? '▾' : '▸' }}</span>
        <span>未生效（{{ inactive.length }} · 不参与展示与自动补位）</span>
      </button>
      <ul v-if="showInactive" class="flex flex-col pb-[var(--jc-space-2)]">
        <li
          v-for="row in inactive"
          :key="row.id"
          class="flex items-center gap-[var(--jc-space-3)] py-[var(--jc-space-2)]"
        >
          <img
            v-if="row.image || row.poster"
            :src="row.image || row.poster"
            alt=""
            class="w-[96px] h-[54px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 opacity-60"
          />
          <span
            v-else
            class="w-[96px] h-[54px] grid place-items-center text-muted text-xs rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 opacity-60"
          >无图</span>
          <span class="truncate flex-1 min-w-0 text-secondary">{{ row.title || `#${row.mid > 0 ? '影片 ' + row.mid : row.id}` }}</span>
          <BaseTag :variant="reasonStyle[row.reason].variant" size="sm" class="shrink-0">
            {{ reasonStyle[row.reason].text }}
          </BaseTag>
          <div class="flex gap-[var(--jc-space-1)] shrink-0">
            <BaseButton
              v-if="row.state !== 0"
              variant="ghost" size="sm" title="启用后转为手动位"
              @click="toggleState(row)"
            >启用</BaseButton>
            <BaseButton variant="ghost" size="sm" @click="openEdit(row)">编辑</BaseButton>
            <BaseButton variant="danger" size="sm" @click="remove(row)">删除</BaseButton>
          </div>
        </li>
      </ul>
    </div>
  </div>

  <ManageSheet v-model="sheetOpen" :title="editing ? '编辑轮播' : adoptSlot !== undefined ? '采纳自动位' : '新增轮播'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--jc-space-4)]">
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

      <div class="grid grid-cols-1 md:grid-cols-2 gap-[var(--jc-space-4)]">
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
