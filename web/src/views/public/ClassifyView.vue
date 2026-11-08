<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { storeToRefs } from 'pinia'
import * as filmApi from '@/api/film'
import type { Card, ClassifyData } from '@/types/film'
import FilmGrid from '@/components/film/FilmGrid.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import SectionHeader from '@/components/base/SectionHeader.vue'
import TvSectionHeader from '@/components/base/TvSectionHeader.vue'
import { useQuerySync } from '@/composables/useQuerySync'
import { useAbortable } from '@/composables/useAbortable'
import { useNavStore } from '@/stores'
import { useViewMode } from '@/composables/useViewMode'

/**
 * /filmClassify?Pid=xxx
 * 分类首页：排行榜 / 最新上线 / 高分榜(scoredCount>0 才显示)。
 * 「最近更新」2026-11-07 下线(update_stamp 是收录时间, 不具参考价值, 用户拍板)。
 */

const { params } = useQuerySync<{ Pid: string }>(
  { Pid: '' },
  {
    path: '/filmClassify',
    onChange: () => {
      void load()
    }
  }
)

const data = ref<ClassifyData>({
  title: undefined,
  news: [],
  top: [],
  score: [],
  scoredCount: 0
})
const loading = ref<boolean>(true)
const loaded = ref<boolean>(false)
const errorMsg = ref<string>('')

const abortable = useAbortable()

// 顶级分类切换 (从 nav store 取全部一级分类, 移动端也能切, 不再只有电影)
const navStore = useNavStore()
const { list: navCats } = storeToRefs(navStore)
const currentPid = computed(() => (params.value.Pid || '').trim())

// 非移动端 (desktop/tv) 顶部 Header 已显示分类导航, 页内胶囊冗余 → 仅移动端显示
const { isMobile, isTV } = useViewMode()

/**
 * 模块行数限制: 每个模块最多显示 3 行(不同分辨率一致)。
 * 列数需与 CSS 对齐 —— 桌面/移动走 theme.css --jc-list-cols 阶梯(3/4/5/6/7),
 * TV 分支本页固定 6 列([data-mode='tv'] .jc-tv-grid), 故按视口宽度计算并监听 resize。
 */
const MAX_MODULE_ROWS = 3
const gridCols = ref(6)
function calcGridCols(): number {
  if (isTV.value) return 6
  const w = window.innerWidth
  if (w < 480) return 3
  if (w < 768) return 4
  if (w < 1024) return 5
  if (w < 1152) return 6
  return 7
}
function updateGridCols(): void {
  gridCols.value = calcGridCols()
}
updateGridCols()
window.addEventListener('resize', updateGridCols)
onBeforeUnmount(() => window.removeEventListener('resize', updateGridCols))

/** 截断到最多 3 行(3 × 当前列数) */
function limitToRows(items: Card[] | undefined): Card[] {
  if (!Array.isArray(items)) return []
  return items.slice(0, gridCols.value * MAX_MODULE_ROWS)
}

async function load(): Promise<void> {
  const pid = (params.value.Pid || '').trim()
  if (!pid) {
    errorMsg.value = '缺少分类参数 Pid'
    loading.value = false
    loaded.value = true
    return
  }
  loading.value = true
  errorMsg.value = ''
  abortable.refresh()
  try {
    const resp = await filmApi.getClassify(pid)
    data.value = resp
    loaded.value = true
  } catch (e) {
    if ((e as { name?: string })?.name === 'CanceledError') return
    errorMsg.value = '加载失败，请稍后重试'
    loaded.value = true
  } finally {
    loading.value = false
  }
}

void load()

function moreLink(sort: string): {
  path: string
  query: Record<string, string>
} {
  return {
    path: '/filmClassifySearch',
    query: { Pid: String(data.value.title?.id || params.value.Pid), Sort: sort }
  }
}

function isReady(items: Card[] | undefined): boolean {
  return Array.isArray(items) && items.length > 0
}

// ─── TV 专用 ──────────────────────────────────────────────
// (顶部胶囊导航已含一级分类, 分类页不再重复 chip bar)

// 标题区目标分类 id (title.id 优先, 否则取 URL Pid)
const tvTitlePid = computed(() =>
  String(data.value.title?.id || params.value.Pid)
)

/**
 * 三段网格配置: **排行榜在最前**(用户定稿), 之后 最新上线 / 高分榜。
 * 「最近更新」已于 2026-11-07 整体下线(用户拍板) —— update_stamp 是收录时间, 不具参考价值;
 * 后端 ClassifyData/ClassifyResp 已同步撤掉 recent 字段, 筛选页排序标签里也移到末位。
 * 每个模块最多 3 行(limitToRows, 3 × 当前列数)。
 * sort 值对齐后端 orderForSpec 白名单(hot=hot_score 降序 / score=db_score 降序 / latest=year+pub_date) ——
 * 旧值 hits/release_stamp 后端仍兼容, 但新代码一律用新值。
 * 高分榜只在 scoredCount>0 时进列表(后端为 0 时不返回该分区, 运行时探测代替分类白名单)。
 */
const sections = computed(() => {
  const secs = [
    {
      key: 'top',
      title: '排行榜',
      sub: '按热度排序',
      items: limitToRows(data.value.top),
      sort: 'hot'
    },
    {
      key: 'news',
      title: '最新上线',
      sub: '每日更新',
      items: limitToRows(data.value.news),
      sort: 'latest'
    }
  ]
  if (data.value.scoredCount > 0) {
    secs.push({
      key: 'score',
      title: '高分榜',
      sub: '豆瓣评分优先',
      items: limitToRows(data.value.score),
      sort: 'score'
    })
  }
  return secs
})

const tvAllEmpty = computed(
  () =>
    !isReady(data.value.news) &&
    !isReady(data.value.top) &&
    !isReady(data.value.score)
)
</script>

<template>
  <!-- ══════════════════ TV (雷鸟卡片式) 分支 ══════════════════ -->
  <div
    v-if="isTV"
    class="jc-classify-tv container-page py-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-6)]"
  >
    <!-- 顶部一级分类已由全局胶囊导航提供, 分类页不再重复 chip bar -->

    <!-- 标题区: 「分类」当前 + 「分类库」入口 (filmClassifySearch)
         (§C8#5 收编 TvSectionHeader, 异形头走默认槽自绘: 渐变标题/分隔符带自有类, .t/.s 保留取迁入样式) -->
    <TvSectionHeader v-if="data.title?.name" class="jc-classify-tv__title">
      <span class="jc-classify-tv__title-grad t">{{ data.title?.name }}</span>
      <span class="jc-classify-tv__title-sep s" aria-hidden="true">|</span>
      <RouterLink
        v-slot="{ navigate, href }"
        :to="{ path: '/filmClassifySearch', query: { Pid: tvTitlePid } }"
        custom
      >
        <a
          class="jc-tv-chip jc-classify-tv__lib"
          :href="href"
          data-focusable="true"
          tabindex="0"
          @click="navigate"
          @keydown.enter="() => navigate()"
        >
          {{ data.title?.name }}库 ›
        </a>
      </RouterLink>
    </TvSectionHeader>

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请检查链接中的 Pid 参数"
    />

    <!-- 骨架 -->
    <div
      v-else-if="loading && !loaded"
      class="flex flex-col gap-[var(--jc-space-6)]"
    >
      <div v-for="n in 3" :key="n" class="flex flex-col gap-[var(--jc-space-4)]">
        <BaseSkeleton width="240px" height="36px" />
        <SkeletonGrid :count="6" layout="poster-tv" height="280px" ratio="3/4" />
      </div>
    </div>

    <!-- 分段网格 (排行榜 / 最新上线 / 高分榜) -->
    <template v-else>
      <section
        v-for="sec in sections"
        v-show="isReady(sec.items)"
        :key="sec.key"
        class="jc-classify-tv__section"
      >
        <TvSectionHeader :title="sec.title" :sub="sec.sub">
          <template #more>
            <RouterLink
              v-slot="{ navigate, href }"
              :to="moreLink(sec.sort)"
              custom
            >
              <a
                class="jc-tv-more"
                :href="href"
                data-focusable="true"
                tabindex="0"
                @click="navigate"
                @keydown.enter="() => navigate()"
              >
                更多 ›
              </a>
            </RouterLink>
          </template>
        </TvSectionHeader>
        <div class="jc-tv-grid">
          <FilmCard
            v-for="item in sec.items"
            :key="item.mid"
            :item="item"
          />
        </div>
      </section>

      <BaseEmpty
        v-if="tvAllEmpty"
        title="该分类暂无影片"
        description="切换到分类库浏览更多内容"
      >
        <template #action>
          <BaseButton
            v-if="data.title?.id"
            variant="gradient"
            size="md"
            @click="
              $router.push({
                path: '/filmClassifySearch',
                query: { Pid: String(data.title?.id) }
              })
            "
          >
            前往 {{ data.title?.name }}库
          </BaseButton>
        </template>
      </BaseEmpty>
    </template>
  </div>

  <!-- ══════════════════ 桌面 / 移动 原始分支 ══════════════════ -->
  <div v-else class="jc-classify container-page py-[var(--jc-space-6)]">
    <!-- 顶级分类切换 (电影 / 电视剧 / 综艺 / 动漫 …) — 仅移动端显示;
         非移动端顶部 Header 已有分类导航, 此处胶囊冗余隐藏 -->
    <nav
      v-if="navCats.length && isMobile"
      class="jc-classify__cats flex flex-wrap gap-[var(--jc-space-2)] mb-[var(--jc-space-6)]"
      aria-label="影视分类"
    >
      <RouterLink
        v-for="cat in navCats"
        :key="cat.id"
        :to="{ path: '/filmClassify', query: { Pid: String(cat.id) } }"
        class="jc-classify__cat"
        :class="String(cat.id) === currentPid ? 'jc-classify__cat--active' : ''"
        data-focusable="true"
        tabindex="0"
      >
        {{ cat.name }}
      </RouterLink>
    </nav>

    <!-- 顶部 title 切换 -->
    <header
      v-if="data.title?.name"
      class="jc-classify__title flex items-center gap-[var(--jc-space-3)] mb-[var(--jc-space-8)]"
    >
      <RouterLink
        :to="{ path: '/filmClassify', query: { Pid: String(data.title?.id) } }"
        class="jc-classify__title-active"
        data-focusable="true"
        tabindex="0"
      >
        {{ data.title?.name }}
      </RouterLink>
      <span class="jc-classify__title-divider" aria-hidden="true">|</span>
      <RouterLink
        :to="{ path: '/filmClassifySearch', query: { Pid: String(data.title?.id) } }"
        class="jc-classify__title-link"
        data-focusable="true"
        tabindex="0"
      >
        {{ data.title?.name }}库
      </RouterLink>
    </header>

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请检查链接中的 Pid 参数"
    />

    <!-- 骨架: 与网格版式同构 -->
    <div
      v-else-if="loading && !loaded"
      class="flex flex-col gap-[var(--jc-space-6)]"
    >
      <div v-for="n in 3" :key="n" class="flex flex-col gap-[var(--jc-space-3)]">
        <BaseSkeleton width="220px" height="32px" />
        <div
          class="grid gap-[var(--jc-list-gap)]"
          style="grid-template-columns: repeat(var(--jc-list-cols), minmax(0, 1fr))"
        >
          <SkeletonGrid :count="12" layout="items" height="0" ratio="3/4" />
        </div>
      </div>
    </div>

    <!-- 三段网格 (卡片摊开, 列数走全站统一阶梯 --jc-list-cols, 与 TV 分支同构) -->
    <div
      v-else
      class="flex flex-col gap-[var(--jc-space-6)]"
    >
      <section
        v-for="sec in sections"
        :key="sec.key"
        v-show="isReady(sec.items)"
      >
        <!-- 区块标题(W1e): 默认形态 = 本页原款(header/h2/更多链接逐字同) -->
        <SectionHeader :title="sec.title" :more-to="moreLink(sec.sort)" />
        <FilmGrid :items="sec.items" />
      </section>

      <BaseEmpty
        v-if="
          !isReady(data.news) &&
          !isReady(data.top) &&
          !isReady(data.score)
        "
        title="该分类暂无影片"
        description="切换到分类库浏览更多内容"
      >
        <template #action>
          <BaseButton
            v-if="data.title?.id"
            variant="gradient"
            size="md"
            @click="
              $router.push({
                path: '/filmClassifySearch',
                query: { Pid: String(data.title?.id) }
              })
            "
          >
            前往 {{ data.title?.name }}库
          </BaseButton>
        </template>
      </BaseEmpty>
    </div>
  </div>
</template>

<style scoped>
.jc-classify__cat {
  display: inline-flex;
  align-items: center;
  min-height: var(--jc-ctrl-h); /* 2026-11-05 三档统一: 原 36 定值作废 */
  padding: 0 var(--jc-ctrl-px);
  border-radius: var(--jc-radius-full);
  background-color: var(--jc-bg-elevated);
  color: var(--jc-text-secondary);
  font-size: var(--jc-ctrl-fs);
  font-weight: var(--jc-fw-medium);
  text-decoration: none;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-classify__cat:hover {
  color: var(--jc-text-primary);
  background-color: rgba(255, 255, 255, 0.08);
}
.jc-classify__cat--active {
  background-image: var(--jc-brand-gradient);
  color: #fff;
  font-weight: var(--jc-fw-semibold);
}
.jc-classify__cat:focus,
.jc-classify__cat:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}

.jc-classify__title {
  flex-wrap: wrap;
}
.jc-classify__title-active,
.jc-classify__title-link {
  text-decoration: none;
  font-size: var(--jc-fs-2xl);
  font-weight: var(--jc-fw-bold);
  outline: none;
  transition: opacity var(--jc-dur-fast) var(--jc-ease-standard);
  border-radius: var(--jc-radius-sm);
}

.jc-classify__title-active {
  background-image: var(--jc-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}

.jc-classify__title-link {
  color: var(--jc-text-secondary);
}
.jc-classify__title-link:hover,
.jc-classify__title-link:focus-visible {
  color: var(--jc-text-primary);
}
.jc-classify__title-active:focus-visible,
.jc-classify__title-link:focus-visible {
  box-shadow: var(--jc-shadow-focus-ring);
}

.jc-classify__title-divider {
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-lg);
}

@media (max-width: 767px) {
  .jc-classify__title-active,
  .jc-classify__title-link {
    font-size: var(--jc-fs-xl);
  }
}

/* ─── TV 分支局部修饰 (chrome 主体在全局 tv-cards.css 的 jc-tv-*) ─── */
.jc-classify-tv__title {
  align-items: center;
}
/* 页头标题字号**不在这里给** —— 该元素同时带 jc-tv-sec 的 .t 类, 由 TvSectionHeader
   组件样式(原 tv-cards.css 全局规则整类迁入, 值/特异性不变)定档 = --jc-fs-base;
   这里再写一次会被它按特异性压掉, 只会误导后来人(筛选页就是照它抄才抄出 2xl 的不一致). */
.jc-classify-tv__title-grad {
  font-weight: var(--jc-fw-bold);
  background-image: var(--jc-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}
.jc-classify-tv__title-sep {
  font-size: var(--jc-fs-base);
  color: var(--jc-text-muted);
  margin: 0 var(--jc-space-3);
}
.jc-classify-tv__lib {
  height: 32px;
  padding: 0 10px;
  font-size: var(--jc-fs-xs);
}
.jc-classify-tv__section {
  display: flex;
  flex-direction: column;
}

/* 分类页 TV: 影片网格固定每行 6 个 (覆盖全局 jc-tv-grid 的 auto-fill) */
[data-mode='tv'] .jc-tv-grid {
  grid-template-columns: repeat(6, minmax(0, 1fr));
}
</style>
