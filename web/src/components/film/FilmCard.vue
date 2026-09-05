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
}

const props = withDefaults(defineProps<Props>(), {
  showTitleBelow: true,
  score: '',
  lazy: true,
  ratio: '3/4'
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

const linkTo = computed(() => ({
  path: '/filmDetail',
  query: { link: String(props.item.mid) }
}))

/** 角标 remarks：更新到第几集这种关键信息（其它如年份/分类太冗，移到 hover 浮层与详情页） */
const remarks = computed(() => props.item.remarks || '')

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
    class="gf-film-card block group"
    data-focusable="true"
    tabindex="0"
    :aria-label="item.name"
  >
    <div class="gf-film-card__poster relative overflow-hidden shadow-card">
      <BaseImage
        :src="item.cover"
        :alt="item.name"
        :ratio="cardRatio"
        :eager="!lazy"
        fit="cover"
      />

      <!-- 右上角评分: 黄色星星 + 数字, 带底色阴影避免被封面同化 -->
      <span v-if="scoreText" class="gf-film-card__score-badge" aria-label="评分">
        <svg viewBox="0 0 24 24" fill="currentColor" width="12" height="12" aria-hidden="true">
          <path d="M12 .587l3.668 7.568L24 9.75l-6 5.852L19.336 24 12 19.897 4.664 24 6 15.602 0 9.75l8.332-1.595z"/>
        </svg>
        {{ scoreText }}
      </span>

      <!-- 卡片下部剧集信息 (remarks: 更新至 N 集 / HD / 独播 等), 常驻在封面底部 -->
      <div v-if="remarks" class="gf-film-card__epinfo">
        {{ remarks }}
      </div>

      <!-- 蒙版 (hover/focus 加深) -->
      <div class="gf-film-card__mask absolute inset-0 pointer-events-none" />

      <!-- PC hover 播放图标 (中央) -->
      <div class="gf-film-card__play absolute inset-0 flex items-center justify-center pointer-events-none z-2" aria-hidden="true">
        <span class="gf-film-card__play-btn">
          <svg viewBox="0 0 24 24" fill="currentColor" width="22" height="22"><path d="M8 5v14l11-7z"/></svg>
        </span>
      </div>
    </div>

    <!-- 卡片下方信息区: 标题 + 副信息 (年份·分类·⭐评分), 常驻可见 (bilibili/腾讯视频风格) -->
    <div v-if="showTitleBelow" class="gf-film-card__below">
      <h4 class="gf-film-card__title-below">
        {{ item.name }}
      </h4>
      <!-- 评分已移到封面右上角星标, 此处只留 年份·地区·分类, 不重复评分 -->
      <div v-if="subTextBelow" class="gf-film-card__sub-below">
        <span class="gf-film-card__sub-meta">{{ subTextBelow }}</span>
      </div>
    </div>
  </RouterLink>
</template>

<style scoped>
.gf-film-card {
  text-decoration: none;
  outline: none;
  border-radius: var(--gf-card-radius);
  transition:
    transform var(--gf-dur-base) var(--gf-ease-spring),
    box-shadow var(--gf-dur-base) var(--gf-ease-standard);
}

.gf-film-card__poster {
  background-color: var(--gf-bg-elevated);
  border-radius: var(--gf-card-radius);
  transition:
    transform var(--gf-dur-base) var(--gf-ease-spring),
    box-shadow var(--gf-dur-base) var(--gf-ease-standard);
}

.gf-film-card__mask {
  background-image: linear-gradient(
    to top,
    var(--gf-hover-overlay) 0%,
    rgba(0, 0, 0, 0.35) 45%,
    rgba(0, 0, 0, 0) 70%
  );
  opacity: 0;
  transition: opacity var(--gf-dur-base) var(--gf-ease-standard);
}

.gf-film-card__hover-info {
  opacity: 0;
  transform: translateY(12px);
  transition:
    opacity var(--gf-dur-base) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-standard);
}

.gf-film-card__meta {
  color: rgba(255, 255, 255, 0.78);
}

/* 中央播放图标 (hover 才显示) */
.gf-film-card__play {
  opacity: 0;
  transform: scale(0.85);
  transition:
    opacity var(--gf-dur-base) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-spring);
}
.gf-film-card__play-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 9999px;
  background-image: var(--gf-brand-gradient);
  color: #fff;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
}

/* 桌面 hover: 卡片轻微缩放 + 蒙版/简介浮层上滑 + 中央播放按钮浮出 */
@media (hover: hover) and (pointer: fine) {
  .gf-film-card:hover,
  .gf-film-card:focus-visible {
    position: relative;
    z-index: 3;
  }
  .gf-film-card:hover .gf-film-card__poster,
  .gf-film-card:focus-visible .gf-film-card__poster {
    transform: scale(1.04);
    box-shadow: var(--gf-shadow-hover);
  }
  .gf-film-card:hover .gf-film-card__mask,
  .gf-film-card:focus-visible .gf-film-card__mask {
    opacity: 1;
  }
  .gf-film-card:hover .gf-film-card__hover-info,
  .gf-film-card:focus-visible .gf-film-card__hover-info {
    opacity: 1;
    transform: translateY(0);
  }
  .gf-film-card:hover .gf-film-card__play,
  .gf-film-card:focus-visible .gf-film-card__play {
    opacity: 1;
    transform: scale(1);
  }
  .gf-film-card:hover .gf-film-card__title-below,
  .gf-film-card:focus-visible .gf-film-card__title-below {
    color: var(--gf-text-primary);
  }
}

/* 移动端按下反馈 */
@media (hover: none) {
  .gf-film-card:active .gf-film-card__poster {
    transform: scale(0.97);
  }
}

/* 焦点态强化 */
.gf-film-card:focus-visible {
  outline: none;
}
.gf-film-card:focus-visible .gf-film-card__poster {
  box-shadow: var(--gf-shadow-focus-ring), var(--gf-shadow-hover);
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 标题下方区域: 双行结构 (bilibili / 腾讯视频风格) */
.gf-film-card__below {
  margin-top: var(--gf-space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.gf-film-card__title-below {
  /* 默认两行截断 */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  /* h4 默认 bold → 改中等字重(用户要求卡片名不加粗) */
  font-weight: var(--gf-fw-medium);
  min-height: calc(var(--gf-fs-sm) * var(--gf-lh-snug, 1.3) * 2);
}
.gf-film-card__sub-below {
  display: flex;
  align-items: center;
  gap: var(--gf-space-2);
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  line-height: 1.4;
}
.gf-film-card__sub-meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

/* 右上角评分徽标: 黄色星星 + 数字, 半透明黑底 + 阴影(防被封面同化) */
.gf-film-card__score-badge {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 20px;
  padding: 0 6px;
  border-radius: var(--gf-radius-sm);
  background-color: rgba(0, 0, 0, 0.72);
  color: #ffc107; /* 黄色星 + 数字 */
  font-size: 12px;
  font-weight: var(--gf-fw-bold);
  line-height: 1;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.55);
  pointer-events: none;
  white-space: nowrap;
}

/* 卡片下部剧集信息条 (remarks: 更新至 N 集 等) */
.gf-film-card__epinfo {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 12px var(--gf-space-2) 6px;
  background-image: linear-gradient(
    to top,
    rgba(0, 0, 0, 0.85) 0%,
    rgba(0, 0, 0, 0.45) 60%,
    rgba(0, 0, 0, 0) 100%
  );
  color: #fff;
  font-size: 12px;
  font-weight: var(--gf-fw-medium);
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  z-index: 2;
  pointer-events: none;
}
@media (min-width: 1024px) {
  .gf-film-card__score-badge { height: 22px; font-size: 13px; }
  .gf-film-card__epinfo { font-size: 13px; }
}
</style>

<style>
[data-mode='tv'] .gf-film-card__mask {
  opacity: 0;
}
/* TV 焦点态：聚焦区域包含整张卡片(封面+下方文字), 焦点环 + 轻微放大.
 * 用 outline(随 border-radius, 不被祖先 overflow 裁切) 而非纯 box-shadow,
 * 解决"卡片聚焦框顶部被截断". 整卡放大用 transform 在卡片根. */
[data-mode='tv'] .gf-film-card:focus,
[data-mode='tv'] .gf-film-card:focus-visible {
  outline: 3px solid var(--gf-brand-cyan);
  outline-offset: 2px;
  border-radius: var(--gf-card-radius);
  transform: scale(1.05);
  z-index: 5;
  box-shadow: 0 0 18px 2px rgba(74, 209, 229, 0.4);
}
[data-mode='tv'] .gf-film-card:focus .gf-film-card__poster,
[data-mode='tv'] .gf-film-card:focus-visible .gf-film-card__poster {
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.7);
}
[data-mode='tv'] .gf-film-card:focus .gf-film-card__title-below,
[data-mode='tv'] .gf-film-card:focus-visible .gf-film-card__title-below {
  color: var(--gf-text-primary);
}
/* TV 卡片标题字号（不靠 hover 显示）— 调小一档(base→sm), 与历史/收藏 gf-tv-card .name 一致 */
[data-mode='tv'] .gf-film-card__title-below {
  font-size: var(--gf-fs-sm);
}
[data-mode='tv'] .gf-film-card__epinfo {
  font-size: 14px;
}
[data-mode='tv'] .gf-film-card__score-badge { height: 26px; font-size: 15px; padding: 0 8px; }
</style>
