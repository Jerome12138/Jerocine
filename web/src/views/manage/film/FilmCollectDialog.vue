<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { normalizeCollectResult, reportCollectResults } from '@/api/manage/film'
import type { CollectSource, SourceFilmResult, SourceSearchHit } from '@/types/manage'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageCheckbox from '@/components/manage/ManageCheckbox.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import SourceCandidateDialog from '@/components/manage/SourceCandidateDialog.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 采集新片弹窗（原独立页 views/manage/film/FilmAddView.vue 的「按片名采集」段收编而来）。
 *
 * 与「手动新增」拆成两个入口：本弹窗只做按片名搜索各采集源 + 采集入库。
 * 搜索固定覆盖全部启用源（预览不落库）；勾选源仅作用于采集入库。
 * 采集到内容后 emit('saved')，由影片列表刷新；同名多版本复用 SourceCandidateDialog。
 */
const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

// 写操作统一 useAsyncAction: busy 并发拦截 + 失败 toast 单点; 请求一律 silent
const { busy, run } = useAsyncAction()

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

/** 加载采集源: 失败置错误态+重试入口, 不再静默吞掉 */
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

/** 采集: ids 为空数组表示采集全部启用源。silent=true 自管结果提示(业务成败是
 *  HTTP 200 里的 error 字段, 由 reportCollectResults 汇报), 异常 toast 由 run 单点 */
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
      reportCollectResults(res)
      if (res.some((r) => !r.error && (r.collected ?? 0) > 0)) emit('saved')
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
        emit('saved')
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
        emit('saved')
      } else {
        toast('error', `${sourceName} · ${h.name} 采集未入库`)
      }
    }
  })
  candidatePicking.value = false
}

watch(
  visible,
  (v) => {
    if (v) void loadSources()
  },
  { immediate: true }
)
</script>

<template>
  <ManageSheet v-model="visible" title="采集新片" width="880px">
    <div class="flex flex-col gap-[var(--jc-space-4)]">
      <p class="text-sm text-muted">
        输入片名搜索各启用采集源的命中结果（预览不落库，搜索固定覆盖全部启用源）；
        勾选源仅作用于采集入库
      </p>

      <!-- 搜索栏 -->
      <div class="flex gap-[var(--jc-space-2)] flex-wrap">
        <ManageInput
          v-model="spiderKeyword"
          placeholder="影片名关键字，如 流浪地球"
          class="flex-1 min-w-[200px]"
          @keydown.enter="searchSpider"
        />
        <ManageButton variant="gradient" :loading="searching" @click="searchSpider">
          <BaseIcon name="search" size="16px" /> 搜索
        </ManageButton>
      </div>

      <!-- 源列表加载失败: 显式提示+重试(此前静默吞掉, 整个勾选区块无声消失) -->
      <div
        v-if="sourcesError"
        class="flex items-center gap-[var(--jc-space-2)] flex-wrap text-sm text-[var(--jc-warning)]"
      >
        <BaseIcon name="info" size="14px" />
        <span>采集源列表加载失败：{{ sourcesError }}</span>
        <ManageButton variant="ghost" @click="loadSources">重试</ManageButton>
      </div>

      <!-- 源勾选 + 采集动作 -->
      <div v-else-if="collectSources.length" class="flex flex-col gap-[var(--jc-space-3)]">
        <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap text-sm">
          <span class="text-muted">采集源：</span>
          <!-- ManageCheckbox 统一勾选框: 停用源禁用态(半透明+不可点)由组件 disabled 承担;
               checked 记录值可能是 undefined(未勾过), 显式归一为 boolean 对齐组件 modelValue 类型 -->
          <ManageCheckbox
            v-for="s in collectSources"
            :key="s.id"
            class="text-secondary"
            :model-value="checked[s.id] === true"
            :disabled="!s.state"
            @update:model-value="checked[s.id] = $event"
          >
            <span>{{ s.name }}</span>
            <BaseTag v-if="s.grade === 0" variant="brand" size="xs">主站</BaseTag>
            <BaseTag v-if="!s.state" variant="warning" size="xs">已停用</BaseTag>
          </ManageCheckbox>
          <div class="flex gap-[var(--jc-space-2)]">
            <ManageButton variant="ghost" size="sm" :disabled="!enabledSourceCount" @click="toggleAll(true)">
              全选
            </ManageButton>
            <ManageButton variant="ghost" size="sm" @click="toggleAll(false)">清空</ManageButton>
          </div>
        </div>
        <div class="flex gap-[var(--jc-space-2)] flex-wrap">
          <ManageButton
            variant="primary"
            :loading="busy"
            :disabled="!enabledSourceCount"
            @click="collectChecked"
          >
            <BaseIcon name="magic" size="16px" /> 采集选中源
          </ManageButton>
          <ManageButton
            variant="outline"
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
                <span class="text-sm text-primary truncate">{{ h.name || `未知影片（#${h.sourceVodId}）` }}</span>
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
    </div>

    <template #footer>
      <ManageButton variant="ghost" @click="visible = false">关闭</ManageButton>
    </template>

    <!-- 同名多版本选择(复用组件; 自带弹层, 由 overlay 栈管理 ESC 层级) -->
    <SourceCandidateDialog
      v-model:visible="candidateDialog.visible"
      :candidates="candidateDialog.list"
      :source-name="candidateDialog.sourceName"
      :picking="candidatePicking"
      @pick="onPickCandidate"
    />
  </ManageSheet>
</template>
