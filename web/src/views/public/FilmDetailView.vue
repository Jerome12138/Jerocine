<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { filmApi } from '@/api'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import RelatedList from '@/components/film/RelatedList.vue'
import EpisodeTabs from '@/components/film/EpisodeTabs.vue'
import type { Card, FilmDetail, FilmDetailResp } from '@/types/film'
import { useHistoryStore } from '@/stores/history'
import { useFavoriteStore } from '@/stores/favorite'
import { useSiteStore } from '@/stores/site'
import { storeToRefs } from 'pinia'
import { isNative } from '@/utils/jerocineNative'
import { formatHotBadge } from '@/utils/format'
import { useViewMode } from '@/composables/useViewMode'
import { useGridRowsLimit } from '@/composables/useGridRowsLimit'
import { dispatchNativePlaylist } from '@/utils/nativePlay'

/**
 * 影片详情 — STORY-009
 * - id 来自 route.query.link（旧站约定）
 * - 调 GET /api/filmDetail
 * - hero：模糊大图背景 + 海报 + 信息（标题 / 评分 / 类型 / 标签 / 导演 / 主演 / 上映 / 地区 / 剧情）
 * - 剧情可展开收起（>140 字截断）
 * - 立即播放：跳 /play?id=&source=&episode=  （source = list[0].id, episode = 0）
 * - EpisodeTabs：sources=detail.sources；选集走 router.push
 * - RelatedList：传入 relate
 *
 * TV(雷鸟卡片式) 分支：isTV 时复用同一份 state/方法/播放契约, 仅换 chrome 布局
 * (cover 模糊大背景 + 左竖海报 + 右信息 + 操作组 + 选集 + 相关推荐 6 列 FilmCard)。
 */

const route = useRoute()
const router = useRouter()
const historyStore = useHistoryStore()
const favoriteStore = useFavoriteStore()
const siteStore = useSiteStore()
const { map: favoriteMap } = storeToRefs(favoriteStore)
const { isDesktop, isTV } = useViewMode()

/** 分享: 复制当前页 URL 到剪贴板, 短暂反馈 (优先用后台配置的 domain 拼绝对地址) */
const shareLabel = ref<string>('分享')
async function handleShare(): Promise<void> {
  const origin = siteStore.basic?.domain || (typeof window !== 'undefined' ? window.location.origin : '')
  const url = origin + (typeof window !== 'undefined' ? window.location.pathname + window.location.search : '')
  if (!url) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url)
    } else {
      const t = document.createElement('textarea')
      t.value = url
      document.body.appendChild(t)
      t.select()
      document.execCommand('copy')
      document.body.removeChild(t)
    }
    shareLabel.value = '已复制'
  } catch {
    shareLabel.value = '复制失败'
  }
  window.setTimeout(() => { shareLabel.value = '分享' }, 1800)
}

const linkId = computed(() => {
  const v = route.query.link
  if (typeof v === 'string') return v
  if (Array.isArray(v) && typeof v[0] === 'string') return v[0]
  return ''
})

const loading = ref(true)
const errored = ref(false)
const detail = ref<FilmDetail | null>(null)
const relate = ref<Card[]>([])
const { limitToRows } = useGridRowsLimit()
const visibleRelate = computed(() => limitToRows(relate.value))

async function loadDetail(id: string): Promise<void> {
  loading.value = true
  errored.value = false
  detail.value = null
  relate.value = []
  if (!id) {
    errored.value = true
    loading.value = false
    return
  }
  try {
    const resp: FilmDetailResp = await filmApi.getFilmDetail(id)
    detail.value = resp?.detail ?? null
    relate.value = Array.isArray(resp?.related) ? resp.related : []
    if (!detail.value) {
      errored.value = true
    }
  } catch {
    errored.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => loadDetail(linkId.value))
watch(linkId, (next) => {
  if (next) loadDetail(next)
})

/** ============== 文案处理 ============== */

/** 清理 content 内 HTML 实体 / 全角空格 / br 等 */
function cleanContent(raw: string | undefined): string {
  if (!raw) return ''
  return raw.replace(/(&.*?;)|( )|(　　)|(\n)|(<[^>]+>)/g, '').trim()
}

/** 取前 N 个，逗号 / 空格 / 斜杠 / 顿号 切片 */
function takeNames(raw: string | undefined, max = 3): string[] {
  if (!raw) return []
  return raw
    .split(/[,，、\/\s]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
    .slice(0, max)
}

/** Hero 背景图: 优先 TMDB 回填的横图(16:9, 构图适合宽幅铺底), 回退竖版 cover（CSS escaping，防御后端字段污染） */
const heroBg = computed(() => detail.value?.backdrop || detail.value?.cover || '')
const heroBgStyle = computed(() => {
  const url = heroBg.value
  if (!url) return undefined
  // 用 JSON.stringify 转义引号 / 反斜杠等 CSS 注入字符
  return { backgroundImage: `url(${JSON.stringify(url)})` }
})

/** 标签 */
const tagChips = computed<string[]>(() => {
  const d = detail.value
  if (!d) return []
  const chips: string[] = []
  if (d.year) chips.push(String(d.year))
  if (d.cName) chips.push(d.cName)
  if (d.area) chips.push(String(d.area))
  if (d.classTag) {
    for (const t of d.classTag.split(/[,，、\/\s]+/)) {
      const v = t.trim()
      if (v) chips.push(v)
      if (chips.length >= 6) break
    }
  }
  return chips.slice(0, 6)
})

const directors = computed(() => takeNames(detail.value?.director, 6))
const actors = computed(() => takeNames(detail.value?.actor, 12))

/** 演职人员卡: 名字 → 渐变首字头像 (基于 hash 选 8 种渐变之一) */
const PERSON_GRADIENTS = [
  'linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #e50914 100%)',
  'linear-gradient(135deg, #22c55e 0%, #4ad1e5 100%)',
  'linear-gradient(135deg, #3b82f6 0%, #9b49e7 100%)',
  'linear-gradient(135deg, #ef4444 0%, #f59e0b 100%)',
  'linear-gradient(135deg, #06b6d4 0%, #6366f1 100%)',
  'linear-gradient(135deg, #ec4899 0%, #f59e0b 100%)',
  'linear-gradient(135deg, #14b8a6 0%, #4ad1e5 100%)'
]
function personGradient(name: string): string {
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) & 0xffffffff
  return PERSON_GRADIENTS[Math.abs(h) % PERSON_GRADIENTS.length] ?? PERSON_GRADIENTS[0]!
}
function personInitial(name: string): string {
  // 圆圈内最多显示 4 个字, 排成 2×2(一行2字两行); CSS 用固定宽 + break 实现换行
  const n = (name ?? '').trim()
  if (!n) return '?'
  return [...n].slice(0, 4).join('')
}

/** 评分 */
const score = computed(() => {
  const n = detail.value?.dbScore
  if (n === undefined || n === null || !Number.isFinite(n) || n < 1) return ''
  return n.toFixed(1)
})

/** 豆瓣热度榜位(榜单刷新任务标记, 缺省/0 = 不在榜) —— 详情页「豆瓣·热门电影 No.N」。
 * hotBoard 是榜位来源榜单中文名(同一部片在多个集合各有位次, 光看 No.N 分不清是哪个榜)。
 * 与轮播首屏大图共用 formatHotBadge: 两处显示必然一致, 也不会出现没有分类的「热门」
 * (后端 douban.HotBoardLabel 在缺 hot_board 时按分类热榜兜底, pid 4 → 热门动漫)。 */
const hotBadge = computed(() => formatHotBadge(detail.value?.hotRank, detail.value?.hotBoard))

/** 剧情展开 */
const SUMMARY_LIMIT = 140
const expanded = ref(false)
const cleanedContent = computed(() =>
  cleanContent(detail.value?.content)
)
const needsClamp = computed(() => cleanedContent.value.length > SUMMARY_LIMIT)
const displayContent = computed(() => {
  if (!needsClamp.value || expanded.value) return cleanedContent.value
  return cleanedContent.value.slice(0, SUMMARY_LIMIT) + '…'
})

/** ============== 跳转 ============== */

/**
 * 跳播放. APK 壳 (isNative) 时直接调 native player 不走 /play 路由 — 之前先 router.push
 * 到 /play, PlayView setup 再 detect native 调 playPlaylist, 体验上是"先闪一下空白 web
 * 页再开 native player". 直跳省去中间 web 渲染.
 *
 * resumeSec 可选, 继续观看时传入秒数.
 *
 * 关键: native 路径必须先 historyStore.record(...) 建一条历史 — 否则 native 周期
 * playerProgress 回到 web, historyStore.updateProgress 因"无已有 record"直接 no-op,
 * 详情页 resumeRecord 永远是旧的, 退出 player 也看不到最新集数 / 进度.
 */
function gotoPlay(sourceId: string, episodeIndex: number, resumeSec = 0): void {
  const d = detail.value
  if (!d) return
  const src = d.sources?.find((s) => s.id === sourceId) ?? d.sources?.[0]
  const ep = src?.episodes?.[episodeIndex]
  if (src && ep) {
    historyStore.record({
      id: String(d.mid),
      name: d.name,
      link: `/play?id=${d.mid}&source=${src.id}&episode=${episodeIndex}` +
        (resumeSec > 0 ? `&currentTime=${Math.floor(resumeSec)}` : ''),
      episode: ep.episode ?? String(episodeIndex + 1),
      picture: d.cover,
      source: src.id,
      episodeIndex,
      currentTime: resumeSec > 0 ? Math.floor(resumeSec) : 0,
      pid: d.pid,
      cid: d.cid
    })
  }
  if (isNative()) {
    // 统一派发(映射源+建历史+skip+proxyBase 全在 dispatchNativePlaylist 一处)
    if (dispatchNativePlaylist(d, sourceId, episodeIndex, resumeSec)) return
    // 数据不全, 回退到 web 路由让 PlayView 自己拉
  }
  router.push({
    path: '/play',
    query: {
      id: String(d.mid),
      source: sourceId,
      episode: String(episodeIndex)
    }
  })
}

function playFirst(): void {
  const d = detail.value
  if (!d) return
  const firstSource = d.sources?.[0]
  if (!firstSource || !firstSource.episodes?.length) return
  gotoPlay(firstSource.id, 0)
}

/** "继续观看": 用户在本机看过该片时显示, 跳到上次中断的源/集 */
const resumeRecord = computed(() => {
  const d = detail.value
  if (!d) return null
  const rec = historyStore.get(String(d.mid))
  if (!rec || !rec.source) return null
  // 校验记录里的 source/episode 在当前 detail 仍然有效
  const src = d.sources?.find((s) => s.id === rec.source)
  if (!src) return null
  const idx = Math.max(0, rec.episodeIndex ?? 0)
  if (!src.episodes[idx]) return null
  return {
    source: rec.source,
    episodeIndex: idx,
    episodeName: src.episodes[idx]?.episode || String(idx + 1),
    currentTime: rec.currentTime ?? 0
  }
})

/** 详情页选集当前激活的播放源(本地可控状态). 切换源 tab 时更新, 让 EpisodeTabs 反映新源并刷新选集网格.
 * 初始: 续播源优先, 否则首源; 详情加载/续播记录就绪后设定一次(用户已手动切源则不覆盖). */
const activeSourceId = ref<string>('')
function syncInitialSource(): void {
  if (activeSourceId.value) return // 用户已手动切源, 不覆盖
  const rec = resumeRecord.value
  const d = detail.value
  if (rec?.source) activeSourceId.value = rec.source
  else if (d?.sources?.[0]?.id) activeSourceId.value = d.sources[0].id
}
watch([detail, resumeRecord], syncInitialSource, { immediate: true })

function onSelectSource(sid: string): void {
  activeSourceId.value = sid
}
function resumeWatching(): void {
  const r = resumeRecord.value
  if (!r) return
  const d = detail.value
  if (!d) return
  // 走统一 gotoPlay (APK 直接 native, web 才路由)
  if (isNative()) {
    gotoPlay(r.source, r.episodeIndex, r.currentTime)
    return
  }
  router.push({
    path: '/play',
    query: {
      id: String(d.mid),
      source: r.source,
      episode: String(r.episodeIndex),
      currentTime: r.currentTime > 0 ? String(r.currentTime) : undefined
    }
  })
}

/** ============== 收藏 ============== */
const isFavorited = computed(() => {
  const d = detail.value
  if (!d) return false
  return !!favoriteMap.value[String(d.mid)]
})

function handleToggleFavorite(): void {
  const d = detail.value
  if (!d) return
  void favoriteStore.toggle({
    id: String(d.mid),
    name: d.name,
    picture: d.cover,
    remarks: d.remarks,
    pid: d.pid,
    cid: d.cid
  })
}

</script>

<template>
  <div class="jc-detail">
    <!-- 加载骨架 -->
    <template v-if="loading">
      <div class="jc-detail__hero jc-detail__hero--skeleton">
        <div class="jc-detail__hero-inner container-page">
          <BaseSkeleton
            shape="rect"
            width="100%"
            ratio="3/4"
            class="jc-detail__poster-skel"
          />
          <div class="flex flex-col gap-[var(--jc-space-3)] flex-1 min-w-0">
            <BaseSkeleton shape="text" width="60%" height="40px" />
            <BaseSkeleton shape="text" width="40%" />
            <BaseSkeleton shape="text" :count="4" />
          </div>
        </div>
      </div>
    </template>

    <!-- 错误 -->
    <template v-else-if="errored || !detail">
      <div class="container-page py-[var(--jc-space-12)]">
        <BaseEmpty
          title="影片不存在或加载失败"
          :description="linkId ? `id=${linkId} 未找到对应内容` : '请从首页或搜索进入此页面'"
        >
          <template #action>
            <BaseButton variant="primary" size="md" @click="router.push('/index')">
              返回首页
            </BaseButton>
          </template>
        </BaseEmpty>
      </div>
    </template>

    <!-- ============== TV(雷鸟卡片式) 分支 ============== -->
    <template v-else-if="isTV">
      <!-- 该片 cover 模糊大背景铺满整框 (overflow:clip 不生成滚动容器) -->
      <div class="jc-detail-tv">
        <div class="jc-detail-tv__bg" :style="heroBgStyle" />
        <div class="jc-detail-tv__mask" />

        <div class="jc-detail-tv__body container-page">
          <!-- ===== Hero: 左竖海报 + 右信息 ===== -->
          <div class="jc-detail-tv__hero">
            <!-- 左侧大竖海报 -->
            <div class="jc-detail-tv__poster" data-focusable="true" tabindex="0">
              <BaseImage
                :src="detail.cover"
                :alt="detail.name"
                ratio="3/4"
                eager
                fit="cover"
              />
              <span v-if="score" class="jc-detail-tv__poster-score">★ {{ score }}</span>
            </div>

            <!-- 右侧信息 -->
            <div class="jc-detail-tv__info">
              <h1 class="jc-detail-tv__title">{{ detail.name }}</h1>

              <!-- ★评分 + 年份·地区·类型 chip -->
              <div v-if="score || tagChips.length" class="jc-detail-tv__chips">
                <span v-if="score" class="jc-detail-tv__score">★ {{ score }}</span>
                <span
                  v-for="(t, i) in tagChips"
                  :key="'chip-' + i"
                  class="jc-tv-chip"
                  :class="i === 0 ? 'cur' : ''"
                >{{ t }}</span>
              </div>

              <!-- 元信息: 上映 / 状态 / 语言 / 导演 / 热度榜位 -->
              <div
                v-if="detail.remarks || detail.language || directors.length || detail.pubDate || hotBadge"
                class="jc-detail-tv__meta"
              >
                <span v-if="detail.pubDate"><i>上映 </i>{{ detail.pubDate }}</span>
                <span v-if="detail.remarks"><i>状态 </i>{{ detail.remarks }}</span>
                <span v-if="detail.language"><i>语言 </i>{{ detail.language }}</span>
                <span v-if="directors.length"><i>导演 </i>{{ directors.join(' / ') }}</span>
                <span v-if="hotBadge"><i>热度 </i>{{ hotBadge }}</span>
              </div>

              <!-- 简介 ≤3 行 -->
              <p v-if="cleanedContent" class="jc-detail-tv__summary">
                {{ cleanedContent }}
              </p>

              <!-- 操作按钮组 (复用现有播放/续播/收藏逻辑与原生派发) -->
              <div class="jc-detail-tv__cta">
                <!-- 续播优先: 有进度时主焦点落在"继续观看" -->
                <button
                  v-if="resumeRecord"
                  type="button"
                  class="jc-tv-btn primary"
                  data-focusable="true"
                  tabindex="0"
                  @click="resumeWatching"
                >
                  <BaseIcon name="play" size="1.1em" />
                  继续观看 · {{ resumeRecord.episodeName }}
                </button>
                <button
                  type="button"
                  class="jc-tv-btn"
                  :class="resumeRecord ? '' : 'primary'"
                  :disabled="!detail.sources?.[0]?.episodes?.length"
                  data-focusable="true"
                  tabindex="0"
                  @click="playFirst"
                >
                  <BaseIcon name="play" size="1.1em" />
                  {{ resumeRecord ? '从头播放' : '立即播放' }}
                </button>
                <button
                  type="button"
                  class="jc-tv-btn"
                  :class="isFavorited ? 'cyan' : ''"
                  data-focusable="true"
                  tabindex="0"
                  @click="handleToggleFavorite"
                >
                  <BaseIcon name="heart" size="1.1em" />
                  {{ isFavorited ? '已收藏' : '收藏' }}
                </button>
              </div>
            </div>
          </div>

          <!-- ===== 选集 (复用现有 EpisodeTabs 组件) ===== -->
          <section v-if="detail.sources?.length" class="jc-detail-tv__section" aria-label="选集">
            <div class="jc-tv-sec">
              <span class="t">选集</span>
              <span class="s">
                共 {{ detail.sources[0]?.episodes?.length ?? 0 }} 集 · {{ detail.sources.length }} 条线路
              </span>
            </div>
            <EpisodeTabs
              :sources="detail.sources"
              :current-source-id="activeSourceId"
              :current-episode="resumeRecord ? (detail.sources.find(s => s.id === resumeRecord!.source)?.episodes?.[resumeRecord!.episodeIndex]?.link ?? '') : ''"
              :watched-links="[]"
              :page-size="30"
              :film-name="detail.name"
              @change-source="onSelectSource"
              @select="(p) => gotoPlay(p.sourceId, p.episodeIndex)"
            />
          </section>

          <!-- ===== 相关推荐 (jc-tv-grid 6 列 FilmCard) ===== -->
          <section v-if="visibleRelate.length" class="jc-detail-tv__section" aria-label="相关推荐">
            <div class="jc-tv-sec">
              <span class="t">相关推荐</span>
            </div>
            <div class="jc-tv-grid jc-detail-tv__relate-grid">
              <FilmCard
                v-for="item in visibleRelate"
                :key="'rel-' + item.mid"
                :item="item"
                :show-title-below="true"
              />
            </div>
          </section>
        </div>
      </div>
    </template>

    <!-- ============== 桌面 / 移动 (原样保留) ============== -->
    <template v-else>
      <!-- Hero -->
      <section class="jc-detail__hero">
        <div
          class="jc-detail__hero-bg"
          :style="heroBgStyle"
        />
        <div class="jc-detail__hero-mask" />

        <div class="jc-detail__hero-inner container-page">
          <!-- 海报 -->
          <div class="jc-detail__poster">
            <BaseImage
              :src="detail.cover"
              :alt="detail.name"
              ratio="3/4"
              eager
              fit="cover"
            />
          </div>

          <!-- 信息 -->
          <div class="jc-detail__info">
            <h1 class="jc-detail__title">{{ detail.name }}</h1>

            <div v-if="score || tagChips.length" class="jc-detail__chips">
              <span v-if="score" class="jc-detail__score">
                <BaseIcon name="star" size="0.9em" class="jc-detail__score-icon" />
                {{ score }}
              </span>
              <BaseTag
                v-for="(t, i) in tagChips"
                :key="i"
                :variant="i === 0 ? 'purple' : 'default'"
                size="md"
              >
                {{ t }}
              </BaseTag>
            </div>

            <!-- hero meta: 年份/地区已由上方标签行承载(chips), 此处不再重复;
                 只留 上映/语言/状态/热度 等标签行没有的字段, 演职人员下沉到独立 section -->
            <dl
              v-if="detail.pubDate || detail.language || detail.remarks || hotBadge"
              class="jc-detail__meta"
            >
              <div v-if="detail.pubDate" class="jc-detail__meta-row">
                <dt>上映</dt>
                <dd>{{ detail.pubDate }}</dd>
              </div>
              <div v-if="detail.language" class="jc-detail__meta-row">
                <dt>语言</dt>
                <dd>{{ detail.language }}</dd>
              </div>
              <div v-if="detail.remarks" class="jc-detail__meta-row">
                <dt>状态</dt>
                <dd>{{ detail.remarks }}</dd>
              </div>
              <div v-if="hotBadge" class="jc-detail__meta-row">
                <dt>热度</dt>
                <dd>{{ hotBadge }}</dd>
              </div>
            </dl>

            <p v-if="cleanedContent" class="jc-detail__summary">
              {{ displayContent }}
              <button
                v-if="needsClamp"
                type="button"
                class="jc-detail__expand"
                @click="expanded = !expanded"
              >
                {{ expanded ? '收起' : '展开' }}
              </button>
            </p>

            <div class="jc-detail__cta">
              <!-- 续播优先, 有进度时主按钮变"继续观看" -->
              <BaseButton
                v-if="resumeRecord"
                variant="primary"
                size="md"
                @click="resumeWatching"
              >
                <template #icon>
                  <BaseIcon name="play" size="1.1em" />
                </template>
                继续观看 · {{ resumeRecord.episodeName }}
              </BaseButton>
              <BaseButton
                :variant="resumeRecord ? 'outline' : 'primary'"
                size="md"
                :disabled="!detail.sources?.[0]?.episodes?.length"
                @click="playFirst"
              >
                <template #icon>
                  <BaseIcon name="play" size="1.1em" />
                </template>
                {{ resumeRecord ? '从头播放' : '立即播放' }}
              </BaseButton>
              <BaseButton
                :variant="isFavorited ? 'primary' : 'outline'"
                size="md"
                @click="handleToggleFavorite"
              >
                <template #icon>
                  <BaseIcon name="heart" size="1.1em" />
                </template>
                {{ isFavorited ? '已收藏' : '收藏' }}
              </BaseButton>
              <!-- 分享: TV 无复制场景不显示; 桌面/移动/Pad 显示, 点击复制链接 -->
              <BaseButton
                v-if="!isTV"
                variant="ghost"
                size="md"
                @click="handleShare"
              >
                <template #icon>
                  <BaseIcon name="share" size="1.1em" />
                </template>
                {{ shareLabel }}
              </BaseButton>
            </div>
          </div>
        </div>
      </section>

      <!-- 集数选择 (详情页直选, 不必先点"立即播放"再切集) -->
      <section
        v-if="detail.sources?.length"
        class="container-page py-[var(--jc-space-5)]"
        aria-label="选集"
      >
        <h2 class="jc-detail__section-title mb-[var(--jc-space-3)]">选集</h2>
        <EpisodeTabs
          :sources="detail.sources"
          :current-source-id="activeSourceId"
          :current-episode="resumeRecord ? (detail.sources.find(s => s.id === resumeRecord!.source)?.episodes?.[resumeRecord!.episodeIndex]?.link ?? '') : ''"
          :watched-links="[]"
          :page-size="30"
          :film-name="detail.name"
          @change-source="onSelectSource"
          @select="(p) => gotoPlay(p.sourceId, p.episodeIndex)"
        />
      </section>

      <!-- 演职人员 (横滚卡片: 渐变首字头像 + 名字), bilibili/腾讯视频风格 -->
      <section
        v-if="directors.length || actors.length"
        class="jc-detail__cast container-page"
        aria-label="演职人员"
      >
        <h2 class="jc-detail__section-title">演职人员</h2>
        <div class="jc-detail__cast-scroll">
          <div
            v-for="(name, i) in directors"
            :key="'d-' + i"
            class="jc-detail__person"
          >
            <span
              class="jc-detail__person-avatar"
              :style="{ backgroundImage: personGradient(name) }"
              aria-hidden="true"
            >
              <span class="jc-detail__person-avatar-text">{{ personInitial(name) }}</span>
            </span>
            <span class="jc-detail__person-name" :title="name">{{ name }}</span>
            <span class="jc-detail__person-role">导演</span>
          </div>
          <div
            v-for="(name, i) in actors"
            :key="'a-' + i"
            class="jc-detail__person"
          >
            <span
              class="jc-detail__person-avatar"
              :style="{ backgroundImage: personGradient(name) }"
              aria-hidden="true"
            >
              <span class="jc-detail__person-avatar-text">{{ personInitial(name) }}</span>
            </span>
            <span class="jc-detail__person-name" :title="name">{{ name }}</span>
            <span class="jc-detail__person-role">主演</span>
          </div>
        </div>
      </section>

      <!-- 相关推荐 (选集职责已移交播放页, 详情页只做"看不看"决策) -->
      <section v-if="visibleRelate.length" class="jc-detail__relate container-page">
        <RelatedList :items="visibleRelate" title="相关推荐" />
      </section>
    </template>
  </div>
</template>

<style scoped>
.jc-detail {
  width: 100%;
  display: flex;
  flex-direction: column;
}

/* ============= Hero ============= */
.jc-detail__hero {
  position: relative;
  width: 100%;
  isolation: isolate;
  /* clip 而非 hidden: 同样裁掉 inset:-40px 的模糊背景, 但不生成滚动容器 —
   * 否则 TV 遥控器聚焦内部按钮时 scrollIntoView 会滚动本区, 致海报+文字整体偏移。 */
  overflow: clip;
}

.jc-detail__hero-bg {
  position: absolute;
  inset: -40px;
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
  filter: blur(40px) brightness(0.4);
  transform: scale(1.1);
  z-index: 0;
}

.jc-detail__hero-mask {
  position: absolute;
  inset: 0;
  background-image: linear-gradient(
    180deg,
    rgba(11, 11, 15, 0.5) 0%,
    rgba(11, 11, 15, 0.78) 75%,
    rgba(11, 11, 15, 1) 100%
  );
  z-index: 0;
}

.jc-detail__hero-inner {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-6);
  padding-top: var(--jc-space-8);
  padding-bottom: var(--jc-space-12);
  align-items: center;
  text-align: center;
}

@media (min-width: 768px) {
  .jc-detail__hero-inner {
    flex-direction: row;
    align-items: flex-start;
    text-align: left;
    gap: var(--jc-space-8);
    padding-top: var(--jc-space-12);
    padding-bottom: var(--jc-space-12);
  }
}

.jc-detail__poster {
  width: 200px;
  flex-shrink: 0;
  border-radius: var(--jc-radius-lg);
  overflow: hidden;
  box-shadow: var(--jc-shadow-xl);
}

.jc-detail__poster-skel {
  width: 200px;
  flex-shrink: 0;
}

@media (min-width: 768px) {
  .jc-detail__poster,
  .jc-detail__poster-skel {
    width: 220px;
  }
}

@media (min-width: 1024px) {
  .jc-detail__poster,
  .jc-detail__poster-skel {
    width: 240px;
  }
}

/* TV 模式 (≥1024 视口下也常用) — 海报再瘦一档, 标题字号小一档.
 * 用户反馈"图有点大文字也有点大", 减视觉压迫. */
[data-mode='tv'] .jc-detail__poster,
[data-mode='tv'] .jc-detail__poster-skel {
  width: 200px;
}
[data-mode='tv'] .jc-detail__title {
  font-size: var(--jc-fs-2xl);
}

.jc-detail__info {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-4);
  flex: 1;
  min-width: 0;
  align-items: center;
}

@media (min-width: 768px) {
  .jc-detail__info {
    align-items: flex-start;
  }
}

.jc-detail__title {
  font-family: var(--jc-font-display);
  /* 影片名小一点: 2xl→xl */
  font-size: var(--jc-fs-xl);
  font-weight: var(--jc-fw-black);
  letter-spacing: var(--jc-tracking-tight);
  line-height: var(--jc-lh-tight);
  color: var(--jc-text-primary);
  margin: 0;
}

@media (min-width: 768px) {
  .jc-detail__title {
    font-size: var(--jc-fs-2xl);
  }
}

/* 平板/TV: 标题再降一档. 用户反馈这两端文字偏大占位多;
 * PC(desktop) 与手机(mobile) 不变. */
[data-mode='tablet'] .jc-detail__title,
[data-mode='tv'] .jc-detail__title {
  font-size: var(--jc-fs-2xl);
}

.jc-detail__chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--jc-space-2);
}

.jc-detail__score {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 28px;
  padding: 0 10px;
  border-radius: var(--jc-radius-sm);
  background-color: rgba(245, 158, 11, 0.16);
  color: var(--jc-warning);
  font-weight: var(--jc-fw-bold);
  font-size: var(--jc-fs-sm);
}

.jc-detail__score-icon {
  margin-bottom: 1px;
}

.jc-detail__meta {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: var(--jc-text-secondary);
}

.jc-detail__meta-row {
  display: flex;
  gap: var(--jc-space-2);
  font-size: var(--jc-fs-sm);
  line-height: var(--jc-lh-snug);
  flex-wrap: wrap;
  justify-content: center;
}

@media (min-width: 768px) {
  .jc-detail__meta-row {
    justify-content: flex-start;
  }
}

.jc-detail__meta-row dt {
  flex: 0 0 auto;
  color: var(--jc-text-muted);
  min-width: 36px;
}

.jc-detail__meta-row dd {
  margin: 0;
  color: var(--jc-text-secondary);
  flex: 1;
  min-width: 0;
}

.jc-detail__summary {
  margin: 0;
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  line-height: var(--jc-lh-relaxed);
  max-width: 720px;
}

.jc-detail__expand {
  display: inline;
  background: transparent;
  border: none;
  color: var(--jc-text-link);
  font-size: var(--jc-fs-sm);
  cursor: pointer;
  padding: 0 4px;
}

.jc-detail__expand:hover,
.jc-detail__expand:focus-visible {
  color: var(--jc-text-link-hover);
  outline: none;
}

.jc-detail__cta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-2);
  justify-content: center;
}

/* 操作按钮整体收一档: 原为 size="lg"(48px 高 / 左右 24px / 18px 字),
 * 现走 md 档(40px 高 / 左右 16px), 字号再收到 16px。
 * 基类 .jc-btn 有 min-height:44px 的下限(触控命中区), 会把 md 的 40px 顶回 44px,
 * 所以这里显式放开下限 —— 该块是详情页核心操作区, 40px 仍满足命中需求。 */
.jc-detail__cta :deep(.jc-btn) {
  min-height: 40px;
  font-size: var(--jc-fs-base);
}

@media (min-width: 768px) {
  .jc-detail__cta {
    justify-content: flex-start;
  }
}

/* ============= Cast (演职人员) ============= */
.jc-detail__cast {
  padding-block: var(--jc-space-8);
}
.jc-detail__cast-scroll {
  display: flex;
  gap: var(--jc-space-4);
  overflow-x: auto;
  padding-block: var(--jc-space-2);
  margin-inline: calc(-1 * var(--jc-gutter-mobile));
  padding-inline: var(--jc-gutter-mobile);
  /* 默认隐藏滚动条(原生横向条不好看), hover 时浮现细条 */
  scrollbar-width: none;
}
.jc-detail__cast-scroll::-webkit-scrollbar {
  height: 6px;
}
.jc-detail__cast-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.jc-detail__cast-scroll::-webkit-scrollbar-thumb {
  background-color: transparent;
  border-radius: 3px;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-detail__cast-scroll:hover {
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.25) transparent;
}
.jc-detail__cast-scroll:hover::-webkit-scrollbar-thumb {
  background-color: rgba(255, 255, 255, 0.25);
}
@media (min-width: 768px) {
  .jc-detail__cast-scroll {
    margin-inline: calc(-1 * var(--jc-gutter-tablet));
    padding-inline: var(--jc-gutter-tablet);
    gap: var(--jc-space-5);
  }
}
@media (min-width: 1024px) {
  .jc-detail__cast-scroll {
    margin-inline: 0;
    padding-inline: 0;
  }
}

.jc-detail__person {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  width: 72px;
  text-align: center;
}
@media (min-width: 768px) {
  .jc-detail__person {
    width: 84px;
  }
}

.jc-detail__person-avatar {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 9999px;
  /* 圈内最多 4 字, 排 2×2(一行2字): 限制内容盒宽 ≈ 2 字, word-break 自然换到第二行 */
  font-size: 14px;
  line-height: 1.15;
  letter-spacing: 0;
  text-align: center;
  word-break: break-all;
  overflow: hidden;
  font-weight: var(--jc-fw-bold);
  color: #fff;
  background-size: cover;
  background-position: center;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
/* 内容盒约束成 2 字宽 → 4 字自动 2 行 2 列 */
.jc-detail__person-avatar-text {
  display: block;
  max-width: 2.1em; /* ≈2 个全角字 */
  word-break: break-all;
}
@media (min-width: 768px) {
  .jc-detail__person-avatar {
    width: 68px;
    height: 68px;
    font-size: 16px;
  }
}

.jc-detail__person-name {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-primary);
  font-weight: var(--jc-fw-medium);
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-top: 6px;
}
.jc-detail__person-role {
  font-size: 10px;
  color: var(--jc-text-muted);
  line-height: 1;
}

/* ============= Episodes / Relate ============= */
.jc-detail__episodes,
.jc-detail__relate {
  padding-block: var(--jc-space-5);
}

.jc-detail__section-title {
  /* 收紧: xl→lg, 行高更紧, 下间距小一点 */
  font-size: var(--jc-fs-lg);
  font-weight: var(--jc-fw-bold);
  line-height: var(--jc-lh-snug);
  color: var(--jc-text-primary);
  margin: 0 0 var(--jc-space-3);
}

/* skeleton 容器 */
.jc-detail__hero--skeleton .jc-detail__hero-inner {
  z-index: 0;
}

/* ============= TV(雷鸟卡片式) 分支 ============= */
.jc-detail-tv {
  position: relative;
  width: 100%;
  isolation: isolate;
  /* clip 而非 hidden: 裁掉 inset:-40px 模糊背景但不生成滚动容器 —
   * 否则遥控器聚焦内部按钮时 scrollIntoView 会滚动本区致整体偏移。 */
  overflow: clip;
}
.jc-detail-tv__bg {
  position: absolute;
  inset: -40px;
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
  filter: blur(40px) brightness(0.42);
  transform: scale(1.1);
  z-index: 0;
}
.jc-detail-tv__mask {
  position: absolute;
  inset: 0;
  background-image: linear-gradient(
    180deg,
    rgba(11, 11, 15, 0.5) 0%,
    rgba(11, 11, 15, 0.82) 78%,
    rgba(11, 11, 15, 1) 100%
  );
  z-index: 0;
}
.jc-detail-tv__body {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-8);
  padding-block: var(--jc-space-6) var(--jc-space-12);
}

.jc-detail-tv__hero {
  display: flex;
  gap: var(--jc-space-6);
  align-items: flex-start;
}
.jc-detail-tv__poster {
  position: relative;
  width: clamp(150px, 16vw, 196px);
  flex-shrink: 0;
  border-radius: var(--jc-radius-lg);
  overflow: hidden;
  box-shadow: var(--jc-shadow-xl);
}
.jc-detail-tv__poster-score {
  position: absolute;
  top: 10px;
  left: 10px;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 10px;
  border-radius: var(--jc-radius-sm);
  background-color: rgba(0, 0, 0, 0.72);
  color: #ffc107;
  font-size: 14px;
  font-weight: var(--jc-fw-bold);
  line-height: 1;
  pointer-events: none;
}

.jc-detail-tv__info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-3);
}
.jc-detail-tv__title {
  font-family: var(--jc-font-display);
  font-size: var(--jc-fs-xl);
  font-weight: var(--jc-fw-black);
  letter-spacing: var(--jc-tracking-tight);
  line-height: var(--jc-lh-tight);
  color: var(--jc-text-primary);
  margin: 0;
}
.jc-detail-tv__chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--jc-space-2);
}
.jc-detail-tv__score {
  display: inline-flex;
  align-items: center;
  height: 40px;
  padding: 0 14px;
  border-radius: 999px;
  background-color: rgba(245, 158, 11, 0.16);
  color: var(--jc-warning);
  font-weight: var(--jc-fw-bold);
  font-size: var(--jc-fs-sm);
}
.jc-detail-tv__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-5);
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-secondary);
}
.jc-detail-tv__meta i {
  font-style: normal;
  color: var(--jc-text-muted);
}
.jc-detail-tv__summary {
  margin: 0;
  font-size: var(--jc-fs-sm);
  line-height: var(--jc-lh-relaxed);
  color: var(--jc-text-secondary);
  max-width: 820px;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.jc-detail-tv__cta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-4);
  margin-top: var(--jc-space-1);
}
.jc-detail-tv__cta .jc-tv-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.jc-detail-tv__relate-grid {
  /* 相关推荐固定 6 列 (覆盖 jc-tv-grid 的 auto-fit) */
  grid-template-columns: repeat(6, 1fr);
}

/* 收紧详情 TV 元素尺寸(用户反馈"文字元素都有点大") */
.jc-detail-tv__chips .jc-tv-chip {
  height: 36px;
  padding: 0 14px;
  font-size: var(--jc-fs-sm);
}
.jc-detail-tv__cta .jc-tv-btn {
  height: 44px;
  padding: 0 18px;
  font-size: var(--jc-fs-sm);
}
.jc-detail-tv__section .jc-tv-chip {
  height: 34px;
  padding: 0 14px;
  font-size: var(--jc-fs-xs);
}
.jc-detail-tv__section .jc-tv-eps {
  grid-template-columns: repeat(auto-fit, minmax(clamp(72px, 7vw, 108px), 1fr));
  gap: var(--jc-space-2);
}
.jc-detail-tv__section .jc-tv-ep {
  height: 44px;
  font-size: var(--jc-fs-sm);
}
</style>

<style>
/* TV 模式：安全区 + 收紧字号/间距 (用户反馈"文字太大、详情与选集间距太大") */
[data-mode='tv'] .jc-detail__hero-inner {
  padding-inline: var(--jc-tv-safe);
  flex-direction: row;
  align-items: flex-start;
  text-align: left;
  /* 收紧 hero 与下方"选集"的垂直间距 */
  padding-top: var(--jc-space-6);
  padding-bottom: var(--jc-space-4);
  gap: var(--jc-space-6);
}
[data-mode='tv'] .jc-detail__poster {
  /* 360 太大占位, 收到 220 */
  width: 220px;
}
[data-mode='tv'] .jc-detail__title {
  /* 用户反馈仍偏大: 2xl → xl */
  font-size: var(--jc-fs-xl);
}
[data-mode='tv'] .jc-detail__info {
  gap: var(--jc-space-3);
}
[data-mode='tv'] .jc-detail__summary,
[data-mode='tv'] .jc-detail__meta-row {
  /* base 仍偏大: 降到 sm */
  font-size: var(--jc-fs-sm);
}
/* 选集 section 自身上下 padding 收紧 (模板用 py-[space-5]) */
[data-mode='tv'] .jc-detail__section-title {
  font-size: var(--jc-fs-lg);
}

/* 平板: hero 文字同样收紧(标题已在 scoped 段降到 2xl, 这里再收简介/元信息) */
[data-mode='tablet'] .jc-detail__summary,
[data-mode='tablet'] .jc-detail__meta-row {
  font-size: var(--jc-fs-sm);
}
[data-mode='tv'] .jc-detail__episodes,
[data-mode='tv'] .jc-detail__relate {
  padding-inline: var(--jc-tv-safe);
}

/* TV(雷鸟卡片式) 分支: 安全区内缩 */
[data-mode='tv'] .jc-detail-tv__body.container-page {
  padding-inline: var(--jc-tv-safe);
}
</style>
