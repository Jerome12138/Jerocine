<script setup lang="ts">
import { computed } from 'vue'
import type { Card } from '@/types/film'
import BaseImage from '@/components/base/BaseImage.vue'

interface Props {
  item: Card
  /** 是否显示卡片下方的标题（移动 / 平板 / TV 默认 true，桌面默认 false 由父级决定） */
  showTitleBelow?: boolean
  /** 评分（可选，1-10） */
  score?: number | string
  /** 是否懒加载图片 */
  lazy?: boolean
  /** 封面比例, 默认 3:4 (影视行业标准); 历史调用方可改 "2/3" 等 */
  ratio?: string
  /** 覆盖默认跳转(/filmDetail?link=mid) — 历史卡直达播放页续播等场景; string 直接交给 RouterLink */
  to?: string | { path: string; query?: Record<string, string | number> }
  /** 观看进度 0-100, >0 时海报底部显示 3px 进度条(继续观看/历史卡) */
  progress?: number
  /** 覆盖标题下方副信息(默认 年份·地区·分类) — 历史卡显示相对时间 */
  subText?: string
}

const props = withDefaults(defineProps<Props>(), {
  showTitleBelow: true,
  score: '',
  lazy: true,
  ratio: '3/4',
  progress: 0,
  subText: ''
})

const cardRatio = computed(() => props.ratio)

/** hover 浮层副信息: 年份 · 地区 · 分类, 缺字段则跳过 */
const metaText = computed(() => {
  const parts: string[] = []
  if (props.item.year) parts.push(String(props.item.year))
  if (props.item.area) parts.push(String(props.item.area))
  if (props.item.cName) parts.push(String(props.item.cName))
  return parts.join(' · ')
})

/** 标题下方常驻副信息: 年份 · 地区 · 分类 (TV 字号大空间够, 不再省略 area); 评分另算 */
const subTextBelow = computed(() => {
  const parts: string[] = []
  if (props.item.year) parts.push(String(props.item.year))
  if (props.item.area) parts.push(String(props.item.area))
  if (props.item.cName) parts.push(String(props.item.cName))
  return parts.join(' · ')
})

const linkTo = computed(() => props.to ?? {
  path: '/filmDetail',
  query: { link: String(props.item.mid) }
})

/** 副信息: 显式 subText 优先(历史卡时间等), 否则 年份·地区·分类 */
const subBelow = computed(() => props.subText || subTextBelow.value)

/** 角标 remarks：更新到第几集这种关键信息（其它如年份/分类太冗，移到 hover 浮层与详情页） */
const remarks = computed(() => props.item.remarks || '')

/** 热度榜位角标: 后端榜单刷新任务标过 hotRank(1 起)才显示, 其余卡片不占位 */
const hotRankText = computed(() => {
  const r = props.item.hotRank ?? 0
  return r > 0 ? `Hot ${r}` : ''
})

/**
 * 评分显示策略：
 *  1. 父组件显式传 score 优先
 *  2. 否则尝试 item.dbScore / item.score（后端列表接口通常不返回，但 mock / 部分聚合接口会带）
 *  3. 评分需要 ≥ 1 才显示，过滤掉 0 / NaN / "暂无"
 */
const scoreText = computed(() => {
  const raw =
    props.score !== '' && props.score !== undefined && props.score !== null
      ? props.score
      : props.item.dbScore ?? ''
  if (raw === '' || raw === undefined || raw === null) return ''
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 1) return ''
  // 1-10 区间保留 1 位小数（已是整数则不加）
  return n % 1 === 0 ? n.toFixed(0) : n.toFixed(1)
})
</script>

<template>
  <RouterLink
    :to="linkTo"
    class="jc-film-card block group"
    data-focusable="true"
    tabindex="0"
    :aria-label="item.name"
  >
    <div class="jc-film-card__poster relative overflow-hidden shadow-card">
      <BaseImage
        :src="item.cover"
        :alt="item.name"
        :ratio="cardRatio"
        :eager="!lazy"
        fit="cover"
      />

      <!-- 右上角评分: 黄色星星 + 数字, 带底色阴影避免被封面同化 -->
      <span v-if="scoreText" class="jc-film-card__score-badge" aria-label="评分">
        <svg viewBox="0 0 24 24" fill="currentColor" width="1em" height="1em" aria-hidden="true">
          <path d="M12 .587l3.668 7.568L24 9.75l-6 5.852L19.336 24 12 19.897 4.664 24 6 15.602 0 9.75l8.332-1.595z"/>
        </svg>
        {{ scoreText }}
      </span>

      <!-- 左上角热度榜位: 火 + Hot N(仅榜单刷新任务标过 hot_rank 的片有) -->
      <span v-if="hotRankText" class="jc-film-card__hot-badge" aria-label="热度榜位">
        <svg viewBox="0 0 24 24" fill="currentColor" width="1em" height="1em" aria-hidden="true">
          <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z"/>
        </svg>
        {{ hotRankText }}
      </span>

      <!-- 卡片下部剧集信息 (remarks: 更新至 N 集 / HD / 独播 等), 常驻在封面底部 -->
      <div v-if="remarks" class="jc-film-card__epinfo">
        {{ remarks }}
      </div>

      <!-- 蒙版 (hover/focus 加深) -->
      <div class="jc-film-card__mask absolute inset-0 pointer-events-none" />

      <!-- 观看进度条 (继续观看/历史卡): 底部 3px, 盖在 remarks 渐变条之上 -->
      <div
        v-if="props.progress > 0"
        class="jc-film-card__progress"
        :aria-label="`已观看 ${props.progress}%`"
      >
        <span :style="{ width: props.progress + '%' }" />
      </div>

      <!-- 调用方自定义海报覆盖层(删除按钮 / 进度时间等) -->
      <slot name="poster-overlay" />

      <!-- PC hover 播放图标 (中央) -->
      <div class="jc-film-card__play absolute inset-0 flex items-center justify-center pointer-events-none z-2" aria-hidden="true">
        <span class="jc-film-card__play-btn">
          <svg viewBox="0 0 24 24" fill="currentColor" width="22" height="22"><path d="M8 5v14l11-7z"/></svg>
        </span>
      </div>
    </div>

    <!-- 卡片下方信息区: 标题 + 副信息 (年份·分类·⭐评分), 常驻可见 (bilibili/腾讯视频风格) -->
    <div v-if="showTitleBelow" class="jc-film-card__below">
      <h4 class="jc-film-card__title-below">
        {{ item.name }}
      </h4>
      <!-- 评分已移到封面右上角星标, 此处只留 副信息(subText 优先, 否则 年份·地区·分类), 不重复评分 -->
      <div v-if="subBelow" class="jc-film-card__sub-below">
        <span class="jc-film-card__sub-meta">{{ subBelow }}</span>
      </div>
    </div>
  </RouterLink>
</template>

<style scoped>
.jc-film-card {
  text-decoration: none;
  outline: none;
  border-radius: var(--jc-card-radius);
  transition:
    transform var(--jc-dur-base) var(--jc-ease-spring),
    box-shadow var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-film-card__poster {
  background-color: var(--jc-bg-elevated);
  border-radius: var(--jc-card-radius);
  transition:
    transform var(--jc-dur-base) var(--jc-ease-spring),
    box-shadow var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-film-card__mask {
  background-image: linear-gradient(
    to top,
    var(--jc-hover-overlay) 0%,
    rgba(0, 0, 0, 0.35) 45%,
    rgba(0, 0, 0, 0) 70%
  );
  opacity: 0;
  transition: opacity var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-film-card__hover-info {
  opacity: 0;
  transform: translateY(12px);
  transition:
    opacity var(--jc-dur-base) var(--jc-ease-standard),
    transform var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-film-card__meta {
  color: rgba(255, 255, 255, 0.78);
}

/* 中央播放图标 (hover 才显示) */
.jc-film-card__play {
  opacity: 0;
  transform: scale(0.85);
  transition:
    opacity var(--jc-dur-base) var(--jc-ease-standard),
    transform var(--jc-dur-base) var(--jc-ease-spring);
}
.jc-film-card__play-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 9999px;
  background-image: var(--jc-brand-gradient);
  color: #fff;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
}

/* 桌面 hover: 卡片轻微缩放 + 蒙版/简介浮层上滑 + 中央播放按钮浮出 */
@media (hover: hover) and (pointer: fine) {
  .jc-film-card:hover,
  .jc-film-card:focus-visible {
    position: relative;
    z-index: 3;
  }
  .jc-film-card:hover .jc-film-card__poster,
  .jc-film-card:focus-visible .jc-film-card__poster {
    transform: scale(1.04);
    box-shadow: var(--jc-shadow-hover);
  }
  .jc-film-card:hover .jc-film-card__mask,
  .jc-film-card:focus-visible .jc-film-card__mask {
    opacity: 1;
  }
  .jc-film-card:hover .jc-film-card__hover-info,
  .jc-film-card:focus-visible .jc-film-card__hover-info {
    opacity: 1;
    transform: translateY(0);
  }
  .jc-film-card:hover .jc-film-card__play,
  .jc-film-card:focus-visible .jc-film-card__play {
    opacity: 1;
    transform: scale(1);
  }
  .jc-film-card:hover .jc-film-card__title-below,
  .jc-film-card:focus-visible .jc-film-card__title-below {
    color: var(--jc-text-primary);
  }
}

/* 移动端按下反馈 */
@media (hover: none) {
  .jc-film-card:active .jc-film-card__poster {
    transform: scale(0.97);
  }
}

/* 焦点态强化 */
.jc-film-card:focus-visible {
  outline: none;
}
.jc-film-card:focus-visible .jc-film-card__poster {
  box-shadow: var(--jc-shadow-focus-ring), var(--jc-shadow-hover);
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 标题下方区域: 双行结构 (bilibili / 腾讯视频风格) */
.jc-film-card__below {
  margin-top: var(--jc-space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.jc-film-card__title-below {
  /* 默认两行截断 */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  /* h4 默认 bold → 改中等字重(用户要求卡片名不加粗) */
  font-weight: var(--jc-fw-medium);
  min-height: calc(var(--jc-fs-sm) * var(--jc-lh-snug, 1.3) * 2);
}
.jc-film-card__sub-below {
  display: flex;
  align-items: center;
  gap: var(--jc-space-2);
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  line-height: 1.4;
}
.jc-film-card__sub-meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

/* 右上角评分徽标: 黄色星星 + 数字, 半透明黑底 + 阴影(防被封面同化) */
.jc-film-card__score-badge {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: auto; /* 由内容 + padding 撑起, 不再固定高度 */
  padding: 1px 5px;
  border-radius: var(--jc-radius-sm);
  background-color: rgba(0, 0, 0, 0.72);
  color: #ffc107; /* 黄色星 + 数字 */
  font-size: var(--jc-fs-badge);
  font-weight: var(--jc-fw-semibold);
  line-height: 1.35;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.55);
  pointer-events: none;
  white-space: nowrap;
}

/* 左上角热度榜位角标: 品牌渐变底 + 白字(与右上角评分黄星徽标左右呼应) */
.jc-film-card__hot-badge {
  position: absolute;
  top: 6px;
  left: 6px;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: auto;
  padding: 1px 5px;
  border-radius: var(--jc-radius-sm);
  background-image: var(--jc-brand-gradient);
  color: #fff;
  font-size: var(--jc-fs-badge);
  font-weight: var(--jc-fw-semibold);
  line-height: 1.35;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.55);
  pointer-events: none;
  white-space: nowrap;
}

/* 窄卡片(移动端): 左右两个角标更易撞在一起, 再收一档内边距 */
@media (max-width: 767px) {
  .jc-film-card__score-badge,
  .jc-film-card__hot-badge {
    padding: 1px 4px;
    gap: 2px;
  }
}

/* 观看进度条 (继续观看/历史卡): 底部 3px, 高于 remarks 渐变条(z-2) */
.jc-film-card__progress {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background-color: var(--jc-progress-bg);
  z-index: 3;
  overflow: hidden;
}
.jc-film-card__progress > span {
  display: block;
  height: 100%;
  background-image: var(--jc-progress-fg);
  transition: width var(--jc-dur-base) var(--jc-ease-standard);
}

/* 卡片下部剧集信息条 (remarks: 更新至 N 集 等) */
.jc-film-card__epinfo {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 12px var(--jc-space-2) 6px;
  background-image: linear-gradient(
    to top,
    rgba(0, 0, 0, 0.85) 0%,
    rgba(0, 0, 0, 0.45) 60%,
    rgba(0, 0, 0, 0) 100%
  );
  color: #fff;
  /* 字号 = 卡片副信息档(--jc-fs-xs), 与"看到第 N 集"角标同规格; 不再按屏宽额外放大一档 */
  font-size: var(--jc-fs-xs);
  font-weight: var(--jc-fw-medium);
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  z-index: 2;
  pointer-events: none;
}
</style>

<style>
[data-mode='tv'] .jc-film-card__mask {
  opacity: 0;
}
/* TV 焦点态：聚焦区域包含整张卡片(封面+下方文字), 焦点环 + 轻微放大.
 * 用 outline(随 border-radius, 不被祖先 overflow 裁切) 而非纯 box-shadow,
 * 解决"卡片聚焦框顶部被截断". 整卡放大用 transform 在卡片根. */
[data-mode='tv'] .jc-film-card:focus,
[data-mode='tv'] .jc-film-card:focus-visible {
  outline: 3px solid var(--jc-brand-cyan);
  outline-offset: 2px;
  border-radius: var(--jc-card-radius);
  transform: scale(1.05);
  z-index: 5;
  box-shadow: 0 0 18px 2px rgba(74, 209, 229, 0.4);
}
[data-mode='tv'] .jc-film-card:focus .jc-film-card__poster,
[data-mode='tv'] .jc-film-card:focus-visible .jc-film-card__poster {
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.7);
}
[data-mode='tv'] .jc-film-card:focus .jc-film-card__title-below,
[data-mode='tv'] .jc-film-card:focus-visible .jc-film-card__title-below {
  color: var(--jc-text-primary);
}
/* TV 卡片标题字号（不靠 hover 显示）— 调小一档(base→sm), 与历史/收藏 jc-tv-card .name 一致 */
[data-mode='tv'] .jc-film-card__title-below {
  font-size: var(--jc-fs-sm);
}
[data-mode='tv'] .jc-film-card__epinfo {
  font-size: var(--jc-fs-xs);
}
/* TV 角标: badge 字号(TV 档 11px) + 左右内边距 4px(上下 1px) —— 比上一版的 2px 舒展,
   肉眼上不再"贴着字边的紧箍"; 同时仍小于桌面版的 5px, 窄卡片上左右两个角标不会撞. */
[data-mode='tv'] .jc-film-card__score-badge,
[data-mode='tv'] .jc-film-card__hot-badge {
  padding: 1px 4px;
  gap: 3px;
}
</style>
