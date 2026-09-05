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
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import SourceCandidateDialog from '@/components/manage/SourceCandidateDialog.vue'

const router = useRouter()
const tree = ref<FilmClass[]>([])
const submitting = ref(false)
const uploading = ref(false)
const uploadProgress = ref(0)
const fileInput = ref<HTMLInputElement | null>(null)

// ============ 按片名搜索站点影片 + 采集 ============
const collectSources = ref<CollectSource[]>([])
const spiderKeyword = ref('')
const searching = ref(false)
const collecting = ref(false)
const results = ref<SourceFilmResult[]>([])
const searched = ref(false)
/** 选中的源 id 集合(供采集勾选) */
const checked = ref<Record<string, boolean>>({})

/** 已勾选的源 id 列表 */
const checkedIds = computed(() =>
  collectSources.value.filter((s) => checked.value[s.id]).map((s) => s.id)
)

function toggleAll(on: boolean): void {
  const next: Record<string, boolean> = {}
  for (const s of collectSources.value) next[s.id] = on
  checked.value = next
}

/** 搜索各源命中影片(预览不落库) */
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

/** 采集: ids 为空数组表示采集全部启用源 */
async function collect(ids: string[]): Promise<void> {
  const kw = spiderKeyword.value.trim()
  if (!kw) {
    toast('error', '请输入片名关键字')
    return
  }
  collecting.value = true
  try {
    const res = normalizeCollectResult(
      await manageApi.film.collectFilm({ keyword: kw, sources: ids })
    )
    results.value = res
    searched.value = true
    reportCollect(res)
  } catch {
    // http 拦截器已 toast
  } finally {
    collecting.value = false
  }
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

/** 命中条目精确采集: 用户已亲选具体版本 → collectFilm({ sourceId, vodId }) */
async function collectHit(sourceId: string, sourceName: string, h: SourceSearchHit): Promise<void> {
  const key = hitKey(sourceId, h.sourceVodId)
  hitCollecting.value[key] = true
  try {
    const res = await manageApi.film.collectFilm({ sourceId, vodId: h.sourceVodId })
    const list = normalizeCollectResult(res)
    const hit = list.find((r) => r.sourceId === sourceId) ?? list[0]
    if (hit?.error) {
      toast('error', `${sourceName} 采集失败: ${hit.error}`)
    } else if (hit && hit.collected > 0) {
      toast('success', `${sourceName} · ${h.name} 采集完成, 入库 ${hit.collected} 集`)
    } else {
      toast('error', `${sourceName} · ${h.name} 采集未入库`)
    }
  } catch {
    // http 拦截器已 toast
  } finally {
    hitCollecting.value[key] = false
  }
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

/** 候选弹框选定某版本 → 精确采集 */
async function onPickCandidate(h: SourceSearchHit): Promise<void> {
  const { sourceId, sourceName } = candidateDialog.value
  if (!sourceId) return
  candidatePicking.value = true
  try {
    const res = await manageApi.film.collectFilm({ sourceId, vodId: h.sourceVodId })
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
  } catch {
    // http 拦截器已 toast
  } finally {
    candidatePicking.value = false
  }
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

onMounted(async () => {
  try {
    tree.value = await manageApi.film.classTree()
  } catch {
    /* 忽略 */
  }
  try {
    collectSources.value = await manageApi.collect.list()
  } catch {
    /* 忽略 */
  }
})

async function handleUpload(e: Event): Promise<void> {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  uploading.value = true
  uploadProgress.value = 0
  try {
    const fd = new FormData()
    fd.append('file', file)
    const res = await manageApi.file.upload(fd, (p) => {
      uploadProgress.value = p
    })
    form.picture = res
  } finally {
    uploading.value = false
    // 清空 input value 允许重复选择同一文件
    target.value = ''
  }
}

function pickFile(): void {
  fileInput.value?.click()
}

async function submit(): Promise<void> {
  submitting.value = true
  try {
    await manageApi.film.add({
      name: form.name,
      enName: form.enName,
      cid: form.cid,
      pid: form.pid,
      cover: form.picture,
      area: form.area,
      year: Number(form.year) || 0,
      director: form.director,
      actor: form.actor,
      classTag: form.classTag,
      content: form.content
    })
    router.push('/manage/film')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex flex-col gap-[var(--gf-space-6)] max-w-[960px]">
    <!-- ============ 按片名搜索站点影片 + 采集 ============ -->
    <section class="bg-surface rounded-card shadow-card p-[var(--gf-space-6)]">
      <header class="mb-[var(--gf-space-4)]">
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">按片名采集</h2>
        <p class="text-sm text-muted">输入片名搜索各采集源命中结果，勾选源后采集入库（预览不落库）</p>
      </header>

      <!-- 搜索栏 -->
      <div class="flex gap-[var(--gf-space-2)] flex-wrap mb-[var(--gf-space-4)]">
        <ManageInput
          v-model="spiderKeyword"
          placeholder="影片名关键字，如 流浪地球"
          class="flex-1 min-w-[200px]"
          @keydown.enter="searchSpider"
        />
        <BaseButton variant="gradient" size="sm" :loading="searching" @click="searchSpider">
          <BaseIcon name="search" size="16px" /> 搜索
        </BaseButton>
      </div>

      <!-- 源勾选 + 采集动作 -->
      <div v-if="collectSources.length" class="flex flex-col gap-[var(--gf-space-3)] mb-[var(--gf-space-4)]">
        <div class="flex items-center gap-[var(--gf-space-3)] flex-wrap text-sm">
          <span class="text-muted">采集源：</span>
          <label
            v-for="s in collectSources"
            :key="s.id"
            class="flex items-center gap-[var(--gf-space-1)] cursor-pointer text-secondary"
          >
            <input
              v-model="checked[s.id]"
              type="checkbox"
              class="accent-[var(--gf-brand-primary)]"
              data-focusable="true"
            />
            <span>{{ s.name }}</span>
            <BaseTag v-if="s.grade === 0" variant="brand" size="xs">主站</BaseTag>
          </label>
          <div class="flex gap-[var(--gf-space-2)]">
            <BaseButton variant="ghost" size="sm" @click="toggleAll(true)">全选</BaseButton>
            <BaseButton variant="ghost" size="sm" @click="toggleAll(false)">清空</BaseButton>
          </div>
        </div>
        <div class="flex gap-[var(--gf-space-2)] flex-wrap">
          <BaseButton variant="primary" size="sm" :loading="collecting" @click="collectChecked">
            <BaseIcon name="magic" size="16px" /> 采集选中源
          </BaseButton>
          <BaseButton variant="outline" size="sm" :loading="collecting" @click="collectAll">
            <BaseIcon name="magic" size="16px" /> 采集全部源
          </BaseButton>
        </div>
      </div>

      <!-- 搜索结果 -->
      <div v-if="searched" class="flex flex-col gap-[var(--gf-space-3)]">
        <BaseEmpty v-if="!results.length" description="各源均无命中" />
        <div
          v-for="r in results"
          :key="r.sourceId"
          class="border border-default rounded-[var(--gf-radius-md)] bg-elevated p-[var(--gf-space-3)]"
        >
          <div class="flex items-center gap-[var(--gf-space-2)] flex-wrap mb-[var(--gf-space-2)]">
            <span class="font-[var(--gf-fw-medium)] text-primary">{{ r.sourceName }}</span>
            <BaseTag v-if="r.master" variant="brand" size="xs">主站</BaseTag>
            <BaseTag v-if="r.collected > 0" variant="success" size="xs">已采 {{ r.collected }} 集</BaseTag>
            <span v-if="r.error" class="text-xs text-[var(--gf-danger)]">采集失败：{{ r.error }}</span>
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
            class="flex items-center gap-[var(--gf-space-2)] flex-wrap mb-[var(--gf-space-2)] text-xs text-muted"
          >
            <BaseIcon name="info" size="14px" />
            <span>该源同名多版本，请在下方命中列表点具体版本采集，或</span>
            <BaseButton variant="ghost" size="sm" @click="openCandidates(r)">选择版本</BaseButton>
          </div>

          <!-- 命中影片列表(每条可单独采集) -->
          <div v-if="r.hits && r.hits.length" class="flex flex-col gap-[var(--gf-space-2)]">
            <div
              v-for="h in r.hits"
              :key="`${r.sourceId}-${h.sourceVodId}`"
              class="flex items-center gap-[var(--gf-space-3)]"
            >
              <div class="w-[40px] h-[54px] shrink-0 rounded-[var(--gf-radius-sm)] overflow-hidden bg-surface">
                <BaseImage v-if="h.cover" :src="h.cover" :alt="h.name" ratio="3/4" />
              </div>
              <div class="min-w-0 flex-1 flex flex-col gap-[2px]">
                <span class="text-sm text-primary truncate">{{ h.name }}</span>
                <span class="text-xs text-muted truncate">
                  {{ [h.year || '', h.typeName, h.remarks].filter(Boolean).join(' · ') || '—' }}
                </span>
                <span class="text-xs text-muted">{{ h.episodes }} 集</span>
              </div>
              <BaseButton
                variant="outline"
                size="sm"
                :loading="hitCollecting[`${r.sourceId}:${h.sourceVodId}`]"
                @click="collectHit(r.sourceId, r.sourceName, h)"
              >
                <BaseIcon name="magic" size="16px" /> 采集
              </BaseButton>
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
    <section class="bg-surface rounded-card shadow-card p-[var(--gf-space-6)]">
      <header class="mb-[var(--gf-space-5)]">
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">手动新增影片</h2>
        <p class="text-sm text-muted">手动录入一部影片信息（采集源会自动同步，此页用于补录）</p>
      </header>

      <form
        class="grid grid-cols-1 md:grid-cols-2 gap-[var(--gf-space-5)]"
        @submit.prevent="submit"
      >
        <ManageFormField label="影片名" required>
          <ManageInput v-model="form.name" />
        </ManageFormField>
        <ManageFormField label="英文名">
          <ManageInput v-model="form.enName" />
        </ManageFormField>

        <ManageFormField label="顶级分类 (Pid)">
          <select
            v-model="form.pid"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-3)]"
            data-focusable="true"
          >
            <option :value="0">请选择</option>
            <option v-for="t in tree" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
        </ManageFormField>
        <ManageFormField label="子分类 (Cid)">
          <ManageInput v-model="form.cid" type="number" />
        </ManageFormField>

        <ManageFormField label="海报">
          <div class="flex items-center gap-[var(--gf-space-3)]">
            <div class="w-[80px] h-[110px] rounded-[var(--gf-radius-md)] overflow-hidden bg-elevated">
              <BaseImage v-if="form.picture" :src="form.picture" alt="poster" ratio="3/4" />
              <div v-else class="w-full h-full flex-center text-muted text-xs">暂无</div>
            </div>
            <div class="flex flex-col gap-[var(--gf-space-2)]">
              <input
                ref="fileInput"
                type="file"
                accept="image/*"
                class="hidden"
                @change="handleUpload"
              />
              <BaseButton
                variant="ghost"
                size="sm"
                type="button"
                :loading="uploading"
                @click="pickFile"
              >
                <BaseIcon name="upload" size="16px" />
                {{ uploading ? `上传中 ${Math.round(uploadProgress * 100)}%` : '选择图片' }}
              </BaseButton>
              <ManageInput v-model="form.picture" placeholder="或粘贴图片 URL" />
            </div>
          </div>
        </ManageFormField>

        <ManageFormField label="年份">
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

        <div class="md:col-span-2 flex justify-end gap-[var(--gf-space-3)]">
          <BaseButton variant="ghost" type="button" @click="router.back()">取消</BaseButton>
          <BaseButton variant="gradient" type="submit" :loading="submitting">保存</BaseButton>
        </div>
      </form>
    </section>
  </div>
</template>
