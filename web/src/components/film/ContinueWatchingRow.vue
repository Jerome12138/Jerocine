<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useHistoryStore } from '@/stores'
import { buildPlayLink } from '@/stores/history'
import { progressPercent, episodeLabel } from '@/composables/useTimeBucket'
import BaseImage from '@/components/base/BaseImage.vue'

/**
 * 首页「继续观看」横滚区 —— 用户要求首页顶部展示观看历史。
 * 数据取 useHistoryStore.list(已按 timeStamp 倒序; 本地/登录云端自动切换), 取前 12 条。
 * 每张卡链到 record.link(`/play?...`): 浏览器直接进播放页; TV(原生 APK) 上会被路由守卫
 * 拦截并派发原生播放器续播(带 currentTime)。无历史时整块不渲染(v-if)。
 */
const historyStore = useHistoryStore()
const { list } = storeToRefs(historyStore)

const recent = computed(() => list.value.slice(0, 12))

function pct(currentTime?: number, duration?: number): number {
  return progressPercent(currentTime, duration)
}
</script>

<template>
  <section
    v-if="recent.length"
    class="gf-continue"
    aria-label="继续观看"
  >
    <header class="container-page gf-continue__header">
      <h2 class="gf-continue__title">继续观看</h2>
      <RouterLink
        to="/history"
        class="gf-continue__more"
        data-focusable="true"
        tabindex="0"
      >
        更多
        <BaseIcon name="chevron-right" size="16px" />
      </RouterLink>
    </header>

    <div class="gf-continue__viewport">
      <div class="gf-continue__scroll" data-focus-zone="rail">
        <div class="gf-continue__edge" aria-hidden="true" />
        <RouterLink
          v-for="rec in recent"
          :key="rec.id"
          :to="buildPlayLink(rec)"
          class="gf-continue__item"
          data-focusable="true"
          tabindex="0"
          :aria-label="`继续观看 ${rec.name}`"
        >
          <div class="gf-continue__poster">
            <BaseImage
              :src="rec.picture || ''"
              :alt="rec.name"
              ratio="3/4"
              fit="cover"
            />
            <span
              v-if="episodeLabel(rec.episode, rec.episodeIndex)"
              class="gf-continue__ep"
            >{{ episodeLabel(rec.episode, rec.episodeIndex) }}</span>
            <div class="gf-continue__play" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="currentColor" width="22" height="22"><path d="M8 5v14l11-7z" /></svg>
            </div>
            <div
              v-if="pct(rec.currentTime, rec.duration) > 0"
              class="gf-continue__bar"
              :aria-label="`已观看 ${pct(rec.currentTime, rec.duration)}%`"
            >
              <span
                class="gf-continue__bar-fill"
                :style="{ width: pct(rec.currentTime, rec.duration) + '%' }"
              />
            </div>
          </div>
          <h3 class="gf-continue__name">{{ rec.name }}</h3>
          <p
            v-if="episodeLabel(rec.episode, rec.episodeIndex)"
            class="gf-continue__sub"
          >
            看到 {{ episodeLabel(rec.episode, rec.episodeIndex) }}
          </p>
        </RouterLink>
        <div class="gf-continue__edge" aria-hidden="true" />
      </div>
    </div>
  </section>
</template>

<style scoped>
.gf-continue {
  display: flex;
  flex-direction: column;
}
.gf-continue__header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--gf-space-4);
  margin-bottom: var(--gf-space-3);
}
.gf-continue__title {
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  line-height: var(--gf-lh-snug);
}
.gf-continue__more {
  color: var(--gf-text-link);
  font-size: var(--gf-fs-sm);
  display: inline-flex;
  align-items: center;
  gap: var(--gf-space-1);
  text-decoration: none;
  flex-shrink: 0;
}
.gf-continue__more:hover,
.gf-continue__more:focus-visible {
  outline: none;
  color: var(--gf-text-link-hover);
}

.gf-continue__scroll {
  display: flex;
  gap: var(--gf-space-3);
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
  /* 与 FilmRow 同理: overflow-x:auto 会把 overflow-y 强制算成 auto,
     卡片 hover scale(1.04) 上下溢出部分会被纵向裁切(顶部被截断)。
     加 padding-block 让放大溢出的上下部分落在 padding 区(不被裁)。 */
  padding-block: 12px;
}
.gf-continue__scroll::-webkit-scrollbar {
  display: none;
}
@media (min-width: 768px) {
  .gf-continue__scroll {
    gap: var(--gf-space-4);
  }
}

.gf-continue__edge {
  flex-shrink: 0;
  width: var(--gf-gutter-mobile);
}
@media (min-width: 768px) {
  .gf-continue__edge {
    width: var(--gf-gutter-tablet);
  }
}
@media (min-width: 1024px) {
  .gf-continue__edge {
    width: var(--gf-gutter-desktop);
  }
}

.gf-continue__item {
  flex-shrink: 0;
  scroll-snap-align: start;
  text-decoration: none;
  outline: none;
  width: calc((100vw - 32px) / 3.2);
  border-radius: var(--gf-card-radius);
  transition: transform var(--gf-dur-base) var(--gf-ease-spring);
}
@media (min-width: 480px) {
  .gf-continue__item {
    width: calc((100vw - 32px) / 4);
  }
}
@media (min-width: 768px) {
  .gf-continue__item {
    width: calc((100vw - 48px) / 5.5);
  }
}
@media (min-width: 1024px) {
  .gf-continue__item {
    width: calc((100vw - 80px) / 6);
  }
}
@media (min-width: 1440px) {
  .gf-continue__item {
    width: calc(min(100vw - 80px, 1280px) / 7);
  }
}

.gf-continue__poster {
  position: relative;
  overflow: hidden;
  border-radius: var(--gf-card-radius);
  background-color: var(--gf-bg-elevated);
  transition:
    transform var(--gf-dur-base) var(--gf-ease-spring),
    box-shadow var(--gf-dur-base) var(--gf-ease-standard);
}
.gf-continue__ep {
  position: absolute;
  top: var(--gf-space-2);
  left: var(--gf-space-2);
  z-index: 2;
  padding: 2px 8px;
  border-radius: var(--gf-radius-sm);
  background-image: var(--gf-brand-gradient);
  color: #fff;
  font-size: var(--gf-fs-xs);
  font-weight: var(--gf-fw-semibold);
  line-height: 1.4;
  white-space: nowrap;
}
.gf-continue__play {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  opacity: 0;
  transform: scale(0.85);
  background-image: var(--gf-mask-card-hover);
  pointer-events: none;
  transition:
    opacity var(--gf-dur-base) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-spring);
}
.gf-continue__bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background-color: var(--gf-progress-bg);
  z-index: 2;
  overflow: hidden;
}
.gf-continue__bar-fill {
  display: block;
  height: 100%;
  background-image: var(--gf-progress-fg);
}
.gf-continue__name {
  margin-top: var(--gf-space-2);
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-primary);
  font-weight: var(--gf-fw-medium);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.gf-continue__sub {
  margin-top: 2px;
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (hover: hover) and (pointer: fine) {
  .gf-continue__item:hover .gf-continue__poster,
  .gf-continue__item:focus-visible .gf-continue__poster {
    transform: scale(1.04);
    box-shadow: var(--gf-shadow-hover);
    position: relative;
    z-index: 3;
  }
  .gf-continue__item:hover .gf-continue__play,
  .gf-continue__item:focus-visible .gf-continue__play {
    opacity: 1;
    transform: scale(1);
  }
}
</style>

<style>
/* TV 模式: 焦点海报放大 + 青光晕(整卡不放大, 文字静止); 横滚条留纵向 padding 防焦点环被裁 */
[data-mode='tv'] .gf-continue__scroll {
  padding-block: 16px;
  gap: 24px;
}
@media (min-width: 768px) {
  [data-mode='tv'] .gf-continue__scroll {
    gap: var(--gf-space-6);
  }
}
[data-mode='tv'] .gf-continue__item {
  width: calc(min(100vw - 96px, 1600px) / 6);
}
[data-mode='tv'] .gf-continue__edge {
  width: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-continue__header.container-page {
  padding-inline: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-continue__title {
  font-size: var(--gf-fs-xl);
}
[data-mode='tv'] .gf-continue__name {
  font-size: var(--gf-fs-base);
}
[data-mode='tv'] .gf-continue__sub {
  font-size: var(--gf-fs-sm);
}
[data-mode='tv'] .gf-continue__more {
  font-size: var(--gf-fs-md);
}
[data-mode='tv'] .gf-continue__item[data-focusable='true']:focus,
[data-mode='tv'] .gf-continue__item[data-focusable='true']:focus-visible {
  outline: none;
  transform: none;
  box-shadow: none;
}
[data-mode='tv'] .gf-continue__item[data-focusable='true']:focus .gf-continue__poster,
[data-mode='tv'] .gf-continue__item[data-focusable='true']:focus-visible .gf-continue__poster {
  transform: scale(var(--gf-tv-focus-scale-card, 1.18));
  box-shadow: var(--gf-tv-focus-ring);
  z-index: 5;
  transition:
    transform var(--gf-dur-focus) var(--gf-ease-spring),
    box-shadow var(--gf-dur-focus) var(--gf-ease-standard);
}
</style>
