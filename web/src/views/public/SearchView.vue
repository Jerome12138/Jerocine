<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as filmApi from '@/api/film'
import type { Card } from '@/types/film'
import type { PageMeta } from '@/types/api'
import FilmGrid from '@/components/film/FilmGrid.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import TvOnScreenKeyboard from '@/components/film/TvOnScreenKeyboard.vue'
import { useQuerySync } from '@/composables/useQuerySync'
import { useAbortable } from '@/composables/useAbortable'
import { useViewMode } from '@/composables/useViewMode'
import { useSearchHistory } from '@/composables/useSearchHistory'
import { useNavStore } from '@/stores'

/**
 * /search?search=xxx&current=1
 * STORY-011 关键字搜索页：顶部搜索框 + 结果列表 + 分页
 */

const router = useRouter()
const { isMobile, isTV } = useViewMode()

const { params, push } = useQuerySync<{ search: string; current: number }>(
  { search: '', current: 1 },
  {
    path: '/search',
    onChange: () => {
      void load()
    }
  }
)

const inputKeyword = ref<string>(params.value.search)
const list = ref<Card[]>([])
const page = ref<PageMeta>({ size: 10, current: 1, pageCount: 0, total: 0 })
const loading = ref(false)
const loaded = ref(false)
const errorMsg = ref<string>('')

const abortable = useAbortable()

const oldSearch = computed(() => params.value.search)

/** 搜索历史 (localStorage) + 热搜词 (用顶层分类名作为 mock 热词) */
const { history: searchHistory, add: addHistory, remove: removeHistory, clear: clearHistory } = useSearchHistory()
const navStore = useNavStore()
const hotKeywords = computed<string[]>(() =>
  (navStore.list || []).slice(0, 8).map((n) => n.name).filter(Boolean)
)

/** 最佳匹配 = 结果第一条; 其余进网格 */
const bestMatch = computed<Card | null>(() => list.value[0] ?? null)
const restList = computed<Card[]>(() => list.value.slice(1))

/** 选词/输入 → 应用搜索: TV 就地更新影片(不导航/不刷新页面), 桌面仍走 URL push。 */
function applySearchTerm(kw: string): void {
  const k = kw.trim()
  inputKeyword.value = kw
  if (isTV.value) {
    params.value = { ...params.value, search: k, current: 1 }
    void load()
  } else {
    void push({ search: k, current: 1 })
  }
}
function pickKeyword(kw: string): void {
  applySearchTerm(kw)
}

watch(
  () => params.value.search,
  (v) => {
    inputKeyword.value = v
  },
  { immediate: true }
)

/** TV 即时搜索: 每输入一个字母 debounce 350ms 自动搜索(无需按确认) */
let liveTimer: number | null = null
watch(inputKeyword, (v) => {
  if (!isTV.value) return
  if (liveTimer !== null) window.clearTimeout(liveTimer)
  const kw = v.trim()
  liveTimer = window.setTimeout(() => {
    if (kw !== (params.value.search || '').trim()) {
      // TV: 就地更新, 不导航(避免整页"刷新"/滚动)
      params.value = { ...params.value, search: kw, current: 1 }
      void load()
    }
  }, 350)
})

async function load(): Promise<void> {
  const keyword = (params.value.search || '').trim()
  if (!keyword) {
    list.value = []
    page.value = { size: 10, current: 1, pageCount: 0, total: 0 }
    loaded.value = true
    return
  }
  loading.value = true
  errorMsg.value = ''
  const signal = abortable.refresh()
  try {
    const resp = await filmApi.getFilms(
      {
        keyword,
        page: params.value.current || 1
      },
      { signal }
    )
    list.value = resp.list ?? []
    page.value = resp.page ?? page.value
    loaded.value = true
  } catch (e) {
    // 401/网络错误已被 http 拦截器 toast；这里仅清空业务态
    if ((e as { name?: string })?.name === 'CanceledError') return
    list.value = []
    page.value = { size: 10, current: 1, pageCount: 0, total: 0 }
    errorMsg.value = ''
    loaded.value = true
  } finally {
    loading.value = false
  }
}

function submitSearch(): void {
  const k = inputKeyword.value.trim()
  if (!k) {
    if (params.value.search) applySearchTerm('')
    return
  }
  // 写入搜索历史 (重复会自动去重 + 置顶)
  addHistory(k)
  applySearchTerm(k)
}

function onPaginate(p: number): void {
  void push({ search: params.value.search, current: p })
  if (typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
}

function play(id: string | number): void {
  void router.push({
    path: '/play',
    query: { id: String(id), source: '0', episode: '0' }
  })
}

// 首次进入或直接刷新带 query 的 URL：触发一次 load
void load()
</script>

<template>
  <!-- ===================== TV(雷鸟卡片式)分支 ===================== -->
  <section v-if="isTV" class="gf-search-tv container-page">
    <div class="gf-search-tv__wrap">
      <!-- ============ 左栏: 输入条 + 字母键盘 + 热搜/历史 ============ -->
      <div class="gf-search-tv__left">
        <!-- 输入显示条 (对应 inputKeyword) -->
        <div class="gf-search-tv__inputbar">
          <BaseIcon name="search" size="22px" class="gf-search-tv__inputbar-ic" />
          <span class="gf-search-tv__kw">{{ inputKeyword || '输入关键字 / 拼音首字母' }}</span>
          <span class="gf-search-tv__caret" aria-hidden="true" />
        </div>

        <!-- 字母/数字虚拟键盘 (受控 v-model, ENTER 提交搜索) -->
        <TvOnScreenKeyboard v-model="inputKeyword" @enter="submitSearch" />

        <!-- 热门搜索 (hotKeywords, 取顶层分类名; 前 3 红角标) -->
        <div v-if="hotKeywords.length" class="gf-search-tv__block">
          <div class="gf-tv-sec">
            <span class="t">🔥 热门搜索</span>
            <span class="s">大家都在搜</span>
          </div>
          <div class="gf-search-tv__chiprow">
            <button
              v-for="(kw, i) in hotKeywords"
              :key="kw"
              type="button"
              class="gf-tv-chip"
              :class="i < 3 ? 'hot' : ''"
              data-focusable="true"
              tabindex="0"
              @click="pickKeyword(kw)"
            >
              <span v-if="i < 3" class="gf-search-tv__rank">{{ i + 1 }}</span>
              {{ kw }}
            </button>
          </div>
        </div>

        <!-- 历史搜索 (searchHistory, 支持清空) -->
        <div v-if="searchHistory.length" class="gf-search-tv__block">
          <div class="gf-tv-sec">
            <span class="t">🕘 历史搜索</span>
            <button
              type="button"
              class="gf-search-tv__clear"
              data-focusable="true"
              tabindex="0"
              @click="clearHistory"
            >
              清空
            </button>
          </div>
          <div class="gf-search-tv__chiprow">
            <span
              v-for="kw in searchHistory"
              :key="kw"
              class="gf-search-tv__hist"
            >
              <button
                type="button"
                class="gf-tv-chip"
                data-focusable="true"
                tabindex="0"
                @click="pickKeyword(kw)"
              >
                {{ kw }}
              </button>
              <button
                type="button"
                class="gf-search-tv__hist-remove"
                :aria-label="`移除 ${kw}`"
                data-focusable="true"
                tabindex="0"
                @click="removeHistory(kw)"
              >
                <BaseIcon name="close" size="16px" />
              </button>
            </span>
          </div>
        </div>
      </div>

      <!-- ============ 右栏: 实时联想 + 结果网格 ============ -->
      <div class="gf-search-tv__right">
        <!-- 已搜索: 联想词 + 结果网格 -->
        <template v-if="oldSearch">
          <!-- 结果区标题 -->
          <div class="gf-tv-sec">
            <span class="t">搜索结果</span>
            <span class="s">「{{ oldSearch }}」命中 {{ page.total }} 部</span>
          </div>

          <!-- loading 骨架 -->
          <div v-if="loading && list.length === 0" class="gf-search-tv__skeleton">
            <BaseSkeleton :count="4" height="280px" />
          </div>

          <!-- 结果网格: FilmCard(海报 + 下方片名), 复用搜索结果 list -->
          <div v-else-if="list.length > 0" class="gf-tv-grid">
            <FilmCard
              v-for="m in list"
              :key="String(m.mid)"
              :item="m"
              :show-title-below="true"
            />
          </div>

          <!-- 空状态 -->
          <div
            v-else-if="loaded"
            class="gf-tv-glass-card gf-search-tv__empty"
          >
            <div class="gf-search-tv__empty-title">未查询到对应影片</div>
            <div class="gf-search-tv__empty-desc">换一个关键词试试，或从首页分类发现内容</div>
            <RouterLink
              to="/index"
              class="gf-tv-btn cyan gf-search-tv__empty-btn"
              data-focusable="true"
              tabindex="0"
            >
              返回首页
            </RouterLink>
          </div>
        </template>

        <!-- 未搜索: 引导提示 -->
        <div v-else class="gf-tv-glass-card gf-search-tv__hint">
          <div class="gf-search-tv__hint-glyph" aria-hidden="true">
            <BaseIcon name="search" size="1em" />
          </div>
          <div class="gf-search-tv__hint-title">开始你的搜索</div>
          <div class="gf-search-tv__hint-desc">
            用遥控器在左侧键盘输入片名 / 拼音首字母，回车即搜
          </div>
        </div>
      </div>
    </div>
  </section>

  <!-- ===================== 桌面 / 移动分支 (原样保留) ===================== -->
  <div v-else class="gf-search container-page py-[var(--gf-space-6)]">
    <!-- 顶部搜索框 -->
    <div class="gf-search__form mx-auto max-w-[640px] mb-[var(--gf-space-8)]">
      <div class="gf-search__input-wrap">
        <BaseIcon name="search" size="20px" class="gf-search__icon" />
        <input
          v-model="inputKeyword"
          class="gf-search__input"
          type="search"
          placeholder="输入关键字搜索 动漫 / 剧集 / 电影"
          aria-label="搜索影片"
          @keydown.enter.prevent="submitSearch"
        />
        <BaseButton
          variant="gradient"
          size="md"
          class="gf-search__btn"
          aria-label="搜索"
          @click="submitSearch"
        >
          <template #icon>
            <BaseIcon name="search" size="18px" />
          </template>
          <span class="hidden sm:inline">搜索</span>
        </BaseButton>
      </div>
    </div>

    <!-- 结果区 -->
    <section v-if="oldSearch" class="gf-search__result">
      <header v-if="!loading" class="mb-[var(--gf-space-6)]">
        <h2
          class="text-[var(--gf-fs-xl)] font-[var(--gf-fw-bold)] text-primary mb-[var(--gf-space-1)]"
        >
          {{ oldSearch }}
        </h2>
        <p class="text-secondary text-[var(--gf-fs-sm)]">
          找到 <strong class="text-primary">{{ page.total }}</strong> 部与
          "{{ oldSearch }}" 相关的影视作品
        </p>
      </header>

      <!-- loading 骨架 -->
      <div v-if="loading && list.length === 0" class="flex flex-col gap-[var(--gf-space-4)]">
        <BaseSkeleton :count="5" height="180px" />
      </div>

      <!-- 最佳匹配大卡 (bilibili 风格首条放大) -->
      <article
        v-else-if="bestMatch && !loading"
        class="gf-search__best mb-[var(--gf-space-6)]"
      >
        <RouterLink
          :to="{ path: '/filmDetail', query: { link: String(bestMatch.mid) } }"
          class="gf-search__best-poster"
          data-focusable="true"
          tabindex="0"
          :aria-label="bestMatch.name"
        >
          <BaseImage :src="bestMatch.cover" :alt="bestMatch.name" ratio="3/4" fit="cover" />
        </RouterLink>
        <div class="gf-search__best-info">
          <BaseTag variant="brand" size="xs" class="gf-search__best-badge">
            最佳匹配
          </BaseTag>
          <h3 class="gf-search__best-name">{{ bestMatch.name }}</h3>
          <div class="gf-search__best-tags">
            <BaseTag v-if="bestMatch.cName" variant="purple" size="sm">
              {{ bestMatch.cName }}
            </BaseTag>
            <BaseTag v-if="bestMatch.year" size="sm">{{ bestMatch.year }}</BaseTag>
            <BaseTag v-if="bestMatch.area" size="sm">{{ bestMatch.area }}</BaseTag>
            <BaseTag v-if="bestMatch.remarks" size="sm">{{ bestMatch.remarks }}</BaseTag>
          </div>
          <div class="gf-search__best-actions flex gap-[var(--gf-space-3)] mt-[var(--gf-space-3)] flex-wrap">
            <BaseButton variant="gradient" size="md" @click.stop="play(bestMatch.mid)">
              <template #icon>
                <BaseIcon name="play" size="16px" />
              </template>
              立即观看
            </BaseButton>
            <RouterLink
              :to="{ path: '/filmDetail', query: { link: String(bestMatch.mid) } }"
              class="gf-search__best-detail"
            >
              查看详情 ›
            </RouterLink>
          </div>
        </div>
      </article>

      <!-- 移动端：列表卡片 (排除已在最佳匹配大卡里的首条) -->
      <div v-if="!loading && isMobile && restList.length > 0" class="gf-search__list-mobile flex flex-col gap-[var(--gf-space-4)]">
        <article
          v-for="m in restList"
          :key="String(m.mid)"
          class="gf-search__row-mobile"
        >
          <RouterLink
            :to="{ path: '/filmDetail', query: { link: String(m.mid) } }"
            class="gf-search__poster-link"
            data-focusable="true"
            tabindex="0"
            :aria-label="m.name"
          >
            <BaseImage :src="m.cover" :alt="m.name" ratio="3/4" fit="cover" />
          </RouterLink>
          <div class="gf-search__info">
            <h3 class="gf-search__name">{{ m.name }}</h3>
            <div class="gf-search__tags">
              <BaseTag v-if="m.cName" variant="brand" size="xs">{{ m.cName }}</BaseTag>
              <BaseTag v-if="m.year" size="xs">{{ m.year }}</BaseTag>
              <BaseTag v-if="m.area" size="xs">{{ m.area }}</BaseTag>
            </div>
            <p v-if="m.remarks" class="gf-search__line">{{ m.remarks }}</p>
            <BaseButton
              variant="gradient"
              size="sm"
              class="gf-search__play"
              @click.stop="play(m.mid)"
            >
              <template #icon>
                <BaseIcon name="play" size="14px" />
              </template>
              立即播放
            </BaseButton>
          </div>
        </article>
      </div>

      <!-- 桌面: 复用主页标准卡片(海报在上/文字在下), 文字不再被裁 -->
      <FilmGrid
        v-if="!loading && !isMobile && restList.length > 0"
        :items="restList"
      />

      <!-- 分页 -->
      <div
        v-if="!loading && page.total > 0"
        class="gf-search__pagination mt-[var(--gf-space-8)] flex justify-center"
      >
        <BasePagination
          :current="page.current || 1"
          :page-size="page.size || 10"
          :total="page.total"
          @change="onPaginate"
        />
      </div>

      <!-- 空状态 -->
      <BaseEmpty
        v-if="!loading && loaded && list.length === 0"
        title="未查询到对应影片"
        description="换一个关键词试试，或者从首页分类发现内容"
      >
        <template #action>
          <BaseButton variant="gradient" size="md" @click="router.push('/index')">
            返回首页
          </BaseButton>
        </template>
      </BaseEmpty>
    </section>

    <!-- 未输入关键字: 显示热搜词 + 历史搜索 -->
    <section v-else class="gf-search__intro flex flex-col gap-[var(--gf-space-8)]">
      <!-- 热搜词 -->
      <div v-if="hotKeywords.length" class="gf-search__hot">
        <header class="gf-search__intro-header">
          <h2 class="gf-search__intro-title">热门搜索</h2>
        </header>
        <div class="gf-search__chip-row">
          <button
            v-for="(kw, i) in hotKeywords"
            :key="kw"
            type="button"
            class="gf-search__chip"
            :class="i < 3 ? 'gf-search__chip--hot' : ''"
            @click="pickKeyword(kw)"
          >
            <span v-if="i < 3" class="gf-search__chip-rank">{{ i + 1 }}</span>
            {{ kw }}
          </button>
        </div>
      </div>

      <!-- 历史搜索 -->
      <div v-if="searchHistory.length" class="gf-search__history">
        <header class="gf-search__intro-header">
          <h2 class="gf-search__intro-title">历史搜索</h2>
          <button
            type="button"
            class="gf-search__intro-clear"
            @click="clearHistory"
          >
            清空
          </button>
        </header>
        <div class="gf-search__chip-row">
          <span
            v-for="kw in searchHistory"
            :key="kw"
            class="gf-search__chip-wrap"
          >
            <button
              type="button"
              class="gf-search__chip"
              @click="pickKeyword(kw)"
            >
              {{ kw }}
            </button>
            <button
              type="button"
              class="gf-search__chip-remove"
              :aria-label="`移除 ${kw}`"
              @click="removeHistory(kw)"
            >
              <BaseIcon name="close" size="12px" />
            </button>
          </span>
        </div>
      </div>

      <!-- 空白引导 -->
      <BaseEmpty
        v-if="!hotKeywords.length && !searchHistory.length"
        title="开始你的搜索"
        description="输入片名 / 演员 / 关键字，发现更多影视作品"
      />
    </section>
  </div>
</template>

<style scoped>
.gf-search__input-wrap {
  position: relative;
  display: flex;
  align-items: center;
  background-color: var(--gf-bg-elevated);
  border-radius: var(--gf-radius-full);
  padding: 6px 6px 6px var(--gf-space-5);
  border: 1px solid transparent;
  transition: border-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-search__input-wrap:focus-within {
  border-color: var(--gf-brand-primary);
  box-shadow: var(--gf-shadow-purple-glow);
}

.gf-search__icon {
  color: var(--gf-text-muted);
  flex-shrink: 0;
  margin-right: var(--gf-space-3);
}

.gf-search__input {
  flex: 1;
  height: 44px;
  background-color: transparent;
  border: none;
  outline: none;
  color: var(--gf-text-primary);
  font-size: var(--gf-fs-md);
  padding: 0;
  min-width: 0;
}

.gf-search__input::placeholder {
  color: var(--gf-text-muted);
}
/* 聚焦高亮交给外层 wrap 的 focus-within(紫边+柔光); input 本身不叠加全局 2px outline + 3px 环, 免得又粗又方 */
.gf-search__input:focus,
.gf-search__input:focus-visible {
  outline: none;
  box-shadow: none;
}

.gf-search__btn {
  flex-shrink: 0;
  margin-left: var(--gf-space-2);
}

/* 移动端列表行 (桌面已改用主页标准 FilmCard, 不再有 row-desktop) */
.gf-search__row-mobile {
  display: flex;
  gap: var(--gf-space-4);
  background-color: var(--gf-bg-elevated);
  border-radius: var(--gf-radius-lg);
  padding: var(--gf-space-3);
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-search__row-mobile:hover {
  background-color: rgba(255, 255, 255, 0.05);
}

.gf-search__poster-link {
  display: block;
  flex-shrink: 0;
  width: 120px;
  border-radius: var(--gf-radius-md);
  overflow: hidden;
  outline: none;
}
.gf-search__poster-link:focus-visible {
  box-shadow: var(--gf-shadow-focus-ring);
}

.gf-search__info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-2);
}

.gf-search__name {
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  line-height: var(--gf-lh-snug);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.gf-search__tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gf-space-1);
}

.gf-search__line {
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.gf-search__play {
  align-self: flex-start;
  margin-top: auto;
}

@media (max-width: 767px) {
  .gf-search__poster-link {
    width: 100px;
  }
  .gf-search__name {
    font-size: var(--gf-fs-md);
  }
}

/* 最佳匹配大卡 */
.gf-search__best {
  display: flex;
  gap: var(--gf-space-5);
  padding: var(--gf-space-5);
  background-image: linear-gradient(
    135deg,
    rgba(155, 73, 231, 0.12) 0%,
    rgba(74, 209, 229, 0.06) 100%
  );
  border-radius: var(--gf-radius-xl);
  border: 1px solid rgba(155, 73, 231, 0.2);
}
.gf-search__best-poster {
  flex-shrink: 0;
  width: 180px;
  border-radius: var(--gf-radius-md);
  overflow: hidden;
  outline: none;
}
.gf-search__best-poster:focus-visible {
  box-shadow: var(--gf-shadow-focus-ring);
}
.gf-search__best-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-2);
}
.gf-search__best-badge {
  align-self: flex-start;
}
.gf-search__best-name {
  font-size: var(--gf-fs-2xl);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  line-height: var(--gf-lh-snug);
  margin: 0;
}
.gf-search__best-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gf-space-1);
}
.gf-search__best-detail {
  align-self: center;
  color: var(--gf-text-link);
  text-decoration: none;
  font-size: var(--gf-fs-sm);
}
.gf-search__best-detail:hover {
  color: var(--gf-text-link-hover);
}
@media (max-width: 767px) {
  .gf-search__best {
    padding: var(--gf-space-3);
    gap: var(--gf-space-3);
  }
  .gf-search__best-poster {
    width: 100px;
  }
  .gf-search__best-name {
    font-size: var(--gf-fs-lg);
  }
}

/* 引导态 (热搜 / 历史) */
.gf-search__intro {
  max-width: 760px;
  margin-inline: auto;
}
.gf-search__intro-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--gf-space-3);
}
.gf-search__intro-title {
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
}
.gf-search__intro-clear {
  background: transparent;
  border: none;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-sm);
  cursor: pointer;
}
.gf-search__intro-clear:hover {
  color: var(--gf-text-secondary);
}
.gf-search__chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gf-space-2);
}
.gf-search__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: var(--gf-chip-height, 32px);
  padding: 0 var(--gf-chip-padding-x, 14px);
  border-radius: var(--gf-chip-radius, 9999px);
  background-color: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-search__chip:hover {
  background-color: rgba(255, 255, 255, 0.12);
  color: var(--gf-text-primary);
}
.gf-search__chip--hot {
  background-image: linear-gradient(135deg, rgba(229, 9, 20, 0.16), rgba(245, 158, 11, 0.08));
  border-color: rgba(229, 9, 20, 0.3);
  color: var(--gf-text-primary);
}
.gf-search__chip-rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  border-radius: 9999px;
  background-color: var(--gf-brand-primary);
  color: #fff;
  font-size: var(--gf-fs-xs);
  font-weight: var(--gf-fw-bold);
  margin-right: 4px;
}
.gf-search__chip-wrap {
  display: inline-flex;
  align-items: center;
  position: relative;
}
.gf-search__chip-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  margin-left: -8px;
  border-radius: 9999px;
  background-color: rgba(0, 0, 0, 0.4);
  border: none;
  color: var(--gf-text-muted);
  cursor: pointer;
  opacity: 0;
  transition: opacity var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-search__chip-wrap:hover .gf-search__chip-remove,
.gf-search__chip-wrap:focus-within .gf-search__chip-remove {
  opacity: 1;
}
.gf-search__chip-remove:hover {
  color: var(--gf-text-primary);
  background-color: rgba(0, 0, 0, 0.6);
}
</style>

<style>
[data-mode='tv'] .gf-search__input {
  font-size: var(--gf-fs-lg);
  height: 56px;
}
[data-mode='tv'] .gf-search__poster-link:focus-visible {
  box-shadow: var(--gf-shadow-focus-ring), var(--gf-shadow-hover);
}

/* ============ TV(雷鸟卡片式)专属布局, 全部 [data-mode='tv'] 限定 ============ */
[data-mode='tv'] .gf-search-tv {
  padding-block: var(--gf-tv-safe-y) var(--gf-space-16);
}
[data-mode='tv'] .gf-search-tv__wrap {
  display: grid;
  grid-template-columns: 300px 1fr;
  gap: var(--gf-space-6);
  align-items: start;
}
[data-mode='tv'] .gf-search-tv__left {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-5);
}
[data-mode='tv'] .gf-search-tv__right {
  min-width: 0;
}

/* 输入显示条 */
[data-mode='tv'] .gf-search-tv__inputbar {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 64px;
  padding: 0 18px;
  border-radius: 16px;
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid var(--gf-brand-cyan);
  box-shadow: 0 0 18px rgba(74, 209, 229, 0.18);
}
[data-mode='tv'] .gf-search-tv__inputbar-ic {
  color: var(--gf-brand-cyan);
  flex-shrink: 0;
}
[data-mode='tv'] .gf-search-tv__kw {
  flex: 1;
  min-width: 0;
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
[data-mode='tv'] .gf-search-tv__caret {
  width: 2px;
  height: 26px;
  flex-shrink: 0;
  background: var(--gf-brand-cyan);
  animation: gf-search-tv-blink 1.1s step-end infinite;
}
@keyframes gf-search-tv-blink {
  50% {
    opacity: 0;
  }
}

/* 操作按钮行 */
[data-mode='tv'] .gf-search-tv__ops {
  display: flex;
  gap: var(--gf-space-3);
  flex-wrap: wrap;
}

/* 区块容器 */
[data-mode='tv'] .gf-search-tv__block {
  display: flex;
  flex-direction: column;
}
[data-mode='tv'] .gf-search-tv__chiprow {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
/* 热门/历史搜索 chip 文字调小(用户反馈热门搜索文字偏大) */
[data-mode='tv'] .gf-search-tv__chiprow .gf-tv-chip {
  height: 40px;
  padding: 0 14px;
  font-size: var(--gf-fs-sm);
}
[data-mode='tv'] .gf-search-tv__rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  border-radius: 999px;
  background: var(--gf-brand-gradient);
  color: #fff;
  font-size: var(--gf-fs-xs);
  font-weight: var(--gf-fw-bold);
  margin-right: 6px;
}
[data-mode='tv'] .gf-search-tv__clear {
  margin-left: auto;
  background: transparent;
  border: none;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-sm);
  cursor: pointer;
}
[data-mode='tv'] .gf-search-tv__clear:focus,
[data-mode='tv'] .gf-search-tv__clear:focus-visible {
  outline: none;
  color: var(--gf-text-primary);
}

/* 历史 chip + 删除 */
[data-mode='tv'] .gf-search-tv__hist {
  display: inline-flex;
  align-items: center;
  position: relative;
}
[data-mode='tv'] .gf-search-tv__hist-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  min-height: 32px;
  margin-left: 4px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.45);
  border: none;
  color: var(--gf-text-muted);
  cursor: pointer;
}
[data-mode='tv'] .gf-search-tv__hist-remove:focus,
[data-mode='tv'] .gf-search-tv__hist-remove:focus-visible {
  outline: none;
  color: var(--gf-text-primary);
  background: rgba(0, 0, 0, 0.7);
}

/* 右栏: 联想词 */
[data-mode='tv'] .gf-search-tv__suggest {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: var(--gf-space-6);
}
[data-mode='tv'] .gf-search-tv__sg {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: 12px;
  background: var(--gf-tv-glass-soft, rgba(30, 30, 40, 0.7));
  border: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
  color: var(--gf-text-primary);
  font-size: var(--gf-fs-base);
  cursor: pointer;
  text-align: left;
  min-height: 48px;
}
[data-mode='tv'] .gf-search-tv__sg:focus,
[data-mode='tv'] .gf-search-tv__sg:focus-visible {
  outline: none;
}
[data-mode='tv'] .gf-search-tv__sg-ic {
  color: var(--gf-text-muted);
  flex-shrink: 0;
}
[data-mode='tv'] .gf-search-tv__sg-txt {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 右栏窄, 4 列更舒展 */
[data-mode='tv'] .gf-search-tv__right .gf-tv-grid {
  grid-template-columns: repeat(4, 1fr);
}

[data-mode='tv'] .gf-search-tv__skeleton {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--gf-space-6);
}

/* 空态 / 引导态 */
[data-mode='tv'] .gf-search-tv__empty,
[data-mode='tv'] .gf-search-tv__hint {
  text-align: center;
  padding: 56px 40px;
}
[data-mode='tv'] .gf-search-tv__hint-glyph {
  font-size: 56px;
  line-height: 1;
  color: var(--gf-text-muted);
  opacity: 0.6;
}
[data-mode='tv'] .gf-search-tv__empty-title,
[data-mode='tv'] .gf-search-tv__hint-title {
  margin-top: 16px;
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-search-tv__empty-desc,
[data-mode='tv'] .gf-search-tv__hint-desc {
  margin-top: 8px;
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
}
[data-mode='tv'] .gf-search-tv__empty-btn {
  margin-top: 20px;
  text-decoration: none;
}
</style>
