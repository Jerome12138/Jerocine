<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import TvSectionHeader from '@/components/base/TvSectionHeader.vue'
import BaseImage from '@/components/base/BaseImage.vue'
// FilmCard/FilmRow 等 film/ 组件不在 unplugin-vue-components 自动注册清单, 须显式引入
import FilmCard from '@/components/film/FilmCard.vue'
import { buildPlayLink, type HistoryRecord } from '@/stores/history'
import { episodeLabel, progressPercent } from '@/composables/useTimeBucket'
import { isExternalLink } from '@/utils/url'
import { formatHotBadge } from '@/utils/format'
import { useTvSettingsEntry } from '@/composables/useTvSettingsEntry'
import type { Card, HeroItem, HomeRow } from '@/types/film'

/**
 * 首页 TV(雷鸟卡片式)渲染组件 — 方案 B 拆分(自 HomeView.vue 下沉)。
 *
 * 职责边界: 本组件只做「TV 专属派生 + 渲染」, 数据获取全在宿主 HomeView.vue
 * (filmApi.getHome/banners/historyStore/userStore/live 源), 经 props 单向传入;
 * 这里不复制任何业务逻辑, 也不另起接口。
 * 宿主以 `v-if="isTV"` 挂载本组件 ⇒ 仅 TV 模式渲染, 组件内无需再判 isTV。
 *
 * 模板/类名/data-focusable/tabindex 与原 HomeView TV 分支(438-631)逐块等价;
 * 金刚区卡片样式来自全局 tv-cards.css 词汇类(jc-tv-fc/jc-tv-panel/jc-tv-carousel
 * 等), 本组件 scoped 样式只补容器/栅格胶水(见文件底部 style 注释)。
 */
interface Props {
  /** 首屏大图数据源(宿主 heroSlides: 后台 banners 优先 + 影片派生兜底, 已合并) */
  heroSlides: HeroItem[]
  /** 全站跨类别热榜(「🔥 热门榜单」面板 = 前 3) */
  topRanking: Card[]
  /** 过滤后的分类行(空态判断 + 功能卡/「查看全部」的落地 Pid) */
  rows: Array<{ pid: number; title: string; items: Card[] }>
  /** 原始区块 rows(「🆕 最新上架」面板与各分类专区的 hot/latest 派生) */
  homeRows: HomeRow[]
  /** 观看历史(「⏱ 近期历史」大卡行 = 前 3) */
  historyList: HistoryRecord[]
  /** 登录态(「我的」卡副标题文案) */
  isLoggedIn: boolean
  /** 金刚区直播卡显隐(有源才渲染); 取数在宿主, 保持 live:sources 缓存预热时机 */
  hasLiveSources: boolean
}
const props = defineProps<Props>()

/** TV 推荐轮播: 自动轮播 heroSlides(每 6s 切一张), 点击进详情 */
const tvHeroIdx = ref(0)
const tvHero = computed<HeroItem | null>(() => {
  const items = props.heroSlides
  if (!items.length) return null
  return items[tvHeroIdx.value % items.length] ?? items[0] ?? null
})
const tvHeroDots = computed<number>(() => Math.min(props.heroSlides.length, 5))
const tvHeroActive = computed<number>(() =>
  props.heroSlides.length ? tvHeroIdx.value % props.heroSlides.length : 0
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
  // 刚完成滑动手势: 触摸合成的 click 会跟着 touchend 到来, 吞掉防误进详情
  if (tvHeroSwiped) {
    tvHeroSwiped = false
    e.preventDefault()
    return
  }
  const link = tvHero.value?.link?.trim()
  if (link && isExternalLink(link)) {
    e.preventDefault()
    window.open(link, '_blank', 'noopener,noreferrer')
  }
}
let tvHeroTimer: number | null = null

/** (重)启动 TV 推荐卡自动轮播: 手动切换后调用, 避免"刚切完 1s 内又自动跳下一张" */
function restartTvHeroTimer(): void {
  if (tvHeroTimer !== null) window.clearInterval(tvHeroTimer)
  tvHeroTimer = window.setInterval(() => {
    const n = props.heroSlides.length
    if (n > 1) tvHeroIdx.value = (tvHeroIdx.value + 1) % n
  }, 6000)
}

/** 手动步进(手势/后续遥控键共用): 环绕 + 重置自动轮播计时 */
function tvHeroStep(delta: number): void {
  const n = props.heroSlides.length
  if (n <= 1) return
  tvHeroIdx.value = (tvHeroIdx.value + delta + n) % n
  restartTvHeroTimer()
}

/* TV 推荐卡触摸横滑切换(2026-10-10 用户要求"手势滑动上/下一张"):
 * 阈值 ±60px 与 HeroCarousel 同口径; 滑动后吞掉合成 click(见 onTvHeroClick)。 */
let tvHeroTouchX = 0
let tvHeroTouchDx = 0
let tvHeroSwiped = false
function onTvHeroTouchStart(e: TouchEvent): void {
  tvHeroTouchX = e.touches[0]?.clientX ?? 0
  tvHeroTouchDx = 0
  tvHeroSwiped = false
}
function onTvHeroTouchMove(e: TouchEvent): void {
  tvHeroTouchDx = (e.touches[0]?.clientX ?? 0) - tvHeroTouchX
}
function onTvHeroTouchEnd(): void {
  if (Math.abs(tvHeroTouchDx) > 60) {
    tvHeroSwiped = true
    tvHeroStep(tvHeroTouchDx < 0 ? 1 : -1)
  }
}

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
const tvRecent = computed(() => props.historyList.slice(0, 3))

/** TV 「热门榜单」面板: 取 topRanking 前 3 (每块≤3) */
const tvHotPanel = computed<Card[]>(() => props.topRanking.slice(0, 3))

/** TV 「最新上架」面板: 跨区块 latest 去重, 且排除已在「热门榜单」的影片(否则后端 hot/latest 高度重叠时两块显示一样) */
const tvLatestPanel = computed<Card[]>(() => {
  const hotIds = new Set(tvHotPanel.value.map((c) => c.mid))
  const out: Card[] = []
  for (const row of props.homeRows) {
    for (const it of row.latest ?? []) {
      if (!hotIds.has(it.mid) && !out.some((o) => o.mid === it.mid)) {
        out.push(it)
        if (out.length >= 3) return out
      }
    }
  }
  return out
})
const tvLatestPid = computed<number | undefined>(() => props.rows[0]?.pid)
/** 首个一级分类 Pid: 给"分类"功能卡 / 热门榜单·热播排行"更多"做落地(否则无 Pid 进分类页报"缺少分类参数 Pid") */
const tvFirstPid = computed<number | undefined>(() => props.rows[0]?.pid)

/**
 * 「设置」功能卡(方案 §6 / 抽屉重设计 §6; 2026-10-10 修订): SPA `/settings` 已删,
 * 设置入口只剩原生抽屉 ⇒ 卡片**只在原生壳里渲染**(v-if), 点击开抽屉(guard 拦掉
 * RouterLink 默认跳转); 纯网页 TV 模式没有设置入口, 卡片隐藏。
 *
 * ⚠️ 必须用 `@click.capture`(不是 `@click`): RouterLink 自己也在同一个 <a> 上绑了
 * onClick, Vue 会按「组件自身 handler 在前」的顺序合并 ⇒ 冒泡阶段我们的 handler 跑在
 * RouterLink 的 navigate 之后, 那时 preventDefault 已经晚了(实测 push 仍发生)。
 * 捕获阶段先于组件的冒泡 handler, 才能真正拦下导航。
 */
const { guard: tvSettingsGuard, isIntercepted: tvSettingsIsNative } = useTvSettingsEntry()
/**
 * 副标题随宿主变化。这里在 setup 期取一次值即可(**不要**用 computed 去包它)：
 * 原生桥是 addJavascriptInterface 在 loadUrl 之前注入的 ⇒ 本次页面生命周期内恒定，
 * 用 computed 包一个非响应式函数只会伪装成"会跟着变"(值永不变、也不会失效)，
 * 与 composable 里"实时判断、不做快照"的注释自相矛盾。
 * 真正需要实时判断的是 open()/guard() 内部的 isIntercepted()。
 */
const showTvSettingsCard = tvSettingsIsNative()
const tvSettingsSub = '设备/账号'

/**
 * 金刚区功能卡数(4~6): 收藏/分类/搜索/我的 恒在, 直播/设置按条件渲染。
 * 作为 --jc-tv-func-n 传给网格 ⇒ 列数锁死为卡数, 任何分辨率下都一行铺满
 * (auto-fit 按视口宽现算列数, 6 张卡时会出现"5+1 折行", 2026-11-08 用户反馈)。
 */
const tvFuncCount = computed(
  () => 4 + (props.hasLiveSources ? 1 : 0) + (showTvSettingsCard ? 1 : 0)
)
function onTvSettingsClick(e: MouseEvent): void {
  tvSettingsGuard(e)
}

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

/** TV 各分类专区: 名称 + 该分类 top 3 影片(替代纯文字分类卡, 用宿主传入的 homeRows 派生) */
interface TvCatPanel {
  id: number
  name: string
  items: Card[]
}
const tvCatPanels = computed<TvCatPanel[]>(() => {
  return props.homeRows
    .map((row) => {
      const list = row.hot?.length ? row.hot : (row.latest ?? [])
      return { id: row.nav.id, name: row.nav.name, items: list.slice(0, 3) }
    })
    .filter((p) => p.items.length > 0)
})

onMounted(() => {
  restartTvHeroTimer()
})
onBeforeUnmount(() => {
  if (tvHeroTimer !== null) {
    window.clearInterval(tvHeroTimer)
    tvHeroTimer = null
  }
})
</script>

<template>
  <div class="jc-home-tv container-page">
    <!-- ① 顶部: 近期历史(3 片同款影片卡显当前集数) + 推荐轮播, 各占一半 -->
    <div class="jc-home-tv__top" :class="{ 'no-recent': !tvRecent.length }">
      <section v-if="tvRecent.length" class="jc-tv-panel z1">
        <TvSectionHeader title="⏱ 近期历史">
          <template #more>
            <RouterLink class="jc-tv-more" to="/history" data-focusable="true" tabindex="0">全部</RouterLink>
          </template>
        </TvSectionHeader>
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

      <!-- 推荐轮播 (自动轮播 heroSlides; 海报作背景, 点击进详情; 触摸横滑切上/下一张)
           图序对齐桌面版 HeroCarousel(2026-10-10): 横图(poster)优先、竖图(cover)兜底 ——
           原来反着写导致 TV 首屏竖图被 16:9 容器裁得只剩脸。 -->
      <RouterLink
        v-if="tvHero"
        :to="tvHeroTo"
        class="jc-tv-carousel jc-home-tv__hero"
        data-focusable="true"
        tabindex="0"
        :aria-label="`为你推荐 ${tvHero.name}`"
        @click="onTvHeroClick"
        @touchstart="onTvHeroTouchStart"
        @touchmove="onTvHeroTouchMove"
        @touchend="onTvHeroTouchEnd"
      >
        <BaseImage
          v-if="tvHero.poster || tvHero.cover"
          :key="tvHero.mid ?? tvHero.name"
          class="jc-home-tv__hero-bg"
          :src="tvHero.poster || tvHero.cover || ''"
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
         注: 卡片本就指向 /favorites, 故主文案用"收藏"(用户拍板), 不再写"历史·收藏"占宽.
         --jc-tv-func-n(2026-11-08): 列数 = 卡数(tv-cards.css), 任何分辨率下恒一行铺满,
         替代 2026-10-10 的 --no-settings 切列方案(其 CSS 钩子已随 2026-10-30 auto-fit 改造删除)。 -->
    <div class="jc-tv-funcs" :style="{ '--jc-tv-func-n': tvFuncCount }">
      <RouterLink class="jc-tv-fc fc-2" to="/favorites" data-focusable="true" tabindex="0">
        <span class="ic"><BaseIcon name="heart" size="32px" /></span><span class="ti">收藏</span><span class="su">记录您的热爱</span>
      </RouterLink>
      <RouterLink class="jc-tv-fc fc-3" :to="tvFirstPid ? { path: '/filmClassify', query: { Pid: tvFirstPid } } : '/filmClassify'" data-focusable="true" tabindex="0">
        <span class="ic"><BaseIcon name="film" size="32px" /></span><span class="ti">分类</span><span class="su">剧/影/综/漫</span>
      </RouterLink>
      <!-- 直播卡: 无源时隐藏(用户原话 "tv金刚区不受开关控制", 只看有无源) -->
      <RouterLink
        v-if="hasLiveSources"
        class="jc-tv-fc fc-4"
        to="/live"
        data-focusable="true"
        tabindex="0"
      >
        <span class="ic"><BaseIcon name="tv" size="32px" /></span><span class="ti">直播</span><span class="su">电视频道在线看</span>
      </RouterLink>
      <RouterLink class="jc-tv-fc fc-4" to="/search" data-focusable="true" tabindex="0">
        <span class="ic"><BaseIcon name="search" size="32px" /></span><span class="ti">搜索</span><span class="su">找片更快</span>
      </RouterLink>
      <!-- 我的卡(2026-10-10): /settings 已删 ⇒ 未登录去 /login; 已登录在原生壳里
           开抽屉(guard 拦掉跳转, 抽屉「账号」行直接登录/退出), 纯网页兜底去 /login
           (LoginView 对已登录用户会自动 redirect 走)。 -->
      <RouterLink class="jc-tv-fc fc-1" to="/login" data-focusable="true" tabindex="0" @click.capture="onTvSettingsClick">
        <span class="ic"><BaseIcon name="user" size="32px" /></span><span class="ti">我的</span><span class="su">{{ isLoggedIn ? '账号 · 退出' : '点击登录' }}</span>
      </RouterLink>
      <RouterLink v-if="showTvSettingsCard" class="jc-tv-fc fc-5" to="/login" data-focusable="true" tabindex="0" @click.capture="onTvSettingsClick">
        <span class="ic"><BaseIcon name="settings" size="32px" /></span><span class="ti">设置</span><span class="su">{{ tvSettingsSub }}</span>
      </RouterLink>
    </div>

    <!-- ④ 专区面板: 热门榜单 + 最新上架 (每块≤3 张 FilmCard) -->
    <div class="jc-tv-duo">
      <section v-if="tvHotPanel.length" class="jc-tv-panel z1">
        <TvSectionHeader title="🔥 热门榜单" sub="最热抢先看">
          <template #more>
            <RouterLink
              class="jc-tv-more"
              to="/hotBoard"
              data-focusable="true"
              tabindex="0"
            >更多内容</RouterLink>
          </template>
        </TvSectionHeader>
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
        <TvSectionHeader title="🆕 最新上架" sub="每日更新">
          <template #more>
            <RouterLink
              class="jc-tv-more"
              :to="{ path: '/filmClassify', query: { Pid: tvLatestPid } }"
              data-focusable="true"
              tabindex="0"
            >查看全部</RouterLink>
          </template>
        </TvSectionHeader>
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
        <TvSectionHeader :title="p.name" :sub="catSubtitle(p.name)">
          <template #more>
            <RouterLink
              class="jc-tv-more"
              :to="{ path: '/filmClassify', query: { Pid: p.id } }"
              data-focusable="true"
              tabindex="0"
            >更多</RouterLink>
          </template>
        </TvSectionHeader>
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
</template>

<!-- ============================================================
 * TV 雷鸟仪表盘布局胶水 (chrome 卡片样式来自全局 tv-cards.css, 此处只补容器/栅格)。
 * 样式门控二选一(方案 B 拆分注记): 本组件**仅 TV 模式挂载**(宿主 HomeView 以
 * v-if="isTV" 渲染), 故由 HomeView 的全局块改为 scoped —— 不会泄漏到桌面/移动;
 * 选择器仍保留 [data-mode='tv'] 前缀, 维持迁移前的特异性口径(双保险)。
 * ============================================================ -->
<style scoped>
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

/* 推荐轮播: 海报作背景铺满, 文字浮在上层。
 * touch-action: pan-y(2026-10-10) —— 横滑由我们的 swipe 检测接管切上/下一张,
 * 声明纵向滚动保留, 防浏览器把横滑当历史导航/其它手势劫持。 */
[data-mode='tv'] .jc-home-tv__hero {
  min-height: clamp(180px, 22vw, 280px);
  touch-action: pan-y;
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
