<script setup lang="ts">
import BaseImage from '@/components/base/BaseImage.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import TvSectionHeader from '@/components/base/TvSectionHeader.vue'
import EpisodeTabs from '@/components/film/EpisodeTabs.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import type { Card, FilmDetail } from '@/types/film'

/**
 * 影片详情页 TV(雷鸟卡片式)渲染组件 — 方案 B 拆分(自 FilmDetailView.vue 下沉)。
 *
 * 职责边界: 本组件只做「TV 渲染」, 数据与业务逻辑(详情拉取 / 续播记录 / 收藏 / 选源 / 跳播放 /
 * 原生播放派发)全在宿主 FilmDetailView.vue, 经 props + 回调单向传入; 这里不复制任何业务逻辑,
 * 也不另起接口。宿主以 `v-else-if="isTV"` 挂载本组件 ⇒ 仅 TV 模式渲染, 组件内无需再判 isTV。
 *
 * heroBgStyle 为两个分支共用(壳内计算一次, 经 prop 传入);
 * 模板 / 类名 / data-focusable / tabindex 与原 TV 分支(:400-544)逐块等价。
 * 样式来源: 原壳内 .jc-detail-tv__ 段(scoped) + 原非 scoped 块的 [data-mode='tv'] 覆盖,
 * 一并随迁并改为 scoped; [data-mode='tablet'] 的收紧规则面向桌面分支, 留在壳内不动。
 */
interface ResumeRecord {
  source: string
  episodeIndex: number
  episodeName: string
  currentTime: number
}
interface ReviewLink {
  name: string
  icon: string
  url: string
}

interface Props {
  /** 影片详情(宿主 detail; 挂载分支已排除 null) */
  detail: FilmDetail
  /** 大背景样式(宿主 heroBgStyle: backdrop 优先, 经 resolveBlobUrl 解析; 无图 undefined) */
  heroBgStyle: { backgroundImage: string } | undefined
  /** 评分展示串(宿主 score: '' 或一位小数) */
  score: string
  /** 年份·地区·类型 chip(宿主 tagChips) */
  tagChips: string[]
  /** 导演(宿主 directors, 已截断) */
  directors: string[]
  /** 热度榜位徽标文案(宿主 hotBadge) */
  hotBadge: string
  /** 影评外链(宿主 reviewLinks) */
  reviewLinks: ReviewLink[]
  /** 简介(宿主 cleanedContent, 已清洗) */
  cleanedContent: string
  /** 续播记录(宿主 resumeRecord; 含 source/episodeIndex/episodeName/currentTime) */
  resumeRecord: ResumeRecord | null
  /** 相关推荐(宿主 visibleRelate, 已按行数截断) */
  visibleRelate: Card[]
  /** 收藏态(宿主 isFavorited) */
  isFavorited: boolean
  /** 当前选中源(宿主 activeSourceId, 选集 tab 激活态) */
  activeSourceId: string
  /** 继续观看(宿主 resumeWatching) */
  onResumeWatching: () => void
  /** 从头/立即播放(宿主 playFirst) */
  onPlayFirst: () => void
  /** 收藏切换(宿主 handleToggleFavorite) */
  onToggleFavorite: () => void
  /** 切换线路(宿主 onSelectSource) */
  onSelectSource: (sid: string) => void
  /** 选集跳播放(宿主 gotoPlay, 内含原生播放派发) */
  onGotoPlay: (sourceId: string, episodeIndex: number) => void
}
defineProps<Props>()
</script>

<template>
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

          <!-- 影评外链(TV 同款): 站点图标圆角方形, 悬停显示站名 -->
          <div v-if="reviewLinks.length" class="jc-detail-tv__reviews">
            <span class="jc-detail-tv__reviews-label">影评</span>
            <a
              v-for="r in reviewLinks"
              :key="r.name"
              :href="r.url"
              :title="`${r.name} · ${detail.name}`"
              target="_blank"
              rel="noopener noreferrer"
              class="jc-detail-tv__review"
              data-focusable="true"
              tabindex="0"
            >
              <img :src="r.icon" :alt="r.name" loading="lazy" />
            </a>
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
                  @click="onResumeWatching"
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
                  @click="onPlayFirst"
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
                  @click="onToggleFavorite"
            >
              <BaseIcon name="heart" size="1.1em" />
              {{ isFavorited ? '已收藏' : '收藏' }}
            </button>
          </div>
        </div>
      </div>

      <!-- ===== 选集 (复用现有 EpisodeTabs 组件)
           playing-source-id: "正在播放的源"已知则传(=续播记录的源, 方案 §C6 #3) —
           浏览态切走该源时, tab 上出 ▶ 小标提示播放所在源; 浏览到该源时其本身即激活态, 不重复出标 -->
      <section v-if="detail.sources?.length" class="jc-detail-tv__section" aria-label="选集">
        <TvSectionHeader title="选集" :sub="`共 ${detail.sources[0]?.episodes?.length ?? 0} 集 · ${detail.sources.length} 条线路`" />
        <EpisodeTabs
          :sources="detail.sources"
          :current-source-id="activeSourceId"
          :playing-source-id="resumeRecord?.source ?? ''"
          :current-episode="resumeRecord ? (detail.sources.find(s => s.id === resumeRecord!.source)?.episodes?.[resumeRecord!.episodeIndex]?.link ?? '') : ''"
          :watched-links="[]"
          :page-size="30"
          :film-name="detail.name"
          @change-source="onSelectSource"
              @select="(p) => onGotoPlay(p.sourceId, p.episodeIndex)"
        />
      </section>

      <!-- ===== 相关推荐 (jc-tv-grid 6 列 FilmCard) ===== -->
      <section v-if="visibleRelate.length" class="jc-detail-tv__section" aria-label="相关推荐">
        <TvSectionHeader title="相关推荐" />
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

<!-- ============================================================
 * TV 详情页样式 (方案 B 拆分注记): 原 FilmDetailView.vue 两处整块迁入 ——
 *   ① scoped 块里的 .jc-detail-tv__ 段(原 :1177-1380)
 *   ② 非 scoped 全局块里的 [data-mode='tv'] 覆盖(原 :1384-1414 与 :1421-1429)
 * 样式门控: 本组件仅 TV 模式挂载(宿主以 v-else-if="isTV" 渲染), 故统一改为 scoped;
 * 选择器保留原有 [data-mode='tv'] 前缀与书写顺序, 维持迁移前的特异性与层叠口径。
 * 全局词汇类(jc-tv-chip/jc-tv-btn/jc-tv-grid/jc-tv-eps/jc-tv-ep)来自 tv-cards.css, 未动。
 * 平板态的两条 [data-mode='tablet'] 收紧规则面向桌面分支, 留在壳内。
 * ============================================================ -->
<style scoped>
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

/* TV 影评外链行: 与 meta 同风格, 图标稍大适配 TV 焦点 */
.jc-detail-tv__reviews {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
}

.jc-detail-tv__reviews-label {
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-sm);
  white-space: nowrap;
}

.jc-detail-tv__review {
  display: inline-flex;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  overflow: hidden;
  background-color: rgba(0, 0, 0, 0.55);
  border: 1px solid rgba(255, 255, 255, 0.14);
  opacity: 0.85;
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.jc-detail-tv__review:hover,
.jc-detail-tv__review:focus-visible {
  opacity: 1;
  transform: scale(1.1);
  outline: none;
}

.jc-detail-tv__review img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
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
[data-mode='tv'] .jc-detail__episodes,
[data-mode='tv'] .jc-detail__relate {
  padding-inline: var(--jc-tv-safe);
}

/* TV(雷鸟卡片式) 分支: 安全区内缩 */
[data-mode='tv'] .jc-detail-tv__body.container-page {
  padding-inline: var(--jc-tv-safe);
}
</style>
