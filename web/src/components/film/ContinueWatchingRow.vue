<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useHistoryStore } from '@/stores'
import { recordToCard, buildPlayLink, type HistoryRecord } from '@/stores/history'
import { progressPercent, episodeLabel, formatRelativeTime } from '@/composables/useTimeBucket'
import FilmCard from '@/components/film/FilmCard.vue'

/**
 * 首页「继续观看」横滚区 —— 用户要求首页顶部展示观看历史。
 * 数据取 useHistoryStore.list(已按 timeStamp 倒序; 本地/登录云端自动切换), 取前 12 条。
 * 卡片复用 FilmCard(与首页影片行同款样式):
 *  - 海报左下角 remarks = 影片自身更新状态(HD / 更新至 N 集), 与普通影片卡完全一致;
 *  - 海报左上角角标 = "看到第 N 集"(历史进度, 与观看历史页一致, 属本行独有信息);
 *  - 海报底部进度条 = 观看进度; 标题下方副信息 = 相对时间。
 * 跳转 buildPlayLink 现拼, 保证接着"当前集 + 当前进度"续播。
 * TV(原生 APK) 上会被路由守卫拦截并派发原生播放器续播(带 currentTime)。无历史时整块不渲染。
 */
const historyStore = useHistoryStore()
const { list } = storeToRefs(historyStore)

const recent = computed(() => list.value.slice(0, 12))

function pct(currentTime?: number, duration?: number): number {
  return progressPercent(currentTime, duration)
}

/** "看到第 N 集" 角标文案(无集数信息时返回空串, 由 v-if 隐藏) */
function epLabel(rec: HistoryRecord): string {
  return episodeLabel(rec.episode, rec.episodeIndex)
}
</script>

<template>
  <section
    v-if="recent.length"
    class="jc-continue"
    aria-label="继续观看"
  >
    <header class="container-page jc-continue__header">
      <h2 class="jc-continue__title">继续观看</h2>
      <RouterLink
        to="/history"
        class="jc-continue__more"
        data-focusable="true"
        tabindex="0"
      >
        更多
        <BaseIcon name="chevron-right" size="16px" />
      </RouterLink>
    </header>

    <div class="jc-continue__viewport">
      <div class="jc-continue__scroll" data-focus-zone="rail">
        <div class="jc-continue__edge" aria-hidden="true" />
        <FilmCard
          v-for="rec in recent"
          :key="rec.id"
          class="jc-continue__item"
          :item="recordToCard(rec)"
          :to="buildPlayLink(rec)"
          :progress="pct(rec.currentTime, rec.duration)"
          :sub-text="formatRelativeTime(rec.timeStamp)"
        >
          <!-- 左上角"看到第 N 集"角标(沿用改动前的样式与位置); 左下角 remarks 由
               recordToCard 提供(item.remarks), 与首页其他影片卡同款 -->
          <template #poster-overlay>
            <span v-if="epLabel(rec)" class="jc-continue__ep">{{ epLabel(rec) }}</span>
          </template>
        </FilmCard>
        <div class="jc-continue__edge" aria-hidden="true" />
      </div>
    </div>
  </section>
</template>

<style scoped>
/* 列数 / 缩进 / 卡间距与 FilmRow 同源（theme.css 的 --jc-rail-*）,
   保证首页各横滚行卡片同宽同距 */
.jc-continue {
  display: flex;
  flex-direction: column;
}
.jc-continue__header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--jc-space-4);
  margin-bottom: var(--jc-space-3);
}
.jc-continue__title {
  font-size: var(--jc-fs-lg);
  font-weight: var(--jc-fw-bold);
  color: var(--jc-text-primary);
  line-height: var(--jc-lh-snug);
}
.jc-continue__more {
  color: var(--jc-text-link);
  font-size: var(--jc-fs-sm);
  display: inline-flex;
  align-items: center;
  gap: var(--jc-space-1);
  text-decoration: none;
  flex-shrink: 0;
}
.jc-continue__more:hover,
.jc-continue__more:focus-visible {
  outline: none;
  color: var(--jc-text-link-hover);
}

.jc-continue__scroll {
  display: flex;
  gap: var(--jc-rail-gap);
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
  /* 与 FilmRow 同理: overflow-x:auto 会把 overflow-y 强制算成 auto,
     卡片 hover scale(1.04) 上下溢出部分会被纵向裁切(顶部被截断)。
     加 padding-block 让放大溢出的上下部分落在 padding 区(不被裁)。 */
  padding-block: 12px;
}
.jc-continue__scroll::-webkit-scrollbar {
  display: none;
}

.jc-continue__edge {
  flex-shrink: 0;
  /* 首尾缩进（web 按页面 gutter; TV 用安全区, 均由变量给出） */
  width: var(--jc-rail-edge);
}

.jc-continue__item {
  flex-shrink: 0;
  scroll-snap-align: start;
  /* 与 FilmRow 同一公式（必须逐字一致, 否则首页各横滚行卡片不同宽）:
     (100% - 1×edge - 可见卡间 gap 道数 × 卡间距) / 列数
     "只扣 1 个 edge"的原因见 FilmRow 内注释。 */
  width: calc(
    (100% - var(--jc-rail-edge) - var(--jc-rail-gaps) * var(--jc-rail-gap)) /
      var(--jc-rail-cols)
  );
}

/* "看到第 N 集" 角标: 海报左上角(沿用改动前的视觉: 品牌渐变 + 圆角 + 白字) */
.jc-continue__ep {
  position: absolute;
  top: var(--jc-space-2);
  left: var(--jc-space-2);
  z-index: 2;
  max-width: calc(100% - var(--jc-space-2) * 2);
  padding: 2px 8px;
  border-radius: var(--jc-radius-sm);
  background-image: var(--jc-brand-gradient);
  color: #fff;
  font-size: var(--jc-fs-xs);
  font-weight: var(--jc-fw-semibold);
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>

<style>
/* TV 模式: 与 FilmRow 同款(容器 100vw - 2×安全区, 每行 6 张完整卡片, 间距 +50%);
   焦点行为直接继承 FilmCard 的 TV 焦点(整卡 outline + 放大), 不再单写。 */
/* TV 的列数(6) / 卡间距(space-6) / 缩进(安全区) 由 theme.css 的 [data-mode="tv"] 统一覆盖,
   此处不再重复定义宽度规则（避免与 FilmRow 两处公式不同步）。 */
[data-mode='tv'] .jc-continue__scroll {
  padding-block: 16px;
}
[data-mode='tv'] .jc-continue__header.container-page {
  padding-inline: var(--jc-tv-safe);
}
[data-mode='tv'] .jc-continue__title {
  font-size: var(--jc-fs-xl);
}
</style>
