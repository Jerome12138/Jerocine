<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { filmApi } from '@/api'
import { useViewMode } from '@/composables/useViewMode'
import { useGridRowsLimit } from '@/composables/useGridRowsLimit'
import { useHistoryStore, useUserStore } from '@/stores'
import { buildPlayLink } from '@/stores/history'
import { episodeLabel, progressPercent } from '@/composables/useTimeBucket'
import { isExternalLink } from '@/utils/url'
import { formatHotBadge } from '@/utils/format'
import HeroCarousel from '@/components/film/HeroCarousel.vue'
import FilmRow from '@/components/film/FilmRow.vue'
import ContinueWatchingRow from '@/components/film/ContinueWatchingRow.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import type { Card, HeroItem, HomeBanner, HomeData } from '@/types/film'

/**
 * 首页 — STORY-008
 * - 调 GET /api/index 拿 { banner, content[] }
 * - HeroCarousel：后台配置的轮播优先，无配置时回退第一行 hot/latest 前 5
 * - 按 content[i] 渲染若干 FilmRow（title=nav.name，items=movies）
 * - PC 大屏右侧栏显示 hot 前 12 条（取 content[0].hot，兜底合并）
 * - 加载中：HeroCarousel 区骨架 + 多行骨架
 * - 失败：整页 BaseEmpty
 * - TV(雷鸟卡片式): isTV 分支独立布局, 复用同一份 state 派生数据, 不另起接口。
 */

interface IndexState {
  loading: boolean
  errored: boolean
  data: HomeData | null
}

const { isTV } = useViewMode()
const { limitToRows } = useGridRowsLimit()

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

/* ============ 以下仅 TV 雷鸟分支复用的派生数据 (均基于已拉取的 state.data, 无新接口) ============ */

/** TV 推荐轮播: 自动轮播 heroSlides(每 6s 切一张), 点击进详情 */
const tvHeroIdx = ref(0)
const tvHero = computed<HeroItem | null>(() => {
  const items = heroSlides.value
  if (!items.length) return null
  return items[tvHeroIdx.value % items.length] ?? items[0] ?? null
})
const tvHeroDots = computed<number>(() => Math.min(heroSlides.value.length, 5))
const tvHeroActive = computed<number>(() =>
  heroSlides.value.length ? tvHeroIdx.value % heroSlides.value.length : 0
)
/** TV 推荐卡跳转: 自定义链接优先, 否则按关联影片进详情 */
const tvHeroTo = computed<string | Record<string, unknown>>(() => {
  const h = tvHero.value
  if (!h) return '/'
  if (h.link) return h.link
  if (h.mid) return { path: '/filmDetail', query: { link: String(h.mid) } }
  return '/'
})

/**
 * TV 推荐卡的站外链接拦截。
 *
 * RouterLink 只认站内路由 —— 把 https://… 交给 :to 会被当成应用内路径推入路由,
 * TV 端点外链轮播会"跳到一个 404 路由"。这里与 HeroCarousel.gotoDetail 共用
 * isExternalLink 口径: 站外链接 preventDefault + 新开窗口, 站内链接放行给 RouterLink。
 * (href 本身仍是真实外链 URL, 对遥控器 focus/无障碍无影响)
 */
function onTvHeroClick(e: MouseEvent): void {
  const link = tvHero.value?.link?.trim()
  if (link && isExternalLink(link)) {
    e.preventDefault()
    window.open(link, '_blank', 'noopener,noreferrer')
  }
}
let tvHeroTimer: number | null = null

/**
 * TV 推荐卡的标签行 / 描述行 —— 口径**完全对齐非 TV 的 HeroCarousel**(用户要求"像非 TV 版一样"):
 *   · 标签行 = classTag 拆分(最多 4 个), 不回退到 年份/分类/地区(那三项占位多、信息量低);
 *   · 描述行 = 评分 · 豆瓣榜位 · remarks(片源状态), 缺哪项少哪项, 全空则不渲染;
 * 顺带把"年份 · 地区 · 分类 · 备注"那版副标题去掉 —— 非 TV 版早就不这么显示了。
 * 注: 这几段是照 HeroCarousel 私有口径复制的, 改那边记得同步这里。
 */
const tvHeroTags = computed<string[]>(() => {
  const raw = tvHero.value?.classTag?.trim()
  if (!raw) return []
  return raw
    .split(/[,，、/|]+/)
    .map((t) => t.trim())
    .filter(Boolean)
    .slice(0, 4)
})
const tvHeroScore = computed<string>(() => {
  const n = tvHero.value?.dbScore
  if (n === undefined || n === null || !Number.isFinite(n) || n < 1) return ''
  return n.toFixed(1)
})
/** 豆瓣榜位(「豆瓣·热门电影 No.1」) —— 复用 formatHotBadge, 与非 TV 版/详情页必然一致 */
const tvHeroHot = computed<string>(() =>
  formatHotBadge(tvHero.value?.hotRank, tvHero.value?.hotBoard)
)
const tvHeroHasMeta = computed<boolean>(
  () => !!(tvHeroScore.value || tvHeroHot.value || tvHero.value?.remarks)
)

/** TV 近期历史: 顶部"近期历史"影片大卡行(前 3 条, 续播入口) */
const historyStore = useHistoryStore()
const { list: tvHistoryList } = storeToRefs(historyStore)
const tvRecent = computed(() => tvHistoryList.value.slice(0, 3))

/** 登录态: 给"我的"功能卡决定去登录页还是账号设置 */
const userStore = useUserStore()
const { isLoggedIn } = storeToRefs(userStore)

/** TV 「热门榜单」面板: 取 topRanking 前 3 (每块≤3) */
const tvHotPanel = computed<Card[]>(() => topRanking.value.slice(0, 3))

/** TV 「最新上架」面板: 跨区块 latest 去重, 且排除已在「热门榜单」的影片(否则后端 hot/latest 高度重叠时两块显示一样) */
const tvLatestPanel = computed<Card[]>(() => {
  const hotIds = new Set(tvHotPanel.value.map((c) => c.mid))
  const out: Card[] = []
  for (const row of state.value.data?.rows ?? []) {
    for (const it of row.latest ?? []) {
      if (!hotIds.has(it.mid) && !out.some((o) => o.mid === it.mid)) {
        out.push(it)
        if (out.length >= 3) return out
      }
    }
  }
  return out
})
const tvLatestPid = computed<number | undefined>(() => rows.value[0]?.pid)
/** 首个一级分类 Pid: 给"分类"功能卡 / 热门榜单·热播排行"更多"做落地(否则无 Pid 进分类页报"缺少分类参数 Pid") */
const tvFirstPid = computed<number | undefined>(() => rows.value[0]?.pid)

/** 分类卡副标题文案 (按常见名称给一句营销语, 命不中给通用语) */
const CAT_SUBTITLES: Record<string, string> = {
  电视剧: '热播好剧抢先看',
  电影: '院线大片合集',
  综艺: '爆笑解压',
  动漫: '国漫日番',
  动画: '国漫日番',
  纪录片: '真实之美',
  少儿: '放心看',
  短剧: '高能反转'
}
function catSubtitle(name: string): string {
  return CAT_SUBTITLES[name] ?? '精彩内容随心看'
}

/** TV 各分类专区: 名称 + 该分类 top 3 影片(替代纯文字分类卡, 用 /home 的 rows 派生) */
interface TvCatPanel {
  id: number
  name: string
  items: Card[]
}
const tvCatPanels = computed<TvCatPanel[]>(() => {
  const data = state.value.data
  if (!data) return []
  return (data.rows ?? [])
    .map((row) => {
      const list = row.hot?.length ? row.hot : (row.latest ?? [])
      return { id: row.nav.id, name: row.nav.name, items: list.slice(0, 3) }
    })
    .filter((p) => p.items.length > 0)
})

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

onMounted(() => {
  loadIndex()
  loadBanners()
  if (isTV.value) {
    tvHeroTimer = window.setInterval(() => {
      const n = heroSlides.value.length
      if (n > 1) tvHeroIdx.value = (tvHeroIdx.value + 1) % n
    }, 6000)
  }
})
onBeforeUnmount(() => {
  if (tvHeroTimer !== null) {
    window.clearInterval(tvHeroTimer)
    tvHeroTimer = null
  }
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
            <BaseSkeleton
              v-for="j in 7"
              :key="j"
              shape="rect"
              ratio="3/4"
              width="100%"
            />
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
      <div v-if="isTV" class="jc-home-tv container-page">
        <!-- ① 顶部: 近期历史(3 片同款影片卡显当前集数) + 推荐轮播, 各占一半 -->
        <div class="jc-home-tv__top" :class="{ 'no-recent': !tvRecent.length }">
          <section v-if="tvRecent.length" class="jc-tv-panel z1">
            <div class="jc-tv-sec">
              <span class="t">⏱ 近期历史</span>
              <RouterLink class="jc-tv-more" to="/history" data-focusable="true" tabindex="0">全部</RouterLink>
            </div>
            <div class="jc-tv-p3">
              <RouterLink
                v-for="rec in tvRecent"
                :key="'rec-' + rec.id"
                :to="buildPlayLink(rec)"
                class="jc-tv-card"
                data-focusable="true"
                tabindex="0"
                :aria-label="`继续观看 ${rec.name}`"
              >
                <div class="poster">
                  <BaseImage :src="rec.picture || ''" :alt="rec.name" ratio="3/4" fit="cover" />
                  <!-- 集数放进图内底部信息条(与 FilmCard 的 __epinfo 同款): 不再单独占图下一行 -->
                  <span class="epinfo">看到 {{ episodeLabel(rec.episode, rec.episodeIndex) || '第 1 集' }}</span>
                  <span v-if="progressPercent(rec.currentTime, rec.duration) > 0" class="pbar"><i :style="{ width: progressPercent(rec.currentTime, rec.duration) + '%' }" /></span>
                </div>
                <div class="name">{{ rec.name }}</div>
              </RouterLink>
            </div>
          </section>

          <!-- 推荐轮播 (自动轮播 heroSlides; 海报作背景, 点击进详情) -->
          <RouterLink
            v-if="tvHero"
            :to="tvHeroTo"
            class="jc-tv-carousel jc-home-tv__hero"
            data-focusable="true"
            tabindex="0"
            :aria-label="`为你推荐 ${tvHero.name}`"
            @click="onTvHeroClick"
          >
            <BaseImage
              v-if="tvHero.cover || tvHero.poster"
              :key="tvHero.mid ?? tvHero.name"
              class="jc-home-tv__hero-bg"
              :src="tvHero.cover || tvHero.poster || ''"
              :alt="tvHero.name"
              ratio=""
              fit="cover"
              :eager="true"
            />
            <span class="jc-home-tv__hero-shade" aria-hidden="true" />
            <span class="tag">为你推荐</span>
            <div class="jc-home-tv__hero-text">
              <h3>{{ tvHero.name }}</h3>
              <!-- 标签行: 与非 TV 版 HeroCarousel 同口径(classTag 拆最多 4 个, purple 胶囊) -->
              <div v-if="tvHeroTags.length" class="jc-home-tv__hero-tags">
                <BaseTag v-for="(t, i) in tvHeroTags" :key="i" variant="purple" size="sm">
                  {{ t }}
                </BaseTag>
              </div>
              <!-- 描述行: 评分 · 豆瓣榜位 · 片源状态 —— 与非 TV 版逐项一致 -->
              <p v-if="tvHeroHasMeta" class="jc-home-tv__hero-meta">
                <span v-if="tvHeroScore" class="score">
                  <BaseIcon name="star" size="0.85em" />{{ tvHeroScore }}
                </span>
                <span v-if="tvHeroHot" class="hot">{{ tvHeroHot }}</span>
                <span v-if="tvHero.remarks" class="remarks">{{ tvHero.remarks }}</span>
              </p>
            </div>
            <div v-if="tvHeroDots > 1" class="jc-tv-dots" aria-hidden="true">
              <i v-for="d in tvHeroDots" :key="d" :class="{ on: d - 1 === tvHeroActive }" />
            </div>
          </RouterLink>
        </div>

        <!-- ② 功能卡(大彩色卡, 左文字右图标 — 对齐设计稿; 继续观看入口已并入顶部"近期历史")
             注: 卡片本就指向 /favorites, 故主文案用"收藏"(用户拍板), 不再写"历史·收藏"占宽. -->
        <div class="jc-tv-funcs">
          <RouterLink class="jc-tv-fc fc-2" to="/favorites" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="heart" size="32px" /></span><span class="ti">收藏</span><span class="su">记录您的热爱</span>
          </RouterLink>
          <RouterLink class="jc-tv-fc fc-3" :to="tvFirstPid ? { path: '/filmClassify', query: { Pid: tvFirstPid } } : '/filmClassify'" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="film" size="32px" /></span><span class="ti">分类</span><span class="su">剧/影/综/漫</span>
          </RouterLink>
          <RouterLink class="jc-tv-fc fc-4" to="/search" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="search" size="32px" /></span><span class="ti">搜索</span><span class="su">找片更快</span>
          </RouterLink>
          <RouterLink class="jc-tv-fc fc-1" :to="isLoggedIn ? { path: '/settings', query: { group: 'account' } } : { path: '/login' }" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="user" size="32px" /></span><span class="ti">我的</span><span class="su">{{ isLoggedIn ? '账号 · 退出' : '点击登录' }}</span>
          </RouterLink>
          <RouterLink class="jc-tv-fc fc-5" to="/settings" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="settings" size="32px" /></span><span class="ti">设置</span><span class="su">画质/过滤</span>
          </RouterLink>
        </div>

        <!-- ④ 专区面板: 热门榜单 + 最新上架 (每块≤3 张 FilmCard) -->
        <div class="jc-tv-duo">
          <section v-if="tvHotPanel.length" class="jc-tv-panel z1">
            <div class="jc-tv-sec">
              <span class="t">🔥 热门榜单</span>
              <span class="s">最热抢先看</span>
              <RouterLink
                v-if="tvFirstPid"
                class="jc-tv-more"
                :to="{ path: '/filmClassify', query: { Pid: tvFirstPid } }"
                data-focusable="true"
                tabindex="0"
              >更多内容</RouterLink>
            </div>
            <div class="jc-tv-p3">
              <FilmCard
                v-for="item in tvHotPanel"
                :key="'hot-' + item.mid"
                :item="item"
                :show-title-below="true"
              />
            </div>
          </section>
          <section v-if="tvLatestPanel.length" class="jc-tv-panel z2">
            <div class="jc-tv-sec">
              <span class="t">🆕 最新上架</span>
              <span class="s">每日更新</span>
              <RouterLink
                class="jc-tv-more"
                :to="{ path: '/filmClassify', query: { Pid: tvLatestPid } }"
                data-focusable="true"
                tabindex="0"
              >查看全部</RouterLink>
            </div>
            <div class="jc-tv-p3">
              <FilmCard
                v-for="item in tvLatestPanel"
                :key="'new-' + item.mid"
                :item="item"
                :show-title-below="true"
              />
            </div>
          </section>
        </div>

        <!-- ⑥ 各分类专区: 名称 + 该分类 top3 影片 (替代纯文字分类卡) -->
        <div class="jc-tv-duo jc-home-tv__cat-panels">
          <section
            v-for="(p, idx) in tvCatPanels"
            :key="'catp-' + p.id"
            class="jc-tv-panel"
            :class="idx % 2 === 0 ? 'z1' : 'z3'"
          >
            <div class="jc-tv-sec">
              <span class="t">{{ p.name }}</span>
              <span class="s">{{ catSubtitle(p.name) }}</span>
              <RouterLink
                class="jc-tv-more"
                :to="{ path: '/filmClassify', query: { Pid: p.id } }"
                data-focusable="true"
                tabindex="0"
              >更多</RouterLink>
            </div>
            <div class="jc-tv-p3">
              <FilmCard
                v-for="item in p.items"
                :key="'catp-' + p.id + '-' + item.mid"
                :item="item"
                :show-title-below="true"
              />
            </div>
          </section>
        </div>

        <BaseEmpty
          v-if="!rows.length && !tvHotPanel.length"
          title="暂无内容"
          description="后端尚未返回分类影片列表。"
        />
      </div>

      <!-- ============================== 桌面 / 移动: 原样 ============================== -->
      <template v-else>
        <!-- 轮播 Banner (后台配置优先, 无配置回退影片派生) -->
        <div
          v-if="heroSlides.length"
          class="container-page pt-[var(--jc-space-6)]"
        >
          <HeroCarousel
            :items="heroSlides"
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
          <header class="jc-home__recommend-header">
            <h2 class="jc-home__recommend-title">猜你喜欢</h2>
            <span class="jc-home__recommend-tip">基于浏览数据混合推荐</span>
          </header>
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

.jc-home__recommend-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--jc-space-5);
  gap: var(--jc-space-3);
}

.jc-home__recommend-title {
  font-size: var(--jc-fs-xl);
  font-weight: var(--jc-fw-bold);
  color: var(--jc-text-primary);
}

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
}

@media (min-width: 768px) {
  .jc-home__rows {
    gap: var(--jc-space-10);
  }
}

</style>

<style>
/* ============================================================
 * TV 雷鸟仪表盘布局胶水 (chrome 卡片样式来自全局 tv-cards.css, 此处只补容器/栅格)
 * 全部 [data-mode='tv'] 作用域, 不影响桌面/移动。
 * ============================================================ */
[data-mode='tv'] .jc-home-tv.container-page {
  padding-inline: var(--jc-tv-safe);
}
[data-mode='tv'] .jc-home-tv {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-6);
  padding-block: var(--jc-space-4) var(--jc-space-12);
}

/* ContinueWatchingRow 自带 container-page 内缩, 在 TV 仪表盘里抵消其与本容器的双重内缩,
 * 让横滚区与下方卡片左右对齐 (其内部 edge 已用 var(--jc-tv-safe) 留白) */
[data-mode='tv'] .jc-home-tv > .jc-continue {
  margin-inline: calc(-1 * var(--jc-tv-safe));
}

/* ① 顶部: 近期历史 + 推荐轮播 并排(对齐设计稿); 无历史时轮播占满 */
[data-mode='tv'] .jc-home-tv__top {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--jc-space-4);
  align-items: stretch;
}
[data-mode='tv'] .jc-home-tv__top.no-recent {
  grid-template-columns: 1fr;
}

/* 推荐轮播: 海报作背景铺满, 文字浮在上层 */
[data-mode='tv'] .jc-home-tv__hero {
  min-height: clamp(180px, 22vw, 280px);
}
[data-mode='tv'] .jc-home-tv__hero-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  z-index: 0;
  pointer-events: none;
}
[data-mode='tv'] .jc-home-tv__hero-shade {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background: linear-gradient(
    90deg,
    rgba(0, 0, 0, 0.78) 0%,
    rgba(0, 0, 0, 0.45) 45%,
    rgba(0, 0, 0, 0.08) 100%
  );
}
/* 注意: 这里**不包含** .tag, 也**不包含** .jc-tv-dots —— 这两者都必须保持 tv-cards.css 里的
 * position:absolute, 才能各自钉在左上角 / 右下角. 曾把它们和文字一起设成 position:relative:
 * 标签掉进 flex 行变成左下角的内联小胶囊、指示点也从右下角回到内容流末尾(用户两次分别反馈过).
 * 只让标题文字用 relative+z-index 压过遮罩层. */
[data-mode='tv'] .jc-home-tv__hero-text {
  position: relative;
  z-index: 1;
}
[data-mode='tv'] .jc-home-tv__hero-text {
  max-width: 70%;
}

/* 推荐轮播的标签行 / 描述行 —— 配色口径照非 TV 的 HeroCarousel(.jc-hero__meta 系列):
   评分暖色粗体、榜位品牌青、片源状态弱化白; 深色渐变底上这组颜色对比足够. */
[data-mode='tv'] .jc-home-tv__hero-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}
[data-mode='tv'] .jc-home-tv__hero-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 14px;
  margin-top: 8px;
  font-size: var(--jc-fs-sm);
  color: rgba(255, 255, 255, 0.8);
}
[data-mode='tv'] .jc-home-tv__hero-meta .score {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  color: var(--jc-warning);
  font-weight: var(--jc-fw-bold);
}
[data-mode='tv'] .jc-home-tv__hero-meta .hot {
  color: var(--jc-brand-cyan);
  font-weight: var(--jc-fw-semibold);
}
[data-mode='tv'] .jc-home-tv__hero-meta .remarks {
  color: rgba(255, 255, 255, 0.6);
}
</style>
