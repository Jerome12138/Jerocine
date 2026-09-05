<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { filmApi } from '@/api'
import { useViewMode } from '@/composables/useViewMode'
import { useHistoryStore, useUserStore } from '@/stores'
import { buildPlayLink } from '@/stores/history'
import { episodeLabel, progressPercent } from '@/composables/useTimeBucket'
import HeroCarousel from '@/components/film/HeroCarousel.vue'
import FilmRow from '@/components/film/FilmRow.vue'
import ContinueWatchingRow from '@/components/film/ContinueWatchingRow.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import type { Card, HomeData } from '@/types/film'

/**
 * 首页 — STORY-008
 * - 调 GET /api/index 拿 { banner, content[] }
 * - HeroCarousel：banner 优先，回退 content[0].movies.slice(0, 5)
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

const state = ref<IndexState>({
  loading: true,
  errored: false,
  data: null
})

// 后端 /home 已做区块化聚合(无独立 banner): 首屏取第一行的 hot/latest 前 5。
const heroItems = computed<Card[]>(() => {
  const data = state.value.data
  if (!data) return []
  for (const row of data.rows ?? []) {
    if (row.hot?.length) return row.hot.slice(0, 5)
    if (row.latest?.length) return row.latest.slice(0, 5)
  }
  return []
})

/** 热门榜单 — 合并各区块 hot(后端真实热门, cover 已进表, 无需回填) */
const topRanking = computed<Card[]>(() => {
  const data = state.value.data
  if (!data) return []
  const merged: Card[] = []
  const seen = new Set<number>()
  for (const row of data.rows ?? []) {
    for (const it of row.hot ?? []) {
      if (!seen.has(it.mid)) {
        seen.add(it.mid)
        merged.push(it)
        if (merged.length >= 10) break
      }
    }
    if (merged.length >= 10) break
  }
  return merged
})

const rows = computed(() => {
  const data = state.value.data
  if (!data) return []
  return (data.rows ?? [])
    .filter((r) => r.latest?.length)
    .map((r) => ({ pid: r.nav.id, title: r.nav.name, items: r.latest }))
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

/** TV 推荐轮播: 自动轮播 heroItems(每 6s 切一张), 点击进详情 */
const tvHeroIdx = ref(0)
const tvHero = computed<Card | null>(() => {
  const items = heroItems.value
  if (!items.length) return null
  return items[tvHeroIdx.value % items.length] ?? items[0] ?? null
})
const tvHeroDots = computed<number>(() => Math.min(heroItems.value.length, 5))
const tvHeroActive = computed<number>(() =>
  heroItems.value.length ? tvHeroIdx.value % heroItems.value.length : 0
)
let tvHeroTimer: number | null = null

/** TV 推荐卡副标题: 年份·地区·分类·更新备注 */
const tvHeroSub = computed<string>(() => {
  const h = tvHero.value
  if (!h) return ''
  return [h.year, h.area, h.cName, h.remarks]
    .filter((v) => v !== undefined && v !== null && v !== '' && v !== 0)
    .join(' · ')
})

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

onMounted(() => {
  loadIndex()
  if (isTV.value) {
    tvHeroTimer = window.setInterval(() => {
      const n = heroItems.value.length
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
  <div class="gf-home flex flex-col">
    <!-- 加载骨架 -->
    <template v-if="state.loading">
      <div class="gf-home__hero-skeleton container-page pt-[var(--gf-space-6)]">
        <BaseSkeleton shape="rect" width="100%" height="45vh" />
      </div>
      <div class="container-page py-[var(--gf-space-8)] flex flex-col gap-[var(--gf-space-8)]">
        <div v-for="i in 3" :key="i" class="flex flex-col gap-[var(--gf-space-3)]">
          <BaseSkeleton shape="text" width="160px" height="24px" />
          <div class="gf-home__row-skeleton">
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
      <div class="container-page py-[var(--gf-space-12)]">
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
      <div v-if="isTV" class="gf-home-tv container-page">
        <!-- ① 顶部: 近期历史(3 片同款影片卡显当前集数) + 推荐轮播, 各占一半 -->
        <div class="gf-home-tv__top" :class="{ 'no-recent': !tvRecent.length }">
          <section v-if="tvRecent.length" class="gf-tv-panel z1">
            <div class="gf-tv-sec">
              <span class="t">⏱ 近期历史</span>
              <RouterLink class="gf-tv-more" to="/history" data-focusable="true" tabindex="0">全部</RouterLink>
            </div>
            <div class="gf-tv-p3">
              <RouterLink
                v-for="rec in tvRecent"
                :key="'rec-' + rec.id"
                :to="buildPlayLink(rec)"
                class="gf-tv-card"
                data-focusable="true"
                tabindex="0"
                :aria-label="`继续观看 ${rec.name}`"
              >
                <div class="poster">
                  <BaseImage :src="rec.picture || ''" :alt="rec.name" ratio="3/4" fit="cover" />
                  <span v-if="progressPercent(rec.currentTime, rec.duration) > 0" class="pbar"><i :style="{ width: progressPercent(rec.currentTime, rec.duration) + '%' }" /></span>
                </div>
                <div class="name">{{ rec.name }}</div>
                <div class="sub">看到 {{ episodeLabel(rec.episode, rec.episodeIndex) || '第 1 集' }}</div>
              </RouterLink>
            </div>
          </section>

          <!-- 推荐轮播 (自动轮播 heroItems; 海报作背景, 点击进详情) -->
          <RouterLink
            v-if="tvHero"
            :to="{ path: '/filmDetail', query: { link: String(tvHero.mid) } }"
            class="gf-tv-carousel gf-home-tv__hero"
            data-focusable="true"
            tabindex="0"
            :aria-label="`为你推荐 ${tvHero.name}`"
          >
            <BaseImage
              v-if="tvHero.cover"
              :key="tvHero.mid"
              class="gf-home-tv__hero-bg"
              :src="tvHero.cover"
              :alt="tvHero.name"
              ratio=""
              fit="cover"
              :eager="true"
            />
            <span class="gf-home-tv__hero-shade" aria-hidden="true" />
            <span class="tag">为你推荐</span>
            <div class="gf-home-tv__hero-text">
              <h3>{{ tvHero.name }}</h3>
              <p v-if="tvHeroSub">{{ tvHeroSub }}</p>
            </div>
            <div v-if="tvHeroDots > 1" class="gf-tv-dots" aria-hidden="true">
              <i v-for="d in tvHeroDots" :key="d" :class="{ on: d - 1 === tvHeroActive }" />
            </div>
          </RouterLink>
        </div>

        <!-- ② 功能卡(大彩色卡, 左文字右图标 — 对齐设计稿; 继续观看入口已并入顶部"近期历史") -->
        <div class="gf-tv-funcs">
          <RouterLink class="gf-tv-fc fc-2" to="/favorites" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="heart" size="42px" /></span><span class="ti">历史 · 收藏</span><span class="su">记录您的热爱</span>
          </RouterLink>
          <RouterLink class="gf-tv-fc fc-3" :to="tvFirstPid ? { path: '/filmClassify', query: { Pid: tvFirstPid } } : '/filmClassify'" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="film" size="42px" /></span><span class="ti">分类</span><span class="su">剧/影/综/漫</span>
          </RouterLink>
          <RouterLink class="gf-tv-fc fc-4" to="/search" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="search" size="42px" /></span><span class="ti">搜索</span><span class="su">找片更快</span>
          </RouterLink>
          <RouterLink class="gf-tv-fc fc-1" :to="isLoggedIn ? { path: '/settings', query: { group: 'account' } } : { path: '/login' }" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="user" size="42px" /></span><span class="ti">我的</span><span class="su">{{ isLoggedIn ? '账号 · 退出' : '点击登录' }}</span>
          </RouterLink>
          <RouterLink class="gf-tv-fc fc-5" to="/settings" data-focusable="true" tabindex="0">
            <span class="ic"><BaseIcon name="settings" size="42px" /></span><span class="ti">设置</span><span class="su">画质/过滤</span>
          </RouterLink>
        </div>

        <!-- ④ 专区面板: 热门榜单 + 最新上架 (每块≤3 张 FilmCard) -->
        <div class="gf-tv-duo">
          <section v-if="tvHotPanel.length" class="gf-tv-panel z1">
            <div class="gf-tv-sec">
              <span class="t">🔥 热门榜单</span>
              <span class="s">最热抢先看</span>
              <RouterLink
                v-if="tvFirstPid"
                class="gf-tv-more"
                :to="{ path: '/filmClassify', query: { Pid: tvFirstPid } }"
                data-focusable="true"
                tabindex="0"
              >更多内容</RouterLink>
            </div>
            <div class="gf-tv-p3">
              <FilmCard
                v-for="item in tvHotPanel"
                :key="'hot-' + item.mid"
                :item="item"
                :show-title-below="true"
              />
            </div>
          </section>
          <section v-if="tvLatestPanel.length" class="gf-tv-panel z2">
            <div class="gf-tv-sec">
              <span class="t">🆕 最新上架</span>
              <span class="s">每日更新</span>
              <RouterLink
                class="gf-tv-more"
                :to="{ path: '/filmClassify', query: { Pid: tvLatestPid } }"
                data-focusable="true"
                tabindex="0"
              >查看全部</RouterLink>
            </div>
            <div class="gf-tv-p3">
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
        <div class="gf-tv-duo gf-home-tv__cat-panels">
          <section
            v-for="(p, idx) in tvCatPanels"
            :key="'catp-' + p.id"
            class="gf-tv-panel"
            :class="idx % 2 === 0 ? 'z1' : 'z3'"
          >
            <div class="gf-tv-sec">
              <span class="t">{{ p.name }}</span>
              <span class="s">{{ catSubtitle(p.name) }}</span>
              <RouterLink
                class="gf-tv-more"
                :to="{ path: '/filmClassify', query: { Pid: p.id } }"
                data-focusable="true"
                tabindex="0"
              >更多</RouterLink>
            </div>
            <div class="gf-tv-p3">
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
        <!-- 轮播 Banner -->
        <div
          v-if="heroItems.length"
          class="container-page pt-[var(--gf-space-6)]"
        >
          <HeroCarousel
            :items="heroItems"
            class="rounded-[var(--gf-radius-lg)] overflow-hidden"
          />
        </div>

        <!-- 热门榜单 + 主推荐 rows 同处 container-page, 宽度/间距与分类完全一致 -->
        <div class="gf-home__rows container-page">
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
          class="gf-home__recommend container-page"
          aria-label="猜你喜欢"
        >
          <header class="gf-home__recommend-header">
            <h2 class="gf-home__recommend-title">猜你喜欢</h2>
            <span class="gf-home__recommend-tip">基于浏览数据混合推荐</span>
          </header>
          <div class="gf-home__recommend-grid">
            <FilmCard
              v-for="(item, idx) in recommendGrid"
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
.gf-home {
  width: 100%;
}

.gf-home__hero-skeleton {
  width: 100%;
}

.gf-home__row-skeleton {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--gf-space-3);
}

/* 猜你喜欢瀑布流 */
.gf-home__recommend {
  padding-block: var(--gf-space-8) var(--gf-space-16);
}

.gf-home__recommend-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--gf-space-5);
  gap: var(--gf-space-3);
}

.gf-home__recommend-title {
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
}

.gf-home__recommend-tip {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
}

.gf-home__recommend-grid {
  display: grid;
  /* 移动端默认 3 列 */
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--gf-space-2);
}

@media (min-width: 480px) {
  .gf-home__recommend-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--gf-card-gap);
  }
}

@media (min-width: 768px) {
  .gf-home__recommend-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .gf-home__recommend-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}

@media (min-width: 1440px) {
  .gf-home__recommend-grid {
    grid-template-columns: repeat(6, minmax(0, 1fr));
  }
}

@media (min-width: 768px) {
  .gf-home__row-skeleton {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .gf-home__row-skeleton {
    grid-template-columns: repeat(7, minmax(0, 1fr));
  }
}

/* 主内容 rows 容器 (单列流式, 不再有 aside) */
.gf-home__rows {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-8);
  padding-block: var(--gf-space-6) var(--gf-space-8);
}

@media (min-width: 768px) {
  .gf-home__rows {
    gap: var(--gf-space-10);
  }
}

/* ========== 热门榜单模块 (Netflix Top 10 / 腾讯视频热播榜风格) ========== */
.gf-home__ranking {
  padding-block: var(--gf-space-6) var(--gf-space-4);
}

.gf-home__section-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--gf-space-4);
  gap: var(--gf-space-3);
}

.gf-home__section-title {
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  display: inline-flex;
  align-items: center;
  gap: var(--gf-space-2);
  margin: 0;
}

.gf-home__section-flame {
  font-size: 1.1em;
}

.gf-home__section-tip {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
}

.gf-home__ranking-scroll {
  display: flex;
  gap: var(--gf-space-3);
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  scrollbar-width: thin;
  padding-block: var(--gf-space-2);
  margin-inline: calc(-1 * var(--gf-gutter-mobile));
  padding-inline: var(--gf-gutter-mobile);
}
@media (min-width: 768px) {
  .gf-home__ranking-scroll {
    gap: var(--gf-space-4);
    margin-inline: calc(-1 * var(--gf-gutter-tablet));
    padding-inline: var(--gf-gutter-tablet);
  }
}
@media (min-width: 1024px) {
  .gf-home__ranking-scroll {
    margin-inline: 0;
    padding-inline: 0;
  }
}

.gf-home__ranking-scroll::-webkit-scrollbar {
  height: 4px;
}
.gf-home__ranking-scroll::-webkit-scrollbar-thumb {
  background-color: rgba(255, 255, 255, 0.18);
  border-radius: 2px;
}

.gf-home__ranking-item {
  flex: 0 0 auto;
  display: grid;
  grid-template-columns: auto 84px 1fr;
  gap: var(--gf-space-3);
  align-items: center;
  width: 280px;
  padding: var(--gf-space-2);
  background-color: var(--gf-bg-surface);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-lg);
  text-decoration: none;
  scroll-snap-align: start;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-spring);
  outline: none;
}
.gf-home__ranking-item:hover {
  background-color: var(--gf-bg-elevated);
  transform: translateY(-2px);
}
.gf-home__ranking-item:focus-visible {
  box-shadow: var(--gf-shadow-focus-ring);
}
@media (min-width: 768px) {
  .gf-home__ranking-item {
    width: 320px;
  }
}

.gf-home__ranking-rank {
  font-family: var(--gf-font-display);
  font-size: 48px;
  font-weight: 900;
  line-height: 1;
  color: var(--gf-text-muted);
  text-align: center;
  min-width: 48px;
  font-style: italic;
  letter-spacing: -0.04em;
}
.gf-home__ranking-rank--top {
  color: transparent;
  background-image: var(--gf-brand-gradient);
  background-clip: text;
  -webkit-background-clip: text;
}

.gf-home__ranking-poster {
  width: 84px;
  border-radius: var(--gf-radius-md);
  overflow: hidden;
  flex-shrink: 0;
}

.gf-home__ranking-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 4px;
}
.gf-home__ranking-name {
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  margin: 0;
}
.gf-home__ranking-meta {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin: 0;
}

.gf-home__hot-remarks {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

<style>
/* TV 模式下不显示侧栏（屏幕宽度足够，但旁栏会破坏 10-foot UI 节奏） */
[data-mode='tv'] .gf-home__aside {
  display: none;
}
[data-mode='tv'] .gf-home__main {
  padding-inline: var(--gf-tv-safe);
}

/* ============================================================
 * TV 雷鸟仪表盘布局胶水 (chrome 卡片样式来自全局 tv-cards.css, 此处只补容器/栅格)
 * 全部 [data-mode='tv'] 作用域, 不影响桌面/移动。
 * ============================================================ */
[data-mode='tv'] .gf-home-tv.container-page {
  padding-inline: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-home-tv {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-6);
  padding-block: var(--gf-space-4) var(--gf-space-12);
}

/* ContinueWatchingRow 自带 container-page 内缩, 在 TV 仪表盘里抵消其与本容器的双重内缩,
 * 让横滚区与下方卡片左右对齐 (其内部 edge 已用 var(--gf-tv-safe) 留白) */
[data-mode='tv'] .gf-home-tv > .gf-continue {
  margin-inline: calc(-1 * var(--gf-tv-safe));
}

/* ① 顶部: 近期历史 + 推荐轮播 并排(对齐设计稿); 无历史时轮播占满 */
[data-mode='tv'] .gf-home-tv__top {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--gf-space-4);
  align-items: stretch;
}
[data-mode='tv'] .gf-home-tv__top.no-recent {
  grid-template-columns: 1fr;
}

/* 推荐轮播: 海报作背景铺满, 文字浮在上层 */
[data-mode='tv'] .gf-home-tv__hero {
  min-height: clamp(180px, 22vw, 280px);
}
[data-mode='tv'] .gf-home-tv__hero-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  z-index: 0;
  pointer-events: none;
}
[data-mode='tv'] .gf-home-tv__hero-shade {
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
[data-mode='tv'] .gf-home-tv__hero .tag,
[data-mode='tv'] .gf-home-tv__hero-text,
[data-mode='tv'] .gf-home-tv__hero .gf-tv-dots {
  position: relative;
  z-index: 1;
}
[data-mode='tv'] .gf-home-tv__hero-text {
  max-width: 70%;
}

/* ⑤ 电视剧/电影大卡 + 热播排行 */
[data-mode='tv'] .gf-home-tv__row5 {
  display: grid;
  grid-template-columns: 1fr 1fr 2fr;
  gap: var(--gf-space-4);
}

/* 排行列表项: 行内可聚焦, 焦点环靠 theme.css */
[data-mode='tv'] .gf-home-tv__rk-item {
  cursor: pointer;
  border-radius: 8px;
  outline: none;
}

/* ⑥ 底部分类卡: 4 列 (与 demo tv-funcs repeat(4) 一致) */
[data-mode='tv'] .gf-home-tv__cats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--gf-space-4);
}

/* 窄屏 TV (真机 WebView dpr 压缩) 降列, 保证可读 */
@media (max-width: 1100px) {
  [data-mode='tv'] .gf-home-tv__row5 {
    grid-template-columns: 1fr 1fr;
  }
  [data-mode='tv'] .gf-home-tv__row5 .gf-tv-rank {
    grid-column: 1 / -1;
  }
  [data-mode='tv'] .gf-home-tv__cats {
    grid-template-columns: repeat(3, 1fr);
  }
}
</style>
