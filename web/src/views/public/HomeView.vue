<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { filmApi } from '@/api'
import { useViewMode } from '@/composables/useViewMode'
import { useGridRowsLimit } from '@/composables/useGridRowsLimit'
import { useUIStore, useHistoryStore, useUserStore } from '@/stores'
import { getLiveSources } from '@/api/live'
import HeroCarousel from '@/components/film/HeroCarousel.vue'
import FilmRow from '@/components/film/FilmRow.vue'
import ContinueWatchingRow from '@/components/film/ContinueWatchingRow.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import SectionHeader from '@/components/base/SectionHeader.vue'
import SkeletonGrid from '@/components/base/SkeletonGrid.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import type { Card, HeroItem, HomeBanner, HomeData, HomeRow } from '@/types/film'

/**
 * 首页 — STORY-008
 * - 调 GET /api/index 拿 { banner, content[] }
 * - HeroCarousel：后台配置的轮播优先，无配置时回退第一行 hot/latest 前 5
 * - 按 content[i] 渲染若干 FilmRow（title=nav.name，items=movies）
 * - PC 大屏右侧栏显示 hot 前 12 条（取 content[0].hot，兜底合并）
 * - 加载中：HeroCarousel 区骨架 + 多行骨架
 * - 失败：整页 BaseEmpty
 * - TV(雷鸟卡片式): 渲染组件 HomeTv.vue(方案 B, defineAsyncComponent 异步分片),
 *   本壳只做数据获取/派生与路由逻辑, 经 props 单向传入; 不另起接口。
 */

interface IndexState {
  loading: boolean
  errored: boolean
  data: HomeData | null
}

/** TV 渲染组件(方案 B 拆分): TV 模板/专属派生/TV CSS 全部在 HomeTv.vue, 仅 isTV 时挂载 */
const HomeTv = defineAsyncComponent(() => import('./HomeTv.vue'))

const { isTV } = useViewMode()
const { limitToRows } = useGridRowsLimit()

/** 000038: TV 金刚区直播卡仅按有无源决定显示(不受顶栏 liveNavEnabled 开关控制)。 */
const hasLiveSources = ref(false)

const state = ref<IndexState>({
  loading: true,
  errored: false,
  data: null
})

// 后端生效轮播位(手动配置 + 热榜自动补位, 与后台管理页同源); 空则按下面 heroItems 派生兜底
const banners = ref<HomeBanner[]>([])

// 后端 /home 已做区块化聚合(无独立 banner): 优先顶层跨类热榜(与「热门榜单」行同口径),
// 空时回退第一行的 hot/latest 前 5。
const heroItems = computed<Card[]>(() => {
  const data = state.value.data
  if (!data) return []
  if (data.hot?.length) return data.hot.slice(0, 5)
  for (const row of data.rows ?? []) {
    if (row.hot?.length) return row.hot.slice(0, 5)
    if (row.latest?.length) return row.latest.slice(0, 5)
  }
  return []
})

/**
 * 首屏大图最终数据源: 后端生效位优先(已含自动补位), 空时回退影片派生。
 * Slide → HeroItem 映射要点: image(横图) 映射到 poster(主视觉背景), poster(竖图) 映射到 cover(侧栏竖海报)。
 */
const heroSlides = computed<HeroItem[]>(() => {
  if (banners.value.length) {
    return banners.value.map((b) => ({
      mid: b.mid || undefined,
      name: b.name || '为你推荐',
      poster: b.image || '',
      cover: b.poster || '',
      remarks: b.subtitle || '',
      link: b.link || '',
      // 元信息(评分/类型标签/榜位)由后端按 mid 补齐; 纯自定义位(无 mid)缺省 → 描述行自动少这几项
      dbScore: b.dbScore,
      classTag: b.classTag,
      hotRank: b.hotRank,
      hotBoard: b.hotBoard
    }))
  }
  return heroItems.value
})

/** 热门榜单 — 后端已给全站跨类别混排(HomeData.hot), 直接消费。
 *  (旧版在此逐行拼各分类的 row.hot, 但 break 位置错误导致永远只取第 1 行 —— 已删,
 *   跨类别口径见 docs/榜单热度方案 §8.1①) */
const topRanking = computed<Card[]>(() => state.value.data?.hot ?? [])

/** 各分类行: items 取**该分类热榜**(rows[].hot, hot_score 降序) —— 与分类页「排行榜」同口径;
 *  空时回退最新上线。全站「热门榜单」行仍用 HomeData.hot(跨类别混排), 两者口径不同, 见后端 views.go 注释。 */
const rows = computed(() => {
  const data = state.value.data
  if (!data) return []
  return (data.rows ?? [])
    .filter((r) => r.hot?.length || r.latest?.length)
    .map((r) => ({ pid: r.nav.id, title: r.nav.name, items: r.hot?.length ? r.hot : r.latest }))
})

/** 原始区块(未过滤): TV「最新上架」面板/各分类专区按 rows[].hot/latest 派生(HomeTv) */
const homeRows = computed<HomeRow[]>(() => state.value.data?.rows ?? [])

/** 猜你喜欢: 合并各区块 latest 去重, 取 24 条(后端已排序, 不再前端 shuffle) */
const recommendGrid = computed<Card[]>(() => {
  const data = state.value.data
  if (!data) return []
  const merged: Card[] = []
  const seen = new Set<number>()
  for (const row of data.rows ?? []) {
    for (const it of row.latest ?? []) {
      if (!seen.has(it.mid)) {
        seen.add(it.mid)
        merged.push(it)
      }
    }
  }
  return merged.slice(0, 24)
})

/** 观看历史 / 登录态: TV 渲染组件的数据供给(「近期历史」前 3 条等派生已随组件下沉) */
const historyStore = useHistoryStore()
const { list: tvHistoryList } = storeToRefs(historyStore)
const userStore = useUserStore()
const { isLoggedIn } = storeToRefs(userStore)

async function loadIndex(): Promise<void> {
  state.value.loading = true
  state.value.errored = false
  try {
    const data = await filmApi.getHome()
    state.value = { loading: false, errored: false, data }
  } catch {
    state.value = { loading: false, errored: true, data: null }
  }
}

/** 轮播单独拉: 失败就静默回退影片派生, 不连累整页 */
async function loadBanners(): Promise<void> {
  try {
    banners.value = (await filmApi.getBanners()) ?? []
  } catch {
    banners.value = []
  }
}

/* ============ 页头实底门槛(2026-10-10) ============
 * 沉浸式轮播期间页头保持渐变阴影态, 轮播背景(含下探尾)滚出视口后才切毛玻璃实底。
 * 背景层高度由 HeroCarousel 的宽度等比模型驱动(盒宽÷真实比例, 1000 封顶,
 * 2026-10-13), 这里直接量真实图片层的文档底部 —— CSS 怎么改高度 JS 都不用跟着调。 */
const uiStore = useUIStore()
const heroWrapEl = ref<HTMLElement | null>(null)
function updateHeaderGate(): void {
  const el = heroWrapEl.value
  const img = el?.querySelector<HTMLElement>('.jc-hero__image')
  if (!el || !img || isTV.value) {
    uiStore.headerSolidAt = 0
    return
  }
  // 实底门槛 = 背景大图(含下探)文档底部。
  // 注: 玻璃淡出已改由 PublicHeader onScroll 实时量视口坐标(2026-10-11),
  // 这里只负责 headerSolidAt(页头无下边线的场景开关)。
  uiStore.headerSolidAt = Math.max(0, img.getBoundingClientRect().bottom + window.scrollY)
}
function onWinResize(): void {
  updateHeaderGate()
}
/** 测量加固(2026-10-11): nextTick 只保证 DOM 更新, 盒子高度(字体/轮播布局)
 * 可能还没定型 —— 双 rAF 等两帧布局稳定后再量, 避免量到偏小/偏大的门槛。 */
function remeasureHeaderGate(): void {
  nextTick(() => {
    requestAnimationFrame(() => requestAnimationFrame(updateHeaderGate))
  })
}
watch(
  () => heroSlides.value.length,
  remeasureHeaderGate
)
watch(isTV, updateHeaderGate)

onMounted(() => {
  loadIndex()
  loadBanners()
  updateHeaderGate()
  remeasureHeaderGate()
  window.addEventListener('resize', onWinResize)
  // TV 金刚区直播卡: 只看有无源(用户原话"不受开关控制"), 复用在顶部 PublicHeader 同一缓存 key
  // 'live:sources', 此处只是让 key 在 HomeView 渲染阶段预热(若该 key 还没被其他页面触发过)。
  // hasLiveSources 是 ref, 有值即可; 不 await, 加载态下卡片默认隐藏(0 源时)更合理。
  void getLiveSources().then((resp) => {
    hasLiveSources.value = (resp?.sources?.length ?? 0) > 0
  }).catch(() => { hasLiveSources.value = false })
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onWinResize)
  uiStore.headerSolidAt = 0
})
</script>

<template>
  <div class="jc-home flex flex-col">
    <!-- 加载骨架 -->
    <template v-if="state.loading">
      <div class="jc-home__hero-skeleton container-page pt-[var(--jc-space-6)]">
        <BaseSkeleton shape="rect" width="100%" height="45vh" />
      </div>
      <div class="container-page py-[var(--jc-space-8)] flex flex-col gap-[var(--jc-space-6)]">
        <div v-for="i in 3" :key="i" class="flex flex-col gap-[var(--jc-space-3)]">
          <BaseSkeleton shape="text" width="160px" height="24px" />
          <div class="jc-home__row-skeleton">
            <SkeletonGrid :count="7" layout="items" shape="rect" ratio="3/4" width="100%" />
          </div>
        </div>
      </div>
    </template>

    <!-- 错误态 -->
    <template v-else-if="state.errored">
      <div class="container-page py-[var(--jc-space-12)]">
        <BaseEmpty
          title="加载失败"
          description="无法获取首页数据，请稍后重试或检查网络。"
        >
          <template #action>
            <BaseButton variant="primary" size="md" @click="loadIndex">
              重新加载
            </BaseButton>
          </template>
        </BaseEmpty>
      </div>
    </template>

    <!-- 正常 -->
    <template v-else-if="state.data">
      <!-- ============================== TV: 雷鸟卡片式仪表盘 ============================== -->
      <!-- 渲染组件(方案 B): TV 模板/专属派生(tvHero 轮播、tvRecent/tvHotPanel/tvLatestPanel/
           tvCatPanels、设置卡守卫)/页级 TV CSS 全部在 HomeTv.vue; 此处只做数据供给。 -->
      <HomeTv
        v-if="isTV"
        :hero-slides="heroSlides"
        :top-ranking="topRanking"
        :rows="rows"
        :home-rows="homeRows"
        :history-list="tvHistoryList"
        :is-logged-in="isLoggedIn"
        :has-live-sources="hasLiveSources"
      />

      <!-- ============================== 桌面 / 移动: 原样 ============================== -->
      <template v-else>
        <!-- 轮播 Banner (后台配置优先, 无配置回退影片派生)。
             沉浸式(2026-10-10 评审定稿): 盒子几何/盒内文字/按钮/右侧海报/
             继续观看区位置全部维持原版; 只把图从"圆角框内显示"改为铺满页面
             顶部全宽背景(限高+底部渐隐, 下探尾压在继续观看区背后)。
             图片 breakout 实现见 HeroCarousel 的 jc-hero--immersive 块。 -->
        <div
          v-if="heroSlides.length"
          ref="heroWrapEl"
          class="container-page pt-[var(--jc-space-6)]"
        >
          <HeroCarousel
            :items="heroSlides"
            immersive
            class="rounded-[var(--jc-radius-lg)] overflow-hidden"
          />
        </div>

        <!-- 热门榜单 + 主推荐 rows 同处 container-page, 宽度/间距与分类完全一致 -->
        <div class="jc-home__rows container-page">
          <!-- 继续观看 (置顶: 有观看历史时显示) -->
          <ContinueWatchingRow />
          <FilmRow
            v-if="topRanking.length"
            title="🔥 热门榜单"
            more-link="/hotBoard"
            :items="topRanking"
          />
          <FilmRow
            v-for="row in rows"
            :key="row.pid + '-' + row.title"
            :title="row.title"
            :more-link="{ path: '/filmClassify', query: { Pid: row.pid } }"
            :items="row.items"
          />
          <BaseEmpty
            v-if="!rows.length"
            title="暂无内容"
            description="后端尚未返回分类影片列表。"
          />
        </div>

        <!-- 猜你喜欢瀑布流 (bilibili 风格底部推荐) -->
        <section
          v-if="recommendGrid.length"
          class="jc-home__recommend container-page"
          aria-label="猜你喜欢"
        >
          <!-- 区块标题(W1e): header/h2 类经 props 整组传入(本页为 items-baseline/fs-xl 变体,
               与默认 items-end/fs-lg 不同, 故整组显式给出), 右侧 tip 走插槽(父作用域 scoped 生效) -->
          <SectionHeader
            title="猜你喜欢"
            header-class="jc-home__recommend-header flex items-baseline justify-between gap-[var(--jc-space-3)] mb-[var(--jc-space-5)]"
            title-class="jc-home__recommend-title text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-bold)] text-primary"
          >
            <span class="jc-home__recommend-tip">基于浏览数据混合推荐</span>
          </SectionHeader>
          <div class="jc-home__recommend-grid">
            <FilmCard
              v-for="(item, idx) in limitToRows(recommendGrid)"
              :key="String(item.mid ?? idx) + '-' + idx"
              :item="item"
              :show-title-below="true"
            />
          </div>
        </section>
      </template>
    </template>
  </div>
</template>

<style scoped>
.jc-home {
  width: 100%;
}

.jc-home__hero-skeleton {
  width: 100%;
}

.jc-home__row-skeleton {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--jc-space-3);
}

/* 猜你喜欢瀑布流 */
.jc-home__recommend {
  padding-block: var(--jc-space-8) var(--jc-space-16);
}

/* 区块标题的 __recommend-header/__recommend-title 样式已随 W1e 收敛进 SectionHeader
   (值以 uno 工具类等价承载; 钩子类保留供全局选择器命中) */

.jc-home__recommend-tip {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
}

.jc-home__recommend-grid {
  display: grid;
  /* 列数/间距取自 theme.css 的全站统一阶梯（与上方横滚行同列数: 3/4/5/6） */
  grid-template-columns: repeat(var(--jc-list-cols), minmax(0, 1fr));
  gap: var(--jc-list-gap);
}

@media (min-width: 768px) {
  .jc-home__row-skeleton {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .jc-home__row-skeleton {
    grid-template-columns: repeat(7, minmax(0, 1fr));
  }
}

/* 主内容 rows 容器 (单列流式, 不再有 aside) */
.jc-home__rows {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-6);
  padding-block: var(--jc-space-6) var(--jc-space-8);
  /* 沉浸式(2026-10-10): 轮播背景图下探尾压在本区背后, hero 是定位元素会盖住
   * 本区的静态内容, 抬 z 让继续观看/榜单卡片压回背景之上(位置不变) */
  position: relative;
  z-index: 1;
}

@media (min-width: 768px) {
  .jc-home__rows {
    gap: var(--jc-space-10);
  }
}
</style>
