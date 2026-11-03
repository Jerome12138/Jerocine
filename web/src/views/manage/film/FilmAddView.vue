<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { normalizeCollectResult } from '@/api/manage/film'
import type {
  CollectSource,
  FilmClass,
  SourceFilmResult,
  SourceSearchHit
} from '@/types/manage'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import ManageImageUpload from '@/components/manage/ManageImageUpload.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import SourceCandidateDialog from '@/components/manage/SourceCandidateDialog.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'
import type { SelectOption } from '@/components/manage/ManageSelect.vue'

const router = useRouter()
const tree = ref<FilmClass[]>([])
const treeError = ref('')

// 写操作统一 useAsyncAction: busy 并发拦截 + 失败 toast 单点; 请求一律 silent
const { busy, run } = useAsyncAction()

// ============ 按片名搜索站点影片 + 采集 ============
const collectSources = ref<CollectSource[]>([])
const sourcesError = ref('')
const spiderKeyword = ref('')
const searching = ref(false)
const results = ref<SourceFilmResult[]>([])
const searched = ref(false)
/** 选中的源 id 集合(供采集勾选) */
const checked = ref<Record<string, boolean>>({})

/** 已勾选的源 id 列表(停用源不可勾, 服务端 resolveSources 也会静默丢弃停用源) */
const checkedIds = computed(() =>
  collectSources.value.filter((s) => s.state && checked.value[s.id]).map((s) => s.id)
)
/** 可勾选的启用源数(全为 0 时按钮组置灰) */
const enabledSourceCount = computed(() => collectSources.value.filter((s) => s.state).length)

function toggleAll(on: boolean): void {
  const next: Record<string, boolean> = {}
  for (const s of collectSources.value) next[s.id] = on && s.state
  checked.value = next
}

/** 加载分类树/采集源: 失败置各自错误态+重试入口, 不再静默吞掉 */
async function loadTree(): Promise<void> {
  treeError.value = ''
  try {
    tree.value = await manageApi.film.classTree()
  } catch (e) {
    treeError.value = (e as Error).message ?? '加载失败'
  }
}

async function loadSources(): Promise<void> {
  sourcesError.value = ''
  try {
    collectSources.value = await manageApi.collect.list()
  } catch (e) {
    sourcesError.value = (e as Error).message ?? '加载失败'
  }
}

/** 搜索各源命中影片(预览不落库)。注意: 服务端空 sources = 全部启用源, 不受下方勾选影响 */
async function searchSpider(): Promise<void> {
  const kw = spiderKeyword.value.trim()
  if (!kw) {
    toast('error', '请输入片名关键字')
    return
  }
  searching.value = true
  searched.value = false
  try {
    results.value = await manageApi.film.searchSources(kw)
    searched.value = true
  } catch {
    // http 拦截器已 toast
  } finally {
    searching.value = false
  }
}

/** 汇总各源采集结果并 toast */
function reportCollect(res: SourceFilmResult[]): void {
  const oks = res.filter((r) => !r.error)
  const errs = res.filter((r) => r.error)
  const total = oks.reduce((sum, r) => sum + (r.collected ?? 0), 0)
  const parts = oks.map((r) => `${r.sourceName} ${r.collected}`).join('、')
  if (oks.length) {
    toast('success', `采集完成, 共入库 ${total} 集` + (parts ? `（${parts}）` : ''))
  }
  if (errs.length) {
    toast('error', `${errs.length} 个源采集失败: ${errs.map((r) => r.sourceName).join('、')}`)
  }
  if (!oks.length && !errs.length) {
    toast('error', '未采集到任何内容')
  }
}

/** 采集: ids 为空数组表示采集全部启用源。silent=true 自管结果提示(业务成败是
 *  HTTP 200 里的 error 字段, 由 reportCollect 汇报), 异常 toast 由 run 单点 */
async function collect(ids: string[]): Promise<void> {
  const kw = spiderKeyword.value.trim()
  if (!kw) {
    toast('error', '请输入片名关键字')
    return
  }
  await run({
    action: async () => {
      const res = normalizeCollectResult(
        await manageApi.film.collectFilm({ keyword: kw, sources: ids }, { silent: true })
      )
      results.value = res
      searched.value = true
      reportCollect(res)
    }
  })
}

function collectChecked(): void {
  if (!checkedIds.value.length) {
    toast('error', '请先勾选采集源')
    return
  }
  void collect(checkedIds.value)
}

function collectAll(): void {
  // 空数组 = 后端采集全部启用源
  void collect([])
}

// ============ 单条命中精确采集 + 候选弹框 ============
/** 正在精确采集的命中(键 = `${sourceId}:${sourceVodId}`) */
const hitCollecting = ref<Record<string, boolean>>({})

/** 候选选择对话框状态(多源采集时某源返回多版本) */
const candidateDialog = ref<{
  visible: boolean
  sourceId: string
  sourceName: string
  list: SourceSearchHit[]
}>({ visible: false, sourceId: '', sourceName: '', list: [] })
const candidatePicking = ref(false)

function hitKey(sourceId: string, vodId: number): string {
  return `${sourceId}:${vodId}`
}

/** 命中条目精确采集: 用户已亲选具体版本 → collectFilm({ sourceId, vodId })(异常 toast 由 run 单点) */
async function collectHit(sourceId: string, sourceName: string, h: SourceSearchHit): Promise<void> {
  const key = hitKey(sourceId, h.sourceVodId)
  hitCollecting.value[key] = true
  await run({
    action: async () => {
      const res = await manageApi.film.collectFilm({ sourceId, vodId: h.sourceVodId }, { silent: true })
      const list = normalizeCollectResult(res)
      const hit = list.find((r) => r.sourceId === sourceId) ?? list[0]
      if (hit?.error) {
        toast('error', `${sourceName} 采集失败: ${hit.error}`)
      } else if (hit && hit.collected > 0) {
        toast('success', `${sourceName} · ${h.name} 采集完成, 入库 ${hit.collected} 集`)
      } else {
        toast('error', `${sourceName} · ${h.name} 采集未入库`)
      }
    }
  })
  hitCollecting.value[key] = false
}

/** 打开某源的候选选择弹框(多源采集返回 candidates 时) */
function openCandidates(r: SourceFilmResult): void {
  if (!r.candidates || !r.candidates.length) return
  candidateDialog.value = {
    visible: true,
    sourceId: r.sourceId,
    sourceName: r.sourceName,
    list: r.candidates
  }
}

/** 候选弹框选定某版本 → 精确采集(异常 toast 由 run 单点) */
async function onPickCandidate(h: SourceSearchHit): Promise<void> {
  const { sourceId, sourceName } = candidateDialog.value
  if (!sourceId) return
  candidatePicking.value = true
  await run({
    action: async () => {
      const res = await manageApi.film.collectFilm({ sourceId, vodId: h.sourceVodId }, { silent: true })
      const list = normalizeCollectResult(res)
      const hit = list.find((r) => r.sourceId === sourceId) ?? list[0]
      if (hit?.error) {
        toast('error', `${sourceName} 采集失败: ${hit.error}`)
      } else if (hit && hit.collected > 0) {
        toast('success', `${sourceName} · ${h.name} 采集完成, 入库 ${hit.collected} 集`)
        candidateDialog.value.visible = false
      } else {
        toast('error', `${sourceName} · ${h.name} 采集未入库`)
      }
    }
  })
  candidatePicking.value = false
}

// ============ 手动新增表单 ============
const form = reactive({
  name: '',
  enName: '',
  cid: 0,
  pid: 0,
  picture: '',
  year: '',
  area: '',
  director: '',
  actor: '',
  classTag: '',
  content: ''
})
/** 字段级错误(ManageFormField error prop 展示) */
const nameError = ref('')
const yearError = ref('')

/** 顶级分类下拉: 「请选择」= 0(顶级), 其余为分类树一级节点(数字 value 原类型回传) */
const pidOptions = computed<SelectOption[]>(() => [
  { label: '请选择', value: 0 },
  ...tree.value.map((t) => ({ label: t.name, value: t.id }))
])

onMounted(() => {
  void loadTree()
  void loadSources()
})

async function submit(): Promise<void> {
  nameError.value = ''
  yearError.value = ''
  const name = form.name.trim()
  if (!name) {
    nameError.value = '请填写影片名'
    return
  }
  const yearStr = String(form.year).trim()
  if (yearStr && !/^\d{4}$/.test(yearStr)) {
    yearError.value = '年份需为 4 位数字（如 2024），可留空'
    return
  }
  await run({
    action: () =>
      manageApi.film.add(
        {
          name,
          enName: form.enName,
          // ManageInput number 清空后是 undefined, 原样上送服务端 int64 绑定会 422 invalid body → 归一 0(与 year 同口径)
          cid: Number(form.cid) || 0,
          pid: form.pid,
          cover: form.picture,
          area: form.area,
          year: Number(form.year) || 0,
          director: form.director,
          actor: form.actor,
          classTag: form.classTag,
          content: form.content
        },
        { silent: true }
      ),
    after: () => {
      router.push('/manage/film')
    }
  })
}
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-6)] max-w-[960px]">
    <!-- ============ 按片名搜索站点影片 + 采集 ============ -->
    <section class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)]">
      <header class="mb-[var(--jc-space-4)]">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">按片名采集</h2>
        <p class="text-sm text-muted">
          输入片名搜索各启用采集源的命中结果（预览不落库，搜索固定覆盖全部启用源）；
          勾选源仅作用于采集入库
        </p>
      </header>

      <!-- 搜索栏 -->
      <div class="flex gap-[var(--jc-space-2)] flex-wrap mb-[var(--jc-space-4)]">
        <ManageInput
          v-model="spiderKeyword"
          placeholder="影片名关键字，如 流浪地球"
          class="flex-1 min-w-[200px]"
          @keydown.enter="searchSpider"
        />
        <ManageButton variant="gradient" size="sm" :loading="searching" @click="searchSpider">
          <BaseIcon name="search" size="16px" /> 搜索
        </ManageButton>
      </div>

      <!-- 源列表加载失败: 显式提示+重试(此前静默吞掉, 整个勾选区块无声消失) -->
      <div
        v-if="sourcesError"
        class="flex items-center gap-[var(--jc-space-2)] flex-wrap mb-[var(--jc-space-4)] text-sm text-[var(--jc-warning)]"
      >
        <BaseIcon name="info" size="14px" />
        <span>采集源列表加载失败：{{ sourcesError }}</span>
        <ManageButton variant="ghost" size="sm" @click="loadSources">重试</ManageButton>
      </div>

      <!-- 源勾选 + 采集动作 -->
      <div v-else-if="collectSources.length" class="flex flex-col gap-[var(--jc-space-3)] mb-[var(--jc-space-4)]">
        <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap text-sm">
          <span class="text-muted">采集源：</span>
          <label
            v-for="s in collectSources"
            :key="s.id"
            class="flex items-center gap-[var(--jc-space-1)] text-secondary"
            :class="s.state ? 'cursor-pointer' : 'opacity-50 cursor-not-allowed'"
          >
            <input
              v-model="checked[s.id]"
              type="checkbox"
              :disabled="!s.state"
              class="accent-[var(--jc-brand-primary)]"
              data-focusable="true"
            />
            <span>{{ s.name }}</span>
            <BaseTag v-if="s.grade === 0" variant="brand" size="xs">主站</BaseTag>
            <BaseTag v-if="!s.state" variant="warning" size="xs">已停用</BaseTag>
          </label>
          <div class="flex gap-[var(--jc-space-2)]">
            <ManageButton variant="ghost" size="sm" :disabled="!enabledSourceCount" @click="toggleAll(true)">全选</ManageButton>
            <ManageButton variant="ghost" size="sm" @click="toggleAll(false)">清空</ManageButton>
          </div>
        </div>
        <div class="flex gap-[var(--jc-space-2)] flex-wrap">
          <ManageButton
            variant="primary"
            size="sm"
            :loading="busy"
            :disabled="!enabledSourceCount"
            @click="collectChecked"
          >
            <BaseIcon name="magic" size="16px" /> 采集选中源
          </ManageButton>
          <ManageButton
            variant="outline"
            size="sm"
            :loading="busy"
            :disabled="!enabledSourceCount"
            @click="collectAll"
          >
            <BaseIcon name="magic" size="16px" /> 采集全部源
          </ManageButton>
        </div>
      </div>

      <!-- 搜索结果 -->
      <div v-if="searched" class="flex flex-col gap-[var(--jc-space-3)]">
        <BaseEmpty v-if="!results.length" description="各源均无命中" />
        <div
          v-for="r in results"
          :key="r.sourceId"
          class="border border-default rounded-[var(--jc-radius-md)] bg-elevated p-[var(--jc-space-3)]"
        >
          <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap mb-[var(--jc-space-2)]">
            <span class="font-[var(--jc-fw-medium)] text-primary">{{ r.sourceName }}</span>
            <BaseTag v-if="r.master" variant="brand" size="xs">主站</BaseTag>
            <BaseTag v-if="r.collected > 0" variant="success" size="xs">已采 {{ r.collected }} 集</BaseTag>
            <span v-if="r.error" class="text-xs text-[var(--jc-danger)]">采集失败：{{ r.error }}</span>
            <span v-else-if="r.hits && r.hits.length" class="text-xs text-muted">
              命中 {{ r.hits.length }} 部
            </span>
            <span v-else-if="r.candidates && r.candidates.length" class="text-xs text-muted">
              多版本 {{ r.candidates.length }} 部
            </span>
            <span v-else class="text-xs text-muted">无命中</span>
          </div>

          <!-- 多版本提示(多源采集时该源同名多版本, 未自动采集) -->
          <div
            v-if="r.candidates && r.candidates.length"
            class="flex items-center gap-[var(--jc-space-2)] flex-wrap mb-[var(--jc-space-2)] text-xs text-muted"
          >
            <BaseIcon name="info" size="14px" />
            <span>该源同名多版本，请在下方命中列表点具体版本采集，或</span>
            <ManageButton variant="ghost" size="sm" @click="openCandidates(r)">选择版本</ManageButton>
          </div>

          <!-- 命中影片列表(每条可单独采集) -->
          <div v-if="r.hits && r.hits.length" class="flex flex-col gap-[var(--jc-space-2)]">
            <div
              v-for="h in r.hits"
              :key="`${r.sourceId}-${h.sourceVodId}`"
              class="flex items-center gap-[var(--jc-space-3)]"
            >
              <div class="w-[40px] h-[54px] shrink-0 rounded-[var(--jc-radius-sm)] overflow-hidden bg-surface">
                <BaseImage v-if="h.cover" :src="h.cover" :alt="h.name" ratio="3/4" />
              </div>
              <div class="min-w-0 flex-1 flex flex-col gap-[2px]">
                <span class="text-sm text-primary truncate">{{ h.name }}</span>
                <span class="text-xs text-muted truncate">
                  {{ [h.year || '', h.typeName, h.remarks].filter(Boolean).join(' · ') || '—' }}
                </span>
                <span class="text-xs text-muted">{{ h.episodes }} 集</span>
              </div>
              <ManageButton
                variant="outline"
                size="sm"
                :loading="hitCollecting[`${r.sourceId}:${h.sourceVodId}`]"
                @click="collectHit(r.sourceId, r.sourceName, h)"
              >
                <BaseIcon name="magic" size="16px" /> 采集
              </ManageButton>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 同名多版本选择对话框(详情页与新增页共用组件) -->
    <SourceCandidateDialog
      v-model:visible="candidateDialog.visible"
      :candidates="candidateDialog.list"
      :source-name="candidateDialog.sourceName"
      :picking="candidatePicking"
      @pick="onPickCandidate"
    />

    <!-- ============ 手动新增影片 ============ -->
    <section class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)]">
      <header class="mb-[var(--jc-space-5)]">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">手动新增影片</h2>
        <p class="text-sm text-muted">手动录入一部影片信息（采集源会自动同步，此页用于补录）</p>
      </header>

      <form
        class="grid grid-cols-1 md:grid-cols-2 gap-[var(--jc-space-5)]"
        @submit.prevent="submit"
      >
        <ManageFormField label="影片名" required :error="nameError">
          <ManageInput v-model="form.name" />
        </ManageFormField>
        <ManageFormField label="英文名">
          <ManageInput v-model="form.enName" />
        </ManageFormField>

        <ManageFormField label="顶级分类 (Pid)">
          <div class="flex flex-col gap-[var(--jc-space-2)]">
            <ManageSelect v-model="form.pid" :options="pidOptions" aria-label="顶级分类" />
            <div v-if="treeError" class="flex items-center gap-[var(--jc-space-2)] text-xs text-[var(--jc-danger)] flex-wrap">
              <span>分类树加载失败：{{ treeError }}</span>
              <ManageButton variant="ghost" size="sm" @click="loadTree">重试</ManageButton>
            </div>
          </div>
        </ManageFormField>
        <ManageFormField label="子分类 (Cid)">
          <ManageInput v-model="form.cid" type="number" />
        </ManageFormField>

        <ManageFormField label="海报" class="md:col-span-2">
          <ManageImageUpload v-model="form.picture" ratio="2/3" placeholder="或粘贴图片 URL" />
        </ManageFormField>

        <ManageFormField label="年份" :error="yearError">
          <ManageInput v-model="form.year" placeholder="2024" />
        </ManageFormField>
        <ManageFormField label="地区">
          <ManageInput v-model="form.area" placeholder="中国大陆" />
        </ManageFormField>
        <ManageFormField label="导演">
          <ManageInput v-model="form.director" />
        </ManageFormField>
        <ManageFormField label="主演">
          <ManageInput v-model="form.actor" placeholder="逗号分隔" />
        </ManageFormField>
        <ManageFormField label="标签">
          <ManageInput v-model="form.classTag" placeholder="逗号分隔" />
        </ManageFormField>

        <ManageFormField label="剧情简介" class="md:col-span-2">
          <ManageTextarea v-model="form.content" :rows="6" />
        </ManageFormField>

        <div class="md:col-span-2 flex justify-end gap-[var(--jc-space-3)]">
          <ManageButton variant="ghost" type="button" @click="router.back()">取消</ManageButton>
          <ManageButton variant="gradient" type="submit" :loading="busy">保存</ManageButton>
        </div>
      </form>
    </section>
  </div>
</template>
